package player

import (
	"bytes"
	"context"
	"errors"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/egoist/mygo/audio"
)

func TestMediaURLBoundary(t *testing.T) {
	valid := "http://127.0.0.1:43210/" + strings.Repeat("a", 64)
	if !validMediaURL(valid) {
		t.Fatal("private loopback capability rejected")
	}
	for _, raw := range []string{
		"https://rr1.googlevideo.com/videoplayback?x=1", "http://localhost:43210/" + strings.Repeat("a", 64),
		"http://127.0.0.1/" + strings.Repeat("a", 64), valid + "?x=1", valid + "#x", valid + "/",
		"http://user:pass@127.0.0.1:43210/" + strings.Repeat("a", 64), "file:///tmp/x",
		"http://127.0.0.1:43210/" + strings.Repeat("A", 64), "http://127.0.0.1:43210/short",
	} {
		if validMediaURL(raw) {
			t.Fatal("non-capability media URL accepted")
		}
	}
}

func TestResolverHeaderBoundary(t *testing.T) {
	for _, value := range []string{"Mozilla/5.0 (Macintosh; Intel Mac OS X)", "https://www.youtube.com/", ""} {
		if !validHeaderValue(value) {
			t.Fatal("valid resolver header rejected")
		}
	}
	for _, value := range []string{"agent\r\noption injected", "agent'", "agent\\", strings.Repeat("x", 1025)} {
		if validHeaderValue(value) {
			t.Fatal("invalid resolver header accepted")
		}
	}
}

func TestErrorsNeverContainDependencyDetails(t *testing.T) {
	for _, err := range []error{errors.New("https://example.test/?signed=secret-value"), errors.New("exit status 1"), context.DeadlineExceeded} {
		if strings.Contains(safeError(err), "secret") || strings.Contains(safeError(err), ":") {
			t.Fatal("raw dependency error escaped")
		}
	}
}

func TestDecoderDiagnosticsExposeOnlyCategories(t *testing.T) {
	d := &decoderDiagnostics{}
	_, _ = d.Write([]byte("https://cdn.example.test/?synthetic=403-secret\n"))
	if got := d.result().Error(); got != "audio_stream_failed" {
		t.Fatal("a URL substring was misclassified as an HTTP status")
	}
	_, _ = d.Write([]byte("HTTP error 403 For"))
	_, _ = d.Write([]byte("bidden https://cdn.example.test/?synthetic=private\n"))
	if got := d.result().Error(); got != "audio_access_denied" {
		t.Fatal("HTTP error was not reduced to a fixed category")
	}
	if d.tail != "" {
		t.Fatal("raw diagnostic tail retained after classification")
	}
}

type zeroStream struct {
	n      atomic.Int64
	closed atomic.Bool
}

func (s *zeroStream) Read(b []byte) (int, error) {
	if s.closed.Load() {
		return 0, io.EOF
	}
	clear(b)
	s.n.Add(int64(len(b)))
	return len(b), nil
}
func (s *zeroStream) Close() error { s.closed.Store(true); return nil }
func zeroDecoder(seconds int) (*pcmDecoder, *zeroStream) {
	s := &zeroStream{}
	done := make(chan error)
	close(done)
	return &pcmDecoder{reader: s, input: s, cancel: func() {}, wait: done, length: int64(seconds * sampleRate)}, s
}
func testPlayer() *Player {
	p := &Player{state: State{Volume: 0}}
	p.resolve = func(context.Context, string) (media, error) { return media{Duration: 60 * time.Second}, nil }
	p.open = func(context.Context, media, time.Duration) (*pcmDecoder, error) {
		d, _ := zeroDecoder(60)
		return d, nil
	}
	return p
}

func waitPlayerCondition(t *testing.T, ready func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for !ready() {
		if time.Now().After(deadline) {
			t.Fatal("player lifecycle transition timed out")
		}
		time.Sleep(time.Millisecond)
	}
}

func TestPreparedTrackTransfersDecoderAndLease(t *testing.T) {
	p := testPlayer()
	defer p.Close()
	var resolves, opens, releasedA, releasedB atomic.Int64
	var preparedDecoder *pcmDecoder
	var preparedStream *zeroStream
	p.resolve = func(_ context.Context, id string) (media, error) {
		if id == "bbbbbbbbbbb" {
			resolves.Add(1)
		}
		release := func() { releasedA.Add(1) }
		if id == "bbbbbbbbbbb" {
			release = func() { releasedB.Add(1) }
		}
		return media{URL: id, Duration: 60 * time.Second, release: release}, nil
	}
	p.open = func(_ context.Context, info media, _ time.Duration) (*pcmDecoder, error) {
		d, stream := zeroDecoder(60)
		if info.URL == "bbbbbbbbbbb" {
			opens.Add(1)
			preparedDecoder, preparedStream = d, stream
		}
		return d, nil
	}
	if err := p.Load(context.Background(), "aaaaaaaaaaa"); err != nil {
		t.Fatal(err)
	}
	p.Pause()
	current := p.session
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := p.Prepare(ctx, "bbbbbbbbbbb"); err != nil {
		t.Fatal(err)
	}
	if p.session != current || p.Snapshot().Playing || preparedStream.n.Load() != 0 {
		t.Fatal("preparation changed audible state or consumed unbounded PCM")
	}
	if err := p.Load(context.Background(), "bbbbbbbbbbb"); err != nil {
		t.Fatal(err)
	}
	if p.session.decoder != preparedDecoder || resolves.Load() != 1 || opens.Load() != 1 {
		t.Fatal("prepared load repeated resolution or decoder creation")
	}
	cancel() // The consumed preparation no longer owns the live decoder.
	p.Pause()
	if err := p.Seek(10 * time.Second); err != nil {
		t.Fatal(err)
	}
	if releasedA.Load() != 1 || releasedB.Load() != 0 || p.Snapshot().Position != 10*time.Second {
		t.Fatal("seek did not retain the current media lease")
	}
	p.Stop()
	p.Close()
	if releasedB.Load() != 1 {
		t.Fatal("current media lease was not released exactly once")
	}
}

