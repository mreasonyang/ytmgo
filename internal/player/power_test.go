package player

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestSuspendRetiresMediaAndResolvesFreshAtBookmark(t *testing.T) {
	p := testPlayer()
	defer p.Close()
	var resolves, releases atomic.Int32
	var streams []*zeroStream
	var opened []time.Duration
	p.resolve = func(context.Context, string) (media, error) {
		resolves.Add(1)
		return media{Duration: 60 * time.Second, release: func() { releases.Add(1) }}, nil
	}
	p.open = func(_ context.Context, _ media, at time.Duration) (*pcmDecoder, error) {
		opened = append(opened, at)
		d, s := zeroDecoder(60)
		streams = append(streams, s)
		return d, nil
	}
	if err := p.Load(context.Background(), "aaaaaaaaaaa"); err != nil {
		t.Fatal(err)
	}
	if err := p.Seek(23 * time.Second); err != nil {
		t.Fatal(err)
	}
	if err := p.Prepare(context.Background(), "bbbbbbbbbbb"); err != nil {
		t.Fatal(err)
	}
	if err := p.Suspend(); err != nil {
		t.Fatal(err)
	}
	before := p.Snapshot()
	if before.Position < 23*time.Second || before.Playing || before.Loading || before.Ended || before.Error != "" {
		t.Fatal("suspend lost the bookmark or left output active")
	}
	if err := p.Prepare(context.Background(), "ccccccccccc"); err != nil {
		t.Fatal(err)
	}
	if resolves.Load() != 2 {
		t.Fatal("sleep started another prefetch")
	}
	if err := p.Suspend(); err != nil {
		t.Fatal(err)
	}
	if err := p.ResumeFromSuspend(context.Background()); err != nil {
		t.Fatal(err)
	}
	waitPlayerCondition(t, func() bool { return releases.Load() == 2 })
	if resolves.Load() != 3 || len(opened) != 4 || opened[3] != before.Position {
		t.Fatal("resume reused stale media or restarted at the wrong position")
	}
	for _, s := range streams[:3] {
		if !s.closed.Load() {
			t.Fatal("suspend retained an old decoder")
		}
	}
	if after := p.Snapshot(); !after.Playing || after.Position < before.Position || after.Error != "" {
		t.Fatal("fresh decoder did not restore playback")
	}
	if err := p.ResumeFromSuspend(context.Background()); err != nil || resolves.Load() != 3 {
		t.Fatal("duplicate wake opened another decoder")
	}
}

func TestSuspendPreservesPausedIntentAndSeek(t *testing.T) {
	p := testPlayer()
	defer p.Close()
	if err := p.Load(context.Background(), "aaaaaaaaaaa"); err != nil {
		t.Fatal(err)
	}
	p.Pause()
	if err := p.Seek(18 * time.Second); err != nil {
		t.Fatal(err)
	}
	if err := p.Suspend(); err != nil {
		t.Fatal(err)
	}
	if err := p.ResumeFromSuspend(context.Background()); err != nil {
		t.Fatal(err)
	}
	if s := p.Snapshot(); s.Playing || s.Position != 18*time.Second {
		t.Fatal("wake started a paused track or moved its bookmark")
	}
	p.Suspend()
	if err := p.Seek(37 * time.Second); err != nil {
		t.Fatal(err)
	}
	if s := p.Snapshot(); s.Playing || s.Position != 37*time.Second {
		t.Fatal("suspended seek lost paused intent")
	}
	if err := p.Play(); err != nil || !p.Snapshot().Playing {
		t.Fatal("paused wake could not subsequently play")
	}
}

func TestCanceledWakeRetainsBookmarkForExplicitPlay(t *testing.T) {
	p := testPlayer()
	defer p.Close()
	if err := p.Load(context.Background(), "aaaaaaaaaaa"); err != nil {
		t.Fatal(err)
	}
	p.Pause()
	p.Seek(29 * time.Second)
	p.Suspend()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := p.ResumeFromSuspend(ctx); !errors.Is(err, context.Canceled) {
		t.Fatal("canceled wake was accepted")
	}
	if err := p.Play(); err != nil {
		t.Fatal(err)
	}
	if s := p.Snapshot(); !s.Playing || s.Position < 29*time.Second {
		t.Fatal("explicit play lost the suspended bookmark")
	}
}

