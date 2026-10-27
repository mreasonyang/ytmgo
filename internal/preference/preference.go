// Package preference stores single-value settings as small files, written
// atomically so an interrupted save never leaves a partial value.
package preference

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// Load returns the stored value, or fallback when it is missing or invalid.
func Load(path string, valid func(string) bool, fallback string) string {
	data, err := os.ReadFile(path)
	value := strings.TrimSpace(string(data))
	if err != nil || !valid(value) {
		return fallback
	}
	return value
}

// Save replaces the stored value; invalid values are rejected.
func Save(path, value string, valid func(string) bool) error {
	if !valid(value) {
		return errors.New("invalid preference value")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+"-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if _, err = file.WriteString(value + "\n"); err != nil {
		file.Close()
		return err
	}
	if err = file.Close(); err != nil {
		return err
	}
	return os.Rename(file.Name(), path)
}