func TestLoadJoinsPendingPreparation(t *testing.T) {
	p := testPlayer()
	defer p.Close()
	entered, gate := make(chan struct{}), make(chan struct{})
	var resolves atomic.Int64
	p.resolve = func(_ context.Context, id string) (media, error) {
		resolves.Add(1)
		return media{Duration: 60 * time.Second}, nil
	}
	p.open = func(ctx context.Context, _ media, _ time.Duration) (*pcmDecoder, error) {
		close(entered)
		select {
		case <-gate:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		d, _ := zeroDecoder(60)
		return d, nil
	}
	prepared, loaded := make(chan error, 1), make(chan error, 1)
	go func() { prepared <- p.Prepare(context.Background(), "bbbbbbbbbbb") }()
	<-entered
	go func() { loaded <- p.Load(context.Background(), "bbbbbbbbbbb") }()
	waitPlayerCondition(t, func() bool { p.mu.Lock(); defer p.mu.Unlock(); return p.prepared == nil })
	close(gate)
	if err := <-prepared; err != nil {
		t.Fatal(err)
	}
	if err := <-loaded; err != nil {
		t.Fatal(err)
	}
	if resolves.Load() != 1 {
		t.Fatal("pending preparation was resolved twice")
	}
}

func TestPreparedReplacementReleasesBeforeOpeningNext(t *testing.T) {
	p := testPlayer()
	defer p.Close()
	var released atomic.Int64
	var first *zeroStream
	p.resolve = func(_ context.Context, id string) (media, error) {
		return media{URL: id, Duration: 60 * time.Second, release: func() { released.Add(1) }}, nil
	}
	p.open = func(_ context.Context, info media, _ time.Duration) (*pcmDecoder, error) {
		if info.URL == "ccccccccccc" && (!first.closed.Load() || released.Load() != 1) {
			t.Error("replacement opened while previous prepared decoder was still retained")
		}
		d, stream := zeroDecoder(60)
		if info.URL == "bbbbbbbbbbb" {
			first = stream
		}
		return d, nil
	}
	if err := p.Prepare(context.Background(), "bbbbbbbbbbb"); err != nil {
		t.Fatal(err)
	}
	if err := p.Prepare(context.Background(), "ccccccccccc"); err != nil {
		t.Fatal(err)
	}
	p.ClearPrepared()
	p.Close()
	if released.Load() != 2 {
		t.Fatal("cleared preparations retained media leases")
	}
}

func TestPreparedContextCancellationReleasesReadyDecoder(t *testing.T) {
	p := testPlayer()
	defer p.Close()
	var released atomic.Int64
	var stream *zeroStream
	p.resolve = func(context.Context, string) (media, error) {
		return media{Duration: 60 * time.Second, release: func() { released.Add(1) }}, nil
	}
	p.open = func(context.Context, media, time.Duration) (*pcmDecoder, error) {
		d, s := zeroDecoder(60)
		stream = s
		return d, nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	if err := p.Prepare(ctx, "bbbbbbbbbbb"); err != nil {
		t.Fatal(err)
	}
	cancel()
	waitPlayerCondition(t, func() bool { return released.Load() == 1 })
	if !stream.closed.Load() {
		t.Fatal("canceled prepared decoder remained open")
	}
}

func TestSupersededClaimedPreparationCannotReplaceCurrentTrack(t *testing.T) {
	p := testPlayer()
	defer p.Close()
	entered, gate := make(chan struct{}), make(chan struct{})
	var released atomic.Int64
	var stale *zeroStream
	p.resolve = func(_ context.Context, id string) (media, error) {
		info := media{URL: id, Duration: 20 * time.Second}
		if id == "bbbbbbbbbbb" {
			info.Duration = 10 * time.Second
			info.release = func() { released.Add(1) }
		}
		return info, nil
	}
	p.open = func(_ context.Context, info media, _ time.Duration) (*pcmDecoder, error) {
		if info.URL == "bbbbbbbbbbb" {
			close(entered)
			<-gate
		}
		d, s := zeroDecoder(60)
		if info.URL == "bbbbbbbbbbb" {
			stale = s
		}
		return d, nil
	}
	prepared, loaded := make(chan error, 1), make(chan error, 1)
	go func() { prepared <- p.Prepare(context.Background(), "bbbbbbbbbbb") }()
	<-entered
	go func() { loaded <- p.Load(context.Background(), "bbbbbbbbbbb") }()
	waitPlayerCondition(t, func() bool { p.mu.Lock(); defer p.mu.Unlock(); return p.prepared == nil })
	if err := p.Load(context.Background(), "ccccccccccc"); err != nil {
		t.Fatal(err)
	}
	close(gate)
	if err := <-prepared; !errors.Is(err, context.Canceled) {
		t.Fatal("superseded preparation was not canceled")
	}
	if err := <-loaded; !errors.Is(err, context.Canceled) {
		t.Fatal("superseded prepared load was not canceled")
	}
	if p.Snapshot().Duration != 20*time.Second || !stale.closed.Load() {
		t.Fatal("late preparation damaged the current track")
	}
	p.Close()
	if released.Load() != 1 {
		t.Fatal("superseded preparation lease was not released exactly once")
	}
}

func TestPrepareFailureDoesNotChangePlaybackState(t *testing.T) {
	p := testPlayer()
	defer p.Close()
	if err := p.Load(context.Background(), "aaaaaaaaaaa"); err != nil {
		t.Fatal(err)
	}
	p.Pause()
	current := p.session
	p.resolve = func(context.Context, string) (media, error) { return media{}, errors.New("stream_resolution_failed") }
	if err := p.Prepare(context.Background(), "bbbbbbbbbbb"); err == nil {
		t.Fatal("failed preparation accepted")
	}
	state := p.Snapshot()
	if p.session != current || state.Error != "" || state.Loading || state.Playing {
		t.Fatal("preparation failure changed current playback")
	}
}

func TestSeekStopAndCloseClearPreparedResources(t *testing.T) {
	for _, action := range []string{"seek", "stop", "close"} {
		t.Run(action, func(t *testing.T) {
			p := testPlayer()
			defer p.Close()
			if err := p.Load(context.Background(), "aaaaaaaaaaa"); err != nil {
				t.Fatal(err)
			}
			var released atomic.Int64
			p.resolve = func(context.Context, string) (media, error) {
				return media{Duration: 60 * time.Second, release: func() { released.Add(1) }}, nil
			}
			if err := p.Prepare(context.Background(), "bbbbbbbbbbb"); err != nil {
				t.Fatal(err)
			}
			switch action {
			case "seek":
				if err := p.Seek(time.Second); err != nil {
					t.Fatal(err)
				}
			case "stop":
				p.Stop()
			case "close":
				p.Close()
			}
			waitPlayerCondition(t, func() bool { return released.Load() == 1 })
		})
	}
}

func TestCloseCancelsInflightPreparation(t *testing.T) {
	p := testPlayer()
	started := make(chan struct{})
	p.resolve = func(ctx context.Context, _ string) (media, error) {
		close(started)
		<-ctx.Done()
		return media{}, ctx.Err()
	}
	done := make(chan error, 1)
	go func() { done <- p.Prepare(context.Background(), "bbbbbbbbbbb") }()
	<-started
	p.Close()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatal("close did not cancel pending preparation")
	}
}

func TestLateLoadCannotReplaceCurrentTrack(t *testing.T) {
	p := testPlayer()
	defer p.Close()
	started, release := make(chan struct{}), make(chan struct{})
	p.resolve = func(ctx context.Context, id string) (media, error) {
		if id == "aaaaaaaaaaa" {
			close(started)
			<-release
			return media{Duration: 10 * time.Second}, nil
		}
		return media{Duration: 20 * time.Second}, nil
	}
	first := make(chan error, 1)
	go func() { first <- p.Load(context.Background(), "aaaaaaaaaaa") }()
	<-started
	if err := p.Load(context.Background(), "bbbbbbbbbbb"); err != nil {
		t.Fatal(err)
	}
	close(release)
	if err := <-first; !errors.Is(err, context.Canceled) {
		t.Fatalf("late load result = %v", err)
	}
	if p.Snapshot().Duration != 20*time.Second {
		t.Fatal("late load replaced current state")
	}
}

func TestAlreadyCanceledLoadCannotStopCurrentTrack(t *testing.T) {
	p := testPlayer()
	defer p.Close()
	if err := p.Load(context.Background(), "aaaaaaaaaaa"); err != nil {
		t.Fatal(err)
	}
	before := p.session
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := p.Load(ctx, "bbbbbbbbbbb"); !errors.Is(err, context.Canceled) {
		t.Fatal("already canceled load was accepted")
	}
	if p.session != before || p.Snapshot().Loading {
		t.Fatal("already canceled load replaced the current track")
	}
}

func TestCanceledQueuedSeekCannotAffectNewTrack(t *testing.T) {
	p := testPlayer()
	defer p.Close()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := p.Load(context.Background(), "bbbbbbbbbbb"); err != nil {
		t.Fatal(err)
	}
	p.Pause()
	current := p.session
	if err := p.SeekContext(ctx, 30*time.Second); !errors.Is(err, context.Canceled) {
		t.Fatal("canceled queued seek was accepted")
	}
	if p.session != current || p.Snapshot().Position != 0 {
		t.Fatal("old queued gesture sought the new track")
	}
}

func TestCanceledQueuedResumeCannotChangeCurrentPlayback(t *testing.T) {
	for _, action := range []string{"load", "seek", "pause"} {
		t.Run(action, func(t *testing.T) {
			p := testPlayer()
			defer p.Close()
			if err := p.Load(context.Background(), "aaaaaaaaaaa"); err != nil {
				t.Fatal(err)
			}
			p.Pause()
			p.mu.Lock()
			p.state.Ended = true
			p.mu.Unlock()
			ctx, cancel := context.WithCancel(context.Background())
			cancel() // The UI invalidates the queued automatic repeat before dispatch.
			switch action {
			case "load":
				if err := p.Load(context.Background(), "bbbbbbbbbbb"); err != nil {
					t.Fatal(err)
				}
				p.Pause()
			case "seek":
				if err := p.Seek(20 * time.Second); err != nil {
					t.Fatal(err)
				}
			case "pause":
				p.Pause()
			}
			current, before := p.session, p.Snapshot()
			if err := p.PlayContext(ctx); !errors.Is(err, context.Canceled) {
				t.Fatal("canceled resume was accepted")
			}
			after := p.Snapshot()
			if p.session != current || before.Position != after.Position || after.Playing || p.wantPlay {
				t.Fatal("old automatic repeat changed newer playback intent")
			}
		})
	}
}

func TestEndedRestartKeepsCapturedMediaAndLosesToNewLoad(t *testing.T) {
	p := testPlayer()
	defer p.Close()
	p.resolve = func(_ context.Context, id string) (media, error) {
		return media{URL: id, Duration: 60 * time.Second}, nil
	}
	if err := p.Load(context.Background(), "aaaaaaaaaaa"); err != nil {
		t.Fatal(err)
	}
	p.Pause()
	p.mu.Lock()
	p.state.Ended = true
	p.mu.Unlock()
	entered, gate := make(chan struct{}), make(chan struct{})
	var stale *zeroStream
	p.open = func(_ context.Context, info media, at time.Duration) (*pcmDecoder, error) {
		if info.URL == "aaaaaaaaaaa" {
			if at != 0 {
				t.Error("ended restart did not open at zero")
			}
			close(entered)
			<-gate
		}
		d, stream := zeroDecoder(60)
		if info.URL == "aaaaaaaaaaa" {
			stale = stream
		}
		return d, nil
	}
	done := make(chan error, 1)
	go func() { done <- p.PlayContext(context.Background()) }()
	<-entered
	if err := p.Load(context.Background(), "bbbbbbbbbbb"); err != nil {
		t.Fatal(err)
	}
	p.Pause()
	if err := p.Seek(25 * time.Second); err != nil {
		t.Fatal(err)
	}
	current := p.session
	close(gate)
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatal("superseded ended restart was accepted")
	}
	if p.session != current || p.media.URL != "bbbbbbbbbbb" || p.Snapshot().Position != 25*time.Second || p.Snapshot().Playing || !stale.closed.Load() {
		t.Fatal("late ended restart reset the replacement track")
	}
}

