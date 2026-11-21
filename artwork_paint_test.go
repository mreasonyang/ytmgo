package main

import (
	"fmt"
	"image/color"
	"reflect"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func TestArtworkColdCoverAfterLayoutChange(t *testing.T) {
	for _, change := range []string{"resize", "reflow"} {
		t.Run(change, func(t *testing.T) {
			var gets int
			var builds int
			building := false
			ink := color.RGBA{R: 201, B: 171, A: 255}
			art := ui.NewBitmap(tintFill(32, 32, func(int, int) color.RGBA { return ink }))
			a := ViewActions{Artwork: func(string, int, artworkPriority) ui.ImageSource {
				if !building {
					t.Fatal("looked up cold artwork outside a build")
				}
				gets++
				return art
			}}
			height := float32(240)
			tt := ui.NewTester(func(c *ui.Context) {
				builds++
				building = true
				defer func() { building = false }()
				view := ui.Column(c).Fill().Clip()
				view.Children(func() {
					ui.Box(c).Height(height).Shrink(0)
					nativeCover(c, "album", 64, 12, a, palette(true), artPrimary).Label("album")
				})
			}, 200, 140)
			if gets != 0 {
				t.Fatal("requested a cover outside the window")
			}
			before := builds
			if change == "resize" {
				tt.SetSize(200, 350)
			} else {
				height = 20
				tt.Frame()
			}
			r, ok := tt.Find("album")
			tt.Advance(time.Second)
			if !ok || gets != 1 || builds-before != 2 || tt.Image().RGBAAt(int(r.X+32), int(r.Y+32)) != ink || tt.FramePending() {
				t.Fatalf("newly visible cover did not settle: area=%v builds=%d requests=%d pending=%v", r, builds-before, gets, tt.FramePending())
			}
		})
	}
}

func TestArtworkWarmScrollPaintsCorrectCardOnEveryBuild(t *testing.T) {
	const side, columns = 56, 6
	names := make([]string, 30)
	colors := make(map[string]color.RGBA)
	images := make(map[string]ui.ImageSource)
	for i := range names {
		name := fmt.Sprint("album-", i)
		ink := color.RGBA{R: uint8(80 + i*3), G: uint8(220 - i*5), B: uint8(60 + i*6), A: 255}
		names[i], colors[name] = name, ink
		images[name] = ui.NewBitmap(tintFill(32, 32, func(int, int) color.RGBA { return ink }))
	}
	peeks := 0
	a := ViewActions{
		Artwork:     func(src string, _ int, _ artworkPriority) ui.ImageSource { return images[src] },
		artworkPeek: func(src string, _ int) ui.ImageSource { peeks++; return images[src] },
	}
	var tt *ui.Tester
	samples, frames := 0, 0
	check := func() {
		// Tester settles all requested frames within one input call. At
		// each subsequent build, inspect the image actually presented before it.
		if tt == nil {
			return
		}
		frame := tt.Image()
		for _, name := range names {
			r, ok := tt.Find(name)
			if !ok || r.H != side || r.W != side {
				continue
			}
			x, y := int(r.X), int(r.Y)
			for dy := range 24 {
				for dx := range 24 {
					if got := frame.RGBAAt(x+16+dx, y+16+dy); got != colors[name] {
						t.Fatalf("frame %d %s interior=%v want %v", frames, name, got, colors[name])
					}
				}
			}
			if frame.RGBAAt(x+1, y+1) == colors[name] {
				t.Fatalf("frame %d %s painted outside its rounded corner", frames, name)
			}
			samples++
		}
		frames++
	}
	padding := float32(400)
	tt = ui.NewTester(func(c *ui.Context) {
		check()
		view := ui.Scroll(c).Fill()
		actions := nativeArtworkViewport(c, a, view)
		view.Children(func() {
			ui.Box(c).Height(padding).Shrink(0)
			for i := 0; i < len(names); i += columns {
				row := names[i : i+columns]
				shelf := ui.ScrollHorizontal(c.Key(i)).Height(side).Shrink(0)
				clipped := nativeShelfViewport(c, actions, shelf)
				shelf.Children(func() {
					for _, name := range row {
						nativeCover(c.Key(name), name, side, 16, clipped, palette(true), artPrimary).Label(name)
					}
				})
			}
		})
	}, 160, 230)
	padding = 0
	tt.Frame()
	check()
	if peeks == 0 {
		t.Fatal("reflow never painted a cached cover from its final geometry")
	}
	for _, delta := range []ui.Point{{X: 168}, {Y: 112}, {X: 112}, {Y: -112}, {X: -168}, {Y: 280}, {Y: -280}} {
		tt.Scroll(32, 30, delta.X, delta.Y)
		check()
	}
	peeks = 0
	tt.Frame()
	if peeks != 0 || samples < 30 || tt.FramePending() {
		t.Fatalf("settled scroll: frame samples=%d peeks=%d pending=%v", samples, peeks, tt.FramePending())
	}
}

func TestArtworkOffscreenEffectAndExpiredGhostDoNotActivateCover(t *testing.T) {
	var builds, requests, peeks, layerPaints int
	a := ViewActions{
		Artwork:     func(string, int, artworkPriority) ui.ImageSource { requests++; return nil },
		artworkPeek: func(string, int) ui.ImageSource { peeks++; return nil },
	}
	show := true
	clipHeight := float32(80)
	tt := ui.NewTester(func(c *ui.Context) {
		builds++
		ui.Column(c).Fill().Children(func() {
			ui.Box(c).Width(160).Height(clipHeight).Shrink(0).Clip().Children(func() {
				if show {
					layer := ui.Column(c.Key("card-layer")).Size(160, 180).Shrink(0).Effect(backdropLayer{}).Transition(nativeFadeLong)
					layer.Children(func() {
						ui.Box(c).Height(100).Shrink(0)
						nativeCover(c, "behind clip", 56, 8, a, palette(true), artPrimary).DrawOver(func(*ui.Painter, ui.Rect) { layerPaints++ })
					})
				}
			})
		})
	}, 200, 200)
	tt.Frame()
	if builds != 2 || layerPaints == 0 || requests != 0 || peeks != 0 || tt.FramePending() {
		t.Fatalf("offscreen effect: builds=%d layer paints=%d requests=%d peeks=%d pending=%v", builds, layerPaints, requests, peeks, tt.FramePending())
	}
	clipHeight = 180
	tt.Frame()
	if builds != 4 || requests != 1 || peeks == 0 || tt.FramePending() {
		t.Fatalf("revealed cold effect: builds=%d requests=%d peeks=%d pending=%v", builds, requests, peeks, tt.FramePending())
	}
	beforePaints, beforeRequests, beforePeeks := layerPaints, requests, peeks
	show = false
	tt.Frame()
	if layerPaints <= beforePaints || requests != beforeRequests || peeks != beforePeeks {
		t.Fatalf("exit ghost: painted=%v requests=%d peeks=%d", layerPaints > beforePaints, requests-beforeRequests, peeks-beforePeeks)
	}
	tt.Advance(tokenMotionMorph + time.Second)
	if tt.FramePending() || requests != beforeRequests || peeks != beforePeeks {
		t.Fatal("departed cover resumed requesting artwork")
	}
}

func TestArtworkPaintReleasesRestoredPixelsWhenPageLeaves(t *testing.T) {
	const raw = "https://lh3.googleusercontent.com/paint-release=s128"
	const original = 384
	data := artworkTestPNG(t, original, original)
	bitmap, err := ui.DecodeBitmap(data)
	if err != nil {
		t.Fatal(err)
	}
	bitmap.ReleasePixels()
	retained := func() int { return artworkTestPixelBytes(bitmap) }
	cache := artworkTestCache(t, nil)
	key := artRequest{url: artworkRequestAt(raw, original), edge: original}
	fresh := time.Now().Add(time.Hour)
	entry := &artEntry{bitmap: bitmap, bytes: len(data), freshUntil: fresh}
	cache.browse.entries[key] = entry
	cache.browse.bytes = len(data)
	height, leave := float32(240), false
	restored := 0
	tt := ui.NewTester(func(c *ui.Context) {
		frame := cache.beginFrame()
		if leave {
			restored = max(restored, retained())
			ui.Box(c).Fill()
		} else {
			a := ViewActions{Artwork: frame.get, artworkPeek: func(src string, edge int) ui.ImageSource {
				img := frame.peek(src, edge)
				if edge != 128 || img != bitmap || len(frame.wanted) != 0 {
					t.Fatal("peek did not reuse the original larger bitmap without adding demand")
				}
				leave = true // Navigate away immediately after this presentation.
				return img
			}}
			view := ui.Scroll(c).Fill()
			a = nativeArtworkViewport(c, a, view)
			view.Children(func() {
				ui.Box(c).Height(height).Shrink(0)
				nativeCover(c, raw, 128, 8, a, palette(true), artPrimary)
			})
		}
		if frame.submit() {
			c.Invalidate()
		}
	}, 200, 160)
	if leave || retained() != 0 {
		t.Fatal("layout restored offscreen pixels")
	}
	height = 16
	tt.Frame()
	if restored != (original/2)*(original/2)*4 || retained() != 0 || cache.visible[key] || cache.entry(key) != entry || entry.bytes != len(data) {
		t.Fatalf("after leaving peeked artwork: restored=%d remaining=%d visible=%v", restored, retained(), cache.visible[key])
	}
	if entry.used != 0 || entry.freshUntil != fresh || cache.clock != 0 {
		t.Fatal("peek changed recency or freshness")
	}
}

// MyGo v0.8 exposes ReleasePixels but no footprint accessor. Include
// mip levels restored by the software renderer as well as the base image.
func artworkTestPixelBytes(bitmap *ui.Bitmap) int {
	value := reflect.ValueOf(bitmap).Elem()
	size := value.FieldByName("img").Elem().FieldByName("Pix").Len()
	levels := value.FieldByName("levels")
	for i := range levels.Len() {
		size += levels.Index(i).Elem().FieldByName("Pix").Len()
	}
	return size
}

func TestArtworkTransitionReleasesReplayedPixels(t *testing.T) {
	const src = "https://lh3.googleusercontent.com/replay=s128"
	data := artworkTestPNG(t, 384, 384)
	bitmap, err := ui.DecodeBitmap(data)
	if err != nil {
		t.Fatal(err)
	}
	bitmap.ReleasePixels()
	cache := artworkTestCache(t, nil)
	key := artRequest{url: artworkRequestAt(src, 384), edge: 384}
	entry := &artEntry{bitmap: bitmap, bytes: len(data), freshUntil: time.Now().Add(time.Hour)}
	cache.browse.entries[key] = entry
	cache.browse.bytes = len(data)
	open, replays, peak := true, 0, 0
	tt := ui.NewTester(func(c *ui.Context) {
		if !open && replays > 0 {
			peak = max(peak, artworkTestPixelBytes(bitmap))
		}
		frame := cache.beginFrame()
		ui.Box(c).Fill().Children(func() {
			if !open {
				return
			}
			a := ViewActions{Artwork: frame.get, artworkPeek: frame.peek, artworkReplayed: func(raw string, shown ui.ImageSource) {
				replays++
				frame.replayed(raw, shown)
			}}
			ui.Box(c.Key("album")).Size(128, 128).Transition(nativeFadeLong).Children(func() {
				nativeCover(c, src, 128, 8, a, palette(true), artPrimary)
			})
		})
		if frame.submit() {
			c.Invalidate()
		}
	}, 180, 180)
	tt.Advance(time.Second)
	open = false
	tt.Frame()
	tt.Advance(tokenMotionSlow / 4)
	if replays == 0 || peak == 0 {
		t.Fatalf("outgoing cover: replays=%d resident=%d", replays, peak)
	}
	tt.Advance(time.Second)
	tt.Frame()
	if artworkTestPixelBytes(bitmap) != 0 || cache.entry(key) != entry || entry.bytes != len(data) {
		t.Fatal("departed transition retained unaccounted CPU pixels")
	}
}
