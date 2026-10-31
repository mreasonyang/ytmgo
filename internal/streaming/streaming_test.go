package streaming

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os/exec"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type roundTrip func(*http.Request) (*http.Response, error)

func (f roundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func testResolver(t *testing.T, fn roundTrip) *Resolver {
	t.Helper()
	r, err := New(Options{Command: func(context.Context, string) *exec.Cmd { t.Fatal("unexpected script process"); return nil }, Client: &http.Client{Transport: fn}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(r.Close)
	return r
}
func response(body string) *http.Response {
	return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body)), ContentLength: int64(len(body))}
}

type stubHost struct {
	run    func(string, any) error
	closed atomic.Int32
}

func (h *stubHost) call(_ context.Context, s string, v any) error { return h.run(s, v) }
func (h *stubHost) Close()                                        { h.closed.Add(1) }
func acceptedFixture() map[string]any {
	return map[string]any{"contents": map[string]any{}, "responseContext": map[string]any{"serviceTrackingParams": []any{map[string]any{"service": "GFEEDBACK", "params": []any{map[string]any{"key": "logged_in", "value": "1"}}}, map[string]any{"service": "CSI", "params": []any{map[string]any{"key": "yt_li", "value": "1"}}}}}}
}
func TestPremiumGoProtocolSkipsProofAndKeepsLeasesAcrossMembershipChange(t *testing.T) {
	var requests atomic.Int32
	r := testResolver(t, func(req *http.Request) (*http.Response, error) {
		requests.Add(1)
		if req.URL.Host != "music.youtube.com" || req.Header.Get("Cookie") == "" || !strings.HasPrefix(req.Header.Get("Authorization"), "SAPISIDHASH ") {
			t.Error("account request boundary")
		}
		var body map[string]any
		if json.NewDecoder(req.Body).Decode(&body) != nil {
			t.Fatal("body")
		}
		if textAt(body, "context", "client", "clientName") != "WEB_REMIX" || textAt(body, "context", "client", "visitorData") != "fixture_visitor" {
			t.Error("client context changed")
		}
		if strings.HasSuffix(req.URL.Path, "/browse") {
			return response(jsJSON(acceptedFixture())), nil
		}
		if valueAt(body, "playbackContext", "contentPlaybackContext", "signatureTimestamp") != float64(12345) {
			t.Error("missing signature timestamp")
		}
		return response(jsJSON(map[string]any{"playabilityStatus": map[string]any{"status": "OK"}, "streamingData": map[string]any{"adaptiveFormats": []any{map[string]any{"mimeType": `audio/mp4; codecs="mp4a.40.2"`, "url": "https://r.googlevideo.com/videoplayback?expire=" + fmt.Sprint(time.Now().Add(time.Hour).Unix()), "contentLength": "128", "approxDurationMs": "120000"}}}})), nil
	})
	session := Session{LoggedIn: true, MusicPremium: true, Revision: 1, Cookie: "SAPISID=synthetic"}
	if err := r.Configure(context.Background(), session); err != nil {
		t.Fatal(err)
	}
	r.seed = extracted{Code: "synthetic", Timestamp: 12345}
	r.sessionClient = map[string]any{"clientName": "WEB_REMIX", "visitorData": "fixture_visitor"}
	result, err := r.Resolve(context.Background(), "AAAAAAAAAAA", "audio")
	if err != nil || result.Duration != 2*time.Minute {
		t.Fatalf("resolve: %v", err)
	}
	if requests.Load() != 2 || r.proof != nil {
		t.Fatal("premium allocated a script or proof host")
	}
	session.MusicPremium = false
	if err = r.Configure(context.Background(), session); err != nil {
		t.Fatal(err)
	}
	if result.Lease.life.Err() != nil {
		t.Fatal("membership update revoked playing lease")
	}
	session.Revision++
	if err = r.Configure(context.Background(), session); err != nil {
		t.Fatal(err)
	}
	if result.Lease.life.Err() == nil {
		t.Fatal("account switch retained lease")
	}
}
func TestBootstrapJSONAndCredentialRouting(t *testing.T) {
	device := make([]any, 108)
	device[13] = "synthetic_visitor"
	device[61] = []any{"install"}
	device[79] = "UTC"
	payload := []any{[]any{nil, nil, []any{[]any{device}, "unused_fixture_key"}}}
	r := testResolver(t, func(req *http.Request) (*http.Response, error) {
		if req.URL.Path != "/sw.js_data" || req.Header.Get("Authorization") != "" || !strings.HasPrefix(req.Header.Get("Cookie"), "PREF=tz=UTC;") {
			t.Error("bootstrap boundary")
		}
		return response(")]}'\n" + jsJSON(payload)), nil
	})
	if err := r.bootstrap(context.Background()); err != nil {
		t.Fatal(err)
	}
	if textAt(r.sessionClient, "visitorData") != "synthetic_visitor" || textAt(r.sessionClient, "configInfo", "appInstallData") != "install" {
		t.Fatal("bootstrap context not preserved")
	}
	_, err := r.request(context.Background(), "GET", "https://www.youtube.com/", nil, http.Header{"Cookie": {"SAPISID=synthetic"}}, "proof_init", "")
	if err == nil || err.Error() != "unexpected_authentication" {
		t.Fatal("credential routed outside music")
	}
	for _, raw := range []string{"http://music.youtube.com/", "https://music.youtube.com:443/", "https://music.youtube.com.evil.invalid/", "https://www.google.com/other", "https://music.youtube.com/redirect"} {
		u, _ := url.Parse(raw)
		if allowedRequest(u, "GET") {
			t.Fatal("unexpected route accepted")
		}
	}
}
func TestFormatSelectionAndURLBoundaries(t *testing.T) {
	item := func(codec string, bitrate float64) map[string]any {
		return map[string]any{"mimeType": codec, "url": "https://r.googlevideo.com/x", "contentLength": "100", "approxDurationMs": "3000", "bitrate": bitrate}
	}
	a := item(`audio/mp4; codecs="mp4a.40.2"`, 128)
	b := item(`audio/webm; codecs="opus"`, 256)
	drm := item(`audio/webm; codecs="opus"`, 512)
	drm["drmFamilies"] = []any{"x"}
	data := map[string]any{"streamingData": map[string]any{"adaptiveFormats": []any{a, drm, b}}}
	f, err := chooseFormat(data, "audio")
	if err != nil || f.contentType != "audio/webm" || f.duration != 3*time.Second {
		t.Fatal("audio selection changed")
	}
	v := item(`video/mp4; codecs="avc1.4d401e"`, 100)
	v["width"], v["height"] = float64(854), float64(480)
	data["streamingData"].(map[string]any)["adaptiveFormats"] = []any{v}
	f, err = chooseFormat(data, "video")
	if err != nil || f.height != 480 {
		t.Fatal("video selection changed")
	}
	v["height"] = float64(720)
	if _, err = chooseFormat(data, "video"); err == nil {
		t.Fatal("video size limit lost")
	}
	for _, raw := range []string{"https://googlevideo.com/x", "https://r.googlevideo.com:443/x", "https://x@r.googlevideo.com/x", "https://r.googlevideo.com/x#x", "https://r.googlevideo.com/x?n=enhanced_except_x", "https://r.googlevideo.com/x?sabr=1", "https://r.googlevideo.com/x'"} {
		if _, err := mediaURL(raw); err == nil {
			t.Fatal("invalid media URL accepted")
		}
	}
}
func TestProofPageIsParsedWithoutExecutingPageCode(t *testing.T) {
	page := []byte(`ytcfg.set({"LOGGED_IN":false,"label":"\ud83c\udfb5"});window.ytAtN({R:{bgChallenge:{program:'synthetic',globalName:'fixture',interpreterUrl:{privateDoNotAccessOrElseTrustedResourceUrlWrappedValue:'https://www.google.com/js/th/fixture.js'},},},});`)
	p, err := parseProofPage(page)
	if err != nil || p.Program != "synthetic" || p.Config["label"] != "🎵" {
		t.Fatal("literal page parse failed")
	}
	encoded := map[string]any{"bgChallenge": map[string]any{"program": "synthetic", "globalName": "fixture", "interpreterUrl": map[string]any{"privateDoNotAccessOrElseTrustedResourceUrlWrappedValue": "https://www.google.com/js/th/fixture.js"}}}
	if _, err := parseProofPage([]byte(`ytcfg.set({"LOGGED_IN":false});window.ytAtN({R:` + jsJSON(jsJSON(encoded)) + `});`)); err != nil {
		t.Fatal("JSON-encoded challenge container rejected")
	}
	for _, bad := range []string{strings.Replace(string(page), "'synthetic'", "runArbitrary()", 1), `ytcfg.set({"LOGGED_IN":true});` + string(page), strings.Replace(string(page), "https://www.google.com/js/th/fixture.js", "https://other.invalid/js/th/fixture.js", 1)} {
		if _, err := parseProofPage([]byte(bad)); err == nil {
			t.Fatal("unsafe page accepted")
		}
	}
	for _, body := range []string{`["fixture",3600,60]`, `["fixture",3600,null]`} {
		if _, _, err := parseIntegrity([]byte(body)); err != nil {
			t.Fatal("valid integrity rejected")
		}
	}
	for _, body := range []string{`["fixture",60,0]`, `["fixture",90000,0]`, `["fixture",3600,-1]`, `["fixture","3600"]`, `{}`} {
		if _, _, err := parseIntegrity([]byte(body)); err == nil {
			t.Fatal("invalid integrity accepted")
		}
	}
}
func TestProofBindingIdentityAndExperiment(t *testing.T) {
	config := map[string]any{"LOGGED_IN": true, "SESSION_INDEX": "2", "DATASYNC_ID": "brand||primary", "VISITOR_DATA": "fixture_visitor", "WEB_PLAYER_CONTEXT_CONFIGS": map[string]any{"player": map[string]any{"serializedExperimentFlags": "html5_generate_content_po_token=true"}}}
	parse := func(s Session) (proofBinding, error) {
		return parseBinding([]byte("ytcfg.set("+jsJSON(config)+");"), s)
	}
	s := Session{LoggedIn: true, AccountIndex: 2, OnBehalfOfUser: "brand"}
	b, err := parse(s)
	if err != nil || !b.VideoBound || b.Binding != "brand||primary" {
		t.Fatal("brand binding changed")
	}
	s.AccountIndex = 1
	if _, err = parse(s); err == nil {
		t.Fatal("wrong account accepted")
	}
	s.AccountIndex = 2
	s.OnBehalfOfUser = ""
	if _, err = parse(s); err == nil {
		t.Fatal("wrong brand accepted")
	}
	if _, err = parse(Session{}); err == nil {
		t.Fatal("anonymous signed page accepted")
	}
}
func TestRangeAndBoundedDirectTransport(t *testing.T) {
	var calls atomic.Int32
	total := int64(140000)
	r := testResolver(t, func(req *http.Request) (*http.Response, error) {
		calls.Add(1)
		if req.Header.Get("Cookie") != "" || req.Header.Get("Authorization") != "" || req.Header.Get("Range") != "" {
			t.Error("upstream request boundary")
		}
		var start, end int64
		if _, err := fmt.Sscanf(req.URL.Query().Get("range"), "%d-%d", &start, &end); err != nil || end-start+1 > 64<<10 {
			t.Fatal("unbounded upstream range")
		}
		res := response(strings.Repeat("x", int(end-start+1)))
		res.StatusCode = 206
		res.Header.Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, total))
		return res, nil
	})
	remote, _ := url.Parse("https://r.googlevideo.com/x?expire=" + fmt.Sprint(time.Now().Add(time.Hour).Unix()))
	lease, err := r.publish(remote, format{length: total, contentType: "audio/mp4"})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(lease)
	defer server.Close()
	req, _ := http.NewRequest("GET", server.URL, nil)
	req.Header.Set("Range", "bytes=10-")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(res.Body)
	res.Body.Close()
	if err != nil || len(body) != int(total-10) || calls.Load() != 3 || res.StatusCode != 206 {
		t.Fatal("range relay failed")
	}
	head, _ := http.NewRequest("HEAD", server.URL, nil)
	res, err = http.DefaultClient.Do(head)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if calls.Load() != 3 || res.ContentLength != total {
		t.Fatal("HEAD issued a remote request")
	}
	for _, tc := range []struct {
		value      string
		start, end int64
		ok         bool
	}{{"bytes=-20", total - 20, total - 1, true}, {"bytes=1-999999", 1, total - 1, true}, {"bytes=1-0", 0, 0, false}, {"bytes=0-1,3-4", 0, 0, false}, {"bytes=", 0, 0, false}, {"bytes=-0", 0, 0, false}} {
		a, b, _, ok := byteRange(tc.value, true, total)
		if ok != tc.ok || ok && (a != tc.start || b != tc.end) {
			t.Fatal("range parser changed")
		}
	}
	lease.Close()
	lease.Close()
	res, err = http.Get(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != 410 || calls.Load() != 3 {
		t.Fatal("revoked lease used")
	}
}
func TestTruncatedStreamClosesAndReportsFixedFailure(t *testing.T) {
	r := testResolver(t, func(*http.Request) (*http.Response, error) {
		res := response("x")
		res.ContentLength = 10
		return res, nil
	})
	remote, _ := url.Parse("https://r.googlevideo.com/x?expire=" + fmt.Sprint(time.Now().Add(time.Hour).Unix()))
	lease, _ := r.publish(remote, format{length: 10, contentType: "audio/mp4"})
	server := httptest.NewServer(lease)
	defer server.Close()
	res, err := http.Get(server.URL)
	if err == nil {
		_, err = io.ReadAll(res.Body)
		res.Body.Close()
	}
	if err == nil || lease.Err() == nil || lease.Err().Error() != "audio_stream_truncated" {
		t.Fatal("truncation became success")
	}
}
func TestCancellationRevokesActiveNetworkAndJoinsConfigure(t *testing.T) {
	began := make(chan struct{})
	r := testResolver(t, func(req *http.Request) (*http.Response, error) {
		close(began)
		<-req.Context().Done()
		return nil, errors.New("synthetic private details")
	})
	if err := r.Configure(context.Background(), Session{Revision: 1}); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- r.Warm(context.Background()) }()
	<-began
	if err := r.Configure(context.Background(), Session{Revision: 2}); err != nil {
		t.Fatal(err)
	}
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatal("active bootstrap not cancelled")
	}
}
func TestDecipherArgumentsAreJSONAndCipherUpdatesURL(t *testing.T) {
	r := testResolver(t, nil)
	r.seed.Code = "const exportedVars={};"
	h := &stubHost{run: func(code string, out any) error {
		if !strings.Contains(code, `process("n-value","sig","quoted\"value")`) {
			t.Error("unescaped transform arguments")
		}
		return json.Unmarshal([]byte(`{"sig":"decoded","n":"transformed"}`), out)
	}}
	r.newHost = func(mode string) (scriptHost, error) {
		if mode != "eval" {
			t.Fatal("unexpected host")
		}
		return h, nil
	}
	cipher := url.Values{"url": {"https://r.googlevideo.com/x?n=n-value&c=WEB_REMIX"}, "s": {"quoted\"value"}, "sp": {"sig"}}
	u, err := r.decipher(context.Background(), format{raw: map[string]any{"signatureCipher": cipher.Encode()}})
	if err != nil || u.Query().Get("sig") != "decoded" || u.Query().Get("n") != "transformed" || u.Query().Get("cver") != musicVersion {
		t.Fatal("cipher transform changed")
	}
}
func TestResponseLimitDoesNotReturnPartialData(t *testing.T) {
	r := testResolver(t, func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: http.Header{}, ContentLength: maxResponse + 1, Body: io.NopCloser(bytes.NewReader(nil))}, nil
	})
	if _, err := r.request(context.Background(), "GET", "https://www.youtube.com/", nil, nil, "proof_init", ""); err == nil || err.Error() != "resolver_response_limit" {
		t.Fatal("response bound lost")
	}
}

