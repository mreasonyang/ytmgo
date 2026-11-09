package main

import (
	"image"
	"image/color"
	"math"
)

// The fictional catalogue's artwork, drawn in code so the screenshots show
// nobody's real covers. Each release has its own visual discipline, the way
// a real record shelf does: a dusk photograph, a Swiss print, a two-ink
// risograph, fluted glass, an aurora, a brutalist block. Mixes, artists and
// Liked music get art that suits their kind. Songs share their album's cover.

const siteCoverSize = 384

// siteCovers maps an artwork key to its painter.
var siteCovers = map[string]func(u, v float64) siteRGB{
	"cover-low-light-hours":  coverLowLightHours,
	"cover-paper-satellites": coverPaperSatellites,
	"cover-neon-tide":        coverNeonTide,
	"cover-glass-garden":     coverGlassGarden,
	"cover-northern-lines":   coverNorthernLines,
	"cover-quiet-engines":    coverQuietEngines,
	"cover-night-drive":      coverNightDrive,
	"cover-deep-focus":       coverDeepFocus,
	"cover-liked":            coverLiked,
	"artist-lumen-drift":     artistLumenDrift,
	"artist-velvet-static":   artistVelvetStatic,
}

// siteAlbumCovers gives each album, and so each of its songs, one cover.
var siteAlbumCovers = map[string]string{
	"Low Light Hours":  "cover-low-light-hours",
	"Paper Satellites": "cover-paper-satellites",
	"Neon Tide":        "cover-neon-tide",
	"Glass Garden":     "cover-glass-garden",
	"Northern Lines":   "cover-northern-lines",
	"Quiet Engines":    "cover-quiet-engines",
}

type siteRGB struct{ r, g, b float64 }

func siteHex(h uint32) siteRGB {
	return siteRGB{float64(h>>16&0xff) / 255, float64(h>>8&0xff) / 255, float64(h&0xff) / 255}
}

func (c siteRGB) mix(d siteRGB, t float64) siteRGB {
	t = siteClamp(t)
	return siteRGB{c.r + (d.r-c.r)*t, c.g + (d.g-c.g)*t, c.b + (d.b-c.b)*t}
}

func (c siteRGB) mul(d siteRGB) siteRGB { return siteRGB{c.r * d.r, c.g * d.g, c.b * d.b} }

func (c siteRGB) add(k float64) siteRGB { return siteRGB{c.r + k, c.g + k, c.b + k} }

func siteClamp(t float64) float64 { return max(0, min(1, t)) }

func siteSmooth(a, b, x float64) float64 {
	t := siteClamp((x - a) / (b - a))
	return t * t * (3 - 2*t)
}

// siteFill is the coverage of a shape whose signed distance is d (negative
// inside), antialiased over about one pixel.
func siteFill(d float64) float64 { return siteClamp(0.5 - d*siteCoverSize) }

// siteHash and siteNoise give repeatable texture: paper, grain, concrete.
func siteHash(x, y float64) float64 {
	h := math.Sin(x*127.1+y*311.7) * 43758.5453
	return h - math.Floor(h)
}

func siteNoise(x, y float64) float64 {
	ix, iy := math.Floor(x), math.Floor(y)
	fx, fy := x-ix, y-iy
	fx, fy = fx*fx*(3-2*fx), fy*fy*(3-2*fy)
	a, b := siteHash(ix, iy), siteHash(ix+1, iy)
	c, d := siteHash(ix, iy+1), siteHash(ix+1, iy+1)
	return a + (b-a)*fx + (c-a)*fy + (a-b-c+d)*fx*fy
}

func siteFBM(x, y float64) float64 {
	s, amp := 0.0, 0.5
	for range 4 {
		s += amp * siteNoise(x, y)
		x, y, amp = x*2.03, y*2.03, amp/2
	}
	return s
}

func siteGrain(u, v, k float64) float64 { return (siteHash(u*977, v*991) - 0.5) * k }

