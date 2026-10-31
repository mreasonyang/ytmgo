package netproxy

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"golang.org/x/net/http/httpproxy"
)

func TestProxySelection(t *testing.T) {
	for _, tt := range []struct {
		name, target  string
		config        httpproxy.Config
		want          string
		platformCalls int
	}{
		{"system HTTPS", "https://music.example.test/", httpproxy.Config{}, "http://system.invalid:8000", 1},
		{"explicit HTTPS", "https://music.example.test/", httpproxy.Config{HTTPSProxy: "http://explicit.invalid:9000"}, "http://explicit.invalid:9000", 0},
		{"explicit HTTP", "http://music.example.test/", httpproxy.Config{HTTPProxy: "http://explicit.invalid:9000"}, "http://explicit.invalid:9000", 0},
		{"scheme-specific fallback", "https://music.example.test/", httpproxy.Config{HTTPProxy: "http://explicit.invalid:9000"}, "http://system.invalid:8000", 1},
		{"system bypass", "https://music.example.test/", httpproxy.Config{NoProxy: ".example.test"}, "", 0},
		{"explicit bypass", "https://music.example.test/", httpproxy.Config{HTTPSProxy: "http://explicit.invalid:9000", NoProxy: "*"}, "", 0},
		{"loopback IPv4", "http://127.0.0.1:8000/", httpproxy.Config{HTTPProxy: "http://explicit.invalid:9000"}, "", 0},
		{"loopback IPv6", "http://[::1]:8000/", httpproxy.Config{}, "", 0},
		{"localhost", "http://localhost:8000/", httpproxy.Config{}, "", 0},
	} {
		t.Run(tt.name, func(t *testing.T) {
			calls := 0
			choose := resolver(tt.config, func(*url.URL) (*url.URL, error) { calls++; return url.Parse("http://system.invalid:8000") })
			req, _ := http.NewRequest("GET", tt.target, nil)
			got, err := choose(req)
			actual := ""
			if got != nil {
				actual = got.String()
			}
			if err != nil || actual != tt.want || calls != tt.platformCalls {
				t.Fatal("proxy route mismatch")
			}
		})
	}
}

func TestPlatformProxyCarriesHTTPRequest(t *testing.T) {
	seen := make(chan bool, 1)
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen <- r.URL.Host == "origin.invalid" && r.URL.Path == "/fixture" && r.Header.Get("Proxy-Authorization") == ""
		_, _ = io.WriteString(w, "fixture response")
	}))
	defer proxy.Close()
	u, _ := url.Parse(proxy.URL)
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = resolver(httpproxy.Config{}, func(*url.URL) (*url.URL, error) { return u, nil })
	defer transport.CloseIdleConnections()
	client := http.Client{Transport: transport}
	response, err := client.Get("http://origin.invalid/fixture")
	if err != nil {
		t.Fatal("configured proxy request failed")
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil || string(body) != "fixture response" || !<-seen {
		t.Fatal("request bypassed configured proxy")
	}
}

func TestProxyErrorsDoNotExposeConfiguration(t *testing.T) {
	choose := resolver(httpproxy.Config{}, func(*url.URL) (*url.URL, error) { return nil, errors.New("synthetic-private-configuration") })
	req, _ := http.NewRequest("GET", "https://origin.invalid/", nil)
	if _, err := choose(req); err != errProxy {
		t.Fatal("unsanitized proxy error")
	}
}
