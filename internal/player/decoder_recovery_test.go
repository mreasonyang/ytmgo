package player

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestMidstreamFailureCategoryReachesSnapshot(t *testing.T) {
	for _, tc := range []struct{ name, bridge, stderr, want string }{
		{"timeout", "audio_network_timeout", "", "audio_network_timeout"},
		{"access", "audio_access_denied", "", "audio_access_denied"},
		{"rate", "audio_rate_limited", "", "audio_rate_limited"},
		{"truncated", "audio_stream_truncated", "", "audio_stream_truncated"},
		{"range", "audio_range_mismatch", "", "audio_range_mismatch"},
		{"decoder_timeout", "", "operation timed out", "audio_network_timeout"},
		{"duration", "", "", "audio_stream_truncated"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := testPlayer()
			defer p.Close()
			p.open = func(context.Context, media, time.Duration) (*pcmDecoder, error) {
				input := io.NopCloser(bytes.NewReader(nil))
				wait := make(chan error, 1)
				if tc.stderr != "" {
					wait <- errors.New("synthetic process failure")
				}
				close(wait)
				d := &pcmDecoder{reader: input, input: input, cancel: func() {}, wait: wait, checkLength: true, length: sampleRate * 2}
				if tc.bridge != "" {
					bridge, err := newStreamBridge(context.Background(), media{URL: "http://127.0.0.1:1/" + strings.Repeat("a", 64)})
					if err != nil {
						return nil, err
					}
					bridge.setError(tc.bridge)
					d.bridge = bridge
				}
				if tc.stderr != "" {
					d.diagnostic = &decoderDiagnostics{}
					_, _ = d.diagnostic.Write([]byte(tc.stderr))
				}
				return d, nil
			}
			if p.Load(context.Background(), "aaaaaaaaaaa") != nil {
				t.Fatal("fixture load failed")
			}
			waitPlayerCondition(t, func() bool { return p.Snapshot().Error != "" })
			state := p.Snapshot()
			if state.Ended || state.Error != tc.want {
				t.Fatal("midstream failure lost its fixed category or advanced the queue")
			}
		})
	}
}

type delayedMedia struct {
	data    []byte
	delay   time.Duration
	waiting atomic.Bool
}

type flushMediaHeader struct{ http.ResponseWriter }

func (w flushMediaHeader) WriteHeader(code int) {
	w.ResponseWriter.WriteHeader(code)
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

type delayedMediaReader struct {
	*bytes.Reader
	ctx    context.Context
	source *delayedMedia
}

func (r *delayedMediaReader) Read(p []byte) (int, error) {
	if r.source.waiting.CompareAndSwap(false, true) {
		timer := time.NewTimer(r.source.delay)
		defer timer.Stop()
		select {
		case <-timer.C:
		case <-r.ctx.Done():
			return 0, r.ctx.Err()
		}
	}
	return r.Reader.Read(p)
}

func (s *delayedMedia) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "audio/mp4")
	http.ServeContent(flushMediaHeader{w}, r, "fixture.m4a", time.Time{}, &delayedMediaReader{Reader: bytes.NewReader(s.data), ctx: r.Context(), source: s})
}

func TestActualDecoderSurvivesTransientStall(t *testing.T) {
	dir := os.Getenv("YTMGO_RUNTIME_DIR")
	if dir == "" || testing.Short() {
		t.Skip("requires explicit FFmpeg runtime and a real 12-second stall")
	}
	data, err := os.ReadFile("testdata/seek.m4a")
	if err != nil {
		t.Fatal("fixture missing")
	}
	source := &delayedMedia{data: data, delay: 12 * time.Second}
	server := httptest.NewServer(source)
	defer server.Close()
	info := media{URL: server.URL + "/" + strings.Repeat("a", 64), Duration: 4 * time.Second}
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	d, err := startDecoder(ctx, filepath.Join(dir, "ffmpeg"), info, 0)
	if err != nil {
		t.Fatal("decoder did not survive a transient body-read stall")
	}
	defer d.Close()
	stop := context.AfterFunc(ctx, func() { d.Close() })
	defer stop()
	for {
		if _, err := d.Read(make([]float32, 4096)); err != nil {
			if err != io.EOF || d.failed.Load() || ctx.Err() != nil {
				t.Fatal("recovered stream did not decode completely")
			}
			break
		}
	}
}

func TestActualDecoderStallRemainsCancelable(t *testing.T) {
	dir := os.Getenv("YTMGO_RUNTIME_DIR")
	if dir == "" {
		t.Skip("requires explicit FFmpeg runtime")
	}
	data, err := os.ReadFile("testdata/seek.m4a")
	if err != nil {
		t.Fatal("fixture missing")
	}
	server := httptest.NewServer(&delayedMedia{data: data, delay: time.Minute})
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	started := time.Now()
	d, err := startDecoder(ctx, filepath.Join(dir, "ffmpeg"), media{URL: server.URL + "/" + strings.Repeat("a", 64), Duration: 4 * time.Second}, 0)
	if d != nil {
		d.Close()
	}
	if !errors.Is(err, context.DeadlineExceeded) || time.Since(started) > 3*time.Second {
		t.Fatal("larger network watchdog delayed cancellation")
	}
}
