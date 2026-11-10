package main

import (
	"fmt"
	"testing"

	"github.com/egoist/mygo/ui"

	"ytmgo/internal/catalog"
)

// Items in a horizontal shelf hang from the shelf's top: a card with a
// two-line subtitle and a song column shorter than four songs line up with
// their neighbours instead of being centred against them.
func TestShelvesAlignItemsAtTheTop(t *testing.T) {
	m, a := nativeViewFixture()
	m.SignedIn = true
	m.Route = "home"
	songs := make([]catalog.Item, 5)
	for i := range songs {
		songs[i] = catalog.Item{ID: fmt.Sprintf("shelf-song-%d", i), Title: fmt.Sprintf("Shelf song %d", i), Artist: "Artist", Kind: "song", Artwork: "art-0"}
	}
	cards := []catalog.Item{
		{Title: "Short card", Artist: "Artist", Kind: "album", BrowseID: "short", Artwork: "art-1"},
		{Title: "Long card", Artist: "An artist whose name is long enough to wrap onto a second line", Kind: "album", BrowseID: "long", Artwork: "art-1"},
	}
	m.Page = catalog.Page{Title: "Good evening", Shelves: []catalog.Shelf{{Title: "Listen again", Items: songs}, {Title: "Albums", Items: cards}}}
	tt := ui.NewTester(func(c *ui.Context) { nativeView(c, m, a) }, 1600, 1000)
	tt.Frame()
	top := func(text string) float32 {
		t.Helper()
		r, ok := tt.Find(text)
		if !ok {
			t.Fatalf("%q is not shown", text)
		}
		return r.Y
	}
	if first, fifth := top("Shelf song 0"), top("Shelf song 4"); first != fifth {
		t.Errorf("the fifth song, alone in its column, sits %g below the first", fifth-first)
	}
	if short, long := top("Short card"), top("Long card"); short != long {
		t.Errorf("a card with a two-line subtitle sits %g off its neighbour", long-short)
	}
}

// The card grid (a page of collections without shelves) lines its rows up
// the same way.
func TestCardGridAlignsItemsAtTheTop(t *testing.T) {
	m, a := nativeViewFixture()
	m.SignedIn = true
	m.Route = "library"
	m.Page = catalog.Page{Title: "Library", Items: []catalog.Item{
		{Title: "Short card", Artist: "Artist", Kind: "album", BrowseID: "short", Artwork: "art-1"},
		{Title: "Long card", Artist: "An artist whose name is long enough to wrap onto a second line", Kind: "album", BrowseID: "long", Artwork: "art-1"},
	}}
	tt := ui.NewTester(func(c *ui.Context) { nativeView(c, m, a) }, 1600, 1000)
	tt.Frame()
	short, ok1 := tt.Find("Short card")
	long, ok2 := tt.Find("Long card")
	if !ok1 || !ok2 {
		t.Fatal("cards are not shown")
	}
	if short.Y != long.Y {
		t.Errorf("a card with a two-line subtitle sits %g off its neighbour", long.Y-short.Y)
	}
}
