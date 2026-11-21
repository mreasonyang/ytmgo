package main

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/egoist/mygo/ui"
)

func artworkPreviewFixture(t *testing.T, edge, padded int, ink color.RGBA) []byte {
	t.Helper()
	var b bytes.Buffer
	im := tintFill(edge, edge, func(int, int) color.RGBA { return ink })
	if err := png.Encode(&b, im); err != nil {
		t.Fatal(err)
	}
	data := b.Bytes()
	// PNG ignores bytes after IEND. Simulate the retained encoded weight
	// of a photographic response while keeping fixture decoding inexpensive.
	if len(data) < padded {
		data = append(data, make([]byte, padded-len(data))...)
	}
	return data
}

func artworkPreviewEvict(t *testing.T, c *artworkCache, key artRequest) {
	t.Helper()
	empty := c.beginFrame()
	empty.submit()
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.pool(key).evictOldest(c.visible) || c.entry(key) != nil {
		t.Fatalf("failed to evict formal rendition %v", key)
	}
	assertArtworkAccounting(t, c)
}

func TestArtworkPreviewPaintsFirstFrameUnderHomeBudget(t *testing.T) {
	c := artworkTestCache(t, nil)
	c.disk = &artworkDisk{dir: t.TempDir()}
	const albums, songs = 140, 108
	ink := color.RGBA{225, 65, 23, 255}
	first := artworkPreviewFixture(t, 384, 576<<10, ink)
	album := artworkPreviewFixture(t, 384, 576<<10, color.RGBA{40, 170, 185, 255})
	song := artworkPreviewFixture(t, 128, 48<<10, color.RGBA{90, 132, 70, 255})
	raw := func(i int) string { return fmt.Sprintf("https://i.ytimg.com/home-preview-%d", i) }
	var requests atomic.Int32
	c.client.Transport = artworkTransport(func(r *http.Request) (*http.Response, error) {
		requests.Add(1)
		data := album
		switch {
		case r.URL.String() == raw(0):
			data = first
		case strings.Contains(r.URL.Path, "song-"):
			data = song
		}
		return artworkHTTPResponse(data, http.Header{"Cache-Control": {"public, max-age=3600"}}), nil
	})
	key := func(i int) artRequest {
		if i >= albums {
			return artRequest{url: fmt.Sprintf("https://i.ytimg.com/home-song-%d", i), edge: 128}
		}
		return artRequest{url: raw(i), edge: 384}
	}
	for i := range albums + songs {
		k := key(i)
		if sourceArtworkFrame(c, k.url, k.edge, artPrimary) != nil {
			t.Fatalf("first visit already had artwork for %s", k.url)
		}
		c.workers.Wait()
		requireArtworkSourceWidth(t, sourceArtworkFrame(c, k.url, k.edge, artPrimary), k.edge)
	}
	if e := c.entry(key(0)); e != nil || c.browse.budget != 48<<20 {
		t.Fatal("production budget did not evict the first formal cover")
	}
	if got := len(c.browsePreviews.entries) + len(c.smallPreviews.entries); got != albums+songs {
		t.Fatalf("retained %d previews for %d displayed sources", got, albums+songs)
	}
	for i := range 25 {
		if c.entry(key(i)) != nil {
			t.Fatalf("expected cover %d to be evicted by encoded pressure", i)
		}
		requireArtworkSourceWidth(t, c.peek(raw(i), 384), artworkPreviewEdge)
	}
	assertArtworkAccounting(t, c)

	// Block restoration so the first committed presentation must use the
	// preview. Inspect the pixels before Tester settles subsequent builds.
	c.mu.Lock()
	c.decoding = artworkDecoders
	c.mu.Unlock()
	unblock := sync.OnceFunc(func() {
		for range artworkDecoders {
			c.releaseDecoder()
		}
	})
	defer unblock()
	open, presented := false, false
	var tt *ui.Tester
	tt = ui.NewTester(func(ctx *ui.Context) {
		if tt != nil && open && !presented {
			if rect, ok := tt.Find("returning cover"); ok && rect.Y < 100 {
				presented = true
				if got := tt.Image().RGBAAt(int(rect.X+40), int(rect.Y+40)); got != ink {
					t.Fatalf("first committed return drew %v, want %v", got, ink)
				}
			}
		}
		f := c.beginFrame()
		a := ViewActions{Artwork: f.get, artworkPeek: f.peek, artworkScale: 4}
		ui.Column(ctx).Fill().Clip().Children(func() {
			height := float32(220)
			if open {
				height = 8
			}
			ui.Box(ctx).Height(height).Shrink(0)
			nativeCover(ctx, raw(0), 80, 8, a, palette(true), artPrimary).Label("returning cover")
		})
		if f.submit() {
			ctx.Invalidate()
		}
	}, 180, 140)
	open = true
	tt.Frame()
	if !presented {
		t.Fatal("did not observe the first committed return")
	}
	c.mu.Lock()
	entry := c.entry(key(0))
	pending := entry != nil && entry.cancel != nil
	c.mu.Unlock()
	if !pending {
		t.Fatal("preview did not trigger formal restoration")
	}
	requireArtworkSourceWidth(t, c.peek(raw(0), 384), artworkPreviewEdge)
	unblock()
	c.workers.Wait()
	requireArtworkSourceWidth(t, sourceArtworkFrame(c, raw(0), 384, artPrimary), 384)
	if got := requests.Load(); got != albums+songs {
		t.Fatalf("return triggered upstream: got %d requests after %d cold covers", got, albums+songs)
	}
	assertArtworkAccounting(t, c)
}

