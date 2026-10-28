package player

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestStreamBridgeCapabilityHeadersAndRanges(t *testing.T) {
	fixture := bytes.Repeat([]byte("0123456789abcdef"), 256)
	var calls atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Header.Get("User-Agent") != "bridge-test" || r.Header.Get("Referer") != "https://www.youtube.com/" || r.Header.Get("Accept-Encoding") != "identity" {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		http.ServeContent(w, r, "fixture", time.Time{}, bytes.NewReader(fixture))
	}))
	defer upstream.Close()
	b, err := newStreamBridge(context.Background(), media{URL: upstream.URL, UserAgent: "bridge-test", Referer: "https://www.youtube.com/"})
	if err != nil {
		t.Fatal("bridge unavailable")
	}
	defer b.Close()
	u, _ := url.Parse(b.URL)
	if u.Hostname() != "127.0.0.1" || len(u.Path) != 65 {
		t.Fatal("invalid local capability")
	}
	for _, test := range []struct {
		path, method string
		status       int
	}{
		{"/missing", "GET", http.StatusNotFound},
		{u.Path + "?unexpected=1", "GET", http.StatusNotFound},
		{u.Path, "POST", http.StatusMethodNotAllowed},
	} {
		req, _ := http.NewRequest(test.method, "http://"+u.Host+test.path, nil)
		response, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal("local request failed")
		}
		response.Body.Close()
		if response.StatusCode != test.status {
			t.Fatal("capability accepted invalid request")
		}
	}
	if calls.Load() != 0 {
		t.Fatal("invalid capability reached upstream")
	}
	req, _ := http.NewRequest(http.MethodGet, b.URL, nil)
	req.Header.Set("Range", "bytes=1024-2047")
	response, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal("range request failed")
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, 2048))
	response.Body.Close()
	if err != nil || response.StatusCode != http.StatusPartialContent || response.Header.Get("Content-Range") != "bytes 1024-2047/4096" || !bytes.Equal(body, fixture[1024:2048]) {
		t.Fatal("range response was not preserved")
	}
	req, _ = http.NewRequest(http.MethodHead, b.URL, nil)
	response, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal("head request failed")
	}
	response.Body.Close()
	if response.ContentLength != int64(len(fixture)) || response.StatusCode != http.StatusOK {
		t.Fatal("head metadata missing")
	}
	if b.Err() != nil {
		t.Fatal("successful transport recorded an error")
	}
}

func TestStreamBridgeBoundedConcurrencyAndCancel(t *testing.T) {
	var active, maximum atomic.Int64
	started := make(chan struct{}, 3)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := active.Add(1)
		defer active.Add(-1)
		for old := maximum.Load(); count > old && !maximum.CompareAndSwap(old, count); old = maximum.Load() {
		}
		started <- struct{}{}
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	defer upstream.Close()
	ctx, cancel := context.WithCancel(context.Background())
	b, err := newStreamBridge(ctx, media{URL: upstream.URL})
	if err != nil {
		t.Fatal("bridge unavailable")
	}
	defer b.Close()
	var readers sync.WaitGroup
	for range 3 {
		readers.Add(1)
		go func() {
			defer readers.Done()
			response, err := http.Get(b.URL)
			if err == nil {
				_, _ = io.Copy(io.Discard, response.Body)
				response.Body.Close()
			}
		}()
	}
	for range 2 {
		select {
		case <-started:
		case <-time.After(time.Second):
			t.Fatal("upstream did not start")
		}
	}
	select {
	case <-started:
		t.Fatal("more than two upstream streams")
	case <-time.After(50 * time.Millisecond):
	}
	begin := time.Now()
	cancel()
	b.Close()
	readers.Wait()
	if time.Since(begin) > time.Second || maximum.Load() > 2 {
		t.Fatal("bridge cancellation or stream bound failed")
	}
	u, _ := url.Parse(b.URL)
	connection, err := net.DialTimeout("tcp", u.Host, 100*time.Millisecond)
	if err == nil {
		connection.Close()
		t.Fatal("bridge listener survived Close")
	}
	deadline := time.Now().Add(time.Second)
	for active.Load() != 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if active.Load() != 0 {
		t.Fatal("upstream connection survived Close")
	}
}

