package player

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func testVideo() *Video {
	v := &Video{}
	v.resolve = func(context.Context, string) (videoMedia, error) {
		return videoMedia{media: media{Duration: time.Minute}, width: 4, height: 2}, nil
	}
	v.open = func(context.Context, videoMedia, time.Duration) (*videoDecoder, error) {
		decoder, _ := zeroVideoDecoder()
		return decoder, nil
	}
	return v
}

func zeroVideoDecoder() (*videoDecoder, *zeroStream) {
	stream := &zeroStream{}
	done := make(chan error)
	close(done)
	return &videoDecoder{reader: stream, cancel: func() {}, wait: done}, stream
}

func waitVideo(t *testing.T, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("video worker did not reach expected state")
}

func TestVideoDimensionsBoundMemory(t *testing.T) {
	for _, dimensions := range [][2]int{{3840, 2160}, {1920, 1080}, {640, 480}, {1080, 1920}, {64, 36}} {
		width, height := videoDimensions(dimensions[0], dimensions[1])
		if width < 2 || height < 2 || width > 854 || height > 480 || width%2 != 0 || height%2 != 0 {
			t.Fatal("output dimensions exceed video memory bound")
		}
	}
}

func TestVideoResolverHeaders(t *testing.T) {
	for _, test := range []struct {
		name, agent, referer string
		valid                bool
	}{
		{"resolver headers", "ytmgo-test-agent", "https://www.youtube.com/", true},
		{"optional referer", "ytmgo-test-agent", "", true},
		{"local capability needs no remote headers", "", "", true},
		{"newline agent", "test\r\nExtra: value", "", false},
		{"quoted referer", "ytmgo-test-agent", "https://example.com/'", false},
		{"escaped referer", "ytmgo-test-agent", "https://example.com/\\", false},
		{"oversized agent", strings.Repeat("a", 1025), "", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			data, err := json.Marshal(map[string]any{
				"url": "http://127.0.0.1:1234/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "duration": 120,
				"width": 640, "height": 360,
				"http_headers": map[string]string{"User-Agent": test.agent, "Referer": test.referer},
			})
			if err != nil {
				t.Fatal("could not create synthetic response")
			}
			info, err := parseVideoMedia(data)
			if !test.valid {
				if err == nil || err.Error() != "invalid_media_response" || info.URL != "" {
					t.Fatal("invalid resolver headers were not safely rejected")
				}
				return
			}
			if err != nil || info.UserAgent != test.agent || info.Referer != test.referer || info.Duration != 2*time.Minute || info.width != 640 || info.height != 360 {
				t.Fatal("resolver metadata was not preserved")
			}
		})
	}
}

func TestVideoDecoderRejectsHeaderInjection(t *testing.T) {
	info := videoMedia{media: media{URL: "http://127.0.0.1:1234/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", UserAgent: "invalid\nheader"}, width: 4, height: 2}
	decoder, err := startVideoDecoder(context.Background(), "unused", info, 0)
	if decoder != nil || err == nil || err.Error() != "invalid_media_response" {
		t.Fatal("unsafe header reached decoder setup")
	}
}

func TestVideoBridgeErrorCategories(t *testing.T) {
	for _, test := range []struct{ input, want string }{
		{"audio_access_denied", "video_access_denied"},
		{"audio_network_timeout", "video_network_timeout"},
		{"audio_connection_failed", "video_connection_failed"},
		{"synthetic private upstream text", "video_stream_failed"},
	} {
		if videoErrorCategory(errors.New(test.input)) != test.want {
			t.Fatal("video error was not safely classified")
		}
	}
}

func TestVideoPauseSeekAndRelease(t *testing.T) {
	v := testVideo()
	defer v.Close()
	var position atomic.Int64
	var mu sync.Mutex
	var seeks []time.Duration
	var streams []*zeroStream
	v.open = func(ctx context.Context, info videoMedia, at time.Duration) (*videoDecoder, error) {
		decoder, stream := zeroVideoDecoder()
		mu.Lock()
		seeks, streams = append(seeks, at), append(streams, stream)
		mu.Unlock()
		return decoder, nil
	}
	if err := v.Start(context.Background(), "aaaaaaaaaaa", func() time.Duration { return time.Duration(position.Load()) }, func() bool { return false }); err != nil {
		t.Fatal(err)
	}
	waitVideo(t, func() bool { frame, _ := v.Frame(); return frame != nil })
	_, first := v.Frame()
	time.Sleep(80 * time.Millisecond)
	if _, sequence := v.Frame(); sequence != first {
		t.Fatal("paused video advanced")
	}
	mu.Lock()
	readAhead := streams[0].n.Load()
	mu.Unlock()
	if readAhead > 4*4*2*4 {
		t.Fatal("paused decoder read more than four small frames")
	}
	position.Store(int64(2 * time.Second))
	waitVideo(t, func() bool { _, sequence := v.Frame(); return sequence > first })
	mu.Lock()
	if len(seeks) < 2 || seeks[1] != 2*time.Second || !streams[0].closed.Load() {
		t.Error("seek did not replace and release the decoder")
	}
	mu.Unlock()
	v.Stop()
	if frame, _ := v.Frame(); frame != nil || v.Loading() {
		t.Fatal("stopped video retained its frame/loading state")
	}
	mu.Lock()
	for _, stream := range streams {
		if !stream.closed.Load() {
			t.Error("stop retained a decoder")
		}
	}
	mu.Unlock()
}