func TestArtworkPreviewRestoresSongAndPreservesLargerFormalReuse(t *testing.T) {
	const raw = "https://lh3.googleusercontent.com/preview-sized=s88-k?variant=a"
	c := artworkTestCache(t, nil)
	small := artworkTestPNG(t, 128, 128)
	large := artworkTestPNG(t, 384, 384)
	var smallRequests, largeRequests atomic.Int32
	c.client.Transport = artworkTransport(func(r *http.Request) (*http.Response, error) {
		data := large
		if strings.Contains(r.URL.Path, "=w128-h128-k") {
			smallRequests.Add(1)
			data = small
		} else {
			largeRequests.Add(1)
		}
		return artworkHTTPResponse(data, http.Header{"Cache-Control": {"public, max-age=3600"}}), nil
	})
	sourceArtworkFrame(c, raw, 384, artPrimary)
	c.workers.Wait()
	largeBitmap := sourceArtworkFrame(c, raw, 384, artPrimary)
	for range 3 {
		if got := sourceArtworkFrame(c, raw, 128, artPrimary); got != largeBitmap {
			t.Fatal("smaller browse target skipped the adequate formal bitmap")
		}
	}
	c.workers.Wait()
	if smallRequests.Load() != 0 || largeRequests.Load() != 1 {
		t.Fatal("adequate formal bitmap triggered a smaller upstream request")
	}

	f := c.beginFrame()
	artworkPreviewEvict(t, c, f.key(raw, 384, artPrimary))
	requireArtworkSourceWidth(t, sourceArtworkFrame(c, raw, 128, artPrimary), artworkPreviewEdge)
	c.workers.Wait()
	requireArtworkSourceWidth(t, sourceArtworkFrame(c, raw, 128, artPrimary), 128)
	artworkPreviewEvict(t, c, f.key(raw, 128, artPrimary))
	requireArtworkSourceWidth(t, c.peek(raw, 128), artworkPreviewEdge)
	requireArtworkSourceWidth(t, sourceArtworkFrame(c, raw, 128, artPrimary), artworkPreviewEdge)
	c.workers.Wait()
	requireArtworkSourceWidth(t, sourceArtworkFrame(c, raw, 128, artPrimary), 128)
	if smallRequests.Load() != 2 || largeRequests.Load() != 1 {
		t.Fatal("an evicted preview at the same or larger bucket blocked the song rendition")
	}
	assertArtworkAccounting(t, c)
}

