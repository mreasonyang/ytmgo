package main

import (
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"ytmgo/internal/catalog"
)

func TestNativeLyricCueBoundaries(t *testing.T) {
	lines := []catalog.LyricLine{{Text: "one", Start: 2 * time.Second, End: 4 * time.Second}, {Text: "two", Start: 5 * time.Second, End: 8 * time.Second}}
	for _, tc := range []struct {
		seconds        float64
		active, target int
	}{{0, -1, 0}, {1.999, -1, 0}, {2, 0, 0}, {3.999, 0, 0}, {4, -1, 0}, {4.999, -1, 0}, {5, 1, 1}, {8, -1, 1}, {99, -1, 1}, {-1, -1, -1}, {math.NaN(), -1, -1}, {math.Inf(1), -1, -1}} {
		active, target := nativeLyricIndices(lines, tc.seconds)
		if active != tc.active || target != tc.target {
			t.Fatalf("time=%v: active/target=%d/%d, want %d/%d", tc.seconds, active, target, tc.active, tc.target)
		}
	}
	if active, target := nativeLyricIndices(nil, 3); active != -1 || target != -1 {
		t.Fatal("plain lyrics must not acquire synthesized timing")
	}
	overlapping := []catalog.LyricLine{{Start: time.Second, End: 8 * time.Second}, {Start: 2 * time.Second, End: 3 * time.Second}}
	if active, target := nativeLyricIndices(overlapping, 4); active != 0 || target != 0 {
		t.Fatal("an overlapping short cue hid a still-active earlier cue")
	}
}

func nativeTimedLyricsFixture() (*UIModel, ViewActions) {
	m, a := nativeViewFixture()
	m.PanelTab, m.Position = "lyrics", 0
	m.clearLyrics("Synthetic timed lyrics")
	m.LyricsAvailable = true
	for i := 0; i < 30; i++ {
		m.LyricLines = append(m.LyricLines, catalog.LyricLine{Text: fmt.Sprintf("Synthetic lyric line %02d", i), Start: time.Duration(i*5) * time.Second, End: time.Duration(i*5+4) * time.Second})
	}
	return m, a
}

func requireLyricVisible(t *testing.T, tt *ui.Tester, line int) ui.Rect {
	t.Helper()
	r, ok := tt.Find(fmt.Sprintf("Synthetic lyric line %02d", line))
	v, vok := tt.Find("Lyrics · Content")
	if !ok || !vok || r.H <= 0 || r.Y < v.Y-1 || r.Y+r.H > v.Y+v.H+1 {
		t.Fatalf("line %d not visible: row=%+v viewport=%+v", line, r, v)
	}
	return r
}

func TestNativeLyricsFollowSeekPauseAndManualScroll(t *testing.T) {
	m, a := nativeTimedLyricsFixture()
	tt := ui.NewTester(func(c *ui.Context) { nativePanelCard(c, m, a, palette(true)).Grow(1) }, 360, 340)
	if tt.HasText(a.Tr("nativeLyricsFollowing")) || tt.HasText(a.Tr("nativeLyricsFollow")) {
		t.Fatal("timed lyrics should follow silently until the reader scrolls")
	}
	requireLyricVisible(t, tt, 0)
	m.Position = 86
	tt.Frame()
	requireLyricVisible(t, tt, 17)
	m.Playing = false
	tt.Frame()
	paused := requireLyricVisible(t, tt, 17)
	tt.Advance(3 * time.Second)
	if after := requireLyricVisible(t, tt, 17); after != paused {
		t.Fatal("paused playback moved the lyric viewport")
	}
	// A backward seek follows the actual player position, even while paused.
	m.Position = 11
	tt.Frame()
	requireLyricVisible(t, tt, 2)
	v, _ := tt.Find("Lyrics · Content")
	tt.Scroll(v.X+v.W/2, v.Y+v.H/2, 0, 150)
	if !tt.HasText(a.Tr("nativeLyricsFollow")) {
		t.Fatal("manual scroll did not suspend automatic following")
	}
	if manualView, _ := tt.Find("Lyrics · Content"); manualView.Y != v.Y {
		t.Fatal("showing the follow action displaced the reader's viewport")
	}
	before, _ := tt.Find("Synthetic lyric line 02")
	m.Position = 131
	m.Playing = true
	tt.Frame()
	after, _ := tt.Find("Synthetic lyric line 02")
	if before.Y != after.Y {
		t.Fatal("playback fought manual scrolling")
	}
	if err := tt.Click(a.Tr("nativeLyricsFollow")); err != nil {
		t.Fatal(err)
	}
	if !tt.Focused("Lyrics · Content") {
		t.Fatal("resuming follow did not return focus to the lyrics")
	}
	requireLyricVisible(t, tt, 26)
	tt.SetSize(360, 240)
	requireLyricVisible(t, tt, 26)
	if tt.HasText(a.Tr("nativeLyricsFollow")) || tt.HasText(a.Tr("nativeLyricsFollowing")) {
		t.Fatal("the follow control should disappear after resuming")
	}
	nativeSnapshot(t, tt, "lyrics-follow-resumed")
	// Switching track resets both scroll position and the manual-follow override.
	m.Current.ID, m.Position = "another-track", 1
	tt.Frame()
	requireLyricVisible(t, tt, 0)
}

