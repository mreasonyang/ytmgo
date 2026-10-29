package player

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"sync"
	"time"
)

var byteRangePattern = regexp.MustCompile(`^bytes=[0-9]*-[0-9]*$`)

// streamBridge relays the worker's local transport to FFmpeg. Remote media
// requests and signed URLs belong exclusively to the Node worker.
// Each response streams through 32 KiB with backpressure, with at most two
// upstream requests. There is no media cache or complete-track buffer.
type streamBridge struct {
	URL       string
	path      string
	info      media
	ctx       context.Context
	cancel    context.CancelFunc
	client    *http.Client
	transport *http.Transport
	server    *http.Server
	done      chan struct{}
	slots     chan struct{}
	once      sync.Once
	mu        sync.Mutex
	closed    bool
	category  string
	requests  sync.WaitGroup
}

func newStreamBridge(parent context.Context, info media) (*streamBridge, error) {
	if err := parent.Err(); err != nil {
		return nil, err
	}
	u, err := url.Parse(info.URL)
	if err != nil || u.Scheme != "http" || u.Hostname() != "127.0.0.1" || u.Port() == "" || u.User != nil ||
		!validHeaderValue(info.UserAgent) || !validHeaderValue(info.Referer) {
		return nil, errors.New("invalid_media_response")
	}
	var secret [32]byte
	if _, err := rand.Read(secret[:]); err != nil {
		return nil, errors.New("audio_decoder_unavailable")
	}
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return nil, errors.New("audio_decoder_unavailable")
	}
	ctx, cancel := context.WithCancel(parent)
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	transport.DisableCompression = true
	transport.MaxConnsPerHost, transport.MaxIdleConnsPerHost = 2, 2
	transport.MaxIdleConns = 2
	transport.ResponseHeaderTimeout = 10 * time.Second
	transport.MaxResponseHeaderBytes = 8 << 10
	transport.TLSHandshakeTimeout = 10 * time.Second
	transport.DialContext = (&net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}).DialContext
	b := &streamBridge{path: "/" + hex.EncodeToString(secret[:]), info: info, ctx: ctx, cancel: cancel,
		transport: transport, done: make(chan struct{}), slots: make(chan struct{}, 2)}
	b.URL = "http://" + listener.Addr().String() + b.path
	b.client = &http.Client{Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	b.server = &http.Server{Handler: http.HandlerFunc(b.serve), ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout: 5 * time.Second, MaxHeaderBytes: 8 << 10, ErrorLog: log.New(io.Discard, "", 0)}
	go func() { defer close(b.done); _ = b.server.Serve(listener) }()
	context.AfterFunc(ctx, func() { _ = b.Close() })
	return b, nil
}

