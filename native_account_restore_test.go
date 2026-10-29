package main

import (
	"fmt"
	"testing"

	"github.com/egoist/mygo/ui"
	"ytmgo/locales"
)

func TestNativeAccountRestoreStatesAreExplained(t *testing.T) {
	for _, language := range locales.Catalog.Languages() {
		for _, state := range []string{accountRestoring, accountRestoreSlow, accountRestoreDeferred} {
			for _, width := range []int{1440, 420} {
				t.Run(fmt.Sprintf("%s-%s-%d", language, state, width), func(t *testing.T) {
					m, a := nativeViewFixture()
					m.Language, m.AccountRestore, m.Route = language, state, "library"
					m.Page.Title, m.Page.Items, m.Page.Shelves = a.Tr("shellLibraryTitle"), nil, nil
					skipped, retried := 0, 0
					a.SkipRestore = func() { skipped++ }
					a.RestoreAccount = func() { retried++ }
					height := 900
					if width == 420 {
						height = 740
					}
					tt := ui.NewTester(func(c *ui.Context) { nativeView(c, m, a) }, width, height)
					if state == accountRestoreDeferred {
						if !tt.HasText(a.Tr("nativeAccountRestoreDeferred")) || !tt.HasText(a.Tr("nativeAccountRestoreDeferredBody")) {
							t.Fatalf("deferred restore is not explained: %v", tt.Texts())
						}
						nativeAssertVisible(t, tt, a.Tr("shellSignIn"), width, height)
						if err := tt.Click(a.Tr("nativeAccountRestoreRetry")); err != nil || retried != 1 {
							t.Fatalf("deferred restore has no retry: %v", err)
						}
					} else {
						if !tt.HasText(a.Tr("nativeAccountRestoring")) || !tt.HasText(a.Tr("nativeAccountRestoringBody")) {
							t.Fatalf("pending restore is shown as signed out: %v", tt.Texts())
						}
						if tt.HasText(a.Tr("shellSignIn")) || tt.HasText(a.Tr("shellLibraryEmptyBody")) {
							t.Fatal("pending restore still offers sign-in")
						}
						if _, exists := tt.Find(a.Tr("nativeAccountRestoreSkip")); exists != (state == accountRestoreSlow) {
							t.Fatal("continue-without-account must appear only once restoring is slow")
						}
						if state == accountRestoreSlow {
							if !tt.HasText(a.Tr("nativeAccountRestoreSlow")) {
								t.Fatal("slow restore is not explained")
							}
							if err := tt.Click(a.Tr("nativeAccountRestoreSkip")); err != nil || skipped != 1 {
								t.Fatalf("slow restore cannot be skipped: %v", err)
							}
						}
					}
					m.Settings = true
					tt.Frame()
					if !tt.HasText(nativeAccountStatus(m, a)) || nativeAccountStatus(m, a) == a.Tr("shellAccountSignedOut") {
						t.Fatal("settings reports a restoring account as signed out")
					}
					if width == 1440 && language == "en" || width == 420 && language == "ja" {
						nativeSnapshot(t, tt, fmt.Sprintf("account-restore-%s-%s-%d", state, language, width))
					}
				})
			}
		}
	}
}

func TestNativeTrackWaitingForAccountExplainsSpinner(t *testing.T) {
	for _, width := range []int{1440, 1100, 420} {
		t.Run(fmt.Sprint(width), func(t *testing.T) {
			m, a := nativeViewFixture()
			m.AccountRestore, m.PlaybackLoading, m.Playing, m.Panel = accountRestoreSlow, true, false, true
			skipped := 0
			a.SkipRestore = func() { skipped++ }
			tt := ui.NewTester(func(c *ui.Context) { nativeView(c, m, a) }, width, 900)
			if !tt.HasText(a.Tr("nativePlaybackAwaitingAccount")) {
				t.Fatalf("pending track has an unexplained spinner: %v", tt.Texts())
			}
			if err := tt.Click(a.Tr("nativeAccountRestoreSkip")); err != nil || skipped != 1 {
				t.Fatalf("pending track has no next step: %v", err)
			}
			m.AccountRestore = ""
			tt.Frame()
			if tt.HasText(a.Tr("nativePlaybackAwaitingAccount")) {
				t.Fatal("settled account still reports a wait")
			}
		})
	}
}

func TestDeferredRestoreIsNotRetriedByPolling(t *testing.T) {
	a := &desktop{model: UIModel{AccountRestore: accountRestoreDeferred}}
	a.accountState.started = true
	// A poll must not reread the store: that can raise the same system
	// authorization prompt the user just dismissed.
	a.refreshAccount()
	if a.accountBusy || a.model.AccountRestore != accountRestoreDeferred || a.accountState.restoreAttempt != 0 {
		t.Fatal("polling restarted a deferred restore")
	}
}
