package main

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/png"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"ytmgo/internal/audioquality"
	"ytmgo/internal/catalog"
	"ytmgo/internal/downloads"
)

const (
	artworkSourceURL    = "https://lh3.googleusercontent.com/shared-album=s88-k"
	artworkSourceID     = "artworkOne1"
	artworkOtherID      = "artworkTwo2"
	artworkEmptyLibrary = "0123456789abcdef01234567"
)

func artworkSourcePNG(t *testing.T, width int, c color.RGBA) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, width, 8))
	for y := range 8 {
		for x := range width {
			img.SetRGBA(x, y, c)
		}
	}
	var body bytes.Buffer
	if err := png.Encode(&body, img); err != nil {
		t.Fatal(err)
	}
	return body.Bytes()
}

func artworkSourceLibrary(t *testing.T, covers map[string][]byte) (*downloads.Manager, string) {
	t.Helper()
	root := t.TempDir()
	m, err := downloads.New(t.Context(), root, &snapshotSecrets{},
		func(context.Context, string, audioquality.Quality) (downloads.Remote, error) {
			return downloads.Remote{
				Format: downloads.Format{ITag: 140, Version: "1", Type: "audio/mp4", Length: 5, Duration: time.Second},
				Open: func(_ context.Context, offset int64) (io.ReadCloser, error) {
					return io.NopCloser(strings.NewReader("audio"[offset:])), nil
				},
				Close: func() {},
			}, nil
		},
		func(_ context.Context, it catalog.Item) (catalog.LyricsResult, []byte) {
			return catalog.LyricsResult{}, covers[it.ID]
		})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(m.Close)
	return m, root
}

func completeArtworkSource(t *testing.T, m *downloads.Manager, id, raw string) {
	t.Helper()
	if err := m.Enqueue(m.Scope(), []catalog.Item{{ID: id, Artwork: raw, Kind: "song"}}, audioquality.Saver); err != nil {
		t.Fatal(err)
	}
	m.Online(true)
	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	for !m.Has(id) {
		select {
		case <-m.Changes():
		case <-deadline.C:
			t.Fatalf("download fixture failed: %v", m.Snapshot())
		}
	}
}

func sourceArtworkFrame(c *artworkCache, raw string, edge int, priority artworkPriority) ui.ImageSource {
	f := c.beginFrame()
	img := f.get(raw, edge, priority)
	f.submit()
	return img
}

func requireArtworkSourceWidth(t *testing.T, img ui.ImageSource, width int) {
	t.Helper()
	bitmap, ok := img.(*ui.Bitmap)
	if !ok {
		t.Fatalf("expected a bitmap of width %d, got %T", width, img)
	}
	if w, _ := bitmap.Size(); w != width {
		t.Fatalf("expected bitmap width %d, got %d", width, w)
	}
}

