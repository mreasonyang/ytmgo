package streaming

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type recoveryTransport struct {
	roundTrip
	closes atomic.Int32
}

func (t *recoveryTransport) CloseIdleConnections() { t.closes.Add(1) }

func TestFailedProofWarmDiscardsSessionAndConnection(t *testing.T) {
	for _, tc := range []struct{ body, category string }{
		{"<html>YouTube Music is not available in your area</html>", "region_unavailable"},
		{"<html>YouTube Music is not available in your country</html>", "region_unavailable"},
		{"<html>unsupported config</html>", "proof_request_rejected"},
	} {
		t.Run(tc.category, func(t *testing.T) {
			var bootstraps atomic.Int32
			r := testResolver(t, func(req *http.Request) (*http.Response, error) {
				if req.URL.Path == "/sw.js_data" {
					bootstraps.Add(1)
					device := make([]any, 108)
					device[13] = "new_visitor"
					return response(")]}'\n" + jsJSON([]any{[]any{nil, nil, []any{[]any{device}}}})), nil
				}
				return response(tc.body), nil
			})
			transport := &recoveryTransport{roundTrip: r.client.Transport.(roundTrip)}
			r.client.Transport = transport
			r.session = Session{Revision: 1}
			r.seed = extracted{Code: "public transform", Timestamp: 12345}
			r.playerID, r.playerChecked = "fixture", time.Now()
			r.sessionClient = map[string]any{"visitorData": "old_visitor"}
			r.binding = proofBinding{Visitor: "old_binding"}
			for attempt := 0; attempt < 2; attempt++ {
				err := r.Warm(context.Background())
				var detail *Failure
				if !errors.As(err, &detail) || detail.Category != tc.category || detail.Stage != "proof_config_parse" {
					t.Fatal("proof rejection was not classified")
				}
				if r.ready || r.proof != nil || r.sessionClient != nil || r.binding != (proofBinding{}) || transport.closes.Load() != int32(attempt+1) {
					t.Fatal("warm failure retained visitor/proof/connection state")
				}
			}
			if bootstraps.Load() != 1 {
				t.Fatal("retry did not fetch a fresh visitor")
			}
		})
	}
}

func TestRefreshDropsProofAndKeepsPublicTransform(t *testing.T) {
	r := testResolver(t, func(*http.Request) (*http.Response, error) { t.Fatal("refresh made a request"); return nil, nil })
	host := &stubHost{}
	r.proof, r.ready, r.expires = host, true, time.Now().Add(time.Hour)
	r.sessionClient = map[string]any{"visitorData": "old"}
	r.binding = proofBinding{Visitor: "old"}
	r.seed = extracted{Code: "public"}
	if err := r.Refresh(context.Background()); err != nil || r.ready || r.proof != nil || r.sessionClient != nil || r.binding != (proofBinding{}) || host.closed.Load() != 1 || r.seed.Code != "public" {
		t.Fatal("refresh retained network state or discarded the public transform")
	}
}

func TestRegionFailureCanResolveOnRetryInSameProcess(t *testing.T) {
	var musicPages, bootstraps atomic.Int32
	r := testResolver(t, func(req *http.Request) (*http.Response, error) {
		switch {
		case req.URL.Path == "/sw.js_data":
			bootstraps.Add(1)
			device := make([]any, 108)
			device[13] = "fresh_visitor"
			return response(")]}'\n" + jsJSON([]any{[]any{nil, nil, []any{[]any{device}}}})), nil
		case req.URL.Host == "music.youtube.com" && req.URL.Path == "/":
			if musicPages.Add(1) == 1 {
				return response("YouTube Music is not available in your area"), nil
			}
			return response(`ytcfg.set({"LOGGED_IN":false,"VISITOR_DATA":"fresh_binding"});`), nil
		case req.URL.Host == "www.youtube.com":
			return response(`ytcfg.set({"LOGGED_IN":false});window.ytAtN({R:{bgChallenge:{program:'synthetic',globalName:'fixture',interpreterUrl:{privateDoNotAccessOrElseTrustedResourceUrlWrappedValue:'https://www.google.com/js/th/fixture.js'}}}});`), nil
		case req.URL.Host == "www.google.com":
			res := response("public interpreter fixture")
			res.Header.Set("Content-Type", "text/javascript")
			return res, nil
		case req.URL.Host == "jnn-pa.googleapis.com":
			return response(`["synthetic-integrity",3600,60]`), nil
		case req.URL.Path == "/youtubei/v1/player":
			return response(jsJSON(map[string]any{"playabilityStatus": map[string]any{"status": "OK"}, "streamingData": map[string]any{"adaptiveFormats": []any{map[string]any{"mimeType": `audio/mp4; codecs="mp4a.40.2"`, "url": "https://r.googlevideo.com/videoplayback?expire=" + fmt.Sprint(time.Now().Add(time.Hour).Unix()), "contentLength": "128", "approxDurationMs": "120000"}}}})), nil
		}
		t.Fatal("unexpected recovery request")
		return nil, nil
	})
	r.newHost = func(mode string) (scriptHost, error) {
		if mode != "dom" {
			t.Fatal("retry discarded its public transform")
		}
		return &stubHost{run: func(code string, out any) error {
			switch {
			case strings.HasPrefix(code, "ytmgoProof.headers"):
				return json.Unmarshal([]byte(`{"Content-Type":"application/json+protobuf","X-Goog-Api-Key":"synthetic"}`), out)
			case strings.HasPrefix(code, "ytmgoProof.snapshot"), strings.HasPrefix(code, "ytmgoProof.mint"):
				return json.Unmarshal([]byte(`"synthetic-proof"`), out)
			case strings.HasPrefix(code, "ytmgoProof.integrity"):
				return json.Unmarshal([]byte(`true`), out)
			}
			return errors.New("unexpected_fixture_call")
		}}, nil
	}
	r.session = Session{Revision: 1}
	r.seed = extracted{Code: "public", Timestamp: 12345}
	r.playerID, r.playerChecked = "fixture", time.Now()
	if _, err := r.Resolve(context.Background(), "AAAAAAAAAAA", "audio"); err == nil || err.Error() != "region_unavailable" {
		t.Fatal("first attempt did not expose the region rejection")
	}
	media, err := r.Resolve(context.Background(), "AAAAAAAAAAA", "audio")
	if err != nil || media.Lease == nil || !r.ready || bootstraps.Load() != 2 || musicPages.Load() != 2 {
		t.Fatal("retry did not rebuild proof and resolve a fresh stream in the same process")
	}
	media.Lease.Close()
}
