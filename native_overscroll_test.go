package main

import (
	"math"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

// A touchpad pulling the page past its top stretches it, less than the
// pull, and it springs back once let go; scrolling into the page scrolls.
func TestPageStretchesPastItsEndsAndSpringsBack(t *testing.T) {
	m, a := nativeViewFixture()
	tt := ui.NewTester(func(c *ui.Context) { nativeView(c, m, a) }, 1440, 640)
	tt.Frame()
	title := func() float32 { return rect(t, tt, "Good evening").Y }
	rest := title()
	tt.Scroll(800, 300, 0, -60)
	tt.Scroll(800, 300, 0, -60)
	tt.Frame()
	pulled := title() - rest
	if pulled <= 4 || pulled >= 120 {
		t.Fatalf("pulled 120 past the top, the page moved %.1f; want a stretch, less than the pull", pulled)
	}
	// Pulling on stretches it further, ever less.
	tt.Scroll(800, 300, 0, -120)
	tt.Frame()
	if more := title() - rest; more <= pulled || more-pulled >= 120 {
		t.Fatalf("pulling 120 more moved it %.1f → %.1f", pulled, more)
	}
	// Let go: it springs back.
	for range 12 {
		tt.Advance(50 * time.Millisecond)
		tt.Frame()
	}
	if back := title(); math.Abs(float64(back-rest)) > 0.5 {
		t.Fatalf("let go, the page rests %.1f from its top", back-rest)
	}
	// Into the page, it scrolls as before.
	tt.Scroll(800, 300, 0, 100)
	tt.Frame()
	if y := m.pageScroll.Y; math.Abs(float64(y-100)) > 0.5 {
		t.Fatalf("scrolling 100 into the page scrolled it to %.1f", y)
	}
}

// Stretched, a pull the other way first takes the stretch back, then
// scrolls the page.
func TestStretchGivesWayToScrollingBack(t *testing.T) {
	m, a := nativeViewFixture()
	tt := ui.NewTester(func(c *ui.Context) { nativeView(c, m, a) }, 1440, 640)
	tt.Frame()
	tt.Scroll(800, 300, 0, -40)
	tt.Frame()
	tt.Scroll(800, 300, 0, 100) // 40 to undo the stretch, then 60 into the page
	tt.Frame()
	if y := m.pageScroll.Y; math.Abs(float64(y-60)) > 0.5 {
		t.Fatalf("after the stretch, scrolling back scrolled the page to %.1f, want 60", y)
	}
}

// Stretched at the end while the next page loads, the pull goes on into
// the songs that came in, rather than stretching further.
func TestStretchGivesWayToContentThatArrives(t *testing.T) {
	m, a := nativeViewFixture()
	tt := ui.NewTester(func(c *ui.Context) { nativeView(c, m, a) }, 1440, 640)
	tt.Frame()
	tt.Scroll(800, 300, 0, 1e6)
	tt.Frame()
	end := m.pageScroll.MaxY
	tt.Scroll(800, 300, 0, 80) // past the end: a stretch
	tt.Frame()
	m.Page.Shelves = append(m.Page.Shelves, m.Page.Shelves...)
	tt.Frame()
	tt.Frame()
	if m.pageScroll.MaxY <= end {
		t.Fatal("the page did not grow")
	}
	tt.Scroll(800, 300, 0, 100)
	tt.Frame()
	if y := m.pageScroll.Y; math.Abs(float64(y-(end+100))) > 0.5 {
		t.Fatalf("after more came in, the pull scrolled the page to %.1f, want %.1f", y, end+100)
	}
}
