package player

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func denyCurrentDecoder(p *Player) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.session.output.Pause()
	p.session.decoder.failure.Store(&decoderFailure{category: "audio_access_denied", details: FailureDetails{Stage: "media_stream", HTTPStatus: 403}})
	p.session.decoder.failed.Store(true)
	p.session.decoder.eof.Store(true)
}

func TestAccessRecoveryReResolvesAtBookmarkAndIsBounded(t *testing.T) {
	p := testPlayer()
	defer p.Close()
	var resolves, refreshes, releases atomic.Int32
	p.resolve = func(context.Context, string) (media, error) {
		resolves.Add(1)
		return media{Duration: time.Minute, release: func() { releases.Add(1) }}, nil
	}
	p.refresh = func(context.Context) error { refreshes.Add(1); return nil }
	if err := p.LoadAt(context.Background(), "aaaaaaaaaaa", 24*time.Second); err != nil {
		t.Fatal(err)
	}
	denyCurrentDecoder(p)
	if state := p.Snapshot(); !state.Loading || state.Error != "" || state.Ended || state.Position < 24*time.Second {
		t.Fatal("denied stream did not enter recovery at its bookmark")
	}
	waitPlayerCondition(t, func() bool { return !p.Snapshot().Loading })
	if state := p.Snapshot(); state.Error != "" || resolves.Load() != 2 || refreshes.Load() != 1 || releases.Load() != 1 || state.Position < 24*time.Second {
		t.Fatal("recovery reused the old lease, proof, or position")
	}
	denyCurrentDecoder(p)
	if state := p.Snapshot(); state.Error != "audio_access_denied" || state.ErrorDetails.HTTPStatus != 403 || state.Loading || resolves.Load() != 2 {
		t.Fatal("persistent denial exceeded its automatic recovery budget")
	}
	if err := p.LoadAt(context.Background(), "aaaaaaaaaaa", 24*time.Second); err != nil {
		t.Fatal(err)
	}
	if resolves.Load() != 3 || refreshes.Load() != 2 {
		t.Fatal("explicit retry failed to refresh resolution state")
	}
}

func TestAccessRecoveryIsCancelledByStop(t *testing.T) {
	p := testPlayer()
	defer p.Close()
	entered, canceled := make(chan struct{}), make(chan struct{})
	p.refresh = func(ctx context.Context) error {
		close(entered)
		<-ctx.Done()
		close(canceled)
		return ctx.Err()
	}
	if err := p.Load(context.Background(), "aaaaaaaaaaa"); err != nil {
		t.Fatal(err)
	}
	denyCurrentDecoder(p)
	p.Snapshot()
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("recovery did not start")
	}
	if err := p.Stop(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-canceled:
	case <-time.After(3 * time.Second):
		t.Fatal("stop did not cancel recovery")
	}
	if s := p.Snapshot(); s.Loading || s.Playing || s.Error != "" {
		t.Fatal("canceled recovery changed the stopped player")
	}
}

func TestSeekDeniedReResolvesAndKeepsPausedIntent(t *testing.T) {
	p := testPlayer()
	defer p.Close()
	var opens, resolves, refreshes atomic.Int32
	p.resolve = func(context.Context, string) (media, error) {
		resolves.Add(1)
		return media{Duration: time.Minute}, nil
	}
	p.refresh = func(context.Context) error { refreshes.Add(1); return nil }
	p.open = func(_ context.Context, _ media, at time.Duration) (*pcmDecoder, error) {
		if opens.Add(1) == 2 {
			return nil, errors.New("audio_access_denied")
		}
		if opens.Load() == 3 && at != 31*time.Second {
			t.Error("seek recovery lost its bookmark")
		}
		d, _ := zeroDecoder(60)
		return d, nil
	}
	if err := p.Load(context.Background(), "aaaaaaaaaaa"); err != nil {
		t.Fatal(err)
	}
	p.Pause()
	if err := p.Seek(31 * time.Second); err != nil {
		t.Fatal(err)
	}
	if s := p.Snapshot(); s.Playing || s.Loading || s.Error != "" || s.Position != 31*time.Second || resolves.Load() != 2 || refreshes.Load() != 1 {
		t.Fatal("seek recovery reused the denied lease or resumed paused playback")
	}
}

