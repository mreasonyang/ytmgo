package main

import (
	"context"
	"errors"
	"github.com/egoist/mygo"
	"net/http"
	"sync"
	"time"
	"ytmgo/internal/accountlogin"
	"ytmgo/internal/accountsession"
	"ytmgo/internal/catalog"
	"ytmgo/internal/language"
	"ytmgo/internal/player"
	"ytmgo/locales"
)

type accountFlow struct {
	ctx        context.Context
	cancel     context.CancelFunc
	generation uint64
}

func (f *accountFlow) begin(parent context.Context, generation uint64) context.Context {
	f.cancelPending()
	f.ctx, f.cancel = context.WithTimeout(parent, 90*time.Second)
	f.generation = generation
	return f.ctx
}
func (f *accountFlow) cancelPending() {
	if f.cancel != nil {
		f.cancel()
	}
	f.ctx, f.cancel, f.generation = nil, nil, 0
}
func (f *accountFlow) finish(ctx context.Context, generation, current uint64, validationErr error, install func() error) (bool, error) {
	if f.ctx != ctx || f.generation != generation {
		return false, nil
	}
	if generation != current {
		f.cancelPending()
		return false, nil
	}
	defer f.cancelPending()
	if err := ctx.Err(); err != nil {
		return true, err
	}
	if validationErr != nil {
		return true, validationErr
	}
	return true, install()
}

type accountState struct {
	work                     sync.Mutex
	loginWait                sync.WaitGroup
	flow                     accountFlow
	store                    accountsession.Store
	session                  *accountsession.Session
	configured               player.AccountSession
	revision                 uint64
	restored, ready, started bool
	pollCancel               context.CancelFunc
	login                    *accountlogin.Login
	loginCancel              context.CancelFunc
	// fallback marks revision as configured (or being configured) for
	// anonymous playback while a saved account is not active.
	fallback       bool
	restoreAttempt uint64
}
type nativeAccountSecrets struct{}

func (nativeAccountSecrets) Set(key string, value []byte) error {
	return setNativeAccountSecret(key, value)
}
func (nativeAccountSecrets) Get(key string) ([]byte, error) {
	value, err := readNativeAccountSecret(key)
	if errors.Is(err, mygo.ErrSecretNotFound) {
		return nil, accountsession.ErrNotFound
	}
	return value, err
}
func (nativeAccountSecrets) Delete(key string) error {
	err := deleteNativeAccountSecret(key)
	if errors.Is(err, mygo.ErrSecretNotFound) {
		return accountsession.ErrNotFound
	}
	return err
}

type accountSessionClient interface {
	SetSession([]*http.Cookie, int, string) error
	AccountStatus(context.Context) (catalog.AccountStatus, error)
	Credentials() (catalog.SessionCredentials, bool)
	ClearSession()
}

func accountErrorMessageKey(err error) string {
	// An HTTP rejection can reflect a service or network restriction. It does
	// not establish that the transferred account session needs replacing.
	var httpError *catalog.HTTPError
	if errors.As(err, &httpError) {
		if key := catalogServiceErrorKey(err); key != "" {
			return key
		}
		return "nativeAccountError"
	}
	if errors.Is(err, accountsession.ErrSession) || errors.Is(err, accountsession.ErrExpired) ||
		errors.Is(err, catalog.ErrInvalidSession) || errors.Is(err, catalog.ErrSessionExpired) ||
		errors.Is(err, catalog.ErrAuthenticationRequired) {
		return "nativeAccountSessionError"
	}
	return "nativeAccountError"
}

