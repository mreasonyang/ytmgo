package catalog

import (
	"encoding/json"
	"net/http"
	"testing"
)

func TestDownloadCollectionFollowsOnlyTrackCursors(t *testing.T) {
	client := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
		var body object
		_ = json.NewDecoder(r.Body).Decode(&body)
		switch {
		case textAt(body, "browseId") == "VLsynthetic":
			response := playlistPaginationResponse()
			section := at(response, "contents", "twoColumnBrowseResultsRenderer", "secondaryContents", "sectionListRenderer").(object)
			section["contents"] = append(arrayAt(section, "contents"), object{"musicShelfRenderer": object{"contents": []any{responsiveSong("rec123ABC_-", "Recommended song")}}})
			_ = json.NewEncoder(w).Encode(response)
		case textAt(body, "continuation") == "synthetic-tracks-2":
			_ = json.NewEncoder(w).Encode(object{"onResponseReceivedActions": []any{object{"appendContinuationItemsAction": object{"continuationItems": []any{responsiveSong("def456ABC_-", "Final track")}}}}})
		default:
			t.Error("collection downloader requested the recommendations cursor")
			w.WriteHeader(http.StatusBadRequest)
		}
	})
	rows, err := client.CollectionTracks(t.Context(), "VLsynthetic", "")
	if err != nil || len(rows) != 2 || rows[0].Title != "First" || rows[1].Title != "Final track" {
		t.Fatal("collection did not contain exactly its own tracks", err)
	}
}
