package main

import (
	"fmt"
	"image"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"ytmgo/internal/catalog"
)

func TestArtworkReturnViewTracksOnlyPaintedPageCovers(t *testing.T) {
	m, actions := nativeViewFixture()
	m.Route = "browse:fixture"
	m.Page = catalog.Page{Title: "Scroll fixture"}
	for i := range 16 {
		id := fmt.Sprintf("page-%02d", i)
		item := catalog.Item{BrowseID: id, Artwork: id, Title: id, Kind: "album"}
		m.Page.Shelves = append(m.Page.Shelves, catalog.Shelf{Title: id, Items: []catalog.Item{item}})
	}
	m.LibraryLoaded, m.SignedIn = true, true
	m.LibraryItems = []catalog.Item{{BrowseID: "side", Kind: "album", Title: "Library", Artwork: "sidebar-only"}}
	m.Current.Artwork = "player-only"
	pic := ui.NewBitmap(image.NewRGBA(image.Rect(0, 0, 48, 48)))
	actions.Artwork = func(string, int, artworkPriority) ui.ImageSource { return pic }
	actions.artworkPeek = func(string, int) ui.ImageSource { return pic }
	var view artworkReturnView
	actions.artworkReturnView = &view
	tt := ui.NewTester(func(c *ui.Context) { nativeView(c, m, actions) }, 1200, 800)
	tt.Advance(time.Second)
	initial := view.items()
	if len(initial) == 0 || len(initial) >= len(m.Page.Shelves) {
		t.Fatalf("captured %d of %d shelves", len(initial), len(m.Page.Shelves))
	}
	first := initial[0].raw
	for _, item := range initial {
		if !strings.HasPrefix(item.raw, "page-") || item.bitmap.Value() != pic {
			t.Fatalf("captured an image outside the page or with wrong identity: %q", item.raw)
		}
	}
	r, ok := tt.Find(actions.Tr("shellContent"))
	if !ok {
		t.Fatal("page scroller missing")
	}
	tt.Scroll(r.X+r.W/2, r.Y+r.H/2, 0, 1600)
	tt.Advance(time.Second)
	if len(view.items()) == 0 {
		t.Fatal("scrolled viewport did not retain any painted covers")
	}
	for _, item := range view.items() {
		if item.raw == first || !strings.HasPrefix(item.raw, "page-") {
			t.Fatalf("scrolled viewport retained an offscreen cover: %q", item.raw)
		}
	}
	// Draw callbacks in old transition layers cannot repopulate the next page.
	pass := view.paintPass()
	view.reset()
	pass.record(first, 384, pic)
	if len(view.items()) != 0 {
		t.Fatal("retired paint pass recorded a cover")
	}
}

func TestArtworkReturnViewLimitsUniquePaintedCovers(t *testing.T) {
	pic := ui.NewBitmap(image.NewRGBA(image.Rect(0, 0, 2, 2)))
	var view artworkReturnView
	pass := view.paintPass()
	for i := range artworkReturnCoverLimit + 8 {
		raw := fmt.Sprintf("fixture-%d", i)
		pass.record(raw, 300, pic)
		pass.record(raw, 350, pic)
	}
	items := view.items()
	if len(items) != artworkReturnCoverLimit {
		t.Fatalf("tracked %d covers, want %d", len(items), artworkReturnCoverLimit)
	}
	for i, item := range items {
		if item.raw != fmt.Sprintf("fixture-%d", i) || item.edge != artworkBucket(300) || item.bitmap.Value() != pic {
			t.Fatalf("duplicate or wrong cover at slot %d", i)
		}
	}
}
