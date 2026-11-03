package catalog

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"
)

// Anonymous structural probe: no account stores, upstream bodies, URLs or
// credentials are read into evidence. Only public field-presence counts leave
// this test. It also verifies the album result used by the native application.
func TestLiveAlbumTrackMetadata(t *testing.T) {
	if os.Getenv("YTMGO_LIVE_ALBUM_METADATA") != "1" {
		t.Skip("set YTMGO_LIVE_ALBUM_METADATA=1 for anonymous album metadata acceptance")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	for index, target := range []struct{ query, title string }{
		{"Derik Fein Antidote", "Antidote"},
		{"Daft Punk Random Access Memories", "Random Access Memories"},
	} {
		client := NewWithOptions(Options{Language: "en"})
		search, err := client.Search(ctx, target.query)
		if err != nil {
			t.Fatal("album_metadata_search_failed")
		}
		var album Item
		for _, item := range search.Items {
			if item.Kind == "album" && strings.EqualFold(item.Title, target.title) {
				album = item
				break
			}
		}
		if album.BrowseID == "" {
			t.Fatal("album_metadata_album_missing")
		}
		response, err := client.api(ctx, "browse", map[string]any{"browseId": album.BrowseID})
		if err != nil {
			t.Fatal("album_metadata_browse_failed")
		}
		row := struct {
			Album, RawRows, RawArtwork, RawArtist, ParsedTracks, ParsedArtwork, ParsedArtist int
			HeaderArtwork, HeaderCreator, CanonicalAlbumID, FirstShelfUntitled               bool
			FirstShelfTracks, IndexedRows                                                    int
			RawVideoRows                                                                     int
		}{Album: index + 1}
		var walk func(any, int)
		walk = func(value any, depth int) {
			if depth > 40 {
				return
			}
			switch node := value.(type) {
			case []any:
				for _, v := range node {
					walk(v, depth+1)
				}
			case map[string]any:
				if r, ok := node["musicResponsiveListItemRenderer"].(map[string]any); ok {
					if item, valid := parseItem(r, "musicResponsiveListItemRenderer"); valid && item.ID != "" {
						row.RawRows++
						if item.Kind == "video" {
							row.RawVideoRows++
						}
						if r["index"] != nil {
							row.IndexedRows++
						}
						if item.Artwork != "" {
							row.RawArtwork++
						}
						if item.Artist != "" {
							row.RawArtist++
						}
					}
					return
				}
				for key, v := range node {
					if key != "menu" && key != "responseContext" && key != "trackingParams" && key != "loggingDirectives" {
						walk(v, depth+1)
					}
				}
			}
		}
		walk(response["contents"], 0)
		page, err := client.BrowseWithParams(ctx, album.BrowseID, album.BrowseParams)
		if err != nil {
			t.Fatal("album_metadata_parse_failed")
		}
		row.HeaderArtwork, row.HeaderCreator = page.Artwork != "", page.Creator != ""
		row.CanonicalAlbumID = strings.HasPrefix(album.BrowseID, "MPRE")
		if len(page.Shelves) > 0 {
			row.FirstShelfUntitled = page.Shelves[0].Title == ""
			for _, item := range page.Shelves[0].Items {
				if item.ID != "" {
					row.FirstShelfTracks++
				}
			}
		}
		for _, item := range page.Items {
			if item.ID == "" {
				continue
			}
			row.ParsedTracks++
			if item.Artwork != "" {
				row.ParsedArtwork++
			}
			if item.Artist != "" {
				row.ParsedArtist++
			}
		}
		data, _ := json.Marshal(row)
		t.Log(string(data))
		if row.ParsedTracks == 0 || !row.HeaderArtwork || !row.HeaderCreator {
			t.Fatal("album_metadata_fixture_unavailable")
		}
		if row.ParsedArtwork != row.ParsedTracks || row.ParsedArtist != row.ParsedTracks {
			t.Error("album_track_metadata_incomplete")
		}
	}
}
