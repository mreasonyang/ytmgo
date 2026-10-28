package main

import (
	"errors"

	"github.com/egoist/mygo"

	"ytmgo/internal/catalog"
	"ytmgo/internal/player"
)

type recoveryKind string

const (
	recoveryChallenge recoveryKind = "challenge"
	recoveryConsent   recoveryKind = "consent"
	recoverySignIn    recoveryKind = "signin"
	recoveryRedirect  recoveryKind = "redirect"
)

type recoveryScope string

const (
	recoveryCatalog        recoveryScope = "catalog"
	recoveryPlayback       recoveryScope = "playback"
	recoveryAccount        recoveryScope = "account"
	recoveryAccountConnect recoveryScope = "account_connect"
)

// A notice carries only a fixed classification and the operation to retry.
// Upstream redirect URLs and response contents never enter UI state.
type recoveryNotice struct {
	Kind  recoveryKind
	Scope recoveryScope
}

func (n recoveryNotice) valid() bool {
	if n.Kind != recoveryChallenge && n.Kind != recoveryConsent && n.Kind != recoverySignIn && n.Kind != recoveryRedirect {
		return false
	}
	return n.Scope == recoveryCatalog || n.Scope == recoveryPlayback || n.Scope == recoveryAccount || n.Scope == recoveryAccountConnect
}

func recoveryForRedirect(redirect string) recoveryKind {
	switch redirect {
	case "google_challenge":
		return recoveryChallenge
	case "consent":
		return recoveryConsent
	case "signin":
		return recoverySignIn
	case "", "other", "same_origin":
		return recoveryRedirect
	default:
		return ""
	}
}

func catalogRecovery(err error, scope recoveryScope) recoveryNotice {
	var failure *catalog.HTTPError
	if errors.As(err, &failure) && failure.StatusCode >= 300 && failure.StatusCode < 400 {
		n := recoveryNotice{Kind: recoveryForRedirect(failure.Redirect), Scope: scope}
		if n.valid() {
			return n
		}
	}
	return recoveryNotice{Scope: scope}
}

func accountRequestRecovery(err error, newIdentity bool) recoveryNotice {
	scope := recoveryAccount
	if newIdentity {
		scope = recoveryAccountConnect
	}
	return catalogRecovery(err, scope)
}

func playbackRecovery(state player.State) recoveryNotice {
	d := state.ErrorDetails.Sanitized()
	if state.Error == "bootstrap_redirect" && d.Stage == "bootstrap" && d.HTTPStatus >= 300 && d.HTTPStatus < 400 {
		n := recoveryNotice{Kind: recoveryForRedirect(d.Redirect), Scope: recoveryPlayback}
		if n.valid() {
			return n
		}
	}
	if state.Error == "login_required" {
		return recoveryNotice{Kind: recoverySignIn, Scope: recoveryPlayback}
	}
	return recoveryNotice{}
}

func recoveryMessageKey(kind recoveryKind) string {
	switch kind {
	case recoveryChallenge:
		return "nativeVerificationRequired"
	case recoveryConsent:
		return "nativeConsentRequired"
	case recoverySignIn:
		return "nativeServiceSignInRequired"
	case recoveryRedirect:
		return "nativeServiceRedirect"
	default:
		return ""
	}
}

func (a *desktop) playbackFailure(state player.State) {
	a.model.PlaybackRecovery = playbackRecovery(state)
	key := recoveryMessageKey(a.model.PlaybackRecovery.Kind)
	if key == "" {
		key = "nativePlaybackError"
	}
	a.model.PlaybackError = a.tr(key)
}

func (a *desktop) catalogFailure(err error) {
	a.model.ErrorRecovery = catalogRecovery(err, recoveryCatalog)
	key := recoveryMessageKey(a.model.ErrorRecovery.Kind)
	if key == "" {
		key = "nativeCatalogError"
	}
	a.model.Error = a.tr(key)
}

func (a *desktop) showRecovery(notice recoveryNotice) {
	if !a.activeRecovery(notice) {
		return
	}
	a.model.Recovery = notice
	a.model.RecoveryOpen = true
	a.model.RecoveryBrowserFailed = false
	a.show()
	a.invalidate()
}

// Opening the default browser does not resolve the failed native request.
// Its result is determined by an explicit retry.
func (a *desktop) openRecoveryBrowser() {
	if !a.model.RecoveryOpen || !a.activeRecovery(a.model.Recovery) {
		return
	}
	notice := a.model.Recovery
	go func() {
		err := mygo.Shell.OpenExternal(musicHomeURL)
		a.post(func() {
			a.recoveryBrowserResult(notice, err != nil)
		})
	}()
}

func (a *desktop) retryRecovery() {
	notice := a.model.Recovery
	active := a.activeRecovery(notice)
	a.model.RecoveryOpen = false
	a.model.RecoveryBrowserFailed = false
	if !active {
		return
	}
	switch notice.Scope {
	case recoveryPlayback:
		if a.model.Current.ID != "" {
			a.startTrack(a.model.Current)
		}
	case recoveryCatalog:
		a.reload()
	case recoveryAccount:
		if a.accountState.session != nil {
			a.refreshAccount()
		} else {
			a.retryAccountConnection()
		}
	case recoveryAccountConnect:
		// A failed transfer is a different identity from any retained account.
		// Ask for a fresh explicit transfer; never verify the previous account
		// and present that as recovery of the requested new connection.
		a.retryAccountConnection()
	}
}

func (a *desktop) retryAccountConnection() {
	a.signIn()
	if a.accountState.bridge == nil {
		a.beginAccountBridge()
	}
}

// Account polling/focus refresh may finish while a catalog error is being
// handled. Its success cannot resolve a failed catalog request or a transfer
// of a different account.
func (a *desktop) clearAccountRequestFailure(newIdentity bool) {
	scope := accountRequestRecovery(nil, newIdentity).Scope
	if a.model.ErrorRecovery.Scope != scope {
		return
	}
	a.model.Error = ""
	a.model.ErrorRecovery = recoveryNotice{}
	if a.model.Recovery.Scope == scope {
		a.model.RecoveryOpen = false
	}
}

func (a *desktop) recoverySignInAction() {
	active := a.activeRecovery(a.model.Recovery)
	a.model.RecoveryOpen = false
	if active && !a.model.SignedIn {
		a.signIn()
	}
}

func (a *desktop) activeRecovery(notice recoveryNotice) bool {
	if !notice.valid() {
		return false
	}
	if notice.Scope == recoveryPlayback {
		return a.model.PlaybackError != "" && a.model.PlaybackRecovery == notice
	}
	return a.model.Error != "" && a.model.ErrorRecovery == notice
}

func (a *desktop) recoveryBrowserResult(notice recoveryNotice, failed bool) {
	if a.model.RecoveryOpen && a.model.Recovery == notice && a.activeRecovery(notice) {
		a.model.RecoveryBrowserFailed = failed
	}
}
