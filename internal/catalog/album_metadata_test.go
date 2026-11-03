package catalog

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
)

func albumMetadataResponse() object {
	bare := responsiveSong("abc123DEF_-", "Album track")
	renderer := bare["musicResponsiveListItemRenderer"].(object)
	renderer["flexColumns"] = renderer["flexColumns"].([]any)[:1]
	delete(renderer, "thumbnail")
	// Album track lists can use official music-video endpoints for the same
	// recording. Their missing shared metadata needs the same album context.
	renderer["navigationEndpoint"] = object{"watchEndpoint": object{"videoId": "abc123DEF_-", "watchEndpointMusicSupportedConfigs": object{"watchEndpointMusicConfig": object{"musicVideoType": "MUSIC_VIDEO_TYPE_OMV"}}}}
	header := object{
		"title": words("Parent album"), "subtitle": words("Album • 2025"),
		"straplineTextOne": object{"runs": []any{object{"text": "Album artist", "navigationEndpoint": browseEndpoint("UC_album_artist", "MUSIC_PAGE_TYPE_ARTIST")}}},
		"thumbnail":        object{"musicThumbnailRenderer": object{"thumbnail": object{"thumbnails": []any{object{"url": "https://lh3.googleusercontent.com/album-cover"}}}}},
	}
	return object{
		"header": object{"musicResponsiveHeaderRenderer": header},
		"contents": object{"sectionListRenderer": object{"contents": []any{
			object{"musicShelfRenderer": object{"contents": []any{bare, responsiveSong("def456ABC_-", "Explicit metadata")}}},
			object{"musicShelfRenderer": object{"title": words("Recommended songs"), "contents": []any{bareAlbumMetadataSong("rec123ABC_-", "Recommendation")}}},
		}}},
	}
}

func bareAlbumMetadataSong(id, title string) object {
	row := responsiveSong(id, title)
	r := row["musicResponsiveListItemRenderer"].(object)
	r["flexColumns"] = r["flexColumns"].([]any)[:1]
	delete(r, "thumbnail")
	return row
}

func TestAlbumTrackMetadataUsesOnlyItsAlbumContext(t *testing.T) {
	for _, id := range []string{"MPRE_album", "VL_playlist", "UC_artist"} {
		t.Run(id, func(t *testing.T) {
			client := fixtureClient(t, func(w http.ResponseWriter, _ *http.Request) {
				_ = json.NewEncoder(w).Encode(albumMetadataResponse())
			})
			page, err := client.Browse(context.Background(), id)
			if err != nil || len(page.Items) != 3 || len(page.Shelves) != 2 {
				t.Fatal("album fixture did not retain its track and recommendation shelves")
			}
			track := page.Items[0]
			if track.Kind != "video" {
				t.Error("album enrichment changed the endpoint media kind")
			}
			if id == "MPRE_album" {
				if track.Artwork != page.Artwork || track.Artist != page.Creator || track.Album != page.Title || track.Artwork == "" {
					t.Error("album track did not inherit missing album display metadata")
				}
				if page.Shelves[0].Items[0] != track {
					t.Error("album shelf and flattened playback item disagree")
				}
			} else if track.Artwork != "" || track.Artist != "" || track.Album != "" {
				t.Error("playlist owner or artist page was attributed to a track")
			}
			if explicit := page.Items[1]; explicit.Artwork != "https://lh3.googleusercontent.com/large" || explicit.Artist != "Artist" || explicit.Album != "Album" {
				t.Error("album context overwrote explicit track metadata")
			}
			if recommended := page.Items[2]; recommended.Artwork != "" || recommended.Artist != "" || recommended.Album != "" {
				t.Error("recommendation inherited the unrelated album metadata")
			}
		})
	}
}

func TestAlbumTrackContinuationKeepsOnlyItsOwnMetadata(t *testing.T) {
	for _, recommendationCursor := range []bool{false, true} {
		client := fixtureClient(t, func(w http.ResponseWriter, req *http.Request) {
			if req.URL.Query().Get("continuation") != "" {
				_ = json.NewEncoder(w).Encode(object{"continuationContents": object{"musicShelfContinuation": object{"contents": []any{bareAlbumMetadataSong("next123AB_-", "Next")}}}})
				return
			}
			response := albumMetadataResponse()
			shelves := at(response, "contents", "sectionListRenderer", "contents").([]any)
			index := 0
			if recommendationCursor {
				index = 1
			}
			shelves[index].(object)["musicShelfRenderer"].(object)["continuations"] = []any{object{"nextContinuationData": object{"continuation": "synthetic-album-next"}}}
			_ = json.NewEncoder(w).Encode(response)
		})
		page, err := client.Browse(context.Background(), "MPRE_album")
		if err != nil || page.Continuation == "" {
			t.Fatal("album continuation fixture failed")
		}
		next, err := client.Continue(context.Background(), page.Continuation)
		if err != nil || len(next.Items) != 1 {
			t.Fatal("album continuation request failed")
		}
		track := next.Items[0]
		if recommendationCursor {
			if track.Artwork != "" || track.Artist != "" || track.Album != "" {
				t.Error("recommendation cursor inherited album context")
			}
		} else if track.Artwork != page.Artwork || track.Artist != page.Creator || track.Album != page.Title {
			t.Error("album continuation lost its originating album context")
		}
	}
}