func TestProofRetryRefreshesBeforeResolution(t *testing.T) {
	p := testPlayer()
	defer p.Close()
	var count atomic.Int32
	p.resolve = func(context.Context, string) (media, error) {
		if count.Add(1) == 1 {
			return media{}, errors.New("region_unavailable")
		}
		return media{Duration: time.Minute}, nil
	}
	refreshed := false
	p.refresh = func(context.Context) error { refreshed = true; return nil }
	if p.Load(context.Background(), "aaaaaaaaaaa") == nil || p.Snapshot().Error != "region_unavailable" {
		t.Fatal("region error was lost")
	}
	if err := p.Load(context.Background(), "aaaaaaaaaaa"); err != nil || !refreshed || p.Snapshot().Error != "" {
		t.Fatal("retry retained failed proof state")
	}
}

func TestAccessRecoveryBudgetReturnsAfterHealthyPlayback(t *testing.T) {
	p := testPlayer()
	defer p.Close()
	var resolves atomic.Int32
	p.resolve = func(context.Context, string) (media, error) {
		resolves.Add(1)
		return media{Duration: time.Minute}, nil
	}
	if err := p.LoadAt(context.Background(), "aaaaaaaaaaa", 12*time.Second); err != nil {
		t.Fatal(err)
	}
	denyCurrentDecoder(p)
	p.Snapshot()
	waitPlayerCondition(t, func() bool {
		s := p.Snapshot()
		return !s.Loading && s.Playing
	})
	// Drive the heard-progress policy explicitly. A hosted audio device need
	// not advance within a wall-clock deadline for a recovery-policy test.
	p.mu.Lock()
	p.observeAccessProgressLocked(p.accessHealthyFrom + accessRecoveryHealthyPlayback - time.Nanosecond)
	early := p.accessRecoveries == 0
	p.observeAccessProgressLocked(p.accessHealthyFrom + accessRecoveryHealthyPlayback)
	restored := p.accessRecoveries == 0
	p.mu.Unlock()
	if early || !restored {
		t.Fatal("recovery budget was not restored at the healthy playback boundary")
	}
	denyCurrentDecoder(p)
	if s := p.Snapshot(); !s.Loading || s.Error != "" {
		t.Fatal("healthy playback did not restore the automatic recovery budget")
	}
	waitPlayerCondition(t, func() bool { return !p.Snapshot().Loading })
	if resolves.Load() != 3 {
		t.Fatal("second denial did not obtain a fresh stream")
	}
	denyCurrentDecoder(p)
	if s := p.Snapshot(); s.Loading || s.Error != "audio_access_denied" || resolves.Load() != 3 {
		t.Fatal("rapid repeated denial escaped the recovery bound")
	}
}

func TestPausedAndSeekTimeCannotRestoreAccessBudget(t *testing.T) {
	p := testPlayer()
	defer p.Close()
	if err := p.Load(context.Background(), "aaaaaaaaaaa"); err != nil {
		t.Fatal(err)
	}
	denyCurrentDecoder(p)
	p.Snapshot()
	waitPlayerCondition(t, func() bool { return !p.Snapshot().Loading })
	p.Pause()
	p.mu.Lock()
	p.observeAccessProgressLocked(p.accessHealthyFrom + accessRecoveryHealthyPlayback)
	p.mu.Unlock()
	p.Snapshot()
	if err := p.Seek(45 * time.Second); err != nil {
		t.Fatal(err)
	}
	p.Snapshot()
	p.mu.Lock()
	spent := p.accessRecoveries != 0
	p.mu.Unlock()
	if !spent {
		t.Fatal("paused wall time or seek distance replenished the budget")
	}
	if err := p.Play(); err != nil {
		t.Fatal(err)
	}
	denyCurrentDecoder(p)
	if s := p.Snapshot(); s.Loading || s.Error != "audio_access_denied" {
		t.Fatal("paused/seeked playback incorrectly received another recovery")
	}
}

