package main

import (
	"fmt"
	"image"
	"math"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"ytmgo/internal/catalog"
	"ytmgo/internal/player"
)

// Read the rendered glyph color, not just the selected index. A stale UI clock
// can pass the old visibility/scroll tests while highlighting the wrong words.
func requireLyricHighlight(t *testing.T, tt *ui.Tester, label string, want bool) {
	t.Helper()
	r, ok := tt.Find(label)
	if !ok {
		t.Fatal("lyric missing")
	}
	img := tt.Image()
	area := image.Rect(int(r.X), int(r.Y), int(r.X+r.W), int(r.Y+r.H)).Intersect(img.Bounds())
	bright := 0
	for y := area.Min.Y; y < area.Max.Y; y++ {
		for x := area.Min.X; x < area.Max.X; x++ {
			p := img.RGBAAt(x, y)
			if p.R > 220 && p.G > 220 && p.B > 220 {
				bright++
			}
		}
	}
	if (bright > 10) != want {
		t.Fatalf("lyric highlight=%t, want %t", bright > 10, want)
	}
}

func TestNativeLyricsHighlightUsesAudioClockBetweenModelPolls(t *testing.T) {
	m, a := nativeTimedLyricsFixture()
	m.Position = 0 // The general playback poll deliberately never runs.
	m.LyricLines = []catalog.LyricLine{
		{Text: "First synthetic cue", Start: 0, End: 100 * time.Millisecond},
		{Text: "Second synthetic cue", Start: 100 * time.Millisecond, End: 200 * time.Millisecond},
		{Text: "Third synthetic cue", Start: 230 * time.Millisecond, End: time.Second},
	}
	var tt *ui.Tester
	var start time.Time
	playing, stalled := true, false
	position := 0.0
	a.LyricsClock = func() (float64, bool) {
		if tt != nil && playing && !stalled {
			position = tt.Now().Sub(start).Seconds()
		}
		return position, playing
	}
	tt = ui.NewTester(func(c *ui.Context) { nativePanelCard(c, m, a, palette(true)).Grow(1) }, 360, 340)
	start = tt.Now()
	requireLyricHighlight(t, tt, "First synthetic cue", true)
	tt.Advance(110 * time.Millisecond)
	requireLyricHighlight(t, tt, "First synthetic cue", false)
	requireLyricHighlight(t, tt, "Second synthetic cue", true)
	// Audio starvation must not advance lyrics based on elapsed wall time.
	stalled = true
	tt.Advance(time.Second)
	requireLyricHighlight(t, tt, "Second synthetic cue", true)
	// A seek while paused reads the new decoder clock even if model.Playing
	// and model.Position still contain the previous general poll.
	playing = false
	position = .24
	tt.Frame()
	requireLyricHighlight(t, tt, "Third synthetic cue", true)
	position = .21
	tt.Frame()
	requireLyricHighlight(t, tt, "Second synthetic cue", false)
	requireLyricHighlight(t, tt, "Third synthetic cue", false)
	position = .01
	tt.Frame()
	requireLyricHighlight(t, tt, "First synthetic cue", true)
	tt.Advance(time.Second)
	requireLyricHighlight(t, tt, "First synthetic cue", true)
	if m.Position != 0 {
		t.Fatal("lyrics mutated the general playback model")
	}
}

func TestLyricsClockKeepsPendingAndRestoredBookmarks(t *testing.T) {
	for _, a := range []*desktop{{model: UIModel{Position: 19, Playing: true}}, {player: &player.Player{}, model: UIModel{Position: 41, Playing: true}, loadPending: true}, {player: &player.Player{}, model: UIModel{Position: 75}, restoredTrack: true}} {
		if at, playing := a.lyricsClock(); at != a.model.Position || playing {
			t.Fatal("unattached audio clock replaced a bookmark or started running")
		}
	}
}

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

// settleLyricGlide lets the lyrics' glide to the sung line land, as a
// reader sees it do over the slow motion; the clock stands still otherwise.
func settleLyricGlide(tt *ui.Tester) {
	tt.Advance(tokenMotionSlow + 100*time.Millisecond)
	tt.Frame()
	tt.Frame()
}

func requireLyricVisible(t *testing.T, tt *ui.Tester, line int) ui.Rect {
	t.Helper()
	settleLyricGlide(tt)
	r, ok := tt.Find(fmt.Sprintf("Synthetic lyric line %02d", line))
	v, vok := tt.Find("Lyrics · Content")
	if !ok || !vok || r.H <= 0 || r.Y < v.Y-1 || r.Y+r.H > v.Y+v.H+1 {
		t.Fatalf("line %d not visible: row=%+v viewport=%+v", line, r, v)
	}
	return r
}