func verifyAccountSession(ctx context.Context, candidate accountSessionClient, session accountsession.Session) (catalog.AccountStatus, catalog.SessionCredentials, error) {
	defer candidate.ClearSession()
	if err := ctx.Err(); err != nil {
		return catalog.AccountStatus{}, catalog.SessionCredentials{}, err
	}
	cookies, err := session.HTTPCookies(time.Now())
	if errors.Is(err, accountsession.ErrExpired) {
		// A known local expiration is explicit loss of authentication. Network
		// errors below remain errors and cannot downgrade membership.
		return catalog.AccountStatus{}, catalog.SessionCredentials{}, nil
	}
	if err != nil {
		return catalog.AccountStatus{}, catalog.SessionCredentials{}, err
	}
	if err = candidate.SetSession(cookies, *session.AccountIndex, session.OnBehalfOfUser); err != nil {
		return catalog.AccountStatus{}, catalog.SessionCredentials{}, err
	}
	status, err := candidate.AccountStatus(ctx)
	if ctx.Err() != nil {
		return catalog.AccountStatus{}, catalog.SessionCredentials{}, ctx.Err()
	}
	if err != nil {
		return catalog.AccountStatus{}, catalog.SessionCredentials{}, err
	}
	credentials, ok := candidate.Credentials()
	if status.LoggedIn && !ok {
		return catalog.AccountStatus{}, catalog.SessionCredentials{}, accountsession.ErrSession
	}
	return status, credentials, nil
}

type accountSessionStore interface {
	Load() (accountsession.Session, error)
	Delete() error
}

func restoreAccountSession(store accountSessionStore) (accountsession.Session, error) {
	session, err := store.Load()
	if !errors.Is(err, accountsession.ErrExpired) {
		return session, err
	}
	// Delete this app's encrypted session and its key. A failed deletion remains
	// a storage failure, never an apparently successful anonymous restore.
	if store.Delete() != nil {
		return accountsession.Session{}, accountsession.ErrStorage
	}
	return accountsession.Session{}, accountsession.ErrNotFound
}

// Saved membership is verified before configuring any playback engine.
func (a *desktop) startAccountPolling() {
	s := &a.accountState
	if s.started {
		return
	}
	s.started = true
	s.revision = 1
	ctx, cancel := context.WithCancel(a.ctx)
	s.pollCancel = cancel
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				a.post(a.refreshAccount)
			}
		}
	}()
	a.refreshAccount()
}
func (a *desktop) refreshAccount() {
	s := &a.accountState
	if !s.started || a.accountBusy || s.login != nil {
		return
	}
	if !s.restored {
		// A deferred store read waits for an explicit retry: rereading can
		// raise the same system authorization prompt on every poll.
		if a.model.AccountRestore != accountRestoreDeferred {
			a.restoreAccount()
		}
		return
	}
	if s.session == nil && s.ready {
		return
	}
	a.updateAccount(s.session, false)
}
func (a *desktop) restoreAccount() {
	s := &a.accountState
	if a.accountBusy {
		return
	}
	a.accountBusy = true
	a.beginAccountRestore()
	generation := a.accountGeneration
	ctx := s.flow.begin(a.ctx, generation)
	go func() {
		s.work.Lock()
		session, err := restoreAccountSession(s.store)
		s.work.Unlock()
		a.post(func() {
			accepted, flowErr := s.flow.finish(ctx, generation, a.accountGeneration, nil, func() error { return nil })
			if !accepted {
				return
			}
			a.accountBusy = false
			if flowErr != nil || (err != nil && !errors.Is(err, accountsession.ErrNotFound)) {
				a.deferAccountRestore()
				a.accountMessage("nativeAccountRestoreFailed")
				return
			}
			s.restored = true
			if err == nil {
				s.session = &session
			} else {
				a.model.AccountRestore = ""
			}
			a.updateAccount(s.session, false)
		})
	}()
}

