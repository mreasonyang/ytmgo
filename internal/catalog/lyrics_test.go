package catalog

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func timedLyricFixture(text string, start, end any) object {
	return object{"lyricLine": text, "cueRange": object{"startTimeMilliseconds": start, "endTimeMilliseconds": end}}
}

func timedLyricsFixture(rows ...any) object {
	return object{"contents": object{"elementRenderer": object{"newElement": object{"type": object{"componentType": object{"model": object{"timedLyricsModel": object{"lyricsData": object{
		"sourceMessage": "Source: synthetic provider", "timedLyricsData": rows,
	}}}}}}}}}
}

func plainLyricsFixture() object {
	return object{"contents": object{"sectionListRenderer": object{"contents": []any{object{"musicDescriptionShelfRenderer": object{
		"description": words("Synthetic first line\nSynthetic second line"), "footer": words("Source: synthetic provider"),
	}}}}}}
}

func TestTimedLyricsPreserveRealIntervalsAndAttribution(t *testing.T) {
	response := timedLyricsFixture(timedLyricFixture("Synthetic first", "1200", "2400"), timedLyricFixture("Synthetic second", float64(3100), float64(4600)))
	result, err := parseTimedLyrics(response)
	if err != nil || result.Text != "Synthetic first\nSynthetic second" || result.Source != "Source: synthetic provider" || len(result.Lines) != 2 {
		t.Fatal("timed lyric content or attribution was not preserved")
	}
	if result.Lines[0] != (LyricLine{"Synthetic first", 1200 * time.Millisecond, 2400 * time.Millisecond}) || result.Lines[1] != (LyricLine{"Synthetic second", 3100 * time.Millisecond, 4600 * time.Millisecond}) {
		t.Fatal("service cue intervals were changed")
	}
	plain, err := parseTimedLyrics(plainLyricsFixture())
	if err != nil || plain.Text == "" || plain.Source != result.Source || len(plain.Lines) != 0 {
		t.Fatal("plain response fallback lost content or invented timestamps")
	}
}

func TestTimedLyricsInvalidCuesRetainOnlyPlainText(t *testing.T) {
	for _, name := range []string{"missing", "missing_start", "missing_end", "negative", "fractional", "overflow", "reversed", "equal", "out_of_order", "not_numeric", "not_finite"} {
		t.Run(name, func(t *testing.T) {
			first := timedLyricFixture("Synthetic first", "1200", "2400")
			second := timedLyricFixture("Synthetic second", "3100", "4600")
			cue := second["cueRange"].(object)
			switch name {
			case "missing":
				delete(second, "cueRange")
			case "missing_start":
				delete(cue, "startTimeMilliseconds")
			case "missing_end":
				delete(cue, "endTimeMilliseconds")
			case "negative":
				cue["startTimeMilliseconds"] = float64(-1)
			case "fractional":
				cue["startTimeMilliseconds"] = 3100.5
			case "overflow":
				cue["endTimeMilliseconds"] = "9223372036854775807"
			case "reversed":
				cue["endTimeMilliseconds"] = "3000"
			case "equal":
				cue["endTimeMilliseconds"] = "3100"
			case "out_of_order":
				cue["startTimeMilliseconds"] = "1000"
			case "not_numeric":
				cue["startTimeMilliseconds"] = "3.1s"
			case "not_finite":
				cue["startTimeMilliseconds"] = math.NaN()
			}
			result, err := parseTimedLyrics(timedLyricsFixture(first, second))
			if err != nil || len(result.Lines) != 0 || result.Text != "Synthetic first\nSynthetic second" {
				t.Fatal("invalid timing did not degrade to unchanged plain lyrics")
			}
		})
	}
}

