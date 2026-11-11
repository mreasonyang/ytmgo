package main

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func TestNativeLyricsExpansionGivesReaderMoreSpace(t *testing.T) {
	for _, size := range []struct {
		width, height int
		minimumGain   float32
	}{
		// 1440×900: the frame gap above the player bar made the panel 8
		// shorter, so the cover the expansion gives back is 4 smaller (196,
		// was 200) and the gain 178 (was 182).
		{1440, 900, 176}, {1440, 640, 30}, {960, 740, 40}, {420, 640, 30}} {
		t.Run(fmt.Sprintf("%dx%d", size.width, size.height), func(t *testing.T) {
			m, a := nativeTimedLyricsFixture()
			m.Panel = true
			a.SetLyricsExpanded = func(expanded bool) { m.LyricsExpanded = expanded }
			tt := ui.NewTester(func(c *ui.Context) { nativeView(c, m, a) }, size.width, size.height)
			viewport := func() ui.Rect {
				r, ok := tt.Find("Lyrics · Content")
				if !ok || r.H <= 0 {
					t.Fatal("lyrics viewport missing")
				}
				return r
			}
			before := viewport()
			nativeSnapshot(t, tt, fmt.Sprintf("lyrics-standard-%dx%d", size.width, size.height))
			if err := tt.Click(a.Tr("nativeLyricsExpand")); err != nil {
				t.Fatal(err)
			}
			if !m.LyricsExpanded {
				t.Fatal("expand action did not select the larger lyrics layout")
			}
			after := viewport()
			if gain := after.H - before.H; gain < size.minimumGain {
				t.Fatalf("expansion gained only %.1f DIPs: before=%+v after=%+v", gain, before, after)
			}
			if _, ok := tt.Find(a.Tr("nativeLyricsRestore")); !ok {
				t.Fatal("expanded layout has no restore action")
			}
			if !tt.HasText(m.Current.Title) || !tt.HasText(m.Current.Artist) {
				t.Fatal("expanding lyrics removed the current song identity")
			}
			nativeSnapshot(t, tt, fmt.Sprintf("lyrics-expanded-%dx%d", size.width, size.height))
			if err := tt.Click(a.Tr("nativeLyricsRestore")); err != nil {
				t.Fatal(err)
			}
			if m.LyricsExpanded || math.Abs(float64(viewport().H-before.H)) > 1 {
				t.Fatalf("restore did not return the original lyrics height: before=%+v after=%+v", before, viewport())
			}
		})
	}
}

func TestNativeLyricsExpansionPreservesVideoLayout(t *testing.T) {
	for _, size := range [][2]int{{1440, 900}, {960, 740}} {
		t.Run(fmt.Sprintf("%dx%d", size[0], size[1]), func(t *testing.T) {
			m, a := nativeTimedLyricsFixture()
			m.Panel, m.Video = true, true
			m.VideoError = "Synthetic video playback error"
			a.SetLyricsExpanded = func(expanded bool) { m.LyricsExpanded = expanded }
			tt := ui.NewTester(func(c *ui.Context) { nativeView(c, m, a) }, size[0], size[1])
			videoBefore, ok := tt.Find(a.Tr("nativeVideo"))
			if !ok {
				t.Fatal("video viewport missing")
			}
			retryBefore, ok := tt.Find(a.Tr("buttonRetry"))
			if !ok {
				t.Fatal("video retry missing")
			}
			if err := tt.Click(a.Tr("nativeArtwork")); err != nil {
				t.Fatal(err)
			}
			if err := tt.Click(a.Tr("nativeLyricsExpand")); err != nil {
				t.Fatal(err)
			}
			expanded, _ := tt.Find("Lyrics · Content")
			if err := tt.Click(a.Tr("nativeVideo")); err != nil || !m.Video {
				t.Fatal("video switch stopped working with expanded lyrics")
			}
			videoAfter, _ := tt.Find(a.Tr("nativeVideo"))
			retryAfter, _ := tt.Find(a.Tr("buttonRetry"))
			if videoAfter.W != videoBefore.W || videoAfter.H != videoBefore.H || retryAfter.W != retryBefore.W || retryAfter.H != retryBefore.H || retryAfter.Y-videoAfter.Y != retryBefore.Y-videoBefore.Y {
				t.Fatal("saved lyrics layout reduced the video or its retry affordance")
			}
			if _, ok := tt.Find(a.Tr("nativeLyricsRestore")); ok {
				t.Fatal("cover resize action is shown in video mode")
			}
			if err := tt.Click(a.Tr("nativeArtwork")); err != nil || m.Video {
				t.Fatal("cover switch stopped working with expanded lyrics")
			}
			returned, _ := tt.Find("Lyrics · Content")
			if !m.LyricsExpanded || math.Abs(float64(returned.H-expanded.H)) > 1 {
				t.Fatal("switching media lost the saved cover and lyrics layout")
			}
			nativeSnapshot(t, tt, fmt.Sprintf("lyrics-expanded-cover-after-video-%dx%d", size[0], size[1]))
		})
	}
}

