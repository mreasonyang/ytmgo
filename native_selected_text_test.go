package main

import (
	"image"
	"testing"

	"github.com/egoist/mygo/ui"

	"ytmgo/internal/catalog"
)

// MyGo's theme accent is the inverted text colour (coral means play), so a
// selected row that only sets a wash would inherit near-background text.
// The open library item must stay readable.
func TestNativeSelectedRowsKeepReadableText(t *testing.T) {
	for _, dark := range []bool{true, false} {
		m, a := nativeViewFixture()
		if !dark {
			m.Appearance = "light"
		}
		m.SignedIn = true
		m.LibraryLoaded = true
		m.LibraryItems = []catalog.Item{{Title: "Weekend mixes", Kind: "playlist", BrowseID: "weekend", Artwork: "art-1"}}
		m.Route = "browse:weekend"
		m.Page = catalog.Page{Title: "Saturday", Items: m.Queue}
		tt := ui.NewTester(func(c *ui.Context) { nativeView(c, m, a) }, 1440, 900)
		tt.SetDark(dark)
		tt.Frame()
		p := palette(dark)
		for _, label := range []string{"Weekend mixes"} {
			r, ok := tt.Find(label)
			if !ok {
				t.Fatalf("dark=%v: %q not drawn", dark, label)
			}
			// Find gives the whole row; measure its title line alone: right of
			// the 48 px cover (8 padding, 12 gap) and above the subtitle.
			r = ui.Rect{X: r.X + 68, Y: r.Y + 8, W: r.W - 76, H: r.H/2 - 8}
			// The title's strongest pixel must be far from the row behind it.
			if d := nativeInkContrast(tt.Image(), r, p); d < 0.35 {
				t.Errorf("dark=%v: %q text is too faint (%.2f)", dark, label, d)
			}
		}
	}
}

// nativeInkContrast is the largest luminance distance, in the rect, from the
// panel surface: how strongly the text stands out.
func nativeInkContrast(img *image.RGBA, r ui.Rect, p nativePalette) float64 {
	lum := func(r, g, b uint8) float64 { return (0.2126*float64(r) + 0.7152*float64(g) + 0.0722*float64(b)) / 255 }
	bg := lum(p.surface.R, p.surface.G, p.surface.B)
	best := 0.0
	s := float64(img.Bounds().Dx()) / 1440
	for y := int(float64(r.Y) * s); y < int(float64(r.Y+r.H)*s); y++ {
		for x := int(float64(r.X) * s); x < int(float64(r.X+r.W)*s); x++ {
			c := img.RGBAAt(x, y)
			if d := lum(c.R, c.G, c.B) - bg; d > best || -d > best {
				best = max(d, -d)
			}
		}
	}
	return best
}
