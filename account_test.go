package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"testing"
	"time"

	"ytmgo/internal/accountsession"
	"ytmgo/internal/catalog"
	"ytmgo/internal/player"
)

type stagedSessionFixture struct {
	setErr              error
	library             func(context.Context) (catalog.AccountStatus, error)
	set, reads, cleared int
	cookieCount         int
}

func TestAccountErrorsOnlyRequestReconnectionForInvalidSessions(t *testing.T) {
	for _, status := range []int{401, 403, 429, 500, 503} {
		t.Run(fmt.Sprintf("http-%d", status), func(t *testing.T) {
			err := fmt.Errorf("account request: %w", &catalog.HTTPError{Stage: "browse", StatusCode: status})
			if accountErrorMessageKey(err) != "nativeAccountError" {
				t.Fatal("HTTP failure incorrectly asked the user to replace their session")
			}
		})
	}
	for _, err := range []error{context.DeadlineExceeded, context.Canceled, catalog.ErrUnavailable, catalog.ErrAccountStatusUnavailable, catalog.ErrSessionChanged} {
		if accountErrorMessageKey(err) != "nativeAccountError" {
			t.Fatal("unverified account failure incorrectly requested reconnection")
		}
	}
	for _, err := range []error{accountsession.ErrSession, accountsession.ErrExpired, catalog.ErrInvalidSession, catalog.ErrSessionExpired, catalog.ErrAuthenticationRequired} {
		if accountErrorMessageKey(fmt.Errorf("account session: %w", err)) != "nativeAccountSessionError" {
			t.Fatal("invalid or expired session did not request reconnection")
		}
	}
}

func (f *stagedSessionFixture) SetSession(cookies []*http.Cookie, _ int, _ string) error {
	f.set++
	f.cookieCount = len(cookies)
	return f.setErr
}
func (f *stagedSessionFixture) AccountStatus(ctx context.Context) (catalog.AccountStatus, error) {
	f.reads++
	return f.library(ctx)
}
func (f *stagedSessionFixture) ClearSession() { f.cleared++ }

func TestAccountSessionInstallsOnlyAfterVerification(t *testing.T) {
	flow := &accountFlow{}
	ctx := flow.begin(context.Background(), 3)
	defer flow.cancelPending()
	activeInstalls := 0
	candidate := &stagedSessionFixture{library: func(context.Context) (catalog.AccountStatus, error) {
		if activeInstalls != 0 {
			t.Fatal("active session changed before verification")
		}
		return catalog.AccountStatus{LoggedIn: true, MusicPremium: true}, nil
	}}
	status, _, err := verifyAccountSession(ctx, candidate, syntheticAccountSession())
	if err != nil || !status.MusicPremium || candidate.set != 1 || candidate.reads != 1 || candidate.cleared != 1 || activeInstalls != 0 {
		t.Fatal("candidate verification changed the active session or retained its cookies")
	}
	accepted, err := flow.finish(ctx, 3, 3, err, func() error { activeInstalls++; return nil })
	if !accepted || err != nil || activeInstalls != 1 {
		t.Fatal("verified current session was not installed once")
	}
	if accepted, _ := flow.finish(ctx, 3, 3, nil, func() error { activeInstalls++; return nil }); accepted || activeInstalls != 1 {
		t.Fatal("completed account result was accepted twice")
	}
}

func TestFailedAccountVerificationPreservesActiveSession(t *testing.T) {
	flow := &accountFlow{}
	ctx := flow.begin(context.Background(), 3)
	defer flow.cancelPending()
	candidate := &stagedSessionFixture{library: func(context.Context) (catalog.AccountStatus, error) {
		return catalog.AccountStatus{}, errors.New("fixture library rejected")
	}}
	_, _, err := verifyAccountSession(ctx, candidate, syntheticAccountSession())
	installs := 0
	accepted, err := flow.finish(ctx, 3, 3, err, func() error { installs++; return nil })
	if !accepted || err == nil || installs != 0 || candidate.cleared != 1 {
		t.Fatal("failed staged session affected active account")
	}
}

