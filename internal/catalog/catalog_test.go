package catalog

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// Handwritten fixtures preserve the public renderer shapes. They contain no
// captured HTML, cookies, context, visitor identifiers, or tracking fields.
const searchFixture = `{
 "contents":{"tabbedSearchResultsRenderer":{"tabs":[{"tabRenderer":{"content":{"sectionListRenderer":{"contents":[
  {"musicShelfRenderer":{"contents":[
   {"musicResponsiveListItemRenderer":{
    "playlistItemData":{"videoId":"4D7u5KF7SP8"},
    "flexColumns":[
     {"musicResponsiveListItemFlexColumnRenderer":{"text":{"runs":[{"text":"Get Lucky"}]}}},
     {"musicResponsiveListItemFlexColumnRenderer":{"text":{"runs":[
      {"text":"Song"},{"text":" • "},
      {"text":"Daft Punk","navigationEndpoint":{"browseEndpoint":{"browseId":"UCRr1xG_2WIDs18a6cIiCxeA"}}},
      {"text":" & "},{"text":"Pharrell Williams","navigationEndpoint":{"browseEndpoint":{"browseId":"UCJw8VyO6e3v6S0327AsgwcQ"}}},
      {"text":" • "},{"text":"Random Access Memories","navigationEndpoint":{"browseEndpoint":{"browseId":"MPREbExample","browseEndpointContextSupportedConfigs":{"browseEndpointMusicConfig":{"pageType":"MUSIC_PAGE_TYPE_ALBUM"}}}}}
     ]}}}
    ],
    "fixedColumns":[{"musicResponsiveListItemFixedColumnRenderer":{"text":{"runs":[{"text":"6:10"}]}}}],
    "thumbnail":{"musicThumbnailRenderer":{"thumbnail":{"thumbnails":[
     {"url":"https://lh3.googleusercontent.com/album-small","width":60},
     {"url":"https://lh3.googleusercontent.com/album-large","width":300},
     {"url":"https://invalid.example/tracker","width":2000}
    ]}}}
   }},
   {"musicResponsiveListItemRenderer":{
    "flexColumns":[{"musicResponsiveListItemFlexColumnRenderer":{"text":{"runs":[{"text":"An album"}]}}}],
    "navigationEndpoint":{"browseEndpoint":{"browseId":"MPREbAlbum"}},
    "menu":{"watchEndpoint":{"videoId":"XPFJOMBmn0E"}}
   }},
   {"musicResponsiveListItemRenderer":{
    "playlistItemData":{"videoId":"short"},
    "flexColumns":[{"musicResponsiveListItemFlexColumnRenderer":{"text":{"runs":[{"text":"Invalid ID"}]}}}]
   }},
   {"musicResponsiveListItemRenderer":{
    "playlistItemData":{"videoId":"hcqrXnykSHo"},
    "flexColumns":[
     {"musicResponsiveListItemFlexColumnRenderer":{"text":{"runs":[{"text":"A podcast"}]}}},
     {"musicResponsiveListItemFlexColumnRenderer":{"text":{"runs":[{"text":"Episode"},{"text":" • "},{"text":"Publisher"}]}}}
    ]
   }},
   {"musicResponsiveListItemRenderer":{
    "playlistItemData":{"videoId":"Rgrt_8mXrK8"},"isPlayable":false,
    "flexColumns":[{"musicResponsiveListItemFlexColumnRenderer":{"text":{"runs":[{"text":"Unavailable song"}]}}}]
   }}
  ]}},
  {"musicCarouselShelfRenderer":{"contents":[
   {"musicTwoRowItemRenderer":{
    "title":{"runs":[{"text":"Daft Punk : Get Lucky // Grammys Rehearsal (HD)"}]},
    "subtitle":{"runs":[{"text":"Video"},{"text":" • "},{"text":"Daft Punk – Archives"},{"text":" • "},{"text":"174K views"},{"text":" • "},{"text":"5:40"}]},
    "navigationEndpoint":{"watchEndpoint":{"videoId":"XPFJOMBmn0E","watchEndpointMusicSupportedConfigs":{"watchEndpointMusicConfig":{"musicVideoType":"MUSIC_VIDEO_TYPE_UGC"}}}},
    "thumbnailRenderer":{"musicThumbnailRenderer":{"thumbnail":{"thumbnails":[{"url":"https://i.ytimg.com/vi/XPFJOMBmn0E/hqdefault.jpg","width":480}]}}}
   }},
   {"musicTwoRowItemRenderer":{"title":{"simpleText":"Duplicate"},"navigationEndpoint":{"watchEndpoint":{"videoId":"4D7u5KF7SP8"}}}},
   {"musicTwoRowItemRenderer":{"title":{"simpleText":"Podcast endpoint"},"navigationEndpoint":{"watchEndpoint":{"videoId":"06jpyskDado","watchEndpointMusicSupportedConfigs":{"watchEndpointMusicConfig":{"musicVideoType":"MUSIC_VIDEO_TYPE_PODCAST_EPISODE"}}}}}}
  ]}}
 ]}}}}]}}
}`