func TestEndedRestartReusesMediaAndRespectsPause(t *testing.T) {
	p := testPlayer()
	defer p.Close()
	var resolves atomic.Int64
	p.resolve = func(context.Context, string) (media, error) {
		resolves.Add(1)
		return media{Duration: 60 * time.Second}, nil
	}
	if err := p.Load(context.Background(), "aaaaaaaaaaa"); err != nil {
		t.Fatal(err)
	}
	p.Pause()
	p.mu.Lock()
	p.state.Ended = true
	p.mu.Unlock()
	entered, gate := make(chan struct{}), make(chan struct{})
	p.open = func(ctx context.Context, _ media, at time.Duration) (*pcmDecoder, error) {
		if at != 0 {
			t.Error("ended restart opened at a nonzero position")
		}
		close(entered)
		<-gate
		d, _ := zeroDecoder(60)
		return d, nil
	}
	done := make(chan error, 1)
	go func() { done <- p.PlayContext(context.Background()) }()
	<-entered
	p.Pause()
	close(gate)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	state := p.Snapshot()
	if state.Playing || state.Loading || state.Ended || state.Position != 0 || resolves.Load() != 1 {
		t.Fatal("ended restart did not preserve pause or unnecessarily re-resolved media")
	}
	current := p.session
	if err := p.PlayContext(context.Background()); err != nil {
		t.Fatal(err)
	}
	if p.session != current || !p.wantPlay {
		t.Fatal("normal paused resume recreated the decoder")
	}
}

