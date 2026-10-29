//go:build darwin

package main

import (
	"errors"
	"testing"
)

// Uses only synthetic callbacks; never opens the machine's credential store.
func TestAccountRestoreDisallowsKeychainPromptsAndRestoresPolicy(t *testing.T) {
	for _, readFails := range []bool{false, true} {
		var state uint8 = 1
		calls := 0
		api := &keychainInteraction{
			get: func(out *uint8) int32 { *out = state; return 0 },
			set: func(v uint8) int32 { state = v; return 0 },
		}
		_, err := readAccountSecretWithoutPrompt(api, func() ([]byte, error) {
			calls++
			if state != 0 {
				t.Fatal("restore could display an authorization prompt")
			}
			if readFails {
				return nil, errors.New("synthetic-read-denied")
			}
			return []byte("synthetic"), nil
		})
		if calls != 1 || state != 1 || (err != nil) != readFails {
			t.Fatal("read did not restore original interaction policy")
		}
	}
}

func TestAccountRestoreFailsClosedWhenPromptPolicyCannotBeSet(t *testing.T) {
	for _, api := range []*keychainInteraction{nil, {get: func(*uint8) int32 { return -1 }}, {get: func(*uint8) int32 { return 0 }, set: func(uint8) int32 { return -1 }}} {
		called := false
		_, err := readAccountSecretWithoutPrompt(api, func() ([]byte, error) { called = true; return nil, nil })
		if err == nil || called {
			t.Fatal("unsafe prompt policy still queried a secret")
		}
	}
}
