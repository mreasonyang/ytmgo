package main

import (
	"errors"
	"fmt"
	"testing"

	"ytmgo/internal/accountbridge"
	"ytmgo/internal/catalog"
	"ytmgo/internal/player"
)

func TestServiceErrorClassificationRequiresMatchingEvidence(t *testing.T) {
	for _, tc := range []struct {
		status         int
		redirect, want string
	}{
		{302, "google_challenge", "nativeVerificationRequired"},
		{307, "signin", "nativeServiceSignInRequired"},
		{303, "consent", "nativeConsentRequired"},
		{302, "other", "nativeServiceRedirect"},
		{302, "same_origin", "nativeServiceRedirect"},
		{302, "", "nativeServiceRedirect"},
		{403, "google_challenge", ""}, {429, "", ""}, {500, "", ""},
		{200, "google_challenge", ""}, {302, "synthetic-private-value", ""},
	} {
		t.Run(fmt.Sprintf("%d-%s", tc.status, tc.redirect), func(t *testing.T) {
			err := fmt.Errorf("wrapped: %w", &catalog.HTTPError{Stage: "browse", StatusCode: tc.status, Redirect: tc.redirect})
			if catalogServiceErrorKey(err) != tc.want {
				t.Fatal("catalog error misclassified service evidence")
			}
			state := player.State{Error: "bootstrap_redirect", ErrorDetails: player.FailureDetails{Stage: "bootstrap", HTTPStatus: tc.status, Redirect: tc.redirect}}
			if playbackServiceErrorKey(state) != tc.want {
				t.Fatal("playback error misclassified service evidence")
			}
		})
	}
	for _, state := range []player.State{
		{Error: "init_failed", ErrorDetails: player.FailureDetails{Stage: "bootstrap", HTTPStatus: 302, Redirect: "google_challenge"}},
		{Error: "bootstrap_redirect", ErrorDetails: player.FailureDetails{Stage: "player_request", HTTPStatus: 302, Redirect: "google_challenge"}},
		{Error: "audio_access_denied"}, {Error: "audio_rate_limited"},
	} {
		if playbackServiceErrorKey(state) != "" {
			t.Fatal("ordinary or mismatched error became verification")
		}
	}
	if playbackServiceErrorKey(player.State{Error: "login_required"}) != "nativeServiceSignInRequired" {
		t.Fatal("explicit service login requirement was discarded")
	}
	if catalogServiceErrorKey(errors.New("synthetic arbitrary upstream error")) != "" {
		t.Fatal("raw error became verification")
	}
}

func TestAccountVerificationMessagesDoNotClaimSessionExpiry(t *testing.T) {
	for redirect, want := range map[string]string{
		"google_challenge": "nativeVerificationRequired", "consent": "nativeConsentRequired",
		"signin": "nativeServiceSignInRequired", "other": "nativeServiceRedirect", "": "nativeServiceRedirect",
	} {
		if accountErrorMessageKey(&catalog.HTTPError{Stage: "browse", StatusCode: 302, Redirect: redirect}) != want {
			t.Fatal("account verification response used the wrong explanation")
		}
	}
	if accountErrorMessageKey(accountbridge.ErrExpired) != "nativeAccountSessionError" {
		t.Fatal("confirmed expired credentials lost reconnect guidance")
	}
}

func TestAccountRefreshCannotClearAnotherRequestsError(t *testing.T) {
	session := syntheticAccountSession()
	for _, scope := range []errorScope{errorCatalog, errorAccountConnect} {
		a := &desktop{model: UIModel{SignedIn: true, MusicPremium: true, Error: "request blocked", ErrorScope: scope}}
		a.accountState.session = &session
		a.clearAccountRequestFailure(false)
		if a.model.Error != "request blocked" || a.model.ErrorScope != scope {
			t.Fatal("refresh of retained account cleared another operation's failure")
		}
		if !a.model.SignedIn || !a.model.MusicPremium || a.accountState.session != &session {
			t.Fatal("service error changed the connected account")
		}
		if scope == errorAccountConnect {
			a.clearAccountRequestFailure(true)
			if a.model.Error != "" || a.model.ErrorScope != "" {
				t.Fatal("successful explicit transfer did not clear its own failure")
			}
		}
	}
}