func TestArtworkSourceRemovalInvalidatesEveryRendition(t *testing.T) {
	fromFirst := artworkSourcePNG(t, 31, color.RGBA{220, 40, 30, 255})
	fromLast := artworkSourcePNG(t, 62, color.RGBA{50, 170, 70, 255})
	fromHTTP := artworkSourcePNG(t, 93, color.RGBA{20, 80, 180, 255})
	m, _ := artworkSourceLibrary(t, map[string][]byte{artworkSourceID: fromFirst, artworkOtherID: fromLast})
	completeArtworkSource(t, m, artworkSourceID, artworkSourceURL)
	completeArtworkSource(t, m, artworkOtherID, "https://LH3.googleusercontent.com/shared-album=w384-h384-k")
	c := artworkTestCache(t, nil)
	c.downloadCovers = m
	var requests atomic.Int32
	c.client.Transport = artworkTransport(func(*http.Request) (*http.Response, error) {
		requests.Add(1)
		return artworkHTTPResponse(fromHTTP, http.Header{"Cache-Control": {"public, max-age=3600"}}), nil
	})
	sourceArtworkFrame(c, artworkSourceURL, 128, artPrimary)
	c.workers.Wait()
	f := c.beginFrame()
	requireArtworkSourceWidth(t, f.get(artworkSourceURL, 768, artPlayer), 62)
	if f.peek("https://LH3.googleusercontent.com/shared-album=s192-k", 128) != f.peek(artworkSourceURL, 128) {
		t.Fatal("CDN URL aliases created independent bitmap identities for one local asset")
	}
	if _, ok := f.tint(artworkSourceURL); !ok {
		t.Fatal("the local cover did not supply a tint")
	}
	f.submit()
	c.workers.Wait()
	if got := requests.Load(); got != 0 {
		t.Fatalf("local source made %d HTTP requests", got)
	}
	if err := m.Remove(artworkOtherID); err != nil {
		t.Fatal(err)
	}
	if got := f.peek(artworkSourceURL, 128); got != nil {
		t.Fatal("a retained frame drew the deleted cover")
	}
	if _, ok := f.tint(artworkSourceURL); ok {
		t.Fatal("a retained frame returned the deleted cover's tint")
	}
	nextURL := "https://i.ytimg.com/vi/next/synthetic.jpg"
	f = c.beginFrame()
	if got := f.get(nextURL, 768, artPlayer); got != nil {
		t.Fatal("the player used the deleted cover as transition fallback")
	}
	if got := f.get(artworkSourceURL, 768, artPrefetch); got != nil {
		t.Fatal("next prefetch reused a deleted rendition")
	}
	// Submit only the prefetched source so a fallback request cannot mask
	// whether the first copy is still reachable.
	f = c.beginFrame()
	f.get(artworkSourceURL, 768, artPrefetch)
	f.submit()
	c.workers.Wait()
	f = c.beginFrame()
	requireArtworkSourceWidth(t, f.get(artworkSourceURL, 384, artPlayer), 31)
	requireArtworkSourceWidth(t, f.peek(artworkSourceURL, 128), 31)
	if got := requests.Load(); got != 0 {
		t.Fatalf("the remaining local copy was bypassed by %d HTTP requests", got)
	}
	if err := m.Remove(artworkSourceID); err != nil {
		t.Fatal(err)
	}
	if got := sourceArtworkFrame(c, artworkSourceURL, 128, artPlayer); got != nil {
		t.Fatal("last removed copy was offered at another size")
	}
	c.workers.Wait()
	requireArtworkSourceWidth(t, sourceArtworkFrame(c, artworkSourceURL, 128, artPlayer), 93)
	assertArtworkAccounting(t, c)
}

