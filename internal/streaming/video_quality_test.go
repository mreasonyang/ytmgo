package streaming

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"
)

func qualityFixture(heights ...int) map[string]any {
	var formats []any
	for _, height := range heights {
		formats = append(formats, map[string]any{
			"mimeType": `video/mp4; codecs="avc1.4d401f"`,
			"url":      "https://r.googlevideo.com/videoplayback?expire=" + fmt.Sprint(time.Now().Add(time.Hour).Unix()),
			"width":    float64(height * 16 / 9), "height": float64(height),
			"contentLength": "128", "approxDurationMs": "120000", "bitrate": float64(height * 1000),
		})
	}
	return map[string]any{"playabilityStatus": map[string]any{"status": "OK"}, "streamingData": map[string]any{"adaptiveFormats": formats}}
}

func TestVideoFormatRespectsQualityAndAvailableSources(t *testing.T) {
	for _, test := range []struct {
		name    string
		sources []int
		choice  int
		want    int
	}{
		{"default", []int{1080, 360, 720, 480}, 0, 720},
		{"raise", []int{720, 1080, 360}, 1080, 1080},
		{"lower", []int{1080, 720, 480, 360, 240, 144}, 240, 240},
		{"missing choice", []int{1080, 360, 480}, 720, 480},
		{"only larger sources", []int{1080, 720}, 360, 720},
	} {
		t.Run(test.name, func(t *testing.T) {
			f, err := chooseFormat(qualityFixture(test.sources...), "video", formatPreference{videoHeight: test.choice})
			if err != nil || f.height != test.want {
				t.Fatalf("selected height=%d error=%v; want %d", f.height, err, test.want)
			}
		})
	}
}

// Exercise the public resolver API as well as its sorter, so the preference
// cannot accidentally disappear between the player and format selection.
func TestResolveVideoCarriesQualityIntoPublishedMedia(t *testing.T) {
	r := testResolver(t, func(req *http.Request) (*http.Response, error) {
		return response(jsJSON(qualityFixture(360, 480, 720, 1080))), nil
	})
	r.session = Session{LoggedIn: true, MusicPremium: true, Revision: 1, Cookie: "SAPISID=synthetic"}
	r.sessionClient = map[string]any{"clientName": "WEB_REMIX", "visitorData": "fixture_visitor"}
	r.ready, r.expires = true, time.Now().Add(time.Hour)
	r.seed = extracted{Code: "synthetic", Timestamp: 12345}
	r.playerID, r.playerChecked = "fixture", time.Now()
	for _, quality := range []int{720, 1080, 360} {
		result, err := r.ResolveVideo(context.Background(), "AAAAAAAAAAA", quality)
		if err != nil {
			t.Fatalf("resolve %dp: %v", quality, err)
		}
		if result.Height != quality {
			t.Fatalf("resolve %dp returned %dp", quality, result.Height)
		}
		result.Lease.Close()
	}
}
