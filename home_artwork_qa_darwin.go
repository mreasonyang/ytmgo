//go:build darwin && ytmgo_artwork_qa

package main

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"golang.org/x/sys/unix"
)

const (
	homeQATimeout   = 60 * time.Second
	homeQAFrame     = 80 * time.Millisecond
	homeQAAwayDwell = 240 * time.Millisecond
	homeQABackDwell = 480 * time.Millisecond
	homeQAMinHot    = 3
	homeQAMaxHot    = 256
	homeQATolerance = 1
)

type homeQAPhase uint8

const (
	homeQAWarm homeQAPhase = iota
	homeQAAway
	homeQABack
)

type homeQAPaint uint8

const (
	homeQANote homeQAPaint = 1 << iota
	homeQAImage
)

type homeQARound struct {
	Offscreen int `json:"offscreen"`
	Reentered int `json:"reentered"`
	Notes     int `json:"notes"`
}

type homeQAResult struct {
	State      string         `json:"state"`
	Reason     string         `json:"reason"`
	Phase      homeQAPhase    `json:"phase"`
	InitialHot int            `json:"initial_hot"`
	InitialY   float32        `json:"initial_y"`
	FinalY     float32        `json:"final_y"`
	MaxDeltaY  float32        `json:"max_delta_y"`
	Restored   int            `json:"restored"`
	Completed  int            `json:"completed_rounds"`
	Rounds     [2]homeQARound `json:"rounds"`
}

type homeQACover struct {
	node     ui.Element // used only in the current committed pass
	left     bool
	returned homeQAPaint
}

// This probe runs on the UI thread. Only the current page's picture nodes
// feed it, and only the published aggregate leaves the process.
type homeArtworkQA struct {
	result  homeQAResult
	finish  func(homeQAResult)
	ended   bool
	phase   homeQAPhase
	round   int
	settled time.Time

	page, scrollerID               uint64
	account                        uint64
	pageBounds                     ui.Rect
	anchorX, lastY, targetY        float32
	move, frameReady, paintedFrame bool
	scroll, anchor                 *ui.ScrollState
	viewport                       ui.Element
	covers                         map[uint64]homeQACover
	previousHot, paint             map[uint64]homeQAPaint
	waiting                        string
}

func (a *desktop) startHomeArtworkQA() {
	if buildKind != "development" || len(os.Args) != 2 {
		return
	}
	dirName, ok := strings.CutPrefix(os.Args[1], "--qa-home-artwork=")
	if !ok {
		return
	}
	dir := openHomeQADir(dirName)
	if dir == nil {
		return
	}
	q := &homeArtworkQA{waiting: "home_not_ready"}
	a.homeArtworkQA = q
	timer := time.AfterFunc(homeQATimeout, func() {
		a.post(func() {
			reason := "timeout"
			if q.phase == homeQAWarm {
				reason = q.waiting
			}
			q.end("inconclusive", reason)
		})
	})
	windowChanged := func() {
		if q.phase != homeQAWarm {
			q.end("inconclusive", "window_changed")
		}
		clear(q.previousHot)
	}
	w := a.window
	stop := []func(){
		w.OnBlur(windowChanged), w.OnHide(windowChanged), w.OnMinimize(windowChanged),
		w.OnMove(windowChanged), w.OnResize(windowChanged),
		w.OnEnterFullScreen(windowChanged), w.OnLeaveFullScreen(windowChanged),
		w.OnFocus(a.invalidate),
		w.OnClose(func(*mygo.CloseEvent) { q.end("inconclusive", "window_changed") }),
		mygo.App.OnBeforeQuit(func(*mygo.QuitEvent) { q.end("inconclusive", "window_changed") }),
	}
	q.finish = func(result homeQAResult) {
		a.homeArtworkQA = nil
		timer.Stop()
		for _, off := range stop {
			off()
		}
		writeHomeQAResult(dir, result)
		dir.Close()
	}
}

