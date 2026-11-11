package main

import (
	"math"
	"testing"

	"github.com/egoist/mygo/ui"
)

func near(a, b float32) bool { return math.Abs(float64(a-b)) <= 0.5 }

// A song (no video) offers no Cover/Video choice and shows its cover, even
// when video was chosen for an earlier track.
func TestSongWithoutVideoHasNoMediaSwitch(t *testing.T) {
	for _, chosen := range []bool{false, true} {
		m, a := nativeViewFixture()
		m.Panel, m.Video, m.VideoAvailable = true, chosen, false
		tt := ui.NewTester(func(c *ui.Context) { nativeView(c, m, a) }, 1440, 900)
		tt.Frame()
		if tt.HasText(a.Tr("nativeArtwork")) || tt.HasText(a.Tr("nativeVideo")) {
			t.Fatalf("video chosen %v: a song offers the Cover/Video switch", chosen)
		}
		if _, ok := tt.Find(a.Tr("nativeVideo")); ok {
			t.Fatalf("video chosen %v: a song shows a video frame", chosen)
		}
	}
}

// In Now Playing a video takes the panel's full width, whatever the
// window's height: the cover's height budget made it a thumbnail.
func TestPanelVideoHasAFixedWatchableSize(t *testing.T) {
	for _, height := range []int{900, 700} {
		m, a := nativeViewFixture()
		m.Panel, m.Video = true, true
		tt := ui.NewTester(func(c *ui.Context) { nativeView(c, m, a) }, 1440, height)
		tt.Frame()
		video := rect(t, tt, a.Tr("nativeVideo"))
		want := float32(nativePanelWidth - 32)
		if !near(video.W, want) || !near(video.H, want*9/16) {
			t.Fatalf("%d high: the video is %.0f × %.0f, want %.0f × %.0f", height, video.W, video.H, want, want*9/16)
		}
	}
}

func hoverVideo(t *testing.T, tt *ui.Tester, a ViewActions) ui.Rect {
	t.Helper()
	video := rect(t, tt, a.Tr("nativeVideo"))
	tt.Move(video.X+video.W/2, video.Y+video.H/2)
	tt.Frame()
	return video
}

// The video's own control fills the play view with it, from Now Playing,
// and gives the view back to Now Playing; Esc steps back the same way.
func TestVideoFillsThePlayViewAndGivesItBack(t *testing.T) {
	for _, how := range []string{"button", "escape"} {
		m, a := nativeViewFixture()
		m.Panel, m.Video = true, true
		tt := ui.NewTester(func(c *ui.Context) { nativeView(c, m, a) }, 1440, 900)
		tt.Frame()
		panel := hoverVideo(t, tt, a)
		if err := tt.Click(a.Tr("nativeVideoTheater")); err != nil {
			t.Fatal(err)
		}
		if !m.Immersive || !m.VideoTheater {
			t.Fatal("the video did not fill the play view")
		}
		full := rect(t, tt, a.Tr("nativeVideo"))
		// The title stays only in the player bar.
		if full.W < 2*panel.W || countText(tt, m.Current.Title) != 1 {
			t.Fatalf("the video fills %v of the play view, the title shown %d times", full, countText(tt, m.Current.Title))
		}
		if how == "button" {
			hoverVideo(t, tt, a)
			if err := tt.Click(a.Tr("nativeVideoTheaterExit")); err != nil {
				t.Fatal(err)
			}
		} else {
			tt.Key(0, ui.KeyEscape)
		}
		if m.Immersive || m.VideoTheater || !m.Panel {
			t.Fatalf("%s: back to immersive %v, theater %v, panel %v; want Now Playing", how, m.Immersive, m.VideoTheater, m.Panel)
		}
	}
}

// Full screen takes the window for the video; its own button or Esc gives
// the window back, to the view it came from.
func TestVideoFullScreen(t *testing.T) {
	for _, how := range []string{"button", "escape"} {
		m, a := nativeViewFixture()
		m.Panel, m.Video = true, true
		var asked []bool
		a.FullScreen = func(on bool) { asked = append(asked, on) }
		tt := ui.NewTester(func(c *ui.Context) { nativeView(c, m, a) }, 1440, 900)
		tt.Frame()
		hoverVideo(t, tt, a)
		if err := tt.Click(a.Tr("nativeVideoFullScreen")); err != nil {
			t.Fatal(err)
		}
		if !m.VideoFullScreen || len(asked) != 1 || !asked[0] {
			t.Fatalf("full screen %v, window asked %v", m.VideoFullScreen, asked)
		}
		video := rect(t, tt, a.Tr("nativeVideo"))
		if !near(video.W, 1440) || tt.HasText(a.Tr("shellNowPlaying")) {
			t.Fatalf("full screen shows %v and Now Playing %v, want the video alone", video, tt.HasText(a.Tr("shellNowPlaying")))
		}
		if how == "button" {
			tt.Move(720, 450)
			tt.Frame()
			if err := tt.Click(a.Tr("nativeVideoExitFullScreen")); err != nil {
				t.Fatal(err)
			}
		} else {
			tt.Key(0, ui.KeyEscape)
		}
		if m.VideoFullScreen || len(asked) != 2 || asked[1] || !m.Panel {
			t.Fatalf("%s: full screen %v, window asked %v, panel %v", how, m.VideoFullScreen, asked, m.Panel)
		}
	}
}

// The full screen controls stay while the pointer moves and fade out (over
// the fast motion, not at once) once it rests during playback.
func TestFullScreenControlsFadeWhenThePointerRests(t *testing.T) {
	m, a := nativeViewFixture()
	m.Video, m.VideoFullScreen = true, true
	tt := motionTester(m, a, 1280, 800)
	tt.Move(640, 400)
	tt.Frame()
	tt.Advance(tokenMotionFast)
	tt.Frame()
	exit := a.Tr("nativeVideoExitFullScreen")
	if _, ok := tt.Find(exit); !ok {
		t.Fatal("the controls did not come with the pointer")
	}
	// The rest counts from the move, a fast motion ago.
	tt.Advance(nativeFullScreenIdle - tokenMotionFast + tokenMotionFast/2)
	tt.Frame()
	if _, ok := tt.Find(exit); !ok {
		t.Fatal("the controls vanished at once instead of fading")
	}
	tt.Advance(tokenMotionFast)
	tt.Frame()
	if _, ok := tt.Find(exit); ok {
		t.Fatal("the controls stayed over the playing video at rest")
	}
}
