package main

import (
	"errors"
	"testing"

	"ytmgo/internal/accountsession"
)

// Uses only synthetic callbacks; never opens the machine's credential store.
func TestAccountKeychainInteractionAppliesAndRestoresPolicy(t *testing.T) {
	for _, previous := range []uint8{0, 1} {
		for _, interaction := range []accountsession.Interaction{accountsession.NoPrompt, accountsession.AllowPrompt} {
			for _, operationErr := range []error{nil, errors.New("synthetic-denied"), accountsession.ErrNotFound} {
				state, requested := previous, uint8(0)
				if interaction == accountsession.AllowPrompt {
					requested = 1
				}
				calls := 0
				api := &keychainInteraction{
					get: func(out *uint8) int32 { *out = state; return 0 },
					set: func(v uint8) int32 { state = v; return 0 },
				}
				err := withKeychainInteraction(api, interaction, func() error {
					calls++
					if state != requested {
						t.Fatal("secret operation used the wrong authorization policy")
					}
					return operationErr
				})
				if calls != 1 || state != previous || !errors.Is(err, operationErr) {
					t.Fatal("operation changed its error or failed to restore authorization policy")
				}
			}
		}
	}
}

func TestAccountKeychainFailsClosedWhenPolicyCannotBeSet(t *testing.T) {
	for _, interaction := range []accountsession.Interaction{accountsession.NoPrompt, accountsession.AllowPrompt} {
		for _, api := range []*keychainInteraction{nil, {get: func(*uint8) int32 { return -1 }}, {get: func(*uint8) int32 { return 0 }, set: func(uint8) int32 { return -1 }}} {
			called := false
			err := withKeychainInteraction(api, interaction, func() error { called = true; return nil })
			if err != accountsession.ErrStorage || called {
				t.Fatal("unknown authorization policy still accessed a secret")
			}
		}
	}
}

func TestAccountKeychainReportsPolicyRestoreFailure(t *testing.T) {
	sets := 0
	api := &keychainInteraction{
		get: func(out *uint8) int32 { *out = 1; return 0 },
		set: func(uint8) int32 {
			sets++
			if sets == 2 {
				return -1
			}
			return 0
		},
	}
	if withKeychainInteraction(api, accountsession.NoPrompt, func() error { return nil }) != accountsession.ErrStorage {
		t.Fatal("failed authorization policy restoration was hidden")
	}
}