func TestFreshURLFirstOpenDenialHasOneExtraAttempt(t *testing.T) {
	for _, persist := range []bool{false, true} {
		t.Run(map[bool]string{false: "transient", true: "persistent"}[persist], func(t *testing.T) {
			p := testPlayer()
			defer p.Close()
			var opens, resolves, releases atomic.Int32
			p.resolve = func(context.Context, string) (media, error) {
				resolves.Add(1)
				return media{Duration: time.Minute, release: func() { releases.Add(1) }}, nil
			}
			p.open = func(context.Context, media, time.Duration) (*pcmDecoder, error) {
				n := opens.Add(1)
				if n == 2 || persist && n >= 2 {
					return nil, errors.New("audio_access_denied")
				}
				d, _ := zeroDecoder(60)
				return d, nil
			}
			if err := p.Load(context.Background(), "aaaaaaaaaaa"); err != nil {
				t.Fatal(err)
			}
			denyCurrentDecoder(p)
			p.Snapshot()
			waitPlayerCondition(t, func() bool { return !p.Snapshot().Loading })
			state := p.Snapshot()
			if opens.Load() != 3 || resolves.Load() != 3 || persist && state.Error != "audio_access_denied" || !persist && state.Error != "" {
				t.Fatal("fresh-open denial was not retried once or exceeded its bound")
			}
			p.Stop()
			if releases.Load() != resolves.Load() {
				t.Fatal("fresh-open attempts leaked or double-released media")
			}
		})
	}
}

func TestColdOpenDenialIsRetriedOnce(t *testing.T) {
	for _, category := range []string{"audio_access_denied", "audio_network_timeout"} {
		t.Run(category, func(t *testing.T) {
			p := testPlayer()
			defer p.Close()
			var opens atomic.Int32
			p.open = func(context.Context, media, time.Duration) (*pcmDecoder, error) {
				opens.Add(1)
				return nil, errors.New(category)
			}
			if err := p.Load(context.Background(), "aaaaaaaaaaa"); err == nil {
				t.Fatal("persistent startup failure was hidden")
			}
			want := int32(1)
			if category == "audio_access_denied" {
				want = 2
			}
			if opens.Load() != want {
				t.Fatal("startup retry was missing, unbounded, or retried another failure category")
			}
		})
	}
}

func TestSeekDuringRecoveryReplacesBookmarkAndCancelsOldWork(t *testing.T) {
	for _, pause := range []bool{false, true} {
		t.Run(map[bool]string{false: "playing", true: "paused"}[pause], func(t *testing.T) {
			p := testPlayer()
			defer p.Close()
			entered, canceled := make(chan struct{}), make(chan struct{})
			var refreshes atomic.Int32
			p.refresh = func(ctx context.Context) error {
				if refreshes.Add(1) == 1 {
					close(entered)
					<-ctx.Done()
					close(canceled)
					return ctx.Err()
				}
				return nil
			}
			if err := p.Load(context.Background(), "aaaaaaaaaaa"); err != nil {
				t.Fatal(err)
			}
			denyCurrentDecoder(p)
			p.Snapshot()
			select {
			case <-entered:
			case <-time.After(3 * time.Second):
				t.Fatal("recovery did not start")
			}
			if pause {
				p.Pause()
			}
			if err := p.SeekContext(context.Background(), 43*time.Second); err != nil {
				t.Fatal("seek during recovery was dropped")
			}
			select {
			case <-canceled:
			case <-time.After(3 * time.Second):
				t.Fatal("old recovery continued after the new seek")
			}
			state := p.Snapshot()
			if state.Loading || state.Error != "" || state.Position < 43*time.Second || state.Playing == pause {
				t.Fatal("recovery seek lost its target or play/pause intent")
			}
		})
	}
}

