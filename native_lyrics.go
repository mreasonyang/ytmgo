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
	track         string
	revision      uint64
	follow        bool
	width, height float32
	language      string
	scroll        ui.ScrollState
	expectedY     float32
	leadingSpace  float32
	trailingSpace float32
	drag          nativeLyricsDrag
	cueCenters    []float32
	viewportFocus float32 // the focus line's height in the scroller (nativeLyricsFocus of what shows)
	// glideTo is where a glide to the sung line goes; gliding holds while it
	// does, so its steps are not taken for the reader scrolling.
	glideTo float32
	gliding bool
	// placed reports whether the current layout has positioned the sung line.
	placed bool
}

// nativeLyricsFocus is where the sung line rests: a third of the way down,
// as Apple Music's and Spotify's do, so two thirds of the lyrics show what
// is still to come. The lyrics start at the top and stay put until the
// sung line reaches it, rather than opening on a half-empty card.
const nativeLyricsFocus = float32(1) / 3

// revealLine glides the sung line, top in the content and h high, to the
// focus line of a viewport viewH high (never above the lyrics' top),
// without moving the outer player panel; an oversized line starts at the
// top for readability. A new line glides
// over the slow motion, as the website's lyrics do; while one glides, it
// is not asked again. On first layout or resize, place the line immediately
// if the lyrics are at rest. A glide already in progress follows the new center.
func (s *nativeLyricsFollow) revealLine(viewH, top, h float32) bool {
	if viewH <= 0 || h <= 0 {
		return false
	}
	y := max(0, min(top-max(0, viewH*nativeLyricsFocus-h/2), s.scroll.MaxY))
	if !s.placed {
		s.placed = true
		if !s.scroll.Scrolling() {
			s.scroll.Y, s.expectedY = y, y
			return true
		}
		// ScrollTo still owns the offset while it animates. Retarget that
		// glide below so its next step is recognized as automatic scrolling.
	}
	if s.gliding {
		if math.Abs(float64(y-s.glideTo)) < 0.5 {
			return false
		}
	} else if math.Abs(float64(y-s.scroll.Y)) < 0.5 {
		return false
	}
	s.glideTo, s.gliding = y, true
	s.scroll.ScrollTo(s.scroll.X, y, ui.Animation{Duration: tokenMotionSlow, Ease: tokenMotionEase})
	return true
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
	// A wheel, key or scrollbar takes over when it moves the lyrics (and
	// stops a glide). A glide's own steps, and the one that ends it on its
	// line, are not the reader. Layout can also clamp the offset if the
	// content becomes shorter.
	moved := state.scroll.Y != min(state.expectedY, state.scroll.MaxY)
	onLine := math.Abs(float64(state.scroll.Y-min(state.glideTo, state.scroll.MaxY))) < 1
	glided := state.gliding && (state.scroll.Scrolling() || onLine)
	// A glide that ended away from its line was turned elsewhere, by a key.
	turned := state.gliding && !state.scroll.Scrolling() && !onLine
	if moved && !glided || turned {
		state.follow = false
	}
	if !state.scroll.Scrolling() {
		state.gliding = false
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
		nativeScrollbar(c, scroll, &state.scroll, p, nil, func() {
			// Keyboard scrolling uses the same ScrollTo as playback. Give
			// the reader control as soon as their key starts a scroll.
			state.follow, state.gliding = false, false
		})
		// Lines scrolled past an edge fade out instead of being sliced; an edge
		// fades only while there is more beyond it.
		scroll.Effect(nativeEdgeFade{top: state.scroll.Y > 0.5, bottom: state.scroll.Y < state.scroll.MaxY-0.5})
		view := scroll.LayoutBounds()
		visible := view
		if a.viewportScroll != nil {
			visible = nativeIntersection(view, a.artViewport)
		}
		state.viewportFocus = visible.Y - view.Y + visible.H*nativeLyricsFocus
		state.releaseDrag(m, a, scroll)
		if nativeLyricsCanSeek(m, a) {
			// The grab cursor says it drags; a tip over the lyrics would cover
			// the chips above them as the reader rests on a line.
			scroll.Cursor(ui.CursorGrab).Description(a.Tr("nativeLyricsDragHint"))
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
				// The lyrics start at the top; below, leave room for the last cue
				// to reach the focus line. Keep these spaces fixed during manual
				// reading so resizing the panel does not move the words the
				// reader is inspecting.
				leading := ui.Box(c.Key("lyrics-leading-space")).Height(state.leadingSpace).Shrink(0)
				state.cueCenters = state.cueCenters[:0]
				var first, last, current ui.Rect
				for i, line := range m.LyricLines {
					// Sung lines muted, the current one text, the rest dim.
					color := p.dim
					switch {
					case i == active:
						color = p.text
					case i < active:
						color = p.muted
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
						top = 0
						bottom = max(0, view.H*(1-nativeLyricsFocus)-last.H/2-12)
					}
					paddingChanged := top != state.leadingSpace || bottom != state.trailingSpace
					state.leadingSpace, state.trailingSpace = top, bottom
					leading.Height(top)
					trailing.Height(bottom)
					if resized || paddingChanged {
						// Place using bounds from the updated layout next frame.
						state.placed = false
						c.Invalidate()
					} else if state.revealLine(visible.H, current.Y-leading.LayoutBounds().Y-(visible.Y-view.Y), current.H) {
						// The lyrics move themselves, not the reader: no bar.
						nativeScrollbarFollowed(scroll, &state.scroll)
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
