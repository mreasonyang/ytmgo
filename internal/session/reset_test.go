package session

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestInterruptedClearRetriesBeforeReturningToService(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, "signout-pending")
	for _, name := range []string{"appearance", "library.json"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("preserved"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	closed := false
	storageFailure := errors.New("simulated failure")
	err := Clear(marker, func() {
		if pending, err := Pending(marker); err != nil || !pending {
			t.Fatal("intent must be persisted before pages close")
		}
		closed = true
	}, func() error {
		if !closed {
			t.Fatal("cleared while pages could still write")
		}
		return storageFailure
	})
	if !errors.Is(err, storageFailure) {
		t.Fatal("clear failure was hidden")
	}
	if pending, err := Pending(marker); err != nil || !pending {
		t.Fatal("failed clear cannot be reported as complete")
	}
	if err := Clear(marker, func() {}, func() error { return nil }); err != nil {
		t.Fatal(err)
	}
	if pending, err := Pending(marker); err != nil || pending {
		t.Fatal("successful retry did not finish")
	}
	for _, name := range []string{"appearance", "library.json"} {
		b, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil || string(b) != "preserved" {
			t.Fatal("local library/preferences changed")
		}
	}
}

func TestCannotClearWithoutDurableIntent(t *testing.T) {
	blocker := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(blocker, nil, 0600); err != nil {
		t.Fatal(err)
	}
	err := Clear(filepath.Join(blocker, "pending"), func() { t.Fatal("closed before saving intent") }, func() error { t.Fatal("cleared before saving intent"); return nil })
	if err == nil {
		t.Fatal("failed marker write was ignored")
	}
}

func TestCrashAfterClosingLeavesRecoveryMarker(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "new-profile", "pending")
	func() {
		defer func() {
			if recover() == nil {
				t.Fatal("expected simulated interruption")
			}
		}()
		_ = Clear(marker, func() { panic("simulated interruption") }, func() error { t.Fatal("clear ran after interruption"); return nil })
	}()
	if pending, err := Pending(marker); err != nil || !pending {
		t.Fatal("restart would reuse old session")
	}
	info, err := os.Stat(marker)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("unexpected marker permissions")
	}
}
