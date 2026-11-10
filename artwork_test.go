package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/png"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

type artworkTransport func(*http.Request) (*http.Response, error)

func (f artworkTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func requestTestArtwork(c *artworkCache, raw string, edge int) ui.ImageSource {
	f := artworkFrame{cache: c}
	image := f.get(raw, edge, artPrimary)
	f.submit()
	return image
}

func TestArtworkWorkRemainsBoundedWhileScrolling(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	c := newArtworkCache(ctx, nil)
	var active, maximum atomic.Int32
	started := make(chan struct{}, artworkWorkers)
	c.client.Transport = artworkTransport(func(r *http.Request) (*http.Response, error) {
		n := active.Add(1)
		defer active.Add(-1)
		for old := maximum.Load(); old < n && !maximum.CompareAndSwap(old, n); old = maximum.Load() {
		}
		started <- struct{}{}
		<-r.Context().Done()
		return nil, r.Context().Err()
	})
	t.Cleanup(func() { cancel(); c.workers.Wait() })
	frame := artworkFrame{cache: c}
	for i := range 5000 {
		frame.get(fmt.Sprintf("https://i.ytimg.com/vi/item-%d/hqdefault.jpg", i), 128, artPrimary)
	}
	frame.submit()
	for range artworkWorkers {
		select {
		case <-started:
		case <-time.After(2 * time.Second):
			t.Fatal("artwork workers did not start")
		}
	}
	c.mu.Lock()
	entries, queued := len(c.entries), len(c.queue)
	c.mu.Unlock()
	if len(frame.wanted) > artworkWorkers+artworkQueueLimit || entries > artworkWorkers || queued > artworkQueueLimit || maximum.Load() > artworkWorkers {
		t.Fatalf("unbounded pending artwork: entries=%d queued=%d workers=%d", entries, queued, maximum.Load())
	}
	cancel()
	done := make(chan struct{})
	go func() { c.workers.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("canceled artwork workers did not exit")
	}
}

// A desktop frame sees the library first. If its first thumbnail also appears
// on the page it should share the page request, even with a full waiting list.
func TestArtworkSchedulesPageBeforeSidebarAndDropsOldViewport(t *testing.T) {
	c := artworkTestCache(t, nil)
	started := make(chan *http.Request, 2*artworkWorkers)
	c.client.Transport = artworkTransport(func(r *http.Request) (*http.Response, error) {
		started <- r
		<-r.Context().Done()
		return nil, r.Context().Err()
	})
	const origin = "https://i.ytimg.com"
	frame := artworkFrame{cache: c}
	for i := range artworkQueueLimit {
		frame.get(fmt.Sprintf("%s/side/%d", origin, i), 128, artSecondary)
	}
	frame.get(origin+"/side/0", 128, artPrimary)
	for i := 1; i < artworkWorkers; i++ {
		frame.get(fmt.Sprintf("%s/page/%d", origin, i), 128, artPrimary)
	}
	frame.submit()
	first := map[string]*http.Request{}
	for range artworkWorkers {
		select {
		case req := <-started:
			path := req.URL.Path
			if first[path] != nil || (path != "/side/0" && !strings.HasPrefix(path, "/page/")) {
				t.Fatalf("duplicate or lower-priority request in the first batch: %s", path)
			}
			first[path] = req
		case <-time.After(2 * time.Second):
			t.Fatal("page did not use all available slots")
		}
	}
	// A new viewport retains one in-flight cover and replaces everything else.
	next := artworkFrame{cache: c}
	next.get(origin+"/side/0", 128, artSecondary)
	next.get(origin+"/new", 128, artPrimary)
	next.submit()
	select {
	case req := <-started:
		if req.URL.Path != "/new" {
			t.Fatal("work from the previous viewport was started")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("new page waited behind obsolete requests")
	}
	for path, req := range first {
		if (req.Context().Err() == nil) != (path == "/side/0") {
			t.Fatalf("wrong request survived the viewport change: %s", path)
		}
	}
}

func TestArtworkSidebarLeavesCapacityForLatePage(t *testing.T) {
	c := artworkTestCache(t, nil)
	started := make(chan string, artworkWorkers)
	c.client.Transport = artworkTransport(func(r *http.Request) (*http.Response, error) {
		started <- r.URL.Path
		<-r.Context().Done()
		return nil, r.Context().Err()
	})
	render := func(withPage bool) {
		f := artworkFrame{cache: c}
		for i := range artworkWorkers {
			f.get(fmt.Sprintf("https://i.ytimg.com/side/%d", i), 128, artSecondary)
		}
		if withPage {
			f.get("https://i.ytimg.com/page", 384, artPrimary)
		}
		f.submit()
	}
	render(false)
	c.mu.Lock()
	active := c.active
	c.mu.Unlock()
	if active != artworkSecondaryLimit {
		t.Fatalf("%d simultaneous sidebar requests", active)
	}
	for range artworkSecondaryLimit {
		select {
		case <-started:
		case <-time.After(2 * time.Second):
			t.Fatal("sidebar did not load")
		}
	}
	render(true)
	select {
	case path := <-started:
		if path != "/page" {
			t.Fatal("sidebar displaced new page")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("page waited for the sidebar")
	}
}

func TestArtworkLargeCoversLeaveSlotsForListThumbnails(t *testing.T) {
	c := artworkTestCache(t, nil)
	c.client.Transport = artworkTransport(func(req *http.Request) (*http.Response, error) {
		<-req.Context().Done()
		return nil, req.Context().Err()
	})
	frame := artworkFrame{cache: c}
	for _, edge := range []int{384, 128} {
		for i := range artworkWorkers {
			frame.get(fmt.Sprintf("https://i.ytimg.com/%d/%d", edge, i), edge, artPrimary)
		}
	}
	frame.submit()
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.active != artworkWorkers || c.largeActive != artworkLargeLimit {
		t.Fatalf("unexpected mix of in-flight covers: %d total, %d large", c.active, c.largeActive)
	}
}

func TestArtworkCanceledLoadCannotReplaceRevisitedCover(t *testing.T) {
	data := artworkTestPNG(t, 16, 16)
	c := artworkTestCache(t, data)
	original := c.client.Transport
	first := make(chan struct{})
	release := make(chan struct{})
	unblock := sync.OnceFunc(func() { close(release) })
	defer unblock()
	var attempts atomic.Int32
	c.client.Transport = artworkTransport(func(r *http.Request) (*http.Response, error) {
		if attempts.Add(1) == 1 {
			close(first)
			<-r.Context().Done()
			// Hold the old failure until after the replacement is cached.
			select {
			case <-release:
			case <-c.ctx.Done():
			}
			return nil, r.Context().Err()
		}
		return original.RoundTrip(r)
	})
	completed := make(chan struct{}, 2)
	c.redraw = func() { completed <- struct{}{} }
	const raw = "https://i.ytimg.com/revisited"
	requestTestArtwork(c, raw, 128)
	select {
	case <-first:
	case <-time.After(2 * time.Second):
		t.Fatal("first download never started")
	}
	empty := artworkFrame{cache: c}
	empty.submit()
	requestTestArtwork(c, raw, 128)
	select {
	case <-completed:
	case <-time.After(2 * time.Second):
		t.Fatal("revisited cover did not load")
	}
	if requestTestArtwork(c, raw, 128) == nil {
		t.Fatal("replacement did not publish its bitmap")
	}
	unblock()
	c.workers.Wait()
	if requestTestArtwork(c, raw, 128) == nil || attempts.Load() != 2 || len(completed) != 0 {
		t.Fatal("obsolete result replaced the image, retried, or requested a redraw")
	}
}

func artworkTestPNG(t *testing.T, width, height int) []byte {
	t.Helper()
	var data bytes.Buffer
	if err := png.Encode(&data, image.NewRGBA(image.Rect(0, 0, width, height))); err != nil {
		t.Fatal(err)
	}
	return data.Bytes()
}

func artworkTestCache(t *testing.T, data []byte) *artworkCache {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	c := newArtworkCache(ctx, nil)
	t.Cleanup(func() { cancel(); c.workers.Wait() })
	c.client.Transport = artworkTransport(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, ContentLength: -1, Body: io.NopCloser(bytes.NewReader(data))}, nil
	})
	return c
}

func TestArtworkResizePreservesAspectAndCapsRetainedPixels(t *testing.T) {
	c := artworkTestCache(t, artworkTestPNG(t, 1536, 768))
	request := artRequest{url: "https://i.ytimg.com/vi/example/hqdefault.jpg", edge: 128}
	requestTestArtwork(c, request.url, request.edge)
	c.workers.Wait()
	if c.entries[request].bitmap == nil {
		t.Fatal("valid artwork did not decode")
	}
	if w, h := c.entries[request].bitmap.Size(); w != 128 || h != 64 {
		t.Fatalf("wrong artwork aspect or resolution: %dx%d", w, h)
	}
	if c.bytes <= 128*64*4 {
		t.Fatal("retained pixel budget excludes renderer mipmaps")
	}
	c = artworkTestCache(t, artworkTestPNG(t, 768, 768))
	for i := range 24 {
		request := artRequest{url: fmt.Sprintf("https://i.ytimg.com/vi/%d/hqdefault.jpg", i), edge: artworkEdge}
		requestTestArtwork(c, request.url, request.edge)
		c.workers.Wait()
		if c.bytes > artworkPixelBudget {
			t.Fatalf("retained pixels exceeded budget after %d covers: %d", i+1, c.bytes)
		}
	}
	if len(c.entries) >= 24 {
		t.Fatal("decoded covers did not evict as the cache filled")
	}
}

func TestArtworkRejectsOversizedDownloadAndDecode(t *testing.T) {
	for _, tc := range []struct {
		name string
		data []byte
	}{
		{"encoded", append(artworkTestPNG(t, 16, 16), make([]byte, artworkDownloadLimit)...)},
		{"decoded", artworkTestPNG(t, 3000, 2000)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := artworkTestCache(t, tc.data)
			request := artRequest{url: "https://i.ytimg.com/vi/example/hqdefault.jpg", edge: artworkEdge}
			requestTestArtwork(c, request.url, request.edge)
			c.workers.Wait()
			if c.entries[request].bitmap != nil || c.bytes != 0 || c.entries[request].cancel != nil {
				t.Fatal("oversized image was retained or remained pending")
			}
		})
	}
}

