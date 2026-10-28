package main

import (
	"errors"
	"fmt"
	"testing"

	"ytmgo/internal/accountbridge"
	"ytmgo/internal/catalog"
	"ytmgo/internal/player"
)

func TestRecoveryClassificationRequiresMatchingServiceEvidence(t *testing.T) {
	for _, tc := range []struct {
		status   int
		redirect string
		want     recoveryKind
	}{
		{302, "google_challenge", recoveryChallenge}, {307, "signin", recoverySignIn},
		{303, "consent", recoveryConsent}, {302, "other", recoveryRedirect},
		{302, "same_origin", recoveryRedirect}, {302, "", recoveryRedirect},
		{403, "google_challenge", ""}, {429, "", ""}, {500, "", ""},
		{200, "google_challenge", ""}, {302, "synthetic-private-value", ""},
	} {
		t.Run(fmt.Sprintf("%d-%s", tc.status, tc.redirect), func(t *testing.T) {
			err := fmt.Errorf("wrapped: %w", &catalog.HTTPError{Stage: "browse", StatusCode: tc.status, Redirect: tc.redirect})
			got := catalogRecovery(err, recoveryCatalog)
			if got.Kind != tc.want || got.valid() != (tc.want != "") {
				t.Fatal("catalog recovery misclassified service evidence")
			}
			state := player.State{Error: "bootstrap_redirect", ErrorDetails: player.FailureDetails{Stage: "bootstrap", HTTPStatus: tc.status, Redirect: tc.redirect}}
			got = playbackRecovery(state)
			if got.Kind != tc.want || got.valid() != (tc.want != "") {
				t.Fatal("playback recovery misclassified service evidence")
			}
		})
	}
	for _, state := range []player.State{
		{Error: "init_failed", ErrorDetails: player.FailureDetails{Stage: "bootstrap", HTTPStatus: 302, Redirect: "google_challenge"}},
		{Error: "bootstrap_redirect", ErrorDetails: player.FailureDetails{Stage: "player_request", HTTPStatus: 302, Redirect: "google_challenge"}},
		{Error: "audio_access_denied"}, {Error: "audio_rate_limited"},
	} {
		if playbackRecovery(state).valid() {
			t.Fatal("ordinary or mismatched error became verification")
		}
	}
	if got := playbackRecovery(player.State{Error: "login_required"}); got.Kind != recoverySignIn {
		t.Fatal("explicit service login requirement was discarded")
	}
	if catalogRecovery(errors.New("synthetic arbitrary upstream error"), recoveryCatalog).valid() {
		t.Fatal("raw error became verification")
	}
}

func TestRecoveryKeepsConnectedAccountAndOnlyRetriesActiveFailure(t *testing.T) {
	session := syntheticAccountSession()
	a := &desktop{model: UIModel{SignedIn: true, MusicPremium: true}}
	a.accountState.session = &session
	a.accountState.configured = player.AccountSession{LoggedIn: true, MusicPremium: true, Revision: 7}
	err := &catalog.HTTPError{Stage: "browse", StatusCode: 302, Redirect: "google_challenge"}
	a.model.Error, a.model.ErrorRecovery = "verification required", accountRequestRecovery(err, false)
	notice := a.model.ErrorRecovery
	if notice != (recoveryNotice{Kind: recoveryChallenge, Scope: recoveryAccount}) ||
		!a.model.SignedIn || !a.model.MusicPremium || a.accountState.session != &session || a.accountState.configured.Revision != 7 {
		t.Fatal("a verification rejection altered the connected account")
	}
	a.showRecovery(notice)
	message := a.model.Error
	a.recoveryBrowserResult(notice, false)
	if !a.model.RecoveryOpen || a.model.Error != message || !a.activeRecovery(notice) {
		t.Fatal("opening Brave incorrectly resolved the failed request")
	}
	a.recoveryBrowserResult(notice, true)
	if !a.model.RecoveryBrowserFailed || a.model.Error != message {
		t.Fatal("browser failure hid the service error")
	}
	a.recoverySignInAction()
	if a.model.RecoveryOpen || a.account != nil || a.accountState.session != &session {
		t.Fatal("connected account was sent through sign-in again")
	}
	a.model.Error, a.model.ErrorRecovery = "generic account failure", accountRequestRecovery(errors.New("generic"), false)
	if a.activeRecovery(notice) || a.model.ErrorRecovery.valid() {
		t.Fatal("old challenge survived an unrelated error")
	}
	// A stale dialog must not restart a request or open the account window.
	a.model.Recovery, a.model.RecoveryOpen = notice, true
	a.retryRecovery()
	if a.model.RecoveryOpen || a.accountBusy || a.account != nil {
		t.Fatal("stale dialog retried a request")
	}
}

