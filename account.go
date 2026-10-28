package main

import (
	"context"
	"embed"
	"errors"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/transfer"
	"github.com/egoist/mygo/ui"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"
	"ytmgo/internal/accountbridge"
	"ytmgo/internal/catalog"
	"ytmgo/internal/language"
	"ytmgo/internal/player"
	"ytmgo/locales"
)

// Only public extension source is embedded or expanded to disk.
//
//go:embed scripts/account-extension/manifest.json scripts/account-extension/popup.html scripts/account-extension/popup.css scripts/account-extension/popup.mjs scripts/account-extension/session-policy.mjs
var accountExtension embed.FS

const musicHomeURL = "https://music.youtube.com/"

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
	flow                     accountFlow
	store                    accountbridge.Store
	session                  *accountbridge.Session
	configured               player.AccountSession
	revision                 uint64
	restored, ready, started bool
	pollCancel               context.CancelFunc
	bridge                   *accountbridge.Bridge
	bridgeCancel             context.CancelFunc
	message                  string
}
type nativeAccountSecrets struct{}

func (nativeAccountSecrets) Set(key string, value []byte) error { return mygo.Secrets.Set(key, value) }
func (nativeAccountSecrets) Get(key string) ([]byte, error) {
	value, err := mygo.Secrets.Get(key)
	if errors.Is(err, mygo.ErrSecretNotFound) {
		return nil, accountbridge.ErrNotFound
	}
	return value, err
}
func (nativeAccountSecrets) Delete(key string) error {
	err := mygo.Secrets.Delete(key)
	if errors.Is(err, mygo.ErrSecretNotFound) {
		return accountbridge.ErrNotFound
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
		if notice := catalogRecovery(err, recoveryAccount); notice.valid() {
			return recoveryMessageKey(notice.Kind)
		}
		return "nativeAccountError"
	}
	if errors.Is(err, accountbridge.ErrSession) || errors.Is(err, accountbridge.ErrExpired) ||
		errors.Is(err, catalog.ErrInvalidSession) || errors.Is(err, catalog.ErrSessionExpired) ||
		errors.Is(err, catalog.ErrAuthenticationRequired) {
		return "nativeAccountSessionError"
	}
	return "nativeAccountError"
}

func verifyAccountSession(ctx context.Context, candidate accountSessionClient, session accountbridge.Session) (catalog.AccountStatus, catalog.SessionCredentials, error) {
	defer candidate.ClearSession()
	if err := ctx.Err(); err != nil {
		return catalog.AccountStatus{}, catalog.SessionCredentials{}, err
	}
	cookies, err := session.HTTPCookies(time.Now())
	if errors.Is(err, accountbridge.ErrExpired) {
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
		return catalog.AccountStatus{}, catalog.SessionCredentials{}, accountbridge.ErrSession
	}
	return status, credentials, nil
}

type accountSessionStore interface {
	Load() (accountbridge.Session, error)
	Delete() error
}

func restoreAccountSession(store accountSessionStore) (accountbridge.Session, error) {
	session, err := store.Load()
	if !errors.Is(err, accountbridge.ErrExpired) {
		return session, err
	}
	// Delete only this app's indexed secret chunks. A failed deletion remains
	// a storage failure, never an apparently successful anonymous restore.
	if store.Delete() != nil {
		return accountbridge.Session{}, accountbridge.ErrStorage
	}
	return accountbridge.Session{}, accountbridge.ErrNotFound
}

// Saved membership is verified before configuring any playback engine.
func (a *desktop) startAccountPolling() {
	s := &a.accountState
	if s.started {
		return
	}
	s.started = true
	s.revision = 1
	s.store = accountbridge.Store{Secrets: nativeAccountSecrets{}}
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
	if !s.started || a.accountBusy || s.bridge != nil {
		return
	}
	if !s.restored {
		a.restoreAccount()
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
	recordAccountDiagnostic(a.diagnosticPath, "restoring")
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
			if flowErr != nil || (err != nil && !errors.Is(err, accountbridge.ErrNotFound)) {
				a.accountMessage("nativeAccountStorageError")
				return
			}
			s.restored = true
			if err == nil {
				s.session = &session
			}
			a.updateAccount(s.session, false)
		})
	}()
}

