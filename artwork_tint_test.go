package main

import (
	"image"
	"image/color"
	"testing"
)

func tintFill(w, h int, paint func(x, y int) color.RGBA) *image.RGBA {
	im := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			im.SetRGBA(x, y, paint(x, y))
		}
	}
	return im
}

func TestArtworkTintFollowsTheCoversColour(t *testing.T) {
	red, ok := artworkTint(tintFill(64, 64, func(int, int) color.RGBA { return color.RGBA{220, 40, 30, 255} }))
	if !ok || red.R <= red.G || red.R <= red.B {
		t.Fatalf("a red cover gave %+v, %v", red, ok)
	}
	blue, ok := artworkTint(tintFill(64, 64, func(int, int) color.RGBA { return color.RGBA{20, 60, 200, 255} }))
	if !ok || blue.B <= blue.R || blue.B <= blue.G {
		t.Fatalf("a blue cover gave %+v, %v", blue, ok)
	}
}

func TestArtworkTintPrefersVividOverDark(t *testing.T) {
	// A night sky with a sunset band: the band is the cover's colour.
	dusk := tintFill(64, 64, func(_, y int) color.RGBA {
		if y > 36 && y < 46 {
			return color.RGBA{240, 140, 70, 255}
		}
		return color.RGBA{30, 20, 60, 255}
	})
	tint, ok := artworkTint(dusk)
	if !ok || tint.R <= tint.B {
		t.Fatalf("the dusk cover's sunset should lead, got %+v", tint)
	}
}

func TestArtworkTintIgnoresCoversWithoutColour(t *testing.T) {
	grey := tintFill(64, 64, func(x, _ int) color.RGBA { v := uint8(x * 4); return color.RGBA{v, v, v, 255} })
	if _, ok := artworkTint(grey); ok {
		t.Fatal("a grey cover lent a colour")
	}
	if _, ok := artworkTint(image.NewRGBA(image.Rect(0, 0, 0, 0))); ok {
		t.Fatal("an empty image lent a colour")
	}
}

func TestArtworkTintIsReadableBehindText(t *testing.T) {
	// Whatever the cover, the tint has one brightness, so the same wash
	// strength keeps text legible.
	for _, c := range []color.RGBA{{255, 250, 120, 255}, {40, 10, 90, 255}, {0, 200, 120, 255}} {
		tint, ok := artworkTint(tintFill(32, 32, func(int, int) color.RGBA { return c }))
		if !ok {
			t.Fatalf("%v lent no colour", c)
		}
		if hi := max(tint.R, tint.G, tint.B); hi < 150 || hi > 165 {
			t.Errorf("%v: tint %+v is not at the normalised brightness", c, tint)
		}
	}
}
