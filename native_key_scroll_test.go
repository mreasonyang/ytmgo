package main

import (
	"fmt"
	"math"
	"testing"

	"github.com/egoist/mygo/ui"
)

// pageOffset reports how far the page has scrolled since tops was taken
// (pageTops), by a shelf title in view now: the harness clamps one scrolled
// out of view to the viewport's edge.
func pageOffset(t *testing.T, tt *ui.Tester, tops []float32, view ui.Rect) float32 {
	t.Helper()
	for i, top := range tops {
		r, ok := tt.Find(fmt.Sprintf("Shelf %d", i))
		if ok && r.H > 0 && r.Y > view.Y+1 && r.Y+r.H < view.Y+view.H-1 {
			return top - r.Y
		}
	}
	t.Fatal("no shelf title in view")
	return 0
}

// pageTops are where the shelf titles sit, in view or not.
func pageTops(tt *ui.Tester) []float32 {
	var tops []float32
	for i := 0; ; i++ {
		r, ok := tt.Find(fmt.Sprintf("Shelf %d", i))
		if !ok {
			return tops
		}
		tops = append(tops, r.Y)
	}
}

// The keys that page (Page Down, End…) glide over the base motion, as
// browsers smooth them, where MyGo jumped: with the pointer over the page,
// Page Down is under way half way through and a page down once it lands.
func TestScrollKeysGlideThePage(t *testing.T) {
	m, a := nativeViewFixture()
	m.SignedIn, m.Route, m.Panel = true, "home", false
	m.Page = nativeContinuationPages(m)["home"]
	tt := ui.NewTester(func(c *ui.Context) { nativeView(c, m, a) }, 1440, 900)
	view, _ := tt.Find(a.Tr("shellContent"))
	tt.Move(view.X+view.W/2, view.Y+view.H/2)
	tt.Frame()
	tops := pageTops(tt)
	tt.Key(0, ui.KeyPageDown)
	tt.Advance(tokenMotionBase / 2)
	tt.Frame()
	half := pageOffset(t, tt, tops, view)
	tt.Advance(tokenMotionBase)
	tt.Frame()
	tt.Frame()
	page := max(view.H-40, view.H/2)
	landed := pageOffset(t, tt, tops, view)
	if math.Abs(float64(landed-page)) > 1 {
		t.Fatalf("Page Down moved %.1f, not a page (%.1f)", landed, page)
	}
	if half <= 1 || half >= page-1 {
		t.Fatalf("Page Down jumped instead of gliding: %.1f of %.1f half way", half, page)
	}
	tt.Key(0, ui.KeyHome)
	tt.Advance(2 * tokenMotionBase)
	tt.Frame()
	tt.Frame()
	if d := pageOffset(t, tt, tops, view); math.Abs(float64(d)) > 1 {
		t.Fatalf("Home left the page %.1f from its top", d)
	}
	// Choosing Home again, scrolled down, glides the page to its top.
	tt.Key(0, ui.KeyPageDown)
	tt.Advance(2 * tokenMotionBase)
	tt.Frame()
	if !m.glideToTop("home") {
		t.Fatal("the scrolled page did not glide to its top")
	}
	tt.Frame()
	tt.Advance(tokenMotionSlow / 2)
	tt.Frame()
	if d := pageOffset(t, tt, tops, view); d <= 1 || d >= page-1 {
		t.Fatalf("the page jumped to its top instead of gliding: %.1f half way", d)
	}
	tt.Advance(tokenMotionSlow)
	tt.Frame()
	tt.Frame()
	if d := pageOffset(t, tt, tops, view); math.Abs(float64(d)) > 1 {
		t.Fatalf("the glide to the top stopped %.1f short", d)
	}
}

// A focused text field keeps the keys it edits with: Home and End move its
// caret, not the page under the pointer.
func TestScrollKeysLeaveTheSearchField(t *testing.T) {
	m, a := nativeViewFixture()
	m.SignedIn, m.Route, m.Panel = true, "home", false
	m.Page = nativeContinuationPages(m)["home"]
	tt := ui.NewTester(func(c *ui.Context) { nativeView(c, m, a) }, 1440, 900)
	view, _ := tt.Find(a.Tr("shellContent"))
	if err := tt.Click(a.Tr("shellNavSearch")); err != nil {
		t.Fatal(err)
	}
	tt.Type("daft")
	tt.Move(view.X+view.W/2, view.Y+view.H/2)
	tt.Frame()
	tops := pageTops(tt)
	for _, key := range []ui.Key{ui.KeyEnd, ui.KeyHome, ui.KeyEnd} {
		tt.Key(0, key)
	}
	tt.Advance(2 * tokenMotionBase)
	tt.Frame()
	if d := pageOffset(t, tt, tops, view); d != 0 {
		t.Fatalf("Home/End in the search field scrolled the page by %.1f", d)
	}
	if m.Query != "daft" {
		t.Fatalf("the search field lost its text: %q", m.Query)
	}
}

// Choosing the page already shown glides it back to its top when it is
// scrolled down; at the top, or another page, it navigates as before.
func TestChoosingTheShownPageGlidesToItsTop(t *testing.T) {
	scrolled := &ui.ScrollState{Y: 900}
	m := &UIModel{Route: "home", pageScroll: scrolled}
	if !m.glideToTop("home") || !scrolled.Scrolling() {
		t.Fatal("Home, chosen on a scrolled Home, did not glide to its top")
	}
	for name, m := range map[string]*UIModel{
		"another page":     {Route: "library", pageScroll: &ui.ScrollState{Y: 900}},
		"at the top":       {Route: "home", pageScroll: &ui.ScrollState{}},
		"loading":          {Route: "home", Loading: true, pageScroll: &ui.ScrollState{Y: 900}},
		"immersive":        {Route: "home", Immersive: true, pageScroll: &ui.ScrollState{Y: 900}},
		"search (focuses)": {Route: "search", pageScroll: &ui.ScrollState{Y: 900}},
	} {
		route := "home"
		if name == "search (focuses)" {
			route = "search"
		}
		if m.glideToTop(route) {
			t.Errorf("%s: glided instead of navigating", name)
		}
	}
}
