//go:build !darwin

package main

import "github.com/egoist/mygo"

func readNativeAccountSecret(key string) ([]byte, error)    { return mygo.Secrets.Get(key) }
func setNativeAccountSecret(key string, value []byte) error { return mygo.Secrets.Set(key, value) }
func deleteNativeAccountSecret(key string) error            { return mygo.Secrets.Delete(key) }