func TestParseNestedSongAndVideoResults(t *testing.T) {
	tracks, err := parseSearch([]byte(searchFixture))
	if err != nil {
		t.Fatal(err)
	}
	want := []Track{
		{ID: "4D7u5KF7SP8", Title: "Get Lucky", Artist: "Daft Punk, Pharrell Williams", Album: "Random Access Memories", Duration: "6:10", Thumbnail: "https://lh3.googleusercontent.com/album-large"},
		{ID: "XPFJOMBmn0E", Title: "Daft Punk : Get Lucky // Grammys Rehearsal (HD)", Artist: "Daft Punk – Archives", Duration: "5:40", Thumbnail: "https://i.ytimg.com/vi/XPFJOMBmn0E/hqdefault.jpg"},
	}
	if !reflect.DeepEqual(tracks, want) {
		t.Fatalf("unexpected parsed public metadata:\n%+v", tracks)
	}
}

func TestParseEmptyMalformedAndServiceErrors(t *testing.T) {
	tracks, err := parseSearch([]byte(`{"contents":{"sectionListRenderer":{"contents":[]}}}`))
	if err != nil || tracks == nil || len(tracks) != 0 {
		t.Fatalf("expected a non-nil empty result, got %#v %v", tracks, err)
	}
	for _, data := range []string{`{`, `{}`, `{"contents":null}`, `{"contents":"unexpected"}`, `null`} {
		if _, err := parseSearch([]byte(data)); !errors.Is(err, ErrInvalidResponse) {
			t.Errorf("expected invalid response, got %v", err)
		}
	}
	if _, err := parseSearch([]byte(`{"error":{"code":403,"message":"upstream diagnostic"}}`)); !errors.Is(err, ErrUnavailable) {
		t.Fatal(err)
	}
}

func TestTrackIDMustComeFromItsOwnPlaybackTarget(t *testing.T) {
	for _, source := range []string{
		`"menu":{"watchEndpoint":{"videoId":"4D7u5KF7SP8"}}`,
		`"navigationEndpoint":{"watchEndpoint":{"videoId":"javascript:alert(1)"}}`,
		`"navigationEndpoint":{"browseEndpoint":{"browseId":"VLplaylist"}},"overlay":{"musicItemThumbnailOverlayRenderer":{"content":{"musicPlayButtonRenderer":{"playNavigationEndpoint":{"watchEndpoint":{"videoId":"4D7u5KF7SP8"}}}}}}`,
	} {
		data := `{"contents":{"musicTwoRowItemRenderer":{"title":{"simpleText":"Not a track"},` + source + `}}}`
		tracks, err := parseSearch([]byte(data))
		if err != nil || len(tracks) != 0 {
			t.Fatalf("unexpected menu/invalid target: %+v %v", tracks, err)
		}
	}
}

