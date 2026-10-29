package accountsession

import (
	"bytes"
	"context"
	"errors"
	"os"
	"testing"
)

func TestVerifiedLoginReplacementDoesNotReadOldKey(t *testing.T) {
	for _, prior := range []string{"locked-key", "missing-key", "corrupt-file"} {
		t.Run(prior, func(t *testing.T) {
			store, secrets := testStore(t)
			if store.Save(context.Background(), fixture()) != nil {
				t.Fatal("initial fixture save failed")
			}
			oldKey := append([]byte(nil), secrets.values[sessionKeyName]...)
			if prior == "missing-key" {
				delete(secrets.values, sessionKeyName)
			}
			if prior == "corrupt-file" {
				if os.WriteFile(store.Path, []byte("unreadable-old-session"), 0600) != nil {
					t.Fatal("fixture setup failed")
				}
			}
			reads, deletes := secrets.getCalls, secrets.deleteCalls
			secrets.failGet = true // Any old-key read would reject this login.
			changed := fixture()
			index := 3
			changed.AccountIndex = &index
			if store.Replace(context.Background(), changed) != nil {
				t.Fatal("fresh login still depends on old storage")
			}
			if secrets.getCalls != reads || secrets.deleteCalls != deletes {
				t.Fatal("replacement accessed or removed an old key")
			}
			if prior != "missing-key" && !bytes.Equal(oldKey, secrets.values[sessionKeyName]) {
				t.Fatal("replacement overwrote old key")
			}
			data := savedBytes(t, store.Path)
			name, _, err := sessionEnvelope(data)
			if err != nil || name == sessionKeyName || len(secrets.values[name]) != 32 || secrets.unexpectedKeyWasTouched {
				t.Fatal("replacement did not select an isolated owned key")
			}
			if bytes.Contains(data, []byte("SAPISID")) {
				t.Fatal("replacement saved plaintext session")
			}
			secrets.failGet = false
			restarted := Store{Secrets: secrets, Path: store.Path}
			loaded, err := restarted.Load()
			if err != nil || *loaded.AccountIndex != 3 {
				t.Fatal("new login did not survive restart")
			}
			if restarted.Delete() != nil || secrets.values[name] != nil {
				t.Fatal("logout did not remove the active rotated key")
			}
			if prior != "missing-key" && !bytes.Equal(oldKey, secrets.values[sessionKeyName]) {
				t.Fatal("logout touched a retired key")
			}
		})
	}
}

func TestVerifiedLoginReplacementFailurePreservesOldSession(t *testing.T) {
	for _, failure := range []string{"set", "cancel-after-set", "file-commit"} {
		t.Run(failure, func(t *testing.T) {
			store, secrets := testStore(t)
			if store.Save(context.Background(), fixture()) != nil {
				t.Fatal("initial fixture save failed")
			}
			original := savedBytes(t, store.Path)
			oldKey := append([]byte(nil), secrets.values[sessionKeyName]...)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			restore := func() {}
			switch failure {
			case "set":
				secrets.failSet = true
			case "cancel-after-set":
				secrets.afterSet = cancel
			case "file-commit":
				secrets.afterSet = func() { restore = blockSessionLocation(t, store.Path) }
			}
			if store.Replace(ctx, fixture()) != ErrStorage {
				t.Fatal("failed replacement was accepted")
			}
			restore()
			if !bytes.Equal(original, savedBytes(t, store.Path)) || !bytes.Equal(oldKey, secrets.values[sessionKeyName]) || len(secrets.values) != 1 {
				t.Fatal("failed replacement damaged previous session or leaked a fresh key")
			}
			secrets.failSet, secrets.afterSet = false, nil
			if _, err := store.Load(); err != nil {
				t.Fatal("old session no longer loads after failed replacement")
			}
			noTemporaryFiles(t, store.Path)
		})
	}
}

func TestRotatedSessionRefreshAndHeaderAuthentication(t *testing.T) {
	store, secrets := testStore(t)
	if store.Replace(context.Background(), fixture()) != nil {
		t.Fatal("replacement failed")
	}
	original := savedBytes(t, store.Path)
	name, _, _ := sessionEnvelope(original)
	if store.Save(context.Background(), fixture()) != nil {
		t.Fatal("rotated session refresh failed")
	}
	if next, _, _ := sessionEnvelope(savedBytes(t, store.Path)); next != name || secrets.setCalls != 1 {
		t.Fatal("ordinary refresh rotated the active key")
	}
	if store.Replace(context.Background(), fixture()) != nil {
		t.Fatal("second replacement failed")
	}
	data := savedBytes(t, store.Path)
	next, _, _ := sessionEnvelope(data)
	if next == name {
		t.Fatal("new identity reused an existing key")
	}
	// Alter only the key ID to point to another valid owned key. AEAD must
	// reject the ciphertext rather than accepting a cross-key header.
	copy(data[len(rotatedFileHeader):len(rotatedFileHeader)+keyIDSize], original[len(rotatedFileHeader):len(rotatedFileHeader)+keyIDSize])
	if os.WriteFile(store.Path, data, 0600) != nil {
		t.Fatal("tamper fixture failed")
	}
	if _, err := store.Load(); !errors.Is(err, ErrStorage) {
		t.Fatal("tampered key selection was accepted")
	}
}

func TestVerifiedLoginRejectsUnsafeOrCanceledReplacementBeforeKeyCreation(t *testing.T) {
	store, secrets := testStore(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if store.Replace(ctx, fixture()) != ErrStorage || secrets.setCalls != 0 {
		t.Fatal("canceled replacement created a key")
	}
	if store.Save(context.Background(), fixture()) != nil {
		t.Fatal("initial fixture save failed")
	}
	sets := secrets.setCalls
	restore := blockSessionLocation(t, store.Path)
	defer restore()
	if store.Replace(context.Background(), fixture()) != ErrStorage || secrets.setCalls != sets {
		t.Fatal("unsafe replacement reached native secrets")
	}
}