func TestCanceledEndedRestartAfterOpenClearsLoading(t *testing.T) {
	p := testPlayer()
	defer p.Close()
	if err := p.Load(context.Background(), "aaaaaaaaaaa"); err != nil {
		t.Fatal(err)
	}
	p.Pause()
	p.mu.Lock()
	p.state.Ended = true
	p.mu.Unlock()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var rejected *zeroStream
	p.open = func(context.Context, media, time.Duration) (*pcmDecoder, error) {
		// Match a UI pause after the decoder has produced PCM, just before
		// install observes the operation's now-canceled context.
		d, stream := zeroDecoder(60)
		rejected = stream
		cancel()
		p.Pause()
		return d, nil
	}
	if err := p.PlayContext(ctx); !errors.Is(err, context.Canceled) {
		t.Fatal("canceled restart was installed")
	}
	state := p.Snapshot()
	if state.Loading || state.Playing || state.Error != "" || p.session != nil || !rejected.closed.Load() {
		t.Fatal("canceled installation retained loading state or its decoder")
	}
}

func TestLatePreparationDoesNotDuplicateForegroundLoad(t *testing.T) {
	p := testPlayer()
	defer p.Close()
	entered, gate := make(chan struct{}), make(chan struct{})
	var resolves atomic.Int64
	p.resolve = func(ctx context.Context, _ string) (media, error) {
		resolves.Add(1)
		close(entered)
		select {
		case <-gate:
		case <-ctx.Done():
			return media{}, ctx.Err()
		}
		return media{Duration: 60 * time.Second}, nil
	}
	done := make(chan error, 1)
	go func() { done <- p.Load(context.Background(), "bbbbbbbbbbb") }()
	<-entered
	if err := p.Prepare(context.Background(), "bbbbbbbbbbb"); err != nil {
		t.Fatal(err)
	}
	close(gate)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if resolves.Load() != 1 {
		t.Fatal("late scheduled preparation repeated foreground resolution")
	}
}

