package catalog

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type object = map[string]any

func words(text string) object { return object{"runs": []any{object{"text": text}}} }
func browseEndpoint(id, kind string) object {
	return object{"browseEndpoint": object{"browseId": id, "browseEndpointContextSupportedConfigs": object{"browseEndpointContextMusicConfig": object{"pageType": kind}}}}
}
func responsiveSong(id, title string) object {
	return object{"musicResponsiveListItemRenderer": object{
		"flexColumns": []any{
			object{"musicResponsiveListItemFlexColumnRenderer": object{"text": object{"runs": []any{object{"text": title, "navigationEndpoint": object{"watchEndpoint": object{"videoId": id}}}}}}},
			object{"musicResponsiveListItemFlexColumnRenderer": object{"text": object{"runs": []any{
				object{"text": "Artist", "navigationEndpoint": browseEndpoint("UC_artist", "MUSIC_PAGE_TYPE_ARTIST")},
				object{"text": " • "},
				object{"text": "Album", "navigationEndpoint": browseEndpoint("MPRE_album", "MUSIC_PAGE_TYPE_ALBUM")},
			}}}},
		},
		"fixedColumns": []any{object{"musicResponsiveListItemFixedColumnRenderer": object{"text": words("3:45")}}},
		"thumbnail": object{"musicThumbnailRenderer": object{"thumbnail": object{"thumbnails": []any{
			object{"url": "https://lh3.googleusercontent.com/small", "width": float64(60), "height": float64(60)},
			object{"url": "https://lh3.googleusercontent.com/large", "width": float64(600), "height": float64(600)},
		}}}},
		"menu": object{"items": []any{object{"musicResponsiveListItemRenderer": object{"videoId": "hidden00000", "title": words("Do not collect")}}}},
	}}
}
func catalogResponse(items ...any) object {
	return object{"contents": object{"singleColumnBrowseResultsRenderer": object{"tabs": []any{
		object{"tabRenderer": object{"selected": true, "content": object{"sectionListRenderer": object{"contents": []any{
			object{"musicShelfRenderer": object{"title": words("Songs"), "contents": items}},
		}}}}},
	}}}}
}
func panelSong(id, title string) object {
	return object{"playlistPanelVideoRenderer": object{"videoId": id, "title": words(title), "longBylineText": words("Artist • Album"), "lengthText": words("3:45")}}
}
func queueResponse(withLyrics bool) object {
	tabs := []any{object{"tabRenderer": object{"content": object{"musicQueueRenderer": object{"content": object{"playlistPanelRenderer": object{"contents": []any{
		panelSong("abc123DEF_-", "First"),
		object{"playlistPanelVideoWrapperRenderer": object{"primaryRenderer": panelSong("def456ABC_-", "Second"), "counterpart": []any{object{"counterpartRenderer": panelSong("hidden00000", "Counterpart")}}}},
	}}}}}}}}
	if withLyrics {
		tabs = append(tabs, object{"tabRenderer": object{"endpoint": browseEndpoint("MPLYt_lyrics", "MUSIC_PAGE_TYPE_TRACK_LYRICS")}})
	}
	return object{"contents": object{"singleColumnMusicWatchNextResultsRenderer": object{"tabbedRenderer": object{"watchNextTabbedResultsRenderer": object{"tabs": tabs}}}}}
}

func fixtureClient(t *testing.T, handler func(http.ResponseWriter, *http.Request)) *Client {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			_, _ = io.WriteString(w, `{"INNERTUBE_CLIENT_VERSION":"1.20261027.01.00"}`)
			return
		}
		handler(w, r)
	}))
	t.Cleanup(server.Close)
	endpoint, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal("fixture endpoint invalid")
	}
	return NewWithOptions(Options{HTTPClient: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		// Authorization is built only for the real service origin. The fixture
		// transport redirects synthetic requests locally without real credentials.
		local := req.Clone(req.Context())
		address := *req.URL
		address.Scheme, address.Host = endpoint.Scheme, endpoint.Host
		local.URL = &address
		return server.Client().Transport.RoundTrip(local)
	})}})
}