func TestNativeLyricsPlainFallbackAndClear(t *testing.T) {
	m, a := nativeTimedLyricsFixture()
	m.clearLyrics("First synthetic plain line\nSecond synthetic plain line")
	m.LyricsAvailable = true
	tt := ui.NewTester(func(c *ui.Context) { nativePanelCard(c, m, a, palette(true)).Grow(1) }, 360, 340)
	if !tt.HasText(m.Lyrics) || !tt.HasText(a.Tr("nativeLyricsUnsynced")) || tt.HasText(a.Tr("nativeLyricsFollow")) || tt.HasText(a.Tr("nativeLyricsFollowing")) {
		t.Fatal("plain lyrics must stay readable and explain unavailable synchronization")
	}
	m.LyricLines = []catalog.LyricLine{{Text: "stale", End: time.Second}}
	m.LyricsSource = "synthetic source"
	revision := m.LyricsRevision
	m.clearLyrics(a.Tr("shellLoading"))
	tt.Frame()
	if len(m.LyricLines) != 0 || m.LyricsSource != "" || m.LyricsAvailable || m.LyricsRevision <= revision || tt.HasText(a.Tr("nativeLyricsUnsynced")) {
		t.Fatal("track reset leaked previous lyrics or incorrectly labeled loading as untimed")
	}
}

func TestNativeLyricsKeyboardHoverStopsFollow(t *testing.T) {
	m, a := nativeTimedLyricsFixture()
	m.Position = 41
	tt := ui.NewTester(func(c *ui.Context) { nativePanelCard(c, m, a, palette(true)).Grow(1) }, 360, 340)
	// Keep focus on a sibling while the keyboard scrolls the hovered lyrics.
	if err := tt.Click(a.Tr("shellLyrics") + " · " + a.Tr("shellNowPlaying")); err != nil {
		t.Fatal(err)
	}
	v, _ := tt.Find("Lyrics · Content")
	tt.Move(v.X+v.W/2, v.Y+v.H/2)
	before, _ := tt.Find("Synthetic lyric line 08")
	tt.Key(0, ui.KeyPageDown)
	after, _ := tt.Find("Synthetic lyric line 08")
	if before.Y == after.Y || !tt.HasText(a.Tr("nativeLyricsFollow")) {
		t.Fatal("keyboard scrolling through the hovered viewport did not suspend follow")
	}
	m.Position = 101
	tt.Frame()
	stable, _ := tt.Find("Synthetic lyric line 08")
	if after.Y != stable.Y {
		t.Fatal("next cue overrode a keyboard reader gesture")
	}
	for i := 0; i < 3 && !tt.Focused("Lyrics · Content"); i++ {
		tt.Key(0, ui.KeyTab)
	}
	if !tt.Focused("Lyrics · Content") {
		t.Fatal("keyboard reader could not reach the lyrics viewport")
	}
	tt.Key(0, ui.KeyTab) // resume action immediately after the lyrics
	if !tt.Focused(a.Tr("nativeLyricsFollow")) {
		t.Fatal("keyboard reader could not reach the resume action")
	}
	tt.Key(0, ui.KeyEnter)
	requireLyricVisible(t, tt, 20)
	if tt.HasText(a.Tr("nativeLyricsFollow")) || !tt.Focused("Lyrics · Content") {
		t.Fatal("resuming by keyboard should dismiss the action and focus the lyrics")
	}
}

func TestNativeLyricsFollowIgnoresIncidentalInput(t *testing.T) {
	for _, gesture := range []string{"horizontal drift", "scroll past top", "click"} {
		t.Run(gesture, func(t *testing.T) {
			m, a := nativeTimedLyricsFixture()
			tt := ui.NewTester(func(c *ui.Context) { nativePanelCard(c, m, a, palette(true)).Grow(1) }, 360, 340)
			before := requireLyricVisible(t, tt, 2)
			x, y := before.X+10, before.Y+10
			switch gesture {
			case "horizontal drift":
				tt.Scroll(x, y, 80, 1)
			case "scroll past top":
				tt.Scroll(x, y, 0, -32)
			case "click":
				tt.ClickAt(x, y)
			}
			if after := requireLyricVisible(t, tt, 2); after.Y != before.Y || tt.HasText(a.Tr("nativeLyricsFollow")) {
				t.Fatalf("input moved lyrics by %g or disabled follow", after.Y-before.Y)
			}
			m.Position = 125
			tt.Frame()
			requireLyricVisible(t, tt, 25)
		})
	}
}

func TestNativeLyricsFollowKeepsOuterPanelStill(t *testing.T) {
	for _, layout := range []struct {
		name      string
		width     int
		immersive bool
	}{{"floating", 960, false}, {"compact immersive", 420, true}} {
		t.Run(layout.name, func(t *testing.T) {
			m, a := nativeTimedLyricsFixture()
			m.Immersive, m.Panel, m.Position = layout.immersive, !layout.immersive, 4.9
			tt := ui.NewTester(func(c *ui.Context) { nativeView(c, m, a) }, layout.width, 620)
			tab := func() ui.Rect {
				r, ok := tt.Find(a.Tr("shellLyrics") + " · " + a.Tr("shellNowPlaying"))
				if !ok {
					t.Fatal("lyrics tab missing")
				}
				return r
			}
			// Scroll the outer panel back to the cover, then let playback
			// cross a cue boundary and seek well beyond the first screen.
			x := tab().X + 4
			tt.Scroll(x, 200, 0, -1200)
			before := tab()
			for _, position := range []float64{5.1, 125} {
				m.Position = position
				tt.Frame()
				if after := tab(); after.Y != before.Y {
					t.Fatalf("at %.1fs the outer panel moved by %g", position, after.Y-before.Y)
				}
			}
			tt.Scroll(x, 200, 0, 1200)
			if tt.HasText(a.Tr("nativeLyricsFollow")) {
				t.Fatal("scrolling the outer panel disabled follow")
			}
			requireLyricVisible(t, tt, 25)
		})
	}
}
