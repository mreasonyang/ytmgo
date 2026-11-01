package main

import (
	"context"
	"time"
)

type accountAvatarRequest struct {
	ctx        context.Context
	cancel     context.CancelFunc
	generation uint64
}

func (a *desktop) clearAccountAvatar() {
	if request := a.accountState.avatar; request != nil {
		request.cancel()
		a.accountState.avatar = nil
	}
	a.model.AccountAvatar = ""
}

func (a *desktop) loadAccountAvatar() {
	if !a.model.SignedIn {
		return
	}
	if previous := a.accountState.avatar; previous != nil {
		previous.cancel()
	}
	ctx, cancel := context.WithTimeout(a.ctx, 15*time.Second)
	request := &accountAvatarRequest{ctx: ctx, cancel: cancel, generation: a.accountGeneration}
	a.accountState.avatar = request
	go func() {
		avatar, err := a.catalog.AccountAvatar(ctx)
		a.post(func() { a.applyAccountAvatar(request, avatar, err) })
	}()
}

func (a *desktop) applyAccountAvatar(request *accountAvatarRequest, avatar string, err error) {
	defer request.cancel()
	if a.accountState.avatar != request {
		return
	}
	a.accountState.avatar = nil
	if request.ctx.Err() != nil || request.generation != a.accountGeneration || !a.model.SignedIn || err != nil {
		return
	}
	a.model.AccountAvatar = avatar
	a.invalidate()
}
