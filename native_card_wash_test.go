package main

import (
	"math"
	"testing"

	"github.com/egoist/mygo/ui"
)

func wcagLuminance(c ui.Color) float64 {
	ch := func(v uint8) float64 {
		x := float64(v) / 255
		if x <= 0.03928 {
			return x / 12.92
		}
		return math.Pow((x+0.055)/1.055, 2.4)
	}
	return 0.2126*ch(c.R) + 0.7152*ch(c.G) + 0.0722*ch(c.B)
}

// wcagContrast is a's contrast on b; a translucent a is seen over b.
func wcagContrast(a, b ui.Color) float64 {
	if a.A < 255 {
		opaque := a
		opaque.A = 255
		a = b.Mix(opaque, float32(a.A)/255)
	}
	la, lb := wcagLuminance(a), wcagLuminance(b)
	return (max(la, lb) + 0.05) / (min(la, lb) + 0.05)
}

// The mini player's card is filled with the playing cover's colour. Its
// artist line (secondary text) must keep the guidebook's 4.5:1 for every
// colour artworkTint can return: any hue, at its one brightness and its
// saturation bounds.
func TestNativeCardWashKeepsTextReadable(t *testing.T) {
	for _, dark := range []bool{true, false} {
		p := palette(dark)
		worst, at := math.Inf(1), 0.0
		for h := 0.0; h < 360; h += 2 {
			for _, s := range []float64{0.4, 0.75} {
				r, g, b := artworkRGB(h, s, 0.62)
				tint := ui.Color{R: uint8(r*255 + 0.5), G: uint8(g*255 + 0.5), B: uint8(b*255 + 0.5), A: 255}
				card := p.raised.Mix(tint, nativeCardWash(p))
				for _, text := range []ui.Color{p.secondary, p.text} {
					if c := wcagContrast(text, card); c < worst {
						worst, at = c, h
					}
				}
			}
		}
		if worst < 4.5 {
			t.Errorf("dark=%v: text on the tinted card falls to %.2f:1 (hue %.0f)", dark, worst, at)
		}
	}
}