func TestArtworkPreviewLimitsAndEligibility(t *testing.T) {
	for _, policy := range []string{"no-store", "public, no-cache", "private, max-age=3600", "public, max-age=3600"} {
		t.Run(policy, func(t *testing.T) {
			c := artworkTestCache(t, nil)
			data := artworkTestPNG(t, 128, 128)
			c.client.Transport = artworkTransport(func(*http.Request) (*http.Response, error) {
				return artworkHTTPResponse(data, http.Header{"Cache-Control": {policy}}), nil
			})
			const raw = "https://i.ytimg.com/preview-policy"
			key := artRequest{url: raw, edge: 128}
			sourceArtworkFrame(c, raw, 128, artPrimary)
			c.workers.Wait()
			if preview := c.smallPreviews.entries[key]; preview != nil && !preview.staged {
				t.Fatal("worker published a preview before a visible read")
			}
			requireArtworkSourceWidth(t, sourceArtworkFrame(c, raw, 128, artPrimary), 128)
			if policy != "public, max-age=3600" {
				if len(c.smallPreviews.entries) != 0 {
					t.Fatal("non-cacheable response retained a preview")
				}
				return
			}
			artworkPreviewEvict(t, c, key)
			c.smallPreviews.entries[key].freshUntil = time.Now().Add(-time.Second)
			if c.peek(raw, 128) != nil || len(c.smallPreviews.entries) != 0 || c.smallPreviews.bytes != 0 {
				t.Fatal("expired preview was reused or remained charged")
			}
			assertArtworkAccounting(t, c)
		})
	}
	c := artworkTestCache(t, artworkTestPNG(t, 384, 384))
	cost := artworkPixelCost(artworkPreviewEdge, artworkPreviewEdge)
	c.smallPreviews.budget = 2 * cost
	c.browsePreviews.limit = 1
	const avatar, next = "https://i.ytimg.com/avatar", "https://i.ytimg.com/next"
	for i := range 4 {
		src := fmt.Sprintf("https://i.ytimg.com/preview-limit-%d", i)
		sourceArtworkFrame(c, src, 128, artPrimary)
		c.workers.Wait()
		sourceArtworkFrame(c, src, 128, artPrimary)
		sourceArtworkFrame(c, src, 384, artPrimary)
		c.workers.Wait()
		sourceArtworkFrame(c, src, 384, artPrimary)
	}
	sourceArtworkFrame(c, avatar, 128, artAvatar)
	c.workers.Wait()
	sourceArtworkFrame(c, avatar, 128, artAvatar)
	sourceArtworkFrame(c, next, 384, artPrefetch)
	c.workers.Wait()
	if len(c.smallPreviews.entries) != 2 || len(c.browsePreviews.entries) != 1 ||
		c.smallPreviews.entries[artRequest{url: avatar, edge: 128, avatar: true}] != nil {
		t.Fatal("preview limits, avatar isolation, or prefetch isolation failed")
	}
	assertArtworkAccounting(t, c)
}

func TestArtworkPreviewTracksDownloadGeneration(t *testing.T) {
	const raw = artworkSourceURL
	original := artworkSourcePNG(t, 128, color.RGBA{225, 65, 23, 255})
	m, _ := artworkSourceLibrary(t, map[string][]byte{artworkSourceID: original})
	completeArtworkSource(t, m, artworkSourceID, raw)
	c := artworkTestCache(t, artworkTestPNG(t, 128, 128))
	c.downloadCovers = m
	sourceArtworkFrame(c, raw, 128, artPrimary)
	c.workers.Wait()
	sourceArtworkFrame(c, raw, 128, artPrimary)
	f := c.beginFrame()
	key := f.key(raw, 128, artPrimary)
	artworkPreviewEvict(t, c, key)
	requireArtworkSourceWidth(t, f.peek("https://LH3.googleusercontent.com/shared-album=w384-h384-k", 384), artworkPreviewEdge)
	requireArtworkSourceWidth(t, sourceArtworkFrame(c, raw, 128, artPrimary), artworkPreviewEdge)
	c.workers.Wait()
	requireArtworkSourceWidth(t, sourceArtworkFrame(c, raw, 128, artPrimary), 128)
	artworkPreviewEvict(t, c, key)
	if err := m.Remove(artworkSourceID); err != nil {
		t.Fatal(err)
	}
	if f.peek(raw, 128) != nil || c.peek(raw, 128) != nil {
		t.Fatal("revoked download preview leaked into a subsequent library generation")
	}
	c.beginFrame()
	if c.smallPreviews.bytes != 0 {
		t.Fatal("revoked download preview remained charged")
	}
}

