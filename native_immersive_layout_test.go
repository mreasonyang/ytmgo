package main

import (
	"image"
	"image/color"
	"math"
	"testing"

	"github.com/egoist/mygo/ui"
)

// Immersive mode's media column and card are one group: centred in the
// stage, nativeImmersivePair apart, the card from the cover's top to the
// artist's line.
func TestImmersiveMediaAndCardAreOneCentredGroup(t *testing.T) {
	coverColor := color.RGBA{237, 19, 173, 255}
	pic := image.NewRGBA(image.Rect(0, 0, 16, 16))
	for i := range pic.Pix {
		pic.Pix[i] = []uint8{coverColor.R, coverColor.G, coverColor.B, 255}[i%4]
	}
	cover := ui.NewBitmap(pic)
	for _, size := range [][2]int{{1365, 765}, {1920, 1080}, {1000, 700}} {
		m, a := nativeViewFixture()
		m.Immersive, m.Video, m.Appearance = true, false, "light"
		m.Current.Title, m.Current.Artwork = "WAIT FOR U (feat. Drake & Tems)", "distinct-cover"
		art := a.Artwork
		a.Artwork = func(src string, edge int, p artworkPriority) ui.ImageSource {
			if src == "distinct-cover" {
				return cover
			}
			return art(src, edge, p)
		}
		w, h := size[0], size[1]
		tt := ui.NewTester(func(c *ui.Context) { nativeView(c, m, a) }, w, h)
		tt.Frame()
		box := image.Rectangle{}
		img := tt.Image()
		for y := 0; y < h-nativePlayerHeight; y++ {
			for x := 0; x < w; x++ {
				if img.RGBAAt(x, y) == coverColor {
					box = box.Union(image.Rect(x, y, x+1, y+1))
				}
			}
		}
		title, artist := rect(t, tt, m.Current.Title), rect(t, tt, m.Current.Artist)
		chips := rect(t, tt, a.Tr("shellLyrics")+" · "+a.Tr("shellNowPlaying"))
		lyrics := rect(t, tt, a.Tr("shellLyrics")+" · "+a.Tr("shellContent"))
		// The card pads its chips and lyrics by 12.
		cardTop, cardLeft, cardRight, cardBottom := chips.Y-12, chips.X-12, lyrics.X+lyrics.W+12, lyrics.Y+lyrics.H+12
		left := min(float32(box.Min.X), title.X)
		right := max(float32(box.Max.X), title.X+title.W)
		edge := float32(tokenProductFrameGap)
		if l, r := left-edge, float32(w)-edge-cardRight; math.Abs(float64(l-r)) > 2 {
			t.Errorf("%v: the group is off centre: %.0f to its left, %.0f to its right", size, l, r)
		}
		if gap := cardLeft - right; math.Abs(float64(gap-nativeImmersivePair)) > 2 {
			t.Errorf("%v: %.0f between the column and the card, want %d", size, gap, nativeImmersivePair)
		}
		if math.Abs(float64(cardTop-float32(box.Min.Y))) > 2 || math.Abs(float64(cardBottom-(artist.Y+artist.H))) > 4 {
			t.Errorf("%v: the card spans %.0f–%.0f, the column %d–%.0f", size, cardTop, cardBottom, box.Min.Y, artist.Y+artist.H)
		}
	}
}
