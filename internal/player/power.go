package player

import (
	"context"
	"errors"
	"time"
)

type suspendedTrack struct {
	id                 string
	position, duration time.Duration
	play               bool
}

// Suspend retires network readers before the OS suspends their clocks/sockets.
// Only the playback bookmark survives. Cleanup is joined by resume and Close.
func (p *Player) Suspend() error {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return ErrClosed
	}
	if p.sleeping {
		p.mu.Unlock()
		return nil
	}
	p.sleeping = true
	at := p.state.Position
	if p.session != nil {
		at = p.session.base + p.session.output.Position()
	}
	if p.state.Duration > 0 {
		at = min(at, p.state.Duration)
	}
	if videoIDPattern.MatchString(p.videoID) {
		p.suspended = &suspendedTrack{id: p.videoID, position: max(at, 0), duration: p.state.Duration, play: p.wantPlay && p.state.Error == ""}
	} else {
		p.suspended = nil
	}
	p.gen++
	if p.cancel != nil {
		p.cancel()
		p.cancel = nil
	}
	old, info := p.session, p.media
	if old != nil {
		old.output.Pause()
	}
	p.session, p.media = nil, nil
	p.clearPreparedLocked()
	p.state.Position, p.state.Playing, p.state.Loading, p.state.Ended = at, false, false, false
	previous := p.suspendDone
	done := make(chan struct{})
	p.suspendDone = done
	p.ops.Add(1)
	p.mu.Unlock()
	go func() {
		defer p.ops.Done()
		defer close(done)
		if previous != nil {
			<-previous
		}
		old.close()
		releaseMedia(info)
	}()
	return nil
}

// ResumeFromSuspend re-resolves the media and reopens at the bookmark. It never
// reuses a pipe/HTTP response that spent time asleep. User intent and generations
// still control publication, so pause, seek, stop or a new track can supersede it.
func (p *Player) ResumeFromSuspend(ctx context.Context) error {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return ErrClosed
	}
	token, done := p.suspended, p.suspendDone
	p.sleeping = false
	p.mu.Unlock()
	if token == nil {
		return nil
	}
	if done != nil {
		select {
		case <-done:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	if err := p.lockAccount(ctx); err != nil {
		return err
	}
	if p.suspended != token || p.sleeping {
		p.mu.Unlock()
		return context.Canceled
	}
	if p.state.Error != "" && !token.play {
		err := errors.New(p.state.Error)
		p.mu.Unlock()
		return err
	}
	bookmark := *token
	ctx, gen, old, err := p.beginLocked(ctx, bookmark.position, bookmark.play, true)
	if err == nil {
		p.state.Duration = bookmark.duration
	}
	p.mu.Unlock()
	if err != nil {
		return err
	}
	defer p.ops.Done()
	old.close()
	delays := []time.Duration{time.Second, 2 * time.Second, 4 * time.Second, 6 * time.Second}
	for attempt := 0; ; attempt++ {
		if err = ctx.Err(); err != nil {
			return p.fail(gen, err)
		}
		info, resolveErr := p.resolve(ctx, bookmark.id)
		err = resolveErr
		var decoder *pcmDecoder
		if err == nil {
			decoder, err = p.open(ctx, info, bookmark.position)
		}
		if err == nil {
			return p.installDecoder(ctx, gen, info, decoder, bookmark.position, true)
		}
		releaseMedia(&info)
		if attempt == len(delays) || ctx.Err() != nil || !resumeTransportFailure(err) {
			return p.fail(gen, err)
		}
		timer := time.NewTimer(delays[attempt])
		select {
		case <-ctx.Done():
			timer.Stop()
			return p.fail(gen, ctx.Err())
		case <-timer.C:
		}
	}
}

func resumeTransportFailure(err error) bool {
	if errors.Is(err, context.Canceled) {
		return false
	}
	switch safeError(err) {
	case "audio_network_timeout", "audio_connection_failed", "audio_server_error", "audio_stream_failed", "playback_timeout", "bootstrap_network_failed", "bootstrap_network_timeout":
		return true
	case "resolver_http_error":
		status := DiagnosticDetails(err).HTTPStatus
		return status == 0 || status >= 500
	}
	return false
}
