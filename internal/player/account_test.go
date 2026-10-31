package player

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func accountFixture(premium bool, revision uint64) AccountSession {
	return AccountSession{LoggedIn: true, MusicPremium: premium, Cookie: "SAPISID=synthetic-fixture", AccountIndex: 1, Revision: revision}
}

func initializeAnonymousLive(t *testing.T, p *Player) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := p.SetAccount(ctx, AccountSession{Revision: 1}); err != nil {
		t.Fatal("anonymous session configuration failed")
	}
}

func TestAccountMembershipSwitchKeepsPlayingAndReleasesPrefetch(t *testing.T) {
	p := testPlayer()
	defer p.Close()
	var calls atomic.Int64
	p.configure = func(context.Context, AccountSession) error { calls.Add(1); return nil }
	if err := p.SetAccount(context.Background(), accountFixture(false, 1)); err != nil {
		t.Fatal("initial account failed")
	}
	if err := p.Load(context.Background(), "aaaaaaaaaaa"); err != nil {
		t.Fatal("fixture load failed")
	}
	if err := p.Prepare(context.Background(), "bbbbbbbbbbb"); err != nil {
		t.Fatal("fixture preparation failed")
	}
	p.mu.Lock()
	current, media, prepared := p.session, p.media, p.prepared
	p.mu.Unlock()
	if err := p.SetAccount(context.Background(), accountFixture(true, 1)); err != nil {
		t.Fatal("membership switch failed")
	}
	select {
	case <-prepared.retired:
	case <-time.After(time.Second):
		t.Fatal("prefetch did not retire")
	}
	p.mu.Lock()
	same := p.session == current && p.media == media && p.prepared == nil
	p.mu.Unlock()
	if !same || !p.Snapshot().Playing || calls.Load() != 2 {
		t.Fatal("membership switch disrupted current track")
	}
	if err := p.SetAccount(context.Background(), accountFixture(true, 1)); err != nil || calls.Load() != 2 {
		t.Fatal("unchanged status rebuilt engine")
	}
}

func TestAccountChangeStopsOldTrackAndRejectsStaleRevision(t *testing.T) {
	p := testPlayer()
	defer p.Close()
	p.configure = func(context.Context, AccountSession) error { return nil }
	if err := p.SetAccount(context.Background(), accountFixture(true, 3)); err != nil {
		t.Fatal("initial account failed")
	}
	if err := p.Load(context.Background(), "aaaaaaaaaaa"); err != nil {
		t.Fatal("fixture load failed")
	}
	if err := p.SetAccount(context.Background(), AccountSession{Revision: 4}); err != nil {
		t.Fatal("sign out failed")
	}
	if state := p.Snapshot(); state.Playing || state.Loading || state.Duration != 0 {
		t.Fatal("account change retained playback")
	}
	if err := p.SetAccount(context.Background(), accountFixture(true, 3)); err == nil || err.Error() != "stale_account_session" {
		t.Fatal("stale account accepted")
	}
	if err := p.SetAccount(context.Background(), accountFixture(true, 4)); err == nil {
		t.Fatal("identity changed without revision")
	}
}

func TestAccountChangeDuringLeaseOpen(t *testing.T) {
	for _, operation := range []string{"seek", "paused-seek", "replay", "load"} {
		for _, changedIdentity := range []bool{false, true} {
			label := "membership"
			if changedIdentity {
				label = "identity"
			}
			t.Run(operation+"/"+label, func(t *testing.T) {
				p := testPlayer()
				defer p.Close()
				p.configure = func(context.Context, AccountSession) error { return nil }
				if p.SetAccount(context.Background(), accountFixture(false, 1)) != nil || p.Load(context.Background(), "aaaaaaaaaaa") != nil {
					t.Fatal("initial fixture track failed")
				}
				if operation == "paused-seek" || operation == "replay" {
					if p.Pause() != nil {
						t.Fatal("fixture pause failed")
					}
				}
				if operation == "replay" {
					p.mu.Lock()
					p.state.Ended = true
					p.mu.Unlock()
				}
				started, releaseOpen := make(chan context.Context, 1), make(chan struct{})
				var once sync.Once
				release := func() { once.Do(func() { close(releaseOpen) }) }
				defer release()
				var opened *pcmDecoder
				var source *zeroStream
				p.open = func(ctx context.Context, _ media, _ time.Duration) (*pcmDecoder, error) {
					started <- ctx
					<-releaseOpen
					// Deliberately finish even after cancellation: installation must
					// reject a late decoder from a previous identity or canceled Load.
					opened, source = zeroDecoder(60)
					return opened, nil
				}
				done := make(chan error, 1)
				go func() {
					switch operation {
					case "replay":
						done <- p.PlayContext(context.Background())
					case "load":
						done <- p.Load(context.Background(), "bbbbbbbbbbb")
					default:
						done <- p.SeekContext(context.Background(), 20*time.Second)
					}
				}()
				var openContext context.Context
				select {
				case openContext = <-started:
				case <-time.After(time.Second):
					t.Fatal("fixture decoder did not start")
				}
				p.mu.Lock()
				generation := p.gen
				p.mu.Unlock()
				next := accountFixture(true, 1)
				if changedIdentity {
					next.Revision = 2
				}
				if p.SetAccount(context.Background(), next) != nil {
					t.Fatal("account reconfiguration failed")
				}
				keep := !changedIdentity && operation != "load"
				p.mu.Lock()
				preserved := p.gen == generation && p.state.Loading
				p.mu.Unlock()
				if keep != preserved || keep != (openContext.Err() == nil) {
					t.Fatal("pending decoder crossed the wrong account boundary")
				}
				release()
				var err error
				select {
				case err = <-done:
				case <-time.After(time.Second):
					t.Fatal("fixture decoder did not complete")
				}
				if keep {
					if err != nil || source.closed.Load() {
						t.Fatal("membership change canceled the existing-lease decoder")
					}
					wantAt := 20 * time.Second
					if operation == "replay" {
						wantAt = 0
					}
					p.mu.Lock()
					installed := p.session != nil && p.session.decoder == opened && p.session.base == wantAt && !p.state.Loading && p.wantPlay == (operation != "paused-seek")
					p.mu.Unlock()
					if !installed {
						t.Fatal("existing lease lost its position or playing state")
					}
				} else {
					if !errors.Is(err, context.Canceled) || !source.closed.Load() {
						t.Fatal("canceled or old-account decoder was not retired")
					}
					p.mu.Lock()
					published := p.session != nil
					p.mu.Unlock()
					if published {
						t.Fatal("old-account decoder was published")
					}
				}
			})
		}
	}
}

