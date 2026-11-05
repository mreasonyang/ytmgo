package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
	"ytmgo/internal/player"
)

func TestCancelledLoginIsNeutralAndPreservesCatalogFailure(t *testing.T) {
	for _, scope := range []errorScope{errorAccountConnect, errorCatalog} {
		a := &desktop{diagnosticPath: t.TempDir(), model: UIModel{Error: "existing failure", ErrorScope: scope}}
		a.accountLoginCancelled()
		if a.model.AccountNotice != "nativeAccountLoginCancelled" || (scope == errorAccountConnect && a.model.Error != "") || (scope == errorCatalog && a.model.Error != "existing failure") {
			t.Fatal("cancellation became a failure or cleared an unrelated error")
		}
		data, err := os.ReadFile(filepath.Join(a.diagnosticPath, "account-status.json"))
		var row struct{ Category string }
		if err != nil || json.Unmarshal(data, &row) != nil || row.Category != "cancelled" {
			t.Fatal("cancellation was recorded as a failed session")
		}
	}
}

func TestNativeNowPlayingExplainsAccessDenied(t *testing.T) {
	for _, locale := range []string{"en", "ja", "zh-Hans", "zh-Hant"} {
		for _, width := range []int{420, 1440} {
			t.Run(fmt.Sprintf("%s-%d", locale, width), func(t *testing.T) {
				m, actions := nativeViewFixture()
				m.Language, m.Panel, m.Playing = locale, true, false
				state := player.State{Error: "audio_access_denied", ErrorDetails: player.FailureDetails{Stage: "media_stream", HTTPStatus: 403}}
				m.PlaybackError = actions.TrArgs(playbackServiceErrorKey(state), map[string]any{"status": 403})
				retries := 0
				actions.RetryPlayback = func() { retries++ }
				tt := ui.NewTester(func(c *ui.Context) { nativeView(c, m, actions) }, width, 940)
				if !strings.Contains(m.PlaybackError, "403") || !tt.HasText(m.PlaybackError) || tt.HasText(actions.Tr("nativePlaybackError")) {
					t.Fatal("Now Playing lost the specific HTTP denial and retry guidance")
				}
				if err := tt.Click(actions.Tr("buttonRetry")); err != nil || retries != 1 {
					t.Fatal("specific playback notice lost its retry action")
				}
				nativeSnapshot(t, tt, fmt.Sprintf("access-denied-%s-%d", locale, width))
			})
		}
	}
}

func TestNativeCancelledLoginAndRegionErrorTranslations(t *testing.T) {
	for _, locale := range []string{"en", "ja", "zh-Hans", "zh-Hant"} {
		t.Run(locale, func(t *testing.T) {
			m, actions := nativeViewFixture()
			m.Language = locale
			m.AccountNotice = "nativeAccountLoginCancelled"
			m.PlaybackError = actions.Tr(playbackServiceErrorKey(player.State{Error: "region_unavailable"}))
			tt := ui.NewTester(func(c *ui.Context) { nativeView(c, m, actions) }, 1440, 940)
			if !tt.HasText(actions.Tr(m.AccountNotice)) || tt.HasText(actions.Tr("nativeAccountLoginFailed")) {
				t.Fatal("cancelled login has no neutral localized notice")
			}
			if m.PlaybackError == "" || m.PlaybackError == "nativeMusicRegionUnavailable" || !tt.HasText(m.PlaybackError) {
				t.Fatal("region error has no localized explanation")
			}
			nativeSnapshot(t, tt, "e2e-regression-"+locale)
		})
	}
}

func TestPlaybackReadyReplacesStaleFailureMetadata(t *testing.T) {
	dir := t.TempDir()
	recordPlaybackDiagnostic(dir, "audio", "audio_access_denied", player.FailureDetails{Stage: "media_stream", HTTPStatus: 403})
	recordPlaybackDiagnostic(dir, "audio", "ready")
	data, err := os.ReadFile(filepath.Join(dir, "audio-status.json"))
	var row struct {
		Category, Stage string
		HTTPStatus      int
	}
	if err != nil || json.Unmarshal(data, &row) != nil || row.Category != "ready" || row.Stage != "" || row.HTTPStatus != 0 {
		t.Fatal("successful playback retained stale failure details")
	}
}
