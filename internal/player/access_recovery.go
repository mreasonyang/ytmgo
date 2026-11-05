package player

import (
	"context"
	"time"
)

const accessRecoveryHealthyPlayback = 10 * time.Second

// Count heard audio within the current decoder, rather than wall time or the
// track's absolute position. Pause and a newly installed seek decoder restart
// the healthy interval; loading and buffering cannot replenish the budget.
func (p *Player) observeAccessPlaybackLocked() {
	if p.accessRecoveries == 0 || p.session == nil || !p.wantPlay || p.state.Loading {
		return
	}
	period := p.accessHealthyPeriod
	if period <= 0 {
		period = accessRecoveryHealthyPlayback
	}
	if p.session.output.Position()-p.accessHealthyFrom >= period {
		p.accessRecoveries = 0
	}
}

// A persistent denial cannot loop. After healthy playback the budget becomes
// available again. The bookmark and play/pause intent survive recovery.
// Called with p.mu held; all process/network work runs outside that lock.
func (p *Player) recoverAccessLocked() {
	at, id, info := p.state.Position, p.videoID, p.media
	ctx, gen, old, err := p.beginLocked(context.Background(), at, p.wantPlay, false)
	if err != nil {
		return
	}
	p.accessRecoveries++
	p.media = nil
	p.seekReloadable = true
	p.clearPreparedLocked()
	go func() {
		defer p.ops.Done()
		old.close()
		releaseMedia(info)
		_ = p.resolveFresh(ctx, gen, id, at)
	}()
}

func (p *Player) resolveFresh(ctx context.Context, gen uint64, id string, at time.Duration) error {
	return p.resolveFreshAttempts(ctx, gen, id, at, 2)
}

func (p *Player) resolveFreshAttempts(ctx context.Context, gen uint64, id string, at time.Duration, attempts int) error {
	if p.refresh != nil {
		if err := p.refresh(ctx); err != nil {
			return p.fail(gen, err)
		}
	}
	// A new URL can also be rejected by a changing egress. One extra fresh
	// resolve/open is allowed here, separate from the healthy-playback budget.
	// Persistent denials still terminate after this finite startup budget.
	for attempt := 0; ; attempt++ {
		if err := ctx.Err(); err != nil {
			return p.fail(gen, err)
		}
		info, err := p.resolve(ctx, id)
		if err != nil {
			return p.fail(gen, err)
		}
		d, err := p.open(ctx, info, at)
		if err == nil {
			return p.installDecoder(ctx, gen, info, d, at, true)
		}
		releaseMedia(&info)
		if attempt+1 >= attempts || safeError(err) != "audio_access_denied" || ctx.Err() != nil {
			return p.fail(gen, err)
		}
		if p.refresh != nil {
			if err := p.refresh(ctx); err != nil {
				return p.fail(gen, err)
			}
		}
	}
}
