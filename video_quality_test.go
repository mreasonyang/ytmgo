package main

import (
	"context"
	"fmt"
	"image"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
	"ytmgo/internal/catalog"
	"ytmgo/internal/player"
	"ytmgo/internal/videoquality"
)

func TestVideoQualityPreferenceRestartsOnlyVisibleVideo(t *testing.T) {
	for _, visible := range []bool{true, false} {
		t.Run(fmt.Sprint(visible), func(t *testing.T) {
			v := &player.Video{}
			v.Close() // A rejected Start proves the video restart was attempted.
			p := &player.Player{}
			a := &desktop{ctx: context.Background(), player: p, video: v, windowVisible: visible,
				videoQualityPath: filepath.Join(t.TempDir(), "video-quality"),
				videoAvailableID: "aaaaaaaaaaa",
				model: UIModel{Current: catalog.Item{ID: "aaaaaaaaaaa"}, Panel: true, Video: true, VideoAvailable: true, VideoQuality: 720,
					Playing: true, Position: 73.125, Volume: 0.7, Muted: true}}
			if visible {
				a.videoID = a.model.Current.ID
				a.videoActive.Store(true)
				a.model.VideoFrame = ui.NewBitmap(image.NewRGBA(image.Rect(0, 0, 8, 8)))
			}
			before := p.Snapshot()
			rejected := a.changeVideoQuality(1080)
			if a.model.VideoQuality != 1080 || videoquality.Load(a.videoQualityPath) != 1080 {
				t.Fatal("choice was not saved for the next application launch")
			}
			if visible {
				if !rejected || a.videoGeneration.Load() != 1 || a.model.VideoFrame != nil {
					t.Fatal("visible video did not restart or retained its old frame")
				}
			} else if a.videoGeneration.Load() != 0 || a.videoActive.Load() {
				t.Fatal("changing quality started a hidden video")
			}
			if after := p.Snapshot(); after != before || a.model.Position != 73.125 || !a.model.Playing || a.model.Volume != 0.7 || !a.model.Muted {
				t.Fatal("changing quality altered audio playback")
			}
		})
	}
}

func TestVideoQualitySaveFailureKeepsCurrentVideo(t *testing.T) {
	path := filepath.Join(t.TempDir(), "video-quality")
	if err := videoquality.Save(path, 720); err != nil {
		t.Fatal(err)
	}
	a := &desktop{videoQualityPath: filepath.Join(path, "child"), videoID: "aaaaaaaaaaa", model: UIModel{VideoQuality: 720}}
	a.setVideoQuality(1080)
	if a.model.VideoQuality != 720 || a.model.VideoQualityError == "" || a.videoGeneration.Load() != 0 || videoquality.Load(path) != 720 {
		t.Fatal("failed save changed the preference or restarted playback")
	}
}

func TestNativeVideoQualityControls(t *testing.T) {
	for _, location := range []string{"settings", "player"} {
		for _, language := range []string{"en", "zh-Hans", "zh-Hant", "ja"} {
			for _, width := range []int{380, 1440} {
				t.Run(fmt.Sprintf("%s-%s-%d", location, language, width), func(t *testing.T) {
					const height = 740
					m, a := nativeViewFixture()
					m.Language = language
					m.VideoQuality = 720
					m.Settings = location == "settings"
					m.Immersive, m.Video = location == "player", location == "player"
					track, position, playing, volume := m.Current, m.Position, m.Playing, m.Volume
					tt := ui.NewTester(func(c *ui.Context) { nativeView(c, m, a) }, width, height)
					for attempt := 0; attempt < 8; attempt++ {
						r, ok := tt.Find("720p")
						if ok && r.Y > 70 && r.Y+r.H < height-45 {
							break
						}
						tt.Scroll(float32(width)/2, height/2, 0, 120)
					}
					nativeAssertVisible(t, tt, "720p", width, height)
					for _, quality := range []int{1080, 360, 720} {
						if err := tt.Click(videoquality.Label(m.VideoQuality)); err != nil {
							t.Fatal(err)
						}
						if err := tt.Click(videoquality.Label(quality)); err != nil {
							t.Fatal(err)
						}
						if m.VideoQuality != quality {
							t.Fatalf("selected %dp, model remained %dp", quality, m.VideoQuality)
						}
					}
					if m.Current != track || m.Position != position || m.Playing != playing || m.Volume != volume {
						t.Fatal("quality control changed audio state")
					}
					if language == "en" {
						nativeSnapshot(t, tt, fmt.Sprintf("video-quality-%s-%d", location, width))
					}
				})
			}
		}
	}
}