func TestArtworkPreviewAuthoritativeWithdrawal(t *testing.T) {
	const raw = "https://lh3.googleusercontent.com/preview-withdrawal=s384"
	for _, scenario := range []struct {
		name   string
		status int
		policy string
		age    string
		revoke bool
	}{
		{"404", 404, "", "", true},
		{"410", 410, "", "", true},
		{"corrupt-no-store", 200, "no-store", "", true},
		{"corrupt-without-public", 200, "max-age=3600", "", true},
		{"corrupt-no-cache", 200, "public, no-cache", "", true},
		{"304-no-store", 304, "no-store", "", true},
		{"304-no-cache", 304, "public, no-cache", "", true},
		{"304-aged", 304, "public, max-age=60", "60", true},
		{"304-fresh", 304, "public, max-age=3600", "1", false},
		{"transient-network-error", 0, "", "", false},
		{"temporary-503", 503, "", "", false},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			c := artworkTestCache(t, nil)
			c.disk = &artworkDisk{dir: t.TempDir()}
			data := artworkTestPNG(t, 384, 384)
			replace := false
			formalURL := artworkRequestAt(raw, 384)
			otherURL := artworkRequestAt(raw, 128)
			c.client.Transport = artworkTransport(func(r *http.Request) (*http.Response, error) {
				if replace && r.URL.String() == formalURL {
					if scenario.status == 0 {
						return nil, errors.New("transient fixture error")
					}
					h := http.Header{"Cache-Control": {scenario.policy}}
					if scenario.age != "" {
						h.Set("Age", scenario.age)
					}
					res := artworkHTTPResponse([]byte("invalid cover"), h)
					res.StatusCode = scenario.status
					return res, nil
				}
				return artworkHTTPResponse(data, http.Header{"Cache-Control": {"public, max-age=3600"}}), nil
			})
			sourceArtworkFrame(c, raw, 128, artPrimary)
			c.workers.Wait()
			sourceArtworkFrame(c, raw, 128, artPrimary)
			sourceArtworkFrame(c, raw, 384, artPrimary)
			c.workers.Wait()
			sourceArtworkFrame(c, raw, 384, artPrimary)
			smallKey := artRequest{url: otherURL, edge: 128}
			browseKey := artRequest{url: formalURL, edge: 384}
			artworkPreviewEvict(t, c, browseKey)
			// Keep a fresh memory preview alongside a stale public disk
			// response so the client must ask the authority on revisit.
			if err := c.disk.Delete(formalURL); err != nil {
				t.Fatal(err)
			}
			loadHTTPArtwork(t, c.disk.dir, raw, func(*http.Request) (*http.Response, error) {
				return artworkHTTPResponse(data, http.Header{"Cache-Control": {"public, max-age=0"}, "Etag": {"\"old\""}}), nil
			})
			if _, err := c.disk.Get(formalURL); err != nil {
				t.Fatalf("missing stale disk fixture: %v", err)
			}
			replace = true
			sourceArtworkFrame(c, raw, 384, artPrimary)
			c.workers.Wait()
			if cached := c.browsePreviews.entries[browseKey] != nil; cached == scenario.revoke {
				t.Fatalf("preview present=%v after %s", cached, scenario.name)
			}
			if c.smallPreviews.entries[smallKey] == nil {
				t.Fatal("withdrawal crossed into a different CDN URL")
			}
			if scenario.revoke && scenario.age == "" && scenario.policy != "public, no-cache" {
				if _, err := c.disk.Get(formalURL); err == nil {
					t.Fatal("old public disk entry survived an authoritative withdrawal")
				}
			}
			if _, err := c.disk.Get(otherURL); err != nil {
				t.Fatal("disk deletion crossed into a different CDN URL")
			}
			assertArtworkAccounting(t, c)
		})
	}
}

func TestArtworkPreviewReplacementWithoutPreviewRoom(t *testing.T) {
	const raw = "https://i.ytimg.com/preview-replacement"
	key := artRequest{url: raw, edge: 384}
	oldBody := artworkPreviewFixture(t, 384, 8192, color.RGBA{200, 15, 15, 255})
	newBody := artworkPreviewFixture(t, 384, 8192, color.RGBA{15, 15, 200, 255})
	c := artworkTestCache(t, nil)
	var gets atomic.Int32
	c.client.Transport = artworkTransport(func(*http.Request) (*http.Response, error) {
		body := oldBody
		if gets.Add(1) > 1 {
			body = newBody
		}
		return artworkHTTPResponse(body, http.Header{"Cache-Control": {"public, max-age=3600"}}), nil
	})
	sourceArtworkFrame(c, raw, 384, artPrimary)
	c.workers.Wait()
	sourceArtworkFrame(c, raw, 384, artPrimary)
	artworkPreviewEvict(t, c, key)
	oldPreview := c.peek(raw, 384)
	requireArtworkSourceWidth(t, oldPreview, artworkPreviewEdge)
	c.browsePreviews.budget = 0
	if sourceArtworkFrame(c, raw, 384, artPrimary) != oldPreview {
		t.Fatal("in-flight replacement could not use current preview")
	}
	c.workers.Wait()
	requireArtworkSourceWidth(t, sourceArtworkFrame(c, raw, 384, artPrimary), 384)
	if gets.Load() != 2 || c.browsePreviews.entries[key] != nil {
		t.Fatal("replacement retained the previous body's preview")
	}
	artworkPreviewEvict(t, c, key)
	if c.peek(raw, 384) != nil {
		t.Fatal("evicting the replacement revived old artwork")
	}
}

