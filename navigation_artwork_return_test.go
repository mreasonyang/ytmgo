package main

import (
	"fmt"
	"image"
	"testing"

	"github.com/egoist/mygo/ui"
	"ytmgo/internal/catalog"
)

func navigationArtworkPreview(a *desktop, raw string) *artEntry {
	frame := a.art.beginFrame()
	key := frame.key(raw, 384, artPrimary)
	pool := a.art.previewPool(key)
	entry := &artEntry{bitmap: ui.NewBitmap(image.NewRGBA(image.Rect(0, 0, 48, 48))), local: true}
	pool.entries[key] = entry
	return entry
}

func TestNavigationArtworkReturnDropsUnpaintedAndBranchedSnapshots(t *testing.T) {
	a := &desktop{historyIndex: -1, art: newArtworkCache(t.Context(), nil), model: UIModel{Route: "home"}}
	a.pushHistory(true)
	const raw = "https://lh3.googleusercontent.com/navigation-origin=s384"
	preview := navigationArtworkPreview(a, raw)
	a.model.pageViewport.y = 201
	oldDraw := a.lastArtworkView.paintPass()
	oldDraw.record(raw, 384, preview.bitmap)
	a.loadPage("downloads", "", true)
	if !preview.returnPreview || a.returnPage.viewport.y != 201 {
		t.Fatal("leaving the painted origin did not protect its preview and viewport")
	}
	oldDraw.record(raw, 384, preview.bitmap)
	if len(a.lastArtworkView.items()) != 0 {
		t.Fatal("obsolete Draw repopulated the departing snapshot")
	}
	a.loadPage("library", "", true)
	if preview.returnPreview || a.returnPage.viewport.y != 0 {
		t.Fatal("quick navigation inherited the viewport that preceded the unpainted page")
	}
	a.travel(-1)
	a.travel(-1)
	if a.model.Route != "home" || a.model.restorePageViewport != nil {
		t.Fatal("second Back restored a viewport from beyond the direct predecessor")
	}
	a.model.pageViewport.y = 301
	a.lastArtworkView.paintPass().record(raw, 384, preview.bitmap)
	a.travel(1)
	if !preview.returnPreview || a.returnPage.viewport.y != 301 {
		t.Fatal("Forward did not protect the currently painted origin")
	}
	a.travel(-1)
	staleReturn := a.model.restorePageViewport
	a.model.pageViewport.y = 408
	a.lastArtworkView.paintPass().record(raw, 384, preview.bitmap)
	a.loadPage("library", "", true)
	if !preview.returnPreview || a.model.CanForward || len(a.history) != 2 {
		t.Fatal("branch did not replace forward history and protect the new return")
	}
	a.travel(-1)
	a.windowVisible = true
	a.finishPageReturn(staleReturn)
	if !preview.returnPreview || a.model.restorePageViewport == nil || a.model.restorePageViewport.y != 408 {
		t.Fatal("an older return callback released the newly branched return")
	}
	a.clearAccountContent()
	if preview.returnPreview || a.returnPage != nil || a.model.restorePageViewport != nil || a.model.pageViewport.y != 0 ||
		a.model.CanBack || a.model.CanForward || len(a.history) != 0 || a.historyIndex != -1 {
		t.Fatal("account clear retained navigation state or protected artwork")
	}
}

func TestNavigationArtworkReturnSurvivesHistoryCapAndReplacement(t *testing.T) {
	a := &desktop{historyIndex: -1, art: newArtworkCache(t.Context(), nil), model: UIModel{Route: "downloads"}}
	for range 24 {
		a.pushHistory(true)
	}
	const raw = "https://lh3.googleusercontent.com/navigation-cap=s384"
	preview := navigationArtworkPreview(a, raw)
	a.model.pageViewport.y = 638
	a.lastArtworkView.paintPass().record(raw, 384, preview.bitmap)
	// The same route can still be a different history entry.
	a.loadPage("downloads", "", true)
	if len(a.history) != 24 || a.returnPage.index != 22 || !preview.returnPreview {
		t.Fatal("history cap lost the displaced origin")
	}
	const dest = "https://lh3.googleusercontent.com/navigation-replaced=s384"
	replaced := navigationArtworkPreview(a, dest)
	a.model.pageViewport.y = 97
	oldDraw := a.lastArtworkView.paintPass()
	oldDraw.record(dest, 384, replaced.bitmap)
	a.finishPageContinuation(a.pageGeneration, catalog.Page{Title: "Replacement"}, true, nil)
	oldDraw.record(dest, 384, replaced.bitmap)
	if !preview.returnPreview || len(a.lastArtworkView.items()) != 0 || a.model.pageViewport.y != 0 || len(a.history) != 24 {
		t.Fatal("asynchronous replacement disturbed the predecessor or retained obsolete current samples")
	}
	a.travel(-1)
	if !preview.returnPreview || a.model.restorePageViewport == nil || a.model.restorePageViewport.y != 638 {
		t.Fatal("Back applied the wrong viewport after truncation and replacement")
	}
	a.windowVisible = true
	a.finishPageReturn(a.model.restorePageViewport)
	if preview.returnPreview || a.model.restorePageViewport != nil {
		t.Fatal("completed return retained protection")
	}
}

