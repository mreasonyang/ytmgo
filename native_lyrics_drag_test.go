package main

import (
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func TestNativeLyricsDragSeeksLastSelectionOnlyOnRelease(t *testing.T) {
	for _, playing := range []bool{true, false} {
		for _, width := range []int{360, 240} {
			t.Run(fmt.Sprintf("playing_%t_width_%d", playing, width), func(t *testing.T) {
				m, a := nativeTimedLyricsFixture()
				m.Position, m.Playing = 41, playing
				var seeks []float64
				a.Seek = func(at float64) { seeks = append(seeks, at) }
				tt := ui.NewTester(func(c *ui.Context) { nativePanelCard(c, m, a, palette(true)).Grow(1) }, width, 340)
				v, _ := tt.Find("Lyrics · Content")
				r := requireLyricCentered(t, tt, 8)
				next, _ := tt.Find("Synthetic lyric line 09")
				step := next.Y - r.Y
				x, y := v.X+v.W/2, v.Y+v.H/2
				tt.Press(x, y)
				tt.Move(x, y-3*step)
				// Changing the clock must not fight the pointer or commit early.
				m.Position = 71
				tt.Frame()
				if len(seeks) != 0 || m.Position != 71 {
					t.Fatal("preview sought before release")
				}
				if !tt.HasText("0:55 · " + a.Tr("nativeLyricsReleaseToSeek")) {
					t.Fatal("preview does not identify the selected cue time")
				}
				requireLyricCentered(t, tt, 11)
				tt.Move(x, y-step) // The last selection wins, not the farthest one.
				requireLyricCentered(t, tt, 9)
				nativeSnapshot(t, tt, fmt.Sprintf("lyrics-drag-preview-%d", width))
				tt.Release(x, y-step)
				if len(seeks) != 1 || seeks[0] != 45 || m.Position != 45 || m.Playing != playing {
					t.Fatalf("release changed playback incorrectly: seeks=%v position=%v playing=%t", seeks, m.Position, m.Playing)
				}
				requireLyricCentered(t, tt, 9)
				if tt.HasText(a.Tr("nativeLyricsFollow")) || tt.HasText("0:45 · "+a.Tr("nativeLyricsReleaseToSeek")) {
					t.Fatal("release did not dismiss preview and resume following")
				}
				m.Position = 101
				tt.Frame()
				requireLyricCentered(t, tt, 20)
				if len(seeks) != 1 {
					t.Fatal("later frames repeated the seek")
				}
			})
		}
	}
}

func TestNativeLyricsDragReleaseOutsideAndEndpoints(t *testing.T) {
	for _, dy := range []float32{-10000, 10000} {
		m, a := nativeTimedLyricsFixture()
		m.Position = 41
		var seeks []float64
		a.Seek = func(at float64) { seeks = append(seeks, at) }
		tt := ui.NewTester(func(c *ui.Context) { nativePanelCard(c, m, a, palette(true)).Grow(1) }, 360, 340)
		v, _ := tt.Find("Lyrics · Content")
		x, y := v.X+v.W/2, v.Y+v.H/2
		tt.Press(x, y)
		tt.Move(x, y+dy)
		if len(seeks) != 0 {
			t.Fatal("leaving the viewport committed a held drag")
		}
		tt.Release(x, y+dy)
		want := 0.0
		if dy < 0 {
			want = 145
		}
		if len(seeks) != 1 || seeks[0] != want {
			t.Fatalf("outside release: seeks=%v, want %v", seeks, want)
		}
		requireLyricCentered(t, tt, int(want/5))
	}
}

func TestNativeLyricsDragCanceledAndUnavailable(t *testing.T) {
	for _, change := range []string{"track", "revision", "loading", "no duration", "no callback", "blur", "resize", "escape"} {
		t.Run(change, func(t *testing.T) {
			m, a := nativeTimedLyricsFixture()
			m.Position = 41
			seeks := 0
			a.Seek = func(float64) { seeks++ }
			tt := ui.NewTester(func(c *ui.Context) { nativePanelCard(c, m, a, palette(true)).Grow(1) }, 360, 340)
			v, _ := tt.Find("Lyrics · Content")
			x, y := v.X+v.W/2, v.Y+v.H/2
			tt.Press(x, y)
			tt.Move(x, y-88)
			switch change {
			case "track":
				m.Current.ID = "next-track"
			case "revision":
				m.LyricsRevision++
			case "loading":
				m.PlaybackLoading = true
			case "no duration":
				m.Duration = 0
			case "no callback":
				a.Seek = nil
			case "blur":
				tt.SetFocused(false)
			case "resize":
				tt.SetSize(240, 280)
			case "escape":
				tt.Key(0, ui.KeyEscape)
			}
			tt.Frame()
			tt.Release(x, y-88)
			if seeks != 0 || m.Position != 41 {
				t.Fatalf("canceled gesture sought: calls=%d position=%v", seeks, m.Position)
			}
		})
	}
	for _, unavailable := range []string{"plain", "loading", "duration", "callback", "invalid cues"} {
		t.Run(unavailable, func(t *testing.T) {
			m, a := nativeTimedLyricsFixture()
			seeks := 0
			a.Seek = func(float64) { seeks++ }
			switch unavailable {
			case "plain":
				m.clearLyrics("Plain lyrics without timestamps")
			case "loading":
				m.PlaybackLoading = true
			case "duration":
				m.Duration = math.NaN()
			case "callback":
				a.Seek = nil
			case "invalid cues":
				for i := range m.LyricLines {
					m.LyricLines[i].Start = -time.Second
				}
			}
			tt := ui.NewTester(func(c *ui.Context) { nativePanelCard(c, m, a, palette(true)).Grow(1) }, 360, 340)
			v, _ := tt.Find("Lyrics · Content")
			x, y := v.X+v.W/2, v.Y+v.H/2
			tt.Press(x, y)
			tt.Move(x, y-88)
			tt.Release(x, y-88)
			if seeks != 0 {
				t.Fatal("unavailable timing permitted a seek")
			}
		})
	}
}

func TestNativeLyricsDragIgnoresClicksJitterAndHorizontalMoves(t *testing.T) {
	for _, delta := range [][2]float32{{0, 0}, {1, 3}, {70, 2}} {
		m, a := nativeTimedLyricsFixture()
		m.Position = 41
		seeks := 0
		a.Seek = func(float64) { seeks++ }
		tt := ui.NewTester(func(c *ui.Context) { nativePanelCard(c, m, a, palette(true)).Grow(1) }, 360, 340)
		v, _ := tt.Find("Lyrics · Content")
		x, y := v.X+v.W/2, v.Y+v.H/2
		tt.Press(x, y)
		tt.Move(x+delta[0], y+delta[1])
		tt.Release(x+delta[0], y+delta[1])
		if seeks != 0 || tt.HasText(a.Tr("nativeLyricsFollow")) {
			t.Fatalf("incidental motion %v sought or stopped follow", delta)
		}
		requireLyricCentered(t, tt, 8)
	}
}

func TestNativeLyricsDragMeasuresWrappedCues(t *testing.T) {
	m, a := nativeTimedLyricsFixture()
	m.Position = 41
	m.LyricLines[9].Text = "A much longer synthetic lyric that wraps across several rows"
	var seeks []float64
	a.Seek = func(at float64) { seeks = append(seeks, at) }
	tt := ui.NewTester(func(c *ui.Context) { nativePanelCard(c, m, a, palette(true)).Grow(1) }, 240, 340)
	v, _ := tt.Find("Lyrics · Content")
	r, _ := tt.Find(m.LyricLines[9].Text)
	x, y := v.X+v.W/2, v.Y+v.H/2
	dy := r.Y + r.H/2 - y
	tt.Press(x, y)
	tt.Move(x, y-dy)
	if !tt.HasText("0:45 · " + a.Tr("nativeLyricsReleaseToSeek")) {
		t.Fatal("wrapped cue geometry chose the wrong timestamp")
	}
	tt.Release(x, y-dy)
	if len(seeks) != 1 || seeks[0] != 45 {
		t.Fatalf("wrapped cue seek=%v", seeks)
	}
}

func TestNativeLyricsDragAcrossPlayerLayouts(t *testing.T) {
	for _, layout := range []struct {
		width, height       int
		immersive, expanded bool
	}{{1440, 900, false, false}, {1440, 900, false, true}, {960, 740, false, false}, {420, 640, false, true}, {420, 640, true, false}} {
		for _, language := range []string{"en", "zh-Hans", "zh-Hant", "ja"} {
			for _, appearance := range []string{"light", "dark"} {
				name := fmt.Sprintf("%dx%d_%t_%t_%s_%s", layout.width, layout.height, layout.immersive, layout.expanded, language, appearance)
				t.Run(name, func(t *testing.T) {
					m, a := nativeTimedLyricsFixture()
					m.Panel, m.Immersive, m.LyricsExpanded = !layout.immersive, layout.immersive, layout.expanded
					m.Language, m.Appearance, m.Position = language, appearance, 41
					var seeks []float64
					a.Seek = func(at float64) { seeks = append(seeks, at) }
					tt := ui.NewTester(func(c *ui.Context) { nativeView(c, m, a) }, layout.width, layout.height)
					v, _ := tt.Find(a.Tr("shellLyrics") + " · " + a.Tr("shellContent"))
					if v.H == 0 {
						// The compact immersive view opens on the cover. Scroll
						// the outer view to reach its lyrics before grabbing them.
						tt.Scroll(float32(layout.width)/2, 200, 0, 1000)
						v, _ = tt.Find(a.Tr("shellLyrics") + " · " + a.Tr("shellContent"))
					}
					x, y := v.X+v.W/2, v.Y+v.H/2
					// Single-line fixture cues are 32 DIPs high with a 12-DIP
					// gap, even when the next cue is clipped by the panel.
					dy := float32(44)
					tt.Press(x, y)
					tt.Move(x, y-dy)
					preview := "0:45 · " + a.Tr("nativeLyricsReleaseToSeek")
					if !tt.HasText(preview) {
						nativeSnapshot(t, tt, "lyrics-drag-failed-"+name)
						t.Logf("viewport=%+v move=%g", v, dy)
					}
					nativeAssertVisible(t, tt, preview, layout.width, layout.height)
					if layout.width == 420 && language == "zh-Hans" && appearance == "light" {
						nativeSnapshot(t, tt, "lyrics-drag-"+name)
					}
					tt.Release(x, y-dy)
					if len(seeks) != 1 || seeks[0] != 45 {
						t.Fatalf("player layout committed %v", seeks)
					}
					after, _ := tt.Find(a.Tr("shellLyrics") + " · " + a.Tr("shellContent"))
					if after.Y != v.Y {
						t.Fatal("drag moved the surrounding player panel")
					}
				})
			}
		}
	}
}
