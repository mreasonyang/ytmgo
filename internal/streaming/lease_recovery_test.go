package streaming

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"
	"time"
)

type rangeReadFailure struct{ err error }

func (b rangeReadFailure) Read([]byte) (int, error) { return 0, b.err }

func TestLeaseRecoversExactUndeliveredRange(t *testing.T) {
	for _, tc := range []struct {
		name    string
		failure error
		cut     int
	}{
		{"timeout_before_bytes", context.DeadlineExceeded, 0},
		{"timeout_after_bytes", context.DeadlineExceeded, 1234},
		{"connection_after_bytes", errors.New("synthetic connection interruption"), 1234},
		{"truncated_after_bytes", io.ErrUnexpectedEOF, 1234},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data := make([]byte, 150000)
			for i := range data {
				data[i] = byte(i * 17 % 251)
			}
			var calls atomic.Int32
			var ranges [][2]int64
			r := testResolver(t, func(req *http.Request) (*http.Response, error) {
				call := calls.Add(1)
				var start, end int64
				if _, err := fmt.Sscanf(req.URL.Query().Get("range"), "%d-%d", &start, &end); err != nil || start < 0 || end >= int64(len(data)) || end-start+1 > 64<<10 {
					t.Fatal("invalid resumed range")
				}
				ranges = append(ranges, [2]int64{start, end})
				body := io.Reader(bytes.NewReader(data[start : end+1]))
				if call <= 2 {
					body = io.MultiReader(bytes.NewReader(data[start:start+int64(tc.cut)]), rangeReadFailure{tc.failure})
				}
				return &http.Response{StatusCode: 206, Header: http.Header{"Content-Range": {fmt.Sprintf("bytes %d-%d/%d", start, end, len(data))}}, ContentLength: end - start + 1, Body: io.NopCloser(body)}, nil
			})
			remote, _ := url.Parse("https://r.googlevideo.com/x?expire=" + fmt.Sprint(time.Now().Add(time.Hour).Unix()))
			lease, err := r.publish(remote, format{length: int64(len(data)), contentType: "audio/mp4"})
			if err != nil {
				t.Fatal("fixture lease failed")
			}
			server := httptest.NewServer(lease)
			defer server.Close()
			res, err := http.Get(server.URL)
			if err != nil {
				t.Fatal("local lease request failed")
			}
			got, err := io.ReadAll(res.Body)
			res.Body.Close()
			if err != nil || !bytes.Equal(got, data) || lease.Err() != nil || calls.Load() != 5 {
				t.Fatal("range recovery duplicated, skipped, or lost bytes")
			}
			if ranges[0] != [2]int64{0, 65535} || ranges[1] != [2]int64{int64(tc.cut), 65535} || ranges[2] != [2]int64{int64(2 * tc.cut), 65535} {
				t.Fatal("recovery replayed delivered bytes or crossed its original range")
			}
		})
	}
}

func TestLeaseCancellationDuringRecovery(t *testing.T) {
	var calls atomic.Int32
	read := make(chan struct{}, 1)
	r := testResolver(t, func(req *http.Request) (*http.Response, error) {
		calls.Add(1)
		read <- struct{}{}
		return &http.Response{StatusCode: 200, Header: make(http.Header), ContentLength: 10, Body: io.NopCloser(rangeReadFailure{context.DeadlineExceeded})}, nil
	})
	remote, _ := url.Parse("https://r.googlevideo.com/x?expire=" + fmt.Sprint(time.Now().Add(time.Hour).Unix()))
	lease, _ := r.publish(remote, format{length: 10, contentType: "audio/mp4"})
	server := httptest.NewServer(lease)
	defer server.Close()
	ctx, cancel := context.WithCancel(context.Background())
	req, _ := http.NewRequestWithContext(ctx, "GET", server.URL, nil)
	done := make(chan struct{})
	go func() {
		defer close(done)
		res, err := http.DefaultClient.Do(req)
		if err == nil {
			_, _ = io.Copy(io.Discard, res.Body)
			res.Body.Close()
		}
	}()
	<-read
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("recovery ignored cancellation")
	}
	lease.Close()
	if calls.Load() != 1 || lease.Err() != nil {
		t.Fatal("canceled recovery retried or became a media failure")
	}
}