func TestPagePreservesGroupingAndBrowseDestinations(t *testing.T) {
	song := responsiveSong("abc123DEF_-", "First")
	album := object{"musicTwoRowItemRenderer": object{
		"title": words("An album"), "subtitle": words("Album • Artist"), "navigationEndpoint": browseEndpoint("MPRE_album", "MUSIC_PAGE_TYPE_ALBUM"),
		"thumbnailOverlay": object{"musicItemThumbnailOverlayRenderer": object{"content": object{"musicPlayButtonRenderer": object{"playNavigationEndpoint": object{"watchEndpoint": object{"videoId": "wrong000000"}}}}}},
	}}
	response := catalogResponse(song, album, song)
	page, _, err := parsePage(response)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Shelves) != 1 || len(page.Items) != 2 {
		t.Fatalf("got %d shelves and %d items", len(page.Shelves), len(page.Items))
	}
	track := page.Items[0]
	if track.ID != "abc123DEF_-" || track.Artist != "Artist" || track.Album != "Album" || track.Duration != 225*time.Second || track.Artwork != "https://lh3.googleusercontent.com/large" {
		t.Fatal("track display metadata is incomplete")
	}
	if page.Items[1].ID != "" || page.Items[1].BrowseID != "MPRE_album" || page.Items[1].Kind != "album" {
		t.Fatal("album incorrectly became its overlay playback target")
	}
}

func TestCardShelfAndSelectedSearchTab(t *testing.T) {
	card := object{"musicCardShelfRenderer": object{"title": words("Top result"), "subtitle": words("Song • Artist"), "navigationEndpoint": object{"watchEndpoint": object{"videoId": "abc123DEF_-"}}, "contents": []any{responsiveSong("def456ABC_-", "Related")}}}
	response := object{"contents": object{"tabbedSearchResultsRenderer": object{"tabs": []any{
		object{"tabRenderer": object{"selected": false, "content": catalogResponse(responsiveSong("hidden00000", "Hidden"))["contents"]}},
		object{"tabRenderer": object{"selected": true, "content": object{"sectionListRenderer": object{"contents": []any{card}}}}},
	}}}}
	page, _, err := parsePage(response)
	if err != nil || len(page.Items) != 2 || page.Items[0].ID != "abc123DEF_-" {
		t.Fatal("selected search tab or top card not preserved")
	}
}

func TestUnplayableAndMenuTargetsAreNotReturned(t *testing.T) {
	song := responsiveSong("abc123DEF_-", "Unavailable")
	song["musicResponsiveListItemRenderer"].(object)["isPlayable"] = false
	page, _, err := parsePage(catalogResponse(song, object{"menu": responsiveSong("hidden00000", "Hidden")}))
	if err != nil || len(page.Items) != 0 {
		t.Fatal("unplayable or hidden menu target was returned")
	}
}

func TestContinuationUsesLocalHandleAndCorrectEndpoint(t *testing.T) {
	var calls atomic.Int32
	client := fixtureClient(t, func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path != "/youtubei/v1/search" {
			t.Error("continuation changed endpoint")
		}
		var payload object
		_ = json.NewDecoder(req.Body).Decode(&payload)
		if calls.Add(1) == 1 {
			response := catalogResponse(responsiveSong("abc123DEF_-", "First"))
			section := at(response, "contents", "singleColumnBrowseResultsRenderer").(object)["tabs"].([]any)[0].(object)["tabRenderer"].(object)["content"].(object)["sectionListRenderer"].(object)
			section["continuations"] = []any{object{"nextContinuationData": object{"continuation": "synthetic-upstream-token"}}}
			_ = json.NewEncoder(w).Encode(response)
		} else {
			if req.URL.Query().Get("continuation") != "synthetic-upstream-token" || textAt(payload, "query") != "hello" {
				t.Error("continuation payload was lost")
			}
			_ = json.NewEncoder(w).Encode(object{"continuationContents": object{"musicShelfContinuation": object{"contents": []any{responsiveSong("def456ABC_-", "Second")}}}})
		}
	})
	page, err := client.Search(context.Background(), "hello")
	if err != nil {
		t.Fatal(err)
	}
	if page.Continuation == "" || page.Continuation == "synthetic-upstream-token" {
		t.Fatal("continuation must be an opaque local handle")
	}
	next, err := client.Continue(context.Background(), page.Continuation)
	if err != nil || len(next.Items) != 1 || next.Items[0].ID != "def456ABC_-" {
		t.Fatal("continuation page was not parsed")
	}
	if _, err := New().Continue(context.Background(), page.Continuation); !errors.Is(err, ErrExpiredContinuation) {
		t.Fatal("continuation crossed clients")
	}
}

