package player

import (
	"context"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"ytmgo/internal/audioquality"
)

func TestAudioQualityReplacesPreloadForSameTrack(t *testing.T) {
	const current, next = "aaaaaaaaaaa", "bbbbbbbbbbb"
	ctx := context.Background()
	p := testPlayer()
	defer p.Close()
	retiring, gate := make(chan struct{}), make(chan struct{})
	var release sync.Once
	releaseOld := func() { release.Do(func() { close(gate) }) }
	defer releaseOld()
	var closed atomic.Bool
	var resolved []audioTrack
	p.resolve = func(_ context.Context, track audioTrack) (media, error) {
		resolved = append(resolved, track)
		m := media{Duration: time.Minute}
		if track.id == next && track.quality == audioquality.Best {
			m.release = func() { close(retiring); <-gate; closed.Store(true) }
		}
		if track.quality == audioquality.Saver && !closed.Load() {
			t.Error("replacement opened before the old prefetch was retired")
		}
		return m, nil
	}
	if err := p.Load(ctx, current); err != nil {
		t.Fatal(err)
	}
	if err := p.Pause(); err != nil {
		t.Fatal(err)
	}
	session := p.session
	if err := p.Prepare(ctx, next); err != nil {
		t.Fatal(err)
	}
	p.SetAudioQuality(audioquality.Standard)
	select {
	case <-retiring:
	case <-time.After(3 * time.Second):
		t.Fatal("old prefetch was retained")
	}
	p.SetAudioQuality(audioquality.Saver)
	done := make(chan error, 1)
	go func() { done <- p.Prepare(ctx, next) }()
	waitPlayerCondition(t, func() bool { p.mu.Lock(); defer p.mu.Unlock(); return p.prepared != nil })
	if p.session != session || p.Snapshot().Playing {
		t.Fatal("preference change disturbed the paused track")
	}
	releaseOld()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	p.SetAudioQuality(audioquality.Saver) // Selecting the same quality keeps this prefetch.
	if err := p.Load(ctx, next); err != nil {
		t.Fatal(err)
	}
	want := []audioTrack{{current, audioquality.Best}, {next, audioquality.Best}, {next, audioquality.Saver}}
	if !slices.Equal(resolved, want) {
		t.Fatalf("resolved %v, want %v", resolved, want)
	}
}

func TestAudioQualityRetainedByRecoveryAndSleep(t *testing.T) {
	const id = "aaaaaaaaaaa"
	ctx := context.Background()
	p := testPlayer()
	defer p.Close()
	resolved := make(chan audioquality.Quality, 5)
	p.resolve = func(_ context.Context, track audioTrack) (media, error) {
		resolved <- track.quality
		return media{Duration: time.Minute}, nil
	}
	p.SetAudioQuality(audioquality.Standard)
	if err := p.Load(ctx, id); err != nil {
		t.Fatal(err)
	}
	p.SetAudioQuality(audioquality.Saver)
	if err := p.Seek(9 * time.Second); err != nil {
		t.Fatal(err)
	}
	denyCurrentDecoder(p)
	p.Snapshot()
	waitPlayerCondition(t, func() bool { return !p.Snapshot().Loading })
	if err := p.Suspend(); err != nil {
		t.Fatal(err)
	}
	if err := p.ResumeFromSuspend(ctx); err != nil {
		t.Fatal(err)
	}
	if err := p.Load(ctx, id); err != nil {
		t.Fatal(err)
	}
	close(resolved)
	var got []audioquality.Quality
	for q := range resolved {
		got = append(got, q)
	}
	want := []audioquality.Quality{audioquality.Standard, audioquality.Standard, audioquality.Standard, audioquality.Saver}
	if !slices.Equal(got, want) {
		t.Fatalf("resolved %v, want %v", got, want)
	}
}