// Serial worker plus UI generation gate prevents old account replies from
// replacing a newer session. Network failures preserve the verified account.
func (a *desktop) updateAccount(session *accountsession.Session, newIdentity bool) {
	s := &a.accountState
	if a.accountBusy {
		return
	}
	a.accountBusy = true
	failureCategory := "refresh_failed"
	if newIdentity {
		a.accountGeneration++
		failureCategory = "session_failed"
	}
	generation := a.accountGeneration
	revision := s.revision
	if newIdentity || (s.fallback && session != nil) {
		revision++
	}
	ctx := s.flow.begin(a.ctx, generation)
	locale := language.ServiceLanguage(mygo.App.Locale())
	previousCredentials, hadCredentials := a.catalog.Credentials()
	a.invalidate()
	go func() {
		s.work.Lock()
		defer s.work.Unlock()
		status := catalog.AccountStatus{}
		credentials := catalog.SessionCredentials{}
		var err error
		if session != nil {
			checkCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
			status, credentials, err = verifyAccountSession(checkCtx, catalog.NewWithOptions(catalog.Options{Language: locale}), *session)
			cancel()
		}
		if err == nil && newIdentity && !status.LoggedIn {
			err = accountsession.ErrSession
		}
		errorKey := accountErrorMessageKey(err)
		if err == nil && newIdentity {
			err = s.store.Replace(ctx, *session)
			if err != nil {
				errorKey = "nativeAccountStorageError"
			}
		}
		verified := err == nil
		var storageErr error
		if err == nil && session != nil && !status.LoggedIn {
			revision++
			session = nil
			credentials = catalog.SessionCredentials{}
			storageErr = s.store.Delete()
			if storageErr != nil {
				errorKey = "nativeAccountStorageError"
			}
		}
		var replay accountReplay
		next := player.AccountSession{LoggedIn: status.LoggedIn, MusicPremium: status.MusicPremium, Cookie: credentials.Cookie, AccountIndex: credentials.AccountIndex, OnBehalfOfUser: credentials.OnBehalfOfUser, Revision: revision}
		if err == nil {
			replay, err = a.beforeAccountConfigure(ctx, generation, next, newIdentity)
			if err == nil {
				err = a.player.SetAccount(ctx, next)
			}
		}
		configured := err == nil
		if err == nil && storageErr != nil {
			err = storageErr
		}
		a.post(func() {
			accepted, flowErr := s.flow.finish(ctx, generation, a.accountGeneration, nil, func() error { return nil })
			if !accepted {
				return
			}
			a.accountBusy = false
			restoring := a.model.AccountRestore == accountRestoring || a.model.AccountRestore == accountRestoreSlow
			if flowErr != nil {
				if restoring {
					a.deferAccountRestore()
				}
				a.accountFailure("nativeAccountError", failureCategory)
				return
			}
			identityChanged := false
			if verified {
				s.session, s.revision, s.restored, s.ready = session, revision, true, configured
				if configured {
					s.configured = next
					s.fallback = false
				}
				a.model.AccountRestore = ""
				if session != nil && status.LoggedIn && (newIdentity || !hadCredentials || credentials != previousCredentials) {
					cookies, cookieErr := session.HTTPCookies(time.Now())
					if cookieErr == nil {
						cookieErr = a.catalog.SetSession(cookies, *session.AccountIndex, session.OnBehalfOfUser)
					}
					if cookieErr != nil {
						a.accountMessage(accountErrorMessageKey(cookieErr))
						return
					}
				} else if !status.LoggedIn {
					a.catalog.ClearSession()
				}
				identityChanged = a.model.SignedIn != status.LoggedIn || newIdentity
				a.model.SignedIn, a.model.MusicPremium = status.LoggedIn, status.MusicPremium
				mygo.App.SetMenu(a.menu())
				if identityChanged {
					if !newIdentity {
						a.accountGeneration++
					}
					a.clearAccountContent()
				}
			}
			if err != nil {
				if restoring && !verified {
					// The saved session stays for the next refresh or an explicit
					// retry; public tracks must not wait for it meanwhile.
					a.deferAccountRestore()
				}
				a.accountRequestFailure(errorKey, failureCategory, newIdentity)
				return
			}
			category := "signed_out"
			if status.LoggedIn {
				category = "standard"
			}
			if status.MusicPremium {
				category = "premium"
			}
			recordAccountDiagnostic(a.diagnosticPath, category)
			a.clearAccountRequestFailure(newIdentity)
			a.preparedID = ""
			if replay.pending && replay.generation == a.playGeneration {
				a.startTrack(replay.item)
			} else if a.model.Playing {
				a.prepareNext()
			}
			if newIdentity {
				a.navigate("library")
				a.show()
			} else if identityChanged {
				a.navigate("home")
			} else {
				a.invalidate()
			}
		})
	}()
}

