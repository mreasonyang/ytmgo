package main

import (
	"bytes"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/egoist/mygo/ui"
	"ytmgo/internal/catalog"
)

const playlistCoverFixtureBase = "https://i.ytimg.com/pl_c/PLSyntheticCover_0123456789abcdefXYZ/studio_square_thumbnail.jpg"
const playlistCoverFixtureURL = playlistCoverFixtureBase + "?sqp=c3ludGhldGljLTU0NA&rs=c2lnbmF0dXJl"

func playlistCoverFixtureResponse() []byte {
	words := func(s string) any { return map[string]any{"runs": []any{map[string]any{"text": s}}} }
	item := map[string]any{"musicTwoRowItemRenderer": map[string]any{
		"title": words("Weekend discoveries"), "subtitle": words("Fixture Curator"),
		"navigationEndpoint": map[string]any{"browseEndpoint": map[string]any{"browseId": "VLPLSyntheticCover_0123456789abcdefXYZ", "browseEndpointContextSupportedConfigs": map[string]any{"browseEndpointContextMusicConfig": map[string]any{"pageType": "MUSIC_PAGE_TYPE_PLAYLIST"}}}},
		"thumbnailRenderer": map[string]any{"musicThumbnailRenderer": map[string]any{"thumbnail": map[string]any{"thumbnails": []any{
			map[string]any{"url": playlistCoverFixtureURL, "width": 544, "height": 544},
		}}}},
	}}
	response := map[string]any{"contents": map[string]any{"sectionListRenderer": map[string]any{"contents": []any{map[string]any{"musicShelfRenderer": map[string]any{"title": words("Forgotten favorites"), "contents": []any{item}}}}}}}
	data, _ := json.Marshal(response)
	return data
}

func playlistCoverTester(c *artworkCache, item catalog.Item, scale float32) (*ui.Tester, *UIModel, ViewActions) {
	m, actions := nativeViewFixture()
	m.Current, m.Queue = catalog.Item{}, nil
	m.Page = catalog.Page{Title: "Home", Shelves: []catalog.Shelf{{Title: "Forgotten favorites", Items: []catalog.Item{item}}}}
	actions.ArtworkTint, actions.artworkPeek = c.tint, c.peek
	actions.artworkScale = float64(scale)
	tt := ui.NewTester(func(ctx *ui.Context) {
		frame := artworkFrame{cache: c}
		actions.Artwork = frame.get
		nativeView(ctx, m, actions)
		if frame.submit() {
			ctx.Invalidate()
		}
	}, 1440, 940)
	tt.SetScale(scale)
	return tt, m, actions
}

func playlistCoverPixel(t *testing.T, tt *ui.Tester, item catalog.Item, m *UIModel, actions ViewActions) color.RGBA {
	t.Helper()
	r, ok := tt.Find(nativeResultLabel(item, m, actions))
	if !ok || r.W <= 0 || r.H <= 0 {
		t.Fatal("public playlist card is not visible")
	}
	scale := float32(tt.Image().Bounds().Dx()) / 1440
	return tt.Image().RGBAAt(int((r.X+12+nativeCardArt(false)/2)*scale), int((r.Y+12+nativeCardArt(false)/2)*scale))
}

func TestPlaylistCoverCatalogToVisibleHomeCard(t *testing.T) {
	client := catalog.NewWithOptions(catalog.Options{HTTPClient: &http.Client{Transport: artworkTransport(func(req *http.Request) (*http.Response, error) {
		if req.Header.Get("Cookie") != "" || req.Header.Get("Authorization") != "" {
			t.Error("anonymous fixture carried account authorization")
		}
		return artworkHTTPResponse(playlistCoverFixtureResponse(), make(http.Header)), nil
	})}})
	page, err := client.Search(t.Context(), "Weekend discoveries Fixture Curator")
	if err != nil || len(page.Items) != 1 {
		t.Fatal("public playlist fixture was not parsed")
	}
	item := page.Items[0]
	im := image.NewRGBA(image.Rect(0, 0, 384, 384))
	want := color.RGBA{210, 85, 45, 255}
	for y := range 384 {
		for x := range 384 {
			im.SetRGBA(x, y, want)
		}
	}
	var encoded bytes.Buffer
	if png.Encode(&encoded, im) != nil {
		t.Fatal("cover fixture encode failed")
	}
	c := artworkTestCache(t, nil)
	dir := t.TempDir()
	c.disk = &artworkDisk{dir: dir}
	var bodies atomic.Int32
	upstream := artworkTransport(func(req *http.Request) (*http.Response, error) {
		if req.URL.String() != playlistCoverFixtureURL || req.Header.Get("Cookie") != "" || req.Header.Get("Authorization") != "" {
			t.Error("playlist image request changed its rendition or carried account authorization")
		}
		bodies.Add(1)
		return artworkHTTPResponse(encoded.Bytes(), http.Header{"Cache-Control": {"public, max-age=3600"}}), nil
	})
	c.client.Transport = upstream
	tt, model, actions := playlistCoverTester(c, item, 1)
	c.workers.Wait()
	for range 20 {
		tt.Frame()
	}
	painted := playlistCoverPixel(t, tt, item, model, actions) == want
	if item.Artwork != playlistCoverFixtureURL || bodies.Load() != 1 || !painted {
		t.Fatalf("visible playlist stayed blank: artwork_present=%t image_requests=%d pixels_painted=%t", item.Artwork != "", bodies.Load(), painted)
	}
	for range 5 {
		model.Page = catalog.Page{Title: "Other page"}
		tt.Frame()
		model.Page = catalog.Page{Title: "Home", Shelves: []catalog.Shelf{{Title: "Forgotten favorites", Items: []catalog.Item{item}}}}
		tt.Frame()
		if playlistCoverPixel(t, tt, item, model, actions) != want {
			t.Fatal("warm Home return blanked the playlist cover")
		}
	}
	loadHTTPArtwork(t, dir, item.Artwork, upstream)
	if bodies.Load() != 1 {
		t.Fatal("public playlist rendition did not reuse its persisted body")
	}
}

func TestPlaylistCoverDiskEligibilityMatchesCatalogPolicy(t *testing.T) {
	if artworkDiskKey(playlistCoverFixtureURL) == "" || artworkRequestAt(playlistCoverFixtureURL, 384) != playlistCoverFixtureURL {
		t.Fatal("public playlist cover was excluded from the disk cache or rewritten")
	}
	for _, raw := range []string{
		playlistCoverFixtureBase + "?sqp=synthetic",
		playlistCoverFixtureURL + "&rs=other",
		playlistCoverFixtureURL + "&account=synthetic",
		strings.Replace(playlistCoverFixtureURL, "/pl_c/", "/%70l_c/", 1),
		strings.Replace(playlistCoverFixtureURL, "i.ytimg.com", "i.ytimg.com.evil.invalid", 1),
	} {
		if artworkDiskKey(raw) != "" {
			t.Fatal("unsupported playlist rendition passed disk admission")
		}
	}
}
