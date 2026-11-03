package main

import (
	"fmt"
	"testing"

	"github.com/egoist/mygo/ui"

	"ytmgo/internal/catalog"
)

func TestNativeLibrarySkeletonWhileLoading(t *testing.T) {
	for _, width := range []int{1100, 1440} {
		m, a := nativeViewFixture()
		m.SignedIn = true
		tt := ui.NewTester(func(c *ui.Context) { nativeView(c, m, a) }, width, 900)
		if _, ok := tt.Find(a.Tr("nativeLibraryLoadingTitle")); !ok {
			t.Fatalf("%d: loading library has no accessible name", width)
		}
		if tt.HasText(a.Tr("nativeLibraryLoadingBody")) {
			t.Fatalf("%d: loading library still explained in prose", width)
		}
		nativeSnapshot(t, tt, fmt.Sprintf("library-skeleton-%d", width))

		m.LibraryErrorKey = "nativeCatalogError"
		tt.Frame()
		if _, ok := tt.Find(a.Tr("nativeLibraryLoadingTitle")); ok {
			t.Fatalf("%d: failed library still shown as loading", width)
		}
		m.LibraryErrorKey, m.LibraryLoaded = "", true
		tt.Frame()
		if _, ok := tt.Find(a.Tr("nativeLibraryLoadingTitle")); ok || (width > 1180 && !tt.HasText(a.Tr("nativeLibraryEmptyTitle"))) {
			t.Fatalf("%d: empty library still shown as loading", width)
		}
	}
}

// nativeContentEnd is where the content column's inset ends on the right.
func nativeContentEnd(t *testing.T, tt *ui.Tester, a ViewActions, compact bool) float32 {
	t.Helper()
	r, ok := tt.Find(a.Tr("shellContent"))
	if !ok {
		t.Fatal("content region missing")
	}
	return r.X + r.W - nativeContentInset(compact)
}

func TestNativeSongShelfFillsWidth(t *testing.T) {
	for _, width := range []int{760, 1100, 1440} {
		m, a := nativeViewFixture()
		tt := ui.NewTester(func(c *ui.Context) { nativeView(c, m, a) }, width, 900)
		end := nativeContentEnd(t, tt, a, width <= 760)
		// "Within" ends the second, last column of "Listen again".
		r, ok := tt.Find("5:20")
		if !ok {
			t.Fatal("last song column missing")
		}
		if gap := end - (r.X + r.W); gap < 0 || gap > 24 {
			t.Fatalf("%d: song columns leave %.0f DIP at the right", width, gap)
		}
	}
}

func TestNativeCardGridFillsWidth(t *testing.T) {
	for _, width := range []int{760, 1100, 1280, 1440, 1920} {
		m, a := nativeViewFixture()
		var cards []catalog.Item
		for i := range 13 {
			cards = append(cards, catalog.Item{Title: fmt.Sprintf("Card %d", i), Artist: "Daft Punk", Kind: "album", BrowseID: fmt.Sprintf("card-%d", i), Artwork: fmt.Sprintf("art-%d", i%8)})
		}
		m.Route, m.Page = "library", catalog.Page{Title: "Library", Items: cards}
		tt := ui.NewTester(func(c *ui.Context) { nativeView(c, m, a) }, width, 900)
		first, ok := tt.Find("Card 0")
		if !ok {
			t.Fatal("grid missing")
		}
		columns, cell := 1, float32(0)
		for i := 1; i < len(cards); i++ {
			r, ok := tt.Find(fmt.Sprintf("Card %d", i))
			if !ok || r.Y != first.Y {
				break
			}
			columns, cell = i+1, (r.X-first.X)/float32(i)
		}
		if columns < 2 {
			t.Fatalf("%d: one card per row", width)
		}
		art := cell - 24
		if art < nativeCardArt(width <= 760) {
			t.Fatalf("%d: cards shrank to %.0f DIP", width, art)
		}
		// Cards sit 12 DIP inside their cell, which overhangs the inset by 12.
		right := first.X - 12 + float32(columns)*cell - 12
		if gap := nativeContentEnd(t, tt, a, width <= 760) - right; gap < -1 || gap > float32(columns) {
			t.Fatalf("%d: %d columns leave %.0f DIP at the right", width, columns, gap)
		}
		if width == 1280 {
			nativeSnapshot(t, tt, "card-grid-1280")
		}
	}
}
