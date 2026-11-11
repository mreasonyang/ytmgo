package main

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"ytmgo/internal/catalog"
)

func requestArtworkPriority(c *artworkCache, raw string, edge int, priority artworkPriority) ui.ImageSource {
	f := artworkFrame{cache: c}
	bitmap := f.get(raw, edge, priority)
	f.submit()
	return bitmap
}

func assertArtworkAccounting(t *testing.T, c *artworkCache) {
	t.Helper()
	for name, pool := range map[string]*artworkPool{"thumbnail": &c.small, "browse": &c.browse, "player": &c.player} {
		if pool.bytes > pool.budget || len(pool.entries) > pool.limit {
			t.Fatalf("%s cache limit exceeded: bytes=%d entries=%d", name, pool.bytes, len(pool.entries))
		}
		accounted := 0
		for _, e := range pool.entries {
			if e.pixelBytes < 0 || e.bytes < e.pixelBytes {
				t.Fatalf("%s retained pixel accounting is invalid", name)
			}
			accounted += e.bytes
		}
		if accounted != pool.bytes {
			t.Fatalf("%s accounting: stored=%d sum=%d", name, pool.bytes, accounted)
		}
	}
}

// Returning to a bounded, previously visited page must keep the original HD
// bitmap. Neither a thumbnail fallback nor an asynchronous disk decode passes.
func TestArtworkWarmScrollRetainsHDBitmapWithoutPlaceholders(t *testing.T) {
	cover := image.NewRGBA(image.Rect(0, 0, 384, 384))
	col := color.RGBA{R: 210, G: 40, B: 60, A: 255}
	for y := range 384 {
		for x := range 384 {
			cover.SetRGBA(x, y, col)
		}
	}
	var body bytes.Buffer
	if err := png.Encode(&body, cover); err != nil {
		t.Fatal(err)
	}
	data := body.Bytes()
	c := artworkTestCache(t, data)
	c.disk = &artworkDisk{dir: t.TempDir()}
	var downloads atomic.Int64
	c.client.Transport = artworkTransport(func(*http.Request) (*http.Response, error) {
		downloads.Add(1)
		return artworkHTTPResponse(data, http.Header{"Cache-Control": {"public, max-age=86400"}}), nil
	})
	const count = 60
	raw := func(i int) string { return fmt.Sprintf("https://lh3.googleusercontent.com/scroll-%d=s384", i) }
	originals := map[artRequest]*ui.Bitmap{}
	for i := range count {
		requestTestArtwork(c, raw(i), 384)
		c.workers.Wait()
		key := artRequest{artworkRequestAt(raw(i), 384), 384}
		originals[key] = c.entry(key).bitmap
	}
	// Any accidental restore blocks, making a same-frame fallback observable.
	for range artworkDecoders {
		c.decoding <- struct{}{}
	}
	defer func() {
		for range artworkDecoders {
			<-c.decoding
		}
	}()
	for round := range 10 {
		for i := range count {
			key := artRequest{artworkRequestAt(raw(i), 384), 384}
			if got := requestTestArtwork(c, raw(i), 384); got != originals[key] {
				t.Fatalf("warm return replaced or blanked HD bitmap: round=%d cover=%d", round, i)
			}
		}
	}
	if len(c.small.entries) != 0 {
		t.Fatal("browsing retained standing derived thumbnails")
	}
	m, a := nativeViewFixture()
	m.Current, m.Queue, m.Page = catalog.Item{}, nil, catalog.Page{}
	for i := range count / 5 {
		shelf := catalog.Shelf{Title: fmt.Sprintf("Scroll shelf %d", i)}
		for j := range 5 {
			n := i*5 + j
			shelf.Items = append(shelf.Items, catalog.Item{Title: fmt.Sprintf("Scroll album %d", n), Kind: "album", BrowseID: fmt.Sprintf("scroll-%d", n), Artwork: raw(n)})
		}
		m.Page.Shelves = append(m.Page.Shelves, shelf)
	}
	a.ArtworkTint = c.tint
	a.artworkPeek = c.peek
	tt := ui.NewTester(func(ctx *ui.Context) {
		frame := artworkFrame{cache: c}
		a.Artwork = frame.get
		nativeView(ctx, m, a)
		if frame.submit() {
			ctx.Invalidate()
		}
	}, 1440, 940)
	assertCover := func() {
		t.Helper()
		r, ok := tt.Find(nativeResultLabel(m.Page.Shelves[0].Items[0], m, a))
		if !ok || r.H <= nativeCardArt(false) {
			t.Fatal("first Home card is not visible")
		}
		x, y := int(r.X+12+nativeCardArt(false)/2), int(r.Y+12+nativeCardArt(false)/2)
		if got := tt.Image().RGBAAt(x, y); got != col {
			t.Fatalf("Home painted a placeholder on a warm return: %v", got)
		}
	}
	assertCover()
	view, _ := tt.Find(a.Tr("shellContent"))
	for range 10 {
		tt.Scroll(view.X+view.W/2, view.Y+view.H/2, 0, 900)
		tt.Scroll(view.X+view.W/2, view.Y+view.H/2, 0, -900)
		assertCover()
	}
	if downloads.Load() != count {
		t.Fatalf("warm scrolling downloaded bodies: %d", downloads.Load())
	}
	assertArtworkAccounting(t, c)
	t.Logf("60 HD covers, 600 exact bitmap returns, 20 native scroll commands: 0 warm placeholders, %d cold bodies, 0 warm bodies", downloads.Load())
}