func TestQueueAndLyricsReuseReference(t *testing.T) {
	var nextCalls atomic.Int32
	client := fixtureClient(t, func(w http.ResponseWriter, req *http.Request) {
		switch req.URL.Path {
		case "/youtubei/v1/next":
			nextCalls.Add(1)
			_ = json.NewEncoder(w).Encode(queueResponse(true))
		case "/youtubei/v1/browse":
			_ = json.NewEncoder(w).Encode(object{"contents": object{"sectionListRenderer": object{"contents": []any{object{"musicDescriptionShelfRenderer": object{"description": words("Line one\nLine two")}}}}}})
		default:
			t.Error("unexpected operation")
		}
	})
	items, err := client.Queue(context.Background(), "abc123DEF_-")
	if err != nil || len(items) != 2 {
		t.Fatal("queue wrapper handling failed")
	}
	text, err := client.Lyrics(context.Background(), "abc123DEF_-")
	if err != nil || text != "Line one\nLine two" || nextCalls.Load() != 1 {
		t.Fatal("lyrics text/reference was not preserved")
	}
}

func TestNoLyricsIsNotAProtocolFailure(t *testing.T) {
	client := fixtureClient(t, func(w http.ResponseWriter, req *http.Request) { _ = json.NewEncoder(w).Encode(queueResponse(false)) })
	_, err := client.Lyrics(context.Background(), "abc123DEF_-")
	if !errors.Is(err, ErrNoLyrics) {
		t.Fatal("missing lyrics must have a specific error")
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func TestTransportFailuresAreSanitized(t *testing.T) {
	client := NewWithOptions(Options{HTTPClient: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("synthetic-secret-bearing-transport-error")
	})}})
	_, err := client.Home(context.Background())
	if err != ErrUnavailable {
		t.Fatal("transport details escaped the catalog error boundary")
	}
}

func TestResponseSizeAndHTTPFailureBounds(t *testing.T) {
	t.Run("oversized", func(t *testing.T) {
		client := fixtureClient(t, func(w http.ResponseWriter, req *http.Request) {
			w.Header().Set("Content-Length", fmt.Sprint(maxResponseBytes+1))
			w.WriteHeader(http.StatusOK)
		})
		if _, err := client.Home(context.Background()); !errors.Is(err, ErrResponseTooLarge) {
			t.Fatal("response size was not bounded")
		}
	})
	t.Run("http-error", func(t *testing.T) {
		client := fixtureClient(t, func(w http.ResponseWriter, req *http.Request) {
			w.WriteHeader(http.StatusForbidden)
			_, _ = io.WriteString(w, "synthetic-private-upstream-body")
		})
		_, err := client.Home(context.Background())
		var httpError *HTTPError
		if !errors.As(err, &httpError) || httpError.StatusCode != 403 || strings.Contains(err.Error(), "private") {
			t.Fatal("HTTP failure did not preserve only safe metadata")
		}
	})
}

