package main

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"ytmgo/internal/catalog"
)

// Drive the actual desktop view and cache. Hold downloads across renders so a
// transient note or a late cover cannot pass just because the final frame is right.
func TestNowPlayingCoverAcrossSkips(t *testing.T) {
	const cdn = "https://lh3.googleusercontent.com/"
	track := func(id string) catalog.Item {
		return catalog.Item{ID: id, Title: id, Artwork: cdn + id + "=s80"}
	}
	queue := []catalog.Item{track("first"), track("prepared"), track("skipped"), track("latest"), track("failed")}
	colors := map[string]color.RGBA{
		"first": {220, 50, 30, 255}, "prepared": {50, 180, 30, 255},
		"skipped": {50, 30, 180, 255}, "latest": {175, 35, 150, 255}, "preview": {120, 45, 110, 255},
	}
	bodies := map[string][]byte{}
	for id, col := range colors {
		img := image.NewRGBA(image.Rect(0, 0, 1, 1))
		img.SetRGBA(0, 0, col)
		var b bytes.Buffer
		if err := png.Encode(&b, img); err != nil {
			t.Fatal(err)
		}
		bodies[id] = b.Bytes()
	}

	cache := artworkTestCache(t, nil)
	skippedStarted := make(chan struct{}, 2)
	releaseSkipped, releaseLarge := make(chan struct{}), make(chan struct{})
	cache.client.Transport = artworkTransport(func(req *http.Request) (*http.Response, error) {
		id, size, _ := strings.Cut(req.URL.Path[1:], "=")
		switch id {
		case "skipped":
			skippedStarted <- struct{}{}
			// Simulate a response arriving even after cancellation.
			select {
			case <-releaseSkipped:
			case <-cache.ctx.Done():
			}
		case "latest":
			if size == "w128-h128" {
				id = "preview"
			} else {
				select {
				case <-releaseLarge:
				case <-cache.ctx.Done():
				}
			}
		}
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Cache-Control": {"public, max-age=3600"}}, Body: io.NopCloser(bytes.NewReader(bodies[id]))}, nil
	})
	// A list has already shown a thumbnail of the last playable track.
	requestTestArtwork(cache, queue[3].Artwork, 128)
	cache.workers.Wait()

	m, actions := nativeViewFixture()
	m.Current, m.Queue, m.Page = queue[0], queue, catalog.Page{}
	m.Panel, m.PanelTab = true, "lyrics"
	app := &desktop{art: cache, model: *m, preparedID: queue[1].ID}
	playerEdge := 0
	tt := ui.NewTester(func(c *ui.Context) {
		frame := artworkFrame{cache: cache}
		actions.Artwork = frame.get
		nativeView(c, &app.model, actions)
		playerEdge = frame.playerEdge
		app.prefetchNextCover(&frame)
		frame.submit()
	}, 1440, 940)
	cache.workers.Wait()
	tt.Frame()
	// Sample the cover just above its Cover/Video switch.
	pixel := func() color.RGBA {
		t.Helper()
		switchButton, ok := tt.Find(actions.Tr("nativeArtwork"))
		if !ok {
			t.Fatal("cover switch missing from Now Playing")
		}
		return tt.Image().RGBAAt(int(switchButton.X+switchButton.W/2), int(switchButton.Y)-32)
	}
	assertCover := func(id string) {
		t.Helper()
		if got := pixel(); got != colors[id] {
			t.Fatalf("expected %s on the Now Playing cover; got %v", id, got)
		}
	}
	skip := func(i int) {
		app.queueIndex, app.model.Current, app.preparedID = i, queue[i], ""
		tt.Frame()
	}

	assertCover("first")
	app.art.mu.Lock()
	warmed := app.art.entry(artRequest{url: artworkRequestAt(queue[1].Artwork, playerEdge), edge: playerEdge})
	ready := warmed != nil && warmed.bitmap != nil
	app.art.mu.Unlock()
	if !ready {
		t.Fatal("next album cover wasn't ready before the track changed")
	}
	skip(1)
	assertCover("prepared")
	skip(2)
	select {
	case <-skippedStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("cold cover request didn't start")
	}
	assertCover("prepared")
	skip(3)
	assertCover("preview")
	close(releaseLarge)
	close(releaseSkipped)
	cache.workers.Wait()
	tt.Frame()
	assertCover("latest")

	skip(4)
	cache.workers.Wait()
	tt.Frame()
	missing := pixel()
	for _, col := range colors {
		if missing == col {
			t.Fatal("failed cover left another track's artwork on screen")
		}
	}
	skip(0)
	assertCover("first")
	app.model.Current.Artwork = ""
	tt.Frame()
	if pixel() != missing {
		t.Fatal("track without artwork did not return to the placeholder")
	}
}

