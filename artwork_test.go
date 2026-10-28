package main

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/png"
	"io"
	"net/http"
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
