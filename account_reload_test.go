package main

import (
	"context"
	"testing"
	"time"

	"ytmgo/internal/accountsession"
	"ytmgo/internal/catalog"
	"ytmgo/internal/player"
)

func TestUnchangedAccountCompletionKeepsPreparedTrack(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	cancelled := false
	a := &desktop{ctx: ctx, player: &player.Player{}, queueIndex: 0, preparedID: "def456ABC_-", prepareCancel: func() { cancelled = true }, model: UIModel{Playing: true, Current: catalog.Item{ID: "abc123DEF_-"}, Queue: []catalog.Item{{ID: "abc123DEF_-"}, {ID: "def456ABC_-"}}}}
	a.finishAccountPlayback(accountReplay{})
	if cancelled || a.preparedID != "def456ABC_-" {
		t.Fatal("unchanged account restarted next track")
	}
}

func TestFocusRefreshAndAvatarRespectFreshness(t *testing.T) {
	now := time.Now()
	s := accountState{ready: true, checkedAt: now.Add(-5*time.Minute + time.Second)}
	if s.refreshDue(now, false) || !s.refreshDue(now, true) {
		t.Fatal("focus throttle skipped the periodic verification")
	}
	s.refreshAttempt = now.Add(-time.Second)
	if s.refreshDue(now, true) {
		t.Fatal("periodic check duplicated an immediately preceding focus check")
	}
	for _, mode := range []string{"verified", "recent_failure"} {
		a := &desktop{accountState: accountState{started: true, restored: true, ready: true, session: &accountsession.Session{}}}
		if mode == "verified" {
			a.accountState.checkedAt = time.Now()
		} else {
			a.accountState.refreshAttempt = time.Now()
		}
		a.refreshAccount()
		if a.accountBusy {
			t.Fatal("focus repeated a recent account request")
		}
	}
	a := &desktop{model: UIModel{SignedIn: true}}
	a.accountState.avatarCheckedAt = time.Now()
	a.loadAccountAvatar()
	if a.accountState.avatar != nil {
		t.Fatal("fresh avatar fetched again")
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	a.accountState.avatarCheckedAt = time.Time{}
	pending := &accountAvatarRequest{ctx: ctx, cancel: cancel}
	a.accountState.avatar = pending
	a.loadAccountAvatar()
	if a.accountState.avatar != pending || ctx.Err() != nil {
		t.Fatal("pending avatar was canceled/restarted")
	}
	a.clearAccountAvatar()
	if !a.accountState.avatarCheckedAt.IsZero() || ctx.Err() == nil {
		t.Fatal("avatar reset retained account data")
	}
}
