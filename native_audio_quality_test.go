package main

import (
	"fmt"
	"testing"

	"github.com/egoist/mygo/ui"
	"ytmgo/internal/audioquality"
)

func TestNativeAudioQualitySelection(t *testing.T) {
	for _, language := range []string{"en", "zh-Hans", "zh-Hant", "ja"} {
		for _, width := range []int{380, 1440} {
			t.Run(fmt.Sprintf("%s-%d", language, width), func(t *testing.T) {
				const height = 900
				m, a := nativeViewFixture()
				m.Language, m.Settings = language, true
				saves := 0
				rejectSave := false
				a.SetAudioQuality = func(quality audioquality.Quality) {
					saves++
					if rejectSave {
						m.AudioQualityNotice = "nativeAudioQualitySaveError"
						return
					}
					m.AudioQuality, m.AudioQualityNotice = quality, "nativeAudioQualitySaved"
				}
				tt := ui.NewTester(func(c *ui.Context) { nativeView(c, m, a) }, width, height)
				click := func(key string) {
					t.Helper()
					if err := tt.Click(a.Tr(key)); err != nil {
						t.Fatal(err)
					}
				}
				nativeAssertVisible(t, tt, a.Tr("nativeAudioQualityBest"), width, height)
				if !tt.HasText(a.Tr("nativeAudioQualityHint")) {
					t.Fatal("settings missing the next-load hint")
				}
				click("nativeAudioQualityBest")
				if language == "zh-Hans" && width == 1440 {
					nativeSnapshot(t, tt, "settings-audio-quality-zh-Hans")
				}
				tt.Key(0, ui.KeyEnter)
				if saves != 0 || tt.HasText(a.Tr("nativeAudioQualitySaver")) {
					t.Fatal("choosing the selected option should only dismiss the menu")
				}
				tt.Key(0, ui.KeyDown)
				tt.Key(0, ui.KeyUp)
				tt.Key(0, ui.KeyEscape)
				if !m.Settings || saves != 0 || tt.HasText(a.Tr("nativeAudioQualitySaver")) {
					t.Fatal("Escape should dismiss only the menu without saving")
				}
				tt.Key(0, ui.KeySpace)
				tt.Key(0, ui.KeyHome)
				tt.Key(0, ui.KeyDown)
				tt.Key(0, ui.KeyEnter)
				if saves != 1 || m.AudioQuality != audioquality.Standard || !tt.HasText(a.Tr("nativeAudioQualitySaved")) {
					t.Fatalf("keyboard selection did not save Standard (calls %d, quality %q)", saves, m.AudioQuality)
				}
				rejectSave = true
				click("nativeAudioQualityStandard")
				click("nativeAudioQualitySaver")
				if !tt.HasText(a.Tr("nativeAudioQualityStandard")) || !tt.HasText(a.Tr("nativeAudioQualitySaveError")) {
					t.Fatal("failed save should keep the prior selection with a retry message")
				}
				tt.Key(0, ui.KeyEscape)
				click("shellSettings")
				if !m.Settings || !tt.HasText(a.Tr("nativeAudioQualityHint")) || !tt.HasText(a.Tr("nativeAudioQualityStandard")) {
					t.Fatal("reopened settings should show the saved choice with its normal hint")
				}
			})
		}
	}
}
