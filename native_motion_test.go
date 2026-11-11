package main

import (
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

// motionTester is a tester with motion on, as the app runs: the layout
// tests' fixture turns it off.
func motionTester(m *UIModel, a ViewActions, width, height int) *ui.Tester {
	m.instantMotion = false
	return ui.NewTester(func(c *ui.Context) { nativeView(c, m, a) }, width, height)
}

// between reports whether v is strictly on its way from a to b.
func between(v, a, b float32) bool {
	lo, hi := min(a, b), max(a, b)
	return v > lo+1 && v < hi-1
}

// motionShot saves the tester's frame under name when YTMGO_MOTION_DIR is
// set, to look at a transition half way.
func motionShot(t *testing.T, tt *ui.Tester, name string) {
	dir := os.Getenv("YTMGO_MOTION_DIR")
	if dir == "" {
		return
	}
	f, err := os.Create(filepath.Join(dir, name+".png"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, tt.Image()); err != nil {
		t.Fatal(err)
	}
}

func rect(t *testing.T, tt *ui.Tester, label string) ui.Rect {
	t.Helper()
	r, ok := tt.Find(label)
	if !ok {
		t.Fatalf("%q missing", label)
	}
	return r
}

// Expanding the lyrics shrinks the cover into the thumbnail beside the
// title while the card rises into the room, instead of cutting; restoring
// runs it back. It ends where the layout without motion puts it.
func TestLyricsExpandMorphs(t *testing.T) {
	m, a := nativeTimedLyricsFixture()
	m.Panel = true
	a.SetLyricsExpanded = func(expanded bool) { m.LyricsExpanded = expanded }
	tt := motionTester(m, a, 1440, 900)
	tt.SetScale(2)
	tt.Frame()
	before := rect(t, tt, "Lyrics · Content")
	if err := tt.Click(a.Tr("nativeLyricsExpand")); err != nil {
		t.Fatal(err)
	}
	tt.Advance(tokenMotionBase / 2)
	tt.Frame()
	half := rect(t, tt, "Lyrics · Content")
	motionShot(t, tt, "lyrics-expand-half")
	tt.Advance(tokenMotionBase)
	tt.Frame()
	after := rect(t, tt, "Lyrics · Content")
	if !between(half.Y, before.Y, after.Y) {
		t.Fatalf("the card cut instead of rising: top %.1f, %.1f half way, %.1f", before.Y, half.Y, after.Y)
	}
	m.instantMotion = true
	tt.Frame()
	if still := rect(t, tt, "Lyrics · Content"); still.Y != after.Y {
		t.Fatalf("the morph ended at %.1f, not where the layout is (%.1f)", after.Y, still.Y)
	}
	m.instantMotion = false
	if err := tt.Click(a.Tr("nativeLyricsRestore")); err != nil {
		t.Fatal(err)
	}
	tt.Advance(tokenMotionBase / 2)
	tt.Frame()
	back := rect(t, tt, "Lyrics · Content")
	motionShot(t, tt, "lyrics-restore-half")
	if !between(back.Y, after.Y, before.Y) {
		t.Fatalf("restoring cut instead of running back: %.1f half way from %.1f to %.1f", back.Y, after.Y, before.Y)
	}
	_ = time.Second
}

// As the track changes, the next title rises into place over the last one
// fading, as the website's player has it, instead of swapping.
func TestTrackChangeFadesTheTitleIn(t *testing.T) {
	m, a := nativeViewFixture()
	m.Panel = false
	tt := motionTester(m, a, 1440, 900)
	tt.Frame()
	m.Current = m.Queue[1]
	m.Current.Title = "A title only the player shows"
	tt.Frame()
	tt.Advance(tokenMotionBase / 4)
	tt.Frame()
	early := rect(t, tt, m.Current.Title)
	tt.Advance(tokenMotionBase)
	tt.Frame()
	settled := rect(t, tt, m.Current.Title)
	if !(early.Y > settled.Y+1 && early.Y <= settled.Y+8) {
		t.Fatalf("the next title cut in: %.1f a quarter way, settles at %.1f", early.Y, settled.Y)
	}
}

// Docked Now Playing slides in from the window's right edge, and back out:
// as it leaves, the page keeps its width until the panel has gone, then
// fills the room once.
func TestNowPlayingSlidesInAndOut(t *testing.T) {
	m, a := nativeViewFixture()
	m.Panel = false
	tt := motionTester(m, a, 1440, 900)
	tt.SetScale(2)
	tt.Frame()
	wide := rect(t, tt, a.Tr("shellContent")).W
	m.Panel = true
	tt.Frame()
	tt.Advance(tokenMotionBase / 2)
	tt.Frame()
	half := rect(t, tt, a.Tr("shellClosePanel"))
	motionShot(t, tt, "now-playing-open-half")
	tt.Advance(tokenMotionBase)
	tt.Frame()
	open := rect(t, tt, a.Tr("shellClosePanel"))
	if !between(half.X, open.X, open.X+nativePanelWidth) {
		t.Fatalf("Now Playing cut in: its close button at %.1f half way, %.1f open", half.X, open.X)
	}
	narrow := rect(t, tt, a.Tr("shellContent")).W
	m.Panel = false
	tt.Frame()
	tt.Advance(tokenMotionBase / 2)
	tt.Frame()
	motionShot(t, tt, "now-playing-close-half")
	if w := rect(t, tt, a.Tr("shellContent")).W; w != narrow {
		t.Fatalf("the page widened under the leaving panel: %.1f, was %.1f", w, narrow)
	}
	tt.Advance(tokenMotionBase)
	tt.Frame()
	if w := rect(t, tt, a.Tr("shellContent")).W; w != wide {
		t.Fatalf("the page did not fill the room once the panel had gone: %.1f, want %.1f", w, wide)
	}
}

// Settings and Keyboard shortcuts are one sheet: going on keeps the frame,
// which takes the next page's width while the pages slide past, and going
// back runs it the other way.
func TestSheetPagesSlideInOneFrame(t *testing.T) {
	m, a, _ := nativeSettingsFixture(1280, 820)
	tt := motionTester(m, a, 1280, 820)
	tt.SetScale(2)
	tt.Frame()
	settings := rect(t, tt, a.Tr("shellClose"))
	if err := tt.Click(a.Tr("nativeSettingsViewAll")); err != nil {
		t.Fatal(err)
	}
	tt.Advance(tokenMotionBase / 2)
	tt.Frame()
	half := rect(t, tt, a.Tr("shellClose"))
	motionShot(t, tt, "sheet-forward-half")
	tt.Advance(tokenMotionBase)
	tt.Frame()
	shortcuts := rect(t, tt, a.Tr("shellClose"))
	if !between(half.X, settings.X, shortcuts.X) {
		t.Fatalf("the sheet cut to the next page: close at %.1f, %.1f half way, %.1f", settings.X, half.X, shortcuts.X)
	}
	if err := tt.Click(a.Tr("shellBackToSettings")); err != nil {
		t.Fatal(err)
	}
	tt.Advance(tokenMotionBase / 2)
	tt.Frame()
	back := rect(t, tt, a.Tr("shellClose"))
	motionShot(t, tt, "sheet-back-half")
	if !between(back.X, shortcuts.X, settings.X) {
		t.Fatalf("going back cut: close at %.1f half way from %.1f to %.1f", back.X, shortcuts.X, settings.X)
	}
}