func TestArtworkSourceBadFileFallsBackToHTTPPolicy(t *testing.T) {
	for _, scenario := range []string{"missing", "corrupt"} {
		t.Run(scenario, func(t *testing.T) {
			body := artworkSourcePNG(t, 46, color.RGBA{180, 40, 30, 255})
			m, root := artworkSourceLibrary(t, map[string][]byte{artworkSourceID: body})
			completeArtworkSource(t, m, artworkSourceID, artworkSourceURL)
			files, err := filepath.Glob(filepath.Join(root, downloads.Guest, "*", "cover"))
			if err != nil || len(files) != 1 {
				t.Fatal("cover fixture missing", err)
			}
			c := artworkTestCache(t, nil)
			c.downloadCovers = m
			c.disk = &artworkDisk{dir: filepath.Join(t.TempDir(), "artwork")}
			var requests atomic.Int32
			c.client.Transport = artworkTransport(func(*http.Request) (*http.Response, error) {
				requests.Add(1)
				return artworkHTTPResponse(body, http.Header{"Cache-Control": {"no-store"}}), nil
			})
			edge := 128
			if scenario == "missing" {
				sourceArtworkFrame(c, artworkSourceURL, 128, artPrimary)
				c.workers.Wait()
				requireArtworkSourceWidth(t, c.peek(artworkSourceURL, 128), 46)
				err = os.Remove(files[0])
				edge = 384 // A new rendition makes the worker discover the removal.
			} else {
				err = os.WriteFile(files[0], []byte("invalid image"), 0600)
			}
			if err != nil {
				t.Fatal(err)
			}
			sourceArtworkFrame(c, artworkSourceURL, edge, artPrimary)
			c.workers.Wait()
			if m.Covers().Source(artworkSourceURL).Asset != nil || !m.Has(artworkSourceID) {
				t.Fatal("failed cover did not withdraw independently of offline audio")
			}
			if c.peek(artworkSourceURL, 128) != nil {
				t.Fatal("an older warm rendition survived withdrawal")
			}
			if requests.Load() != 0 {
				t.Fatal("same worker fetched HTTP under the local cover generation")
			}
			sourceArtworkFrame(c, artworkSourceURL, 128, artPrefetch)
			c.workers.Wait()
			for range 3 {
				sourceArtworkFrame(c, artworkSourceURL, 128, artPrefetch)
			}
			if requests.Load() != 1 || c.peek(artworkSourceURL, 128) != nil {
				t.Fatal("no-store prefetch was repeated or made reusable")
			}
			if got := sourceArtworkFrame(c, artworkSourceURL, 128, artPlayer); got != nil {
				t.Fatal("a real display reused the no-store prefetch")
			}
			c.workers.Wait()
			requireArtworkSourceWidth(t, sourceArtworkFrame(c, artworkSourceURL, 128, artPlayer), 46)
			f := c.beginFrame()
			if f.get(artworkSourceURL, 384, artPrimary) != nil || f.peek(artworkSourceURL, 128) != nil {
				t.Fatal("a no-store fallback was reused by another size or peek")
			}
			if _, ok := f.tint(artworkSourceURL); !ok {
				t.Fatal("currently displayed non-cacheable cover lost its colour")
			}
			// Submit a frame with no cover on screen. The old response can no
			// longer supply either pixels or colour when the track returns.
			empty := c.beginFrame()
			empty.submit()
			next := c.beginFrame()
			if _, ok := next.tint(artworkSourceURL); ok {
				t.Fatal("departed non-cacheable cover retained its colour")
			}
			if requests.Load() != 2 {
				t.Fatalf("expected two real HTTP requests, got %d", requests.Load())
			}
			if _, err := c.disk.Get(artworkRequestAt(artworkSourceURL, 128)); err == nil {
				t.Fatal("no-store fallback wrote a public cache record")
			}
		})
	}
}

func TestArtworkSourceCorruptWinnerFallsBackToAnotherDownload(t *testing.T) {
	good := artworkTestPNG(t, 29, 8)
	m, _ := artworkSourceLibrary(t, map[string][]byte{artworkSourceID: good, artworkOtherID: []byte("not an image")})
	completeArtworkSource(t, m, artworkSourceID, artworkSourceURL)
	completeArtworkSource(t, m, artworkOtherID, artworkSourceURL)
	c := artworkTestCache(t, nil)
	c.downloadCovers = m
	c.client.Transport = artworkTransport(func(req *http.Request) (*http.Response, error) {
		t.Error("HTTP requested while a valid local copy was available")
		return nil, errors.New("unexpected HTTP request")
	})
	old := m.Covers().Source(artworkSourceURL)
	sourceArtworkFrame(c, artworkSourceURL, 128, artPrimary)
	c.workers.Wait()
	fallback := m.Covers().Source(artworkSourceURL)
	if fallback.Asset == nil || fallback == old || !m.Has(artworkOtherID) {
		t.Fatal("decode failure did not withdraw only the selected cover")
	}
	sourceArtworkFrame(c, artworkSourceURL, 128, artPrimary)
	c.workers.Wait()
	requireArtworkSourceWidth(t, c.peek(artworkSourceURL, 128), 29)
}

