package player

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// This test deliberately stalls sequential reads before they reach the seek
// target. A decoder that scans and discards earlier audio cannot pass merely
// because the local fixture server is faster than the real media transport.
// The bundled decoder has no AAC encoder/MP4 muxer, so fixture creation uses an
// explicitly supplied full FFmpeg. No generated media is kept in the repository.
func TestFFmpegTailSeekUsesRangesAndMatchesPCM(t *testing.T) {
	runtimeDir, fixtureFFmpeg := os.Getenv("YTMGO_RUNTIME_DIR"), os.Getenv("YTMGO_FIXTURE_FFMPEG")
	if runtimeDir == "" || fixtureFFmpeg == "" {
		t.Skip("set YTMGO_RUNTIME_DIR and YTMGO_FIXTURE_FFMPEG for ranged AAC seek acceptance")
	}
	executable := filepath.Join(runtimeDir, "ffmpeg")
	fixture := filepath.Join(t.TempDir(), "long-seek.m4a")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	generate := childCommand(ctx, fixtureFFmpeg, "-hide_banner", "-loglevel", "error", "-nostdin",
		"-f", "lavfi", "-i", "sine=frequency=523:sample_rate=48000:duration=48",
		"-ac", "2", "-c:a", "aac", "-b:a", "192k", "-movflags", "+faststart", fixture)
	// Even synthetic-fixture failures must not print raw tool diagnostics.
	generate.Stdout, generate.Stderr = io.Discard, io.Discard
	if err := generate.Run(); err != nil {
		t.Fatal("seek_fixture_generation_failed")
	}
	data, err := os.ReadFile(fixture)
	if err != nil || len(data) < 512<<10 {
		t.Fatal("seek_fixture_unavailable")
	}

	for _, at := range []time.Duration{47*time.Second + 125*time.Millisecond, 47*time.Second + 750*time.Millisecond, 48*time.Second - time.Millisecond} {
		t.Run(fmt.Sprintf("at_%dms", at.Milliseconds()), func(t *testing.T) {
			var mu sync.Mutex
			var offsets []int
			var active atomic.Int64
			const sequentialLimit = 256 << 10
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				active.Add(1)
				defer active.Add(-1)
				start, end := 0, len(data)-1
				partial := r.Header.Get("Range") != ""
				if partial {
					parts := strings.Split(strings.TrimPrefix(r.Header.Get("Range"), "bytes="), "-")
					if len(parts) != 2 {
						w.WriteHeader(http.StatusRequestedRangeNotSatisfiable)
						return
					}
					var err error
					start, err = strconv.Atoi(parts[0])
					if err != nil || start < 0 || start > end {
						w.WriteHeader(http.StatusRequestedRangeNotSatisfiable)
						return
					}
					if parts[1] != "" {
						end, err = strconv.Atoi(parts[1])
						if err != nil || end < start || end >= len(data) {
							w.WriteHeader(http.StatusRequestedRangeNotSatisfiable)
							return
						}
					}
				}
				w.Header().Set("Content-Type", "audio/mp4")
				w.Header().Set("Accept-Ranges", "bytes")
				w.Header().Set("Content-Length", strconv.Itoa(end-start+1))
				if partial {
					w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, len(data)))
					w.WriteHeader(http.StatusPartialContent)
				}
				if r.Method == http.MethodHead {
					return
				}
				mu.Lock()
				offsets = append(offsets, start)
				mu.Unlock()
				limit := end + 1
				if start < sequentialLimit {
					limit = min(limit, sequentialLimit)
				}
				if _, err := w.Write(data[start:limit]); err != nil {
					return
				}
				w.(http.Flusher).Flush()
				if limit <= end {
					<-r.Context().Done()
				}
			}))
			defer server.Close()
			ready, readyCancel := context.WithTimeout(ctx, 5*time.Second)
			defer readyCancel()
			d, err := startDecoder(ready, executable, media{URL: server.URL + "/" + strings.Repeat("a", 64), Duration: 48 * time.Second}, at)
			if err != nil {
				t.Fatalf("tail_seek_did_not_reach_pcm: %s", safeError(err))
			}
			defer d.Close()
			stopReadTimeout := context.AfterFunc(ctx, func() { d.Close() })
			defer stopReadTimeout()
			mu.Lock()
			jumped := false
			for _, offset := range offsets {
				jumped = jumped || offset > len(data)/2
			}
			mu.Unlock()
			if !jumped {
				t.Fatal("tail_seek_did_not_request_later_byte_range")
			}

			var actual []byte
			buf := make([]float32, 4096)
			for {
				n, err := d.Read(buf)
				for _, sample := range buf[:n] {
					actual = binary.LittleEndian.AppendUint32(actual, math.Float32bits(sample))
				}
				if err != nil {
					if err != io.EOF {
						t.Fatal("tail_pcm_read_failed")
					}
					break
				}
			}
			if !d.eof.Load() || d.failed.Load() {
				t.Fatal("tail_pcm_did_not_end_cleanly")
			}
			d.Close()
			// Compare with an independently seeked local file, including codec
			// preroll and padding. No private transport URL enters process argv.
			reference := childCommand(ctx, executable, "-hide_banner", "-loglevel", "error", "-nostdin",
				"-ss", fmt.Sprintf("%.6f", at.Seconds()), "-i", fixture,
				"-vn", "-sn", "-dn", "-threads", "1", "-ac", "2", "-ar", "48000", "-f", "f32le", "pipe:1")
			reference.Stderr = io.Discard
			expected, err := reference.Output()
			if err != nil {
				t.Fatal("reference_pcm_decode_failed")
			}
			if !bytes.Equal(actual, expected) {
				t.Fatalf("seek_pcm_mismatch: actual_frames=%d reference_frames=%d", len(actual)/8, len(expected)/8)
			}
			deadline := time.Now().Add(time.Second)
			for active.Load() != 0 && time.Now().Before(deadline) {
				time.Sleep(time.Millisecond)
			}
			if active.Load() != 0 {
				t.Fatal("seek_transport_not_released")
			}
		})
	}
}