func TestInitialAccountRestoreBlocksResolutionWithoutWarming(t *testing.T) {
	p := testPlayer()
	defer p.Close()
	p.accountInitialized = make(chan struct{})
	p.configure = func(context.Context, AccountSession) error { return nil }
	var resolves atomic.Int64
	p.resolve = func(context.Context, string) (media, error) {
		resolves.Add(1)
		return media{Duration: time.Minute}, nil
	}
	done := make(chan error, 1)
	go func() { done <- p.Load(context.Background(), "aaaaaaaaaaa") }()
	select {
	case <-done:
		t.Fatal("load bypassed account restore")
	case <-time.After(30 * time.Millisecond):
	}
	if resolves.Load() != 0 {
		t.Fatal("resolution started before account restore")
	}
	if err := p.SetAccount(context.Background(), accountFixture(true, 1)); err != nil {
		t.Fatal("configure failed")
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal("load did not resume")
		}
	case <-time.After(time.Second):
		t.Fatal("account restore blocked load")
	}
}

func TestAccountSwitchWaitsForConfigurationAndCloseCancelsIt(t *testing.T) {
	p := testPlayer()
	started := make(chan struct{})
	p.configure = func(ctx context.Context, _ AccountSession) error { close(started); <-ctx.Done(); return ctx.Err() }
	configured := make(chan error, 1)
	go func() { configured <- p.SetAccount(context.Background(), accountFixture(true, 1)) }()
	<-started
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := p.Load(ctx, "aaaaaaaaaaa"); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("load did not wait for configuration")
	}
	closed := make(chan struct{})
	go func() { p.Close(); close(closed) }()
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("close did not cancel configuration")
	}
	if err := <-configured; !errors.Is(err, context.Canceled) {
		t.Fatal("configuration was not canceled")
	}
}

func TestAccountConfigurationFailureBlocksNewResolutionAndCanRetry(t *testing.T) {
	p := testPlayer()
	defer p.Close()
	p.configure = func(context.Context, AccountSession) error { return errors.New("runtime_failed") }
	if err := p.SetAccount(context.Background(), accountFixture(true, 1)); err == nil {
		t.Fatal("failure hidden")
	}
	if err := p.Load(context.Background(), "aaaaaaaaaaa"); err == nil || err.Error() != "account_session_unavailable" {
		t.Fatal("stale mode used after failure")
	}
	p.configure = func(context.Context, AccountSession) error { return nil }
	if err := p.SetAccount(context.Background(), accountFixture(true, 1)); err != nil {
		t.Fatal("retry rejected")
	}
}

func TestResolverKeepsAccountInGoUntilAnOperationNeedsScripts(t *testing.T) {
	r, err := (runtimePaths{helper: "unused-fixture-helper"}).newResolver()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	if err := r.Configure(context.Background(), accountFixture(true, 2)); err != nil {
		t.Fatal("Go configure failed")
	}
	for _, s := range []AccountSession{{MusicPremium: true, Revision: 2}, {Revision: 0}, {LoggedIn: true, Cookie: "bad\nvalue", Revision: 2}, {LoggedIn: true, Cookie: "fixture", AccountIndex: 100, Revision: 2}} {
		if err := r.Configure(context.Background(), s); err == nil || err.Error() != "invalid_account_session" {
			t.Fatal("invalid account accepted")
		}
	}
}
