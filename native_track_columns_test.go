package main

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"ytmgo/internal/catalog"
)

// The playing row's indicator has its own slot, so its Album column lines up
// with the other rows'. (It once took width from the row, shifting it.)
func TestNativeTrackColumnsLineUpWhilePlaying(t *testing.T) {
	m, a := nativeViewFixture()
	items := []catalog.Item{
		{ID: "a", Title: "First", Artist: "A", Album: "Album One", Kind: "song", Duration: 200e9},
		{ID: "b", Title: "Second", Artist: "B", Album: "Album Two", Kind: "song", Duration: 200e9},
	}
	m.Current = items[0]
	tt := ui.NewTester(func(c *ui.Context) { nativeTrackList(c, items, m, a, palette(true), false, false, 0) }, 900, 200)
	one, ok1 := tt.Find("Album One")
	two, ok2 := tt.Find("Album Two")
	if !ok1 || !ok2 {
		t.Fatal("album column missing for a mixed list")
	}
	if one.X != two.X {
		t.Fatalf("album column shifts on the playing row: %v vs %v", one.X, two.X)
	}
}

// An album's own track list doesn't repeat the album in every row.
func TestNativeTrackListHidesAlbumOnItsOwnPage(t *testing.T) {
	m, a := nativeViewFixture()
	items := []catalog.Item{
		{ID: "x", Title: "One", Artist: "A", Album: "Same Album", Kind: "song"},
		{ID: "y", Title: "Two", Artist: "A", Album: "Same Album", Kind: "song"},
	}
	tt := ui.NewTester(func(c *ui.Context) { nativeTrackList(c, items, m, a, palette(true), false, true, 0) }, 900, 300)
	if tt.HasText("Same Album") || tt.HasText(a.Tr("shellColumnAlbum")) {
		t.Fatal("an album's own list repeats its album column")
	}
	if !nativeOneAlbum(items) || nativeOneAlbum(append(items, catalog.Item{Album: "Other"})) || nativeOneAlbum([]catalog.Item{{}, {}}) {
		t.Fatal("nativeOneAlbum misjudges")
	}
}