func TestRapidSeeksCancelOnlyTheOlderDecoder(t *testing.T) {
	p := testPlayer()
	defer p.Close()
	if err := p.Load(context.Background(), "aaaaaaaaaaa"); err != nil {
		t.Fatal(err)
	}
	p.Pause()
	entered, release := make(chan struct{}), make(chan struct{})
	var superseded *zeroStream
	p.open = func(ctx context.Context, _ media, at time.Duration) (*pcmDecoder, error) {
		if at == 10*time.Second {
			close(entered)
			<-release
			d, stream := zeroDecoder(60)
			superseded = stream
			return d, nil
		}
		d, _ := zeroDecoder(60)
		return d, nil
	}
	older := make(chan error, 1)
	go func() { older <- p.Seek(10 * time.Second) }()
	<-entered
	if err := p.Seek(20 * time.Second); err != nil {
		t.Fatal("newer seek was rejected")
	}
	close(release)
	if err := <-older; !errors.Is(err, context.Canceled) {
		t.Fatal("superseded seek was not canceled")
	}
	if !superseded.closed.Load() {
		t.Fatal("superseded decoder was not closed")
	}
	state := p.Snapshot()
	if state.Position != 20*time.Second || state.Playing || state.Loading || state.Error != "" {
		t.Fatal("old seek damaged newer state")
	}
}

func TestConcurrentSeeksAllReachReplacement(t *testing.T) {
	p := testPlayer()
	defer p.Close()
	if err := p.Load(context.Background(), "aaaaaaaaaaa"); err != nil {
		t.Fatal(err)
	}
	p.Pause()
	const count = 32
	var entered atomic.Int64
	gate := make(chan struct{})
	p.open = func(ctx context.Context, _ media, _ time.Duration) (*pcmDecoder, error) {
		entered.Add(1)
		<-gate
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		d, _ := zeroDecoder(60)
		return d, nil
	}
	var work sync.WaitGroup
	start := make(chan struct{})
	for i := range count {
		work.Add(1)
		go func() { defer work.Done(); <-start; _ = p.Seek(time.Duration(i) * time.Second) }()
	}
	close(start)
	deadline := time.Now().Add(time.Second)
	for entered.Load() < count && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	close(gate)
	work.Wait()
	if entered.Load() != count {
		t.Fatal("concurrent seek was rejected before replacing the older operation")
	}
	if state := p.Snapshot(); state.Loading || state.Error != "" {
		t.Fatal("concurrent seek left invalid state")
	}
}

func TestPauseSeekStopCloseLifecycle(t *testing.T) {
	p := testPlayer()
	var seeks []time.Duration
	var streams []*zeroStream
	p.open = func(ctx context.Context, m media, at time.Duration) (*pcmDecoder, error) {
		d, s := zeroDecoder(60)
		seeks = append(seeks, at)
		streams = append(streams, s)
		return d, nil
	}
	if err := p.Load(context.Background(), "aaaaaaaaaaa"); err != nil {
		t.Fatal(err)
	}
	if err := p.Pause(); err != nil {
		t.Fatal(err)
	}
	if err := p.Seek(30 * time.Second); err != nil {
		t.Fatal(err)
	}
	s := p.Snapshot()
	if s.Playing || s.Position != 30*time.Second || seeks[1] != 30*time.Second {
		t.Fatal("paused seek state incorrect", s)
	}
	if !streams[0].closed.Load() {
		t.Fatal("replaced decoder not closed")
	}
	if err := p.Play(); err != nil {
		t.Fatal(err)
	}
	if err := p.Stop(); err != nil {
		t.Fatal(err)
	}
	if !streams[1].closed.Load() || p.Snapshot().Position != 0 {
		t.Fatal("stop retained stream")
	}
	p.Close()
	p.Close()
	if err := p.Play(); err != ErrClosed {
		t.Fatal("closed player accepted play")
	}
}

func TestPausedStreamHasBoundedReadAhead(t *testing.T) {
	d, s := zeroDecoder(24 * 60 * 60)
	out := audio.NewPlayer(d)
	defer func() { out.Close(); d.Close() }()
	time.Sleep(100 * time.Millisecond)
	if n := s.n.Load(); n == 0 || n > sampleRate*channels*4+16384 {
		t.Fatalf("read ahead is not bounded to about one second: %d", n)
	}
	first := s.n.Load()
	time.Sleep(100 * time.Millisecond)
	if s.n.Load() != first {
		t.Fatal("paused decoder kept buffering")
	}
}