func TestArtworkSourceAvatarDoesNotUseDownloadOrPublicDisk(t *testing.T) {
	local := artworkTestPNG(t, 20, 16)
	avatar := artworkTestPNG(t, 45, 16)
	m, _ := artworkSourceLibrary(t, map[string][]byte{artworkSourceID: local})
	completeArtworkSource(t, m, artworkSourceID, artworkSourceURL)
	c := artworkTestCache(t, nil)
	c.downloadCovers = m
	c.disk = &artworkDisk{dir: filepath.Join(t.TempDir(), "public")}
	var requests atomic.Int32
	c.client.Transport = artworkTransport(func(*http.Request) (*http.Response, error) {
		requests.Add(1)
		return artworkHTTPResponse(avatar, http.Header{"Cache-Control": {"public, max-age=3600"}}), nil
	})
	f := c.beginFrame()
	f.get(artworkSourceURL, 128, artPrimary)
	f.get(artworkSourceURL, 128, artAvatar)
	f.submit()
	c.workers.Wait()
	f = c.beginFrame()
	requireArtworkSourceWidth(t, f.get(artworkSourceURL, 128, artPrimary), 20)
	requireArtworkSourceWidth(t, f.get(artworkSourceURL, 128, artAvatar), 45)
	if requests.Load() != 1 {
		t.Fatal("avatar and cover used the same source")
	}
	if _, err := os.Stat(c.disk.dir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("avatar opened the public cache: %v", err)
	}
}

func TestArtworkSourceBlockedDecodeCannotPublishAfterLibrarySwitch(t *testing.T) {
	local := artworkTestPNG(t, 28, 16)
	remote := artworkTestPNG(t, 59, 16)
	m, _ := artworkSourceLibrary(t, map[string][]byte{artworkSourceID: local})
	completeArtworkSource(t, m, artworkSourceID, artworkSourceURL)
	c := artworkTestCache(t, remote)
	c.downloadCovers = m
	for range artworkDecoders {
		if !c.takeDecoder(t.Context(), nil, time.Time{}) {
			t.Fatal("could not reserve the test decoder")
		}
	}
	f := c.beginFrame()
	old := f.key(artworkSourceURL, 128, artPlayer)
	f.get(artworkSourceURL, 128, artPlayer)
	f.submit()
	deadline := time.NewTimer(2 * time.Second)
	defer deadline.Stop()
	poll := time.NewTicker(time.Millisecond)
	defer poll.Stop()
	for {
		c.mu.Lock()
		waiting := len(c.decodeQueue) == 1
		c.mu.Unlock()
		if waiting {
			break
		}
		select {
		case <-poll.C:
		case <-deadline.C:
			t.Fatal("old worker did not reach the decoder after reading the cover")
		}
	}
	// Exercise the finish check without a frame canceling the original job.
	c.mu.Lock()
	switchErr := m.Switch(artworkEmptyLibrary, false)
	empty := m.Covers()
	restoreErr := m.Switch(downloads.Guest, false)
	c.mu.Unlock()
	if switchErr != nil || restoreErr != nil {
		t.Fatal("switching the fixture library failed", switchErr, restoreErr)
	}
	if empty.Source(artworkSourceURL).Asset != nil || old.matches(m.Covers()) {
		t.Fatal("library selection revived a previous publication")
	}
	for range artworkDecoders {
		c.releaseDecoder()
	}
	c.workers.Wait()
	c.mu.Lock()
	oldEntry := c.entry(old)
	c.mu.Unlock()
	if oldEntry != nil {
		t.Fatal("old worker published a bitmap or retry under a revoked source")
	}
	if got := sourceArtworkFrame(c, artworkSourceURL, 128, artPlayer); got != nil {
		t.Fatal("old bitmap filled the new publication")
	}
	c.workers.Wait()
	requireArtworkSourceWidth(t, sourceArtworkFrame(c, artworkSourceURL, 128, artPlayer), 28)
	if err := m.Switch(artworkEmptyLibrary, false); err != nil {
		t.Fatal(err)
	}
	if got := sourceArtworkFrame(c, artworkSourceURL, 128, artPlayer); got != nil {
		t.Fatal("a previously selected library cover survived switching to an empty one")
	}
	c.workers.Wait()
	requireArtworkSourceWidth(t, sourceArtworkFrame(c, artworkSourceURL, 128, artPlayer), 59)
	assertArtworkAccounting(t, c)
}

