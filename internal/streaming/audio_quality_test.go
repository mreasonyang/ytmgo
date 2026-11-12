package streaming

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"ytmgo/internal/audioquality"
)

// The synthetic source length identifies the selected format on the published
// lease, so these cases also cover transferring the choice through ResolveAudio.
func TestResolveAudioSourceForQuality(t *testing.T) {
	type rates struct{ average, nominal string }
	mixed := []rates{{"", "128000"}, {"64000", "160000"}, {"-1", "256000"}, {"", "NaN"}}
	above := []rates{{"", ""}, {"192000", ""}, {"", "132000"}}
	unknown := []rates{{"bad", "0"}, {"+Inf", "-50"}}
	for _, tc := range []struct {
		name    string
		sources []rates
		quality audioquality.Quality
		want    int
	}{
		{"legacy audio defaults to highest", mixed, "", 2},
		{"highest uses fallback when average invalid", mixed, audioquality.Best, 2},
		{"saver prefers known average", mixed, audioquality.Saver, 1},
		{"standard includes exact boundary", mixed, audioquality.Standard, 0},
		{"standard picks lowest above target", above, audioquality.Standard, 2},
		{"highest available below target", []rates{{"", "48000"}, {"96000", ""}}, audioquality.Standard, 1},
		{"all unknown retain first compatible", unknown, audioquality.Saver, 0},
		{"ties retain service order", []rates{{"", "96000"}, {"96000", ""}}, audioquality.Standard, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var formats []any
			for i, source := range tc.sources {
				f := map[string]any{
					"mimeType":      `audio/webm; codecs="opus"`,
					"url":           fmt.Sprintf("https://r.googlevideo.com/x?expire=%d", time.Now().Add(time.Hour).Unix()),
					"contentLength": fmt.Sprint(500 + i), "approxDurationMs": "20000",
				}
				if source.average != "" {
					f["averageBitrate"] = source.average
				}
				if source.nominal != "" {
					f["bitrate"] = source.nominal
				}
				formats = append(formats, f)
			}
			resp := jsJSON(map[string]any{"playabilityStatus": map[string]any{"status": "OK"}, "streamingData": map[string]any{"adaptiveFormats": formats}})
			r := testResolver(t, func(*http.Request) (*http.Response, error) { return response(resp), nil })
			r.session = Session{Revision: 1}
			r.sessionClient = map[string]any{"clientName": "WEB_REMIX", "visitorData": "fixture_visitor"}
			r.ready, r.expires = true, time.Now().Add(time.Hour)
			r.seed = extracted{Code: "synthetic", Timestamp: 12345}
			r.playerID, r.playerChecked = "fixture", time.Now()
			var media Media
			var err error
			if tc.quality == "" {
				media, err = r.Resolve(t.Context(), "AAAAAAAAAAA", "audio")
			} else {
				media, err = r.ResolveAudio(t.Context(), "AAAAAAAAAAA", tc.quality)
			}
			if err != nil {
				t.Fatal(err)
			}
			defer media.Lease.Close()
			if got := media.Lease.length - 500; got != int64(tc.want) {
				t.Errorf("source index = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestAudioQualitySkipsUnusableSources(t *testing.T) {
	source := func(rate, length, duration string) map[string]any {
		return map[string]any{"mimeType": `audio/webm; codecs="opus"`, "url": "https://r.googlevideo.com/x", "bitrate": rate, "contentLength": length, "approxDurationMs": duration}
	}
	low := source("48000", "", "3000")
	mid := source("128000", "100", "-1")
	high := source("256000", "150", "3000")
	for _, tc := range []struct {
		name      string
		quality   audioquality.Quality
		formats   []any
		wantError string
	}{
		{"saver tries higher sources", audioquality.Saver, []any{mid, low, high}, ""},
		{"standard tries other sources above target", audioquality.Standard, []any{low, high, mid}, ""},
		{"saver preserves error from its first choice", audioquality.Saver, []any{mid, low}, "invalid_content_length"},
		{"standard preserves error from its first choice", audioquality.Standard, []any{mid, low}, "invalid_duration"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data := map[string]any{"streamingData": map[string]any{"adaptiveFormats": tc.formats}}
			got, err := chooseFormat(data, "audio", formatPreference{audioQuality: tc.quality})
			if tc.wantError != "" {
				if err == nil || err.Error() != tc.wantError {
					t.Fatalf("error = %v, want %s", err, tc.wantError)
				}
				return
			}
			if err != nil || got.length != 150 {
				t.Fatalf("selected length = %d, error = %v", got.length, err)
			}
		})
	}
}