func TestCloseCancelsInflightResolution(t *testing.T) {
	p := testPlayer()
	started := make(chan struct{})
	p.resolve = func(ctx context.Context, _ string) (media, error) {
		close(started)
		<-ctx.Done()
		return media{}, ctx.Err()
	}
	var wg sync.WaitGroup
	wg.Add(1)
	go func() { defer wg.Done(); _ = p.Load(context.Background(), "aaaaaaaaaaa") }()
	<-started
	p.Close()
	wg.Wait()
	if p.Snapshot().Loading {
		t.Fatal("closed player still loading")
	}
}

func TestCloseUnblocksStalledPCMRead(t *testing.T) {
	rd, wr := io.Pipe()
	defer wr.Close()
	wait := make(chan error)
	close(wait)
	d := &pcmDecoder{reader: rd, input: rd, cancel: func() {}, wait: wait, length: sampleRate}
	read := make(chan struct{})
	go func() {
		_, _ = d.Read(make([]float32, 4096))
		close(read)
	}()
	d.Close()
	select {
	case <-read:
	case <-time.After(time.Second):
		t.Fatal("closing the decoder left its PCM reader blocked")
	}
}

func TestSuccessfulDecoderExitStillDetectsTruncatedPCM(t *testing.T) {
	for _, tc := range []struct {
		name   string
		frames int
		failed bool
	}{
		{"complete", sampleRate * 2, false},
		{"codec_padding", sampleRate*2 - 720, false},
		{"truncated", sampleRate, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			input := io.NopCloser(bytes.NewReader(make([]byte, tc.frames*channels*4)))
			done := make(chan error)
			close(done)
			d := &pcmDecoder{reader: input, input: input, cancel: func() {}, wait: done, length: sampleRate * 2, checkLength: true}
			defer d.Close()
			for {
				if _, err := d.Read(make([]float32, 4096)); err != nil {
					break
				}
			}
			if !d.eof.Load() || d.failed.Load() != tc.failed {
				t.Fatal("duration validation misclassified stream")
			}
		})
	}
}

func TestFFmpegActualSeek(t *testing.T) {
	if os.Getenv("YTMGO_RUNTIME_DIR") == "" {
		t.Skip("set YTMGO_RUNTIME_DIR to test bundled FFmpeg")
	}
	r := runtimePaths{ffmpeg: filepath.Join(os.Getenv("YTMGO_RUNTIME_DIR"), "ffmpeg")}
	fixture, err := os.ReadFile("testdata/seek.m4a")
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		http.ServeContent(w, request, "seek.m4a", time.Time{}, bytes.NewReader(fixture))
	}))
	defer server.Close()
	for _, test := range []struct {
		at time.Duration
		hz float64
	}{{0, 440}, {2500 * time.Millisecond, 880}} {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		d, err := startDecoder(ctx, r.ffmpeg, media{URL: server.URL + "/" + strings.Repeat("a", 64), Duration: 4 * time.Second}, test.at)
		if err != nil {
			cancel()
			t.Fatal(err)
		}
		var last float32
		crossings, samples := 0, 0
		buf := make([]float32, 4096)
		for samples < sampleRate {
			n, err := d.Read(buf)
			for i := 0; i < n; i += 2 {
				if last <= 0 && buf[i] > 0 {
					crossings++
				}
				last = buf[i]
				samples++
			}
			if err != nil {
				break
			}
		}
		d.Close()
		cancel()
		frequency := float64(crossings) * sampleRate / float64(samples)
		if math.Abs(frequency-test.hz) > 5 {
			t.Fatalf("seek %.0fs decoded %.1fHz, want %.0fHz", test.at.Seconds(), frequency, test.hz)
		}
	}
}

func TestFFmpegEndedRestartLocal(t *testing.T) {
	if os.Getenv("YTMGO_RUNTIME_DIR") == "" {
		t.Skip("set YTMGO_RUNTIME_DIR for actual FFmpeg EOF/restart")
	}
	r := runtimePaths{ffmpeg: filepath.Join(os.Getenv("YTMGO_RUNTIME_DIR"), "ffmpeg")}
	fixture, err := os.ReadFile("testdata/seek.m4a")
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		http.ServeContent(w, request, "seek.m4a", time.Time{}, bytes.NewReader(fixture))
	}))
	defer server.Close()
	p := testPlayer()
	defer p.Close()
	var resolves atomic.Int64
	p.resolve = func(context.Context, string) (media, error) {
		resolves.Add(1)
		return media{URL: server.URL + "/" + strings.Repeat("a", 64), Duration: 4 * time.Second, UserAgent: "fixture"}, nil
	}
	p.open = r.open
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := p.Load(ctx, "aaaaaaaaaaa"); err != nil {
		t.Fatal(safeError(err))
	}
	deadline := time.NewTimer(6 * time.Second)
	defer deadline.Stop()
	for !p.Snapshot().Ended {
		if p.Snapshot().Error != "" {
			t.Fatal("fixture_audio_failed")
		}
		select {
		case <-deadline.C:
			t.Fatal("fixture_eof_timeout")
		case <-time.After(20 * time.Millisecond):
		}
	}
	if err := p.PlayContext(ctx); err != nil {
		t.Fatal(safeError(err))
	}
	time.Sleep(600 * time.Millisecond)
	state := p.Snapshot()
	if !state.Playing || state.Ended || state.Position < 300*time.Millisecond || state.Position > 2*time.Second || resolves.Load() != 1 {
		t.Fatal("fixture_ended_restart_failed")
	}
	p.Pause()
}