func TestArtworkGeneratedMixReusesPublicDiskCache(t *testing.T) {
	data := artworkTestPNG(t, 384, 384)
	dir := t.TempDir()
	downloads := 0
	const raw = "https://music.youtube.com/image/synthetic-mix?r=synthetic"
	for range 2 {
		c := artworkTestCache(t, data)
		c.disk = &artworkDisk{dir: dir}
		c.client.Transport = artworkTransport(func(*http.Request) (*http.Response, error) {
			downloads++
			return artworkHTTPResponse(data, http.Header{"Cache-Control": {"public, max-age=86400"}}), nil
		})
		requestTestArtwork(c, raw, 384)
		c.workers.Wait()
	}
	if downloads != 1 {
		t.Fatalf("public mix cover bypassed disk cache: %d bodies", downloads)
	}
}

func TestArtworkPoolsCannotEvictEachOther(t *testing.T) {
	for _, victim := range []int{128, 384, 768} {
		t.Run(fmt.Sprint(victim), func(t *testing.T) {
			c := artworkTestCache(t, nil)
			c.small.limit, c.browse.limit, c.player.limit = 4, 4, 4
			data := artworkTestPNG(t, 768, 768)
			c.client.Transport = artworkTransport(func(*http.Request) (*http.Response, error) {
				return artworkHTTPResponse(data, http.Header{"Cache-Control": {"public, max-age=3600"}}), nil
			})
			priority := func(edge int) artworkPriority {
				if edge == 768 {
					return artPlayer
				}
				return artPrimary
			}
			const raw = "https://lh3.googleusercontent.com/pool-victim=s80"
			requestArtworkPriority(c, raw, victim, priority(victim))
			c.workers.Wait()
			key := artRequest{artworkRequestAt(raw, victim), victim}
			bitmap := c.entry(key).bitmap
			for _, edge := range []int{128, 384, 768} {
				if edge == victim {
					continue
				}
				for i := range 12 {
					requestArtworkPriority(c, fmt.Sprintf("https://lh3.googleusercontent.com/churn-%d-%d=s80", edge, i), edge, priority(edge))
					c.workers.Wait()
				}
				if e := c.entry(key); e == nil || e.bitmap != bitmap {
					t.Fatalf("%d churn evicted independent %d bitmap", edge, victim)
				}
			}
			assertArtworkAccounting(t, c)
		})
	}
}