func TestCancellationAndInputValidation(t *testing.T) {
	client := NewWithOptions(Options{HTTPClient: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		<-req.Context().Done()
		return nil, req.Context().Err()
	})}})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := client.Home(ctx); !errors.Is(err, context.Canceled) {
		t.Fatal("cancellation was lost")
	}
	if _, err := client.Search(ctx, " "); !errors.Is(err, ErrEmptyQuery) {
		t.Fatal("empty query accepted")
	}
	if _, err := client.Search(ctx, strings.Repeat("界", 201)); !errors.Is(err, ErrQueryTooLong) {
		t.Fatal("oversized query accepted")
	}
	if _, err := client.Queue(ctx, "https://example.com/"); !errors.Is(err, ErrInvalidID) {
		t.Fatal("invalid video ID accepted")
	}
	if _, err := client.Browse(ctx, "../../private"); !errors.Is(err, ErrInvalidID) {
		t.Fatal("invalid browse ID accepted")
	}
}

func TestArtworkURLBoundary(t *testing.T) {
	for _, raw := range []string{"https://googleusercontent.com.evil.test/x", "http://lh3.googleusercontent.com/x", "https://name:secret@lh3.googleusercontent.com/x", "https://lh3.googleusercontent.com:444/x", "https://lh3.googleusercontent.com/x?signature=synthetic"} {
		if safeArtwork(raw) != "" {
			t.Fatal("unsafe artwork address accepted")
		}
	}
	if safeArtwork("https://i.ytimg.com/vi/abc123DEF_-/hqdefault.jpg") == "" {
		t.Fatal("public image address rejected")
	}
	if got := safeArtwork("https://i.ytimg.com/vi/abc123DEF_-/hqdefault.jpg?sqp=synthetic&rs=synthetic"); got != "https://i.ytimg.com/vi/abc123DEF_-/hqdefault.jpg" {
		t.Fatal("public video thumbnail did not discard tracking query fields")
	}
}

