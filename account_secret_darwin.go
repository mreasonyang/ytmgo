//go:build darwin

package main

import (
	"sync"

	"github.com/ebitengine/purego"
	"github.com/egoist/mygo"
	"ytmgo/internal/accountsession"
)

// MyGo uses the file-based login Keychain. Its interaction flag is process-wide,
// so serialize all of this app's secret operations while a restore reads without
// UI. A denied/locked old item becomes a recoverable error, never a hidden prompt.
var accountSecretMu sync.Mutex

type keychainInteraction struct {
	get func(*uint8) int32
	set func(uint8) int32
}

var accountKeychainInteraction = sync.OnceValue(func() *keychainInteraction {
	lib, err := purego.Dlopen("/System/Library/Frameworks/Security.framework/Security", purego.RTLD_NOW|purego.RTLD_LOCAL)
	if err != nil {
		return nil
	}
	get, err := purego.Dlsym(lib, "SecKeychainGetUserInteractionAllowed")
	if err != nil {
		return nil
	}
	set, err := purego.Dlsym(lib, "SecKeychainSetUserInteractionAllowed")
	if err != nil {
		return nil
	}
	api := &keychainInteraction{}
	purego.RegisterFunc(&api.get, get)
	purego.RegisterFunc(&api.set, set)
	return api
})

func readNativeAccountSecret(key string) ([]byte, error) {
	accountSecretMu.Lock()
	defer accountSecretMu.Unlock()
	api := accountKeychainInteraction()
	return readAccountSecretWithoutPrompt(api, func() ([]byte, error) { return mygo.Secrets.Get(key) })
}

func readAccountSecretWithoutPrompt(api *keychainInteraction, read func() ([]byte, error)) ([]byte, error) {
	var previous uint8
	if api == nil || api.get(&previous) != 0 || api.set(0) != 0 {
		return nil, accountsession.ErrStorage
	}
	defer api.set(previous)
	return read()
}

func setNativeAccountSecret(key string, value []byte) error {
	accountSecretMu.Lock()
	defer accountSecretMu.Unlock()
	return mygo.Secrets.Set(key, value)
}

func deleteNativeAccountSecret(key string) error {
	accountSecretMu.Lock()
	defer accountSecretMu.Unlock()
	return mygo.Secrets.Delete(key)
}