func TestArtworkFailedImageCanRecoverAfterBackoff(t *testing.T) {
	for _, failure := range []string{"connection", "http", "decode"} {
		t.Run(failure, func(t *testing.T) {
			data := artworkTestPNG(t, 16, 16)
			c := artworkTestCache(t, data)
			attempts := 0
			c.client.Transport = artworkTransport(func(*http.Request) (*http.Response, error) {
				attempts++
				body, status := data, http.StatusOK
				if attempts == 1 {
					switch failure {
					case "connection":
						return nil, errors.New("synthetic connection failure")
					case "http":
						status = http.StatusServiceUnavailable
					case "decode":
						body = []byte("synthetic incomplete image")
					}
				}
				return &http.Response{StatusCode: status, ContentLength: -1, Body: io.NopCloser(bytes.NewReader(body))}, nil
			})
			const raw = "https://i.ytimg.com/vi/example/hqdefault.jpg"
			request := artRequest{url: raw, edge: 128}
			requestTestArtwork(c, raw, 128)
			c.workers.Wait()
			e := c.entries[request]
			if e.cancel != nil || e.bitmap != nil || !e.retryAt.After(time.Now()) {
				t.Fatal("failed image did not enter retry backoff")
			}
			for range 100 {
				requestTestArtwork(c, raw, 128)
			}
			if len(c.queue) != 0 || attempts != 1 {
				t.Fatal("redraws retried before backoff elapsed")
			}
			e.retryAt = time.Now().Add(-time.Second)
			frame := artworkFrame{cache: c}
			for range 100 {
				frame.get(raw, 128, artPrimary)
			}
			frame.submit()
			c.workers.Wait()
			if requestTestArtwork(c, raw, 128) == nil || e.cancel != nil || e.failures != 0 || !e.retryAt.IsZero() || attempts != 2 {
				t.Fatal("failed image did not recover with a single successful retry")
			}
		})
	}
}