func TestEmptySeekRequiresSuccessfulBoundedTail(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("process fixtures use a POSIX shell")
	}
	for _, tc := range []struct {
		name     string
		duration time.Duration
		at       time.Duration
		body     string
		wantOK   bool
	}{
		{"codec_padding_at_tail", 4 * time.Second, 4*time.Second - time.Millisecond, "exit 0", true},
		{"short_initial_empty_stream", 100 * time.Millisecond, 0, "exit 0", false},
		{"empty_middle", 4 * time.Second, time.Second, "exit 0", false},
		{"outside_padding_tolerance", 4 * time.Second, 3500*time.Millisecond - time.Nanosecond, "exit 0", false},
		{"failed_decoder_at_tail", 4 * time.Second, 4*time.Second - time.Millisecond, "exit 7", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			executable := filepath.Join(t.TempDir(), "decoder-fixture")
			if err := os.WriteFile(executable, []byte("#!/bin/sh\n"+tc.body+"\n"), 0700); err != nil {
				t.Fatal("decoder_fixture_unavailable")
			}
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			d, err := startDecoder(ctx, executable, media{URL: "http://127.0.0.1:1/" + strings.Repeat("a", 64), Duration: tc.duration}, tc.at)
			if (err == nil) != tc.wantOK {
				if d != nil {
					d.Close()
				}
				t.Fatalf("empty_decoder_acceptance_mismatch: accepted=%t", err == nil)
			}
			if d != nil {
				defer d.Close()
				if n, err := d.Read(make([]float32, 16)); n != 0 || err != io.EOF || !d.eof.Load() || d.failed.Load() {
					t.Fatal("empty_tail_did_not_end_cleanly")
				}
			}
		})
	}
}

func TestEmptySeekWaitHonorsReadinessCancellation(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("process fixture uses a POSIX shell")
	}
	executable := filepath.Join(t.TempDir(), "decoder-fixture")
	if err := os.WriteFile(executable, []byte("#!/bin/sh\nexec 1>&-\n/bin/sleep 60\n"), 0700); err != nil {
		t.Fatal("decoder_fixture_unavailable")
	}
	for _, mode := range []string{"deadline", "cancel"} {
		t.Run(mode, func(t *testing.T) {
			var ctx context.Context
			var cancel context.CancelFunc
			want := context.DeadlineExceeded
			if mode == "deadline" {
				ctx, cancel = context.WithTimeout(context.Background(), 100*time.Millisecond)
			} else {
				ctx, cancel = context.WithCancel(context.Background())
				time.AfterFunc(100*time.Millisecond, cancel)
				want = context.Canceled
			}
			defer cancel()
			started := time.Now()
			d, err := startDecoder(ctx, executable, media{URL: "http://127.0.0.1:1/" + strings.Repeat("a", 64), Duration: 4 * time.Second}, 4*time.Second-time.Millisecond)
			if d != nil {
				d.Close()
			}
			if !errors.Is(err, want) || time.Since(started) > 2*time.Second {
				t.Fatal("empty_decoder_wait_did_not_cancel_promptly")
			}
		})
	}
}

