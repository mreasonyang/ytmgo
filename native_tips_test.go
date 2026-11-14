package main

import (
	"math"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"ytmgo/internal/catalog"
)

// A library row's name shows in a tip only where it is cut off, or in the
// rail, which shows none; the tip goes beside the cover, level with it,
// where it hides none of the rows above or below.
func TestLibraryTipsOnlyWhereNamesAreCut(t *testing.T) {
	short := catalog.Item{Title: "Night Tapes", Artist: "Daft Punk", Kind: "album", BrowseID: "album-1"}
	long := catalog.Item{Title: "Night Tapes, Vol. 2 (Extended Anniversary Edition, Remastered)", Artist: "Daft Punk", Kind: "album", BrowseID: "album-2"}
	// tips counts the copies of it's title a hover adds: the page (Home's
	// quick-access tiles) may show the name too.
	var added int
	hover := func(t *testing.T, collapsed bool, it catalog.Item) (*ui.Tester, *UIModel, ViewActions) {
		m, a := nativeViewFixture()
		m.SignedIn, m.LibraryLoaded, m.SidebarCollapsed = true, true, collapsed
		m.LibraryItems = []catalog.Item{short, long}
		m.Route, m.Page = "browse:other", catalog.Page{Title: "Other"}
		tt := ui.NewTester(func(c *ui.Context) { nativeView(c, m, a) }, 1440, 900)
		tt.Frame()
		tt.Frame()
		before := countText(tt, it.Title)
		defer func() { added = countText(tt, it.Title) - before }()
		r := rect(t, tt, nativeItemLabel(it))
		tt.Move(r.X+r.W/2, r.Y+r.H/2)
		tt.Frame()
		tt.Advance(time.Second)
		tt.Frame()
		return tt, m, a
	}
	if hover(t, false, short); added != 0 {
		t.Fatal("a name shown whole got a tip repeating it")
	}
	if hover(t, false, long); added != 1 {
		t.Fatal("a name cut off got no tip")
	}
	tt, _, a := hover(t, true, short)
	tip, ok := tt.Find(short.Title)
	if !ok || added != 1 {
		t.Fatal("the rail's row got no tip")
	}
	row := rect(t, tt, nativeItemLabel(short))
	if tip.X < row.X+row.W-8 || math.Abs(float64((tip.Y+tip.H/2)-(row.Y+row.H/2))) > 12 {
		t.Fatalf("the rail's tip %v is not beside its row %v", tip, row)
	}
	_ = a
}