func TestSearchAnonymousRequestAndBootstrapCache(t *testing.T) {
	var bootstrapCalls, searchCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Cookie") != "" || r.Header.Get("Authorization") != "" || r.URL.Query().Has("key") {
			t.Error("request unexpectedly carried account data or an API key")
		}
		switch r.URL.Path {
		case "/":
			bootstrapCalls.Add(1)
			w.Header().Set("Set-Cookie", "example=not-retained; Path=/")
			fmt.Fprint(w, `<html><script>var config={"INNERTUBE_CLIENT_VERSION":"1.20261020.10.00"};</script></html>`)
		case "/youtubei/v1/search":
			searchCalls.Add(1)
			if r.Method != http.MethodPost || r.Header.Get("X-Youtube-Client-Name") != "67" || r.Header.Get("X-Youtube-Client-Version") != "1.20261020.10.00" {
				t.Error("incorrect anonymous client metadata")
			}
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
			}
			if body["query"] != "Daft Punk" || textAt(body, "context", "client", "clientName") != "WEB_REMIX" {
				t.Error("incorrect query/context")
			}
			if len(body) != 2 || len(at(body, "context").(map[string]any)) != 1 || len(at(body, "context", "client").(map[string]any)) != 4 {
				t.Error("unexpected additional client fields")
			}
			fmt.Fprint(w, searchFixture)
		default:
			t.Error("unexpected request path")
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client := NewClient()
	client.origin = server.URL
	client.http = server.Client()
	for range 2 {
		tracks, err := client.Search(context.Background(), " Daft Punk ")
		if err != nil || len(tracks) != 2 {
			t.Fatalf("search failed: %v", err)
		}
	}
	if bootstrapCalls.Load() != 1 || searchCalls.Load() != 2 {
		t.Fatal("public client version was not cached")
	}
}

func TestSearchRejectsInvalidQueryBeforeNetwork(t *testing.T) {
	client := NewClient()
	for _, query := range []string{"", " \n\t", strings.Repeat("界", 201), string([]byte{0xff})} {
		_, err := client.Search(context.Background(), query)
		if !errors.Is(err, ErrEmptyQuery) && !errors.Is(err, ErrQueryTooLong) {
			t.Fatalf("query was not rejected: %v", err)
		}
	}
}

func TestHTTPFailuresAndLimits(t *testing.T) {
	for _, test := range []struct {
		name   string
		status int
		body   string
		want   error
	}{
		{"blocked", http.StatusForbidden, "private upstream diagnostic", ErrUnavailable},
		{"bad bootstrap", http.StatusOK, "no usable public configuration", ErrInvalidResponse},
		{"large bootstrap", http.StatusOK, strings.Repeat("x", maxBootstrapBytes+1), ErrResponseTooLarge},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(test.status); fmt.Fprint(w, test.body) }))
			defer server.Close()
			client := NewClient()
			client.origin = server.URL
			client.http = server.Client()
			_, err := client.Search(context.Background(), "test")
			if !errors.Is(err, test.want) {
				t.Fatalf("got %v, want %v", err, test.want)
			}
			if strings.Contains(err.Error(), "private upstream") {
				t.Fatal("returned response contents in an error")
			}
		})
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, strings.Repeat("x", maxSearchBytes+1)) }))
	defer server.Close()
	client := NewClient()
	client.origin = server.URL
	client.http = server.Client()
	client.version = "1.20261020.10.00"
	client.versionExpires = time.Now().Add(time.Minute)
	if _, err := client.Search(context.Background(), "test"); !errors.Is(err, ErrResponseTooLarge) {
		t.Fatalf("large search response: %v", err)
	}
}

func TestCancellationAndNetworkFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { <-r.Context().Done() }))
	client := NewClient()
	client.origin = server.URL
	client.http = server.Client()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	if _, err := client.Search(ctx, "test"); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected deadline error, got %v", err)
	}
	server.Close()
	if _, err := client.Search(context.Background(), "test"); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("expected network failure, got %v", err)
	}
}

func TestThumbnailURLsAreRestricted(t *testing.T) {
	for _, url := range []string{"javascript:alert(1)", "http://i.ytimg.com/a", "https://ytimg.com.evil.example/a", "https://example.com/a", "https://user@i.ytimg.com/a", "https://i.ytimg.com:1234/a"} {
		if safeThumbnail(url) != "" {
			t.Errorf("accepted unsafe thumbnail %q", url)
		}
	}
	for _, url := range []string{"https://i.ytimg.com/vi/4D7u5KF7SP8/hqdefault.jpg", "https://lh3.googleusercontent.com/image", "https://yt3.ggpht.com/image"} {
		if safeThumbnail(url) != url {
			t.Errorf("refused image CDN %q", url)
		}
	}
}
