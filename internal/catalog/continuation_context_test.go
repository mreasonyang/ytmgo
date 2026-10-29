package catalog

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

func continuationResponse(token, visitor string, command bool) object {
	response := catalogResponse(responsiveSong("abc123DEF_-", "First"))
	section := at(response, "contents", "singleColumnBrowseResultsRenderer", "tabs").([]any)[0].(object)["tabRenderer"].(object)["content"].(object)["sectionListRenderer"].(object)
	if command {
		section["contents"] = append(section["contents"].([]any), object{"continuationItemRenderer": object{"continuationEndpoint": object{"continuationCommand": object{"token": token}}}})
	} else {
		section["continuations"] = []any{object{"nextContinuationData": object{"continuation": token}}}
	}
	if visitor != "" {
		response["responseContext"] = object{"visitorData": visitor}
	}
	return response
}

func TestContinuationKeepsItsOriginatingVisitorAcrossPages(t *testing.T) {
	for _, command := range []bool{false, true} {
		name := "query"
		if command {
			name = "command"
		}
		t.Run(name, func(t *testing.T) {
			client := fixtureClient(t, func(w http.ResponseWriter, req *http.Request) {
				var payload object
				if json.NewDecoder(req.Body).Decode(&payload) != nil {
					t.Error("invalid fixture request")
					return
				}
				token := req.URL.Query().Get("continuation")
				if command {
					token = textAt(payload, "continuation")
				}
				if token == "" {
					if textAt(payload, "context", "client", "visitorData") != "" || req.Header.Get("X-Goog-Visitor-Id") != "" {
						t.Error("independent page inherited another page's visitor")
					}
					if req.URL.Path == "/youtubei/v1/search" {
						_ = json.NewEncoder(w).Encode(continuationResponse("search-first", "synthetic-search-visitor", command))
					} else {
						_ = json.NewEncoder(w).Encode(continuationResponse("home-first", "synthetic-home-visitor", command))
					}
					return
				}
				want := "synthetic-home-visitor"
				if token == "search-first" {
					want = "synthetic-search-visitor"
				} else if token == "home-third" {
					want = "synthetic-refreshed-visitor"
				}
				if textAt(payload, "context", "client", "visitorData") != want || req.Header.Get("X-Goog-Visitor-Id") != want {
					t.Error("continuation lost its page-bound visitor context")
				}
				switch token {
				case "home-first":
					// The next response need not repeat the visitor. Keep the one
					// used for this successful request for the following page.
					_ = json.NewEncoder(w).Encode(continuationResponse("home-second", "", command))
				case "home-second":
					_ = json.NewEncoder(w).Encode(continuationResponse("home-third", "synthetic-refreshed-visitor", command))
				default:
					_ = json.NewEncoder(w).Encode(catalogResponse(responsiveSong("def456ABC_-", "Last")))
				}
			})
			ctx := context.Background()
			home, err := client.Home(ctx)
			if err != nil {
				t.Fatal("initial home failed")
			}
			search, err := client.Search(ctx, "other page")
			if err != nil {
				t.Fatal("initial search failed")
			}
			for i := 0; i < 3; i++ {
				home, err = client.Continue(ctx, home.Continuation)
				if err != nil || len(home.Items) == 0 {
					t.Fatal("home continuation failed")
				}
			}
			if home.Continuation != "" {
				t.Error("terminal page retained a continuation")
			}
			if _, err = client.Continue(ctx, search.Continuation); err != nil {
				t.Fatal("independent search continuation failed")
			}
		})
	}
}

func TestContinuationRejectsMalformedVisitorData(t *testing.T) {
	for _, visitor := range []string{"line\r\nbreak", "contains space", "非ASCII", strings.Repeat("x", 4097)} {
		calls := 0
		client := fixtureClient(t, func(w http.ResponseWriter, req *http.Request) {
			calls++
			if calls == 1 {
				_ = json.NewEncoder(w).Encode(continuationResponse("synthetic-next", visitor, false))
				return
			}
			var payload object
			_ = json.NewDecoder(req.Body).Decode(&payload)
			if textAt(payload, "context", "client", "visitorData") != "" || req.Header.Get("X-Goog-Visitor-Id") != "" {
				t.Error("malformed visitor was forwarded")
			}
			_ = json.NewEncoder(w).Encode(catalogResponse(responsiveSong("def456ABC_-", "Last")))
		})
		page, err := client.Home(context.Background())
		if err != nil {
			t.Fatal("initial home failed")
		}
		if _, err = client.Continue(context.Background(), page.Continuation); err != nil {
			t.Fatal("continuation failed")
		}
	}
}

// Opt-in metadata-only verification: contrast an unbound request with the
// production request for the same cursor, then follow further bound pages.
func TestLiveHomeContinuationContext(t *testing.T) {
	if os.Getenv("YTMGO_LIVE_CATALOG") != "1" {
		t.Skip("set YTMGO_LIVE_CATALOG=1 for anonymous network verification")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	client := New()
	page, err := client.Home(ctx)
	if err != nil {
		t.Fatalf("home request failed (%T)", err)
	}
	entry, ok := client.continuations[page.Continuation]
	if !ok {
		t.Skip("home has no continuation in this service response")
	}
	t.Logf("initial: shelves=%d items=%d visitor_present=%v", len(page.Shelves), len(page.Items), entry.visitorData != "")
	var unbound map[string]any
	if entry.cursor.query {
		unbound, err = client.apiRequest(ctx, entry.endpoint, entry.payload, entry.cursor.token)
	} else {
		unbound, err = client.api(ctx, entry.endpoint, map[string]any{"continuation": entry.cursor.token})
	}
	if err != nil {
		t.Fatalf("comparison request failed (%T)", err)
	}
	before, next, err := parsePage(unbound)
	if err != nil {
		t.Fatal("comparison parse failed")
	}
	t.Logf("without visitor: shelves=%d items=%d has_next=%v", len(before.Shelves), len(before.Items), next.token != "")
	for i := 1; i <= 3 && page.Continuation != ""; i++ {
		page, err = client.Continue(ctx, page.Continuation)
		if err != nil {
			t.Fatalf("bound continuation failed (%T)", err)
		}
		t.Logf("bound page=%d shelves=%d items=%d has_next=%v", i, len(page.Shelves), len(page.Items), page.Continuation != "")
		if len(page.Items) == 0 && (i == 1 || page.Continuation != "") {
			t.Fatal("bound continuation made no progress")
		}
	}
}