func TestVideoCloseCancelsResolutionAndRejectsRestart(t *testing.T) {
	v := testVideo()
	started := make(chan struct{})
	v.resolve = func(ctx context.Context, id string) (videoMedia, error) {
		close(started)
		<-ctx.Done()
		return videoMedia{}, ctx.Err()
	}
	if err := v.Start(context.Background(), "aaaaaaaaaaa", func() time.Duration { return 0 }, func() bool { return true }); err != nil {
		t.Fatal(err)
	}
	<-started
	v.Close()
	if v.Loading() || v.Error() != "" {
		t.Fatal("closed video still reports loading/failure")
	}
	if err := v.Start(context.Background(), "aaaaaaaaaaa", func() time.Duration { return 0 }, func() bool { return true }); err != ErrClosed {
		t.Fatal("closed video accepted restart")
	}
}

func TestVideoDependencyErrorRemainsPrivate(t *testing.T) {
	v := testVideo()
	defer v.Close()
	v.resolve = func(context.Context, string) (videoMedia, error) {
		return videoMedia{}, errors.New("synthetic-private-upstream-error")
	}
	if err := v.Start(context.Background(), "aaaaaaaaaaa", func() time.Duration { return 0 }, func() bool { return true }); err != nil {
		t.Fatal(err)
	}
	waitVideo(t, func() bool { return !v.Loading() })
	if v.Error() != "video_resolution_failed" {
		t.Fatal("raw dependency error escaped")
	}
}

func TestVideoOpenErrorRetainsSafeCategory(t *testing.T) {
	for _, test := range []struct{ name, upstream, want string }{
		{"access denial", "audio_access_denied", "video_access_denied"},
		{"unknown upstream error", "synthetic-private-upstream-error", "video_stream_failed"},
	} {
		t.Run(test.name, func(t *testing.T) {
			v := testVideo()
			defer v.Close()
			v.open = func(context.Context, videoMedia, time.Duration) (*videoDecoder, error) {
				return nil, errors.New(test.upstream)
			}
			if err := v.Start(context.Background(), "aaaaaaaaaaa", func() time.Duration { return 0 }, func() bool { return true }); err != nil {
				t.Fatal("could not start video error fixture")
			}
			waitVideo(t, func() bool { return !v.Loading() })
			if v.Error() != test.want {
				t.Fatal("video open error did not retain its safe category")
			}
		})
	}
}

func TestVideoCanceledStartPreservesCurrentFrame(t *testing.T) {
	v := testVideo()
	defer v.Close()
	if err := v.Start(context.Background(), "aaaaaaaaaaa", func() time.Duration { return 0 }, func() bool { return false }); err != nil {
		t.Fatal(err)
	}
	waitVideo(t, func() bool { frame, _ := v.Frame(); return frame != nil })
	_, before := v.Frame()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := v.Start(ctx, "bbbbbbbbbbb", func() time.Duration { return 0 }, func() bool { return false }); !errors.Is(err, context.Canceled) {
		t.Fatal("canceled start accepted")
	}
	if _, after := v.Frame(); after != before {
		t.Fatal("canceled start replaced frame")
	}
}

