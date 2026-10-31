package streaming

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"strings"
	"time"

	"ytmgo/internal/netproxy"
)

func transportClient(input *http.Client) *http.Client {
	var c http.Client
	if input != nil {
		c = *input
	} else {
		tr := http.DefaultTransport.(*http.Transport).Clone()
		tr.Proxy = netproxy.Proxy
		tr.MaxIdleConns = 4
		tr.MaxIdleConnsPerHost = 2
		tr.MaxConnsPerHost = 4
		tr.ResponseHeaderTimeout = 15 * time.Second
		tr.MaxResponseHeaderBytes = 32 << 10
		tr.TLSHandshakeTimeout = 10 * time.Second
		tr.DisableCompression = true
		c.Transport = tr
	}
	c.Jar = nil
	c.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &c
}
func allowedRequest(u *url.URL, method string) bool {
	if u.Scheme != "https" || u.User != nil || u.Port() != "" || u.Fragment != "" || u.RawPath != "" || path.Clean(u.Path) != u.Path {
		return false
	}
	switch u.Host {
	case "www.youtube.com":
		return method == "GET" && (u.Path == "/" || u.Path == "/sw.js_data" || u.Path == "/iframe_api" || strings.HasPrefix(u.Path, "/s/player/"))
	case "music.youtube.com":
		return method == "GET" && u.Path == "/" && u.RawQuery == "" || method == "POST" && (u.Path == "/youtubei/v1/player" || u.Path == "/youtubei/v1/browse")
	case "www.google.com":
		return method == "GET" && strings.HasPrefix(u.Path, "/js/th/")
	case "jnn-pa.googleapis.com":
		return method == "POST" && u.Path == "/$rpc/google.internal.waa.v1.Waa/GenerateIT"
	}
	return false
}
func (r *Resolver) request(ctx context.Context, method, raw string, body []byte, headers http.Header, stage, step string) ([]byte, error) {
	parent := ctx
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, method, raw, bytes.NewReader(body))
	if err != nil || !allowedRequest(req.URL, method) {
		return nil, failure("unexpected_request_host", stage)
	}
	req.Header = headers.Clone()
	if req.Header == nil {
		req.Header = make(http.Header)
	}
	req.Header.Set("User-Agent", userAgent)
	if stage == "proof_init" {
		req.Header.Set("User-Agent", proofUserAgent)
	}
	req.Header.Set("Accept-Encoding", "identity")
	if req.Header.Get("Accept") == "" {
		req.Header.Set("Accept", "*/*")
	}
	if req.Header.Get("Accept-Language") == "" {
		req.Header.Set("Accept-Language", "*")
	}
	req.Header.Del("Proxy-Authorization")
	bootstrapCookie := req.URL.Host == "www.youtube.com" && req.URL.Path == "/sw.js_data" && regexp.MustCompile(`^PREF=tz=UTC;VISITOR_INFO1_LIVE=[A-Za-z0-9_-]{11};$`).MatchString(req.Header.Get("Cookie"))
	if req.URL.Host != "music.youtube.com" && (req.Header.Get("Authorization") != "" || req.Header.Get("Cookie") != "" && !bootstrapCookie) {
		return nil, failure("unexpected_authentication", stage)
	}
	res, err := r.client.Do(req)
	if err != nil {
		if parent.Err() != nil {
			return nil, parent.Err()
		}
		category := "resolver_http_error"
		if stage == "bootstrap" {
			category = "bootstrap_network_failed"
			var ne net.Error
			if ctx.Err() != nil || errors.As(err, &ne) && ne.Timeout() {
				category = "bootstrap_network_timeout"
			}
			var cert *tls.CertificateVerificationError
			if errors.As(err, &cert) {
				category = "bootstrap_tls_failed"
			}
		}
		return nil, &Failure{Category: category, Stage: stage, BootstrapStep: step}
	}
	defer res.Body.Close()
	d := &Failure{Stage: stage, BootstrapStep: step, HTTPStatus: res.StatusCode, MIME: mimeClass(res.Header.Get("Content-Type"))}
	if res.StatusCode >= 300 && res.StatusCode < 400 {
		d.Category = "unexpected_request_host"
		if stage == "bootstrap" {
			d.Category = "bootstrap_redirect"
			d.Redirect = redirectClass(res.Header.Get("Location"), req.URL)
		}
		return nil, d
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		d.Category = "resolver_http_error"
		if res.StatusCode == 401 || res.StatusCode == 403 {
			d.Category = "resolver_access_denied"
		}
		if res.StatusCode == 429 {
			d.Category = "resolver_rate_limited"
		}
		return nil, d
	}
	if res.ContentLength > maxResponse {
		return nil, failure("resolver_response_limit", stage)
	}
	data, err := io.ReadAll(io.LimitReader(res.Body, maxResponse+1))
	if err != nil {
		if parent.Err() != nil {
			return nil, parent.Err()
		}
		if stage == "bootstrap" && ctx.Err() != nil {
			return nil, &Failure{Category: "bootstrap_network_timeout", Stage: stage, BootstrapStep: step}
		}
		return nil, failure("resolver_http_error", stage)
	}
	if len(data) > maxResponse {
		return nil, failure("resolver_response_limit", stage)
	}
	return data, nil
}
func mimeClass(s string) string {
	switch {
	case strings.Contains(s, "text/html"):
		return "html"
	case strings.Contains(s, "javascript"):
		return "javascript"
	case strings.Contains(s, "json"):
		return "json"
	case strings.HasPrefix(s, "text/"):
		return "text"
	case s == "":
		return "missing"
	}
	return "other"
}
func redirectClass(s string, base *url.URL) string {
	if len(s) > 8192 || strings.ContainsAny(s, "\\\x00\r\n\t ") {
		return "other"
	}
	u, err := base.Parse(s)
	if err != nil || u.User != nil {
		return "other"
	}
	if u.Scheme == "https" && u.Port() == "" && !strings.Contains(strings.Split(s, "?")[0], "%") {
		if (u.Host == "www.google.com" || u.Host == "google.com") && (u.Path == "/sorry" || strings.HasPrefix(u.Path, "/sorry/")) {
			return "google_challenge"
		}
		if u.Host == "accounts.google.com" && (strings.HasPrefix(u.Path, "/ServiceLogin") || strings.HasPrefix(u.Path, "/signin") || strings.HasPrefix(u.Path, "/v3/signin") || strings.HasPrefix(u.Path, "/InteractiveLogin")) {
			return "signin"
		}
		if (u.Host == "consent.google.com" || u.Host == "consent.youtube.com") && (u.Path == "/m" || u.Path == "/ml") {
			return "consent"
		}
	}
	if u.Scheme == base.Scheme && u.Host == base.Host {
		return "same_origin"
	}
	return "other"
}
func (r *Resolver) api(ctx context.Context, endpoint string, payload map[string]any, stage string) (map[string]any, error) {
	client := map[string]any{}
	for k, v := range r.sessionClient {
		client[k] = v
	}
	if r.binding.Visitor != "" {
		client["visitorData"] = r.binding.Visitor
	}
	user := map[string]any{"lockedSafetyMode": false, "enableSafetyMode": false}
	if r.session.OnBehalfOfUser != "" {
		user["onBehalfOfUser"] = r.session.OnBehalfOfUser
	}
	payload["context"] = map[string]any{"client": client, "user": user, "request": map[string]any{"useSsl": true, "internalExperimentFlags": []any{}}}
	data, _ := json.Marshal(payload)
	headers := http.Header{"Content-Type": {"application/json"}, "Origin": {musicOrigin}, "Referer": {musicOrigin + "/"}, "X-Youtube-Client-Name": {"67"}, "X-Youtube-Client-Version": {musicVersion}}
	req, _ := http.NewRequest("POST", musicOrigin+"/youtubei/v1/"+endpoint, nil)
	req.Header = headers
	r.session.authorize(req, true)
	if visitor, _ := client["visitorData"].(string); visitor != "" {
		headers.Set("X-Goog-Visitor-Id", visitor)
	}
	raw, err := r.request(ctx, "POST", musicOrigin+"/youtubei/v1/"+endpoint+"?prettyPrint=false&alt=json", data, headers, stage, "")
	if err != nil {
		return nil, err
	}
	var response map[string]any
	if json.Unmarshal(raw, &response) != nil || response == nil || response["error"] != nil {
		return nil, failure("resolver_http_error", stage)
	}
	if r.session.LoggedIn && valueAt(response, "responseContext", "mainAppWebResponseContext", "loggedOut") == true {
		return nil, failure("login_required", stage)
	}
	return response, nil
}
func valueAt(v any, path ...string) any {
	for _, key := range path {
		m, ok := v.(map[string]any)
		if !ok {
			return nil
		}
		v = m[key]
	}
	return v
}
func textAt(v any, path ...string) string { s, _ := valueAt(v, path...).(string); return s }