func TestFFmpegRetriesSameStreamAndHonorsDeadline(t *testing.T) {
	if os.Getenv("YTMGO_RUNTIME_DIR") == "" {
		t.Skip("set YTMGO_RUNTIME_DIR to test bundled FFmpeg")
	}
	executable := filepath.Join(os.Getenv("YTMGO_RUNTIME_DIR"), "ffmpeg")
	fixture, err := os.ReadFile("testdata/seek.m4a")
	if err != nil {
		t.Fatal(err)
	}
	t.Run("temporary_server_failure", func(t *testing.T) {
		var requests atomic.Int64
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("User-Agent") != "ytmgo-test-agent" {
				w.WriteHeader(http.StatusForbidden)
				return
			}
			if requests.Add(1) == 1 {
				w.WriteHeader(http.StatusServiceUnavailable)
				return
			}
			w.Header().Set("Content-Type", "audio/mp4")
			http.ServeContent(w, r, "seek.m4a", time.Time{}, bytes.NewReader(fixture))
		}))
		defer server.Close()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		d, err := startDecoder(ctx, executable, media{URL: server.URL + "/" + strings.Repeat("a", 64), Duration: 4 * time.Second, UserAgent: "ytmgo-test-agent"}, 0)
		if err != nil {
			t.Fatal(err)
		}
		d.Close()
		if requests.Load() < 2 {
			t.Fatal("temporary server error was not retried")
		}
	})
	t.Run("access_denied_is_final", func(t *testing.T) {
		var requests atomic.Int64
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requests.Add(1)
			w.WriteHeader(http.StatusForbidden)
		}))
		defer server.Close()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if d, err := startDecoder(ctx, executable, media{URL: server.URL + "/" + strings.Repeat("a", 64), Duration: 4 * time.Second}, 0); err == nil {
			d.Close()
			t.Fatal("access denial was accepted")
		}
		if requests.Load() != 1 {
			t.Fatal("access denial was retried")
		}
	})
	t.Run("stalled_read_cancels_and_reaps", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			<-r.Context().Done()
		}))
		defer server.Close()
		ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
		defer cancel()
		started := time.Now()
		_, err := startDecoder(ctx, executable, media{URL: server.URL + "/" + strings.Repeat("a", 64), Duration: 4 * time.Second}, 0)
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatal("stalled decoder did not respect the readiness deadline")
		}
		if time.Since(started) > 3*time.Second {
			t.Fatal("decoder cancellation did not reap promptly")
		}
	})
}

func TestNativePlaybackAcceptanceLive(t *testing.T) {
	if os.Getenv("YTMGO_LIVE_ACCEPTANCE") != "1" {
		t.Skip("set YTMGO_LIVE_ACCEPTANCE=1 for three-load and 30-second native acceptance")
	}
	id := os.Getenv("YTMGO_LIVE_VIDEO_ID")
	if id == "" {
		id = "4D7u5KF7SP8"
	}
	p, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	initializeAnonymousLive(t, p)
	p.SetVolume(.05)
	resolve, open := p.resolve, p.open
	var cycle int
	p.resolve = func(ctx context.Context, id string) (media, error) {
		at := time.Now()
		info, err := resolve(ctx, id)
		t.Logf("attempt=%d phase=resolve seconds=%.2f success=%t", cycle, time.Since(at).Seconds(), err == nil)
		return info, err
	}
	p.open = func(ctx context.Context, info media, at time.Duration) (*pcmDecoder, error) {
		start := time.Now()
		d, err := open(ctx, info, at)
		category := "ok"
		if err != nil {
			category = safeError(err)
		}
		t.Logf("attempt=%d phase=first_pcm offset_seconds=%.2f seconds=%.2f category=%s", cycle, at.Seconds(), time.Since(start).Seconds(), category)
		return d, err
	}
	for attempt := 1; attempt <= 3; attempt++ {
		cycle = attempt
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		start := time.Now()
		err := p.Load(ctx, id)
		cancel()
		if err != nil {
			t.Fatalf("attempt=%d category=%s", cycle, safeError(err))
		}
		t.Logf("attempt=%d load_seconds=%.2f", cycle, time.Since(start).Seconds())
		if cycle < 3 {
			time.Sleep(time.Second)
			p.Stop()
		}
	}
	before := p.Snapshot().Position
	for range 60 {
		time.Sleep(500 * time.Millisecond)
		state := p.Snapshot()
		if !state.Playing || state.Error != "" {
			t.Fatalf("sustained playback failed: %+v", state)
		}
	}
	played := p.Snapshot().Position - before
	if played < 28*time.Second {
		t.Fatalf("playback did not sustain for 30 seconds: %.2fs", played.Seconds())
	}
	p.Pause()
	time.Sleep(100 * time.Millisecond)
	position := p.Snapshot().Position
	time.Sleep(500 * time.Millisecond)
	if p.Snapshot().Position-position > 50*time.Millisecond {
		t.Fatal("pause did not hold")
	}
	if err := p.Seek(60 * time.Second); err != nil {
		t.Fatal(err)
	}
	if state := p.Snapshot(); state.Playing || state.Position < 60*time.Second {
		t.Fatal("paused seek failed")
	}
	p.Play()
	time.Sleep(2 * time.Second)
	if state := p.Snapshot(); !state.Playing || state.Position < 61*time.Second || state.Error != "" {
		t.Fatal("seeked resume failed")
	}
	p.Stop()
	t.Logf("three consecutive loads, %.2fs sustained native playback, pause, 60s seek, resume, stop and close passed", played.Seconds())
}

