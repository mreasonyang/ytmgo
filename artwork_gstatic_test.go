package main

import (
	"bytes"
	"errors"
	"net/http"
	"testing"

	"github.com/bartventer/httpcache/store/driver"
	"ytmgo/internal/downloads"
)

const (
	systemCoverPath = "/youtube/media/ytm/images/pbg/liked-songs-delhi-1200.png"
	systemCoverBare = "https://gstatic.com" + systemCoverPath
	systemCoverWWW  = "https://www.gstatic.com" + systemCoverPath
)

func TestSystemArtworkReloadUsesRedirectTargetOnDisk(t *testing.T) {
	dir := t.TempDir()
	body := artworkTestPNG(t, 96, 64)
	var requests int
	upstream := artworkTransport(func(r *http.Request) (*http.Response, error) {
		requests++
		switch r.URL.String() {
		case systemCoverBare:
			return &http.Response{StatusCode: http.StatusMovedPermanently, Header: http.Header{"Location": {systemCoverWWW}}, Body: http.NoBody}, nil
		case systemCoverWWW:
			return artworkHTTPResponse(body, http.Header{"Cache-Control": {"public, max-age=3600"}}), nil
		default:
			return nil, errors.New("unexpected upstream artwork URL")
		}
	})

	// Seed the disk through the original request path, with a real HTTP redirect.
	legacy := artworkTestCache(t, nil)
	legacy.disk = &artworkDisk{dir: dir}
	legacy.client.Transport = upstream
	got, _ := legacy.download(t.Context(), artDemand{artRequest: artRequest{url: systemCoverBare}})
	if !bytes.Equal(got, body) || requests != 2 {
		t.Fatalf("legacy load did not follow the redirect: requests=%d", requests)
	}
	if _, err := legacy.disk.Get(systemCoverWWW); err != nil {
		t.Fatalf("redirect target was not persisted: %v", err)
	}
	if _, err := legacy.disk.Get(systemCoverBare); !errors.Is(err, driver.ErrNotExist) {
		t.Fatalf("unexpected redirect source record: %v", err)
	}

	c := loadHTTPArtwork(t, dir, systemCoverWWW, upstream)
	key := artRequest{url: systemCoverWWW, edge: 384}
	oldBitmap := c.entry(key).bitmap
	c.mu.Lock()
	evicted := c.pool(key).evictOldest(nil)
	c.mu.Unlock()
	if !evicted || requestTestArtwork(c, systemCoverBare, 384) != nil {
		t.Fatal("reload did not start cold after bitmap eviction")
	}
	c.workers.Wait()
	if requests != 2 {
		t.Fatalf("disk load and reload made %d upstream requests", requests-2)
	}
	gotBitmap := requestTestArtwork(c, systemCoverWWW, 384)
	if gotBitmap == oldBitmap {
		t.Fatal("reload reused the evicted bitmap")
	}
	requireArtworkSourceWidth(t, gotBitmap, 96)
}

func TestSystemArtworkDownloadAliasesSurviveLibraryReopen(t *testing.T) {
	body := artworkTestPNG(t, 96, 64)
	m, root := artworkSourceLibrary(t, map[string][]byte{artworkSourceID: body, artworkOtherID: artworkTestPNG(t, 32, 32)})
	completeArtworkSource(t, m, artworkSourceID, systemCoverBare)
	completeArtworkSource(t, m, artworkOtherID, systemCoverWWW+"#fragment")
	check := func(m *downloads.Manager) {
		index := m.Covers()
		source := index.Source(systemCoverBare)
		if source.Asset == nil || source != index.Source(systemCoverWWW) {
			t.Fatal("both hosts must resolve to the same local asset and generation")
		}
		if got, err := source.Asset.Read(t.Context()); err != nil || !bytes.Equal(got, body) {
			t.Fatalf("the shared source did not read the downloaded cover: %v", err)
		}
		for _, other := range []string{
			systemCoverBare + "?",
			systemCoverBare + "?signature=synthetic",
			systemCoverBare + "#fragment",
			systemCoverWWW + "#fragment",
			"https://gstatic.com/youtube/media/ytm/images/pbg/%6ciked-songs-delhi-1200.png",
			"https://gstatic.com/other/liked-songs-delhi-1200.png",
			"https://images.gstatic.com" + systemCoverPath,
			"https://gstatic.com:443" + systemCoverPath,
		} {
			if index.Source(other) == source || artworkRequestAt(other, 384) != other {
				t.Errorf("URL outside the canonical boundary was aliased: %s", other)
			}
		}
	}
	check(m)
	m.Close()
	reopened, err := downloads.New(t.Context(), root, &snapshotSecrets{}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(reopened.Close)
	check(reopened)
}
