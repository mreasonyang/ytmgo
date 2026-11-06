package catalog

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"
)

func playlistContinuationItem(token string) object {
	return object{"continuationItemRenderer": object{"continuationEndpoint": object{"continuationCommand": object{"token": token}}}}
}

// Mirrors the playlist's two independent streams: the section list continues
// into recommendations, while the nested playlist shelf continues its tracks.
func playlistPaginationResponse() object {
	return object{"contents": object{"twoColumnBrowseResultsRenderer": object{
		"secondaryContents": object{"sectionListRenderer": object{
			"continuations": []any{object{"nextContinuationData": object{"continuation": "synthetic-recommendations"}}},
			"contents": []any{object{"musicPlaylistShelfRenderer": object{"contents": []any{
				responsiveSong("abc123DEF_-", "First"), playlistContinuationItem("synthetic-tracks-2"),
			}}}},
		}},
	}}}
}

func TestPlaylistTrackCursorMustTakePriorityOverSectionCursor(t *testing.T) {
	page, cursor, err := parsePage(playlistPaginationResponse())
	if err != nil || len(page.Items) != 1 {
		t.Fatal("playlist fixture did not parse")
	}
	if cursor.token != "synthetic-tracks-2" || cursor.query || !cursor.trackShelf {
		t.Fatal("playlist selected the section cursor instead of its track cursor")
	}
}

func TestPlaylistContinuationMustRequestTrackPage(t *testing.T) {
	client := fixtureClient(t, func(w http.ResponseWriter, req *http.Request) {
		var body object
		_ = json.NewDecoder(req.Body).Decode(&body)
		switch {
		case textAt(body, "continuation") == "synthetic-tracks-2":
			_ = json.NewEncoder(w).Encode(object{"continuationContents": object{"musicPlaylistShelfContinuation": object{
				"contents": []any{responsiveSong("def456ABC_-", "Second")},
			}}})
		case req.URL.Query().Get("continuation") == "synthetic-recommendations":
			_ = json.NewEncoder(w).Encode(catalogResponse(responsiveSong("rec123ABC_-", "Recommendation")))
		default:
			_ = json.NewEncoder(w).Encode(playlistPaginationResponse())
		}
	})
	page, err := client.Browse(context.Background(), "VLsynthetic")
	if err != nil || page.Continuation == "" {
		t.Fatal("playlist fixture has no continuation")
	}
	next, err := client.Continue(context.Background(), page.Continuation)
	if err != nil || len(next.Items) != 1 || next.Items[0].ID != "def456ABC_-" {
		t.Fatal("playlist continuation returned recommendations instead of subsequent tracks")
	}
}

func setPlaylistCursor(shelf object, token string, query bool) {
	if query {
		shelf["continuations"] = []any{object{"nextContinuationData": object{"continuation": token}}}
	} else {
		shelf["contents"] = append(shelf["contents"].([]any), playlistContinuationItem(token))
	}
}

