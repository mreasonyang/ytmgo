package main

import (
	"context"
	"errors"
	"testing"
)

func TestAccountAvatarCompletionHonorsIdentityAndFailures(t *testing.T) {
	for _, state := range []string{"success", "missing", "failure", "canceled", "switched", "signed-out", "superseded"} {
		t.Run(state, func(t *testing.T) {
			a := &desktop{model: UIModel{SignedIn: true, MusicPremium: true, AccountAvatar: "previous"}, accountGeneration: 1}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			request := &accountAvatarRequest{ctx: ctx, cancel: cancel, generation: 1}
			a.accountState.avatar = request
			avatar, want := "new", "previous"
			var err error
			switch state {
			case "success":
				want = avatar
			case "missing":
				avatar, want = "", ""
			case "failure":
				err = errors.New("synthetic-network-failure")
			case "canceled":
				cancel()
			case "switched":
				a.accountGeneration++
				a.clearAccountContent()
				want = ""
			case "signed-out":
				a.model.SignedIn = false
				a.clearAccountContent()
				want = ""
			case "superseded":
				a.accountState.avatar = &accountAvatarRequest{}
			}
			a.applyAccountAvatar(request, avatar, err)
			if a.model.AccountAvatar != want || !a.model.MusicPremium {
				t.Fatal("avatar completion changed the wrong account state")
			}
			if (state == "switched" || state == "signed-out") && ctx.Err() == nil {
				t.Fatal("account reset did not cancel the avatar request")
			}
		})
	}
}