func TestClosingOrLoggingOutCancelsStagedAccountRead(t *testing.T) {
	flow := &accountFlow{}
	ctx := flow.begin(context.Background(), 3)
	started := make(chan struct{})
	candidate := &stagedSessionFixture{library: func(ctx context.Context) (catalog.AccountStatus, error) {
		close(started)
		<-ctx.Done()
		return catalog.AccountStatus{}, ctx.Err()
	}}
	finished := make(chan error, 1)
	go func() { _, _, err := verifyAccountSession(ctx, candidate, syntheticAccountSession()); finished <- err }()
	<-started
	flow.cancelPending() // both auth-window close and confirmed sign-out use this
	select {
	case err := <-finished:
		if !errors.Is(err, context.Canceled) {
			t.Fatal("staged network request was not canceled")
		}
	case <-time.After(time.Second):
		t.Fatal("staged network request remained active")
	}
	installs := 0
	if accepted, _ := flow.finish(ctx, 3, 4, nil, func() error { installs++; return nil }); accepted || installs != 0 || candidate.cleared != 1 {
		t.Fatal("canceled account verification installed credentials")
	}
}

func TestReplacedAccountFlowCannotInstallLateResult(t *testing.T) {
	flow := &accountFlow{}
	first := flow.begin(context.Background(), 3)
	second := flow.begin(context.Background(), 4)
	defer flow.cancelPending()
	if !errors.Is(first.Err(), context.Canceled) {
		t.Fatal("replacing account flow did not cancel prior request")
	}
	installs := 0
	if accepted, _ := flow.finish(first, 3, 4, nil, func() error { installs++; return nil }); accepted || installs != 0 {
		t.Fatal("stale verification was installed")
	}
	if accepted, err := flow.finish(second, 4, 4, nil, func() error { installs++; return nil }); !accepted || err != nil || installs != 1 {
		t.Fatal("stale completion disturbed the current flow")
	}
}

func TestAccountDeadlineReportsFailureWithoutInstalling(t *testing.T) {
	parent, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()
	flow := &accountFlow{}
	ctx := flow.begin(parent, 1)
	installs := 0
	accepted, err := flow.finish(ctx, 1, 1, nil, func() error { installs++; return nil })
	if !accepted || !errors.Is(err, context.DeadlineExceeded) || installs != 0 || flow.ctx != nil {
		t.Fatal("expired verification left the sign-in flow pending or installed credentials")
	}
}

func TestIncompleteStagedCookiesNeverQueryLibrary(t *testing.T) {
	candidate := &stagedSessionFixture{setErr: catalog.ErrInvalidSession}
	_, _, err := verifyAccountSession(context.Background(), candidate, syntheticAccountSession())
	if !errors.Is(err, catalog.ErrInvalidSession) || candidate.reads != 0 || candidate.cleared != 1 {
		t.Fatal("incomplete sign-in reached library or retained candidate cookies")
	}
}

func (f *stagedSessionFixture) Credentials() (catalog.SessionCredentials, bool) {
	return catalog.SessionCredentials{Cookie: "SAPISID=synthetic-only", AccountIndex: 2}, true
}
func syntheticAccountSession() accountsession.Session {
	i := 2
	secure := true
	return accountsession.Session{AccountIndex: &i, Cookies: []accountsession.Cookie{{Name: "SAPISID", Value: "synthetic-only", Domain: ".youtube.com", Path: "/", Secure: &secure}}}
}