// accountRestoreSlowAfter is how long restoration may run before the view
// explains the wait and offers anonymous playback.
var accountRestoreSlowAfter = 12 * time.Second

func (a *desktop) beginAccountRestore() {
	s := &a.accountState
	recordAccountDiagnostic(a.diagnosticPath, "restoring")
	a.model.AccountRestore = accountRestoring
	s.restoreAttempt++
	attempt := s.restoreAttempt
	// The secret store can wait on a system authorization prompt that never
	// reports progress, so the wait itself becomes visible state.
	time.AfterFunc(accountRestoreSlowAfter, func() {
		a.post(func() {
			if s.restoreAttempt == attempt && a.model.AccountRestore == accountRestoring {
				recordAccountDiagnostic(a.diagnosticPath, "restore_slow")
				a.model.AccountRestore = accountRestoreSlow
			}
		})
	})
}

// skipAccountRestore stops waiting for a slow restoration. A late store read
// or verification is ignored; the saved session stays for an explicit retry.
func (a *desktop) skipAccountRestore() {
	s := &a.accountState
	if a.model.AccountRestore != accountRestoreSlow {
		return
	}
	s.flow.cancelPending()
	a.accountBusy = false
	// Supersede any configuration the abandoned flow may still send.
	s.revision++
	recordAccountDiagnostic(a.diagnosticPath, "restore_deferred")
	a.deferAccountRestore()
}

// retryAccountRestore rereads or reverifies the saved session on request.
func (a *desktop) retryAccountRestore() {
	s := &a.accountState
	if a.accountBusy || s.login != nil || a.model.AccountRestore != accountRestoreDeferred {
		return
	}
	a.clearAccountRequestFailure(false)
	if !s.restored {
		a.restoreAccount()
		return
	}
	a.beginAccountRestore()
	a.updateAccount(s.session, false)
}

func (a *desktop) deferAccountRestore() {
	a.model.AccountRestore = accountRestoreDeferred
	a.useAnonymousPlayback()
}

// useAnonymousPlayback lets public tracks play while a saved account is not
// active instead of waiting for it. It never replaces a configured account.
func (a *desktop) useAnonymousPlayback() {
	s := &a.accountState
	if s.ready || s.fallback || a.player == nil {
		return
	}
	s.fallback = true
	generation, next := a.accountGeneration, player.AccountSession{Revision: s.revision}
	ctx, cancel := context.WithTimeout(a.ctx, 90*time.Second)
	go func() {
		defer cancel()
		// Not under s.work: a stalled store read can hold it. The player orders
		// this against later account changes by revision.
		replay, err := a.beforeAccountConfigure(ctx, generation, next, false)
		if err == nil {
			err = a.player.SetAccount(ctx, next)
		}
		a.post(func() {
			if err != nil || !s.fallback || s.ready || s.revision != next.Revision || generation != a.accountGeneration {
				return
			}
			s.ready, s.configured = true, next
			if replay.pending && replay.generation == a.playGeneration {
				a.startTrack(replay.item)
			}
		})
	}()
}

type accountReplay struct {
	item       catalog.Item
	generation uint64
	pending    bool
}