func TestSwitchingNowPlayingMediaKeepsLyricsFollowing(t *testing.T) {
	for _, startVideo := range []bool{false, true} {
		t.Run(fmt.Sprintf("from-video-%t", startVideo), func(t *testing.T) {
			m, a := nativeTimedLyricsFixture()
			m.Panel, m.Video, m.instantMotion = true, startVideo, false
			position := 41.0
			a.LyricsClock = func() (float64, bool) { return position, true }
			tt := ui.NewTester(func(c *ui.Context) { nativeView(c, m, a) }, 1440, 900)
			requireLyricCentered(t, tt, 8)

			// Switch media while the current lyric is gliding into place.
			position = 46
			tt.Frame()
			tt.Advance(tokenMotionSlow / 4)
			switchTo, switchBack := a.Tr("nativeVideo"), a.Tr("nativeArtwork")
			if startVideo {
				switchTo, switchBack = switchBack, switchTo
			}
			if err := tt.Click(switchTo); err != nil || m.Video == startVideo {
				t.Fatalf("media switch did not change presentation: %v", err)
			}
			for range 16 {
				tt.Advance(50 * time.Millisecond)
				if tt.HasText(a.Tr("nativeLyricsFollow")) {
					t.Fatal("switching media suspended lyrics without a reader gesture")
				}
			}
			requireLyricCentered(t, tt, 9)
			position = 101
			tt.Frame()
			requireLyricCentered(t, tt, 20)

			// Switching back should also respect someone reading manually.
			view := rect(t, tt, "Lyrics · Content")
			tt.Scroll(view.X+view.W/2, view.Y+view.H/2, 0, -120)
			if !tt.HasText(a.Tr("nativeLyricsFollow")) {
				t.Fatal("scrolling the lyrics did not pause following")
			}
			if err := tt.Click(switchBack); err != nil || m.Video != startVideo {
				t.Fatalf("could not switch back to original media: %v", err)
			}
			settleLyricGlide(tt)
			if !tt.HasText(a.Tr("nativeLyricsFollow")) {
				t.Fatal("switching media resumed following during manual reading")
			}
		})
	}
}

