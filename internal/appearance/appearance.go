// Package appearance stores the app's appearance independently of the music library.
package appearance

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

func Valid(value string) bool { return value == "system" || value == "light" || value == "dark" }

func Load(path string) string {
	data, err := os.ReadFile(path)
	value := strings.TrimSpace(string(data))
	if err != nil || !Valid(value) {
		return "system"
	}
	return value
}

func Save(path, value string) error {
	if !Valid(value) {
		return errors.New("invalid appearance")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".appearance-*")
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
