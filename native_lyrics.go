package main

import (
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/egoist/mygo/ui"
	"ytmgo/internal/catalog"
)

func (m *UIModel) clearLyrics(message string) {
	m.lyricsMessage = uiMessage{}
	m.Lyrics = message
	m.LyricLines = nil
	m.LyricsSource = ""
	m.LyricsAvailable = false
	m.LyricsRevision++
}

// Only provider-supplied cue ranges determine the active line. In an
// instrumental gap the last line stays in view, without remaining highlighted.
func nativeLyricIndices(lines []catalog.LyricLine, seconds float64) (active, target int) {
	if len(lines) == 0 || math.IsNaN(seconds) || math.IsInf(seconds, 0) || seconds < 0 {
		return -1, -1
	}
	i := sort.Search(len(lines), func(i int) bool { return lines[i].Start.Seconds() > seconds }) - 1
	if i < 0 {
		return -1, 0
	}
	// Duets can have overlapping cues. A later short cue ending must not
	// hide an earlier cue that is still being sung.
	for j := i; j >= 0; j-- {
		if seconds < lines[j].End.Seconds() {
			return j, j
		}
	}
	return -1, i
}

// The general playback model is polled for controls and metadata. Lyrics need
// the audio clock at the moment they are drawn, especially at cue boundaries.
// Never extrapolate wall time: a starved decoder's audible position stands still.
func (a *desktop) lyricsClock() (float64, bool) {
	if a.player == nil || a.loadPending || a.restoredTrack {
		return a.model.Position, false
	}
	state := a.player.Snapshot()
	return state.Position.Seconds(), state.Playing && !state.Loading && state.Error == ""
}

func nativeLyricRefreshDelay(lines []catalog.LyricLine, seconds float64, active int) time.Duration {
	delay := 250 * time.Millisecond
	if math.IsNaN(seconds) || math.IsInf(seconds, 0) || seconds < 0 {
		return delay
	}
	at := time.Duration(seconds * float64(time.Second))
	if active >= 0 {
		delay = min(delay, lines[active].End-at)
	}
	next := sort.Search(len(lines), func(i int) bool { return lines[i].Start > at })
	if next < len(lines) {
		delay = min(delay, lines[next].Start-at)
	}
	return max(10*time.Millisecond, delay)
}

type nativeLyricsFollow struct {
	track          string
	revision       uint64
	follow         bool
	width, height  float32
	language       string
	scroll         ui.ScrollState
	expectedY      float32
	leadingSpace   float32
	trailingSpace  float32
	drag           nativeLyricsDrag
	cueCenters     []float32
	viewportCenter float32
}

// revealLine centers the sung line within the lyrics viewport without moving
// the outer player panel. Oversized lines start at the top for readability.
func (s *nativeLyricsFollow) revealLine(view, line ui.Rect) {
	if view.H <= 0 || line.H <= 0 {
		return
	}
	top := line.Y - view.Y
	y := s.scroll.Y + top - max(0, (view.H-line.H)/2)
	s.scroll.Y = max(0, min(y, s.scroll.MaxY))
	s.expectedY = s.scroll.Y
}

