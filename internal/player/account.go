package player

import (
	"context"
	"errors"
	"strings"
)

// AccountSession travels only through private in-memory IPC. Never include it
// in logs, diagnostics, errors, command arguments, or exported evidence.
type AccountSession struct {
	LoggedIn       bool   `json:"loggedIn"`
	MusicPremium   bool   `json:"musicPremium"`
	Cookie         string `json:"cookie"`
	AccountIndex   int    `json:"accountIndex"`
	OnBehalfOfUser string `json:"onBehalfOfUser"`
	Revision       uint64 `json:"revision"`
}

func (s AccountSession) valid() bool {
	if s.Revision == 0 || s.Revision > 9007199254740991 || s.AccountIndex < 0 || s.AccountIndex > 99 || len(s.Cookie) > 64<<10 || len(s.OnBehalfOfUser) > 128 {
		return false
	}
	if !s.LoggedIn {
		return !s.MusicPremium && s.Cookie == "" && s.AccountIndex == 0 && s.OnBehalfOfUser == ""
	}
	if strings.TrimSpace(s.Cookie) == "" {
		return false
	}
	for _, b := range s.Cookie {
		if b < 32 || b > 126 {
			return false
		}
	}
	for _, b := range s.OnBehalfOfUser {
		if !(b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' || b >= '0' && b <= '9' || b == '_' || b == '-') {
			return false
		}
	}
	return true
}

// SetAccount switches resolution without interrupting an already decoded track
// when only membership changes. A different account revokes its streams and
// stops output. All prefetches are canceled before the old engine is reaped.
// Call from a worker; the UI thread must remain free while cleanup completes.
func (p *Player) SetAccount(ctx context.Context, next AccountSession) error {
	if !next.valid() {
		return errors.New("invalid_account_session")
	}
	p.accountMu.Lock()
	defer p.accountMu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return ErrClosed
	}
	if p.accountSet && next.Revision < p.account.Revision {
		p.mu.Unlock()
		return errors.New("stale_account_session")
	}
	if p.accountSet && next.Revision == p.account.Revision && (next.LoggedIn != p.account.LoggedIn || next.AccountIndex != p.account.AccountIndex || next.OnBehalfOfUser != p.account.OnBehalfOfUser) {
		p.mu.Unlock()
		return errors.New("invalid_account_session")
	}
	if p.accountSet && next == p.account && p.accountErr == nil {
		p.mu.Unlock()
		return nil
	}
	identityChanged := !p.accountSet || next.Revision != p.account.Revision
	// Seek and ended-track replay reopen the existing lease. A same-identity
	// reconfiguration keeps that lease in the runtime bridge, so its decoder
	// may finish installing while the next-resolution engine is replaced.
	// Load clears media before opening and must still be canceled; a changed
	// identity always revokes the lease and invalidates any pending decoder.
	keepLeaseOpen := !identityChanged && p.state.Loading && p.media != nil
	ready := make(chan struct{})
	p.accountReady = ready
	life, cancel := context.WithCancel(ctx)
	p.accountCancel = cancel
	p.ops.Add(1)
	p.account, p.accountSet = next, true
	if !keepLeaseOpen {
		p.gen++
		if p.cancel != nil {
			p.cancel()
			p.cancel = nil
		}
		p.state.Loading = false
	}
	p.clearPreparedLocked()
	var old *session
	var oldMedia *media
	if identityChanged {
		old, oldMedia = p.session, p.media
		p.session, p.media, p.wantPlay = nil, nil, false
		p.suspended, p.sleeping = nil, false
		p.state.Position, p.state.Duration = 0, 0
		p.state.Playing, p.state.Ended = false, false
		p.state.Error, p.state.ErrorDetails = "", FailureDetails{}
	}
	p.mu.Unlock()
	defer p.ops.Done()
	defer cancel()
	old.close()
	releaseMedia(oldMedia)
	err := errors.New("runtime_missing")
	if p.configure != nil {
		err = p.configure(life, next)
	}
	p.mu.Lock()
	p.accountErr = err
	p.accountCancel = nil
	p.accountReady = nil
	close(ready)
	if p.accountInitialized != nil {
		close(p.accountInitialized)
		p.accountInitialized = nil
	}
	p.mu.Unlock()
	return err
}

// lockAccount returns with mu locked only on success. Loading cannot race the
// initial credential restore or publish a decoder during a mode switch.
func (p *Player) lockAccount(ctx context.Context) error {
	for {
		p.mu.Lock()
		if p.closed {
			p.mu.Unlock()
			return ErrClosed
		}
		if err := ctx.Err(); err != nil {
			p.mu.Unlock()
			return err
		}
		ready := p.accountReady
		if ready == nil {
			ready = p.accountInitialized
		}
		if ready == nil {
			if p.accountErr != nil {
				p.mu.Unlock()
				return errors.New("account_session_unavailable")
			}
			return nil
		}
		p.mu.Unlock()
		select {
		case <-ready:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func (p *Player) resolveVideo(ctx context.Context, id string) (videoMedia, error) {
	if err := p.lockAccount(ctx); err != nil {
		return videoMedia{}, err
	}
	p.mu.Unlock()
	return p.resolver.ResolveVideo(ctx, id)
}
