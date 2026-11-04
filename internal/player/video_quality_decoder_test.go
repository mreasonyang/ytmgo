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
	"strings"
	"testing"
	"time"
)

// Decode actual HD pixels with the bundled runtime, including a switch down
// and back up. This catches an old 480p scale surviving the selector change.
func TestFFmpegVideoQualityProducesHDFrames(t *testing.T) {
	runtimeDir, fixtureFFmpeg := os.Getenv("YTMGO_RUNTIME_DIR"), os.Getenv("YTMGO_FIXTURE_FFMPEG")
	if runtimeDir == "" || fixtureFFmpeg == "" {
		t.Skip("set YTMGO_RUNTIME_DIR and YTMGO_FIXTURE_FFMPEG for HD video decoding")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	fixture := filepath.Join(t.TempDir(), "hd.mp4")
	generate := childCommand(ctx, fixtureFFmpeg, "-hide_banner", "-loglevel", "error", "-nostdin",
		"-f", "lavfi", "-i", "testsrc2=size=1920x1080:rate=24:duration=2",
		"-c:v", "libx264", "-preset", "ultrafast", "-pix_fmt", "yuv420p", "-movflags", "+faststart", fixture)
	generate.Stdout, generate.Stderr = io.Discard, io.Discard
	if err := generate.Run(); err != nil {
		t.Fatal("HD fixture generation failed")
	}
	data, err := os.ReadFile(fixture)
	if err != nil {
		t.Fatal("HD fixture unavailable")
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "video/mp4")
		http.ServeContent(w, r, "hd.mp4", time.Time{}, bytes.NewReader(data))
	}))
	defer server.Close()
	for _, quality := range []int{720, 1080, 360, 720} {
		t.Run(fmt.Sprint(quality), func(t *testing.T) {
			width, height := videoDimensions(1920, 1080, quality)
			info := videoMedia{media: media{URL: server.URL + "/" + strings.Repeat("a", 64), Duration: 2 * time.Second}, width: width, height: height}
			decoder, err := startVideoDecoder(ctx, filepath.Join(runtimeDir, "ffmpeg"), info, 0)
			if err != nil {
				t.Fatal("HD decoder unavailable")
			}
			defer decoder.Close()
			first, second := make([]byte, width*height*4), make([]byte, width*height*4)
			if _, err := io.ReadFull(decoder.reader, first); err != nil {
				t.Fatal("first HD frame unavailable")
			}
			if _, err := io.ReadFull(decoder.reader, second); err != nil {
				t.Fatal("second HD frame unavailable")
			}
			if height != quality || bytes.Equal(first, second) {
				t.Fatal("selected resolution was lost or video did not advance")
			}
			t.Logf("decoded moving %dx%d frames", width, height)
		})
	}
}
