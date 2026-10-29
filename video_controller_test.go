package main

import (
	"context"
	"fmt"
	"image"
	"testing"

	"github.com/egoist/mygo/ui"
	"ytmgo/internal/catalog"
	"ytmgo/internal/player"
)

func TestNativeVideoErrorOverridesOldFrameAndRetriesOnlyVideo(t *testing.T) {
	for _, language := range []string{"en", "zh-Hans", "zh-Hant", "ja"} {
		for _, width := range []int{420, 761, 960, 1440} {
			t.Run(fmt.Sprintf("%s-%d", language, width), func(t *testing.T) {
				m, a := nativeViewFixture()
				m.Language, m.Immersive, m.Video = language, true, true
				m.VideoError = a.Tr("nativeVideoUnavailable")
				m.VideoLoading = true // A delayed snapshot must not hide the error.
				m.VideoFrame = ui.NewBitmap(image.NewRGBA(image.Rect(0, 0, 160, 90)))
				m.Position, m.Volume, m.Muted = 73.125, 0, true
				track, playing := m.Current, m.Playing
				videoRetries, audioRetries := 0, 0
				a.Video = func(value bool) {
					if !value {
						t.Fatal("video retry switched to cover")
					}
					videoRetries++
				}
				a.RetryPlayback = func() { audioRetries++ }
				tt := ui.NewTester(func(c *ui.Context) { nativeView(c, m, a) }, width, 640)
				if !tt.HasText(m.VideoError) {
					t.Fatal("old video frame concealed the playback error")
				}
				nativeAssertVisible(t, tt, a.Tr("buttonRetry"), width, 640)
				if err := tt.Click(a.Tr("buttonRetry")); err != nil {
					t.Fatal(err)
				}
				if videoRetries != 1 || audioRetries != 0 || m.Current != track || m.Playing != playing || m.Position != 73.125 || m.Volume != 0 || !m.Muted {
					t.Fatal("video retry changed the audio session")
				}
				if width == 761 && language == "ja" || width == 960 && language == "zh-Hant" {
					nativeSnapshot(t, tt, fmt.Sprintf("video-error-retry-%s-%d", language, width))
				}
			})
		}
	}
}

func TestNativeVideoLoadingDoesNotPresentOldFrameAsCurrent(t *testing.T) {
	m, a := nativeViewFixture()
	m.Video, m.Immersive, m.VideoLoading = true, true, true
	m.VideoFrame = ui.NewBitmap(image.NewRGBA(image.Rect(0, 0, 160, 90)))
	tt := ui.NewTester(func(c *ui.Context) { nativeView(c, m, a) }, 960, 640)
	nativeAssertVisible(t, tt, a.Tr("shellLoading"), 960, 640)
	if tt.HasText(a.Tr("buttonRetry")) {
		t.Fatal("loading video prematurely offered retry")
	}
}

func TestVideoErrorClearsStaleFrameAndIgnoresSupersededGeneration(t *testing.T) {
	frame := image.NewRGBA(image.Rect(0, 0, 8, 8))
	bitmap := ui.NewBitmap(frame)
	a := &desktop{videoID: "aaaaaaaaaaa", videoFrame: bitmap, videoFrameNumber: 5,
		model: UIModel{Video: true, VideoLoading: true, VideoFrame: bitmap, Playing: true, Position: 73.125, Muted: true}}
	a.videoActive.Store(true)
	a.videoGeneration.Store(2)
	a.applyVideoFrame(2, frame, 5, true, "video_network_timeout")
	if a.model.VideoLoading || a.model.VideoError == "" || a.model.VideoFrame != nil || a.videoFrame != nil {
		t.Fatal("video error did not replace the stale frame and spinner")
	}
	a.applyVideoFrame(1, frame, 6, false, "")
	if a.model.VideoError == "" || a.model.VideoFrame != nil {
		t.Fatal("superseded video poll erased the failure")
	}
	a.applyVideoFrame(2, frame, 6, false, "")
	if a.model.VideoError != "" || a.model.VideoFrame == nil {
		t.Fatal("successful retry did not replace the error")
	}
	if !a.model.Playing || a.model.Position != 73.125 || !a.model.Muted {
		t.Fatal("video status changed audio state")
	}
	a.videoActive.Store(false)
	a.applyVideoFrame(2, nil, 7, false, "video_network_timeout")
	if a.model.VideoError != "" {
		t.Fatal("inactive video accepted a delayed error")
	}
}

func TestVideoRetryReentersSameIDWithoutRestartingAudio(t *testing.T) {
	// A closed Video deterministically rejects Start without a runtime/network.
	// That rejection distinguishes a real retry from the former same-ID no-op.
	v := &player.Video{}
	v.Close()
	p := &player.Player{}
	frame := ui.NewBitmap(image.NewRGBA(image.Rect(0, 0, 8, 8)))
	a := &desktop{ctx: context.Background(), video: v, player: p, windowVisible: true, videoID: "aaaaaaaaaaa", videoFrame: frame,
		model: UIModel{Current: catalog.Item{ID: "aaaaaaaaaaa"}, Panel: true, Video: true, VideoError: "previous failure", VideoFrame: frame, Playing: true, Position: 73.125, Muted: true}}
	a.videoActive.Store(true)
	before := p.Snapshot()
	a.syncVideo()
	if a.videoGeneration.Load() != 0 {
		t.Fatal("ordinary rendering retried the failed video automatically")
	}
	if !a.syncVideoSession(true) {
		t.Fatal("closed video did not reject an actual retry")
	}
	if a.videoGeneration.Load() != 1 || a.videoID != "aaaaaaaaaaa" || a.model.VideoError != "" || a.model.VideoFrame != nil || a.model.VideoLoading || a.videoActive.Load() {
		t.Fatal("explicit retry did not restart the failed same-ID video")
	}
	if p.Snapshot() != before || !a.model.Playing || a.model.Position != 73.125 || !a.model.Muted {
		t.Fatal("video retry altered audio playback")
	}
	a.syncVideo()
	if a.videoGeneration.Load() != 1 {
		t.Fatal("rejected retry looped on the next presentation update")
	}
}

func TestVideoRetryPreservesVisibilityAndCoverResourceRules(t *testing.T) {
	for _, mode := range []string{"hidden", "cover", "panel_closed"} {
		t.Run(mode, func(t *testing.T) {
			v := &player.Video{}
			defer v.Close()
			a := &desktop{ctx: context.Background(), video: v, player: &player.Player{}, windowVisible: true, videoID: "aaaaaaaaaaa",
				model: UIModel{Current: catalog.Item{ID: "aaaaaaaaaaa"}, Panel: true, Video: true, VideoError: "failure", VideoLoading: true}}
			a.videoActive.Store(true)
			if mode == "hidden" {
				a.windowVisible = false
			} else if mode == "panel_closed" {
				a.model.Panel = false
			}
			a.setVideo(mode != "cover")
			if a.videoID != "" || a.videoActive.Load() || a.model.VideoLoading || a.model.VideoError != "" {
				t.Fatal("hidden/cover presentation retained or restarted video")
			}
		})
	}
}
