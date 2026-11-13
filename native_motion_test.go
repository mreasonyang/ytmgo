package main

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
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

// Docked Now Playing slides in from the window's right edge, and back out,
// and the page gives it room and takes it back in step: the same motion,
// at once, not after (a page catching up after the panel looked broken).
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
	halfOpen := rect(t, tt, a.Tr("shellContent")).W
	motionShot(t, tt, "now-playing-open-half")
	tt.Advance(tokenMotionBase)
	tt.Frame()
	open := rect(t, tt, a.Tr("shellClosePanel"))
	narrow := rect(t, tt, a.Tr("shellContent")).W
	if !between(half.X, open.X, open.X+nativePanelWidth) {
		t.Fatalf("Now Playing cut in: its close button at %.1f half way, %.1f open", half.X, open.X)
	}
	if !(halfOpen < wide-1 && halfOpen > narrow+1) {
		t.Fatalf("half way in, the page is %.1f wide; want between %.1f and %.1f, in step", halfOpen, narrow, wide)
	}
	m.Panel = false
	tt.Frame()
	tt.Advance(tokenMotionBase / 2)
	tt.Frame()
	motionShot(t, tt, "now-playing-close-half")
	if w := rect(t, tt, a.Tr("shellContent")).W; !(w > narrow+1 && w < wide-1) {
		t.Fatalf("half way out, the page is %.1f wide; want between %.1f and %.1f, in step", w, narrow, wide)
	}
	tt.Advance(tokenMotionBase)
	tt.Frame()
	if w := rect(t, tt, a.Tr("shellContent")).W; w != wide {
		t.Fatalf("as the panel finished going, the page is %.1f wide, want %.1f", w, wide)
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

func TestNestedCollectionHistoryRestoresCoverSources(t *testing.T) {
	app := &desktop{model: UIModel{Route: "home"}, historyIndex: -1}
	app.pushHistory(true)
	album := nativeCollectionMatch{route: "home", button: 1, id: "album"}
	playlist := nativeCollectionMatch{route: "browse:album", button: 2, id: "playlist"}
	for _, cover := range []nativeCollectionMatch{album, playlist} {
		app.model.openedCover = cover
		app.model.Route = "browse:" + cover.id
		app.pushHistory(true)
	}
	for _, step := range []struct {
		delta int
		want  nativeCollectionMatch
	}{
		{-1, album},
		{-1, nativeCollectionMatch{}},
		{1, album},
		{1, playlist},
	} {
		app.travel(step.delta)
		if got := app.model.openedCover; got != step.want {
			t.Fatalf("cover source on %s: got %+v, want %+v", app.model.Route, got, step.want)
		}
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

// Cover to video, the media frame changes shape in place, from the square
// to 16:9, rather than one frame replacing the other.
func TestCoverToVideoChangesShape(t *testing.T) {
	m, a := nativeViewFixture()
	m.Panel = true
	a.Video = func(on bool) { m.Video = on }
	tt := motionTester(m, a, 1440, 900)
	tt.Frame()
	m.Video = true
	tt.Frame()
	tt.Advance(tokenMotionBase / 3)
	tt.Frame()
	half := rect(t, tt, a.Tr("nativeVideo"))
	tt.Advance(tokenMotionBase)
	tt.Frame()
	video := rect(t, tt, a.Tr("nativeVideo"))
	if !between(half.H, video.H, video.W) {
		t.Fatalf("the frame cut to 16:9: %.1f high a third through, %.1f as video, %.1f as the square", half.H, video.H, video.W)
	}
}

// The library narrows to the rail, and widens back, instead of snapping.
func TestSidebarCollapseNarrows(t *testing.T) {
	m, a := nativeViewFixture()
	m.Panel = false
	tt := motionTester(m, a, 1440, 900)
	tt.Frame()
	// The page's left edge follows the library's width.
	full := rect(t, tt, a.Tr("shellContent")).X
	m.SidebarCollapsed = true
	tt.Frame()
	tt.Advance(tokenMotionMorph / 3)
	tt.Frame()
	half := rect(t, tt, a.Tr("shellContent")).X
	tt.Advance(tokenMotionMorph)
	tt.Frame()
	rail := rect(t, tt, a.Tr("shellContent")).X
	if !between(half, rail, full) {
		t.Fatalf("the library snapped: the page at %.1f, %.1f a third through, %.1f", full, half, rail)
	}
}

// A selected collection also has a large cover in the page header. Its
// sidebar thumbnail should stay aligned with its neighbors as the rail moves.
func TestSidebarCollapseKeepsSelectedCoverInRow(t *testing.T) {
	items := []catalog.Item{
		{BrowseID: "VLLM", Title: "Liked Music", Artwork: "liked", Kind: "playlist"},
		{BrowseID: "VLmix", Title: "New Release Mix", Artwork: "mix", Kind: "playlist"},
		{BrowseID: "album-reference", Title: "Reference album", Artwork: "album", Kind: "album"},
	}
	ink := color.RGBA{R: 244, B: 195, A: 255}
	solid := ui.NewBitmap(tintFill(48, 48, func(_, _ int) color.RGBA { return ink }))
	check := func(t *testing.T, tt *ui.Tester) {
		t.Helper()
		frame := tt.Image()
		scale := frame.Bounds().Dx() / 1440
		var firstStart int
		for i, item := range items {
			row := rect(t, tt, item.Title)
			y := int(row.Y+row.H/2) * scale
			start, end := -1, -1
			for x := int(row.X) * scale; x < int(row.X+row.W)*scale; x++ {
				if frame.RGBAAt(x, y) == ink {
					if start < 0 {
						start = x
					}
					end = x
				}
			}
			width := end - start + 1
			if start < 0 || width != 48*scale {
				t.Fatalf("%s cover at x=%d width=%d, want a visible 48 DIP thumbnail", item.Title, start, width)
			}
			if i == 0 {
				firstStart = start
			} else if start != firstStart {
				t.Fatalf("%s cover at x=%d, first cover at x=%d", item.Title, start, firstStart)
			}
		}
	}
	for _, selected := range items[:2] {
		t.Run(selected.Title, func(t *testing.T) {
			m, a := nativeViewFixture()
			m.SignedIn, m.LibraryLoaded, m.LibraryItems = true, true, items
			a.Artwork = func(_ string, _ int, _ artworkPriority) ui.ImageSource { return solid }
			a.Open = func(item catalog.Item) {
				m.Route = "browse:" + item.BrowseID
				m.Page = catalog.Page{Title: item.Title, Artwork: item.Artwork}
			}
			tt := motionTester(m, a, 1440, 900)
			if err := tt.Click(nativeItemLabel(selected)); err != nil {
				t.Fatal(err)
			}
			if m.Route != "browse:"+selected.BrowseID || m.openedCover.route != "home" || m.openedCover.button == 0 || m.openedCover.id != selected.BrowseID {
				t.Fatalf("opening sidebar cover reached route %q with source %+v", m.Route, m.openedCover)
			}
			tt.Advance(tokenMotionMorph)
			for _, key := range []string{"shellSidebarCollapse", "shellSidebarExpand"} {
				if err := tt.Click(a.Tr(key)); err != nil {
					t.Fatal(err)
				}
				check(t, tt)
				for range 4 {
					tt.Advance(tokenMotionMorph / 4)
					check(t, tt)
				}
			}
		})
	}
}

// Paint a single collection in an unmistakable color. Reading its bounds from
// the frame captures the presented position, including the matched animation.
func collectionMarker(a *ViewActions, width int) func(*ui.Tester, ui.Rect) image.Rectangle {
	ink := color.RGBA{R: 245, B: 147, A: 255}
	solid := ui.NewBitmap(tintFill(64, 64, func(_, _ int) color.RGBA { return ink }))
	a.artworkPeek = func(src string, _ int) ui.ImageSource {
		if src == "marked-cover" {
			return solid
		}
		return nil
	}
	a.Artwork = func(src string, edge int, _ artworkPriority) ui.ImageSource { return a.artworkPeek(src, edge) }
	return func(tt *ui.Tester, area ui.Rect) image.Rectangle {
		img := tt.Image()
		scale := img.Bounds().Dx() / width
		var bounds image.Rectangle
		for y := int(area.Y); y < int(area.Y+area.H); y++ {
			for x := int(area.X); x < int(area.X+area.W); x++ {
				if img.RGBAAt(x*scale, y*scale) == ink {
					bounds = bounds.Union(image.Rect(x, y, x+1, y+1))
				}
			}
		}
		return bounds
	}
}

func TestHomeCollectionUsesTheClickedCopy(t *testing.T) {
	for _, width := range []int{420, 1440} {
		for _, openTile := range []bool{true, false} {
			t.Run(fmt.Sprintf("%d/tile=%v", width, openTile), func(t *testing.T) {
				m, a := nativeViewFixture()
				item := catalog.Item{Title: "Saved mix", Kind: "playlist", BrowseID: "VLmix", Artwork: "marked-cover"}
				m.SignedIn, m.LibraryLoaded, m.LibraryItems = true, true, []catalog.Item{item}
				m.Page = catalog.Page{Title: "Welcome", Shelves: []catalog.Shelf{{Title: "Recommendations", Items: []catalog.Item{item}}}}
				a.Open = func(it catalog.Item) {
					m.Route = "browse:" + it.BrowseID
					m.Page = catalog.Page{Title: it.Title, Artwork: it.Artwork}
				}
				marker := collectionMarker(&a, width)
				tt := motionTester(m, a, width, 940)
				tt.Advance(tokenMotionMorph)
				page := rect(t, tt, a.Tr("shellContent"))
				heading := rect(t, tt, "Recommendations")
				area := page
				if openTile {
					area.H = heading.Y - page.Y
				} else {
					area.Y = heading.Y
					area.H = page.Y + page.H - area.Y
				}
				clicked := marker(tt, area)
				if clicked.Empty() {
					t.Fatal("expected a cover to click")
				}
				tt.ClickAt(float32(clicked.Min.X+clicked.Dx()/2), float32(clicked.Min.Y+clicked.Dy()/2))
				if m.Route != "browse:"+item.BrowseID {
					t.Fatal("the collection did not open")
				}
				if start := marker(tt, page); start != clicked {
					t.Fatalf("header started at %v, clicked %v", start, clicked)
				}
				tt.Advance(tokenMotionMorph)
				if end := marker(tt, page); end == clicked || end.Empty() {
					t.Fatalf("header did not arrive at its own bounds: %v", end)
				}
			})
		}
	}
}

func TestSidebarCollectionOnlyMatchesAcrossNavigation(t *testing.T) {
	m, a := nativeViewFixture()
	item := catalog.Item{Title: "Liked Music", Kind: "playlist", BrowseID: "VLLM", Artwork: "marked-cover"}
	other := catalog.Item{Title: "Album", Kind: "album", BrowseID: "MPREalbum"}
	m.SignedIn, m.LibraryLoaded, m.LibraryItems = true, true, []catalog.Item{item, other}
	home := m.Page
	a.Open = func(it catalog.Item) {
		m.Route = "browse:" + it.BrowseID
		m.Page = catalog.Page{Title: it.Title, Artwork: it.Artwork}
	}
	a.Back = func() { m.Route, m.Page, m.openedCover = "home", home, nativeCollectionMatch{} }
	marker := collectionMarker(&a, 1440)
	tt := motionTester(m, a, 1440, 940)
	tt.Advance(tokenMotionMorph)
	click := func(label string) {
		t.Helper()
		if err := tt.Click(label); err != nil {
			t.Fatal(err)
		}
	}
	page := rect(t, tt, a.Tr("shellContent"))
	sidebar := rect(t, tt, a.Tr("shellLibraryRegion"))
	thumb := marker(tt, sidebar)
	if thumb.Dx() != 48 || thumb.Dy() != 48 {
		t.Fatalf("initial thumbnail: %v", thumb)
	}
	click(nativeItemLabel(item))
	if !marker(tt, page).Empty() {
		t.Fatal("header jumped to its destination on the opening frame")
	}
	tt.Advance(tokenMotionMorph / 2)
	half := marker(tt, page)
	tt.Advance(tokenMotionMorph)
	if end := marker(tt, page); half.Empty() || half == end || end.Empty() {
		t.Fatalf("header transition: halfway %v, settled %v", half, end)
	}

	click(a.Tr("shellFilterAlbums"))
	tt.Advance(tokenMotionMorph)
	click(a.Tr("shellFilterClear"))
	for range 5 {
		if got := marker(tt, sidebar); got != thumb {
			t.Fatalf("restored sidebar thumbnail: %v, expected %v", got, thumb)
		}
		tt.Advance(tokenMotionMorph / 4)
	}

	click(a.Tr("shellBack"))
	if got := marker(tt, sidebar); got == thumb {
		t.Fatal("Back jumped straight to the sidebar thumbnail")
	}
	tt.Advance(tokenMotionMorph / 2)
	half = marker(tt, sidebar)
	tt.Advance(tokenMotionMorph)
	if end := marker(tt, sidebar); half.Empty() || half == end || end != thumb {
		t.Fatalf("return transition: halfway %v, settled %v, expected %v", half, end, thumb)
	}
}

func TestClosingDockWithReducedMotionFillsPageImmediately(t *testing.T) {
	m, a := nativeViewFixture()
	tt := motionTester(m, a, 1440, 900)
	tt.SetPreferences(ui.Preferences{ReduceMotion: true})
	pageWidth := func() float32 { return rect(t, tt, a.Tr("shellContent")).W }
	wide := pageWidth()
	m.Panel = true
	tt.Frame()
	if pageWidth() >= wide {
		t.Fatal("expected the dock to occupy page space")
	}
	if err := tt.Click(a.Tr("shellClosePanel")); err != nil {
		t.Fatal(err)
	}
	if got := pageWidth(); got != wide {
		t.Fatalf("page width after closing: %.0f, want %.0f", got, wide)
	}
}

func TestFloatingNowPlayingReversesFromVisiblePosition(t *testing.T) {
	m, a := nativeViewFixture()
	tt := motionTester(m, a, 420, 860)
	closeLabel := a.Tr("shellClosePanel")
	if err := tt.Click(a.Tr("shellOpenNowPlaying")); err != nil {
		t.Fatal(err)
	}
	tt.Advance(32 * time.Millisecond)
	tt.Frame()
	position := func() float32 { return rect(t, tt, closeLabel).Y }
	opening := position()
	if err := tt.Click(closeLabel); err != nil {
		t.Fatal(err)
	}
	closing := position()
	if math.Abs(float64(closing-opening)) > 1 {
		t.Fatalf("close jumped from %.1f to %.1f", opening, closing)
	}
	tt.Advance(16 * time.Millisecond)
	tt.Frame()
	duringClose := position()
	// The application menu can reopen the sheet before it finishes closing.
	m.Panel = true
	tt.Frame()
	if reopened := position(); math.Abs(float64(reopened-duringClose)) > 1 {
		t.Fatalf("reopen jumped from %.1f to %.1f", duringClose, reopened)
	}
	tt.Advance(tokenMotionBase)
	if position() >= opening {
		t.Fatal("the reopened panel did not arrive")
	}
}

// Into immersive mode and back, the cover, the lyrics card and the sung
// line in it each travel one way, with no swing past their place, and the
// cover flies whole (square, never a sliver cut by the column or the
// panel it lands in).
func TestImmersiveJourneyIsSmooth(t *testing.T) {
	for _, panel := range []bool{true} {
		t.Run(fmt.Sprintf("from-now-playing-%v", panel), func(t *testing.T) { immersiveJourney(t, panel) })
	}
}

func immersiveJourney(t *testing.T, panel bool) {
	m, a := nativeTimedLyricsFixture()
	m.Panel, m.Appearance, m.Position = panel, "light", 52
	for i := range m.LyricLines {
		m.LyricLines[i].Text = fmt.Sprintf("L%02d I cannot convince you that I love you for a livin, will wait for you", i)
	}
	sung := m.LyricLines[10].Text
	pink := color.RGBA{237, 19, 173, 255}
	pic := image.NewRGBA(image.Rect(0, 0, 16, 16))
	for i := range pic.Pix {
		pic.Pix[i] = []uint8{pink.R, pink.G, pink.B, 255}[i%4]
	}
	m.Current.Artwork = "pink"
	cover := ui.NewBitmap(pic)
	art := a.Artwork
	a.Artwork = func(src string, edge int, p artworkPriority) ui.ImageSource {
		if src == "pink" {
			return cover
		}
		return art(src, edge, p)
	}
	tt := motionTester(m, a, 1440, 900)
	tt.Frame()
	tt.Frame()
	type pose struct{ coverX, coverW, cardX, cardY, line float32 }
	measure := func() pose {
		box := image.Rectangle{}
		img := tt.Image()
		for y := 0; y < 900-nativePlayerHeight; y++ {
			for x := 0; x < 1440; x++ {
				if img.RGBAAt(x, y) == pink {
					box = box.Union(image.Rect(x, y, x+1, y+1))
				}
			}
		}
		if box.Dx() < 8 || math.Abs(float64(box.Dx()-box.Dy())) > 2 {
			t.Fatalf("the cover is cut in flight: %v", box)
		}
		// Without Now Playing, the player bar's cover alone travels.
		card, ok := tt.Find("Lyrics · Content")
		line, _ := tt.Find(sung)
		if !ok || !panel {
			return pose{float32(box.Min.X), float32(box.Dx()), 0, 0, 0}
		}
		return pose{float32(box.Min.X), float32(box.Dx()), card.X, card.Y, line.Y - card.Y}
	}
	journey := func(name string) {
		t.Helper()
		from := measure()
		tt.Key(ui.Cmd|ui.Shift, ui.KeyF)
		var poses []pose
		for i := 0; i < 12; i++ {
			tt.Advance(tokenMotionMorph / 10)
			tt.Frame()
			poses = append(poses, measure())
		}
		to := poses[len(poses)-1]
		prev := from
		for i, p := range poses {
			for _, v := range [][3]float32{{prev.coverX, p.coverX, to.coverX}, {prev.coverW, p.coverW, to.coverW}, {prev.cardX, p.cardX, to.cardX}, {prev.cardY, p.cardY, to.cardY}, {prev.line, p.line, to.line}} {
				// Each step goes toward the end, and never past it.
				if (v[2]-v[0])*(v[1]-v[0]) < -1 || (v[2]-v[1])*(v[2]-v[0]) < -1 {
					t.Fatalf("%s, frame %d: %+v after %+v swings away from %+v", name, i+1, p, prev, to)
				}
			}
			prev = p
		}
	}
	journey("into immersive mode")
	journey("out of immersive mode")
}