func requireLyricCentered(t *testing.T, tt *ui.Tester, line int) ui.Rect {
	t.Helper()
	r := requireLyricVisible(t, tt, line)
	v, _ := tt.Find("Lyrics · Content")
	if delta := r.Y + r.H/2 - v.Y - v.H/2; math.Abs(float64(delta)) > 1 {
		t.Fatalf("current line %d is %.1f pixels away from viewport center", line, delta)
	}
	return r
}

func TestNativeLyricsKeepsCurrentLineCentered(t *testing.T) {
	for _, width := range []int{360, 240} {
		t.Run(fmt.Sprintf("width_%d", width), func(t *testing.T) {
			m, a := nativeTimedLyricsFixture()
			// Exercise natural playback, forward/backward seeks, and both ends.
			tt := ui.NewTester(func(c *ui.Context) { nativePanelCard(c, m, a, palette(true)).Grow(1) }, width, 340)
			for _, line := range []int{0, 1, 2, 17, 29, 8, 0} {
				m.Position = float64(line*5 + 1)
				tt.Frame()
				requireLyricCentered(t, tt, line)
			}
			// This line wraps to several rows; center its rendered block.
			m.LyricLines[8].Text = "A synthetic lyric that wraps across several rows in a narrow viewport"
			m.Position = 41
			tt.Frame()
			settleLyricGlide(tt)
			r, ok := tt.Find(m.LyricLines[8].Text)
			v, vok := tt.Find("Lyrics · Content")
			if !ok || !vok || r.H <= 32 || math.Abs(float64(r.Y+r.H/2-v.Y-v.H/2)) > 1 {
				t.Fatalf("wrapped cue is not centered: row=%+v viewport=%+v", r, v)
			}
			nativeSnapshot(t, tt, fmt.Sprintf("lyrics-centered-%d", width))
		})
	}
}

