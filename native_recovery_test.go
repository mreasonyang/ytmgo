package main

import (
	"fmt"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestNativeRecoveryEntryPoints(t *testing.T) {
	for _, width := range []int{420, 1440} {
		for _, immersive := range []bool{false, true} {
			for _, locale := range []string{"en", "zh-Hans", "zh-Hant", "ja"} {
				t.Run(fmt.Sprintf("%d-%v-%s", width, immersive, locale), func(t *testing.T) {
					m, a := nativeViewFixture()
					m.Language, m.Immersive, m.Playing = locale, immersive, false
					m.PlaybackError = a.Tr("nativeVerificationRequired")
					m.PlaybackRecovery = recoveryNotice{Kind: recoveryChallenge, Scope: recoveryPlayback}
					a.ShowRecovery = func(n recoveryNotice) {
						m.Recovery, m.RecoveryOpen = n, true
					}
					tt := ui.NewTester(func(c *ui.Context) { nativeView(c, m, a) }, width, 900)
					nativeAssertVisible(t, tt, a.Tr("nativeRecoveryHelp"), width, 900)
					if err := tt.Click(a.Tr("nativeRecoveryHelp")); err != nil {
						t.Fatal(err)
					}
					if !m.RecoveryOpen || m.Recovery != m.PlaybackRecovery {
						t.Fatal("playback recovery did not open for its failure")
					}
					if !tt.HasText(a.Tr("nativeVerificationHelp")) {
						t.Fatal("opening recovery did not render the explanation")
					}
				})
			}
		}
	}
	for _, scope := range []recoveryScope{recoveryCatalog, recoveryAccount} {
		t.Run(string(scope), func(t *testing.T) {
			m, a := nativeViewFixture()
			m.Error = a.Tr("nativeConsentRequired")
			m.ErrorRecovery = recoveryNotice{Kind: recoveryConsent, Scope: scope}
			a.ShowRecovery = func(n recoveryNotice) { m.Recovery, m.RecoveryOpen = n, true }
			tt := ui.NewTester(func(c *ui.Context) { nativeView(c, m, a) }, 420, 740)
			if err := tt.Click(a.Tr("nativeRecoveryHelp")); err != nil {
				t.Fatal(err)
			}
			if !m.RecoveryOpen || m.Recovery != m.ErrorRecovery || !tt.HasText(a.Tr("nativeConsentHelp")) {
				t.Fatal("page/account recovery lost the source or explanation")
			}
		})
	}
}

func TestNativeRecoveryDialogActionsAndAccountStates(t *testing.T) {
	for _, locale := range []string{"en", "zh-Hans", "zh-Hant", "ja"} {
		for _, identity := range []string{"anonymous", "standard", "premium"} {
			for _, width := range []int{420, 1100} {
				t.Run(fmt.Sprintf("%s-%s-%d", locale, identity, width), func(t *testing.T) {
					m, a := nativeViewFixture()
					m.Language = locale
					m.SignedIn, m.MusicPremium = identity != "anonymous", identity == "premium"
					m.PlaybackError = a.Tr("nativeVerificationRequired")
					m.Recovery = recoveryNotice{Kind: recoveryChallenge, Scope: recoveryPlayback}
					m.RecoveryOpen = true
					opened, retries, logins := 0, 0, 0
					a.OpenRecoveryBrowser = func() { opened++; m.RecoveryBrowserFailed = true }
					a.RetryRecovery = func() { retries++; m.RecoveryOpen = false }
					a.RecoveryAccount = func() { logins++; m.RecoveryOpen = false }
					tt := ui.NewTester(func(c *ui.Context) {
						nativeRecoveryDialog(c, m, a, palette(false), float32(width), 740)
					}, width, 740)
					for _, key := range []string{"nativeAccountOpenMusic", "buttonRetry", "shellClose"} {
						nativeAssertVisible(t, tt, a.Tr(key), width, 740)
					}
					_, loginShown := tt.Find(a.Tr("shellSignIn"))
					if loginShown == m.SignedIn {
						t.Fatal("login action must be shown only when signed out")
					}
					hint := "nativeRecoverySignInHint"
					if m.SignedIn {
						hint = "nativeRecoveryConnectedHint"
					}
					if !tt.HasText(a.Tr(hint)) || !tt.HasText(a.Tr("nativeRecoveryLimits")) {
						t.Fatal("recovery omitted account context or verification limitation")
					}
					if identity == "anonymous" && (locale == "en" || locale == "zh-Hans") {
						nativeSnapshot(t, tt, fmt.Sprintf("recovery-%s-%d", locale, width))
					}
					if err := tt.Click(a.Tr("nativeAccountOpenMusic")); err != nil {
						t.Fatal(err)
					}
					tt.Frame()
					if opened != 1 || !m.RecoveryOpen || m.PlaybackError == "" || !tt.HasText(a.Tr("nativeAccountBrowserError")) {
						t.Fatal("browser opening must preserve failure and show its own error")
					}
					if err := tt.Click(a.Tr("buttonRetry")); err != nil {
						t.Fatal(err)
					}
					if retries != 1 || m.RecoveryOpen || logins != 0 {
						t.Fatal("explicit retry did not reach its action")
					}
					m.RecoveryOpen = true
					tt.Frame()
					if !m.SignedIn {
						if err := tt.Click(a.Tr("shellSignIn")); err != nil {
							t.Fatal(err)
						}
						if logins != 1 || m.RecoveryOpen {
							t.Fatal("sign-in action did not close recovery and reach account flow")
						}
						m.RecoveryOpen = true
						tt.Frame()
					}
					if err := tt.Click(a.Tr("shellClose")); err != nil {
						t.Fatal(err)
					}
					if m.RecoveryOpen || m.PlaybackError == "" {
						t.Fatal("closing recovery must preserve the unresolved failure")
					}
				})
			}
		}
	}
}

func TestNativeRecoveryDistinctServiceRequests(t *testing.T) {
	for _, row := range []struct {
		kind recoveryKind
		body string
	}{{recoveryChallenge, "nativeVerificationHelp"}, {recoveryConsent, "nativeConsentHelp"}, {recoverySignIn, "nativeServiceSignInHelp"}, {recoveryRedirect, "nativeServiceRedirectHelp"}} {
		t.Run(string(row.kind), func(t *testing.T) {
			m, a := nativeViewFixture()
			m.Recovery = recoveryNotice{Kind: row.kind, Scope: recoveryCatalog}
			m.RecoveryOpen = true
			tt := ui.NewTester(func(c *ui.Context) { nativeRecoveryDialog(c, m, a, palette(false), 420, 580) }, 420, 580)
			if !tt.HasText(a.Tr(row.body)) || !tt.HasText(a.Tr(recoveryMessageKey(row.kind))) {
				t.Fatal("service request classification has the wrong explanation")
			}
			for _, key := range []string{"nativeAccountOpenMusic", "buttonRetry", "shellSignIn", "shellClose"} {
				nativeAssertVisible(t, tt, a.Tr(key), 420, 580)
			}
		})
	}
}

func TestNativeRecoveryNewConnectionDoesNotClaimPreviousAccountSucceeded(t *testing.T) {
	for _, signedIn := range []bool{true, false} {
		for _, locale := range []string{"en", "zh-Hans", "zh-Hant", "ja"} {
			t.Run(fmt.Sprintf("%s-existing-%v", locale, signedIn), func(t *testing.T) {
				m, a := nativeViewFixture()
				m.Language, m.SignedIn = locale, signedIn
				m.Recovery = recoveryNotice{Kind: recoveryChallenge, Scope: recoveryAccountConnect}
				m.RecoveryOpen = true
				reconnected, duplicateSignIn := 0, 0
				a.RetryRecovery = func() { reconnected++ }
				a.RecoveryAccount = func() { duplicateSignIn++ }
				tt := ui.NewTester(func(c *ui.Context) { nativeRecoveryDialog(c, m, a, palette(false), 420, 580) }, 420, 580)
				if !tt.HasText(a.Tr("nativeRecoveryConnectionHint")) || tt.HasText(a.Tr("nativeRecoveryConnectedHint")) {
					t.Fatal("failed new connection must not imply that the requested account connected")
				}
				if _, ok := tt.Find(a.Tr("shellSignIn")); ok {
					t.Fatal("connection recovery must not show a duplicate sign-in action")
				}
				nativeAssertVisible(t, tt, a.Tr("nativeRecoveryReconnect"), 420, 580)
				if err := tt.Click(a.Tr("nativeRecoveryReconnect")); err != nil {
					t.Fatal(err)
				}
				if reconnected != 1 || duplicateSignIn != 0 || m.SignedIn != signedIn {
					t.Fatal("reconnect must reach the scoped retry without changing the existing account")
				}
			})
		}
	}
}
