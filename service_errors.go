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
	if state.Error == "audio_access_denied" {
		if status := state.ErrorDetails.Sanitized().HTTPStatus; status == 401 || status == 403 {
			return "nativePlaybackAccessDeniedHTTP"
		}
		return "nativePlaybackAccessDenied"
	}
	if state.Error == "region_unavailable" {
		return "nativeMusicRegionUnavailable"
	}
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
	a.model.setPlaybackFailure(state, a.trArgs)
}

func (m *UIModel) setPlaybackFailure(state player.State, tr func(string, map[string]any) string) {
	key := playbackServiceErrorKey(state)
	if key == "" {
		key = "nativePlaybackError"
	}
	if key == "nativePlaybackAccessDeniedHTTP" {
		m.playbackMessage.set(&m.PlaybackError, tr, key, map[string]any{"status": state.ErrorDetails.Sanitized().HTTPStatus})
		return
	}
	m.playbackMessage.set(&m.PlaybackError, tr, key, nil)
}

func catalogErrorKey(err error) string {
	if errors.Is(err, catalog.ErrExpiredContinuation) {
		return "nativeCatalogPageExpired"
	}
	if key := catalogServiceErrorKey(err); key != "" {
		return key
	}
	return "nativeCatalogError"
}

func (a *desktop) catalogFailure(err error) {
	a.model.ErrorScope = errorCatalog
	a.setError(catalogErrorKey(err))
}

func (m *UIModel) setContinuationFailure(err error, tr func(string, map[string]any) string) {
	// Refreshing the retained account can invalidate an in-flight cursor.
	if errors.Is(err, catalog.ErrSessionChanged) {
		err = catalog.ErrExpiredContinuation
	}
	m.ErrorScope = errorCatalog
	key := catalogErrorKey(err)
	if key == "nativeCatalogError" {
		key = "nativeCatalogMoreError"
	}
	m.errorMessage.set(&m.Error, tr, key, nil)
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