// siteRender paints a cover with 2×2 supersampling.
func siteRender(paint func(u, v float64) siteRGB) *image.RGBA {
	const n = siteCoverSize
	img := image.NewRGBA(image.Rect(0, 0, n, n))
	for y := range n {
		for x := range n {
			var acc siteRGB
			for _, o := range [4][2]float64{{.25, .25}, {.75, .25}, {.25, .75}, {.75, .75}} {
				c := paint((float64(x)+o[0])/n, (float64(y)+o[1])/n)
				acc = siteRGB{acc.r + c.r/4, acc.g + c.g/4, acc.b + c.b/4}
			}
			img.SetRGBA(x, y, color.RGBA{uint8(siteClamp(acc.r)*255 + .5), uint8(siteClamp(acc.g)*255 + .5), uint8(siteClamp(acc.b)*255 + .5), 255})
		}
	}
	return img
}

// Low Light Hours — Lumen Drift. A dusk photograph: the sun half set over
// still water, its reflection broken into ripples, with film grain.
func coverLowLightHours(u, v float64) siteRGB {
	const horizon = 0.6
	sunX, sunR := 0.5, 0.15
	var c siteRGB
	if v < horizon {
		t := v / horizon
		c = siteHex(0x18143a).mix(siteHex(0x5a2a5e), siteSmooth(0, .7, t)).mix(siteHex(0xf08a4b), siteSmooth(.55, 1, t))
		d := math.Hypot(u-sunX, v-horizon) - sunR
		c = c.mix(siteHex(0xffc98a).mix(siteHex(0xffeccc), 1-(horizon-v)/sunR), siteFill(d))
		c = c.mix(siteHex(0xffb070), 0.35*math.Exp(-math.Max(0, d)*9))
	} else {
		t := (v - horizon) / (1 - horizon)
		c = siteHex(0x3a1c3e).mix(siteHex(0x0c0a18), siteSmooth(0, 1, t))
		width := 0.06 + 0.3*t
		ripple := math.Sin(v*190+siteNoise(u*9, v*40)*5) > 0.15
		if ripple && math.Abs(u-sunX) < width*(0.6+0.4*siteNoise(v*30, 1)) {
			c = c.mix(siteHex(0xffb070), 0.75*(1-t)*(1-math.Abs(u-sunX)/width))
		}
	}
	return c.add(siteGrain(u, v, 0.06))
}

// Paper Satellites — Nadia Kerr. A Swiss print: flat shapes on warm paper,
// a vermilion planet, a hairline orbit and one cobalt satellite.
func coverPaperSatellites(u, v float64) siteRGB {
	c := siteHex(0xebe5d8).add((siteFBM(u*60, v*8) - 0.5) * 0.05)
	c = c.mix(siteHex(0xe5432a), siteFill(math.Hypot(u-.4, v-.42)-.25))
	// The orbit: a rotated ellipse, stroked thin.
	a := -0.38
	x, y := u-.54, v-.5
	x, y = x*math.Cos(a)-y*math.Sin(a), x*math.Sin(a)+y*math.Cos(a)
	// Divide the implicit value by its gradient for an even stroke all round.
	k := math.Hypot(x/.4, y/.13)
	grad := math.Hypot(x/(.4*.4), y/(.13*.13)) / math.Max(k, 1e-6)
	c = c.mix(siteHex(0x161616), siteFill(math.Abs(k-1)/grad-.0022))
	sx, sy := .54+.4*math.Cos(.5)*math.Cos(a)+.13*math.Sin(.5)*math.Sin(a), .5-.4*math.Cos(.5)*math.Sin(a)+.13*math.Sin(.5)*math.Cos(a)
	c = c.mix(siteHex(0x2340b8), siteFill(math.Hypot(u-sx, v-sy)-.04))
	if u > .08 && u < .92 {
		c = c.mix(siteHex(0x161616), siteFill(math.Abs(v-.87)-.002))
	}
	return c
}

