package streaming

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Lease contains a signed URL only in private Go memory. Seek decoders share
// its lifetime. Revocation cancels every request and clears the capability.
type Lease struct {
	owner       *Resolver
	mu          sync.Mutex
	remote      *url.URL
	length      int64
	contentType string
	life        context.Context
	cancel      context.CancelFunc
	timer       *time.Timer
	category    string
	httpStatus  int
	once        sync.Once
	requests    sync.WaitGroup
}

func (r *Resolver) publish(remote *url.URL, f format) (*Lease, error) {
	expires, err := strconv.ParseInt(remote.Query().Get("expire"), 10, 64)
	if err != nil || expires <= 0 || expires > 253402300799 {
		return nil, errors.New("media_expired")
	}
	lifetime := min(26*time.Hour, time.Until(time.Unix(expires, 0).Add(-30*time.Second)))
	if lifetime <= 0 {
		return nil, errors.New("media_expired")
	}
	life, cancel := context.WithCancel(r.life)
	l := &Lease{owner: r, remote: remote, length: f.length, contentType: f.contentType, life: life, cancel: cancel}
	r.leaseMu.Lock()
	defer r.leaseMu.Unlock()
	if len(r.leases) >= 4 {
		cancel()
		return nil, errors.New("capacity_limit")
	}
	r.leases[l] = struct{}{}
	l.timer = time.AfterFunc(lifetime, l.Close)
	return l, nil
}
func (l *Lease) Close() {
	l.once.Do(func() {
		l.cancel()
		l.mu.Lock()
		l.remote = nil
		l.mu.Unlock()
		l.requests.Wait()
		l.owner.leaseMu.Lock()
		if l.timer != nil {
			l.timer.Stop()
		}
		delete(l.owner.leases, l)
		l.owner.leaseMu.Unlock()
	})
}
func (l *Lease) Err() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.category == "" {
		return nil
	}
	return &Failure{Category: l.category, Stage: "media_stream", HTTPStatus: l.httpStatus}
}
func (l *Lease) fail(category string, status ...int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.category == "" {
		l.category = category
		if len(status) == 1 && status[0] >= 100 && status[0] <= 599 {
			l.httpStatus = status[0]
		}
	}
}

func byteRange(value string, present bool, total int64) (start, end int64, partial, ok bool) {
	if total <= 0 {
		return
	}
	end = total - 1
	if !present {
		return 0, end, false, true
	}
	if len(value) > 128 || !strings.HasPrefix(value, "bytes=") {
		return 0, 0, false, false
	}
	a, b, found := strings.Cut(strings.TrimPrefix(value, "bytes="), "-")
	if !found || a == "" && b == "" {
		return 0, 0, false, false
	}
	parse := func(s string) (int64, error) {
		for _, c := range s {
			if c < '0' || c > '9' {
				return 0, errors.New("range")
			}
		}
		return strconv.ParseInt(s, 10, 64)
	}
	var err error
	if a == "" {
		suffix, e := parse(b)
		if e != nil || suffix <= 0 {
			return 0, 0, false, false
		}
		start = max(0, total-suffix)
	} else {
		start, err = parse(a)
		if err != nil || start >= total {
			return 0, 0, false, false
		}
		if b != "" {
			end, err = parse(b)
			if err != nil || end < start {
				return 0, 0, false, false
			}
			end = min(end, total-1)
		}
	}
	return start, end, true, true
}
func streamError(err error) string {
	var ne net.Error
	if errors.Is(err, context.DeadlineExceeded) || errors.As(err, &ne) && ne.Timeout() {
		return "audio_network_timeout"
	}
	return "audio_connection_failed"
}
func (l *Lease) rangeRequest(ctx context.Context, remote *url.URL, start, end int64) (*http.Response, context.CancelFunc, error) {
	u := *remote
	for attempt := 0; attempt < 3; attempt++ {
		// A new timeout belongs to each chunk, never to the whole track.
		requestCtx, cancel := context.WithCancel(ctx)
		timer := time.AfterFunc(20*time.Second, cancel)
		res, err := l.requestRangeWithRedirects(requestCtx, u, start, end)
		stopped := timer.Stop()
		if !stopped && ctx.Err() == nil {
			err = context.DeadlineExceeded
		}
		retry := err != nil || res != nil && (res.StatusCode == 429 || res.StatusCode >= 500)
		if !retry || attempt == 2 || ctx.Err() != nil {
			return res, cancel, err
		}
		if res != nil {
			res.Body.Close()
		}
		cancel()
		wait := time.NewTimer(time.Duration(attempt+1) * 250 * time.Millisecond)
		select {
		case <-ctx.Done():
			wait.Stop()
			return nil, func() {}, ctx.Err()
		case <-wait.C:
		}
	}
	return nil, func() {}, errors.New("audio_connection_failed")
}