// Coordinate a resolver change with the UI before SetAccount cancels an
// in-progress Load. Already decoded audio survives membership-only changes.
func (a *desktop) beforeAccountConfigure(ctx context.Context, generation uint64, next player.AccountSession, newIdentity bool) (accountReplay, error) {
	result := make(chan accountReplay, 1)
	a.post(func() {
		if ctx.Err() != nil || generation != a.accountGeneration {
			return
		}
		identityChanged := newIdentity || (a.model.SignedIn && !next.LoggedIn) || (a.accountState.fallback && next.LoggedIn)
		changed := accountConfigurationChanged(a.accountState.configured, next, a.accountState.ready)
		replay := accountReplay{}
		if changed {
			if a.prepareCancel != nil {
				a.prepareCancel()
			}
			a.preparedID = ""
			if a.loadPending || identityChanged {
				if a.loadPending && !identityChanged {
					replay.item = a.model.Current
					replay.pending = replay.item.ID != ""
				}
				if a.playCancel != nil {
					a.playCancel()
				}
				if a.seekCancel != nil {
					a.seekCancel()
				}
				a.cancelResume()
				a.playGeneration++
				a.loadPending = false
				a.model.PlaybackLoading = false
				replay.generation = a.playGeneration
			}
			if identityChanged {
				a.clearAccountPlayback()
			}
		}
		result <- replay
	})
	select {
	case replay := <-result:
		return replay, nil
	case <-ctx.Done():
		return accountReplay{}, ctx.Err()
	}
}

func accountConfigurationChanged(previous, next player.AccountSession, ready bool) bool {
	return !ready || previous != next
}
func (a *desktop) signIn() {
	if a.accountBusy {
		return
	}
	s := &a.accountState
	if s.login != nil {
		_ = s.login.Focus()
		return
	}
	a.clearAccountRequestFailure(true)
	a.clearAccountRequestFailure(false)
	ctx, cancel := context.WithCancel(a.ctx)
	login, err := accountlogin.Start(ctx, accountlogin.Options{Title: locales.LoginWindowTitle()})
	if err != nil {
		cancel()
		a.accountRequestFailure(loginErrorMessageKey(err), "failed", true)
		return
	}
	s.login, s.loginCancel = login, cancel
	s.loginWait.Add(1)
	go func() {
		session, err := login.Wait()
		// Quit waits for the child, never for a callback on the stopped UI loop.
		s.loginWait.Done()
		a.post(func() {
			if s.login != login {
				return
			}
			s.login, s.loginCancel = nil, nil
			cancel()
			if errors.Is(err, accountlogin.ErrCancelled) {
				return
			}
			if err != nil {
				a.accountRequestFailure(loginErrorMessageKey(err), "failed", true)
				return
			}
			a.updateAccount(&session, true)
		})
	}()
}

func loginErrorMessageKey(err error) string {
	switch {
	case errors.Is(err, accountlogin.ErrRuntimeMissing):
		return "nativeAccountLoginMissing"
	case errors.Is(err, accountlogin.ErrRuntimeOutdated):
		return "nativeAccountLoginOutdated"
	case errors.Is(err, accountlogin.ErrTimeout):
		return "nativeAccountLoginTimeout"
	default:
		return "nativeAccountLoginFailed"
	}
}

func (a *desktop) stopAccountLogin() {
	s := &a.accountState
	login, cancel := s.login, s.loginCancel
	s.login, s.loginCancel = nil, nil
	if cancel != nil {
		cancel()
	}
	if login != nil {
		login.Cancel()
	}
}

func (a *desktop) closeAccount() {
	a.accountGeneration++
	s := &a.accountState
	s.flow.cancelPending()
	if s.pollCancel != nil {
		s.pollCancel()
	}
	a.stopAccountLogin()
	s.session = nil
	s.configured = player.AccountSession{}
}
func (a *desktop) accountMessage(key string) {
	a.accountFailure(key, "session_failed")
}
func (a *desktop) accountFailure(key, category string) {
	recordAccountDiagnostic(a.diagnosticPath, category)
	a.model.Error = a.tr(key)
	a.model.ErrorScope = errorAccount
	a.invalidate()
}