// A two-second H.264 fixture: one second red followed by one second blue.
// The tiny generated bitstream is embedded so the test needs only the shipped
// decoder, with no fixture encoder, network or third-party runtime dependency.
const videoSeekFixture = "AAAAIGZ0eXBpc29tAAACAGlzb21pc28yYXZjMW1wNDEAAAPlbW9vdgAAAGxtdmhkAAAAAAAAAAAAAAAAAAAD6AAAB9AAAQAAAQAAAAAAAAAAAAAAAAEAAAAAAAAAAAAAAAAAAAABAAAAAAAAAAAAAAAAAABAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAgAAAxB0cmFrAAAAXHRraGQAAAADAAAAAAAAAAAAAAABAAAAAAAAB9AAAAAAAAAAAAAAAAAAAAAAAAEAAAAAAAAAAAAAAAAAAAABAAAAAAAAAAAAAAAAAABAAAAAAEAAAAAkAAAAAAAkZWR0cwAAABxlbHN0AAAAAAAAAAEAAAfQAAAAAAABAAAAAAKIbWRpYQAAACBtZGhkAAAAAAAAAAAAAAAAAAAwAAAAYABVxAAAAAAALWhkbHIAAAAAAAAAAHZpZGUAAAAAAAAAAAAAAABWaWRlb0hhbmRsZXIAAAACM21pbmYAAAAUdm1oZAAAAAEAAAAAAAAAAAAAACRkaW5mAAAAHGRyZWYAAAAAAAAAAQAAAAx1cmwgAAAAAQAAAfNzdGJsAAAAt3N0c2QAAAAAAAAAAQAAAKdhdmMxAAAAAAAAAAEAAAAAAAAAAAAAAAAAAAAAAEAAJABIAAAASAAAAAAAAAABFExhdmM2My4xLjEwMiBsaWJ4MjY0AAAAAAAAAAAAAAAAGP//AAAALWF2Y0MBQsAK/+EAFmdCwAraEf58BEAAAAMAQAAADAPEiagBAARozg/IAAAAEHBhc3AAAAABAAAAAQAAABRidHJ0AAAAAAAAEYwAAAAAAAAAGHN0dHMAAAAAAAAAAQAAADAAAAIAAAAAGHN0c3MAAAAAAAAAAgAAAAEAAAAZAAAAHHN0c2MAAAAAAAAAAQAAAAEAAAAwAAAAAQAAANRzdHN6AAAAAAAAAAAAAAAwAAACdgAAAAoAAAAKAAAACgAAAAoAAAAKAAAACgAAAAoAAAAKAAAACgAAAAoAAAAKAAAACgAAAAoAAAAKAAAACgAAAAoAAAAKAAAACgAAAAoAAAAKAAAACgAAAAoAAAAKAAAAIQAAAAoAAAAKAAAACgAAAAoAAAAKAAAACgAAAAoAAAAKAAAACgAAAAoAAAAKAAAACgAAAAoAAAAKAAAACgAAAAoAAAAKAAAACgAAAAoAAAAKAAAACgAAAAoAAAAKAAAAFHN0Y28AAAAAAAAAAQAABBUAAABhdWR0YQAAAFltZXRhAAAAAAAAACFoZGxyAAAAAAAAAABtZGlyYXBwbAAAAAAAAAAAAAAAACxpbHN0AAAAJKl0b28AAAAcZGF0YQAAAAEAAAAATGF2ZjYzLjEuMTAyAAAACGZyZWUAAARrbWRhdAAAAlIGBf//TtxF6b3m2Ui3lizYINkj7u94MjY0IC0gY29yZSAxNjUgcjMyMjIgYjM1NjA1YSAtIEguMjY0L01QRUctNCBBVkMgY29kZWMgLSBDb3B5bGVmdCAyMDAzLTIwMjUgLSBodHRwOi8vd3d3LnZpZGVvbGFuLm9yZy94MjY0Lmh0bWwgLSBvcHRpb25zOiBjYWJhYz0wIHJlZj0xIGRlYmxvY2s9MDowOjAgYW5hbHlzZT0wOjAgbWU9ZGlhIHN1Ym1lPTAgcHN5PTEgcHN5X3JkPTEuMDA6MC4wMCBtaXhlZF9yZWY9MCBtZV9yYW5nZT0xNiBjaHJvbWFfbWU9MSB0cmVsbGlzPTAgOHg4ZGN0PTAgY3FtPTAgZGVhZHpvbmU9MjEsMTEgZmFzdF9wc2tpcD0xIGNocm9tYV9xcF9vZmZzZXQ9MCB0aHJlYWRzPTEgbG9va2FoZWFkX3RocmVhZHM9MSBzbGljZWRfdGhyZWFkcz0wIG5yPTAgZGVjaW1hdGU9MSBpbnRlcmxhY2VkPTAgYmx1cmF5X2NvbXBhdD0wIGNvbnN0cmFpbmVkX2ludHJhPTAgYmZyYW1lcz0wIHdlaWdodHA9MCBrZXlpbnQ9MjQga2V5aW50X21pbj0yIHNjZW5lY3V0PTAgaW50cmFfcmVmcmVzaD0wIHJjPWNyZiBtYnRyZWU9MCBjcmY9MjMuMCBxY29tcD0wLjYwIHFwbWluPTAgcXBtYXg9NjkgcXBzdGVwPTQgaXBfcmF0aW89MS40MCBhcT0wAIAAAAAcZYiEOhGKAAIY8cAAQPY4AAh5ScnJ111111114AAAAAZBmiAQobAAAAAGQZpAEKGwAAAABkGaYBChsAAAAAZBmoARobAAAAAGQZqgEaGwAAAABkGawBGhsAAAAAZBmuARobAAAAAGQZsAEaGwAAAABkGbIBGhsAAAAAZBm0ARobAAAAAGQZtgEaGwAAAABkGbgBGhsAAAAAZBm6ARobAAAAAGQZvAEaGwAAAABkGb4BGhsAAAAAZBmgARobAAAAAGQZogEaGwAAAABkGaQBGhsAAAAAZBmmARobAAAAAGQZqAEaGwAAAABkGaoBGhsAAAAAZBmsARobAAAAAGQZrgEaGwAAAAHWWIggE6EYoAA1fxwABbWjgACHzJycnXXXXXXXXgAAAABkGaIBChsAAAAAZBmkAQobAAAAAGQZpgEKGwAAAABkGagBGhsAAAAAZBmqARobAAAAAGQZrAEaGwAAAABkGa4BGhsAAAAAZBmwARobAAAAAGQZsgEaGwAAAABkGbQBGhsAAAAAZBm2ARobAAAAAGQZuAEaGwAAAABkGboBGhsAAAAAZBm8ARobAAAAAGQZvgEaGwAAAABkGaABGhsAAAAAZBmiARobAAAAAGQZpAEaGwAAAABkGaYBGhsAAAAAZBmoARobAAAAAGQZqgEaGwAAAABkGawBGhsAAAAAZBmuARobA="

