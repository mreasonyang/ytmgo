package main

import (
	"image"
	"math"

	"github.com/egoist/mygo/ui"
)

// artworkTint finds the colour a cover lends the interface: the hue that
// carries the most colour in it, as the playing track's wash behind Now
// Playing and the player bar (design/README.md: "the music leads").
//
// Near-black, near-white and grey pixels don't count, and the hue's weight
// grows with its pixels' saturation and brightness, so a dusk photograph
// gives its sunset, not its night. The result is normalised to one brightness
// and a bounded saturation, so text over the wash stays readable whatever
// the cover. A cover without colour gives none.
func artworkTint(im image.Image) (ui.Color, bool) {
	const grid, bins = 32, 18
	b := im.Bounds()
	if b.Empty() {
		return ui.Color{}, false
	}
	var weight [bins]float64
	var sum [bins][3]float64
	for gy := 0; gy < grid; gy++ {
		for gx := 0; gx < grid; gx++ {
			x := b.Min.X + (2*gx+1)*b.Dx()/(2*grid)
			y := b.Min.Y + (2*gy+1)*b.Dy()/(2*grid)
			r16, g16, b16, a16 := im.At(x, y).RGBA()
			if a16 == 0 {
				continue
			}
			r, g, bl := float64(r16)/65535, float64(g16)/65535, float64(b16)/65535
			h, s, v := artworkHSV(r, g, bl)
			if v < 0.15 || s < 0.2 {
				continue
			}
			bin := int(h/360*bins) % bins
			w := s * s * v * v
			weight[bin] += w
			sum[bin][0] += r * w
			sum[bin][1] += g * w
			sum[bin][2] += bl * w
		}
	}
	best := 0
	for i := range weight {
		if weight[i] > weight[best] {
			best = i
		}
	}
	// Under a handful of vivid sample pixels' worth, the cover has no colour
	// worth lending.
	if weight[best] < 4 {
		return ui.Color{}, false
	}
	h, s, _ := artworkHSV(sum[best][0]/weight[best], sum[best][1]/weight[best], sum[best][2]/weight[best])
	r, g, bl := artworkRGB(h, math.Max(0.4, math.Min(0.75, s)), 0.62)
	return ui.Color{R: uint8(r*255 + 0.5), G: uint8(g*255 + 0.5), B: uint8(bl*255 + 0.5), A: 255}, true
}

func artworkHSV(r, g, b float64) (h, s, v float64) {
	hi, lo := math.Max(r, math.Max(g, b)), math.Min(r, math.Min(g, b))
	v = hi
	if hi == 0 {
		return 0, 0, 0
	}
	d := hi - lo
	s = d / hi
	if d == 0 {
		return 0, s, v
	}
	switch hi {
	case r:
		h = math.Mod((g-b)/d, 6)
	case g:
		h = (b-r)/d + 2
	default:
		h = (r-g)/d + 4
	}
	h *= 60
	if h < 0 {
		h += 360
	}
	return h, s, v
}

func artworkRGB(h, s, v float64) (r, g, b float64) {
	c := v * s
	x := c * (1 - math.Abs(math.Mod(h/60, 2)-1))
	m := v - c
	switch {
	case h < 60:
		r, g, b = c, x, 0
	case h < 120:
		r, g, b = x, c, 0
	case h < 180:
		r, g, b = 0, c, x
	case h < 240:
		r, g, b = 0, x, c
	case h < 300:
		r, g, b = x, 0, c
	default:
		r, g, b = c, 0, x
	}
	return r + m, g + m, b + m
}