func TestNativeLyricsFollowSeekPauseAndManualScroll(t *testing.T) {
	m, a := nativeTimedLyricsFixture()
	tt := ui.NewTester(func(c *ui.Context) { nativePanelCard(c, m, a, palette(true)).Grow(1) }, 360, 340)
	if tt.HasText(a.Tr("nativeLyricsFollowing")) || tt.HasText(a.Tr("nativeLyricsFollow")) {
		t.Fatal("timed lyrics should follow silently until the reader scrolls")
	}
	requireLyricCentered(t, tt, 0)
	m.Position = 86
	tt.Frame()
	requireLyricCentered(t, tt, 17)
	m.Playing = false
	tt.Frame()
	paused := requireLyricCentered(t, tt, 17)
	tt.Advance(3 * time.Second)
	if after := requireLyricVisible(t, tt, 17); after != paused {
		t.Fatal("paused playback moved the lyric viewport")
	}
	// A backward seek follows the actual player position, even while paused.
	m.Position = 11
	tt.Frame()
	requireLyricCentered(t, tt, 2)
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
	requireLyricCentered(t, tt, 26)
	tt.SetSize(360, 240)
	requireLyricCentered(t, tt, 26)
	if tt.HasText(a.Tr("nativeLyricsFollow")) || tt.HasText(a.Tr("nativeLyricsFollowing")) {
		t.Fatal("the follow control should disappear after resuming")
	}
	nativeSnapshot(t, tt, "lyrics-follow-resumed")
	// Switching track resets both scroll position and the manual-follow override.
	m.Current.ID, m.Position = "another-track", 1
	tt.Frame()
	requireLyricCentered(t, tt, 0)
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

func TestNativeLyricsPanelKeyboardReadingSurvivesMediaSwitch(t *testing.T) {
	for _, focused := range []bool{false, true} {
		t.Run(fmt.Sprintf("focused_%t", focused), func(t *testing.T) {
			m, a := nativeTimedLyricsFixture()
			m.Panel, m.instantMotion = true, false
			tt := ui.NewTester(func(c *ui.Context) { nativeView(c, m, a) }, 1440, 900)
			requireLyricCentered(t, tt, 0)
			if err := tt.Click(a.Tr("shellLyrics") + " · " + a.Tr("shellNowPlaying")); err != nil {
				t.Fatal(err)
			}
			hoverLyrics := func() {
				view, _ := tt.Find("Lyrics · Content")
				tt.Move(view.X+view.W/2, view.Y+view.H/2)
			}
			if focused {
				for i := 0; i < 5 && !tt.Focused("Lyrics · Content"); i++ {
					tt.Key(0, ui.KeyTab)
				}
				if !tt.Focused("Lyrics · Content") {
					t.Fatal("could not focus lyrics")
				}
				tt.Move(0, 0)
			} else {
				hoverLyrics()
			}
			follow := a.Tr("nativeLyricsFollow")
			tt.Key(0, ui.KeyPageUp)
			settleLyricGlide(tt)
			if tt.HasText(follow) {
				t.Fatal("Page Up at the first line disabled follow")
			}
			m.Position = 51
			tt.Frame()
			requireLyricCentered(t, tt, 10)
			if focused {
				m.Position = 56
				tt.Frame()
				tt.Advance(tokenMotionSlow / 4)
			}
			tt.Key(0, ui.KeyPageDown)
			settleLyricGlide(tt)
			lineOffset := func() float32 {
				t.Helper()
				line, ok := tt.Find("Synthetic lyric line 15")
				view, _ := tt.Find("Lyrics · Content")
				if !ok || line.H <= 0 || line.Y < view.Y || line.Y+line.H > view.Y+view.H+1 {
					t.Fatal("the line reached by Page Down is outside the viewport")
				}
				return line.Y - view.Y
			}
			readAt := lineOffset()
			if !tt.HasText(follow) {
				t.Fatal("Page Down did not suspend follow")
			}
			m.Position = 101
			for _, mode := range []string{"nativeVideo", "nativeArtwork"} {
				if err := tt.Click(a.Tr(mode)); err != nil || m.Video != (mode == "nativeVideo") {
					t.Fatalf("could not switch to %s", mode)
				}
				settleLyricGlide(tt)
				if !tt.HasText(follow) || math.Abs(float64(lineOffset()-readAt)) > 1 {
					t.Fatalf("%s lost the keyboard reader's position", mode)
				}
			}
			// The reader can resume while another key scroll is still moving.
			hoverLyrics()
			tt.Key(0, ui.KeyPageDown)
			tt.Advance(tokenMotionBase / 4)
			if err := tt.Click(follow); err != nil {
				t.Fatal(err)
			}
			requireLyricCentered(t, tt, 20)
			if tt.HasText(follow) {
				t.Fatal("the previous key scroll interrupted resuming follow")
			}
		})
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

// Following playback, the lyrics glide to the next line over the slow
// motion instead of jumping; asked for less motion, they move at once. When
// they appear they are at their line already: nothing glides into place.
func TestNativeLyricsGlideToTheNextLine(t *testing.T) {
	lineCenter := func(tt *ui.Tester, line int) float32 {
		r, _ := tt.Find(fmt.Sprintf("Synthetic lyric line %02d", line))
		v, _ := tt.Find("Lyrics · Content")
		return r.Y + r.H/2 - (v.Y + v.H/2)
	}
	m, a := nativeTimedLyricsFixture()
	m.Position = 41 // line 8
	tt := ui.NewTester(func(c *ui.Context) { nativePanelCard(c, m, a, palette(true)).Grow(1) }, 360, 340)
	if d := lineCenter(tt, 8); math.Abs(float64(d)) > 1 {
		t.Fatalf("lyrics glided into place on appearing: line 8 is %.1f off centre", d)
	}
	m.Position = 46 // line 9
	tt.Frame()
	start := lineCenter(tt, 9)
	tt.Advance(tokenMotionSlow / 2)
	tt.Frame()
	half := lineCenter(tt, 9)
	if !(math.Abs(float64(half)) > 1 && math.Abs(float64(half)) < math.Abs(float64(start))-1) {
		t.Fatalf("the next line jumped instead of gliding: %.1f off centre, then %.1f half way", start, half)
	}
	settleLyricGlide(tt)
	if d := lineCenter(tt, 9); math.Abs(float64(d)) > 1 {
		t.Fatalf("the glide did not land: line 9 is %.1f off centre", d)
	}

	tt.SetPreferences(ui.Preferences{ReduceMotion: true})
	m.Position = 51 // line 10
	tt.Frame()
	tt.Frame()
	if d := lineCenter(tt, 10); math.Abs(float64(d)) > 1 {
		t.Fatalf("with less motion the line did not move at once: %.1f off centre", d)
	}
}

// Grabbing the lyrics as they glide stops the glide: they move with the
// pointer, one to one, at once.
func TestNativeLyricsDragTakesOverFromAGlide(t *testing.T) {
	m, a := nativeTimedLyricsFixture()
	m.Position, m.Playing = 41, true
	a.Seek = func(float64) {}
	tt := ui.NewTester(func(c *ui.Context) { nativePanelCard(c, m, a, palette(true)).Grow(1) }, 360, 340)
	m.Position = 46 // line 9: a glide sets off
	tt.Frame()
	tt.Advance(tokenMotionSlow / 4)
	tt.Frame()
	v, _ := tt.Find("Lyrics · Content")
	x, y := v.X+v.W/2, v.Y+v.H/2
	tt.Press(x, y)
	before, _ := tt.Find("Synthetic lyric line 09")
	tt.Move(x, y-40)
	tt.Move(x, y-80)
	tt.Advance(tokenMotionSlow / 4)
	tt.Frame()
	after, _ := tt.Find("Synthetic lyric line 09")
	if d := after.Y - before.Y; math.Abs(float64(d+80)) > 1 {
		t.Fatalf("dragging 80 up moved the lyrics %.1f: the glide fought the pointer", d)
	}
	tt.Release(x, y-80)
}