func TestArtworkRenditionSharesPlayerAndBrowseBitmap(t *testing.T) {
	c := artworkTestCache(t, artworkTestPNG(t, 384, 384))
	var bodies atomic.Int64
	upstream := c.client.Transport
	c.client.Transport = artworkTransport(func(r *http.Request) (*http.Response, error) { bodies.Add(1); return upstream.RoundTrip(r) })
	const raw = "https://lh3.googleusercontent.com/shared=s80"
	f := artworkFrame{cache: c}
	f.get(raw, 384, artPrimary)
	f.get(raw, 384, artPlayer)
	f.submit()
	c.workers.Wait()
	key := artRequest{artworkRequestAt(raw, 384), 384}
	bitmap := c.entry(key).bitmap
	for range 5 {
		f = artworkFrame{cache: c}
		if f.get(raw, 384, artPrimary) != bitmap || f.get(raw, 384, artPlayer) != bitmap {
			t.Fatal("identical rendition did not share bitmap")
		}
		f.submit()
	}
	if bodies.Load() != 1 || len(c.browse.entries)+len(c.player.entries) != 1 {
		t.Fatal("identical rendition was fetched or charged twice")
	}
	assertArtworkAccounting(t, c)
}

func TestArtworkFirstPaintReleasesPixelsAndRetainsEncodedIdentity(t *testing.T) {
	c := artworkTestCache(t, artworkTestPNG(t, 384, 384))
	const raw = "https://lh3.googleusercontent.com/release=s384"
	requestTestArtwork(c, raw, 384)
	c.workers.Wait()
	key := artRequest{artworkRequestAt(raw, 384), 384}
	e := c.entry(key)
	bitmap := e.bitmap
	before, pixels := e.bytes, e.pixelBytes
	if pixels != artworkPixelCost(384, 384) {
		t.Fatal("initial charge excludes possible renderer mipmaps")
	}
	f := artworkFrame{cache: c}
	if f.get(raw, 384, artPrimary) != bitmap || !f.submit() {
		t.Fatal("first paint did not request a release frame")
	}
	f = artworkFrame{cache: c}
	if f.get(raw, 384, artPrimary) != bitmap {
		t.Fatal("release frame changed bitmap")
	}
	f.submit()
	if e.pixelBytes != 0 || e.bytes != before-pixels || e.bytes <= 0 {
		t.Fatal("first paint did not leave only charged encoded backing")
	}
	if requestTestArtwork(c, raw, 384) != bitmap {
		t.Fatal("released pixels replaced HD bitmap")
	}
	assertArtworkAccounting(t, c)
}

// Observe the image from each completed frame at the start of the next view
// build. Tester normally settles several frames, which can hide a one-frame
// note icon when a warm page gets fresh layout keys.
func TestArtworkWarmNavigationFirstPresentedFramePaintsCover(t *testing.T) {
	im := image.NewRGBA(image.Rect(0, 0, 384, 384))
	col := color.RGBA{70, 155, 220, 255}
	for y := range 384 {
		for x := range 384 {
			im.SetRGBA(x, y, col)
		}
	}
	var b bytes.Buffer
	if png.Encode(&b, im) != nil {
		t.Fatal("fixture encoding failed")
	}
	c := artworkTestCache(t, b.Bytes())
	const raw = "https://lh3.googleusercontent.com/first-layout=s384"
	requestTestArtwork(c, raw, 384)
	c.workers.Wait()
	m, a := nativeViewFixture()
	m.Current, m.Queue, m.Page = catalog.Item{}, nil, catalog.Page{Title: "Other page"}
	item := catalog.Item{Title: "Warm navigation cover", Kind: "album", BrowseID: "warm-navigation", Artwork: raw}
	a.ArtworkTint, a.artworkPeek = c.tint, c.peek
	var tt *ui.Tester
	samples := 0
	observe := func() {
		if tt == nil {
			return
		}
		r, ok := tt.Find(nativeResultLabel(item, m, a))
		if !ok {
			return
		}
		x, y := int(r.X+12+nativeCardArt(false)/2), int(r.Y+12+nativeCardArt(false)/2)
		if got := tt.Image().RGBAAt(x, y); got != col {
			t.Fatalf("completed navigation frame painted a placeholder: %v", got)
		}
		samples++
	}
	tt = ui.NewTester(func(ctx *ui.Context) {
		observe()
		f := artworkFrame{cache: c}
		a.Artwork = f.get
		nativeView(ctx, m, a)
		if f.submit() {
			ctx.Invalidate()
		}
	}, 1440, 940)
	for round := range 10 {
		m.Route = "home"
		m.Page = catalog.Page{Title: fmt.Sprintf("Warm page %d", round), Shelves: []catalog.Shelf{{Title: "Home", Items: []catalog.Item{item}}}}
		tt.Frame()
		observe()
		m.Route = "browse:other"
		m.Page = catalog.Page{Title: "Other page"}
		tt.Frame()
	}
	if samples < 20 {
		t.Fatalf("did not observe intermediate and settled navigation frames: %d", samples)
	}
	t.Logf("%d completed raster frames sampled across 10 warm navigations; 0 placeholder pixels", samples)
}

