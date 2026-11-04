package main

import (
	"math"

	"github.com/egoist/mygo/ui"
)

type nativeLyricsDrag struct {
	pressed, moved, followed, released bool
	x, y, offset                       float32
	selected                           int
}

func nativeLyricsCanSeek(m *UIModel, a ViewActions) bool {
	return a.Seek != nil && m.Current.ID != "" && len(m.LyricLines) > 0 &&
		m.Duration > 0 && !math.IsNaN(m.Duration) && !math.IsInf(m.Duration, 0) && !m.PlaybackLoading
}

func (s *nativeLyricsFollow) cancelDrag() {
	if s.drag.pressed || s.drag.released {
		s.follow = s.drag.followed
	}
	s.drag = nativeLyricsDrag{}
}

// Use measured cue centers, including wrapped lines. Their coordinates are
// relative to the leading spacer, so scrolling never makes this geometry stale.
func (s *nativeLyricsFollow) selectDraggedLyric(m *UIModel) {
	s.drag.selected = -1
	nearest := float32(math.MaxFloat32)
	center := s.scroll.Y + s.viewportCenter
	for i, at := range s.cueCenters {
		if i >= len(m.LyricLines) || at <= 0 {
			continue
		}
		line := m.LyricLines[i]
		if line.Start < 0 || line.End <= line.Start || line.Start.Seconds() >= m.Duration {
			continue
		}
		distance := float32(math.Abs(float64(at - center)))
		if distance < nearest {
			s.drag.selected, nearest = i, distance
		}
	}
}

func (s *nativeLyricsFollow) dragInput(ev ui.InputEvent, m *UIModel, a ViewActions) bool {
	if ev.Kind == ui.InputScroll {
		// Wheel/trackpad scrolling remains available for reading. A held drag
		// owns its offset until release, including outside the viewport.
		return s.drag.pressed || math.Abs(float64(ev.DX)) > math.Abs(float64(ev.DY))
	}
	if s.track != m.Current.ID || s.revision != m.LyricsRevision || !nativeLyricsCanSeek(m, a) {
		s.cancelDrag()
		return false
	}
	switch ev.Kind {
	case ui.InputPointerDown:
		if ev.Button != 0 {
			return false
		}
		s.drag = nativeLyricsDrag{pressed: true, followed: s.follow, x: ev.X, y: ev.Y, offset: s.scroll.Y, selected: -1}
		return true
	case ui.InputPointerMove:
		if !s.drag.pressed || ev.Button != 0 {
			return false
		}
		dx, dy := ev.X-s.drag.x, ev.Y-s.drag.y
		if !s.drag.moved && (math.Abs(float64(dy)) < 6 || math.Abs(float64(dy)) < math.Abs(float64(dx))) {
			return true
		}
		s.drag.moved, s.follow = true, false
		s.scroll.Y = max(0, min(s.drag.offset-dy, s.scroll.MaxY))
		s.expectedY = s.scroll.Y
		s.selectDraggedLyric(m)
		return true
	case ui.InputPointerUp:
		if !s.drag.pressed || ev.Button != 0 {
			return false
		}
		s.drag.pressed, s.drag.released = false, true
		return true
	case ui.InputKeyDown:
		if s.drag.pressed && ev.Key == ui.KeyEscape {
			s.cancelDrag()
			return true
		}
	}
	return false
}

func (s *nativeLyricsFollow) releaseDrag(m *UIModel, a ViewActions, scroll ui.Element) {
	if !s.drag.released {
		return
	}
	drag := s.drag
	s.cancelDrag()
	// Losing window focus synthesizes a release. Check focus in the new
	// frame; Elements cannot be read from an input callback between frames.
	if drag.moved && scroll.Focused() && nativeLyricsCanSeek(m, a) && drag.selected >= 0 && drag.selected < len(m.LyricLines) {
		seconds := m.LyricLines[drag.selected].Start.Seconds()
		s.follow = true
		m.Position = seconds
		a.Seek(seconds)
	}
}

func nativeLyricsDragPreview(c *ui.Context, scroll ui.Element, s *nativeLyricsFollow, m *UIModel, a ViewActions, p nativePalette) {
	if !s.drag.moved || s.drag.selected < 0 || s.drag.selected >= len(m.LyricLines) {
		return
	}
	scroll.Draw(func(painter *ui.Painter, r ui.Rect) {
		painter.Fill(ui.Rect{X: r.X, Y: r.Y + s.viewportCenter, W: r.W, H: 1}, p.accent.Alpha(0.25), 0)
	})
	ui.Overlay(c, func() {
		text := nativeTime(m.LyricLines[s.drag.selected].Start.Seconds()) + " · " + a.Tr("nativeLyricsReleaseToSeek")
		ui.Text(c, text).AttachTo(scroll, ui.AnchorTopRight, ui.AnchorTopRight).Margin(4, 8, 0, 0).
			Padding(4, 8).Radius(4).Background(p.raised).TextColor(p.text).FontSize(12).FontWeight(600).
			Shadow(0, 2, 8, 0, ui.RGBA(0, 0, 0, 0.25)).PassThrough().Role(ui.RoleTooltip)
	})
}
