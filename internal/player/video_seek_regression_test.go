package player

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// Video needs its own seek regression: a concat inpoint that works for AAC
// can change the first H.264 frame or produce no tail frame when B-frames are
// present. Generate the longer fixture temporarily with an explicit encoder;
// the shipped FFmpeg performs both the application and reference decodes.
func TestFFmpegVideoBFrameSeekMatchesLocalReference(t *testing.T) {
	runtimeDir, fixtureFFmpeg := os.Getenv("YTMGO_RUNTIME_DIR"), os.Getenv("YTMGO_FIXTURE_FFMPEG")
	if runtimeDir == "" || fixtureFFmpeg == "" {
		t.Skip("set YTMGO_RUNTIME_DIR and YTMGO_FIXTURE_FFMPEG for H.264 B-frame seek acceptance")
	}
	executable := filepath.Join(runtimeDir, "ffmpeg")
	const frameBytes, maximumFrames = 320 * 180 * 4, 10
	targets := []time.Duration{
		0, 10 * time.Millisecond, 100 * time.Millisecond,
		4*time.Second + time.Millisecond, 36*time.Second + 123*time.Millisecond,
		73*time.Second + 111*time.Millisecond, 73*time.Second + 125*time.Millisecond,
		117*time.Second + 625*time.Millisecond, 119*time.Second + 900*time.Millisecond,
		119*time.Second + 950*time.Millisecond,
	}
	for _, sourceFPS := range []int{24, 30} {
		t.Run(fmt.Sprintf("source_%dfps", sourceFPS), func(t *testing.T) {
			fixture := filepath.Join(t.TempDir(), "long-bframes.mp4")
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()
			generate := childCommand(ctx, fixtureFFmpeg, "-hide_banner", "-loglevel", "error", "-nostdin",
				"-f", "lavfi", "-i", fmt.Sprintf("testsrc2=size=320x180:rate=%d:duration=120", sourceFPS),
				"-c:v", "libx264", "-preset", "veryfast", "-crf", "20", "-g", "96", "-keyint_min", "96",
				"-sc_threshold", "0", "-bf", "3", "-pix_fmt", "yuv420p", "-movflags", "+faststart", fixture)
			generate.Stdout, generate.Stderr = io.Discard, io.Discard
			if err := generate.Run(); err != nil {
				t.Fatal("video_fixture_generation_failed")
			}
			data, err := os.ReadFile(fixture)
			if err != nil {
				t.Fatal("video_fixture_unavailable")
			}
			for _, at := range targets {
				t.Run(fmt.Sprintf("at_%dms", at.Milliseconds()), func(t *testing.T) {
					// Decode the independent local reference first. Near the tail
					// fewer than ten frames may exist; compare that exact remainder.
					referenceCtx, referenceCancel := context.WithTimeout(ctx, 5*time.Second)
					reference := childCommand(referenceCtx, executable, "-hide_banner", "-loglevel", "quiet", "-nostdin",
						"-threads", "1", "-ss", fmt.Sprintf("%.6f", at.Seconds()), "-i", fixture, "-frames:v", strconv.Itoa(maximumFrames),
						"-an", "-sn", "-dn", "-threads", "1", "-filter_threads", "1", "-vf",
						"scale=320:180:flags=fast_bilinear,fps=24,format=rgba", "-c:v", "rawvideo", "-pix_fmt", "rgba", "-f", "rawvideo", "pipe:1")
					reference.Stderr = io.Discard
					expected, err := reference.Output()
					referenceCancel()
					if err != nil || len(expected)%frameBytes != 0 || len(expected) > maximumFrames*frameBytes {
						t.Fatal("reference_video_frames_failed")
					}

					var laterRange atomic.Bool
					server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						// A fast local server hides sequential scans before the target.
						// Stop an initial response at 256 KiB; later range requests are
						// unaffected, so a correct seek can still read its keyframe.
						start, _ := strconv.ParseInt(strings.Split(strings.TrimPrefix(r.Header.Get("Range"), "bytes="), "-")[0], 10, 64)
						if start >= 256<<10 {
							laterRange.Store(true)
						}
						w.Header().Set("Content-Type", "video/mp4")
						reader := &videoSeekRangeReader{Reader: bytes.NewReader(data), ctx: r.Context(), limited: start < 256<<10}
						http.ServeContent(w, r, "long-bframes.mp4", time.Time{}, reader)
					}))
					defer server.Close()
					deadline, deadlineCancel := context.WithTimeout(ctx, 5*time.Second)
					defer deadlineCancel()
					info := videoMedia{media: media{URL: server.URL + "/" + strings.Repeat("a", 64), Duration: 120 * time.Second}, width: 320, height: 180}
					decoder, err := startVideoDecoder(deadline, executable, info, at)
					if err != nil {
						t.Fatal("video_seek_open_failed")
					}
					// Reap the decoder and cancel its HTTP readers before closing
					// the server, including every failed assertion path.
					defer decoder.Close()
					actual := make([]byte, len(expected))
					if _, err := io.ReadFull(decoder.reader, actual); err != nil {
						t.Fatal("video_seek_produced_incomplete_frame_sequence")
					}
					if len(expected) < maximumFrames*frameBytes {
						// This also validates the zero-reference-frame case instead
						// of allowing empty byte slices to compare equal vacuously.
						var extra [1]byte
						if n, err := decoder.reader.Read(extra[:]); n != 0 || err != io.EOF {
							t.Fatal("video_seek_tail_has_unexpected_extra_output")
						}
					}
					decoder.Close()
					if at >= 30*time.Second && !laterRange.Load() {
						t.Fatal("video_seek_did_not_request_later_byte_range")
					}
					if !bytes.Equal(actual, expected) {
						for i := range actual {
							if actual[i] != expected[i] {
								t.Fatalf("video_seek_frame_sequence_mismatch_at_frame_%d_of_%d", i/frameBytes, len(expected)/frameBytes)
							}
						}
					}
					t.Logf("matched %d frames; later_range=%t", len(expected)/frameBytes, laterRange.Load())
				})
			}
		})
	}
}

type videoSeekRangeReader struct {
	*bytes.Reader
	ctx     context.Context
	limited bool
}

func (r *videoSeekRangeReader) Read(p []byte) (int, error) {
	if r.limited {
		position, _ := r.Reader.Seek(0, io.SeekCurrent)
		remaining := int64(256<<10) - position
		if remaining <= 0 {
			<-r.ctx.Done()
			return 0, r.ctx.Err()
		}
		p = p[:min(int64(len(p)), remaining)]
	}
	return r.Reader.Read(p)
}
