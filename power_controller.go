package main

import (
	"context"
	"errors"
	"time"
)

func (a *desktop) suspendForPower() {
	if a.quitting || a.player == nil || a.restoredTrack {
		return
	}
	a.powerGeneration++
	a.powerPlaybackSuspended = true
	a.cancelResume()
	if a.playCancel != nil {
		a.playCancel()
	}
	a.cancelSeek()
	if a.prepareCancel != nil {
		a.prepareCancel()
	}
	a.playGeneration++
	a.seekGeneration++
	a.preparedID = ""
	a.loadPending = false
	_ = a.player.Suspend()
	state := a.player.Snapshot()
	a.model.Playing, a.model.PlaybackLoading = false, false
	a.model.Position = state.Position.Seconds()
	a.endedHandled = true
	a.syncVideo()
	a.invalidate()
}

func (a *desktop) resumeAfterPower() {
	if a.quitting || a.player == nil || !a.powerPlaybackSuspended {
		return
	}
	a.cancelResume()
	ctx, cancel := context.WithTimeout(a.ctx, 45*time.Second)
	a.resumeCancel = cancel
	a.powerGeneration++
	powerGeneration, playGeneration := a.powerGeneration, a.playGeneration
	go func() {
		err := a.player.ResumeFromSuspend(ctx)
		a.post(func() {
			defer cancel()
			if powerGeneration != a.powerGeneration {
				return
			}
			a.powerPlaybackSuspended = false
			defer a.syncVideo()
			if playGeneration != a.playGeneration || errors.Is(ctx.Err(), context.Canceled) {
				return
			}
			a.resumeCancel = nil
			if err != nil {
				state := a.player.Snapshot()
				recordPlaybackDiagnostic(a.diagnosticPath, "audio", state.Error, state.ErrorDetails)
				a.playbackFailure(state)
			} else {
				a.model.PlaybackError = ""
				a.endedHandled = false
				a.prepareNext()
				a.applyPlaybackState(a.player.Snapshot())
			}
		})
	}()
}