func TestEmptySeekDecoderProcess(t *testing.T) {
	found := false
	for _, arg := range os.Args {
		if arg == "--empty-seek-fixture" {
			found = true
		}
	}
	if !found {
		return
	}
	input, _ := io.ReadAll(io.LimitReader(os.Stdin, 64<<10))
	for _, line := range strings.Split(string(input), "\n") {
		if strings.HasPrefix(line, "file '") && strings.HasSuffix(line, "'") {
			target := strings.TrimSuffix(strings.TrimPrefix(line, "file '"), "'")
			if !validMediaURL(target) {
				os.Exit(2)
			}
			client := http.Client{Timeout: 2 * time.Second}
			if response, err := client.Get(target); err == nil {
				_, _ = io.Copy(io.Discard, response.Body)
				response.Body.Close()
			}
			os.Exit(0)
		}
	}
	os.Exit(2)
}

func TestEmptySeekDoesNotMaskBridgeFailure(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX decoder wrapper fixture")
	}
	executable := filepath.Join(t.TempDir(), "decoder-fixture")
	binary, err := os.Executable()
	if err != nil {
		t.Fatal("fixture executable unavailable")
	}
	quoted := "'" + strings.ReplaceAll(binary, "'", "'\\''") + "'"
	stub := "#!/bin/sh\nexec " + quoted + " -test.run=^TestEmptySeekDecoderProcess$ -- --empty-seek-fixture\n"

	if err := os.WriteFile(executable, []byte(stub), 0700); err != nil {
		t.Fatal("decoder_fixture_unavailable")
	}
	for _, tc := range []struct{ name, want string }{
		{"access_denied", "audio_access_denied"},
		{"truncated", "audio_stream_truncated"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if tc.name == "access_denied" {
					w.WriteHeader(http.StatusForbidden)
					return
				}
				w.Header().Set("Content-Length", "64")
				if r.Method == http.MethodHead {
					w.Header().Set("X-Ytmgo-Stream-Error", "upstream_truncated")
					return
				}
				_, _ = w.Write(make([]byte, 8))
			}))
			defer server.Close()
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			d, err := startDecoder(ctx, executable, media{URL: server.URL + "/" + strings.Repeat("a", 64), Duration: 4 * time.Second}, 4*time.Second-time.Millisecond)
			if d != nil {
				d.Close()
			}
			if err == nil || safeError(err) != tc.want {
				t.Fatal("empty_tail_masked_transport_failure")
			}
		})
	}
}

func TestEmptySeekPreservesPauseAndReachesNaturalEnd(t *testing.T) {
	runtimeDir := os.Getenv("YTMGO_RUNTIME_DIR")
	if runtimeDir == "" {
		t.Skip("set YTMGO_RUNTIME_DIR for actual AAC end-point playback")
	}
	fixture, err := os.ReadFile("testdata/seek.m4a")
	if err != nil {
		t.Fatal("seek_fixture_unavailable")
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.ServeContent(w, r, "seek.m4a", time.Time{}, bytes.NewReader(fixture))
	}))
	defer server.Close()
	p := testPlayer()
	defer p.Close()
	p.resolve = func(context.Context, string) (media, error) {
		return media{URL: server.URL + "/" + strings.Repeat("a", 64), Duration: 4 * time.Second}, nil
	}
	p.open = runtimePaths{ffmpeg: filepath.Join(runtimeDir, "ffmpeg")}.open
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := p.Load(ctx, "aaaaaaaaaaa"); err != nil {
		t.Fatal("endpoint_initial_load_failed")
	}
	if err := p.Pause(); err != nil {
		t.Fatal("endpoint_pause_failed")
	}
	// Exercise Player's 100% gesture clamp to duration minus one millisecond.
	if err := p.SeekContext(ctx, 4*time.Second); err != nil {
		t.Fatal("endpoint_seek_failed")
	}
	state := p.Snapshot()
	if state.Playing || state.Ended || state.Loading || state.Error != "" || state.Position != 4*time.Second-time.Millisecond {
		t.Fatal("endpoint_seek_did_not_preserve_pause")
	}
	if err := p.PlayContext(ctx); err != nil {
		t.Fatal("endpoint_resume_failed")
	}
	waitPlayerCondition(t, func() bool {
		p.mu.Lock()
		defer p.mu.Unlock()
		return p.session != nil && p.session.decoder.eof.Load() && !p.session.output.Playing()
	})
	state = p.Snapshot()
	if !state.Ended || state.Playing || state.Loading || state.Error != "" {
		t.Fatal("endpoint_did_not_reach_natural_end")
	}
}