// Serial worker plus UI generation gate prevents old account replies from
// replacing a newer session. Network failures preserve the verified account.
func (a *desktop) updateAccount(session *accountbridge.Session, newIdentity bool) {
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
	if newIdentity {
		revision++
	}
	ctx := s.flow.begin(a.ctx, generation)
	locale := language.ServiceLanguage(mygo.App.Locale())
	previousCredentials, hadCredentials := a.catalog.Credentials()
	s.message = "nativeAccountChecking"
	a.invalidateAccount()
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
			err = accountbridge.ErrSession
		}
		errorKey := accountErrorMessageKey(err)
		if err == nil && newIdentity {
			err = s.store.Save(ctx, *session)
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
			if flowErr != nil {
				a.accountFailure("nativeAccountError", failureCategory)
				return
			}
			identityChanged := false
			if verified {
				s.session, s.revision, s.restored, s.ready = session, revision, true, configured
				if configured {
					s.configured = next
				}
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
				a.accountRequestFailure(errorKey, failureCategory, err, newIdentity)
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
			s.message = ""
			a.clearAccountRequestFailure(newIdentity)
			a.preparedID = ""
			if replay.pending && replay.generation == a.playGeneration {
				a.startTrack(replay.item)
			} else if a.model.Playing {
				a.prepareNext()
			}
			if newIdentity {
				if a.account != nil && !a.account.IsDestroyed() {
					a.account.Destroy()
					a.account = nil
				}
				a.navigate("library")
				a.show()
			} else if identityChanged {
				a.navigate("home")
			} else {
				a.invalidateAccount()
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
		identityChanged := newIdentity || (a.model.SignedIn && !next.LoggedIn)
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
	if a.account != nil && !a.account.IsDestroyed() {
		a.account.Show()
		a.account.Focus()
		return
	}
	a.accountState.message = "nativeAccountWaiting"
	w := mygo.NewWindow(mygo.WindowOptions{Title: locales.LoginWindowTitle(), Width: 640, Height: 600, MinWidth: 480, MinHeight: 480, Content: ui.View(a.accountView)})
	a.account = w
	w.OnClose(func(*mygo.CloseEvent) {
		if a.account == w {
			a.account = nil
			a.stopAccountBridge()
		}
	})
	a.beginAccountBridge()
}
func (a *desktop) beginAccountBridge() {
	if a.accountBusy {
		return
	}
	// A deliberate new transfer replaces the failed transfer's prompt, while
	// catalog failures and the previously connected account remain untouched.
	a.clearAccountRequestFailure(true)
	a.stopAccountBridge()
	ctx, cancel := context.WithCancel(a.ctx)
	bridge, err := accountbridge.Listen(ctx)
	if err != nil {
		cancel()
		a.accountMessage("nativeAccountBridgeError")
		return
	}
	s := &a.accountState
	s.bridge, s.bridgeCancel, s.message = bridge, cancel, "nativeAccountWaiting"
	a.invalidateAccount()
	go func() {
		session, err := bridge.Wait(ctx)
		a.post(func() {
			if s.bridge != bridge {
				return
			}
			a.stopAccountBridge()
			if err != nil {
				a.accountMessage("nativeAccountExpired")
				return
			}
			a.updateAccount(&session, true)
		})
	}()
}
func (a *desktop) stopAccountBridge() {
	s := &a.accountState
	if s.bridge != nil {
		s.bridge.Close()
		s.bridge = nil
	}
	if s.bridgeCancel != nil {
		s.bridgeCancel()
		s.bridgeCancel = nil
	}
}
func (a *desktop) closeAccount() {
	a.accountGeneration++
	s := &a.accountState
	s.flow.cancelPending()
	if s.pollCancel != nil {
		s.pollCancel()
	}
	a.stopAccountBridge()
	s.session = nil
	s.configured = player.AccountSession{}
}
func (a *desktop) accountMessage(key string) {
	a.accountFailure(key, "session_failed")
}
func (a *desktop) accountFailure(key, category string) {
	recordAccountDiagnostic(a.diagnosticPath, category)
	a.accountState.message = key
	a.model.Error = a.tr(key)
	a.model.ErrorRecovery = recoveryNotice{Scope: recoveryAccount}
	a.invalidateAccount()
}

func (a *desktop) accountRequestFailure(key, category string, err error, newIdentity bool) {
	a.accountFailure(key, category)
	a.model.ErrorRecovery = accountRequestRecovery(err, newIdentity)
}
func (a *desktop) invalidateAccount() {
	a.invalidate()
	if a.account != nil && !a.account.IsDestroyed() {
		a.account.Update(func() {})
	}
}
func (a *desktop) accountView(c *ui.Context) {
	dark := c.Theme().Dark
	if a.model.Appearance == "dark" {
		dark = true
	}
	if a.model.Appearance == "light" {
		dark = false
	}
	c.SetTheme(nativeTheme(dark))
	p := palette(dark)
	ui.Scroll(c).Fill().Background(p.base).TextColor(p.text).Padding(24).Gap(16).Children(func() {
		ui.Text(c, locales.LoginWindowTitle()).FontSize(24)
		ui.Text(c, a.tr("nativeAccountGuide")).FontSize(14).LineHeight(1.5)
		if nativePill(c, a.tr("nativeAccountExtensionFolder"), p, false).AlignSelf(ui.Start).Clicked() {
			a.openAccountExtension()
		}
		if nativePill(c, a.tr("nativeAccountCopyMusicURL"), p, false).AlignSelf(ui.Start).Clicked() {
			if mygo.Clipboard.Write(transfer.TextData(musicHomeURL)) != nil {
				a.accountMessage("nativeAccountMusicURLCopyError")
			} else {
				a.accountState.message = "nativeAccountMusicURLCopied"
				a.invalidateAccount()
			}
		}
		if nativePill(c, a.tr("nativeAccountOpenMusic"), p, true).AlignSelf(ui.Start).Clicked() {
			a.openAccountBrowser()
		}
		ui.Text(c, a.tr("nativeAccountPrivacy")).FontSize(13).TextColor(p.muted).LineHeight(1.4)
		accountRecovery := (a.model.ErrorRecovery.Scope == recoveryAccount || a.model.ErrorRecovery.Scope == recoveryAccountConnect) && a.model.ErrorRecovery.valid()
		if accountRecovery && !a.accountBusy {
			ui.Text(c, a.model.Error).FontSize(13).LineHeight(1.4)
			if nativePill(c, a.tr("nativeRecoveryHelp"), p, true).AlignSelf(ui.Start).Clicked() {
				a.showRecovery(a.model.ErrorRecovery)
			}
		} else if a.accountState.message != "" {
			ui.Text(c, a.tr(a.accountState.message)).FontSize(13).LineHeight(1.4)
		}
		if a.accountState.bridge == nil && !a.accountBusy {
			if nativePill(c, a.tr("buttonRetry"), p, false).Clicked() {
				a.beginAccountBridge()
			}
		}
	})
}
func (a *desktop) openAccountBrowser() {
	go func() {
		if mygo.Shell.OpenExternal(musicHomeURL) != nil {
			a.post(func() { a.accountMessage("nativeAccountBrowserError") })
		}
	}()
}
func (a *desktop) openAccountExtension() {
	go func() {
		root, err := mygo.App.Path(mygo.PathUserData)
		if err == nil {
			root = filepath.Join(root, "account-extension")
			err = os.MkdirAll(root, 0700)
		}
		for _, name := range []string{"manifest.json", "popup.html", "popup.css", "popup.mjs", "session-policy.mjs"} {
			if err != nil {
				break
			}
			var data []byte
			data, err = accountExtension.ReadFile("scripts/account-extension/" + name)
			if err == nil {
				err = os.WriteFile(filepath.Join(root, name), data, 0600)
			}
		}
		if err == nil {
			err = mygo.Shell.OpenPath(root)
		}
		if err != nil {
			a.post(func() { a.accountMessage("nativeAccountBridgeError") })
		}
	}()
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
	a.model.ErrorRecovery = recoveryNotice{}
	a.model.RecoveryOpen = false
	a.history = nil
	a.historyIndex = -1
}

func (a *desktop) clearAccountPlayback() {
	a.model.Queue = nil
	a.model.Current = catalog.Item{}
	a.queueIndex = -1
	a.queueGeneration++
	a.model.PlaybackError = ""
	a.model.PlaybackRecovery = recoveryNotice{}
	a.model.Lyrics = ""
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
	a.accountBusy = true
	go func() {
		answer, err := mygo.Dialog.Message(mygo.MessageOptions{Parent: a.window, Title: locales.MenuSignOut(), Message: a.tr("nativeSignOutConfirm"), Buttons: []string{a.tr("shellClose"), a.tr("shellSignOut")}, CancelButton: 0})
		a.post(func() {
			a.accountBusy = false
			if err != nil || answer.Button != 1 {
				return
			}
			a.stopAccountBridge()
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
			if a.account != nil && !a.account.IsDestroyed() {
				a.account.Destroy()
				a.account = nil
			}
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
			a.model.PlaybackRecovery = recoveryNotice{}
			a.model.Lyrics = ""
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