func TestAccountConfigurationChangesIncludeCredentialRefresh(t *testing.T) {
	previous := player.AccountSession{LoggedIn: true, MusicPremium: true, Cookie: "SAPISID=synthetic; PREF=synthetic", AccountIndex: 2, Revision: 4}
	if accountConfigurationChanged(previous, previous, true) {
		t.Fatal("unchanged verified account must preserve in-flight playback")
	}
	if !accountConfigurationChanged(previous, previous, false) {
		t.Fatal("failed configuration must retry")
	}
	for _, change := range []func(*player.AccountSession){
		func(s *player.AccountSession) { s.Cookie = "SAPISID=synthetic" },
		func(s *player.AccountSession) { s.MusicPremium = false },
		func(s *player.AccountSession) { s.Revision++ },
	} {
		next := previous
		change(&next)
		if !accountConfigurationChanged(previous, next, true) {
			t.Fatal("resolver-changing state was omitted from Load coordination")
		}
	}
}

func TestAccountOptionalExpiryStillQueriesMembership(t *testing.T) {
	s := syntheticAccountSession()
	optional := s.Cookies[0]
	optional.Name = "PREF"
	expired := float64(time.Now().Add(-time.Minute).Unix())
	optional.ExpirationDate = &expired
	s.Cookies = append(s.Cookies, optional)
	candidate := &stagedSessionFixture{library: func(context.Context) (catalog.AccountStatus, error) {
		return catalog.AccountStatus{LoggedIn: true, MusicPremium: true}, nil
	}}
	status, _, err := verifyAccountSession(context.Background(), candidate, s)
	if err != nil || !status.LoggedIn || !status.MusicPremium || candidate.reads != 1 || candidate.cookieCount != 1 {
		t.Fatal("optional expiry bypassed service membership verification")
	}
}

func TestAccountRequiredExpiryReturnsExplicitSignedOutWithoutNetwork(t *testing.T) {
	s := syntheticAccountSession()
	expired := float64(time.Now().Add(-time.Minute).Unix())
	s.Cookies[0].ExpirationDate = &expired
	candidate := &stagedSessionFixture{}
	status, credentials, err := verifyAccountSession(context.Background(), candidate, s)
	if err != nil || status.LoggedIn || status.MusicPremium || credentials.Cookie != "" || candidate.reads != 0 || candidate.set != 0 || candidate.cleared != 1 {
		t.Fatal("known required expiry retained authentication or queried network")
	}
}

type restoreSessionFixture struct {
	loadErr, deleteErr                 error
	deletes                            int
	loadInteraction, deleteInteraction accountsession.Interaction
}

func (f *restoreSessionFixture) Load(interaction accountsession.Interaction) (accountsession.Session, error) {
	f.loadInteraction = interaction
	return accountsession.Session{}, f.loadErr
}
func (f *restoreSessionFixture) Delete(interaction accountsession.Interaction) error {
	f.deletes++
	f.deleteInteraction = interaction
	return f.deleteErr
}
func TestAccountRestoreDistinguishesExpiryStorageAndMalformedSession(t *testing.T) {
	for _, test := range []struct {
		loadErr, deleteErr, want error
		deletes                  int
	}{
		{accountsession.ErrExpired, nil, accountsession.ErrNotFound, 1},
		{accountsession.ErrExpired, errors.New("synthetic-denied"), accountsession.ErrStorage, 1},
		{accountsession.ErrStorage, nil, accountsession.ErrStorage, 0},
		{accountsession.ErrSession, nil, accountsession.ErrSession, 0},
		{accountsession.ErrNotFound, nil, accountsession.ErrNotFound, 0},
	} {
		for _, interaction := range []accountsession.Interaction{accountsession.NoPrompt, accountsession.AllowPrompt} {
			store := &restoreSessionFixture{loadErr: test.loadErr, deleteErr: test.deleteErr}
			_, err := restoreAccountSession(store, interaction)
			if !errors.Is(err, test.want) || store.deletes != test.deletes {
				t.Fatal("restore conflated expiration with a storage or validation failure")
			}
			if store.loadInteraction != interaction || store.deletes != 0 && store.deleteInteraction != interaction {
				t.Fatal("restore lost the caller's authorization policy")
			}
		}
	}
}