func TestLeaseHTTPFailureRetainsActualStatus(t *testing.T) {
	for _, status := range []int{403, 416, 429, 503} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			var calls atomic.Int32
			r := testResolver(t, func(*http.Request) (*http.Response, error) {
				calls.Add(1)
				res := response("")
				res.StatusCode = status
				return res, nil
			})
			remote, _ := url.Parse("https://r.googlevideo.com/x?expire=" + fmt.Sprint(time.Now().Add(time.Hour).Unix()))
			lease, _ := r.publish(remote, format{length: 10, contentType: "audio/mp4"})
			server := httptest.NewServer(lease)
			defer server.Close()
			res, err := http.Get(server.URL)
			if err != nil {
				t.Fatal("local lease request failed")
			}
			res.Body.Close()
			var failure *Failure
			wantCalls := int32(1)
			if status == 429 || status >= 500 {
				wantCalls = 3
			}
			if !errors.As(lease.Err(), &failure) || failure.Stage != "media_stream" || failure.HTTPStatus != status || calls.Load() != wantCalls {
				t.Fatal("actual media status was lost or its retry limit changed")
			}
		})
	}
}

func TestMediaCDNRedirectKeepsExactRangeAndNoAccountHeaders(t *testing.T) {
	for _, status := range []int{301, 302, 303, 307, 308} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			var calls atomic.Int32
			r := testResolver(t, func(req *http.Request) (*http.Response, error) {
				calls.Add(1)
				if req.Header.Get("Authorization") != "" || req.Header.Get("Cookie") != "" || req.Header.Get("Proxy-Authorization") != "" || req.Header.Get("Range") != "" {
					t.Fatal("media redirect forwarded authentication or changed the range protocol")
				}
				if req.URL.Host == "r.googlevideo.com" {
					res := response("")
					res.StatusCode = status
					res.Header.Set("Location", "https://edge.googlevideo.com/videoplayback?range=999-999&synthetic=relocated")
					return res, nil
				}
				if req.URL.Host != "edge.googlevideo.com" || req.URL.Query().Get("range") != "2-7" || req.URL.Query().Get("synthetic") != "relocated" {
					t.Fatal("redirect escaped the media boundary or changed its byte range")
				}
				res := response("234567")
				res.StatusCode = 206
				res.Header.Set("Content-Range", "bytes 2-7/10")
				return res, nil
			})
			remote, _ := url.Parse("https://r.googlevideo.com/x?expire=" + fmt.Sprint(time.Now().Add(time.Hour).Unix()))
			lease, _ := r.publish(remote, format{length: 10, contentType: "audio/mp4"})
			server := httptest.NewServer(lease)
			defer server.Close()
			req, _ := http.NewRequest("GET", server.URL, nil)
			req.Header.Set("Range", "bytes=2-7")
			res, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatal("local request failed")
			}
			data, err := io.ReadAll(res.Body)
			res.Body.Close()
			if err != nil || string(data) != "234567" || calls.Load() != 2 || lease.Err() != nil {
				t.Fatal("CDN relocation interrupted the media range")
			}
		})
	}
}

func TestMediaRedirectBoundaryAndLoopLimit(t *testing.T) {
	for _, location := range []string{"http://edge.googlevideo.com/x", "https://example.invalid/x", "https://googlevideo.com/x", "https://edge.googlevideo.com.evil.invalid/x", "https://user:synthetic@edge.googlevideo.com/x", "https://edge.googlevideo.com:443/x", "https://edge.googlevideo.com/x#fragment", "https://edge.googlevideo.com/x"} {
		t.Run(fmt.Sprint(len(location))+location[0:4], func(t *testing.T) {
			var calls atomic.Int32
			loop := location == "https://edge.googlevideo.com/x"
			r := testResolver(t, func(req *http.Request) (*http.Response, error) {
				call := calls.Add(1)
				if !loop && call > 1 {
					t.Fatal("unsafe redirect target was contacted")
				}
				res := response("")
				res.StatusCode = 302
				res.Header.Set("Location", location)
				return res, nil
			})
			remote, _ := url.Parse("https://r.googlevideo.com/x?expire=" + fmt.Sprint(time.Now().Add(time.Hour).Unix()))
			lease, _ := r.publish(remote, format{length: 10, contentType: "audio/mp4"})
			server := httptest.NewServer(lease)
			defer server.Close()
			res, err := http.Get(server.URL)
			if err != nil {
				t.Fatal("local request failed")
			}
			res.Body.Close()
			want := int32(1)
			if loop {
				want = 4
			}
			var failure *Failure
			if calls.Load() != want || !errors.As(lease.Err(), &failure) || failure.HTTPStatus != 302 {
				t.Fatal("redirect rejection or hop bound failed")
			}
		})
	}
}
