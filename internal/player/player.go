// Package player owns account-aware audio resolution, bounded PCM streaming and
// native MyGo audio output. Media URLs never leave this package.
package player

import (
	"context"
	"errors"
	"math"
	"regexp"
	"sync"
	"time"

	"github.com/egoist/mygo/audio"
)

var (
	ErrClosed      = errors.New("player_closed")
	ErrNoTrack     = errors.New("no_track")
	videoIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{11}$`)
)

type State struct {
	Playing, Loading   bool
	Position, Duration time.Duration
	Volume             float64
	Error              string
	ErrorDetails       FailureDetails
	Ended              bool
}

type Player struct {
	mu                 sync.Mutex
	ops                sync.WaitGroup
	resolve            func(context.Context, string) (media, error)
	open               func(context.Context, media, time.Duration) (*pcmDecoder, error)
	resolver           *resolver
	shutdown           func()
	gen                uint64
	cancel             context.CancelFunc
	closed, wantPlay   bool
	state              State
	media              *media
	session            *session
	prepared           *preparedTrack
	videoID            string
	accountMu          sync.Mutex
	account            AccountSession
	accountSet         bool
	accountReady       chan struct{}
	accountErr         error
	accountCancel      context.CancelFunc
	configure          func(context.Context, AccountSession) error
	accountInitialized chan struct{}
}

// A prepared track owns one decoder and one resolver lease. Its decoder is not
// attached to an audio output, so the bounded PCM pipe supplies backpressure.
type preparedTrack struct {
	id      string
	ctx     context.Context
	cancel  context.CancelFunc
	done    chan struct{}
	retired chan struct{}
	info    *media
	decoder *pcmDecoder
	err     error
}

type session struct {
	output  *audio.Player
	decoder *pcmDecoder
	base    time.Duration
}

func (s *session) close() {
	if s != nil {
		// MyGo closes a decoder from its decoding goroutine. Unblock any pipe
		// read first so that replacing/closing a stalled stream always finishes.
		s.output.Close()
		s.decoder.Close()
	}
}

func New() (*Player, error) {
	r, err := findRuntime()
	if err != nil {
		return nil, err
	}
	resolver, err := r.newResolver()
	if err != nil {
		return nil, err
	}
	p := &Player{state: State{Volume: .7}, resolver: resolver, shutdown: resolver.Close, accountInitialized: make(chan struct{})}
	p.resolve = resolver.Resolve
	p.configure = resolver.Configure
	p.open = r.open
	return p, nil
}

// Load resolves and starts a track. Call from a worker, never the UI thread.
// A newer Load/Seek/Stop/Close cancels this operation and its child processes.
// ctx bounds loading only; successful playback has its own lifetime.
func (p *Player) Load(ctx context.Context, videoID string) error {
	if !videoIDPattern.MatchString(videoID) {
		return errors.New("invalid_video_id")
	}
	if err := p.lockAccount(ctx); err != nil {
		return err
	}
	oldMedia := p.media
	ctx, gen, previous, err := p.beginLocked(ctx, 0, true, true)
	if err != nil {
		p.mu.Unlock()
		return err
	}
	p.videoID = videoID
	prepared := p.prepared
	if prepared != nil && prepared.id == videoID && prepared.ctx.Err() == nil {
		p.prepared = nil // Transfer ownership to this Load, including a pending open.
		loadCancel := p.cancel
		p.cancel = func() {
			// Superseding this Load must synchronously cancel the claimed
			// preparation, before a later decoder open can finish publishing.
			prepared.cancel()
			loadCancel()
		}
	} else {
		p.clearPreparedLocked()
		prepared = nil
	}
	p.mu.Unlock()
	defer p.ops.Done()
	previous.close()
	releaseMedia(oldMedia)
	if prepared != nil {
		stop := context.AfterFunc(ctx, prepared.cancel)
		select {
		case <-ctx.Done():
			stop()
			p.mu.Lock()
			p.disposePreparedLocked(prepared)
			p.mu.Unlock()
			return p.fail(gen, ctx.Err())
		case <-prepared.done:
		}
		stop()
		p.mu.Lock()
		info, decoder, prepareErr := prepared.info, prepared.decoder, prepared.err
		prepared.info, prepared.decoder = nil, nil
		p.mu.Unlock()
		prepared.cancel()
		if prepareErr != nil {
			return p.fail(gen, prepareErr)
		}
		return p.installDecoder(ctx, gen, *info, decoder, 0, true)
	}
	info, err := p.resolve(ctx, videoID)
	if err != nil {
		return p.fail(gen, err)
	}
	d, err := p.open(ctx, info, 0)
	if err != nil {
		releaseMedia(&info)
		return p.fail(gen, err)
	}
	return p.installDecoder(ctx, gen, info, d, 0, true)
}

// Prepare resolves and opens only the next queued track. It never changes the
// audible track or playback state. Call from a worker, never the UI thread.
func (p *Player) Prepare(ctx context.Context, videoID string) error {
	if !videoIDPattern.MatchString(videoID) {
		return errors.New("invalid_video_id")
	}
	if err := p.lockAccount(ctx); err != nil {
		return err
	}
	if p.closed || ctx.Err() != nil {
		p.mu.Unlock()
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return ErrClosed
	}
	// A queued preparation worker may start after the user has already chosen
	// that track. The foreground load owns its resolution in that case.
	if p.state.Loading && p.videoID == videoID {
		p.mu.Unlock()
		return nil
	}
	if existing := p.prepared; existing != nil && existing.id == videoID && existing.ctx.Err() == nil {
		p.mu.Unlock()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-existing.done:
			return existing.err
		}
	}
	previous := p.prepared
	p.clearPreparedLocked()
	life, cancel := context.WithCancel(ctx)
	prepared := &preparedTrack{id: videoID, ctx: life, cancel: cancel, done: make(chan struct{}), retired: make(chan struct{})}
	p.prepared = prepared
	p.ops.Add(1)
	p.mu.Unlock()
	defer p.ops.Done()
	stop := context.AfterFunc(life, func() {
		p.mu.Lock()
		if p.prepared == prepared {
			p.clearPreparedLocked()
		}
		p.mu.Unlock()
	})
	loading, done := context.WithTimeout(life, 90*time.Second)
	defer done()
	var info media
	var err error
	if previous != nil {
		select {
		case <-previous.retired:
		case <-loading.Done():
			err = loading.Err()
		}
	}
	if err == nil {
		info, err = p.resolve(loading, videoID)
	}
	var decoder *pcmDecoder
	if err == nil {
		decoder, err = p.open(loading, info, 0)
	}
	if err == nil {
		err = loading.Err()
	}
	if err != nil {
		if decoder != nil {
			decoder.Close()
		}
		releaseMedia(&info)
		stop()
		cancel()
	}
	p.mu.Lock()
	if err == nil {
		prepared.info, prepared.decoder = &info, decoder
	} else {
		prepared.err = err
		if p.prepared == prepared {
			p.prepared = nil
		}
	}
	close(prepared.done)
	p.mu.Unlock()
	return err
}

// ClearPrepared detaches the slot immediately; decoder/process cleanup happens
// off the calling thread and is joined by Close.
func (p *Player) ClearPrepared() {
	p.mu.Lock()
	p.clearPreparedLocked()
	p.mu.Unlock()
}

func (p *Player) clearPreparedLocked() {
	if old := p.prepared; old != nil {
		p.prepared = nil
		p.disposePreparedLocked(old)
	}
}

func (p *Player) disposePreparedLocked(old *preparedTrack) {
	old.cancel()
	p.ops.Add(1)
	go func() {
		defer p.ops.Done()
		defer close(old.retired)
		<-old.done
		p.mu.Lock()
		d, info := old.decoder, old.info
		old.decoder, old.info = nil, nil
		p.mu.Unlock()
		if d != nil {
			d.Close()
		}
		releaseMedia(info)
	}()
}

func releaseMedia(info *media) {
	if info != nil && info.release != nil {
		info.release()
	}
}

func (p *Player) beginLocked(ctx context.Context, at time.Duration, play, clear bool) (context.Context, uint64, *session, error) {
	if p.closed {
		return nil, 0, nil, ErrClosed
	}
	if ctx.Err() != nil {
		return nil, 0, nil, ctx.Err()
	}
	p.ops.Add(1)
	p.gen++
	if p.cancel != nil {
		p.cancel()
	}
	ctx, p.cancel = context.WithTimeout(ctx, 90*time.Second)
	old := p.session
	p.session = nil
	if clear {
		p.media = nil
		p.state.Duration = 0
	}
	p.wantPlay = play
	p.state.Position, p.state.Loading, p.state.Playing = at, true, false
	p.state.Error, p.state.ErrorDetails, p.state.Ended = "", FailureDetails{}, false
	return ctx, p.gen, old, nil
}

func (p *Player) install(ctx context.Context, gen uint64, info media, at time.Duration) error {
	d, err := p.open(ctx, info, at)
	if err != nil {
		return p.fail(gen, err)
	}
	return p.installDecoder(ctx, gen, info, d, at, false)
}

func (p *Player) installDecoder(ctx context.Context, gen uint64, info media, d *pcmDecoder, at time.Duration, ownsMedia bool) error {
	p.mu.Lock()
	if p.closed || gen != p.gen || ctx.Err() != nil {
		p.mu.Unlock()
		d.Close()
		if ownsMedia {
			releaseMedia(&info)
		}
		return p.fail(gen, context.Canceled)
	}
	defer p.mu.Unlock()
	if p.cancel != nil {
		p.cancel()
		p.cancel = nil
	}
	out := audio.NewPlayer(d).SetVolume(p.state.Volume)
	p.media, p.session = &info, &session{out, d, at}
	p.state.Duration, p.state.Loading = info.Duration, false
	p.state.Playing = p.wantPlay
	if p.wantPlay {
		out.Play()
	}
	return nil
}

func (p *Player) fail(gen uint64, err error) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if gen != p.gen || p.closed {
		return context.Canceled
	}
	if p.cancel != nil {
		p.cancel()
		p.cancel = nil
	}
	p.state.Loading, p.state.Playing = false, false
	if errors.Is(err, context.Canceled) {
		return err
	}
	// Dependencies emit only fixed category errors. Never propagate raw
	// exec/network errors, which can contain signed media URLs.
	p.state.Error, p.state.ErrorDetails = safeError(err), DiagnosticDetails(err)
	return errors.New(p.state.Error)
}

func (p *Player) Play() error {
	return p.PlayContext(context.Background())
}

// PlayContext keeps an ended-track restart in the same transaction as reading
// its media. A queued resume can be canceled before it touches a newer track.
func (p *Player) PlayContext(ctx context.Context) error {
	if err := p.lockAccount(ctx); err != nil {
		return err
	}
	if p.closed {
		p.mu.Unlock()
		return ErrClosed
	}
	if err := ctx.Err(); err != nil {
		p.mu.Unlock()
		return err
	}
	ended := p.state.Ended
	if s := p.session; s != nil && p.wantPlay && !s.output.Playing() && s.decoder.eof.Load() {
		ended = true
	}
	p.wantPlay = true
	if p.state.Loading {
		p.mu.Unlock()
		return nil
	}
	if p.session != nil && !ended {
		p.session.output.Play()
		p.mu.Unlock()
		return nil
	}
	if p.media == nil {
		p.mu.Unlock()
		return ErrNoTrack
	}
	info := *p.media
	ctx, gen, old, err := p.beginLocked(ctx, 0, true, false)
	if err == nil {
		p.clearPreparedLocked()
	}
	p.mu.Unlock()
	if err != nil {
		return err
	}
	defer p.ops.Done()
	old.close()
	return p.install(ctx, gen, info, 0)
}

func (p *Player) Pause() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return ErrClosed
	}
	p.wantPlay = false
	if p.session != nil {
		p.session.output.Pause()
	}
	p.state.Playing = false
	return nil
}

func (p *Player) Stop() error {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return ErrClosed
	}
	p.gen++
	if p.cancel != nil {
		p.cancel()
		p.cancel = nil
	}
	old := p.session
	oldMedia := p.media
	p.session, p.media, p.wantPlay = nil, nil, false
	p.clearPreparedLocked()
	p.state.Position, p.state.Playing, p.state.Loading, p.state.Ended = 0, false, false, false
	p.state.Error, p.state.ErrorDetails = "", FailureDetails{}
	p.mu.Unlock()
	old.close()
	releaseMedia(oldMedia)
	return nil
}

// Seek opens a fresh HTTP range-capable decoder at at. No full-track PCM
// buffer or file is retained. Paused tracks remain paused after seeking.
func (p *Player) Seek(at time.Duration) error {
	return p.SeekContext(context.Background(), at)
}

// SeekContext lets the UI cancel a seek before its worker is scheduled, so a
// delayed gesture cannot seek a different track selected in the meantime.
func (p *Player) SeekContext(ctx context.Context, at time.Duration) error {
	if err := p.lockAccount(ctx); err != nil {
		return err
	}
	if p.closed {
		p.mu.Unlock()
		return ErrClosed
	}
	if p.media == nil {
		p.mu.Unlock()
		return ErrNoTrack
	}
	info, play := *p.media, p.wantPlay
	at = max(at, 0)
	if info.Duration > 0 {
		at = min(at, max(info.Duration-time.Millisecond, 0))
	}
	// Reading the track and replacing the operation must be atomic: otherwise
	// concurrent seeks can both capture the same generation and reject the
	// newer position instead of canceling the older decoder.
	ctx, gen, old, err := p.beginLocked(ctx, at, play, false)
	if err == nil {
		p.clearPreparedLocked()
	}
	p.mu.Unlock()
	if err != nil {
		return err
	}
	defer p.ops.Done()
	old.close()
	return p.install(ctx, gen, info, at)
}

func (p *Player) SetVolume(v float64) {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.state.Volume = min(max(v, 0), 1)
	if p.session != nil {
		p.session.output.SetVolume(p.state.Volume)
	}
}

func (p *Player) Snapshot() State {
	p.mu.Lock()
	defer p.mu.Unlock()
	if s := p.session; s != nil {
		p.state.Position = s.base + s.output.Position()
		if p.state.Duration > 0 {
			p.state.Position = min(p.state.Position, p.state.Duration)
		}
		p.state.Playing = s.output.Playing()
		if !p.state.Playing && p.wantPlay && s.decoder.eof.Load() {
			p.state.Ended = !s.decoder.failed.Load()
			if s.decoder.failed.Load() {
				p.state.Error, p.state.ErrorDetails = "audio_stream_failed", FailureDetails{}
			}
		}
		if p.state.Playing && audio.Err() != nil {
			p.state.Error, p.state.ErrorDetails = "audio_output_unavailable", FailureDetails{}
			p.state.Playing = false
			s.output.Pause()
			p.wantPlay = false
		}
	}
	return p.state
}

func (p *Player) Close() {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return
	}
	p.closed = true
	if p.accountCancel != nil {
		p.accountCancel()
	}
	if p.accountInitialized != nil {
		close(p.accountInitialized)
		p.accountInitialized = nil
	}
	p.gen++
	if p.cancel != nil {
		p.cancel()
		p.cancel = nil
	}
	old := p.session
	oldMedia := p.media
	p.session, p.media = nil, nil
	p.clearPreparedLocked()
	p.state.Playing, p.state.Loading = false, false
	p.mu.Unlock()
	old.close()
	releaseMedia(oldMedia)
	p.ops.Wait()
	if p.shutdown != nil {
		p.shutdown()
	}
}

func safeError(err error) string {
	if errors.Is(err, context.DeadlineExceeded) {
		return "playback_timeout"
	}
	var failure *resolutionFailure
	if errors.As(err, &failure) {
		err = failure.err
	}
	switch err.Error() {
	case "login_required", "stream_resolution_failed", "invalid_media_response", "audio_stream_failed", "audio_decoder_unavailable", "runtime_missing",
		"audio_access_denied", "audio_rate_limited", "audio_server_error", "audio_network_timeout", "audio_connection_failed",
		"audio_range_mismatch", "audio_stream_truncated",
		"proof_failed", "proof_expired", "proof_request_rejected", "bootstrap_cache_miss",
		"unexpected_request_host", "unexpected_authentication", "unexpected_client", "resolver_http_error",
		"init_failed", "resolver_response_limit", "cache_limit",
		"bootstrap_redirect", "bootstrap_network_failed", "bootstrap_network_timeout", "bootstrap_tls_failed":
		return err.Error()
	default:
		return "playback_failed"
	}
}
