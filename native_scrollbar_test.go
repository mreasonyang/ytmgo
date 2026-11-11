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

// Lyrics following playback scroll by themselves: that isn't the reader, so
// the bar doesn't flash on every line.
func TestScrollbarIgnoresLyricsFollowingPlayback(t *testing.T) {
	m, a := nativeTimedLyricsFixture()
	p := palette(true)
	tt := ui.NewTester(func(c *ui.Context) { nativePanelCard(c, m, a, p).Grow(1) }, 360, 340)
	tt.SetScale(2)
	tt.Frame()
	view, ok := tt.Find("Lyrics · Content")
	if !ok {
		t.Fatal("lyrics viewport missing")
	}
	thumbShown := func() bool {
		img := tt.Image()
		_, _, _, _ = img.At(0, 0).RGBA()
		bg, _, _, _ := img.At(int(2*(view.X+view.W-30)), int(2*(view.Y+4))).RGBA()
		for y := view.Y + 4; y < view.Y+view.H-4; y += 2 {
			r, _, _, _ := img.At(int(2*(view.X+view.W-3-nativeScrollbarWidth/2)), int(2*y)).RGBA()
			if r>>8 > bg>>8+8 {
				return true
			}
		}
		return false
	}
	// Each line's glide, sampled on its way and once it has landed.
	for line := 1; line < 5; line++ {
		m.Position = float64(line*5) + 0.5
		tt.Frame()
		for _, step := range []time.Duration{tokenMotionSlow / 2, tokenMotionSlow} {
			tt.Advance(step)
			tt.Frame()
			if thumbShown() {
				t.Fatalf("the bar flashed as the lyrics followed line %d", line)
			}
		}
	}
	requireLyricCentered(t, tt, 4) // they did follow, gliding
}

// The page panel ends as the library beside it does: rows reaching its
// foot fade into the panel while more lies below, never cut flat.
func TestPageFootFadesWhileMoreLiesBelow(t *testing.T) {
	m, a := nativeViewFixture()
	m.Appearance, m.Panel = "light", false
	tt := ui.NewTester(func(c *ui.Context) { nativeView(c, m, a) }, 1280, 720)
	tt.Frame()
	page := rect(t, tt, a.Tr("shellContent"))
	// The album covers run past the foot at this height, 24 in from the
	// page's left edge plus a cover's half.
	x, foot := int(page.X+24+80), int(page.Y+page.H)-1
	img := tt.Image()
	surface := tokenLight.surface
	near := func(a, b uint8) bool { return max(a, b)-min(a, b) < 8 }
	if c := img.RGBAAt(x, foot); !near(c.R, surface.R) || !near(c.G, surface.G) || !near(c.B, surface.B) {
		t.Fatalf("the page's foot is cut, not faded: %v at its last row", c)
	}
	if c := img.RGBAAt(x, foot-nativeFadeBelowSize-4); c.R > 200 && c.G > 200 && c.B > 200 {
		t.Fatalf("no cover above the fade to fade: %v", c)
	}
}
