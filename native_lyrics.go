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
	last          int
	width, height float32
	language      string
	scroll        ui.ScrollState
	observedY     float32
}

func nativeLyricsBody(c *ui.Context, m *UIModel, a ViewActions, p nativePalette) ui.Element {
	body := ui.Column(c.Key("lyrics-body")).Grow(1).MinHeight(68).Gap(8)
	state := ui.Local(body, "follow", func() nativeLyricsFollow { return nativeLyricsFollow{last: -2, follow: true} })
	if state.track != m.Current.ID || state.revision != m.LyricsRevision {
		*state = nativeLyricsFollow{track: m.Current.ID, revision: m.LyricsRevision, follow: true, last: -2}
	}
	width, height := c.Size()
	resized := state.width != width || state.height != height || state.language != m.Language
	// Native keyboard scrolling can target the hovered scroll area while the
	// keyboard focus is on a sibling button, bypassing this area's input handler.
	// Observe actual offsets too. DrawOver records the final laid-out offset,
	// after ScrollIntoView, so automatic reveals are not reader gestures.
	if !resized && state.scroll.Y != state.observedY {
		state.follow = false
	}
	state.observedY = state.scroll.Y
	return body.Children(func() {
		if len(m.LyricLines) > 0 {
			ui.Row(c).Shrink(0).Children(func() {
				key := "nativeLyricsFollow"
				if state.follow {
					key = "nativeLyricsFollowing"
				}
				if nativeChip(c, a.Tr(key), p, state.follow).Clicked() {
					state.follow, state.last = true, -2
				}
			})
		} else if m.LyricsAvailable {
			ui.Text(c, a.Tr("nativeLyricsUnsynced")).Shrink(0).FontSize(12).TextColor(p.secondary)
		}
		scroll := ui.Scroll(c.Key("lyrics-scroll")).Grow(1).MinHeight(0).Gap(12).TrackScroll(&state.scroll).Focusable().Label(a.Tr("shellLyrics") + " · " + a.Tr("shellContent"))
		scroll.DrawOver(func(_ *ui.Painter, _ ui.Rect) { state.observedY = state.scroll.Y })
		scroll.HandleInput(func(ev ui.InputEvent) bool {
			// A wheel, trackpad or scrollbar gesture gives control to the reader.
			if ev.Kind == ui.InputScroll && ev.DY != 0 || ev.Kind == ui.InputPointerDown {
				state.follow = false
			}
			if ev.Kind == ui.InputKeyDown {
				switch ev.Key {
				case ui.KeyUp, ui.KeyDown, ui.KeyPageUp, ui.KeyPageDown, ui.KeyHome, ui.KeyEnd:
					state.follow = false
				}
			}
			return false
		})
		active, target := nativeLyricIndices(m.LyricLines, m.Position)
		reveal := state.follow && target >= 0 && (state.last != target || resized)
		state.width, state.height, state.language = width, height, m.Language
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
					if reveal && i == target {
						row.ScrollIntoView()
					}
				}
			}
			if m.LyricsSource != "" {
				ui.Text(c, m.LyricsSource).FontSize(12).TextColor(p.secondary).Shrink(0)
			}
		})
		if reveal {
			state.last = target
			c.Invalidate()
		}
	})
}