func TestLateWakeLosesToUserActions(t *testing.T) {
	for _, action := range []string{"pause", "load", "stop", "close", "suspend"} {
		t.Run(action, func(t *testing.T) {
			p := testPlayer()
			defer p.Close()
			if err := p.Load(context.Background(), "aaaaaaaaaaa"); err != nil {
				t.Fatal(err)
			}
			p.Suspend()
			entered, finish := make(chan struct{}), make(chan struct{})
			var released atomic.Int32
			var late *zeroStream
			p.open = func(_ context.Context, _ media, _ time.Duration) (*pcmDecoder, error) {
				close(entered)
				<-finish
				d, s := zeroDecoder(60)
				late = s
				return d, nil
			}
			p.resolve = func(context.Context, string) (media, error) {
				return media{Duration: 60 * time.Second, release: func() { released.Add(1) }}, nil
			}
			done := make(chan error, 1)
			go func() { done <- p.ResumeFromSuspend(context.Background()) }()
			<-entered
			closed := make(chan struct{})
			switch action {
			case "pause":
				p.Pause()
			case "load":
				p.open = func(context.Context, media, time.Duration) (*pcmDecoder, error) {
					d, _ := zeroDecoder(60)
					return d, nil
				}
				if err := p.Load(context.Background(), "bbbbbbbbbbb"); err != nil {
					t.Fatal(err)
				}
			case "stop":
				p.Stop()
			case "close":
				go func() { p.Close(); close(closed) }()
				waitPlayerCondition(t, func() bool { p.mu.Lock(); defer p.mu.Unlock(); return p.closed })
			case "suspend":
				p.Suspend()
			}
			close(finish)
			err := <-done
			if action == "pause" {
				if err != nil || p.Snapshot().Playing {
					t.Fatal("late wake ignored pause")
				}
			} else {
				if !errors.Is(err, context.Canceled) || !late.closed.Load() || released.Load() != 1 {
					t.Fatal("superseded wake published or retained stale media")
				}
			}
			if action == "load" && (p.videoID != "bbbbbbbbbbb" || !p.Snapshot().Playing) {
				t.Fatal("late wake replaced the newer track")
			}
			if action == "close" {
				<-closed
			}
		})
	}
}

func TestWakeRetriesOnlyTransientTransportFailure(t *testing.T) {
	for _, category := range []string{"audio_connection_failed", "login_required", "rate_limited"} {
		t.Run(category, func(t *testing.T) {
			p := testPlayer()
			defer p.Close()
			p.Load(context.Background(), "aaaaaaaaaaa")
			p.Suspend()
			calls := 0
			p.resolve = func(context.Context, string) (media, error) {
				calls++
				if calls == 1 {
					return media{}, errors.New(category)
				}
				return media{Duration: time.Minute}, nil
			}
			err := p.ResumeFromSuspend(context.Background())
			if category == "audio_connection_failed" {
				if err != nil || calls != 2 || !p.Snapshot().Playing {
					t.Fatal("wake did not recover a transient transport failure")
				}
			} else if err == nil || calls != 1 || p.Snapshot().Playing {
				t.Fatal("wake retried a permanent failure")
			}
		})
	}
}

func TestWakeBookmarkCannotCrossAccountIdentity(t *testing.T) {
	p := testPlayer()
	defer p.Close()
	p.Load(context.Background(), "aaaaaaaaaaa")
	p.Suspend()
	p.configure = func(context.Context, AccountSession) error { return nil }
	if err := p.SetAccount(context.Background(), AccountSession{Revision: 1}); err != nil {
		t.Fatal(err)
	}
	if err := p.ResumeFromSuspend(context.Background()); err != nil || p.Snapshot().Playing || p.suspended != nil {
		t.Fatal("wake revived an account's revoked track")
	}
}

func TestWakeDoesNotHideAPreexistingFailure(t *testing.T) {
	p := testPlayer()
	defer p.Close()
	p.Load(context.Background(), "aaaaaaaaaaa")
	p.mu.Lock()
	p.state.Error = "audio_stream_failed"
	p.mu.Unlock()
	p.Suspend()
	if err := p.ResumeFromSuspend(context.Background()); err == nil || p.Snapshot().Playing || p.Snapshot().Error != "audio_stream_failed" {
		t.Fatal("wake hid a failure that preceded sleep")
	}
}
