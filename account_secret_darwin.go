//go:build darwin

package main

import (
	"sync"

	"github.com/ebitengine/purego"
	"github.com/egoist/mygo"
	"ytmgo/internal/accountsession"
)

// MyGo uses the file-based login Keychain. Its interaction flag is process-wide,
// so serialize this app's secret operations while setting the requested policy.
// Background restoration is silent; explicit account actions may request access.
var accountSecretMu sync.Mutex

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

func withNativeAccountSecret(interaction accountsession.Interaction, operation func() error) error {
	accountSecretMu.Lock()
	defer accountSecretMu.Unlock()
	return withKeychainInteraction(accountKeychainInteraction(), interaction, operation)
}

func readNativeAccountSecret(key string, interaction accountsession.Interaction) ([]byte, error) {
	var value []byte
	err := withNativeAccountSecret(interaction, func() error {
		var err error
		value, err = mygo.Secrets.Get(key)
		return err
	})
	return value, err
}

func setNativeAccountSecret(key string, value []byte) error {
	return withNativeAccountSecret(accountsession.AllowPrompt, func() error { return mygo.Secrets.Set(key, value) })
}

func deleteNativeAccountSecret(key string, interaction accountsession.Interaction) error {
	return withNativeAccountSecret(interaction, func() error { return mygo.Secrets.Delete(key) })
}
