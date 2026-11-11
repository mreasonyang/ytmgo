package main

import (
	"fmt"
	"image"
	"testing"

	"github.com/egoist/mygo/ui"
)

// The page lays out from the width the frame gives it, computed as the
// frame changes, so it matches the panel it is drawn in.
func TestPageWidthMatchesItsPanel(t *testing.T) {
	for _, width := range []int{900, 1180, 1181, 1440, 1920} {
		for _, panel := range []bool{false, true} {
			for _, collapsed := range []bool{false, true} {
				t.Run(fmt.Sprintf("%d-panel=%v-collapsed=%v", width, panel, collapsed), func(t *testing.T) {
					m, a := nativeViewFixture()
					m.Panel, m.SidebarCollapsed = panel, collapsed
					tt := ui.NewTester(func(c *ui.Context) { nativeView(c, m, a) }, width, 900)
					r, ok := tt.Find(a.Tr("shellContent"))
					if !ok {
						t.Fatal("page missing")
					}
					if want := nativePageWidth(m, float32(width)); r.W != want {
						t.Fatalf("page is %g wide, laid out for %g", r.W, want)
					}
				})
			}
		}
	}
}

// Opening Now Playing narrows the page in the same frame: no frame shows
// the page laid out for its old width.
func TestOpeningNowPlayingDrawsNoStaleFrame(t *testing.T) {
	m, a := nativeViewFixture()
	m.Panel = false
	tt := ui.NewTester(func(c *ui.Context) { nativeView(c, m, a) }, 1440, 900)
	tt.Frame()
	m.Panel, m.PanelTab = true, "lyrics"
	tt.Frame()
	first := tt.Image()
	tt.Frame()
	tt.Frame()
	if n := changedPixels(first, tt.Image()); n > 0 {
		t.Fatalf("the first frame with the panel open changed %d pixels as it settled", n)
	}
}

func changedPixels(a, b image.Image) int {
	n := 0
	r := a.Bounds()
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			ar, ag, ab, _ := a.At(x, y).RGBA()
			br, bg, bb, _ := b.At(x, y).RGBA()
			if ar != br || ag != bg || ab != bb {
				n++
			}
		}
	}
	return n
}