// Only media requests may follow CDN relocation. Each hop is independently
// validated against the same media-origin boundary as the original lease;
// account/protocol clients still refuse redirects. Signed URLs stay in Go.
func (l *Lease) requestRangeWithRedirects(ctx context.Context, endpoint url.URL, start, end int64) (*http.Response, error) {
	for hop := 0; ; hop++ {
		q := endpoint.Query()
		q.Set("range", fmt.Sprintf("%d-%d", start, end))
		endpoint.RawQuery = q.Encode()
		req, err := http.NewRequestWithContext(ctx, "GET", endpoint.String(), nil)
		if err != nil {
			return nil, errors.New("audio_connection_failed")
		}
		req.Header = http.Header{"Accept": {"*/*"}, "Accept-Encoding": {"identity"}, "Origin": {"https://www.youtube.com"}, "Referer": {"https://www.youtube.com"}, "Dnt": {"?1"}}
		res, err := l.owner.client.Do(req)
		if err != nil {
			return res, err
		}
		switch res.StatusCode {
		case 301, 302, 303, 307, 308:
		default:
			return res, nil
		}
		if hop >= 3 {
			return res, nil
		}
		location := res.Header.Get("Location")
		if len(location) == 0 || len(location) > 65536 {
			return res, nil
		}
		next, err := endpoint.Parse(location)
		if err != nil {
			return res, nil
		}
		validated, err := mediaURL(next.String())
		if err != nil {
			return res, nil
		}
		res.Body.Close()
		endpoint = *validated
	}
}