// Neon Tide — Harbor Lights Collective. A two-ink risograph: fluorescent
// pink and teal halftones of a swell, slightly out of register.
func coverNeonTide(u, v float64) siteRGB {
	paper := siteHex(0xf2eee4)
	halftone := func(u, v, angle, tone float64) float64 {
		x, y := u*math.Cos(angle)-v*math.Sin(angle), u*math.Sin(angle)+v*math.Cos(angle)
		const cells = 30.0
		cx, cy := x*cells-math.Floor(x*cells)-.5, y*cells-math.Floor(y*cells)-.5
		return siteFill((math.Hypot(cx, cy) - .55*math.Sqrt(siteClamp(tone))) / cells)
	}
	swell := func(u, v float64) float64 {
		return siteSmooth(-.15, .25, v-(.42+.13*math.Sin(u*7.5+1)+.05*math.Sin(u*19)))
	}
	pink := halftone(u, v, .26, swell(u, v)*.9)
	teal := halftone(u+.008, v+.006, 1.31, (1-swell(u+.08, v-.12))*.75+.15*math.Sin(u*3))
	c := paper.mul(paper.mix(siteHex(0xff4f9a), pink*.92))
	c = c.mul(siteRGB{1, 1, 1}.mix(siteHex(0x00a6ad), teal*.88))
	return c.add(siteGrain(u, v, 0.05))
}

// Glass Garden — Velvet Static. Blurred blossoms seen through fluted glass:
// each rib flips and squeezes what's behind it and catches a highlight.
func coverGlassGarden(u, v float64) siteRGB {
	behind := func(u, v float64) siteRGB {
		c := siteHex(0x0f2a22)
		for _, b := range []struct {
			x, y, r float64
			col     uint32
		}{{.3, .35, .22, 0xff7eb6}, {.7, .3, .18, 0xb8f06a}, {.55, .72, .26, 0x7a4dff}, {.2, .8, .16, 0xffb347}} {
			c = c.mix(siteHex(b.col), math.Exp(-math.Pow(math.Hypot(u-b.x, v-b.y)/b.r, 2)*1.6))
		}
		return c
	}
	const ribs = 14.0
	cell := math.Floor(u * ribs)
	f := u*ribs - cell
	centre := (cell + .5) / ribs
	c := behind(centre-(f-.5)/ribs*1.8, v)
	c = c.add(0.18*math.Pow(siteSmooth(.75, 1, f), 3) - 0.12*siteSmooth(.25, 0, f))
	return c.add(siteGrain(u, v, 0.03))
}

// Northern Lines — Mika Sorensen. An aurora drawn as fine vertical lines over
// a dark ridge, with a few stars.
func coverNorthernLines(u, v float64) siteRGB {
	c := siteHex(0x04121a).mix(siteHex(0x0b2a33), v)
	if siteHash(math.Floor(u*190), math.Floor(v*190)) > .996 && v < .7 {
		c = c.add(.6)
	}
	h := .38 + .1*math.Sin(u*5+.6) + .04*math.Sin(u*13)
	band := math.Exp(-math.Pow((v-h)/.14, 2)) * (0.55 + 0.45*siteNoise(u*14, 3))
	line := 1 - siteSmooth(.18, .42, math.Abs(u*96-math.Floor(u*96)-.5))
	glow := siteHex(0x5cffb0).mix(siteHex(0x9a6bff), siteSmooth(h+.05, h-.18, v))
	c = c.mix(glow, band*(.25+.75*line))
	ridge := .84 - .05*math.Sin(u*4+1) - .03*siteNoise(u*12, 7)
	return c.mix(siteHex(0x02080b), siteFill(ridge-v))
}

// Quiet Engines — Tidal Bloom. Brutalist: a black block on concrete, crossed
// by one safety-orange bar.
func coverQuietEngines(u, v float64) siteRGB {
	c := siteHex(0xa9a59d).add((siteFBM(u*55, v*55)-0.5)*0.12 + (siteFBM(u*6, v*6)-0.5)*0.05 + siteGrain(u, v, 0.09))
	block := math.Max(math.Abs(u-.37)-.25, math.Abs(v-.54)-.34)
	c = c.mix(siteHex(0x121212), siteFill(block))
	bar := math.Max(math.Abs(u-.66)-.28, math.Abs(v-.66)-.028)
	return c.mix(siteHex(0xff6a13), siteFill(bar))
}