func TestStreamBridgeErrorsAndRetryBounds(t *testing.T) {
	for _, test := range []struct {
		status   int
		attempts int64
		category string
	}{
		{http.StatusForbidden, 1, "audio_access_denied"},
		{http.StatusTooManyRequests, 3, "audio_rate_limited"},
		{http.StatusServiceUnavailable, 3, "audio_server_error"},
	} {
		t.Run(test.category, func(t *testing.T) {
			var calls atomic.Int64
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodHead {
					calls.Add(1)
				}
				w.WriteHeader(test.status)
				_, _ = io.WriteString(w, "private upstream diagnostic must not be forwarded")
			}))
			defer upstream.Close()
			b, err := newStreamBridge(context.Background(), media{URL: upstream.URL})
			if err != nil {
				t.Fatal("bridge unavailable")
			}
			defer b.Close()
			response, err := http.Get(b.URL)
			if err != nil {
				t.Fatal("local request failed")
			}
			body, _ := io.ReadAll(io.LimitReader(response.Body, 1024))
			response.Body.Close()
			if response.StatusCode != test.status || len(body) != 0 || calls.Load() != test.attempts || b.Err() == nil || b.Err().Error() != test.category {
				t.Fatal("HTTP failure classification or retry bound incorrect")
			}
		})
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := newStreamBridge(ctx, media{URL: "https://example.invalid"}); !errors.Is(err, context.Canceled) {
		t.Fatal("accepted canceled bridge")
	}
	if _, err := newStreamBridge(context.Background(), media{URL: "file:/tmp/media"}); err == nil {
		t.Fatal("accepted alternate transport")
	}
}

func TestStreamBridgePostHeaderFailureUsesOnlyFixedLocalStatus(t *testing.T) {
	for _, test := range []struct{ header, category string }{
		{"upstream_access_denied", "audio_access_denied"},
		{"upstream_rate_limited", "audio_rate_limited"},
		{"upstream_http_error", "audio_server_error"},
		{"upstream_timeout", "audio_network_timeout"},
		{"upstream_connection_failed", "audio_connection_failed"},
		{"upstream_range_mismatch", "audio_range_mismatch"},
		{"upstream_truncated", "audio_stream_truncated"},
		{"synthetic-private-diagnostic", "audio_connection_failed"},
		{"upstream_access_denied synthetic-private-diagnostic", "audio_connection_failed"},
	} {
		t.Run(test.header, func(t *testing.T) {
			var gets, heads atomic.Int64
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Length", "65536")
				if r.Method == http.MethodHead {
					heads.Add(1)
					w.Header().Set("X-Ytmgo-Stream-Error", test.header)
					w.WriteHeader(http.StatusOK)
					return
				}
				gets.Add(1)
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("synthetic-pcm"))
				w.(http.Flusher).Flush()
				// Return before the declared length: only the private Node status
				// distinguishes a later CDN refusal from a broken local socket.
			}))
			defer upstream.Close()
			bridge, err := newStreamBridge(context.Background(), media{URL: upstream.URL})
			if err != nil {
				t.Fatal("bridge unavailable")
			}
			defer bridge.Close()
			response, err := http.Get(bridge.URL)
			if err != nil {
				t.Fatal("local request unavailable")
			}
			_, _ = io.Copy(io.Discard, response.Body)
			response.Body.Close()
			if response.Header.Get("X-Ytmgo-Stream-Error") != "" || response.ContentLength != 65536 ||
				gets.Load() != 1 || heads.Load() != 1 || bridge.Err() == nil || bridge.Err().Error() != test.category {
				t.Fatal("fixed status handoff or range metadata failed")
			}
		})
	}
}
