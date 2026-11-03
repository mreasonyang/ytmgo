package main

import (
	"fmt"
	"math"
	"sort"

	"github.com/egoist/mygo/ui"
	"ytmgo/internal/catalog"
)

func (m *UIModel) clearLyrics(message string) {
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

type nativeLyricsFollow struct {
	track         string
	revision      uint64
	follow        bool
	width, height float32
	language      string
	scroll        ui.ScrollState
	expectedY     float32
}

// revealLine moves only the lyrics viewport, even when it is below the
// fold in a scrolling player panel. A line taller than the viewport
// starts at the top so its first words remain readable.
func (s *nativeLyricsFollow) revealLine(view, line ui.Rect) {
	if view.H <= 0 || line.H <= 0 {
		return
	}
	top := line.Y - view.Y
	lastTop := max(0, view.H-line.H)
	y := s.scroll.Y
	if top < 0 {
		y += top
	} else if top > lastTop {
		y += top - lastTop
	}
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
	// A wheel, key or scrollbar takes over when it moves the lyrics.
	// Layout can also clamp the offset if the content becomes shorter.
	if state.scroll.Y != min(state.expectedY, state.scroll.MaxY) {
		state.follow = false
	}
	state.expectedY = state.scroll.Y
	state.width, state.height, state.language = width, height, m.Language
	return body.Children(func() {
		if len(m.LyricLines) == 0 && m.LyricsAvailable {
			ui.Text(c, a.Tr("nativeLyricsUnsynced")).Shrink(0).FontSize(12).TextColor(p.secondary)
		}
		scroll := ui.Scroll(c.Key("lyrics-scroll")).Grow(1).MinHeight(0).Gap(12).TrackScroll(&state.scroll).Focusable().Label(a.Tr("shellLyrics") + " · " + a.Tr("shellContent"))
		scroll.HandleInput(func(ev ui.InputEvent) bool {
			return ev.Kind == ui.InputScroll && math.Abs(float64(ev.DX)) > math.Abs(float64(ev.DY))
		})
		active, target := nativeLyricIndices(m.LyricLines, m.Position)
		scroll.Children(func() {
			if len(m.LyricLines) == 0 {
				lyrics, color := m.Lyrics, p.text
				if lyrics == "" {
					lyrics, color = a.Tr("nativeLyricsUnavailable"), p.secondary
				}
				ui.Text(c, lyrics).FontSize(20).FontWeight(700).LineHeight(1.6).TextColor(color)
			} else {
				for i, line := range m.LyricLines {
					color := p.secondary
					if i == active {
						color = p.text
					}
					row := ui.Text(c.Key(fmt.Sprintf("lyric-%d", i)), line.Text).Shrink(0).FontSize(20).FontWeight(700).LineHeight(1.6).TextColor(color)
					if state.follow && i == target {
						if resized {
							// Bounds from this layout are available next frame.
							c.Invalidate()
						} else {
							state.revealLine(scroll.LayoutBounds(), row.LayoutBounds())
						}
					}
				}
			}
			if m.LyricsSource != "" {
				ui.Text(c, m.LyricsSource).FontSize(12).TextColor(p.secondary).Shrink(0)
			}
		})
		if len(m.LyricLines) > 0 && !state.follow {
			// Put the action below the viewport so the line being read stays put.
			if nativeLinkButton(c, a.Tr("nativeLyricsFollow"), p, "").AlignSelf(ui.End).Shrink(0).Clicked() {
				state.follow = true
				scroll.Focus()
				c.Invalidate()
			}
		}
	})
}