func TestArtworkSimultaneousSizesShareNonResizableHTTPBody(t *testing.T) {
	c := artworkTestCache(t, nil)
	c.disk = &artworkDisk{dir: t.TempDir()}
	data := artworkTestPNG(t, 768, 768)
	started, release := make(chan struct{}), make(chan struct{})
	var requests atomic.Int32
	c.client.Transport = artworkTransport(func(req *http.Request) (*http.Response, error) {
		if requests.Add(1) == 1 {
			close(started)
		}
		select {
		case <-release:
		case <-req.Context().Done():
			return nil, req.Context().Err()
		}
		return artworkHTTPResponse(data, http.Header{"Cache-Control": {"public, max-age=3600"}}), nil
	})
	const raw = "https://i.ytimg.com/same-source"
	f := artworkFrame{cache: c}
	f.get(raw, 128, artPrimary)
	f.get(raw, 384, artPrimary)
	f.get(raw, 768, artPlayer)
	f.submit()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("shared body did not start")
	}
	close(release)
	c.workers.Wait()
	if requests.Load() != 1 {
		t.Fatalf("same public URL downloaded %d simultaneous bodies", requests.Load())
	}
	for _, edge := range []int{128, 384, 768} {
		e := c.entry(artRequest{raw, edge})
		if e == nil || e.bitmap == nil {
			t.Fatal("shared body did not produce every requested rendition")
		}
		if w, h := e.bitmap.Size(); w != edge || h != edge {
			t.Fatal("shared body reused another size's bitmap")
		}
	}
	assertArtworkAccounting(t, c)
}

func TestArtworkEncodedAndTransientPixelPressureStayBounded(t *testing.T) {
	im := image.NewRGBA(image.Rect(0, 0, 384, 384))
	state := uint32(0x12345678)
	for y := range 384 {
		for x := range 384 {
			state ^= state << 13
			state ^= state >> 17
			state ^= state << 5
			im.SetRGBA(x, y, color.RGBA{byte(state), byte(state >> 8), byte(state >> 16), 255})
		}
	}
	var encoded bytes.Buffer
	if png.Encode(&encoded, im) != nil {
		t.Fatal("pressure fixture encoding failed")
	}
	c := artworkTestCache(t, encoded.Bytes())
	c.browse.budget = 2 << 20
	for i := range 8 {
		raw := fmt.Sprintf("https://i.ytimg.com/encoded-pressure-%d", i)
		requestTestArtwork(c, raw, 384)
		c.workers.Wait()
		e := c.entry(artRequest{raw, 384})
		if e == nil || e.bitmap == nil || e.bytes-e.pixelBytes < len(encoded.Bytes()) {
			t.Fatal("encoded backing was uncharged or current cover was rejected")
		}
		assertArtworkAccounting(t, c)
	}
	if len(c.browse.entries) >= 8 {
		t.Fatal("encoded-byte pressure never evicted a cold bitmap")
	}
	t.Logf("%d-byte compressed fixture; %d cached entries and %d charged bytes under 2 MiB including transient pixels", encoded.Len(), len(c.browse.entries), c.browse.bytes)
}