func TestArtworkPreviewYieldsAtCombinedBudget(t *testing.T) {
	cost := artworkPixelCost(artworkPreviewEdge, artworkPreviewEdge)
	data := artworkTestPNG(t, 128, 128)
	downloaded, err := io.ReadAll(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	charge := cap(downloaded) + artworkPixelCost(128, 128)
	c := artworkTestCache(t, data)
	// Represent the settled retained charge at each pool's production limit.
	// The incoming thumbnail fits without an eviction in the original pool.
	for _, f := range []struct {
		pool  *artworkPool
		key   artRequest
		bytes int
	}{
		{&c.small, artRequest{url: "small", edge: 128}, artworkSmallBudget - charge},
		{&c.browse, artRequest{url: "browse", edge: 384}, artworkBrowseBudget},
		{&c.player, artRequest{url: "player", edge: 768}, artworkPlayerBudget - cost},
	} {
		f.pool.entries[f.key] = &artEntry{bytes: f.bytes, local: true}
		f.pool.bytes = f.bytes
	}
	art := decoded{preview: ui.NewBitmap(image.NewRGBA(image.Rect(0, 0, artworkPreviewEdge, artworkPreviewEdge))), local: true}
	version := artworkVersion{request: 1}
	displayed := artRequest{url: "displayed", edge: 384}
	staged := artRequest{url: "staged", edge: 128}
	c.stagePreview(displayed, art, version, true)
	c.rememberPreview(displayed, &artEntry{local: true, version: version}, time.Now(), true)
	c.stagePreview(staged, art, version, true)
	if c.smallPreviews.entries[staged] == nil || c.browsePreviews.entries[displayed].staged {
		t.Fatal("could not establish staged and displayed preview fixtures")
	}
	assertArtworkAccounting(t, c)
	const raw = "https://i.ytimg.com/combined-budget-admission"
	sourceArtworkFrame(c, raw, 128, artPrimary)
	c.workers.Wait()
	key := artRequest{url: raw, edge: 128}
	if c.entry(key).bitmap == nil || len(c.small.entries) != 2 || c.small.bytes != artworkSmallBudget {
		t.Fatal("preview pressure displaced a formal thumbnail")
	}
	if len(c.smallPreviews.entries) != 0 || c.browsePreviews.entries[displayed] == nil || c.artworkRoom() != 0 {
		t.Fatal("formal admission did not reclaim the staged preview first at 80 MiB")
	}
	assertArtworkAccounting(t, c)
}

func TestArtworkPreviewWithdrawalOrdersOverlappingLoads(t *testing.T) {
	for _, scenario := range []string{"newer-withdraws", "older-withdraws", "newer-replaces"} {
		t.Run(scenario, func(t *testing.T) {
			c := artworkTestCache(t, nil)
			data := artworkTestPNG(t, 384, 384)
			newBody := artworkPreviewFixture(t, 384, 0, color.RGBA{15, 15, 200, 255})
			const raw = "https://i.ytimg.com/preview-overlap"
			started, release, finished := make(chan struct{}), make(chan struct{}), make(chan struct{}, 8)
			unblock := sync.OnceFunc(func() { close(release) })
			defer unblock()
			c.redraw = func() { finished <- struct{}{} }
			var requests atomic.Int32
			c.client.Transport = artworkTransport(func(r *http.Request) (*http.Response, error) {
				call := requests.Add(1)
				if call == 1 {
					close(started)
					<-release
				}
				if (call == 2 && scenario == "newer-withdraws") || (call == 1 && scenario == "older-withdraws") {
					res := artworkHTTPResponse(nil, nil)
					res.StatusCode = http.StatusGone
					return res, nil
				}
				if call == 2 && scenario == "newer-replaces" {
					return artworkHTTPResponse(newBody, http.Header{"Cache-Control": {"public, max-age=3600"}}), nil
				}
				return artworkHTTPResponse(data, http.Header{"Cache-Control": {"public, max-age=3600"}}), nil
			})
			sourceArtworkFrame(c, raw, 384, artPrimary)
			select {
			case <-started:
			case <-time.After(time.Second):
				t.Fatal("old request did not arrive")
			}
			f := c.beginFrame()
			f.get(raw, 384, artPrimary)
			f.get(raw, 128, artPrimary)
			f.submit()
			select {
			case <-finished:
			case <-time.After(time.Second):
				t.Fatal("new request did not finish")
			}
			if scenario != "newer-withdraws" {
				requireArtworkSourceWidth(t, sourceArtworkFrame(c, raw, 128, artPrimary), 128)
			}
			unblock()
			c.workers.Wait()
			if scenario == "newer-withdraws" {
				if c.peek(raw, 384) != nil {
					t.Fatal("old successful body resurfaced after newer 410")
				}
			} else {
				artworkPreviewEvict(t, c, artRequest{url: raw, edge: 128})
				requireArtworkSourceWidth(t, c.peek(raw, 384), artworkPreviewEdge)
			}
			assertArtworkAccounting(t, c)
		})
	}
}

func TestArtworkFreshnessFollowsNewestAcceptedRequest(t *testing.T) {
	for _, ttl := range []struct {
		name         string
		older, newer int
	}{
		{"shortens", 600, 1},
		{"extends", 1, 600},
	} {
		for _, usePreview := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/preview=%t", ttl.name, usePreview), func(t *testing.T) {
				synctest.Test(t, func(t *testing.T) {
					const raw = "https://i.ytimg.com/freshness-overlap"
					c := artworkTestCache(t, nil)
					data := artworkTestPNG(t, 768, 768)
					releaseOlder := make(chan struct{})
					unblock := sync.OnceFunc(func() { close(releaseOlder) })
					defer unblock()
					var requests atomic.Int32
					c.client.Transport = artworkTransport(func(*http.Request) (*http.Response, error) {
						seconds := ttl.older
						switch requests.Add(1) {
						case 2:
							<-releaseOlder
						case 3:
							seconds = ttl.newer
						}
						h := http.Header{"Cache-Control": {fmt.Sprintf("public, max-age=%d", seconds)}}
						return artworkHTTPResponse(data, h), nil
					})

					sourceArtworkFrame(c, raw, 128, artPrimary)
					c.workers.Wait()
					requireArtworkSourceWidth(t, sourceArtworkFrame(c, raw, 128, artPrimary), 128)
					sourceArtworkFrame(c, raw, 768, artPlayer)
					synctest.Wait() // The older large rendition has reached the transport barrier.
					f := c.beginFrame()
					f.get(raw, 768, artPlayer)
					f.get(raw, 384, artPrimary)
					f.submit()
					synctest.Wait()
					requireArtworkSourceWidth(t, c.peek(raw, 384), 384)
					acceptedAt := time.Now()
					time.Sleep(250 * time.Millisecond)
					unblock()
					c.workers.Wait()

					f = c.beginFrame()
					for _, edge := range artworkBuckets {
						requireArtworkSourceWidth(t, f.get(raw, edge, artPrimary), edge)
					}
					f.submit()
					if usePreview {
						for _, edge := range artworkBuckets {
							artworkPreviewEvict(t, c, artRequest{url: raw, edge: edge})
						}
					}
					empty := c.beginFrame()
					empty.submit() // The following reads represent re-entry into the viewport.
					displayed := make(map[int]ui.ImageSource)
					for _, edge := range artworkBuckets {
						displayed[edge] = c.peek(raw, edge)
					}

					assertFresh := func() {
						t.Helper()
						f := c.beginFrame()
						for _, edge := range artworkBuckets {
							width := edge
							if usePreview {
								width = artworkPreviewEdge
							}
							got := f.peek(raw, edge)
							requireArtworkSourceWidth(t, got, width)
							if got != displayed[edge] || f.get(raw, edge, artPrimary) != got {
								t.Fatalf("%d px fell back before the latest deadline", edge)
							}
						}
					}
					if ttl.newer > ttl.older {
						time.Sleep(time.Until(acceptedAt.Add(2 * time.Second)))
						assertFresh() // Both older deadlines are already in the past.
					}
					deadline := acceptedAt.Add(time.Duration(ttl.newer) * time.Second)
					time.Sleep(time.Until(deadline) - time.Nanosecond)
					assertFresh()
					time.Sleep(time.Nanosecond)
					f = c.beginFrame()
					for _, edge := range artworkBuckets {
						if f.peek(raw, edge) != nil || f.get(raw, edge, artPrimary) != nil {
							t.Fatalf("%d px artwork survived the latest deadline", edge)
						}
					}
					if n := requests.Load(); n != 3 {
						t.Fatalf("expected three rendition requests, got %d", n)
					}
				})
			})
		}
	}
}

