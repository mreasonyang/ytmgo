//go:build !darwin

package main

import (
	"github.com/egoist/mygo"
	"ytmgo/internal/accountsession"
)

func readNativeAccountSecret(key string, _ accountsession.Interaction) ([]byte, error) {
	return mygo.Secrets.Get(key)
}
func setNativeAccountSecret(key string, value []byte) error { return mygo.Secrets.Set(key, value) }
func deleteNativeAccountSecret(key string, _ accountsession.Interaction) error {
	return mygo.Secrets.Delete(key)
}