// ServeHTTP is mounted behind a per-decoder random loopback capability. It
// performs bounded direct HTTPS ranges, without a second HTTP relay or cache.
func (l *Lease) ServeHTTP(w http.ResponseWriter, in *http.Request) {
	l.mu.Lock()
	remote := l.remote
	if remote != nil && l.life.Err() == nil {
		l.requests.Add(1)
	} else {
		remote = nil
	}
	l.mu.Unlock()
	stop := func(status int) { w.Header().Set("Content-Length", "0"); w.WriteHeader(status) }
	if remote == nil || l.life.Err() != nil {
		if remote != nil {
			l.requests.Done()
		}
		stop(http.StatusGone)
		return
	}
	defer l.requests.Done()
	if in.Method != "GET" && in.Method != "HEAD" {
		w.Header().Set("Allow", "GET, HEAD")
		stop(405)
		return
	}
	ranges := in.Header.Values("Range")
	if len(ranges) > 1 {
		stop(416)
		return
	}
	start, end, partial, ok := byteRange(in.Header.Get("Range"), len(ranges) > 0, l.length)
	if !ok {
		w.Header().Set("Content-Range", fmt.Sprintf("bytes */%d", l.length))
		stop(416)
		return
	}
	headers := func() {
		w.Header().Set("Accept-Ranges", "bytes")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Type", l.contentType)
		w.Header().Set("Content-Length", strconv.FormatInt(end-start+1, 10))
		if partial {
			w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, l.length))
			w.WriteHeader(206)
		} else {
			w.WriteHeader(200)
		}
	}
	if in.Method == "HEAD" {
		headers()
		return
	}
	ctx, cancel := context.WithCancel(in.Context())
	defer cancel()
	detach := context.AfterFunc(l.life, cancel)
	defer detach()
	controller := http.NewResponseController(w)
	aborted := make(chan struct{})
	abortWrite := context.AfterFunc(ctx, func() {
		_ = controller.SetWriteDeadline(time.Now())
		close(aborted)
	})
	defer func() {
		if !abortWrite() {
			<-aborted
		}
	}()
	select {
	case l.owner.slots <- struct{}{}:
		defer func() { <-l.owner.slots }()
	case <-ctx.Done():
		stop(410)
		return
	}
	sent := false
	fail := func(category string, status int, upstreamStatus ...int) {
		if ctx.Err() == nil {
			l.fail(category, upstreamStatus...)
		}
		if sent {
			panic(http.ErrAbortHandler)
		}
		stop(status)
	}
	buffer := make([]byte, 32<<10)
	chunk := int64(64 << 10)
	if l.contentType == "video/mp4" {
		chunk = 512 << 10
	}
	last, bodyRetries := start-1, 0
	for offset := start; offset <= end; {
		if ctx.Err() != nil {
			fail("", 410)
			return
		}
		if offset > last {
			last, bodyRetries = min(end, offset+chunk-1), 0
		}
		expected := last - offset + 1
		res, cancelRequest, err := l.rangeRequest(ctx, remote, offset, last)
		if err != nil {
			if res != nil {
				res.Body.Close()
			}
			cancelRequest()
			fail(streamError(err), 502)
			return
		}
		if res.StatusCode != 200 && res.StatusCode != 206 {
			category, status := "audio_server_error", 502
			if res.StatusCode == 401 || res.StatusCode == 403 {
				category, status = "audio_access_denied", 403
			} else if res.StatusCode == 429 {
				category, status = "audio_rate_limited", 429
			}
			res.Body.Close()
			cancelRequest()
			fail(category, status, res.StatusCode)
			return
		}
		cr := res.Header.Get("Content-Range")
		if res.ContentLength != expected || cr != "" && cr != fmt.Sprintf("bytes %d-%d/%d", offset, last, l.length) || res.StatusCode == 206 && cr == "" || res.Header.Get("Content-Encoding") != "" && res.Header.Get("Content-Encoding") != "identity" {
			res.Body.Close()
			cancelRequest()
			fail("audio_range_mismatch", 502)
			return
		}
		if !sent {
			headers()
			sent = true
		}
		received := int64(0)
		category := ""
		for {
			// Reset around each read, so backpressure cannot consume a network timer.
			timer := time.AfterFunc(20*time.Second, cancelRequest)
			n, readErr := res.Body.Read(buffer)
			timedOut := !timer.Stop()
			if timedOut {
				category = "audio_network_timeout"
				break
			}
			received += int64(n)
			if received > expected {
				category = "audio_range_mismatch"
				break
			}
			if n > 0 {
				if ctx.Err() != nil {
					res.Body.Close()
					cancelRequest()
					return
				}
				// A paused decoder or next-track prefetch may stop reading for
				// minutes. Only cancellation ends downstream backpressure; the
				// network timer above applies exclusively to upstream reads.
				_, writeErr := w.Write(buffer[:n])
				if writeErr != nil {
					res.Body.Close()
					cancelRequest()
					return
				}
			}
			if received == expected {
				break
			}
			if readErr != nil {
				if readErr != io.EOF && readErr != io.ErrUnexpectedEOF {
					category = streamError(readErr)
				} else if received != expected {
					category = "audio_stream_truncated"
				}
				break
			}
		}
		res.Body.Close()
		cancelRequest()
		if category != "" {
			// Resume only undelivered bytes of this exact range. Already-written
			// bytes are never replayed, and a failing range gets at most two body
			// recoveries. Range mismatches and access refusals remain terminal.
			retryable := category == "audio_network_timeout" || category == "audio_connection_failed" || category == "audio_stream_truncated"
			if retryable && bodyRetries < 2 && ctx.Err() == nil {
				bodyRetries++
				offset += received
				timer := time.NewTimer(time.Duration(bodyRetries) * 250 * time.Millisecond)
				select {
				case <-ctx.Done():
					timer.Stop()
					return
				case <-timer.C:
					continue
				}
			}
			fail(category, 502)
			return
		}
		offset = last + 1
	}
}