func TestArtworkPreviewCanceledWithdrawalKeepsLastDisplay(t *testing.T) {
	c := artworkTestCache(t, nil)
	data := artworkTestPNG(t, 384, 384)
	const raw = "https://i.ytimg.com/preview-canceled"
	key := artRequest{url: raw, edge: 384}
	started, release := make(chan struct{}), make(chan struct{})
	unblock := sync.OnceFunc(func() { close(release) })
	defer unblock()
	var calls atomic.Int32
	c.client.Transport = artworkTransport(func(*http.Request) (*http.Response, error) {
		if calls.Add(1) == 1 {
			return artworkHTTPResponse(data, http.Header{"Cache-Control": {"public, max-age=3600"}}), nil
		}
		close(started)
		<-release // The transport delivers a late status even after cancellation.
		res := artworkHTTPResponse(nil, nil)
		res.StatusCode = http.StatusGone
		return res, nil
	})
	sourceArtworkFrame(c, raw, 384, artPrimary)
	c.workers.Wait()
	sourceArtworkFrame(c, raw, 384, artPrimary)
	artworkPreviewEvict(t, c, key)
	preview := sourceArtworkFrame(c, raw, 384, artPrimary)
	requireArtworkSourceWidth(t, preview, artworkPreviewEdge)
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("revisit did not reach upstream")
	}
	c.mu.Lock()
	c.abandonLoad(c.running[c.entry(key)])
	c.mu.Unlock()
	unblock()
	c.workers.Wait()
	if c.peek(raw, 384) != preview {
		t.Fatal("a canceled load removed the last displayed preview")
	}
	assertArtworkAccounting(t, c)
}