// Night Drive Mix. A road running to a glowing horizon, edge lines in the
// two colours of street light.
func coverNightDrive(u, v float64) siteRGB {
	const hy = .46
	c := siteHex(0x07060d)
	if v < hy {
		c = c.mix(siteHex(0xff3d7f), .55*math.Exp(-(hy-v)*9))
	} else {
		c = c.mix(siteHex(0x1a0f2a), .5*math.Exp(-(v-hy)*6))
		t := (v - hy) / (1 - hy)
		x := (u - .5) / math.Max(t, 1e-3)
		c = c.mix(siteHex(0xff3d7f), siteFill((math.Abs(x+.9)-.012)*t))
		c = c.mix(siteHex(0x3dd6ff), siteFill((math.Abs(x-.9)-.012)*t))
		if math.Mod(math.Log(t+.02)*6+20, 1) < .45 {
			c = c.mix(siteHex(0xf4f1ea), siteFill((math.Abs(x)-.01)*t))
		}
	}
	return c.add(siteGrain(u, v, 0.03))
}

// Deep Focus. Calm sage, a few concentric rings and a still point.
func coverDeepFocus(u, v float64) siteRGB {
	d := math.Hypot(u-.5, v-.5)
	c := siteHex(0x93ac9e).mix(siteHex(0x5b7568), d*1.6)
	ring := math.Abs(math.Mod(d, .085) - .0425)
	if d > .09 && d < .46 {
		c = c.mix(siteHex(0xeef3ee), 0.35*siteFill(.0425-ring-.0015))
	}
	return c.mix(siteHex(0xf4f1e8), siteFill(d-.05))
}

// Liked music. A soft gradient with the app's heart.
func coverLiked(u, v float64) siteRGB {
	c := siteHex(0x4a3ad6).mix(siteHex(0xb54ad8), u*.6+v*.4).mix(siteHex(0xff6c9a), siteSmooth(.6, 1.2, u+v*.5))
	return c.mix(siteRGB{1, 1, 1}, siteFill(siteHeart((u-.5)/.36, (.71-v)/.36)*.36))
}

// siteHeart is the signed distance to a heart whose point sits at the
// origin, about one unit tall, y up.
func siteHeart(x, y float64) float64 {
	x = math.Abs(x)
	if x+y > 1 {
		return math.Hypot(x-.25, y-.75) - math.Sqrt2/4
	}
	m := .5 * math.Max(x+y, 0)
	d := math.Sqrt(math.Min((x)*(x)+(y-1)*(y-1), (x-m)*(x-m)+(y-m)*(y-m)))
	if x < y {
		return -d
	}
	return d
}

// siteSmin joins two shapes with a smooth seam.
func siteSmin(a, b, k float64) float64 {
	h := siteClamp(.5 + .5*(b-a)/k)
	return b + (a-b)*h - k*h*(1-h)
}

// siteSilhouette is a head, neck and shoulders, for the artist portraits.
func siteSilhouette(u, v float64) float64 {
	head := math.Hypot((u-.5)/.88, v-.41) - .155
	neck := math.Max(math.Abs(u-.5)-.06, math.Abs(v-.62)-.08)
	shoulders := math.Hypot((u-.5)/1.75, (v-1.08)/1.05) - .4
	return siteSmin(siteSmin(head, neck, .04), shoulders, .06)
}

// Lumen Drift. Backlit against a warm sky, with a rim of light.
func artistLumenDrift(u, v float64) siteRGB {
	c := siteHex(0xffb36b).mix(siteHex(0xa8325a), siteSmooth(.1, .9, math.Hypot(u-.5, v-.35)*1.6))
	d := siteSilhouette(u, v)
	c = c.mix(siteHex(0xfff0d6), siteFill(math.Abs(d+.006)-.004)*.8)
	c = c.mix(siteHex(0x1c0f1e), siteFill(d+.008))
	return c.add(siteGrain(u, v, 0.04))
}

// Velvet Static. The same pose in violet, ghosted in two colours like a
// signal that hasn't settled, over scan lines.
func artistVelvetStatic(u, v float64) siteRGB {
	c := siteHex(0x2a1556).mix(siteHex(0x5b2fa0), v)
	c = c.add(0.05*math.Sin(v*siteCoverSize*1.5) + siteGrain(u, v, 0.1))
	c = c.mix(siteHex(0x3dd6ff), 0.7*siteFill(siteSilhouette(u-.018, v)))
	c = c.mix(siteHex(0xff4f9a), 0.7*siteFill(siteSilhouette(u+.018, v)))
	return c.mix(siteHex(0x0e0820), siteFill(siteSilhouette(u, v)))
}