func TestTransferredSessionAndLike(t *testing.T) {
	var mutations atomic.Int32
	client := fixtureClient(t, func(w http.ResponseWriter, req *http.Request) {
		if req.Header.Get("Authorization") == "" || req.Header.Get("Cookie") == "" || req.Header.Get("X-Goog-Authuser") != "0" {
			t.Error("transferred session not applied")
		}
		if strings.Contains(req.Header.Get("Cookie"), "unrelated") {
			t.Error("unrelated domain cookie included")
		}
		switch req.URL.Path {
		case "/youtubei/v1/browse":
			_ = json.NewEncoder(w).Encode(catalogResponse())
		case "/youtubei/v1/like/like", "/youtubei/v1/like/removelike":
			mutations.Add(1)
			_ = json.NewEncoder(w).Encode(object{"responseContext": object{}})
		default:
			t.Error("unexpected account operation")
		}
	})
	if _, err := client.Library(context.Background()); !errors.Is(err, ErrAuthenticationRequired) {
		t.Fatal("anonymous library request was sent")
	}
	if err := client.SetSession([]*http.Cookie{{Name: "unrelated", Value: "fixture", Domain: "example.com"}}, 0, ""); !errors.Is(err, ErrInvalidSession) {
		t.Fatal("incomplete session accepted")
	}
	if err := client.SetSession([]*http.Cookie{
		{Name: "SAPISID", Value: "synthetic-test-only", Domain: ".youtube.com", Secure: true},
		{Name: "unrelated", Value: "fixture", Domain: "example.com"},
	}, 0, ""); err != nil {
		t.Fatal(err)
	}
	if _, ok := client.Credentials(); !ok {
		t.Fatal("session was not accepted")
	}
	if _, err := client.Library(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := client.Like(context.Background(), "abc123DEF_-", true); err != nil {
		t.Fatal(err)
	}
	if err := client.Like(context.Background(), "abc123DEF_-", false); err != nil {
		t.Fatal(err)
	}
	if mutations.Load() != 2 {
		t.Fatal("like operations not delivered")
	}
	client.ClearSession()
	if _, ok := client.Credentials(); ok {
		t.Fatal("sign-out did not clear account session")
	}
	if err := client.Like(context.Background(), "abc123DEF_-", true); !errors.Is(err, ErrAuthenticationRequired) {
		t.Fatal("mutation allowed after sign-out")
	}
}

func TestLanguageChangeKeepsSessionAndSnapshotsRequest(t *testing.T) {
	client := fixtureClient(t, func(w http.ResponseWriter, req *http.Request) {
		var payload object
		_ = json.NewDecoder(req.Body).Decode(&payload)
		if textAt(payload, "context", "client", "hl") != "zh-CN" || req.Header.Get("Accept-Language") != "zh-CN" {
			t.Error("request locale was not applied")
		}
		if req.Header.Get("Authorization") == "" {
			t.Error("language change discarded session")
		}
		_ = json.NewEncoder(w).Encode(catalogResponse())
	})
	if err := client.SetSession([]*http.Cookie{{Name: "SAPISID", Value: "synthetic-test-only", Domain: ".youtube.com", Secure: true}}, 0, ""); err != nil {
		t.Fatal(err)
	}
	client.SetLanguage("zh-CN")
	if _, err := client.Home(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestBrowseParametersPreserveGenreNavigation(t *testing.T) {
	button := func(title, params string) object {
		return object{"musicNavigationButtonRenderer": object{"buttonText": words(title), "clickCommand": object{"browseEndpoint": object{"browseId": "FEmusic_moods_and_genres_category", "params": params}}}}
	}
	page, _, err := parsePage(catalogResponse(button("Jazz", "genre_one"), button("Rock", "genre_two")))
	if err != nil || len(page.Items) != 2 || page.Items[0].BrowseParams != "genre_one" {
		t.Fatal("genre navigation parameters or distinct destinations were lost")
	}
	client := fixtureClient(t, func(w http.ResponseWriter, req *http.Request) {
		var payload object
		_ = json.NewDecoder(req.Body).Decode(&payload)
		if textAt(payload, "params") != "genre_one" {
			t.Error("genre request lost its parameters")
		}
		_ = json.NewEncoder(w).Encode(catalogResponse())
	})
	if _, err := client.BrowseWithParams(context.Background(), page.Items[0].BrowseID, page.Items[0].BrowseParams); err != nil {
		t.Fatal(err)
	}
}

func TestTrackRatingControls(t *testing.T) {
	videoID := "abc123DEF_-"
	button := func(status, target string) object {
		return object{"topLevelButtons": []any{object{"likeButtonRenderer": object{"likeStatus": status, "target": object{"videoId": target}}}}}
	}
	toggle := func(status, icon string, toggled bool) object {
		return object{"items": []any{object{"toggleMenuServiceItemRenderer": object{
			"defaultIcon": object{"iconType": icon}, "isToggled": toggled,
			"defaultServiceEndpoint": object{"likeEndpoint": object{"status": status, "target": object{"videoId": videoID}}},
			"toggledServiceEndpoint": object{"likeEndpoint": object{"status": "INDIFFERENT", "target": object{"videoId": videoID}}},
		}}}}
	}
	for _, test := range []struct {
		name         string
		menu         object
		liked, known bool
	}{
		{"liked button", button("LIKE", videoID), true, true},
		{"unliked button", button("INDIFFERENT", videoID), false, true},
		{"disliked button", button("DISLIKE", videoID), false, true},
		{"unrelated button", button("LIKE", "def456ABC_-"), false, false},
		{"queue unlike action", toggle("INDIFFERENT", "LIKE", false), true, true},
		{"queue like action", toggle("LIKE", "LIKE", false), false, true},
		{"queue toggled action", toggle("LIKE", "LIKE", true), true, true},
		{"dislike control", toggle("INDIFFERENT", "DISLIKE", false), false, false},
		{"omitted status", object{}, false, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			renderer := responsiveSong(videoID, "Song")["musicResponsiveListItemRenderer"].(object)
			renderer["menu"] = object{"menuRenderer": test.menu}
			item, ok := parseItem(renderer, "musicResponsiveListItemRenderer")
			if !ok || item.Liked != test.liked || item.LikeKnown != test.known {
				t.Fatal("rating control did not preserve current liked/known state")
			}
		})
	}
}

func TestTrackFetchesRatingFromNext(t *testing.T) {
	client := fixtureClient(t, func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path != "/youtubei/v1/next" {
			t.Error("track refresh did not use next")
		}
		response := queueResponse(true)
		tabs := arrayAt(response, "contents", "singleColumnMusicWatchNextResultsRenderer", "tabbedRenderer", "watchNextTabbedResultsRenderer", "tabs")
		items := arrayAt(tabs[0], "tabRenderer", "content", "musicQueueRenderer", "content", "playlistPanelRenderer", "contents")
		renderer := at(items[0], "playlistPanelVideoRenderer").(object)
		renderer["menu"] = object{"menuRenderer": object{"items": []any{object{"toggleMenuServiceItemRenderer": object{
			"defaultServiceEndpoint": object{"likeEndpoint": object{"status": "INDIFFERENT", "target": object{"videoId": "abc123DEF_-"}}},
		}}}}}
		_ = json.NewEncoder(w).Encode(response)
	})
	item, err := client.Track(context.Background(), "abc123DEF_-")
	if err != nil || !item.Liked || !item.LikeKnown || item.ID != "abc123DEF_-" {
		t.Fatal("selected track rating was not refreshed from queue")
	}
}

// Opt in explicitly: YTMGO_LIVE_CATALOG=1 go test -run TestLiveCatalog -v ./internal/catalog
// This test is anonymous and prints counts only. It never signs in, mutates an
// account, saves server responses, or emits lyric text/URLs.
func TestLiveCatalog(t *testing.T) {
	if os.Getenv("YTMGO_LIVE_CATALOG") != "1" {
		t.Skip("set YTMGO_LIVE_CATALOG=1 for anonymous network verification")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	client := New()
	search, err := client.Search(ctx, "Daft Punk Get Lucky")
	if err != nil {
		t.Fatal(err)
	}
	if len(search.Items) == 0 {
		t.Fatal("search returned no catalog items")
	}
	videoID := ""
	for _, item := range search.Items {
		if item.ID != "" {
			videoID = item.ID
			break
		}
	}
	if videoID == "" {
		t.Fatal("search returned no playable track")
	}
	t.Logf("search: %d shelves, %d items", len(search.Shelves), len(search.Items))
	for name, fetch := range map[string]func(context.Context) (Page, error){"home": client.Home, "explore": client.Explore} {
		page, err := fetch(ctx)
		if err != nil {
			t.Fatal(name, err)
		}
		if len(page.Items) == 0 {
			t.Fatalf("%s returned no catalog items", name)
		}
		t.Logf("%s: %d shelves, %d items", name, len(page.Shelves), len(page.Items))
		if page.Continuation != "" {
			next, err := client.Continue(ctx, page.Continuation)
			if err != nil {
				t.Fatal(name, "continuation", err)
			}
			if len(next.Items) == 0 {
				t.Fatal(name, "continuation returned no items")
			}
			t.Logf("%s continuation: %d shelves, %d items", name, len(next.Shelves), len(next.Items))
		}
		if name == "home" {
			for _, item := range page.Items {
				if item.BrowseID != "" && (item.Kind == "album" || item.Kind == "playlist") {
					detail, err := client.Browse(ctx, item.BrowseID)
					if err != nil {
						t.Fatal("browse", err)
					}
					if len(detail.Items) == 0 {
						t.Fatal("browse returned no catalog items")
					}
					t.Logf("browse: %d shelves, %d items", len(detail.Shelves), len(detail.Items))
					break
				}
			}
		}
	}
	queue, err := client.Queue(ctx, videoID)
	if err != nil {
		t.Fatal(err)
	}
	if len(queue) == 0 {
		t.Fatal("queue returned no tracks")
	}
	t.Logf("queue: %d tracks", len(queue))
	knownRatings := 0
	for _, item := range queue {
		if item.LikeKnown {
			knownRatings++
		}
	}
	t.Logf("queue rating controls: %d known states (anonymous request)", knownRatings)
	lyrics, err := client.Lyrics(ctx, videoID)
	if err != nil {
		t.Fatal("vocal lyrics", err)
	}
	if len(lyrics) == 0 {
		t.Fatal("lyrics were empty")
	}
	t.Log("lyrics: available (content not logged)")
}