func TestSeekSupersedesInitialResolution(t *testing.T) {
	p := testPlayer()
	defer p.Close()
	entered := make(chan struct{})
	var resolves atomic.Int32
	p.resolve = func(ctx context.Context, _ string) (media, error) {
		if resolves.Add(1) == 1 {
			close(entered)
			<-ctx.Done()
			return media{}, ctx.Err()
		}
		return media{Duration: time.Minute}, nil
	}
	done := make(chan error, 1)
	go func() { done <- p.Load(context.Background(), "aaaaaaaaaaa") }()
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("initial resolution did not start")
	}
	if err := p.Seek(35 * time.Second); err != nil {
		t.Fatal("seek during initial resolution was dropped")
	}
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatal("seek did not supersede the old initial load")
	}
	if s := p.Snapshot(); s.Error != "" || s.Loading || s.Position < 35*time.Second {
		t.Fatal("old load overwrote the newer seek bookmark")
	}
}

func TestSeekAfterCancellingDeniedSeekRecovery(t *testing.T) {
	for _, stage := range []string{"refresh", "resolve", "open"} {
		for _, handoff := range []string{"finished", "in-flight"} {
			for _, paused := range []bool{false, true} {
				name := stage + "/" + handoff + "/" + map[bool]string{false: "playing", true: "paused"}[paused]
				t.Run(name, func(t *testing.T) {
					p := testPlayer()
					t.Cleanup(p.Close)
					entered, canceled, finish := make(chan struct{}), make(chan struct{}), make(chan struct{})
					var finishOnce sync.Once
					unblock := func() { finishOnce.Do(func() { close(finish) }) }
					ctx, cancel := context.WithCancel(context.Background())
					t.Cleanup(func() { cancel(); unblock() })
					block := func(ctx context.Context) error {
						close(entered)
						<-ctx.Done()
						close(canceled)
						<-finish
						return ctx.Err()
					}
					var refreshes, resolves, opens, releases atomic.Int32
					p.refresh = func(ctx context.Context) error {
						if refreshes.Add(1) == 1 && stage == "refresh" {
							return block(ctx)
						}
						return nil
					}
					p.resolve = func(ctx context.Context, _ string) (media, error) {
						if resolves.Add(1) == 2 && stage == "resolve" {
							return media{}, block(ctx)
						}
						return media{Duration: time.Minute, release: func() { releases.Add(1) }}, nil
					}
					var discarded *zeroStream
					p.open = func(ctx context.Context, _ media, _ time.Duration) (*pcmDecoder, error) {
						n := opens.Add(1)
						if n == 2 {
							return nil, errors.New("audio_access_denied")
						}
						if n == 3 && stage == "open" {
							_ = block(ctx)
							// Even a dependency returning success after cancellation
							// must not publish its stale decoder or leak its lease.
							d, stream := zeroDecoder(60)
							discarded = stream
							return d, nil
						}
						d, _ := zeroDecoder(60)
						return d, nil
					}
					if err := p.Load(context.Background(), "aaaaaaaaaaa"); err != nil {
						t.Fatal(err)
					}
					if paused {
						p.Pause()
					}
					done := make(chan error, 1)
					go func() { done <- p.SeekContext(ctx, 10*time.Second) }()
					waitRecoverySignal(t, entered)
					// desktop.seek cancels the previous gesture before scheduling
					// the new worker. Exercise both sides of that scheduling gap.
					cancel()
					waitRecoverySignal(t, canceled)
					if handoff == "finished" {
						unblock()
						if err := <-done; !errors.Is(err, context.Canceled) {
							t.Fatalf("old seek = %v, want cancellation", err)
						}
						if s := p.Snapshot(); s.Loading || s.Playing || s.Error != "" {
							t.Fatalf("canceled recovery left a spinner or error: %+v", s)
						}
					}
					if err := p.SeekContext(context.Background(), 43*time.Second); err != nil {
						t.Fatalf("replacement seek was lost after cancellation: %v", err)
					}
					if handoff == "in-flight" {
						unblock()
						if err := <-done; !errors.Is(err, context.Canceled) {
							t.Fatalf("stale seek = %v, want cancellation", err)
						}
					}
					if s := p.Snapshot(); s.Loading || s.Error != "" || s.Position < 43*time.Second || s.Playing == paused {
						t.Fatalf("replacement lost its bookmark or play/pause intent: %+v", s)
					}
					if discarded != nil && !discarded.closed.Load() {
						t.Fatal("canceled decoder was not closed")
					}
					p.Stop()
					wantReleases := resolves.Load()
					if stage == "resolve" {
						wantReleases-- // The canceled resolution never acquired a lease.
					}
					if releases.Load() != wantReleases {
						t.Fatalf("leases released = %d, want %d", releases.Load(), wantReleases)
					}
					if err := p.Seek(20 * time.Second); !errors.Is(err, ErrNoTrack) {
						t.Fatalf("seek resurrected a stopped track: %v", err)
					}
				})
			}
		}
	}
}