func TestRecoveryBrowserCompletionCannotReplaceNewFailure(t *testing.T) {
	old := recoveryNotice{Kind: recoveryChallenge, Scope: recoveryPlayback}
	current := recoveryNotice{Kind: recoveryConsent, Scope: recoveryCatalog}
	a := &desktop{model: UIModel{Error: "consent", ErrorRecovery: current, Recovery: current, RecoveryOpen: true}}
	a.recoveryBrowserResult(old, true)
	if a.model.RecoveryBrowserFailed || a.model.Error != "consent" || a.model.Recovery != current {
		t.Fatal("stale browser completion changed the current recovery")
	}
	a.model.Error, a.model.ErrorRecovery = "", recoveryNotice{}
	a.recoveryBrowserResult(current, true)
	if a.model.RecoveryBrowserFailed {
		t.Fatal("browser completion reintroduced a resolved failure")
	}
}

func TestAccountVerificationMessagesDoNotClaimSessionExpiry(t *testing.T) {
	for redirect, want := range map[string]string{
		"google_challenge": "nativeVerificationRequired", "consent": "nativeConsentRequired",
		"signin": "nativeServiceSignInRequired", "other": "nativeServiceRedirect", "": "nativeServiceRedirect",
	} {
		if got := accountErrorMessageKey(&catalog.HTTPError{Stage: "browse", StatusCode: 302, Redirect: redirect}); got != want {
			t.Fatal("account verification response used the wrong guidance")
		}
	}
	if got := accountErrorMessageKey(accountbridge.ErrExpired); got != "nativeAccountSessionError" {
		t.Fatal("confirmed expired credentials lost reconnect guidance")
	}
}

func TestAccountRefreshCannotResolveCatalogOrNewAccountTransfer(t *testing.T) {
	err := &catalog.HTTPError{Stage: "browse", StatusCode: 302, Redirect: "google_challenge"}
	session := syntheticAccountSession()
	for _, notice := range []recoveryNotice{
		catalogRecovery(err, recoveryCatalog), accountRequestRecovery(err, true),
	} {
		a := &desktop{model: UIModel{SignedIn: true, MusicPremium: true, Error: "request blocked", ErrorRecovery: notice, Recovery: notice, RecoveryOpen: true}}
		a.accountState.session = &session
		a.clearAccountRequestFailure(false)
		if a.model.Error != "request blocked" || a.model.ErrorRecovery != notice || !a.activeRecovery(notice) || !a.model.RecoveryOpen {
			t.Fatal("refresh of the retained account cleared another operation's failure")
		}
		if notice.Scope == recoveryAccountConnect {
			// Even with an old session present, retry must use the new-account
			// connection action, not refresh that old session.
			if accountRequestRecovery(err, false).Scope == notice.Scope {
				t.Fatal("new connection and old account refresh share a retry action")
			}
			a.clearAccountRequestFailure(true)
			if a.model.Error != "" || a.model.RecoveryOpen || a.activeRecovery(notice) {
				t.Fatal("successful explicit transfer did not clear its own failure")
			}
		}
	}
}