func TestFFmpegVideoActualSeek(t *testing.T) {
	if os.Getenv("YTMGO_RUNTIME_DIR") == "" {
		t.Skip("set YTMGO_RUNTIME_DIR to verify bundled native video decoding")
	}
	data, err := base64.StdEncoding.DecodeString(videoSeekFixture)
	if err != nil {
		t.Fatal("invalid video fixture")
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "video/mp4")
		http.ServeContent(w, r, "seek.mp4", time.Time{}, bytes.NewReader(data))
	}))
	defer server.Close()
	for _, test := range []struct {
		at   time.Duration
		blue bool
	}{{0, false}, {1500 * time.Millisecond, true}} {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		info := videoMedia{media: media{URL: server.URL, Duration: 2 * time.Second}, width: 64, height: 36}
		decoder, err := startVideoDecoder(ctx, filepath.Join(os.Getenv("YTMGO_RUNTIME_DIR"), "ffmpeg"), info, test.at)
		if err != nil {
			cancel()
			t.Fatal(err)
		}
		frame := make([]byte, 64*36*4)
		_, err = io.ReadFull(decoder.reader, frame)
		decoder.Close()
		cancel()
		if err != nil {
			t.Fatal("bundled FFmpeg did not decode an RGBA frame")
		}
		if test.blue {
			if frame[2] < 200 || frame[0] > 30 {
				t.Fatal("seek did not reach the blue segment")
			}
		} else {
			if frame[0] < 200 || frame[2] > 30 {
				t.Fatal("initial frame was not the red segment")
			}
		}
	}
}