func TestLyricsParsingBoundsAndMissingData(t *testing.T) {
	tooMany := make([]any, maxLyricsLines+1)
	for index := range tooMany {
		tooMany[index] = timedLyricFixture("Synthetic", "0", "1000")
	}
	for _, test := range []struct {
		name string
		data object
		want error
	}{
		{"large_line", timedLyricsFixture(timedLyricFixture(strings.Repeat("x", maxLyricsBytes+1), "0", "1000")), ErrResponseTooLarge},
		{"large_combined_text", timedLyricsFixture(timedLyricFixture(strings.Repeat("x", maxLyricsBytes/2), "0", "1000"), timedLyricFixture(strings.Repeat("x", maxLyricsBytes/2), "1000", "2000")), ErrResponseTooLarge},
		{"many_rows", timedLyricsFixture(tooMany...), ErrResponseTooLarge},
		{"missing_text", timedLyricsFixture(object{"cueRange": object{"startTimeMilliseconds": "0", "endTimeMilliseconds": "1000"}}), ErrInvalidResponse},
		{"empty", timedLyricsFixture(), ErrNoLyrics},
		{"unrelated", object{"unrelated": timedLyricsFixture(timedLyricFixture("Hidden", "0", "1000"))}, ErrNoLyrics},
	} {
		t.Run(test.name, func(t *testing.T) {
			if result, err := parseTimedLyrics(test.data); !errors.Is(err, test.want) || result.Text != "" || len(result.Lines) != 0 {
				t.Fatal("lyric bounds or supported container boundary was not enforced")
			}
		})
	}
	data := timedLyricsFixture(timedLyricFixture("Synthetic", "0", "1000"))
	model := at(data, "contents", "elementRenderer", "newElement", "type", "componentType", "model", "timedLyricsModel", "lyricsData").(object)
	model["sourceMessage"] = strings.Repeat(" ", maxLyricsSource+1)
	if _, err := parseTimedLyrics(data); !errors.Is(err, ErrResponseTooLarge) {
		t.Fatal("source size was not bounded before normalization")
	}
	for _, value := range []any{"86400001", "-1", "1e3", true, nil, math.Inf(1), float64(maxLyricMillis + 1)} {
		if _, ok := lyricTime(value); ok {
			t.Fatal("invalid or excessive timestamp accepted")
		}
	}
}

func TestLyricsMobileProfileIsRequestLocalAndAccountScoped(t *testing.T) {
	for _, authenticated := range []bool{false, true} {
		t.Run(map[bool]string{false: "anonymous", true: "synthetic_account"}[authenticated], func(t *testing.T) {
			entered, release := make(chan struct{}), make(chan struct{})
			var nextCalls, mobileCalls, plainCalls atomic.Int32
			client := fixtureClient(t, func(w http.ResponseWriter, req *http.Request) {
				if req.Header.Get("Origin") != serviceOrigin || req.Header.Get("Referer") != serviceOrigin+"/" {
					t.Error("first-party origin headers changed")
				}
				if authenticated {
					if req.Header.Get("Authorization") == "" || req.Header.Get("Cookie") == "" || req.Header.Get("X-Goog-Authuser") != "2" || req.Header.Get("X-Goog-Pageid") != "synthetic_brand" {
						t.Error("request lost its explicit synthetic account scope")
					}
				} else {
					for _, key := range []string{"Cookie", "Authorization", "X-Goog-Authuser", "X-Goog-Pageid", "X-Origin"} {
						if req.Header.Get(key) != "" {
							t.Error("anonymous lyrics carried account credentials")
						}
					}
				}
				var body object
				if json.NewDecoder(req.Body).Decode(&body) != nil {
					t.Error("synthetic request was invalid")
					w.WriteHeader(400)
					return
				}
				mobile := textAt(body, "context", "client", "clientName") == "ANDROID_MUSIC"
				if mobile {
					if req.URL.Path != "/youtubei/v1/browse" || textAt(body, "browseId") != "MPLYt_lyrics" || textAt(body, "context", "client", "clientVersion") != lyricsClientVersion || req.Header.Get("X-Youtube-Client-Name") != "21" || req.Header.Get("X-Youtube-Client-Version") != lyricsClientVersion {
						t.Error("mobile lyric request contract changed")
					}
					mobileCalls.Add(1)
					close(entered)
					<-release
					_ = json.NewEncoder(w).Encode(timedLyricsFixture(timedLyricFixture("Synthetic line", "1200", "2400")))
					return
				}
				if textAt(body, "context", "client", "clientName") != "WEB_REMIX" || req.Header.Get("X-Youtube-Client-Name") != "67" || req.Header.Get("X-Youtube-Client-Version") != musicClientVersion {
					t.Error("mobile profile leaked into another catalog request")
				}
				switch req.URL.Path {
				case "/youtubei/v1/next":
					nextCalls.Add(1)
					_ = json.NewEncoder(w).Encode(queueResponse(true))
				case "/youtubei/v1/search":
					_ = json.NewEncoder(w).Encode(catalogResponse(responsiveSong("abc123DEF_-", "Synthetic")))
				case "/youtubei/v1/browse":
					plainCalls.Add(1)
					_ = json.NewEncoder(w).Encode(plainLyricsFixture())
				default:
					t.Error("unexpected lyrics request")
					w.WriteHeader(400)
				}
			})
			if authenticated && client.SetSession(syntheticCookies(), 2, "synthetic_brand") != nil {
				t.Fatal("synthetic account setup failed")
			}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			type reply struct {
				result LyricsResult
				err    error
			}
			done := make(chan reply, 1)
			go func() { result, err := client.LyricsWithTiming(ctx, "abc123DEF_-"); done <- reply{result, err} }()
			select {
			case <-entered:
			case <-ctx.Done():
				close(release)
				t.Fatal("mobile request did not start")
			}
			_, searchErr := client.Search(ctx, "synthetic")
			close(release)
			response := <-done
			if searchErr != nil || response.err != nil || len(response.result.Lines) != 1 {
				t.Fatal("concurrent mobile lyrics and web search did not succeed")
			}
			plain, err := client.Lyrics(ctx, "abc123DEF_-")
			if err != nil || plain == "" || nextCalls.Load() != 1 || mobileCalls.Load() != 1 || plainCalls.Load() != 1 {
				t.Fatal("legacy lyrics behavior or browse reference reuse changed")
			}
		})
	}
}