func (a *desktop) accountRequestFailure(key, category string, newIdentity bool) {
	a.accountFailure(key, category)
	a.model.ErrorScope = accountErrorScope(newIdentity)
}
func (a *desktop) clearAccountContent() {
	if a.likeCancel != nil {
		a.likeCancel()
	}
	if a.pageCancel != nil {
		a.pageCancel()
	}
	a.pageGeneration++
	a.model.LibraryItems = nil
	a.model.Page = catalog.Page{}
	a.model.Loading = false
	a.model.Error = ""
	a.model.ErrorScope = ""
	a.history = nil
	a.historyIndex = -1
}

func (a *desktop) clearAccountPlayback() {
	a.model.Queue = nil
	a.model.Current = catalog.Item{}
	a.queueIndex = -1
	a.queueGeneration++
	a.model.PlaybackError = ""
	a.model.clearLyrics("")
	a.model.Liked = false
	a.model.Position = 0
	a.model.Duration = 0
	a.model.Playing = false
	a.model.Video = false
	a.syncVideo()
	mygo.NowPlaying.Clear()
}
func (a *desktop) signOut() {
	if a.accountBusy || !a.model.SignedIn {
		return
	}
	a.stopAccountLogin()
	a.accountBusy = true
	go func() {
		answer, err := mygo.Dialog.Message(mygo.MessageOptions{Parent: a.window, Title: locales.MenuSignOut(), Message: a.tr("nativeSignOutConfirm"), Buttons: []string{a.tr("shellClose"), a.tr("shellSignOut")}, CancelButton: 0})
		a.post(func() {
			a.accountBusy = false
			if err != nil || answer.Button != 1 {
				return
			}
			a.stopAccountLogin()
			a.accountState.flow.cancelPending()
			a.accountGeneration++
			generation := a.accountGeneration
			s := &a.accountState
			s.revision++
			revision := s.revision
			s.session = nil
			s.restored = true
			s.ready = false
			a.accountBusy = true
			if a.playCancel != nil {
				a.playCancel()
			}
			if a.prepareCancel != nil {
				a.prepareCancel()
			}
			if a.seekCancel != nil {
				a.seekCancel()
			}
			a.cancelResume()
			a.preparedID = ""
			a.playGeneration++
			a.loadPending = false
			a.model.PlaybackLoading = false
			a.catalog.ClearSession()
			a.model.SignedIn = false
			a.model.MusicPremium = false
			a.clearAccountContent()
			a.model.Queue = nil
			a.model.Current = catalog.Item{}
			a.queueIndex = -1
			a.model.PlaybackError = ""
			a.model.clearLyrics("")
			a.model.Liked = false
			a.model.Position = 0
			a.model.Duration = 0
			a.model.Playing = false
			a.model.Video = false
			a.syncVideo()
			mygo.NowPlaying.Clear()
			mygo.App.SetMenu(a.menu())
			ctx := s.flow.begin(a.ctx, generation)
			go func() {
				s.work.Lock()
				storageErr := s.store.Delete()
				configureErr := a.player.SetAccount(ctx, player.AccountSession{Revision: revision})
				s.work.Unlock()
				a.post(func() {
					accepted, flowErr := s.flow.finish(ctx, generation, a.accountGeneration, nil, func() error { return nil })
					if !accepted {
						return
					}
					a.accountBusy = false
					s.ready = configureErr == nil && flowErr == nil
					if s.ready {
						s.configured = player.AccountSession{Revision: revision}
					}
					if storageErr != nil {
						a.accountMessage("nativeAccountStorageError")
					} else if configureErr != nil || flowErr != nil {
						a.accountMessage("nativeAccountError")
					} else {
						recordAccountDiagnostic(a.diagnosticPath, "signed_out")
						a.model.Error = ""
						a.navigate("home")
					}
				})
			}()
		})
	}()
}