func TestFFmpegVideoHeadersAndRetryBoundaries(t *testing.T) {
	if os.Getenv("YTMGO_RUNTIME_DIR") == "" {
		t.Skip("set YTMGO_RUNTIME_DIR to verify bundled video HTTP requests")
	}
	data, err := base64.StdEncoding.DecodeString(videoSeekFixture)
	if err != nil {
		t.Fatal("invalid video fixture")
	}
	for _, test := range []struct {
		name   string
		status int
	}{
		{"headers", http.StatusOK},
		{"retry server failure", http.StatusServiceUnavailable},
		{"access denied is final", http.StatusForbidden},
	} {
		t.Run(test.name, func(t *testing.T) {
			var attempts atomic.Int32
			var invalidHeaders atomic.Bool
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				attempt := attempts.Add(1)
				if r.Header.Get("User-Agent") != "ytmgo-video-test-agent" || r.Header.Get("Referer") != "https://www.youtube.com/" {
					invalidHeaders.Store(true)
					w.WriteHeader(http.StatusForbidden)
					return
				}
				if test.status == http.StatusForbidden || test.status == http.StatusServiceUnavailable && attempt == 1 {
					w.WriteHeader(test.status)
					return
				}
				w.Header().Set("Content-Type", "video/mp4")
				http.ServeContent(w, r, "fixture.mp4", time.Time{}, bytes.NewReader(data))
			}))
			defer server.Close()
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			info := videoMedia{media: media{URL: server.URL, Duration: 2 * time.Second, UserAgent: "ytmgo-video-test-agent", Referer: "https://www.youtube.com/"}, width: 64, height: 36}
			decoder, err := startVideoDecoder(ctx, filepath.Join(os.Getenv("YTMGO_RUNTIME_DIR"), "ffmpeg"), info, 0)
			if err != nil {
				t.Fatal("could not start bundled video decoder")
			}
			frame := make([]byte, 64*36*4)
			_, readErr := io.ReadFull(decoder.reader, frame)
			decoder.Close()
			if invalidHeaders.Load() {
				t.Fatal("resolver headers were not sent to the media server")
			}
			if test.status == http.StatusForbidden {
				if readErr == nil || attempts.Load() != 1 {
					t.Fatal("access denial was retried or decoded")
				}
				if videoErrorCategory(decoder.streamError()) != "video_access_denied" {
					t.Fatal("access denial did not retain its safe bridge category")
				}
				return
			}
			if readErr != nil || frame[0] < 200 || frame[2] > 30 {
				t.Fatal("native HTTP video decode did not deliver the expected frame")
			}
			if test.status == http.StatusServiceUnavailable && attempts.Load() < 2 {
				t.Fatal("transient server failure was not retried")
			}
		})
	}
}

func TestFFmpegVideoCloseCancelsBridgeRequest(t *testing.T) {
	if os.Getenv("YTMGO_RUNTIME_DIR") == "" {
		t.Skip("set YTMGO_RUNTIME_DIR to verify video bridge cancellation")
	}
	started, canceled := make(chan struct{}), make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-r.Context().Done()
		close(canceled)
	}))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	info := videoMedia{media: media{URL: server.URL, Duration: 2 * time.Second}, width: 64, height: 36}
	decoder, err := startVideoDecoder(ctx, filepath.Join(os.Getenv("YTMGO_RUNTIME_DIR"), "ffmpeg"), info, 0)
	if err != nil {
		t.Fatal("could not start bundled video decoder")
	}
	defer decoder.Close()
	select {
	case <-started:
	case <-ctx.Done():
		t.Fatal("video decoder did not request its upstream stream")
	}
	closed := make(chan struct{})
	go func() { decoder.Close(); close(closed) }()
	select {
	case <-closed:
	case <-time.After(2 * time.Second):
		t.Fatal("video close did not release decoder and bridge")
	}
	select {
	case <-canceled:
	case <-time.After(2 * time.Second):
		t.Fatal("video close retained an upstream request")
	}
	client := &http.Client{Timeout: time.Second}
	response, err := client.Get(decoder.bridge.URL)
	if err == nil {
		response.Body.Close()
		t.Fatal("video close retained the loopback bridge service")
	}
}

// Anonymous network test; signed URLs and image contents are never printed.
func TestNativeVideoLive(t *testing.T) {
	if os.Getenv("YTMGO_LIVE_VIDEO") != "1" {
		t.Skip("set YTMGO_LIVE_VIDEO=1 for native video network verification")
	}
	p, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	initializeAnonymousLive(t, p)
	v, err := NewVideo(p)
	if err != nil {
		t.Fatal(err)
	}
	defer v.Close()
	var clock atomic.Int64
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	start := time.Now()
	if err := v.Start(ctx, "52fNscjkST4", func() time.Duration { return time.Duration(clock.Load()) }, func() bool { return false }); err != nil {
		t.Fatal(err)
	}
	for v.Loading() && ctx.Err() == nil {
		time.Sleep(50 * time.Millisecond)
	}
	if v.Error() != "" {
		t.Fatal(v.Error())
	}
	frame, first := v.Frame()
	if frame == nil {
		t.Fatal("native video produced no frame")
	}
	t.Logf("first native frame: %dx%d in %.1fs", frame.Bounds().Dx(), frame.Bounds().Dy(), time.Since(start).Seconds())
	clock.Store(int64(30 * time.Second))
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		_, sequence := v.Frame()
		if sequence > first {
			break
		}
		if v.Error() != "" {
			t.Fatal(v.Error())
		}
		time.Sleep(50 * time.Millisecond)
	}
	if _, sequence := v.Frame(); sequence <= first {
		t.Fatal("native video seek did not deliver a new frame")
	}
	v.Stop()
	if frame, _ := v.Frame(); frame != nil {
		t.Fatal("native video retained frame after hide")
	}
	t.Log("native video seek and release: passed")
}