func TestNavigationArtworkReturnReleasesAfterFirstVisibleRestoredPaint(t *testing.T) {
	m, actions := nativeViewFixture()
	m.Route, m.SignedIn = "library", true
	m.Current, m.Queue, m.Playing = catalog.Item{}, nil, false
	m.Page = catalog.Page{Title: "Library"}
	for row := range 8 {
		shelf := catalog.Shelf{Title: fmt.Sprintf("Shelf %d", row)}
		for col := range 12 {
			id := fmt.Sprintf("Card %d-%d", row, col)
			shelf.Items = append(shelf.Items, catalog.Item{Title: id, Kind: "album", BrowseID: id})
		}
		m.Page.Shelves = append(m.Page.Shelves, shelf)
	}
	original := m.Page
	a := &desktop{historyIndex: -1, art: newArtworkCache(t.Context(), nil), model: *m, windowVisible: true}
	a.pushHistory(true)
	actions.LoadMore = nil
	const raw = "https://lh3.googleusercontent.com/navigation-render=s384"
	preview := navigationArtworkPreview(a, raw)
	var returned nativePageViewport
	painted := 0
	actions.pageReturnPainted = func(viewport *nativePageViewport) {
		if !preview.returnPreview {
			t.Error("preview was released before the returned viewport painted")
		}
		if a.windowVisible {
			painted++
			returned = a.model.pageViewport
		}
		a.finishPageReturn(viewport)
	}
	builds := 0
	tt := ui.NewTester(func(c *ui.Context) {
		builds++
		nativeView(c, &a.model, actions)
	}, 1100, 900)
	center := func(label string) (float32, float32) {
		t.Helper()
		r, ok := tt.Find(label)
		if !ok {
			t.Fatalf("%q was missing from the viewport", label)
		}
		return r.X + r.W/2, r.Y + r.H/2
	}
	pageX, pageY := center(actions.Tr("shellContent"))
	x, y := center("Card 0-0")
	tt.Scroll(x, y, 512, 0)
	first := nativeShelfKey{index: 0, title: "Shelf 0"}
	if a.model.pageViewport.shelfX(first) == 0 {
		t.Fatal("the initial horizontal scroll was not captured")
	}
	tt.Scroll(pageX, pageY, 0, 920)
	x, y = center("Card 3-0")
	tt.Scroll(x, y, 450, 0)
	want := a.model.pageViewport
	visible := nativeShelfKey{index: 3, title: "Shelf 3"}
	if want.y <= 0 || want.shelfX(visible) <= 0 || want.shelfX(first) != 0 {
		t.Fatalf("departure viewport has wrong offsets (y=%g, visible=%g, offscreen=%g)", want.y, want.shelfX(visible), want.shelfX(first))
	}
	a.lastArtworkView.paintPass().record(raw, 384, preview.bitmap)
	a.loadPage("downloads", "", true)
	tt.Frame()
	request := &libraryRequest{}
	a.libraryRequest = request
	a.history[0].needsReload = true
	a.travel(-1)
	tt.Frame()
	if !a.model.Loading || !preview.returnPreview || a.model.restorePageViewport == nil || painted != 0 {
		t.Fatal("returning loading frame consumed the viewport or its protection")
	}
	a.windowVisible = false
	a.finishLibrary(request, original, nil)
	tt.Frame()
	if !preview.returnPreview || a.model.restorePageViewport == nil || painted != 0 {
		t.Fatal("painting while hidden consumed the viewport or its protection")
	}
	a.windowVisible = true
	beforeVisible := builds
	tt.Frame()
	if builds != beforeVisible+1 || tt.FramePending() {
		t.Fatal("releasing the finished return requested an extra build")
	}
	if painted != 1 || returned.y != want.y || returned.shelfX(visible) != want.shelfX(visible) || returned.shelfX(first) != 0 {
		t.Fatalf("first visible return had wrong viewport (paints=%d, y=%g/%g, visible=%g/%g, offscreen=%g)", painted,
			returned.y, want.y, returned.shelfX(visible), want.shelfX(visible), returned.shelfX(first))
	}
	if preview.returnPreview || a.model.restorePageViewport != nil {
		t.Fatal("first visible restored paint did not release the protection")
	}
	tt.Scroll(pageX, pageY, 0, -920)
	tt.Frame()
	if painted != 1 || a.model.pageViewport.y != 0 || a.model.pageViewport.shelfX(first) != 0 {
		t.Fatal("a later scroll reapplied the return or restored an offscreen shelf")
	}
}
