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
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewReader(bodies[id]))}, nil
	})
	// A list has already shown a thumbnail of the last playable track.
	requestTestArtwork(cache, queue[3].Artwork, 128)
	cache.workers.Wait()

	m, actions := nativeViewFixture()
	m.Current, m.Queue, m.Page = queue[0], queue, catalog.Page{}
	m.Panel, m.PanelTab = true, "lyrics"
	app := &desktop{art: cache, model: *m, preparedID: queue[1].ID}
	tt := ui.NewTester(func(c *ui.Context) {
		frame := artworkFrame{cache: cache}
		actions.Artwork = frame.get
		nativeView(c, &app.model, actions)
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
	warmed := app.art.entries[artRequest{artworkRequestAt(queue[1].Artwork, artworkEdge), artworkEdge}]
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