func (a *desktop) prepareHomeArtworkQA(c *ui.Context, actions *ViewActions) {
	q := a.homeArtworkQA
	if q == nil {
		return
	}
	if a.model.Route != "home" {
		q.end("inconclusive", "page_changed")
		return
	}
	actions.homeArtworkQA = q // also observe real input while waiting for home
	width, _ := c.Size()
	now := c.Now()
	page, account := a.pageGeneration, a.accountGeneration
	blocked := func() string {
		m := &a.model
		switch {
		case m.Route != "home" || a.pageGeneration != page || a.accountGeneration != account:
			return "page_changed"
		case !a.windowVisible || !a.window.IsFocused() || m.Immersive ||
			nativeFloatingWanted(m, width) || m.floating.arrival(now) > 0 && width <= 1180 ||
			m.VideoFullScreen || m.Settings || m.Shortcuts || m.About || m.confirm != nil ||
			m.barMenu || m.sheetMenu || m.accountMenu:
			return "window_not_ready"
		case len(m.Page.Shelves)+len(m.Page.Items) == 0:
			return "home_not_ready"
		case q.phase == homeQAWarm && (m.Loading || a.accountBusy || nativeRestoring(m)):
			return "home_not_ready"
		}
		// Normal pagination can continue after the starting page is ready.
		return ""
	}
	q.prepareFrame(c, blocked, page, account)
}

func (q *homeArtworkQA) prepareFrame(c *ui.Context, blocked func() string, page, account uint64) {
	q.scroll, q.viewport = nil, ui.Element{}
	q.frameReady, q.paintedFrame = false, false
	if q.phase != homeQAWarm && (q.page != page || q.account != account) {
		q.end("inconclusive", "page_changed")
		return
	}
	if reason := blocked(); reason != "" {
		if q.phase != homeQAWarm {
			q.end("inconclusive", reason)
		}
		q.waiting = reason
		clear(q.previousHot)
		return
	}
	if q.page != page || q.account != account {
		clear(q.previousHot)
		q.page, q.account = page, account
	}
	if q.paint == nil {
		q.paint = make(map[uint64]homeQAPaint)
	}
	q.frameReady = true
	q.waiting = "insufficient_hot"
	clear(q.paint)
	now := c.Now()
	c.After(homeQAFrame)
	c.Root().DrawOver(func(*ui.Painter, ui.Rect) {
		if q.ended || q.paintedFrame {
			return
		}
		q.paintedFrame = true
		if reason := blocked(); reason == "" {
			q.afterPaint(now)
		} else if q.phase != homeQAWarm {
			q.end("inconclusive", reason)
		} else {
			q.waiting = reason
			clear(q.previousHot)
		}
	})
}

func (q *homeArtworkQA) observeInput(root ui.Element) {
	if q == nil || q.ended {
		return
	}
	root.ObserveInput(func(event ui.InputEvent) {
		nativeReleaseScrollGrabs(event)
		switch event.Kind {
		case ui.InputKeyDown, ui.InputPointerDown, ui.InputCommand:
			q.manualInput()
		}
	})
	// MyGo routes wheel and trackpad events through HandleInput. Observe
	// them without consuming the event or replacing native scroll behavior.
	root.HandleInput(func(event ui.InputEvent) bool {
		if event.Kind == ui.InputScroll || event.Kind == ui.InputZoom {
			q.manualInput()
		}
		return false
	})
}

func (q *homeArtworkQA) manualInput() {
	if q != nil {
		q.end("inconclusive", "manual_input")
	}
}

