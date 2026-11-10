package main

import (
	"fmt"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

// A scroller's bar shows as the pointer comes over it and as it scrolls,
// fades once the pointer rests on the content, and stays while the
// pointer is on the bar's strip, where it can be grabbed.
func TestScrollbarFadesWhileThePointerRests(t *testing.T) {
	p := palette(true)
	var st ui.ScrollState
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Fill().Background(p.surface).Children(func() {
			scroll := ui.Scroll(c).Grow(1).MinHeight(0).TrackScroll(&st)
			nativeScrollbar(c, scroll, &st, p, nil)
			scroll.Children(func() {
				for i := range 40 {
					ui.Text(c, fmt.Sprint("Row ", i)).Height(24)
				}
			})
		})
	}, 200, 200)
	settle := func(d time.Duration) {
		for elapsed := time.Duration(0); elapsed < d; elapsed += 50 * time.Millisecond {
			tt.Advance(50 * time.Millisecond)
			tt.Frame()
		}
	}
	// The thumb's middle, where MyGo would draw it.
	shown := func() bool {
		bar := nativeScrollbarThumbs(ui.Rect{W: 200, H: 200}, &st, false)[0]
		c := tt.Image().At(int(2*(bar.X+bar.W/2)), int(2*(bar.Y+bar.H/2)))
		r, _, _, _ := c.RGBA()
		bg, _, _, _ := tt.Image().At(2*100, 2*190).RGBA()
		return r>>8 > bg>>8+8
	}
	tt.SetScale(2)
	tt.Frame()
	if shown() {
		t.Fatal("a bar shows before the pointer comes over the scroller")
	}
	tt.Move(60, 100)
	settle(300 * time.Millisecond)
	if !shown() {
		t.Fatal("the bar does not show as the pointer comes over the scroller")
	}
	settle(2 * time.Second)
	if shown() {
		t.Fatal("the bar stays while the pointer rests on the content")
	}
	tt.Move(194, 100)
	settle(2 * time.Second)
	if !shown() {
		t.Fatal("the bar is not there to grab with the pointer on its strip")
	}
	tt.Move(60, 100)
	settle(2 * time.Second)
	st.Y = 0
	tt.Scroll(60, 100, 0, 120)
	settle(300 * time.Millisecond)
	if st.Y == 0 {
		t.Fatal("the scroller did not scroll")
	}
	if !shown() {
		t.Fatal("the bar does not show while the content scrolls")
	}
	// What shows is what grabs: dragging the drawn thumb scrolls.
	tt.Move(194, 100)
	settle(300 * time.Millisecond)
	before := st.Y
	bar := nativeScrollbarThumbs(ui.Rect{W: 200, H: 200}, &st, false)[0]
	tt.Press(bar.X+bar.W/2, bar.Y+bar.H/2)
	tt.Move(bar.X+bar.W/2, bar.Y+bar.H/2+40)
	tt.Release(bar.X+bar.W/2, bar.Y+bar.H/2+40)
	settle(100 * time.Millisecond)
	if st.Y <= before {
		t.Fatalf("dragging the drawn thumb did not scroll (%g -> %g)", before, st.Y)
	}
}
