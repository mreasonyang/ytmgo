// Package session clears the WebView store without reading its credentials.
package session

import (
	"errors"
	"os"
	"path/filepath"
)

// Pending inspects only a non-secret completion marker, never browser data.
func Pending(marker string) (bool, error) {
	_, err := os.Stat(marker)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return err == nil, err
}

// Clear records intent before closing pages and clearing the runtime's store.
// On interruption or failure the marker remains, so startup must retry before
// loading any remote page. The marker contains no account or credential data.
func Clear(marker string, closePages func(), clearStore func() error) error {
	if err := os.MkdirAll(filepath.Dir(marker), 0700); err != nil {
		return err
	}
	f, err := os.OpenFile(marker, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	_, err = f.WriteString("pending\n")
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	closePages()
	if err := clearStore(); err != nil {
		return err
	}
	return os.Remove(marker)
}