func (q *homeArtworkQA) trackPage(view ui.Element, scroll *ui.ScrollState) *homeArtworkQA {
	if q == nil || q.ended || !q.frameReady {
		return nil
	}
	if q.phase != homeQAWarm {
		if scroll != q.anchor || view.ID() != q.scrollerID {
			q.end("inconclusive", "page_changed")
			return nil
		}
		if scroll.Scrolling() || scroll.Y != q.lastY || scroll.X != q.anchorX || len(nativeScrollGrabs) > 0 {
			q.end("inconclusive", "scroll_changed")
			return nil
		}
		if q.move {
			// TrackScroll adopts this request during layout. The next paint
			// checks the actual committed offset, including any clamp.
			scroll.Y = q.targetY
			q.lastY, q.move = q.targetY, false
		}
	}
	q.viewport, q.scroll = view, scroll
	return q
}

func (q *homeArtworkQA) trackCover(view ui.Element) {
	if q == nil || q.ended || q.phase == homeQAWarm {
		return
	}
	id := view.ID()
	if cover, ok := q.covers[id]; ok {
		cover.node = view
		q.covers[id] = cover
	}
}

func (q *homeArtworkQA) paintedCover(view ui.Element, image bool) {
	if q == nil || q.ended || q.paintedFrame || !view.InViewAmount(0.6) {
		return
	}
	id := view.ID()
	if q.phase == homeQAWarm {
		if image && len(q.paint) < homeQAMaxHot {
			q.paint[id] = homeQAImage
		}
		return
	}
	if q.phase == homeQABack && q.covers[id].left {
		if image {
			q.paint[id] |= homeQAImage
		} else {
			q.paint[id] |= homeQANote
		}
	}
}

func (q *homeArtworkQA) afterPaint(now time.Time) {
	if q.scroll == nil || !q.viewport.Valid() || !q.viewport.InView() {
		if q.phase != homeQAWarm {
			q.end("inconclusive", "window_changed")
		}
		clear(q.previousHot)
		return
	}
	bounds, y := q.viewport.Bounds(), q.scroll.Y
	if q.phase == homeQAWarm {
		q.warm(bounds, y)
		return
	}
	q.result.FinalY = y
	q.result.MaxDeltaY = max(q.result.MaxDeltaY, y-q.result.InitialY)
	if !homeQASameBounds(q.pageBounds, bounds) {
		q.end("inconclusive", "window_changed")
		return
	}
	if !homeQANear(y, q.targetY) || !homeQANear(q.scroll.X, q.anchorX) || q.scroll.Scrolling() {
		q.end("inconclusive", "scroll_changed")
		return
	}
	q.lastY = y
	if q.settled.IsZero() {
		q.settled = now
	}
	round := &q.result.Rounds[q.round]
	if q.phase == homeQAAway {
		for id, cover := range q.covers {
			if cover.left || !cover.node.Valid() || cover.node.InView() {
				continue
			}
			if box := cover.node.Bounds(); box.H > 0 && box.Y+box.H <= bounds.Y {
				cover.left = true
				q.covers[id] = cover
				round.Offscreen++
			}
		}
		if now.Sub(q.settled) >= homeQAAwayDwell {
			q.travel(homeQABack, q.result.InitialY)
		}
		return
	}
	q.result.Restored = 1
	for id, paint := range q.paint {
		cover := q.covers[id]
		if cover.returned == 0 {
			round.Reentered++
		}
		if paint&homeQANote != 0 && cover.returned&homeQANote == 0 {
			round.Notes++
		}
		cover.returned |= paint
		q.covers[id] = cover
	}
	if now.Sub(q.settled) < homeQABackDwell {
		return
	}
	q.result.Completed++
	q.round++
	if q.round == len(q.result.Rounds) {
		state, reason := "pass", "complete"
		for _, sample := range q.result.Rounds {
			if sample.Offscreen == 0 {
				state, reason = "inconclusive", "no_offscreen_samples"
				break
			}
			if sample.Reentered != sample.Offscreen {
				state, reason = "inconclusive", "missing_return_samples"
				break
			}
			if sample.Notes > 0 {
				state, reason = "fail", "notes_shown"
			}
		}
		q.end(state, reason)
		return
	}
	for id := range q.covers {
		q.covers[id] = homeQACover{}
	}
	q.down(bounds)
}

