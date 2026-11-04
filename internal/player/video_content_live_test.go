package player

import (
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"ytmgo/internal/catalog"
)

// This public opt-in checks actual visual changes in an official MV, at two positions, alongside
// the separate moving-clock/audio-sync probes. No image, media URL, track ID,
// account input or upstream message is retained in the numeric evidence.
// Optional frame exports contain only the explicitly selected public MV.
func TestNativeOfficialVideoContentLive(t *testing.T) {
	if os.Getenv("YTMGO_LIVE_MV_CONTENT") != "1" {
		t.Skip("explicit public MV content validation not selected")
	}
	var evidence struct {
		Category                                        string `json:"category"`
		OfficialVideoMatched                            bool   `json:"official_video_matched"`
		StartupMS                                       int64  `json:"startup_ms"`
		Samples, MovingSamples                          int
		FramesAdvanced                                  uint64  `json:"frames_advanced"`
		MaximumPixelDifference                          float64 `json:"maximum_pixel_difference"`
		ResolutionCategory, ResolutionStage, VideoError string
		Width, Height                                   int
	}
	evidence.Category = "setup_failed"
	defer func() {
		if path := os.Getenv("YTMGO_LIVE_EVIDENCE"); path != "" {
			data, _ := json.MarshalIndent(evidence, "", "  ")
			if os.WriteFile(path, append(data, '\n'), 0600) != nil {
				t.Error("MV evidence write failed")
			}
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute+5*time.Second)
	defer cancel()
	client := catalog.New()
	defer client.ClearSession()
	page, err := client.Search(ctx, "P!nk Try Official Video")
	if err != nil {
		evidence.Category = "search_failed"
		t.Fatal("public MV lookup failed")
	}
	var id string
	for _, item := range page.Items {
		if item.Kind == "video" && strings.Contains(strings.ToLower(item.Title), "try") && strings.Contains(strings.ToLower(item.Title), "official video") && strings.Contains(strings.ToLower(item.Artist), "p!nk") {
			id = item.ID
			break
		}
	}
	if !videoIDPattern.MatchString(id) {
		evidence.Category = "video_not_matched"
		t.Fatal("official MV not present in public results")
	}
	evidence.OfficialVideoMatched = true
	p, err := New()
	if err != nil {
		t.Fatal("MV runtime unavailable")
	}
	defer p.Close()
	initializeAnonymousLive(t, p)
	v, err := NewVideo(p)
	if err != nil {
		t.Fatal("MV runtime unavailable")
	}
	defer v.Close()
	resolve := v.resolve
	v.resolve = func(ctx context.Context, id string, quality int) (videoMedia, error) {
		info, err := resolve(ctx, id, quality)
		if err != nil {
			evidence.ResolutionCategory = safeError(err)
			evidence.ResolutionStage = safeErrorStage(err)
		}
		return info, err
	}
	var baseline []byte
	for _, anchor := range []time.Duration{73 * time.Second, 117 * time.Second} {
		started := time.Now()
		if v.Start(ctx, id, func() time.Duration { return anchor + time.Since(started) }, func() bool { return true }) != nil {
			t.Fatal("MV start failed")
		}
		for v.Loading() && ctx.Err() == nil {
			time.Sleep(20 * time.Millisecond)
		}
		frame, first := v.Frame()
		if frame == nil || v.Error() != "" || ctx.Err() != nil {
			evidence.VideoError = v.Error()
			evidence.Category = "first_frame_failed"
			t.Fatal("official MV first frame failed")
		}
		evidence.StartupMS += time.Since(started).Milliseconds()
		evidence.Width, evidence.Height = frame.Bounds().Dx(), frame.Bounds().Dy()
		if dir := os.Getenv("YTMGO_LIVE_MV_FRAMES"); dir != "" {
			if os.MkdirAll(dir, 0700) != nil {
				t.Fatal("public MV frame directory failed")
			}
			file, err := os.Create(filepath.Join(dir, fmt.Sprintf("mv-%d.png", int(anchor.Seconds()))))
			if err != nil {
				t.Fatal("public MV frame file failed")
			}
			err = png.Encode(file, frame)
			closed := file.Close()
			if err != nil || closed != nil {
				t.Fatal("public MV frame export failed")
			}
		}
		if baseline == nil {
			baseline = videoContentSample(frame)
		}
		until := time.Now().Add(3 * time.Second)
		var last uint64
		for time.Now().Before(until) {
			frame, sequence := v.Frame()
			if frame == nil || v.Error() != "" || ctx.Err() != nil {
				evidence.Category = "playback_failed"
				t.Fatal("official MV presentation failed")
			}
			pixels := videoContentSample(frame)
			difference := 0
			for i, value := range pixels {
				d := int(value) - int(baseline[i])
				if d < 0 {
					d = -d
				}
				difference += d
			}
			ratio := float64(difference) / float64(len(pixels)*255)
			evidence.Samples++
			if ratio > 0.03 {
				evidence.MovingSamples++
			}
			evidence.MaximumPixelDifference = max(evidence.MaximumPixelDifference, ratio)
			last = sequence
			time.Sleep(50 * time.Millisecond)
		}
		evidence.FramesAdvanced += last - first
		v.Stop()
	}
	if evidence.MovingSamples < 3 || evidence.FramesAdvanced < 24 {
		evidence.Category = "visual_motion_missing"
		t.Fatal("official MV did not demonstrate visual motion")
	}
	evidence.Category = "pass"
}

func videoContentSample(frame image.Image) []byte {
	b := frame.Bounds()
	data := make([]byte, 0, 16*16*3)
	for y := 0; y < 16; y++ {
		for x := 0; x < 16; x++ {
			r, g, blue, _ := frame.At(b.Min.X+(2*x+1)*b.Dx()/32, b.Min.Y+(2*y+1)*b.Dy()/32).RGBA()
			data = append(data, byte(r>>8), byte(g>>8), byte(blue>>8))
		}
	}
	return data
}