func TestNativeLyricsExpansionPreservesManualReadingAndFollow(t *testing.T) {
	m, a := nativeTimedLyricsFixture()
	m.Panel, m.Position = true, 41
	a.SetLyricsExpanded = func(expanded bool) { m.LyricsExpanded = expanded }
	tt := ui.NewTester(func(c *ui.Context) { nativeView(c, m, a) }, 1440, 900)
	requireLyricVisible(t, tt, 8)
	v, _ := tt.Find("Lyrics · Content")
	tt.Scroll(v.X+v.W/2, v.Y+v.H/2, 0, 150)
	if !tt.HasText(a.Tr("nativeLyricsFollow")) {
		t.Fatal("manual reading did not suspend following")
	}
	lineOffset := func() float32 {
		line, found := tt.Find("Synthetic lyric line 08")
		view, visible := tt.Find("Lyrics · Content")
		if !found || !visible {
			t.Fatal("lyrics or their viewport disappeared")
		}
		return line.Y - view.Y
	}
	before := lineOffset()
	if err := tt.Click(a.Tr("nativeLyricsExpand")); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText(a.Tr("nativeLyricsFollow")) || math.Abs(float64(lineOffset()-before)) > 1 {
		t.Fatal("expanding lyrics reset the reader's position or resumed following")
	}
	m.Position = 126
	tt.Frame()
	if math.Abs(float64(lineOffset()-before)) > 1 {
		t.Fatal("a new cue overrode manual reading after expansion")
	}
	if err := tt.Click(a.Tr("nativeLyricsFollow")); err != nil {
		t.Fatal(err)
	}
	requireLyricVisible(t, tt, 25)
	if err := tt.Click(a.Tr("nativeLyricsRestore")); err != nil {
		t.Fatal(err)
	}
	requireLyricVisible(t, tt, 25)
	if tt.HasText(a.Tr("nativeLyricsFollow")) {
		t.Fatal("restoring the cover suspended automatic following")
	}
}

func TestNativeLyricsExpansionSurvivesPanelAndTabChanges(t *testing.T) {
	m, a := nativeTimedLyricsFixture()
	m.Panel = true
	a.SetLyricsExpanded = func(expanded bool) { m.LyricsExpanded = expanded }
	tt := ui.NewTester(func(c *ui.Context) { nativeView(c, m, a) }, 1440, 900)
	if err := tt.Click(a.Tr("nativeLyricsExpand")); err != nil {
		t.Fatal(err)
	}
	expanded, _ := tt.Find("Lyrics · Content")
	if err := tt.Click(a.Tr("shellQueue") + " · " + a.Tr("shellNowPlaying")); err != nil {
		t.Fatal(err)
	}
	if _, ok := tt.Find(a.Tr("nativeLyricsRestore")); ok {
		t.Fatal("lyrics resize action is shown for the queue")
	}
	if err := tt.Click(a.Tr("shellLyrics") + " · " + a.Tr("shellNowPlaying")); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click(a.Tr("shellClosePanel")); err != nil {
		t.Fatal(err)
	}
	m.Current.ID = "next-song"
	m.Panel = true
	tt.Frame()
	reopened, ok := tt.Find("Lyrics · Content")
	if !m.LyricsExpanded || !ok || math.Abs(float64(expanded.H-reopened.H)) > 1 {
		t.Fatal("tab, panel or track changes lost the chosen lyrics layout")
	}
}

func TestLyricsLayoutPreferenceSurvivesRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "preferences", "lyrics-layout")
	if loadLyricsExpanded(path) {
		t.Fatal("new installations should use the standard layout")
	}
	a := &desktop{lyricsLayoutPath: path}
	a.setLyricsExpanded(true)
	if !a.model.LyricsExpanded || !loadLyricsExpanded(path) {
		t.Fatal("expanded layout was not saved for a fresh launch")
	}
	reopened := &desktop{lyricsLayoutPath: path, model: UIModel{LyricsExpanded: loadLyricsExpanded(path)}}
	reopened.setLyricsExpanded(false)
	if reopened.model.LyricsExpanded || loadLyricsExpanded(path) {
		t.Fatal("restoring the standard layout was not saved")
	}
	if err := os.WriteFile(path, []byte("unknown-layout\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if loadLyricsExpanded(path) {
		t.Fatal("an invalid preference did not fall back to the standard layout")
	}
}

func TestLyricsLayoutSaveFailureDoesNotClaimPersistence(t *testing.T) {
	// A directory cannot be replaced by the atomic preference file.
	a := &desktop{lyricsLayoutPath: t.TempDir()}
	a.setLyricsExpanded(true)
	if a.model.LyricsExpanded || a.model.Error == "" {
		t.Fatal("a failed save changed the layout or failed to explain the error")
	}
}