func TestArtworkRetryBackoffIsCapped(t *testing.T) {
	c := artworkTestCache(t, []byte("synthetic invalid image"))
	request := artRequest{url: "https://i.ytimg.com/vi/example/hqdefault.jpg", edge: 128}
	e := &artEntry{}
	c.entries[request] = e
	for attempt := range 10 {
		e.retryAt = time.Time{}
		before := time.Now()
		requestTestArtwork(c, request.url, request.edge)
		c.workers.Wait()
		want := min(artworkRetryBase<<attempt, artworkRetryMax)
		if e.retryAt.Before(before.Add(want)) || e.retryAt.After(time.Now().Add(want)) {
			t.Fatal("retry did not use capped exponential backoff")
		}
	}
}

func TestArtworkRetryWakesIdleViewWithoutFetchingOffscreen(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	redraws := make(chan struct{}, 8)
	c := newArtworkCache(ctx, func() { redraws <- struct{}{} })
	t.Cleanup(func() { cancel(); c.workers.Wait() })
	var attempts atomic.Int32
	c.client.Transport = artworkTransport(func(*http.Request) (*http.Response, error) {
		attempts.Add(1)
		return &http.Response{StatusCode: http.StatusServiceUnavailable, Body: io.NopCloser(bytes.NewReader(nil))}, nil
	})
	requestTestArtwork(c, "https://i.ytimg.com/vi/example/hqdefault.jpg", 128)
	for _, event := range []string{"failed load", "retry deadline"} {
		select {
		case <-redraws:
		case <-time.After(artworkRetryBase + 2*time.Second):
			t.Fatalf("idle view did not redraw after %s", event)
		}
	}
	if attempts.Load() != 1 || len(c.queue) != 0 {
		t.Fatal("retry timer fetched artwork without a visible view requesting it")
	}
}