func TestPlaylistDeferredSectionKeepsRequestContextAndRetry(t *testing.T) {
	for _, trackQuery := range []bool{false, true} {
		for _, sectionQuery := range []bool{false, true} {
			t.Run(fmt.Sprintf("tracks-query=%v/section-query=%v", trackQuery, sectionQuery), func(t *testing.T) {
				attempts := map[string]int{}
				client := fixtureClient(t, func(w http.ResponseWriter, req *http.Request) {
					var body object
					_ = json.NewDecoder(req.Body).Decode(&body)
					query := req.URL.Query().Get("continuation")
					token := query
					if token == "" {
						token = textAt(body, "continuation")
					}
					attempts[token]++
					if req.URL.Path != "/youtubei/v1/browse" {
						t.Error("playlist changed endpoint")
					}
					if token == "" {
						response := playlistPaginationResponse()
						section := at(response, "contents", "twoColumnBrowseResultsRenderer", "secondaryContents", "sectionListRenderer").(object)
						delete(section, "continuations")
						setPlaylistCursor(section, "synthetic-recommendations", sectionQuery)
						shelf := section["contents"].([]any)[0].(object)["musicPlaylistShelfRenderer"].(object)
						shelf["contents"] = []any{responsiveSong("abc123DEF_-", "First")}
						setPlaylistCursor(shelf, "synthetic-tracks-2", trackQuery)
						response["responseContext"] = object{"visitorData": "synthetic-original-visitor"}
						_ = json.NewEncoder(w).Encode(response)
						return
					}
					isTrack := strings.HasPrefix(token, "synthetic-tracks-")
					wantQuery := sectionQuery
					if isTrack {
						wantQuery = trackQuery
					}
					if (query != "") != wantQuery || wantQuery && (req.URL.Query().Get("ctoken") != token || textAt(body, "browseId") != "VLsynthetic") {
						t.Error("cursor lost its query/body request mode or original browse payload")
					}
					wantVisitor := "synthetic-original-visitor"
					if token == "synthetic-tracks-3" {
						wantVisitor = "synthetic-track-visitor"
					} else if token == "synthetic-recommendations-2" {
						wantVisitor = "synthetic-recommendation-visitor"
					}
					if textAt(body, "context", "client", "visitorData") != wantVisitor || req.Header.Get("X-Goog-Visitor-Id") != wantVisitor {
						t.Error("cursor inherited another stream's visitor")
					}
					if (token == "synthetic-tracks-2" || token == "synthetic-recommendations") && attempts[token] == 1 {
						w.WriteHeader(http.StatusServiceUnavailable)
						return
					}
					switch token {
					case "synthetic-tracks-2":
						unplayable := responsiveSong("bad123ABC_-", "Unavailable")
						unplayable["musicResponsiveListItemRenderer"].(object)["isPlayable"] = false
						shelf := object{"contents": []any{unplayable}}
						setPlaylistCursor(shelf, "synthetic-tracks-3", trackQuery)
						_ = json.NewEncoder(w).Encode(object{"responseContext": object{"visitorData": "synthetic-track-visitor"}, "continuationContents": object{"musicPlaylistShelfContinuation": shelf}})
					case "synthetic-tracks-3":
						_ = json.NewEncoder(w).Encode(object{"continuationContents": object{"musicPlaylistShelfContinuation": object{"contents": []any{responsiveSong("def456ABC_-", "Last track")}}}})
					case "synthetic-recommendations":
						shelf := object{"title": words("Recommended songs"), "contents": []any{responsiveSong("rec123ABC_-", "Recommendation")}}
						setPlaylistCursor(shelf, "synthetic-recommendations-2", sectionQuery)
						_ = json.NewEncoder(w).Encode(object{"responseContext": object{"visitorData": "synthetic-recommendation-visitor"}, "continuationContents": object{"sectionListContinuation": object{"contents": []any{object{"musicShelfRenderer": shelf}}}}})
					case "synthetic-recommendations-2":
						_ = json.NewEncoder(w).Encode(catalogResponse(responsiveSong("rec456ABC_-", "Last recommendation")))
					default:
						t.Error("unexpected synthetic cursor")
						w.WriteHeader(http.StatusBadRequest)
					}
				})
				ctx := context.Background()
				page, err := client.Browse(ctx, "VLsynthetic")
				if err != nil {
					t.Fatal("initial playlist request failed")
				}
				original := page.Continuation
				for i, wantID := range []string{"", "def456ABC_-", "rec123ABC_-", "rec456ABC_-"} {
					handle := page.Continuation
					if handle == "" {
						t.Fatalf("stream ended before page %d", i+2)
					}
					if i == 0 || i == 2 {
						if _, err := client.Continue(ctx, handle); !errors.Is(err, ErrUnavailable) {
							t.Fatal("synthetic failure did not remain retryable")
						}
					}
					page, err = client.Continue(ctx, handle)
					if err != nil || wantID == "" && len(page.Items) != 0 || wantID != "" && (len(page.Items) != 1 || page.Items[0].ID != wantID) {
						t.Fatalf("wrong stream order or lost empty-page cursor at page %d", i+2)
					}
				}
				if page.Continuation != "" || attempts["synthetic-recommendations"] != 2 {
					t.Fatal("recommendations did not finish exactly once after retry")
				}
				// Cached/back-navigation handles must retain an immutable branch.
				replayed, err := client.Continue(ctx, original)
				if err != nil || replayed.Continuation == "" {
					t.Fatal("following the stream consumed the original page handle")
				}
			})
		}
	}
}