func TestProofAndPlayerAgentContractsRemainSeparate(t *testing.T) {
	r := testResolver(t, func(req *http.Request) (*http.Response, error) {
		expected := userAgent
		if req.URL.Path == "/" || req.URL.Host == "www.google.com" || req.URL.Host == "jnn-pa.googleapis.com" {
			expected = proofUserAgent
		}
		if req.Header.Get("User-Agent") != expected {
			t.Error("request agent contract changed")
		}
		return response("fixture"), nil
	})
	for _, tc := range []struct{ method, url, stage string }{
		{"GET", "https://www.youtube.com/", "proof_init"},
		{"GET", "https://www.google.com/js/th/fixture.js", "proof_init"},
		{"POST", "https://jnn-pa.googleapis.com/$rpc/google.internal.waa.v1.Waa/GenerateIT", "proof_init"},
		{"GET", "https://www.youtube.com/sw.js_data", "bootstrap"},
		{"POST", "https://music.youtube.com/youtubei/v1/player", "player_request"},
	} {
		if _, err := r.request(context.Background(), tc.method, tc.url, nil, nil, tc.stage, ""); err != nil {
			t.Fatal("fixture request failed")
		}
	}
}

func TestPausedConsumerRetainsMediaStream(t *testing.T) {
	if testing.Short() {
		t.Skip("exercises a real pause longer than the network idle limit")
	}
	const total int64 = 64 << 20
	r := testResolver(t, func(req *http.Request) (*http.Response, error) {
		var start, end int64
		if _, err := fmt.Sscanf(req.URL.Query().Get("range"), "%d-%d", &start, &end); err != nil {
			t.Fatal("range fixture")
		}
		res := response(strings.Repeat("x", int(end-start+1)))
		res.StatusCode = 206
		res.Header.Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, total))
		return res, nil
	})
	remote, _ := url.Parse("https://r.googlevideo.com/x?expire=" + fmt.Sprint(time.Now().Add(time.Hour).Unix()))
	lease, err := r.publish(remote, format{length: total, contentType: "audio/mp4"})
	if err != nil {
		t.Fatal("publish")
	}
	server := httptest.NewServer(lease)
	defer server.Close()
	address := strings.TrimPrefix(server.URL, "http://")
	conn, err := net.DialTimeout("tcp", address, 2*time.Second)
	if err != nil {
		t.Fatal("connect")
	}
	defer conn.Close()
	if tcp, ok := conn.(*net.TCPConn); ok {
		_ = tcp.SetReadBuffer(128 << 10)
	}
	_ = conn.SetDeadline(time.Now().Add(35 * time.Second))
	req, _ := http.NewRequest("GET", server.URL, nil)
	if req.Write(conn) != nil {
		t.Fatal("request")
	}
	res, err := http.ReadResponse(bufio.NewReader(conn), req)
	if err != nil {
		t.Fatal("headers")
	}
	defer res.Body.Close()
	// A paused decoder or prepared next track intentionally stops reading.
	// This is downstream backpressure, not an upstream network timeout.
	time.Sleep(22 * time.Second)
	n, err := io.Copy(io.Discard, res.Body)
	if err != nil || n != total {
		t.Fatalf("paused stream truncated: received=%d expected=%d", n, total)
	}
}
