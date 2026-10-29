package main

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"testing"

	"github.com/egoist/mygo/ui"
)

var videoLayoutEdges = [4]color.RGBA{{231, 17, 33, 255}, {17, 49, 231, 255}, {31, 231, 49, 255}, {231, 207, 17, 255}}

func videoLayoutFrame(width, height int) *image.RGBA {
	frame := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			pixel := color.RGBA{90, 90, 90, 255}
			switch {
			case y < 8:
				pixel = videoLayoutEdges[0]
			case y >= height-8:
				pixel = videoLayoutEdges[1]
			case x < 8:
				pixel = videoLayoutEdges[2]
			case x >= width-8:
				pixel = videoLayoutEdges[3]
			}
			frame.SetRGBA(x, y, pixel)
		}
	}
	return frame
}

func TestNativeShortImmersiveVideoUsesHeightBudget(t *testing.T) {
	coverColor := color.RGBA{237, 19, 173, 255}
	coverImage := image.NewRGBA(image.Rect(0, 0, 16, 16))
	for y := 0; y < 16; y++ {
		for x := 0; x < 16; x++ {
			coverImage.SetRGBA(x, y, coverColor)
		}
	}
	cover := ui.NewBitmap(coverImage)
	for _, width := range []int{761, 960, 1440} {
		for _, height := range []int{609, 640} {
			for _, language := range []string{"en", "zh-Hant"} {
				for _, appearance := range []string{"dark", "light"} {
					t.Run(fmt.Sprintf("%dx%d-%s-%s", width, height, language, appearance), func(t *testing.T) {
						m, a := nativeViewFixture()
						m.Immersive, m.Video = true, true
						m.Language, m.Appearance = language, appearance
						m.Current.Title = "Daft Punk : Get Lucky // Grammys Rehearsal (HD)"
						m.Current.Artist = "Daft Punk – Archives"
						m.Current.Artwork = "distinct-cover"
						m.VideoFrame = ui.NewBitmap(videoLayoutFrame(160, 90))
						originalArtwork := a.Artwork
						a.Artwork = func(source string, edge int) ui.ImageSource {
							if source == "distinct-cover" {
								return cover
							}
							return originalArtwork(source, edge)
						}
						// 609/640 windows leave 409/440 DIPs of immersive content: the
						// cover takes what the switch, gaps and the title block, as the
						// fonts lay it out, leave; a video keeps that height budget.
						mediaWidth := float32(width) - 16 - 80 - 40 - min(380, float32(width)*0.32)
						want := float32(height-200) - 32 - 48 - nativeImmersiveTitleHeight(m, a, mediaWidth)
						tt := ui.NewTester(func(c *ui.Context) { nativeView(c, m, a) }, width, height)
						video := nativeAssertVisible(t, tt, a.Tr("nativeVideo"), width, height)
						viewport := nativeAssertVisible(t, tt, a.Tr("shellContent"), width, height)
						if math.Abs(float64(video.W-min(mediaWidth, want*16/9))) > 2 || video.W > viewport.W+1 || video.X < viewport.X-1 || video.X+video.W > viewport.X+viewport.W+1 || math.Abs(float64(video.W/video.H-16.0/9.0)) > 0.01 {
							t.Fatalf("short-window video wastes width or overflows its column: video=%+v viewport=%+v budget=%v", video, viewport, want)
						}
						nativeAssertVisible(t, tt, m.Current.Title, width, height)
						artist := nativeAssertVisible(t, tt, m.Current.Artist, width, height)
						if artist.Y+artist.H > float32(height-nativePlayerHeight-24) {
							t.Fatal("larger video displaced the artist into the player bar")
						}
						nativeAssertVisible(t, tt, a.Tr("shellPause"), width, height)
						nativeAssertVisible(t, tt, a.Tr("nativeArtwork"), width, height)
						if width == 960 && height == 609 && language == "zh-Hant" && appearance == "dark" {
							nativeSnapshot(t, tt, "short-immersive-video-expanded")
						}
						if err := tt.Click(a.Tr("nativeArtwork")); err != nil || m.Video {
							t.Fatal("cover switch is no longer reachable")
						}
						// Cover geometry must fill the same height budget.
						bounds := image.Rectangle{}
						rendered := tt.Image()
						for y := 0; y < height-nativePlayerHeight; y++ {
							for x := 0; x < width; x++ {
								if rendered.RGBAAt(x, y) == coverColor {
									bounds = bounds.Union(image.Rect(x, y, x+1, y+1))
								}
							}
						}
						if math.Abs(float64(bounds.Dx())-float64(want)) > 2 || math.Abs(float64(bounds.Dy())-float64(want)) > 2 {
							t.Fatalf("cover dimensions changed: got=%v want=%v square", bounds.Size(), want)
						}
						if err := tt.Click(a.Tr("nativeVideo")); err != nil || !m.Video {
							t.Fatal("video switch is no longer reachable")
						}
						if current := nativeAssertVisible(t, tt, a.Tr("nativeVideo"), width, height); math.Abs(float64(current.W-video.W)) > 1 {
							t.Fatal("cover/video switch changed the video layout budget")
						}
					})
				}
			}
		}
	}
}

