// Command icon generates ytmgo's original, deterministic app and tray artwork.
package main

import (
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
)

func main() {
	_ = os.MkdirAll("resources", 0755)
	app := image.NewNRGBA(image.Rect(0, 0, 1024, 1024))
	for y := 0; y < 1024; y++ {
		for x := 0; x < 1024; x++ {
			fx, fy := float64(x)+.5, float64(y)+.5
			// Rounded, charcoal tile with a coral record and a white play glyph.
			qx, qy := math.Abs(fx-512)-284, math.Abs(fy-512)-284
			d := math.Hypot(math.Max(qx, 0), math.Max(qy, 0)) + math.Min(math.Max(qx, qy), 0) - 180
			if d > 0 {
				continue
			}
			c := color.NRGBA{R: 24, G: 25, B: 29, A: uint8(math.Min(1, -d) * 255)}
			r := math.Hypot(fx-512, fy-512)
			if r < 310 {
				c = color.NRGBA{R: 255, G: uint8(78 + fy/30), B: 94, A: 255}
			}
			if r < 241 {
				c = color.NRGBA{R: 24, G: 25, B: 29, A: 255}
			}
			if fx >= 442 && fx <= 637 && math.Abs(fy-512) < (637-fx)*.63 {
				c = color.NRGBA{R: 250, G: 247, B: 244, A: 255}
			}
			app.SetNRGBA(x, y, c)
		}
	}
	write("resources/icon.png", app)
	tray := image.NewNRGBA(image.Rect(0, 0, 32, 32))
	for y := 0; y < 32; y++ {
		for x := 0; x < 32; x++ {
			r := math.Hypot(float64(x)-15.5, float64(y)-15.5)
			if (r > 12 && r < 15) || (x >= 12 && x <= 23 && math.Abs(float64(y)-15.5) < float64(23-x)*.65) {
				tray.SetNRGBA(x, y, color.NRGBA{A: 255})
			}
		}
	}
	write("resources/tray.png", tray)
}
func write(path string, img image.Image) {
	f, err := os.Create(path)
	if err != nil {
		panic(err)
	}
	defer f.Close()
	if err = png.Encode(f, img); err != nil {
		panic(err)
	}
}