func waitRecoverySignal(t *testing.T, signal <-chan struct{}) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(3 * time.Second):
		t.Fatal("recovery transition timed out")
	}
}

func TestCancelledSeekRecoveryCannotReviveRetiredTrack(t *testing.T) {
	for _, action := range []string{"stop", "account switch", "new load", "terminal failure"} {
		t.Run(action, func(t *testing.T) {
			p := testPlayer()
			t.Cleanup(p.Close)
			entered := make(chan struct{})
			p.refresh = func(ctx context.Context) error {
				close(entered)
				<-ctx.Done()
				return ctx.Err()
			}
			var opens atomic.Int32
			p.open = func(context.Context, media, time.Duration) (*pcmDecoder, error) {
				if opens.Add(1) == 2 {
					return nil, errors.New("audio_access_denied")
				}
				d, _ := zeroDecoder(60)
				return d, nil
			}
			if err := p.Load(context.Background(), "aaaaaaaaaaa"); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			t.Cleanup(cancel)
			done := make(chan error, 1)
			go func() { done <- p.SeekContext(ctx, 10*time.Second) }()
			waitRecoverySignal(t, entered)
			cancel()
			if err := <-done; !errors.Is(err, context.Canceled) {
				t.Fatalf("seek = %v, want cancellation", err)
			}
			switch action {
			case "stop":
				if err := p.Stop(); err != nil {
					t.Fatal(err)
				}
			case "account switch":
				p.configure = func(context.Context, AccountSession) error { return nil }
				if err := p.SetAccount(context.Background(), AccountSession{Revision: 1}); err != nil {
					t.Fatal(err)
				}
			case "new load":
				p.resolve = func(context.Context, string) (media, error) { return media{}, context.Canceled }
				if err := p.Load(context.Background(), "bbbbbbbbbbb"); !errors.Is(err, context.Canceled) {
					t.Fatalf("replacement load = %v, want cancellation", err)
				}
			case "terminal failure":
				p.refresh = func(context.Context) error { return errors.New("audio_network_timeout") }
				if err := p.Seek(30 * time.Second); err == nil || err.Error() != "audio_network_timeout" {
					t.Fatalf("replacement seek lost its terminal error: %v", err)
				}
			}
			if err := p.Seek(43 * time.Second); !errors.Is(err, ErrNoTrack) {
				t.Fatalf("seek revived a track after %s: %v", action, err)
			}
		})
	}
}
