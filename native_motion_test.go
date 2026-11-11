package main

import (
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"ytmgo/internal/catalog"
)

// motionTester is a tester with motion on, as the app runs: the layout
// tests' fixture turns it off.
func motionTester(m *UIModel, a ViewActions, width, height int) *ui.Tester {
	m.instantMotion = false
	tt := ui.NewTester(func(c *ui.Context) { nativeView(c, m, a) }, width, height)
	if os.Getenv("YTMGO_MOTION_DIR") != "" {
		tt.SetScale(2) // sharp frames to look at; slower
	}
	return tt
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
	tt.Advance(tokenMotionBase)
	tt.Frame()
	tt.Frame()
	if r, ok := tt.Find(a.Tr("nativeSettingsViewAll")); !ok || r.H <= 0 || r.Y+r.H > 820-40 {
		t.Fatalf("back in Settings, the Keyboard card is not in view: %v %v", r, ok)
	}
}

// Into immersive mode the cover and the lyrics card travel from docked Now
// Playing to their places (the card to the right column), while the
// library and the global bar fade; out of it they travel back.
func TestImmersiveMorphsFromNowPlaying(t *testing.T) {
	m, a := nativeTimedLyricsFixture()
	m.Panel = true
	tt := motionTester(m, a, 1440, 900)
	tt.Frame()
	tt.Frame()
	docked := rect(t, tt, "Lyrics · Content")
	tt.Key(ui.Cmd|ui.Shift, ui.KeyF)
	if !m.Immersive {
		t.Fatal("⌘⇧F did not enter immersive mode")
	}
	tt.Advance(tokenMotionMorph / 2)
	tt.Frame()
	half := rect(t, tt, "Lyrics · Content")
	motionShot(t, tt, "immersive-in-half")
	tt.Advance(tokenMotionMorph)
	tt.Frame()
	immersive := rect(t, tt, "Lyrics · Content")
	if !between(half.Y, docked.Y, immersive.Y) && !between(half.X, docked.X, immersive.X) {
		t.Fatalf("the lyrics card cut into immersive mode: %+v, %+v half way, %+v", docked, half, immersive)
	}
	tt.Key(ui.Cmd|ui.Shift, ui.KeyF)
	tt.Advance(tokenMotionMorph / 2)
	tt.Frame()
	back := rect(t, tt, "Lyrics · Content")
	motionShot(t, tt, "immersive-out-half")
	if !between(back.Y, immersive.Y, docked.Y) && !between(back.X, immersive.X, docked.X) {
		t.Fatalf("the lyrics card cut out of immersive mode: %+v half way from %+v to %+v", back, immersive, docked)
	}
}

// Opening a collection, its cover travels from the card into the page's
// header, which shows at once from what the card says; the tracks follow as
// they load. Half way it is on its way; it ends where the layout is.
func TestOpeningACollectionCarriesItsCover(t *testing.T) {
	m, a := nativeViewFixture()
	m.Panel = false
	for i := range m.Page.Shelves[1].Items {
		m.Page.Shelves[1].Items[i].BrowseID = fmt.Sprintf("album-%d", i)
	}
	a.Open = func(item catalog.Item) {
		// As desktop.openItem: the route, and a header seeded from the item.
		m.Route, m.Loading = "browse:"+item.BrowseID, true
		m.Page = catalog.Page{Title: item.Title, Creator: item.Artist, Artwork: item.Artwork}
	}
	tt := motionTester(m, a, 1440, 900)
	tt.Frame()
	if err := tt.Click("Discovery"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(tokenMotionMorph / 3)
	tt.Frame()
	early := tt.Image()
	motionShot(t, tt, "collection-open-third")
	tt.Advance(tokenMotionMorph)
	tt.Frame()
	settled := tt.Image()
	m.instantMotion = true
	tt.Frame()
	if n := changedPixels(settled, tt.Image()); n > 0 {
		t.Fatalf("the cover's journey ended %d pixels off the layout", n)
	}
	if n := changedPixels(early, settled); n == 0 {
		t.Fatal("the page cut to its header: nothing was on its way a third through")
	}
}

// On a narrow window Now Playing floats: it rises from the bottom, where the
// mini player that opens it is, and sinks back before it goes.
func TestFloatingNowPlayingRisesAndSinks(t *testing.T) {
	m, a := nativeViewFixture()
	m.Panel = false
	tt := motionTester(m, a, 420, 860)
	tt.Frame()
	if err := tt.Click(a.Tr("shellOpenNowPlaying")); err != nil {
		t.Fatal(err)
	}
	tt.Advance(tokenMotionBase / 3)
	tt.Frame()
	rising := rect(t, tt, a.Tr("shellClosePanel"))
	motionShot(t, tt, "floating-open-third")
	tt.Advance(tokenMotionBase)
	tt.Frame()
	open := rect(t, tt, a.Tr("shellClosePanel"))
	if !between(rising.Y, open.Y, open.Y+nativeFloatRise) {
		t.Fatalf("the sheet cut in: close button at %.1f a third through, %.1f open", rising.Y, open.Y)
	}
	m.Panel = false
	tt.Frame()
	tt.Advance(tokenMotionBase / 3)
	tt.Frame()
	sinking, ok := tt.Find(a.Tr("shellClosePanel"))
	if !ok || !between(sinking.Y, open.Y, open.Y+nativeFloatRise) {
		t.Fatalf("the sheet cut out instead of sinking: %v %v", sinking, ok)
	}
	tt.Advance(tokenMotionBase)
	tt.Frame()
	tt.Frame()
	if _, ok := tt.Find(a.Tr("shellClosePanel")); ok {
		t.Fatal("the sheet stayed after sinking")
	}
}