func TestNativeArtworkRevocationDropsCurrentAndExitingSurfaces(t *testing.T) {
	const raw = "https://lh3.googleusercontent.com/same-url=s128"
	red := color.RGBA{247, 17, 54, 255}
	local := ui.NewBitmap(tintFill(64, 64, func(int, int) color.RGBA { return red }))
	p := palette(true)
	type surface func(*ui.Context, *UIModel, ViewActions)
	for _, test := range []struct {
		name string
		draw surface
	}{
		{"crossfade", func(c *ui.Context, m *UIModel, a ViewActions) {
			nativePlayerArtwork(c, m.Current.Artwork, 128, 8, a, p)
		}},
		{"immersive blur", func(c *ui.Context, m *UIModel, a ViewActions) {
			m.Immersive = true
			nativeBackdrop(c, m, a, p)
		}},
		{"page wash", func(c *ui.Context, m *UIModel, a ViewActions) {
			var scroll *ui.ScrollState
			nativePageWash(c, m, a, p, &scroll)
		}},
		{"player wash", func(c *ui.Context, m *UIModel, a ViewActions) { nativePlayerWash(c, m, a, p) }},
		{"now playing wash", func(c *ui.Context, m *UIModel, a ViewActions) {
			// Isolate the panel wash from the independently tested cover.
			a.Artwork = nil
			nativeNowPlaying(c, m, a, p, 370, false, false).Fill()
		}},
		{"mini player wash", func(c *ui.Context, m *UIModel, a ViewActions) {
			a.Artwork = nil
			nativeMiniPlayer(c, m, a, p)
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			m, a := nativeViewFixture()
			m.instantMotion, m.Playing, m.VideoAvailable = false, false, false
			m.Route = "browse:local-artwork"
			m.Page = catalog.Page{Artwork: raw}
			m.Current.Artwork = raw
			m.Queue = nil
			available, exiting := true, false
			playing := local
			a.Artwork = func(src string, _ int, _ artworkPriority) ui.ImageSource {
				if available && src == raw {
					return playing
				}
				return nil
			}
			a.ArtworkTint = func(src string) (ui.Color, bool) { return ui.RGB(red.R, red.G, red.B), available && src == raw }
			var tt *ui.Tester
			redPixels := func() int {
				frame := tt.Image()
				width := frame.Bounds().Dx()
				count := 0
				for y := 0; y < frame.Bounds().Dy(); y++ {
					for x := 0; x < width; x++ {
						switch test.name {
						case "now playing wash", "mini player wash":
							// Sample the wash at the edges, outside text and controls.
							// Windows subpixel glyphs can contain red on a neutral wash.
							if x >= 12 && x < width-12 {
								continue
							}
						}
						c := frame.RGBAAt(x, y)
						if int(c.R) > int(c.G)+8 && int(c.R) > int(c.B)+6 {
							count++
						}
					}
				}
				return count
			}
			paintedRevision := uint64(0)
			tt = ui.NewTester(func(c *ui.Context) {
				if paintedRevision != 0 && redPixels() != 0 {
					t.Fatal("revoked artwork remained in an intermediate presentation")
				}
				frame := a
				frame.artworkState = frame.artworkWindow(c)
				c.Root().DrawOver(func(*ui.Painter, ui.Rect) { paintedRevision = frame.artworkRevision })
				ui.Column(c).Fill().Background(p.surface).Children(func() {
					// Keep one live surface and leave another as a parent exit
					// copy, whose callbacks predate the source revocation.
					ui.Column(c.Key("live")).Height(260).Shrink(0).Clip().Children(func() { test.draw(c, m, frame) })
					if !exiting {
						ui.Column(c.Key("leaving")).Height(260).Shrink(0).Clip().Transition(nativeFadeLong).Children(func() { test.draw(c, m, frame) })
					}
				})
			}, 370, 520)
			full := redPixels()
			if full < 100 {
				t.Fatalf("fixture never showed local artwork: %d pixels", full)
			}
			exiting = true
			tt.Frame()
			if test.name == "crossfade" {
				// A higher rendition of the same local source arrives while
				// the old parent is still exiting. Both can still be drawn.
				playing = ui.NewBitmap(tintFill(128, 128, func(int, int) color.RGBA { return red }))
				tt.Frame()
			}
			tt.Advance(tokenMotionFast / 3)
			if got := redPixels(); got <= full/2 {
				t.Fatalf("exiting parent's old artwork was not visible before revocation: %d of %d pixels", got, full)
			}
			available = false
			a.artworkRevision++
			tt.Frame()
			if got := redPixels(); got != 0 || paintedRevision != a.artworkRevision {
				t.Fatalf("local artwork remained on the revocation frame: %d pixels", got)
			}
		})
	}
}