// Explicitly gated: verifies an actual EOF restart through the same media
// lease and native output, without a second resolver request.
func TestNativeEndedRestartLive(t *testing.T) {
	if os.Getenv("YTMGO_LIVE_ENDED") != "1" {
		t.Skip("set YTMGO_LIVE_ENDED=1 for a short native end-and-restart check")
	}
	p, err := New()
	if err != nil {
		t.Fatal(safeError(err))
	}
	defer p.Close()
	initializeAnonymousLive(t, p)
	p.SetVolume(.02)
	var resolves atomic.Int64
	resolve := p.resolve
	p.resolve = func(ctx context.Context, id string) (media, error) {
		resolves.Add(1)
		return resolve(ctx, id)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	if err := p.Load(ctx, "52fNscjkST4"); err != nil {
		t.Fatal(safeError(err))
	}
	if err := p.SeekContext(ctx, p.Snapshot().Duration-500*time.Millisecond); err != nil {
		t.Fatal(safeError(err))
	}
	deadline := time.NewTimer(6 * time.Second)
	defer deadline.Stop()
	for !p.Snapshot().Ended {
		if p.Snapshot().Error != "" {
			t.Fatal("audio_stream_failed")
		}
		select {
		case <-deadline.C:
			t.Fatal("native_eof_timeout")
		case <-ctx.Done():
			t.Fatal("playback_timeout")
		case <-time.After(20 * time.Millisecond):
		}
	}
	started := time.Now()
	if err := p.PlayContext(ctx); err != nil {
		t.Fatal(safeError(err))
	}
	restart := time.Since(started)
	time.Sleep(time.Second)
	state := p.Snapshot()
	if !state.Playing || state.Ended || state.Position < 500*time.Millisecond || state.Position > 3*time.Second || state.Error != "" || resolves.Load() != 1 {
		t.Fatal("native_ended_restart_failed")
	}
	p.Pause()
	t.Logf("native_eof_restart_pass restart_seconds=%.3f resolver_requests=%d position_seconds=%.3f", restart.Seconds(), resolves.Load(), p.Snapshot().Position.Seconds())
}

// Explicitly gated: contacts YouTube and exercises actual CoreAudio output.
func TestNativePlaybackLive(t *testing.T) {
	if os.Getenv("YTMGO_LIVE_PLAYBACK") != "1" {
		t.Skip("set YTMGO_LIVE_PLAYBACK=1 for live native playback")
	}
	p, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	initializeAnonymousLive(t, p)
	p.SetVolume(.1)
	resolve, open := p.resolve, p.open
	p.resolve = func(ctx context.Context, id string) (media, error) {
		start := time.Now()
		info, err := resolve(ctx, id)
		t.Logf("resolver phase %.2fs, success=%t", time.Since(start).Seconds(), err == nil)
		return info, err
	}
	p.open = func(ctx context.Context, info media, at time.Duration) (*pcmDecoder, error) {
		start := time.Now()
		d, err := open(ctx, info, at)
		t.Logf("first-PCM phase %.2fs, success=%t", time.Since(start).Seconds(), err == nil)
		return d, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	start := time.Now()
	if err := p.Load(ctx, "52fNscjkST4"); err != nil {
		t.Fatal(err)
	}
	t.Logf("first PCM ready after %.2fs", time.Since(start).Seconds())
	time.Sleep(3 * time.Second)
	s := p.Snapshot()
	if !s.Playing || s.Position < time.Second || s.Error != "" {
		t.Fatalf("native playback did not progress: %+v", s)
	}
	p.Pause()
	time.Sleep(100 * time.Millisecond)
	paused := p.Snapshot().Position
	time.Sleep(500 * time.Millisecond)
	if p.Snapshot().Position-paused > 50*time.Millisecond {
		t.Fatal("pause did not hold position")
	}
	if err := p.Seek(30 * time.Second); err != nil {
		t.Fatal(err)
	}
	if s := p.Snapshot(); s.Playing || s.Position < 30*time.Second {
		t.Fatalf("paused seek failed: %+v", s)
	}
	if err := p.Play(); err != nil {
		t.Fatal(err)
	}
	time.Sleep(2 * time.Second)
	if s := p.Snapshot(); !s.Playing || s.Position < 31*time.Second || s.Error != "" {
		t.Fatalf("seeked playback failed: %+v", s)
	}
	p.Stop()
	if s := p.Snapshot(); s.Playing || s.Position != 0 {
		t.Fatalf("stop failed: %+v", s)
	}
	t.Log("native output, pause, seek, resume, stop and close passed")
}