func TestArtworkPreviewMatchesJPEGOrientation(t *testing.T) {
	var data bytes.Buffer
	im := tintFill(96, 64, func(x, y int) color.RGBA {
		return color.RGBA{uint8(x * 2), uint8(y * 3), 20, 255}
	})
	if err := jpeg.Encode(&data, im, nil); err != nil {
		t.Fatal(err)
	}
	// APP1 EXIF, little endian, with one short Orientation field (6).
	exif := []byte{0xff, 0xe1, 0, 34, 'E', 'x', 'i', 'f', 0, 0,
		'I', 'I', 42, 0, 8, 0, 0, 0, 1, 0, 18, 1, 3, 0, 1, 0, 0, 0, 6, 0, 0, 0, 0, 0, 0, 0}
	body := append(append(append([]byte{}, data.Bytes()[:2]...), exif...), data.Bytes()[2:]...)
	c := artworkTestCache(t, body)
	const raw = "https://i.ytimg.com/preview-jpeg"
	sourceArtworkFrame(c, raw, 128, artPrimary)
	c.workers.Wait()
	requireArtworkSourceWidth(t, sourceArtworkFrame(c, raw, 128, artPrimary), 64)
	artworkPreviewEvict(t, c, artRequest{url: raw, edge: 128})
	b := c.peek(raw, 128).(*ui.Bitmap)
	if w, h := b.Size(); w != 32 || h != 48 {
		t.Fatalf("EXIF preview size = %dx%d", w, h)
	}
}

func BenchmarkArtworkPreviewCreate(b *testing.B) {
	for _, im := range []image.Image{
		image.NewRGBA(image.Rect(0, 0, 384, 384)),
		image.NewYCbCr(image.Rect(0, 0, 384, 384), image.YCbCrSubsampleRatio420),
	} {
		b.Run(fmt.Sprintf("%T", im), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				newArtworkPreview(im, nil)
			}
		})
	}
}
