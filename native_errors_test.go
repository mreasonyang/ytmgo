package main

import (
	"fmt"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestNativeServiceErrorsUseExistingRetries(t *testing.T) {
	for _, locale := range []string{"en", "zh-Hans", "zh-Hant", "ja"} {
		for _, playback := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s-playback-%v", locale, playback), func(t *testing.T) {
				m, a := nativeViewFixture()
				m.Language, m.Playing = locale, false
				message := a.Tr("nativeVerificationRequired")
				reloads, plays := 0, 0
				a.Reload = func() { reloads++ }
				a.RetryPlayback = func() { plays++ }
				if playback {
					m.PlaybackError, m.Immersive = message, true
				} else {
					m.Error, m.ErrorScope = message, errorCatalog
				}
				tt := ui.NewTester(func(c *ui.Context) { nativeView(c, m, a) }, 420, 740)
				if !tt.HasText(message) {
					t.Fatal("service failure has no explanation")
				}
				if _, exists := tt.Find(a.Tr("nativeAccountOpenMusic")); exists {
					t.Fatal("service error must not offer browser verification")
				}
				nativeAssertVisible(t, tt, a.Tr("buttonRetry"), 420, 740)
				if err := tt.Click(a.Tr("buttonRetry")); err != nil {
					t.Fatal(err)
				}
				if playback && (plays != 1 || reloads != 0) || !playback && (reloads != 1 || plays != 0) {
					t.Fatal("ordinary retry reached the wrong failed operation")
				}
				nativeSnapshot(t, tt, fmt.Sprintf("service-error-%s-playback-%v", locale, playback))
			})
		}
	}
}

func TestNativeAccountErrorCannotRetryCatalog(t *testing.T) {
	for _, scope := range []errorScope{errorAccount, errorAccountConnect} {
		t.Run(string(scope), func(t *testing.T) {
			m, a := nativeViewFixture()
			m.Error, m.ErrorScope = a.Tr("nativeVerificationRequired"), scope
			tt := ui.NewTester(func(c *ui.Context) { nativeView(c, m, a) }, 420, 740)
			if !tt.HasText(m.Error) {
				t.Fatal("account failure has no explanation")
			}
			if _, exists := tt.Find(a.Tr("buttonRetry")); exists {
				t.Fatal("account failure must not retry an unrelated catalog request")
			}
		})
	}
}
