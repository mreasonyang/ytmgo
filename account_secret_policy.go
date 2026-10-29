package main

import "ytmgo/internal/accountsession"

// Kept separate from the native API binding so the authorization policy can be
// tested on every platform without touching a real credential store.
type keychainInteraction struct {
	get func(*uint8) int32
	set func(uint8) int32
}

func withKeychainInteraction(api *keychainInteraction, interaction accountsession.Interaction, operation func() error) (err error) {
	var previous, requested uint8
	if interaction == accountsession.AllowPrompt {
		requested = 1
	}
	if api == nil || api.get(&previous) != 0 || api.set(requested) != 0 {
		return accountsession.ErrStorage
	}
	defer func() {
		if api.set(previous) != 0 {
			err = accountsession.ErrStorage
		}
	}()
	return operation()
}