func TestNativeImmersiveVideoContainsAllSourceEdges(t *testing.T) {
	for _, dimensions := range [][2]int{{160, 90}, {160, 120}, {160, 160}, {90, 160}} {
		t.Run(fmt.Sprintf("%dx%d", dimensions[0], dimensions[1]), func(t *testing.T) {
			m, a := nativeViewFixture()
			m.Immersive, m.Video = true, true
			m.VideoFrame = ui.NewBitmap(videoLayoutFrame(dimensions[0], dimensions[1]))
			tt := ui.NewTester(func(c *ui.Context) { nativeView(c, m, a) }, 960, 609)
			video := nativeAssertVisible(t, tt, a.Tr("nativeVideo"), 960, 609)
			counts := [4]int{}
			rendered := tt.Image()
			for y := int(math.Ceil(float64(video.Y))); y < int(video.Y+video.H); y++ {
				for x := int(math.Ceil(float64(video.X))); x < int(video.X+video.W); x++ {
					pixel := rendered.RGBAAt(x, y)
					for edge, want := range videoLayoutEdges {
						if pixel == want {
							counts[edge]++
						}
					}
				}
			}
			for edge, count := range counts {
				if count < 50 {
					t.Fatalf("source edge %d cropped after expansion: pixels=%d", edge, count)
				}
			}
		})
	}
}

func TestNativeCompactShortVideoUsesWidth(t *testing.T) {
	for _, size := range [][2]int{{380, 609}, {420, 609}, {420, 740}, {757, 641}, {760, 609}} {
		for _, language := range []string{"en", "ja", "zh-Hans", "zh-Hant"} {
			t.Run(fmt.Sprintf("%dx%d-%s", size[0], size[1], language), func(t *testing.T) {
				width, height := size[0], size[1]
				m, a := nativeViewFixture()
				m.Immersive, m.Video, m.Language = true, true, language
				m.Current.Title = "Daft Punk : Get Lucky // Grammys Rehearsal (HD) — Extended Live Archive Edition"
				m.VideoFrame = ui.NewBitmap(videoLayoutFrame(160, 90))
				tt := ui.NewTester(func(c *ui.Context) { nativeView(c, m, a) }, width, height)
				video := nativeAssertVisible(t, tt, a.Tr("nativeVideo"), width, height)
				// The square-cover budget is 180 high; video may use 16:9 of it.
				want := min(float32(width-64), 320)
				if video.W < want-1 || video.W > float32(width-64)+1 || math.Abs(float64(video.W/video.H-16.0/9.0)) > 0.01 {
					t.Fatalf("compact video undersized: %+v want width %.0f", video, want)
				}
				nativeAssertVisible(t, tt, a.Tr("nativeArtwork"), width, height)
				nativeAssertVisible(t, tt, a.Tr("shellPause"), width, height)
				if size == [2]int{757, 641} && language == "en" {
					nativeSnapshot(t, tt, "compact-short-video-757")
				}
			})
		}
	}
}