func TestNativeArtworkRevocationKeepsScrollAndFocus(t *testing.T) {
	m, a := nativeViewFixture()
	m.Playing = false
	shelf := m.Page.Items
	m.Route = "browse:art-test"
	m.Page = catalog.Page{Artwork: m.Current.Artwork}
	for range 12 {
		m.Page.Items = append(m.Page.Items, shelf...)
	}
	tt := ui.NewTester(func(c *ui.Context) { nativeView(c, m, a) }, 1440, 900)
	scroll := rect(t, tt, a.Tr("shellContent"))
	tt.Scroll(scroll.X+scroll.W/2, scroll.Y+scroll.H/2, 0, 320)
	if m.pageScroll == nil || m.pageScroll.Y <= 0 {
		t.Fatal("fixture did not scroll")
	}
	before := m.pageScroll.Y
	volume := a.Tr("shellVolume")
	if err := tt.Click(volume); err != nil || !tt.Focused(volume) {
		t.Fatalf("fixture did not focus the player's volume control: %v", err)
	}
	value := m.Volume
	a.artworkRevision++
	a.Artwork = nil
	a.ArtworkTint = nil
	tt.Frame()
	if m.pageScroll.Y != before || !tt.Focused(volume) || m.Volume != value {
		t.Fatalf("revocation disturbed scroll or volume: y=%g want %g focused=%v volume=%g want %g", m.pageScroll.Y, before, tt.Focused(volume), m.Volume, value)
	}
}

func TestNativeArtworkFirstFrameAfterDownloadRemoval(t *testing.T) {
	local, remote := color.RGBA{225, 33, 50, 255}, color.RGBA{50, 110, 235, 255}
	library, _ := artworkSourceLibrary(t, map[string][]byte{artworkSourceID: artworkSourcePNG(t, 32, local)})
	completeArtworkSource(t, library, artworkSourceID, artworkSourceURL)
	cache := artworkTestCache(t, nil)
	cache.downloadCovers = library
	body := artworkSourcePNG(t, 64, remote)
	allowRemote := make(chan struct{})
	cache.client.Transport = artworkTransport(func(r *http.Request) (*http.Response, error) {
		select {
		case <-allowRemote:
			return artworkHTTPResponse(body, http.Header{"Cache-Control": {"public, max-age=3600"}}), nil
		case <-r.Context().Done():
			return nil, r.Context().Err()
		}
	})
	tt := ui.NewTester(func(c *ui.Context) {
		frame := cache.beginFrame()
		a := ViewActions{Artwork: frame.get, artworkRevision: frame.covers.Revocation()}
		nativePlayerArtwork(c, artworkSourceURL, 120, 12, a, palette(true))
		if frame.submit() {
			c.Invalidate()
		}
	}, 120, 120)
	cache.workers.Wait()
	tt.Advance(tokenMotionBase + time.Second)
	tt.Frame()
	tt.Advance(tokenMotionBase)
	pixel := func() color.RGBA { return tt.Image().RGBAAt(60, 60) }
	if got := pixel(); got != local {
		t.Fatalf("fixture did not draw the downloaded cover: %v", got)
	}
	if err := library.Remove(artworkSourceID); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if got := pixel(); got == local || got == remote || tt.FramePending() {
		t.Fatalf("removed source survived or started a player transition before an HTTP response: %v", got)
	}
	close(allowRemote)
	cache.workers.Wait()
	tt.Frame()
	tt.Advance(tokenMotionBase + time.Second)
	if got := pixel(); got != remote {
		t.Fatalf("same URL did not draw its HTTP cover after removal: %v", got)
	}
}

func TestNativePlayerArtworkStillCrossfadesWhenTrackChanges(t *testing.T) {
	from, to := color.RGBA{215, 33, 60, 255}, color.RGBA{35, 73, 205, 255}
	images := map[string]ui.ImageSource{
		"first": ui.NewBitmap(tintFill(64, 64, func(int, int) color.RGBA { return from })),
		"next":  ui.NewBitmap(tintFill(64, 64, func(int, int) color.RGBA { return to })),
	}
	a := ViewActions{Artwork: func(src string, _ int, _ artworkPriority) ui.ImageSource { return images[src] }}
	src := "first"
	tt := ui.NewTester(func(c *ui.Context) {
		nativePlayerArtwork(c, src, 100, 8, a, palette(true))
	}, 100, 100)
	pixel := func() color.RGBA { return tt.Image().RGBAAt(50, 50) }
	if got := pixel(); got != from {
		t.Fatalf("first cover = %v, want %v", got, from)
	}
	src = "next"
	tt.Frame()
	if got := pixel(); got != from {
		t.Fatalf("first transition frame discarded the outgoing cover: %v", got)
	}
	tt.Advance(tokenMotionBase / 2)
	if got := pixel(); got.R <= to.R || got.R >= from.R || got.B <= from.B || got.B >= to.B {
		t.Fatalf("halfway through the crossfade, got %v", got)
	}
	tt.Advance(tokenMotionBase)
	if got := pixel(); got != to {
		t.Fatalf("finished cover = %v, want %v", got, to)
	}
}