func nativeLyricsBody(c *ui.Context, m *UIModel, a ViewActions, p nativePalette) ui.Element {
	body := ui.Column(c.Key("lyrics-body")).Grow(1).MinHeight(68).Gap(8)
	state := ui.Local(body, "follow", func() nativeLyricsFollow { return nativeLyricsFollow{follow: true} })
	if state.track != m.Current.ID || state.revision != m.LyricsRevision {
		*state = nativeLyricsFollow{track: m.Current.ID, revision: m.LyricsRevision, follow: true}
	}
	width, height := c.Size()
	resized := state.width != width || state.height != height || state.language != m.Language
	if resized || !nativeLyricsCanSeek(m, a) {
		state.cancelDrag()
	}
	// A wheel, key or scrollbar takes over when it moves the lyrics.
	// Layout can also clamp the offset if the content becomes shorter.
	if state.scroll.Y != min(state.expectedY, state.scroll.MaxY) {
		state.follow = false
	}
	state.expectedY = state.scroll.Y
	state.width, state.height, state.language = width, height, m.Language
	return body.Children(func() {
		if len(m.LyricLines) == 0 && m.LyricsAvailable {
			ui.Text(c, a.Tr("nativeLyricsUnsynced")).Shrink(0).FontSize(tokenSizeMicro).TextColor(p.secondary)
		}
		scroll := ui.Scroll(c.Key("lyrics-scroll")).Grow(1).MinHeight(0).Gap(12).TrackScroll(&state.scroll).Focusable().Label(a.Tr("shellLyrics") + " · " + a.Tr("shellContent"))
		scroll.HandleInput(func(ev ui.InputEvent) bool {
			return state.dragInput(ev, m, a)
		})
		// Lines scrolled past an edge fade out instead of being sliced; an edge
		// fades only while there is more beyond it.
		scroll.Effect(nativeEdgeFade{top: state.scroll.Y > 0.5, bottom: state.scroll.Y < state.scroll.MaxY-0.5})
		view := scroll.LayoutBounds()
		visible := view
		if a.viewportScroll != nil {
			visible = nativeIntersection(view, a.artViewport)
		}
		state.viewportCenter = visible.Y - view.Y + visible.H/2
		state.releaseDrag(m, a, scroll)
		if nativeLyricsCanSeek(m, a) {
			scroll.Cursor(ui.CursorGrab).Tooltip(a.Tr("nativeLyricsDragHint"))
			if state.drag.pressed {
				scroll.Cursor(ui.CursorGrabbing)
			}
		}
		position, playing := m.Position, false
		if len(m.LyricLines) > 0 && a.LyricsClock != nil {
			position, playing = a.LyricsClock()
		}
		active, target := nativeLyricIndices(m.LyricLines, position)
		if playing {
			c.After(nativeLyricRefreshDelay(m.LyricLines, position, active))
		}
		scroll.Children(func() {
			if len(m.LyricLines) == 0 {
				lyrics, color := m.Lyrics, p.text
				if lyrics == "" {
					lyrics, color = a.Tr("nativeLyricsUnavailable"), p.secondary
				}
				ui.Text(c, lyrics).FontSize(tokenSizeLineMin).FontWeight(700).LineHeight(1.6).TextColor(color)
			} else {
				// Leave enough room to center even the first and last cues. Keep
				// these spaces fixed during manual reading so resizing the panel
				// does not move the words the reader is inspecting.
				leading := ui.Box(c.Key("lyrics-leading-space")).Height(state.leadingSpace).Shrink(0)
				state.cueCenters = state.cueCenters[:0]
				var first, last, current ui.Rect
				for i, line := range m.LyricLines {
					color := p.secondary
					if i == active {
						color = p.text
					}
					if state.drag.moved && i == state.drag.selected {
						color = p.accent
					}
					row := ui.Text(c.Key(fmt.Sprintf("lyric-%d", i)), line.Text).Shrink(0).FontSize(tokenSizeLineMin).FontWeight(700).LineHeight(1.6).TextColor(color)
					if i == 0 {
						first = row.LayoutBounds()
					}
					last = row.LayoutBounds()
					state.cueCenters = append(state.cueCenters, last.Y-leading.LayoutBounds().Y+last.H/2)
					if i == target {
						current = last
					}
				}
				trailing := ui.Box(c.Key("lyrics-trailing-space")).Height(state.trailingSpace).Shrink(0)
				if state.follow && !state.drag.pressed {
					top, bottom := state.leadingSpace, state.trailingSpace
					if view.H > 0 && first.H > 0 && last.H > 0 {
						top = max(0, (view.H-first.H)/2-12)
						bottom = max(0, (view.H-last.H)/2-12)
					}
					paddingChanged := top != state.leadingSpace || bottom != state.trailingSpace
					state.leadingSpace, state.trailingSpace = top, bottom
					leading.Height(top)
					trailing.Height(bottom)
					if resized || paddingChanged {
						// Center using bounds from the updated layout next frame.
						c.Invalidate()
					} else {
						state.revealLine(visible, current)
					}
				}
			}
			if m.LyricsSource != "" {
				ui.Text(c, m.LyricsSource).FontSize(tokenSizeMicro).TextColor(p.secondary).Shrink(0)
			}
		})
		nativeLyricsDragPreview(c, scroll, state, m, a, p)
		if len(m.LyricLines) > 0 && !state.follow && (!state.drag.pressed || !state.drag.followed) {
			// Put the action below the viewport so the line being read stays put.
			if nativeLinkButton(c, a.Tr("nativeLyricsFollow"), p, "").AlignSelf(ui.End).Shrink(0).Clicked() {
				state.follow = true
				scroll.Focus()
				c.Invalidate()
			}
		}
	})
}

// nativeEdgeFade fades an element's top and bottom edges to transparent, as
// CSS's mask-image does on the website's lyrics: it draws the rendered
// element in strips, each with less alpha toward the edge, so it fades over
// whatever lies behind (a card, or a cover's blur in immersive mode).
type nativeEdgeFade struct{ top, bottom bool }

// nativeEdgeFadeSize is how far into the element an edge fades.
const nativeEdgeFadeSize = 24

func (nativeEdgeFade) EffectMargin() ui.Point { return ui.Point{} }

func (f nativeEdgeFade) PaintEffect(p *ui.Painter, box ui.Rect, content *ui.Bitmap, _ ui.Point) {
	w, h := content.Size()
	if box.H <= 0 || w == 0 || h == 0 {
		return
	}
	px := float32(h) / box.H // bitmap pixels per DIP
	fade := min(float32(nativeEdgeFadeSize), box.H/3)
	top, bottom := float32(0), float32(0)
	if f.top {
		top = fade
	}
	if f.bottom {
		bottom = fade
	}
	band := func(y0, y1, alpha float32) {
		if y1 <= y0 {
			return
		}
		var o ui.ImageOptions
		if alpha < 1 {
			o.Tint = ui.Color{R: 255, G: 255, B: 255, A: uint8(alpha*255 + 0.5)}
		}
		p.ImageRegion(content, ui.Rect{X: 0, Y: y0 * px, W: float32(w), H: (y1 - y0) * px}, ui.Rect{X: box.X, Y: box.Y + y0, W: box.W, H: y1 - y0}, o)
	}
	band(top, box.H-bottom, 1)
	// One strip per DIP: smooth at any scale, and few enough to be cheap.
	for i := float32(0); i < top; i++ {
		band(i, min(i+1, top), (i+0.5)/top)
	}
	for i := float32(0); i < bottom; i++ {
		band(box.H-bottom+i, min(box.H-bottom+i+1, box.H), 1-(i+0.5)/bottom)
	}
}