func TestLyricsMobileFailureFallsBackToPlain(t *testing.T) {
	for _, mode := range []string{"http", "missing", "malformed", "oversized", "plain_response", "untimed_rows"} {
		t.Run(mode, func(t *testing.T) {
			var mobile, web atomic.Int32
			client := fixtureClient(t, func(w http.ResponseWriter, req *http.Request) {
				if req.URL.Path == "/youtubei/v1/next" {
					_ = json.NewEncoder(w).Encode(queueResponse(true))
					return
				}
				var body object
				_ = json.NewDecoder(req.Body).Decode(&body)
				if textAt(body, "context", "client", "clientName") == "ANDROID_MUSIC" {
					mobile.Add(1)
					switch mode {
					case "http":
						w.WriteHeader(403)
					case "missing":
						_ = json.NewEncoder(w).Encode(object{"contents": object{}})
					case "malformed":
						_ = json.NewEncoder(w).Encode(timedLyricsFixture(object{"lyricLine": true}))
					case "oversized":
						_ = json.NewEncoder(w).Encode(timedLyricsFixture(timedLyricFixture(strings.Repeat("x", maxLyricsBytes+1), "0", "1000")))
					case "plain_response":
						_ = json.NewEncoder(w).Encode(plainLyricsFixture())
					case "untimed_rows":
						_ = json.NewEncoder(w).Encode(timedLyricsFixture(object{"lyricLine": "Synthetic plain line"}))
					}
					return
				}
				web.Add(1)
				_ = json.NewEncoder(w).Encode(plainLyricsFixture())
			})
			result, err := client.LyricsWithTiming(context.Background(), "abc123DEF_-")
			expectedWeb := int32(1)
			if mode == "plain_response" || mode == "untimed_rows" {
				expectedWeb = 0
			}
			if err != nil || result.Text == "" || len(result.Lines) != 0 || mobile.Load() != 1 || web.Load() != expectedWeb {
				t.Fatal("mobile failure or unsupported timing did not retain bounded plain fallback")
			}
		})
	}
}

func TestLyricsCancellationAndSessionChangeCannotPublishOrFallback(t *testing.T) {
	for _, mode := range []string{"cancel", "replace", "clear", "expire"} {
		t.Run(mode, func(t *testing.T) {
			entered, release := make(chan struct{}), make(chan struct{})
			var calls atomic.Int32
			client := NewWithOptions(Options{HTTPClient: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				calls.Add(1)
				close(entered)
				<-release
				body, _ := json.Marshal(timedLyricsFixture(timedLyricFixture("Synthetic", "1200", "2400")))
				return directAPIResponse(req, 200, string(body)), nil
			})}})
			if client.SetSession(syntheticCookies(), 1, "") != nil {
				t.Fatal("synthetic session setup failed")
			}
			client.lyrics["abc123DEF_-"] = lyricsReference{browseID: "MPLYt_lyrics", expires: time.Now().Add(time.Minute)}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			done := make(chan error, 1)
			go func() {
				result, err := client.LyricsWithTiming(ctx, "abc123DEF_-")
				if result.Text != "" || len(result.Lines) != 0 {
					t.Error("obsolete request published lyric data")
				}
				done <- err
			}()
			select {
			case <-entered:
			case <-time.After(time.Second):
				close(release)
				t.Fatal("synthetic mobile request did not start")
			}
			want := ErrSessionChanged
			switch mode {
			case "cancel":
				cancel()
				want = context.Canceled
			case "replace":
				if client.SetSession(syntheticCookies(), 2, "") != nil {
					close(release)
					t.Fatal("replacement synthetic session failed")
				}
			case "clear":
				client.ClearSession()
			case "expire":
				client.mu.Lock()
				client.session.expires = time.Now().Add(-time.Second)
				client.mu.Unlock()
				want = ErrSessionExpired
			}
			close(release)
			if err := <-done; !errors.Is(err, want) || calls.Load() != 1 {
				t.Fatal("canceled or obsolete lyric request continued or changed error category")
			}
		})
	}
}
