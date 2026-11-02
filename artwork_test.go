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
	"sync/atomic"
	"testing"
	"time"
)

type artworkTransport func(*http.Request) (*http.Response, error)

func (f artworkTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

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
	for i := range 5000 {
		c.get(fmt.Sprintf("https://i.ytimg.com/vi/item-%d/hqdefault.jpg", i), 128)
	}
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
	if entries > artworkWorkers+artworkQueueLimit || queued > artworkQueueLimit || maximum.Load() > artworkWorkers {
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

func artworkTestPNG(t *testing.T, width, height int) []byte {
	t.Helper()
	var data bytes.Buffer
	if err := png.Encode(&data, image.NewRGBA(image.Rect(0, 0, width, height))); err != nil {
		t.Fatal(err)
	}
	return data.Bytes()
}

func artworkTestCache(data []byte) *artworkCache {
	return &artworkCache{ctx: context.Background(), entries: map[artRequest]*artEntry{}, client: &http.Client{Transport: artworkTransport(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, ContentLength: -1, Body: io.NopCloser(bytes.NewReader(data))}, nil
	})}}
}

func TestArtworkResizePreservesAspectAndCapsRetainedPixels(t *testing.T) {
	c := artworkTestCache(artworkTestPNG(t, 1536, 768))
	request := artRequest{url: "https://i.ytimg.com/vi/example/hqdefault.jpg", edge: 128}
	c.entries[request] = &artEntry{pending: true}
	c.load(request)
	if c.entries[request].bitmap == nil {
		t.Fatal("valid artwork did not decode")
	}
	if w, h := c.entries[request].bitmap.Size(); w != 128 || h != 64 {
		t.Fatalf("wrong artwork aspect or resolution: %dx%d", w, h)
	}
	if c.bytes <= 128*64*4 {
		t.Fatal("retained pixel budget excludes renderer mipmaps")
	}
	c = artworkTestCache(artworkTestPNG(t, 768, 768))
	for i := range 24 {
		request := artRequest{url: fmt.Sprintf("https://i.ytimg.com/vi/%d/hqdefault.jpg", i), edge: artworkEdge}
		c.entries[request] = &artEntry{pending: true, used: uint64(i + 1)}
		c.load(request)
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
			c := artworkTestCache(tc.data)
			request := artRequest{url: "https://i.ytimg.com/vi/example/hqdefault.jpg", edge: artworkEdge}
			c.entries[request] = &artEntry{pending: true}
			c.load(request)
			if c.entries[request].bitmap != nil || c.bytes != 0 || c.entries[request].pending {
				t.Fatal("oversized image was retained or remained pending")
			}
		})
	}
}

func TestArtworkFailedImageCanRecoverAfterBackoff(t *testing.T) {
	for _, failure := range []string{"connection", "http", "decode"} {
		t.Run(failure, func(t *testing.T) {
			data := artworkTestPNG(t, 16, 16)
			c := artworkTestCache(data)
			c.queue = make(chan artRequest, artworkQueueLimit)
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
			c.get(raw, 128)
			request := <-c.queue
			c.load(request)
			e := c.entries[request]
			if e.pending || e.bitmap != nil || !e.retryAt.After(time.Now()) {
				t.Fatal("failed image did not enter retry backoff")
			}
			for range 100 {
				c.get(raw, 128)
			}
			if len(c.queue) != 0 || attempts != 1 {
				t.Fatal("redraws retried before backoff elapsed")
			}
			e.retryAt = time.Now().Add(-time.Second)
			for range artworkQueueLimit {
				c.queue <- artRequest{url: "synthetic queued image"}
			}
			c.get(raw, 128)
			if e.pending || len(c.queue) != artworkQueueLimit {
				t.Fatal("retry was marked pending while the queue was full")
			}
			for range artworkQueueLimit {
				<-c.queue
			}
			for range 100 {
				c.get(raw, 128)
			}
			if len(c.queue) != 1 || !e.pending {
				t.Fatal("retry was lost or queued more than once")
			}
			c.load(<-c.queue)
			if c.get(raw, 128) == nil || e.pending || e.failures != 0 || !e.retryAt.IsZero() || attempts != 2 {
				t.Fatal("failed image remained empty after a successful retry")
			}
		})
	}
}

func TestArtworkRetryBackoffIsCapped(t *testing.T) {
	c := artworkTestCache([]byte("synthetic invalid image"))
	request := artRequest{url: "https://i.ytimg.com/vi/example/hqdefault.jpg", edge: 128}
	e := &artEntry{}
	c.entries[request] = e
	for attempt := range 10 {
		before := time.Now()
		c.load(request)
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
	c.get("https://i.ytimg.com/vi/example/hqdefault.jpg", 128)
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
		c.get(source, 128)
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatal("official system artwork never reached the downloader")
		}
		if c.get(source, 128) == nil {
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