func (q *homeArtworkQA) warm(bounds ui.Rect, y float32) {
	if q.scroll.Scrolling() || len(nativeScrollGrabs) > 0 {
		clear(q.previousHot)
		return
	}
	if !homeQANear(y, q.lastY) || q.scrollerID != q.viewport.ID() || !homeQASameBounds(q.pageBounds, bounds) {
		clear(q.previousHot)
	}
	q.lastY, q.scrollerID, q.pageBounds = y, q.viewport.ID(), bounds
	if len(q.previousHot) >= homeQAMinHot && len(q.paint) >= homeQAMinHot {
		covers := make(map[uint64]homeQACover)
		for id := range q.paint {
			if q.previousHot[id] != 0 {
				covers[id] = homeQACover{}
			}
		}
		if len(covers) >= homeQAMinHot {
			q.covers, q.anchor, q.anchorX = covers, q.scroll, q.scroll.X
			q.result.InitialHot, q.result.InitialY, q.result.FinalY = len(covers), y, y
			q.down(bounds)
			return
		}
	}
	if q.previousHot == nil {
		q.previousHot = make(map[uint64]homeQAPaint)
	}
	q.previousHot, q.paint = q.paint, q.previousHot
}

func (q *homeArtworkQA) down(bounds ui.Rect) {
	target := min(q.scroll.MaxY, q.result.InitialY+bounds.H+48)
	if target-q.result.InitialY <= homeQATolerance {
		q.end("inconclusive", "insufficient_scroll")
		return
	}
	q.result.Restored = 0
	q.travel(homeQAAway, target)
}

func (q *homeArtworkQA) travel(phase homeQAPhase, y float32) {
	q.phase, q.targetY, q.move, q.settled = phase, y, true, time.Time{}
}

func (q *homeArtworkQA) end(state, reason string) {
	if q.ended {
		return
	}
	q.ended, q.result.State, q.result.Reason, q.result.Phase = true, state, reason, q.phase
	q.covers, q.paint, q.previousHot = nil, nil, nil
	if q.finish != nil {
		finish := q.finish
		q.finish = nil
		finish(q.result)
	}
}

func homeQANear(a, b float32) bool { return a-b <= homeQATolerance && b-a <= homeQATolerance }

func homeQASameBounds(a, b ui.Rect) bool {
	return homeQANear(a.X, b.X) && homeQANear(a.Y, b.Y) &&
		homeQANear(a.W, b.W) && homeQANear(a.H, b.H)
}

func openHomeQADir(path string) *os.File {
	if filepath.Clean(path) != path || filepath.Dir(path) != "/private/tmp" ||
		!strings.HasPrefix(filepath.Base(path), "ytmgo-home-qa.") {
		return nil
	}
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil
	}
	dir := os.NewFile(uintptr(fd), path)
	var stat unix.Stat_t
	if unix.Fstat(fd, &stat) != nil || stat.Uid != uint32(os.Geteuid()) || stat.Mode&0777 != 0700 {
		dir.Close()
		return nil
	}
	if _, err := dir.Readdirnames(1); err != io.EOF {
		dir.Close()
		return nil
	}
	return dir
}

func writeHomeQAResult(dir *os.File, result homeQAResult) {
	parent := int(dir.Fd())
	const pending = "result.pending"
	fd, err := unix.Openat(parent, pending, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0600)
	if err != nil {
		return
	}
	defer unix.Unlinkat(parent, pending, 0)
	f := os.NewFile(uintptr(fd), pending)
	writeErr := json.NewEncoder(f).Encode(result)
	if closeErr := f.Close(); writeErr == nil && closeErr == nil {
		// Publish atomically without replacing any existing file or symlink.
		_ = unix.Linkat(parent, pending, parent, "result.json", 0)
	}
}