func TestArtworkRetryWakeHandlesDifferentDeadlines(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	redraws := make(chan struct{}, 8)
	c := &artworkCache{ctx: ctx, entries: map[artRequest]*artEntry{}, redraw: func() { redraws <- struct{}{} }}
	now := time.Now()
	c.entries[artRequest{url: "first"}] = &artEntry{retryAt: now.Add(10 * time.Millisecond)}
	c.entries[artRequest{url: "second"}] = &artEntry{retryAt: now.Add(500 * time.Millisecond)}
	c.mu.Lock()
	c.scheduleRetry(now)
	c.mu.Unlock()
	for range 2 {
		select {
		case <-redraws:
		case <-time.After(2 * time.Second):
			t.Fatal("a later retry deadline did not wake the view")
		}
	}
}

func TestSystemArtworkReachesDownloaderWithoutCDNSizeRewrite(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{}, 4)
	c := newArtworkCache(ctx, func() { done <- struct{}{} })
	t.Cleanup(func() { cancel(); c.workers.Wait() })
	data := artworkTestPNG(t, 16, 16)
	const raw = "https://gstatic.com/youtube/media/ytm/images/pbg/liked-songs-delhi-1200.png"
	const mix = "https://music.youtube.com/image/synthetic-system-mix?r=synthetic-image-option"
	c.client.Transport = artworkTransport(func(req *http.Request) (*http.Response, error) {
		if (req.URL.String() != raw && req.URL.String() != mix) || req.Header.Get("Cookie") != "" || req.Header.Get("Authorization") != "" {
			t.Error("system cover was rewritten or carried account credentials")
		}
		return &http.Response{StatusCode: http.StatusOK, ContentLength: int64(len(data)), Body: io.NopCloser(bytes.NewReader(data))}, nil
	})
	for _, source := range []string{raw, mix} {
		requestTestArtwork(c, source, 128)
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatal("official system artwork never reached the downloader")
		}
		if requestTestArtwork(c, source, 128) == nil {
			t.Fatal("system artwork did not decode")
		}
	}
	for _, rejected := range []string{
		"https://gstatic.com/other.png",
		"https://gstatic.com/youtube/media/ytm/images/pbg/cover.png?signature=synthetic",
		"https://gstatic.com.evil.test/youtube/media/ytm/images/pbg/cover.png",
		"https://music.youtube.com/youtubei/v1/browse?r=synthetic",
		"https://music.youtube.com/image/fixture?r=synthetic&account=synthetic",
	} {
		u, _ := url.Parse(rejected)
		if artworkURL(u) {
			t.Fatal("downloader accepts artwork outside the static directory")
		}
	}
}

// A cover that changes size bucket, as a panel opens or the window resizes,
// paints from the size already cached instead of blanking to a placeholder:
// a larger one as it is, a smaller one while its own size loads.
func TestArtworkPaintsCachedSizeWhileItsBucketChanges(t *testing.T) {
	c := artworkTestCache(t, artworkTestPNG(t, 16, 16))
	const raw = "https://i.ytimg.com/vi/example/hqdefault.jpg"
	medium := artRequest{url: artworkRequestAt(raw, 384), edge: 384}
	requestTestArtwork(c, raw, 300)
	c.workers.Wait()
	cached := c.entries[medium]
	if cached == nil || cached.bitmap == nil {
		t.Fatal("the 384 cover did not load")
	}
	// Smaller: the larger cover paints, and nothing loads.
	f := artworkFrame{cache: c}
	if got := f.get(raw, 100, artPrimary); got != cached.bitmap {
		t.Fatal("a smaller card blanked instead of painting the cached cover")
	}
	if len(f.wanted) != 0 {
		t.Fatalf("a cover smaller than one cached loaded again: %v", f.wanted)
	}
	// Larger: the cached cover paints while the larger one loads.
	f = artworkFrame{cache: c}
	if got := f.get(raw, 500, artPrimary); got != cached.bitmap {
		t.Fatal("a larger card blanked while its cover loads")
	}
	if len(f.wanted) != 1 {
		t.Fatalf("the larger cover was not asked for: %v", f.wanted)
	}
}