func TestArtworkSourceLateHTTPDoesNotSurviveRemoteLocalRemote(t *testing.T) {
	first, last := artworkTestPNG(t, 24, 16), artworkTestPNG(t, 73, 16)
	m, _ := artworkSourceLibrary(t, map[string][]byte{artworkSourceID: first})
	c := artworkTestCache(t, nil)
	c.downloadCovers = m
	c.disk = &artworkDisk{dir: filepath.Join(t.TempDir(), "public")}
	started, release := make(chan struct{}), make(chan struct{})
	var requests atomic.Int32
	c.client.Transport = artworkTransport(func(req *http.Request) (*http.Response, error) {
		body := last
		if requests.Add(1) == 1 {
			close(started)
			select {
			case <-release:
				body = first
			case <-req.Context().Done():
				return nil, req.Context().Err()
			}
		}
		return artworkHTTPResponse(body, http.Header{"Cache-Control": {"public, max-age=3600"}}), nil
	})
	f := c.beginFrame()
	old := f.key(artworkSourceURL, 128, artPlayer)
	f.get(artworkSourceURL, 128, artPlayer)
	f.submit()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("old HTTP worker did not start")
	}
	completeArtworkSource(t, m, artworkSourceID, artworkSourceURL)
	if err := m.Remove(artworkSourceID); err != nil {
		t.Fatal(err)
	}
	close(release)
	c.workers.Wait()
	c.mu.Lock()
	oldEntry := c.entry(old)
	c.mu.Unlock()
	if oldEntry != nil || old.matches(m.Covers()) {
		t.Fatal("old remote generation survived a complete local replacement cycle")
	}
	if got := sourceArtworkFrame(c, artworkSourceURL, 128, artPlayer); got != nil {
		t.Fatal("late HTTP result was reused by the returned remote source")
	}
	c.workers.Wait()
	requireArtworkSourceWidth(t, sourceArtworkFrame(c, artworkSourceURL, 128, artPlayer), 73)
	if requests.Load() != 2 {
		t.Fatalf("expected the new source to fetch again, got %d requests", requests.Load())
	}
}

func TestArtworkSourceHTTPFallbackRevalidatesAndReuses304(t *testing.T) {
	const raw = "https://i.ytimg.com/vi/example/hqdefault.jpg"
	local, remote := artworkTestPNG(t, 11, 8), artworkTestPNG(t, 51, 8)
	m, _ := artworkSourceLibrary(t, map[string][]byte{artworkSourceID: local})
	completeArtworkSource(t, m, artworkSourceID, raw)
	c := artworkTestCache(t, nil)
	c.downloadCovers = m
	c.disk = &artworkDisk{dir: t.TempDir()}
	var requests atomic.Int32
	c.client.Transport = artworkTransport(func(req *http.Request) (*http.Response, error) {
		h := http.Header{"Etag": {`"fallback"`}, "Vary": {"Accept-Encoding"}}
		if requests.Add(1) == 1 {
			h.Set("Cache-Control", "public, no-cache")
			return artworkHTTPResponse(remote, h), nil
		}
		if req.Header.Get("If-None-Match") != `"fallback"` {
			t.Error("download fallback failed to send the cached HTTP validator")
		}
		h.Set("Cache-Control", "public, max-age=3600")
		return &http.Response{StatusCode: http.StatusNotModified, Header: h, Body: http.NoBody}, nil
	})
	sourceArtworkFrame(c, raw, 128, artPrimary)
	c.workers.Wait()
	requireArtworkSourceWidth(t, sourceArtworkFrame(c, raw, 128, artPrimary), 11)
	if err := m.Remove(artworkSourceID); err != nil {
		t.Fatal(err)
	}
	sourceArtworkFrame(c, raw, 128, artPrimary)
	c.workers.Wait()
	requireArtworkSourceWidth(t, sourceArtworkFrame(c, raw, 128, artPrimary), 51)
	if c.peek(raw, 128) != nil {
		t.Fatal("unvalidated fallback was reusable")
	}
	f := c.beginFrame()
	f.submit()
	sourceArtworkFrame(c, raw, 128, artPrimary)
	c.workers.Wait()
	requireArtworkSourceWidth(t, c.peek(raw, 128), 51)
	requireArtworkSourceWidth(t, sourceArtworkFrame(c, raw, 384, artPrimary), 51)
	c.workers.Wait()
	if requests.Load() != 2 {
		t.Fatalf("expected one HTTP body and one validation, got %d requests", requests.Load())
	}
}
