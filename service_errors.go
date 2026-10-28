package main

import (
	"errors"

	"ytmgo/internal/catalog"
	"ytmgo/internal/player"
)

// Error ownership prevents a successful account refresh from clearing an
// unrelated catalog failure or failed transfer of a different account.
type errorScope string

const (
	errorCatalog        errorScope = "catalog"
	errorAccount        errorScope = "account"
	errorAccountConnect errorScope = "account_connect"
)

func serviceRedirectErrorKey(redirect string) string {
	switch redirect {
	case "google_challenge":
		return "nativeVerificationRequired"
	case "consent":
		return "nativeConsentRequired"
	case "signin":
		return "nativeServiceSignInRequired"
	case "", "other", "same_origin":
		return "nativeServiceRedirect"
	default:
		return ""
	}
}

func catalogServiceErrorKey(err error) string {
	var failure *catalog.HTTPError
	if errors.As(err, &failure) && failure.StatusCode >= 300 && failure.StatusCode < 400 {
		return serviceRedirectErrorKey(failure.Redirect)
	}
	return ""
}

func playbackServiceErrorKey(state player.State) string {
	d := state.ErrorDetails.Sanitized()
	if state.Error == "bootstrap_redirect" && d.Stage == "bootstrap" && d.HTTPStatus >= 300 && d.HTTPStatus < 400 {
		return serviceRedirectErrorKey(d.Redirect)
	}
	if state.Error == "login_required" {
		return "nativeServiceSignInRequired"
	}
	return ""
}

func (a *desktop) playbackFailure(state player.State) {
	key := playbackServiceErrorKey(state)
	if key == "" {
		key = "nativePlaybackError"
	}
	a.model.PlaybackError = a.tr(key)
}

func (a *desktop) catalogFailure(err error) {
	a.model.ErrorScope = errorCatalog
	key := catalogServiceErrorKey(err)
	if key == "" {
		key = "nativeCatalogError"
	}
	a.model.Error = a.tr(key)
}

func accountErrorScope(newIdentity bool) errorScope {
	if newIdentity {
		return errorAccountConnect
	}
	return errorAccount
}

func (a *desktop) clearAccountRequestFailure(newIdentity bool) {
	if a.model.ErrorScope != accountErrorScope(newIdentity) {
		return
	}
	a.model.Error = ""
	a.model.ErrorScope = ""
}