func (b *streamBridge) serve(w http.ResponseWriter, incoming *http.Request) {
	if incoming.URL.Path != b.path || incoming.URL.RawQuery != "" {
		http.NotFound(w, incoming)
		return
	}
	if incoming.Method != http.MethodGet && incoming.Method != http.MethodHead {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	rangeValue := incoming.Header.Get("Range")
	if rangeValue != "" && (len(rangeValue) > 128 || !byteRangePattern.MatchString(rangeValue)) {
		w.WriteHeader(http.StatusRequestedRangeNotSatisfiable)
		return
	}
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	b.requests.Add(1)
	b.mu.Unlock()
	defer b.requests.Done()
	ctx, cancel := context.WithCancel(incoming.Context())
	stop := context.AfterFunc(b.ctx, cancel)
	defer func() { stop(); cancel() }()
	select {
	case b.slots <- struct{}{}:
		defer func() { <-b.slots }()
	case <-ctx.Done():
		return
	}
	var response *http.Response
	var err error
	for attempt := 0; attempt < 3; attempt++ {
		request, requestErr := http.NewRequestWithContext(ctx, incoming.Method, b.info.URL, nil)
		if requestErr != nil {
			b.setError("audio_stream_failed")
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		request.Header.Set("User-Agent", b.info.UserAgent)
		request.Header.Set("Accept-Encoding", "identity")
		if b.info.Referer != "" {
			request.Header.Set("Referer", b.info.Referer)
		}
		if rangeValue != "" {
			request.Header.Set("Range", rangeValue)
		}
		response, err = b.client.Do(request)
		retry := err != nil || response.StatusCode == http.StatusTooManyRequests || response.StatusCode >= 500
		if !retry || attempt == 2 || ctx.Err() != nil {
			break
		}
		if response != nil {
			response.Body.Close()
		}
		timer := time.NewTimer(time.Duration(attempt+1) * 250 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
	if err != nil {
		if ctx.Err() != nil {
			return
		}
		category := "audio_connection_failed"
		var netErr net.Error
		if errors.As(err, &netErr) && netErr.Timeout() {
			category = "audio_network_timeout"
		}
		b.setError(category)
		w.WriteHeader(http.StatusBadGateway)
		return
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK && response.StatusCode != http.StatusPartialContent {
		category := "audio_stream_failed"
		switch {
		case response.StatusCode == http.StatusForbidden || response.StatusCode == http.StatusUnauthorized:
			category = "audio_access_denied"
		case response.StatusCode == http.StatusTooManyRequests:
			category = "audio_rate_limited"
		case response.StatusCode >= 500:
			category = "audio_server_error"
		}
		response.Body.Close()
		// A final HTTP refusal already identifies the failure. Only a generic
		// server failure needs the worker's more specific transport status;
		// probing a 401/403/429 would issue an extra request after its retry
		// policy has finished and could overwrite that definitive category.
		if response.StatusCode >= 500 {
			if reported := b.reportedStreamError(); reported != "" {
				category = reported
			}
		}
		b.setError(category)
		w.WriteHeader(response.StatusCode)
		return
	}
	for _, name := range []string{"Content-Type", "Content-Length", "Content-Range", "Accept-Ranges"} {
		if value := response.Header.Get(name); value != "" {
			w.Header().Set(name, value)
		}
	}
	w.WriteHeader(response.StatusCode)
	if incoming.Method == http.MethodGet {
		// Wrappers deliberately prevent io.CopyBuffer's ReaderFrom/WriterTo
		// shortcuts from selecting an unbounded transport-specific buffer.
		_, copyErr := io.CopyBuffer(struct{ io.Writer }{w}, struct{ io.Reader }{response.Body}, make([]byte, 32<<10))
		if copyErr != nil && ctx.Err() == nil {
			response.Body.Close()
			category := b.reportedStreamError()
			if category == "" {
				category = "audio_connection_failed"
			}
			b.setError(category)
		}
	}
}

// Once response bytes have been sent, HTTP cannot replace the status code.
// Query this exact local capability for a fixed status, without changing its
// content length, replaying a remote range, or inspecting a dependency error.
func (b *streamBridge) reportedStreamError() string {
	ctx, cancel := context.WithTimeout(b.ctx, time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodHead, b.info.URL, nil)
	if err != nil {
		return ""
	}
	request.Header.Set("Accept-Encoding", "identity")
	response, err := b.client.Do(request)
	if err != nil {
		return ""
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK && response.StatusCode != http.StatusPartialContent {
		return ""
	}
	switch response.Header.Get("X-Ytmgo-Stream-Error") {
	case "upstream_access_denied":
		return "audio_access_denied"
	case "upstream_rate_limited":
		return "audio_rate_limited"
	case "upstream_http_error":
		return "audio_server_error"
	case "upstream_timeout":
		return "audio_network_timeout"
	case "upstream_connection_failed":
		return "audio_connection_failed"
	case "upstream_range_mismatch":
		return "audio_range_mismatch"
	case "upstream_truncated":
		return "audio_stream_truncated"
	default:
		return ""
	}
}

func (b *streamBridge) setError(category string) {
	b.mu.Lock()
	b.category = category
	b.mu.Unlock()
}

func (b *streamBridge) Err() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.category == "" {
		return nil
	}
	return errors.New(b.category)
}

func (b *streamBridge) Close() error {
	b.once.Do(func() {
		b.mu.Lock()
		b.closed = true
		b.mu.Unlock()
		b.cancel()
		_ = b.server.Close()
		b.transport.CloseIdleConnections()
		<-b.done
		b.requests.Wait()
	})
	return nil
}