func TestPlaylistLargePaginationKeepsAllTracksBeforeRecommendations(t *testing.T) {
	for _, count := range []int{127, 2723} {
		t.Run(strconv.Itoa(count), func(t *testing.T) {
			recommendations := 0
			client := fixtureClient(t, func(w http.ResponseWriter, req *http.Request) {
				var body object
				_ = json.NewDecoder(req.Body).Decode(&body)
				if req.URL.Query().Get("continuation") == "synthetic-recommendations" {
					recommendations++
					_ = json.NewEncoder(w).Encode(catalogResponse(responsiveSong("rec123ABC_-", "Recommendation")))
					return
				}
				start, _ := strconv.Atoi(strings.TrimPrefix(textAt(body, "continuation"), "tracks-"))
				var rows []any
				for i := start; i < min(start+100, count); i++ {
					rows = append(rows, responsiveSong(fmt.Sprintf("track%06d", i), fmt.Sprintf("Track %d", i)))
				}
				if start+100 < count {
					rows = append(rows, playlistContinuationItem(fmt.Sprintf("tracks-%d", start+100)))
				}
				if start == 0 {
					response := playlistPaginationResponse()
					section := at(response, "contents", "twoColumnBrowseResultsRenderer", "secondaryContents", "sectionListRenderer").(object)
					section["contents"] = []any{object{"musicPlaylistShelfRenderer": object{"contents": rows}}}
					_ = json.NewEncoder(w).Encode(response)
				} else {
					_ = json.NewEncoder(w).Encode(object{"onResponseReceivedActions": []any{object{"appendContinuationItemsAction": object{"continuationItems": rows}}}})
				}
			})
			ctx := context.Background()
			page, err := client.Browse(ctx, "VLsynthetic")
			tracks := 0
			for pageNumber := 0; ; pageNumber++ {
				if err != nil || pageNumber > count/100+2 {
					t.Fatal("large playlist failed or did not terminate")
				}
				for _, item := range page.Items {
					if tracks < count {
						if item.ID != fmt.Sprintf("track%06d", tracks) || recommendations != 0 {
							t.Fatal("recommendations interrupted the main track sequence")
						}
						tracks++
					} else if item.ID != "rec123ABC_-" {
						t.Fatal("unexpected trailing playlist item")
					}
				}
				if page.Continuation == "" {
					break
				}
				page, err = client.Continue(ctx, page.Continuation)
			}
			if tracks != count || recommendations != 1 {
				t.Fatalf("got %d/%d tracks and %d recommendation requests", tracks, count, recommendations)
			}
		})
	}
}

func TestPlaylistDeferredContinuationRespectsSessionAndExpiry(t *testing.T) {
	for _, change := range []string{"clear", "replace", "expire-handle"} {
		t.Run(change, func(t *testing.T) {
			calls := 0
			client := fixtureClient(t, func(w http.ResponseWriter, req *http.Request) {
				calls++
				if calls == 1 {
					_ = json.NewEncoder(w).Encode(playlistPaginationResponse())
				} else {
					_ = json.NewEncoder(w).Encode(object{"continuationContents": object{"musicPlaylistShelfContinuation": object{"contents": []any{responsiveSong("def456ABC_-", "Last")}}}})
				}
			})
			if client.SetSession(syntheticCookies(), 0, "") != nil {
				t.Fatal("synthetic session rejected")
			}
			ctx := context.Background()
			page, err := client.Browse(ctx, "VLsynthetic")
			if err != nil {
				t.Fatal("initial playlist failed")
			}
			page, err = client.Continue(ctx, page.Continuation)
			if err != nil || page.Continuation == "" {
				t.Fatal("track exhaustion lost the deferred section")
			}
			switch change {
			case "clear":
				client.ClearSession()
			case "replace":
				if client.SetSession(syntheticCookies(), 1, "") != nil {
					t.Fatal("replacement synthetic session rejected")
				}
			case "expire-handle":
				entry := client.continuations[page.Continuation]
				entry.expires = time.Now().Add(-time.Minute)
				client.continuations[page.Continuation] = entry
			}
			if _, err := client.Continue(ctx, page.Continuation); !errors.Is(err, ErrExpiredContinuation) || calls != 2 {
				t.Fatal("deferred cursor survived invalidation or issued a stale request")
			}
		})
	}
}
