package accountsession

import (
	"bytes"
	"context"
	"errors"
	"os"
	"testing"
)

func TestLoginPreparesKeyBeforeSessionAndSaveNeverAccessesNativeSecrets(t *testing.T) {
	for _, previous := range []string{"new", "saved", "corrupt-file", "restored"} {
		t.Run(previous, func(t *testing.T) {
			store, secrets := testStore(t)
			if previous != "new" {
				if saveAfterPrepare(context.Background(), &store, fixture()) != nil {
					t.Fatal("initial save failed")
				}
				store.clearKey() // Simulate the next app process.
			}
			if previous == "corrupt-file" {
				if os.WriteFile(store.Path, []byte("synthetic-damaged-session"), 0600) != nil {
					t.Fatal("cannot set up damaged session")
				}
			}
			if previous == "restored" {
				if _, err := store.Load(NoPrompt); err != nil {
					t.Fatal("restore failed")
				}
				secrets.failGet = true // The restored key is already ready.
			}
			var original []byte
			if previous != "new" {
				original = savedBytes(t, store.Path)
			}
			if store.PrepareLogin(context.Background()) != nil {
				t.Fatal("login preparation failed")
			}
			if len(secrets.values) != 1 || len(secrets.values[sessionKeyName]) != 32 || secrets.setCalls != 1 || secrets.deleteCalls != 0 {
				t.Fatal("preparation did not retain exactly one persistent key")
			}
			if previous == "new" {
				if _, err := os.Stat(store.Path); !errors.Is(err, os.ErrNotExist) {
					t.Fatal("preparation saved an unverified account")
				}
			} else if !bytes.Equal(original, savedBytes(t, store.Path)) {
				t.Fatal("preparation replaced the saved account")
			}
			reads, writes, deletes := secrets.getCalls, secrets.setCalls, secrets.deleteCalls
			secrets.failGet, secrets.failSet, secrets.failDelete = true, true, true
			changed := fixture()
			changed.Cookies[0].Value = "synthetic-new-login"
			if store.Save(context.Background(), changed) != nil {
				t.Fatal("completed login required native secret access")
			}
			if secrets.getCalls != reads || secrets.setCalls != writes || secrets.deleteCalls != deletes || secrets.unexpectedKeyWasTouched {
				t.Fatal("saving after login accessed native secrets")
			}
			secrets.failGet = false
			restarted := Store{Secrets: secrets, Path: store.Path}
			loaded, err := restarted.Load(NoPrompt)
			if err != nil || loaded.Cookies[0].Value != changed.Cookies[0].Value {
				t.Fatal("replacement session did not survive restart")
			}
			noTemporaryFiles(t, store.Path)
		})
	}
}

func TestUnpreparedSaveAndLogoutCannotOpenLateAuthorization(t *testing.T) {
	store, secrets := testStore(t)
	if store.Save(context.Background(), fixture()) != ErrStorage || secrets.getCalls != 0 || secrets.setCalls != 0 {
		t.Fatal("unprepared save accessed the native store")
	}
	if store.PrepareLogin(context.Background()) != nil {
		t.Fatal("preparation failed")
	}
	key := store.key
	if store.Delete(AllowPrompt) != nil || len(store.key) != 0 || !bytes.Equal(key, make([]byte, len(key))) {
		t.Fatal("logout retained the in-memory key")
	}
	reads, writes := secrets.getCalls, secrets.setCalls
	if store.Save(context.Background(), fixture()) != ErrStorage || secrets.getCalls != reads || secrets.setCalls != writes {
		t.Fatal("late login resurrected an account after logout")
	}
}

func TestVerifiedLoginReusesKeyWithoutReadingPreviousCiphertext(t *testing.T) {
	store, secrets := testStore(t)
	if saveAfterPrepare(context.Background(), &store, fixture()) != nil {
		t.Fatal("initial save failed")
	}
	key := append([]byte(nil), secrets.values[sessionKeyName]...)
	if os.WriteFile(store.Path, []byte("synthetic-corrupt-session"), 0600) != nil {
		t.Fatal("corrupt file setup failed")
	}
	// Existing keys must only be read, even if the store would refuse a write.
	secrets.failSet = true
	changed := fixture()
	index := 3
	changed.AccountIndex = &index
	changed.Cookies[0].Value = "synthetic-new-account"
	if saveAfterPrepare(context.Background(), &store, changed) != nil {
		t.Fatal("verified login depended on previous ciphertext or a key write")
	}
	if !bytes.Equal(key, secrets.values[sessionKeyName]) || secrets.setCalls != 1 || secrets.deleteCalls != 0 || len(secrets.values) != 1 {
		t.Fatal("new login rotated or duplicated the key")
	}
	restarted := Store{Secrets: secrets, Path: store.Path}
	loaded, err := restarted.Load(NoPrompt)
	if err != nil || *loaded.AccountIndex != index || loaded.Cookies[0].Value != changed.Cookies[0].Value {
		t.Fatal("new account did not survive restart")
	}
}

func TestVerifiedLoginFailurePreservesSavedAccount(t *testing.T) {
	for _, failure := range []string{"denied-key", "invalid-key", "cancel-after-get", "file-commit"} {
		t.Run(failure, func(t *testing.T) {
			store, secrets := testStore(t)
			if saveAfterPrepare(context.Background(), &store, fixture()) != nil {
				t.Fatal("initial save failed")
			}
			original := savedBytes(t, store.Path)
			store.clearKey() // A new app process must prepare native access again.
			key := append([]byte(nil), secrets.values[sessionKeyName]...)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			restore := func() {}
			switch failure {
			case "denied-key":
				secrets.failGet = true
			case "invalid-key":
				secrets.values[sessionKeyName] = []byte("synthetic-invalid-key")
			case "cancel-after-get":
				secrets.afterGet = cancel
			case "file-commit":
				secrets.afterGet = func() { restore = blockSessionLocation(t, store.Path) }
			}
			storedKey := append([]byte(nil), secrets.values[sessionKeyName]...)
			if saveAfterPrepare(ctx, &store, fixture()) != ErrStorage {
				t.Fatal("failed save was accepted")
			}
			restore()
			if !bytes.Equal(original, savedBytes(t, store.Path)) || !bytes.Equal(storedKey, secrets.values[sessionKeyName]) || len(secrets.values) != 1 || secrets.setCalls != 1 || secrets.deleteCalls != 0 {
				t.Fatal("failed save modified the session or the key")
			}
			secrets.failGet, secrets.afterGet = false, nil
			secrets.values[sessionKeyName] = key
			if _, err := store.Load(NoPrompt); err != nil {
				t.Fatal("previous session could not be restored after access recovered")
			}
			noTemporaryFiles(t, store.Path)
		})
	}
}

func TestMissingKeyIsCreatedDuringLoginPreparation(t *testing.T) {
	store, secrets := testStore(t)
	if saveAfterPrepare(context.Background(), &store, fixture()) != nil {
		t.Fatal("initial save failed")
	}
	key := append([]byte(nil), secrets.values[sessionKeyName]...)
	original := savedBytes(t, store.Path)
	delete(secrets.values, sessionKeyName)
	if _, err := store.Load(NoPrompt); err != ErrStorage || secrets.setCalls != 1 || !bytes.Equal(original, savedBytes(t, store.Path)) {
		t.Fatal("restore recreated a missing key or changed the saved session")
	}
	secrets.failSet = true
	if saveAfterPrepare(context.Background(), &store, fixture()) != ErrStorage || len(secrets.values) != 0 || !bytes.Equal(original, savedBytes(t, store.Path)) {
		t.Fatal("failed key creation damaged saved ciphertext")
	}
	secrets.failSet = false
	if saveAfterPrepare(context.Background(), &store, fixture()) != nil || len(secrets.values) != 1 || bytes.Equal(key, secrets.values[sessionKeyName]) {
		t.Fatal("verified login failed to create the missing key")
	}
	if _, err := store.Load(NoPrompt); err != nil {
		t.Fatal("newly saved session could not be restored")
	}
}

func TestFailedFirstFileCommitKeepsOneReusableKey(t *testing.T) {
	store, secrets := testStore(t)
	secrets.afterSet = func() {
		if os.Mkdir(store.Path, 0700) != nil {
			t.Fatal("cannot block initial file commit")
		}
	}
	if saveAfterPrepare(context.Background(), &store, fixture()) != ErrStorage || len(secrets.values) != 1 || secrets.deleteCalls != 0 {
		t.Fatal("failed file commit discarded the persistent key")
	}
	key := append([]byte(nil), secrets.values[sessionKeyName]...)
	noTemporaryFiles(t, store.Path)
	if os.Remove(store.Path) != nil {
		t.Fatal("cannot unblock initial file commit")
	}
	secrets.afterSet = nil
	restarted := Store{Secrets: secrets, Path: store.Path}
	reads := secrets.getCalls
	if _, err := restarted.Load(NoPrompt); err != ErrNotFound || secrets.getCalls != reads {
		t.Fatal("a key without a committed file was treated as a saved login")
	}
	if saveAfterPrepare(context.Background(), &restarted, fixture()) != nil || secrets.setCalls != 1 || !bytes.Equal(key, secrets.values[sessionKeyName]) {
		t.Fatal("retry after restart failed to reuse the persistent key")
	}
}

func TestLogoutResetsKeyEvenForDamagedOrMissingFile(t *testing.T) {
	for _, file := range []string{"valid", "damaged", "missing"} {
		t.Run(file, func(t *testing.T) {
			store, secrets := testStore(t)
			if saveAfterPrepare(context.Background(), &store, fixture()) != nil {
				t.Fatal("initial save failed")
			}
			key := append([]byte(nil), secrets.values[sessionKeyName]...)
			switch file {
			case "damaged":
				if os.WriteFile(store.Path, []byte("synthetic-corrupt-file"), 0600) != nil {
					t.Fatal("cannot damage saved session")
				}
			case "missing":
				if os.Remove(store.Path) != nil {
					t.Fatal("cannot remove saved session")
				}
			}
			secrets.values[sessionKeyName] = []byte("synthetic-invalid-key")
			secrets.failGet = true
			reads := secrets.getCalls
			if store.Delete(AllowPrompt) != nil || len(secrets.values) != 0 || secrets.getCalls != reads {
				t.Fatal("explicit reset required reading a key or a valid file")
			}
			if _, err := os.Stat(store.Path); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("logout retained saved session")
			}
			secrets.failGet = false
			if saveAfterPrepare(context.Background(), &store, fixture()) != nil || len(secrets.values) != 1 || bytes.Equal(key, secrets.values[sessionKeyName]) {
				t.Fatal("login after logout failed to create a fresh key")
			}
		})
	}
}

func TestSecretStoreInteractionPolicy(t *testing.T) {
	store, secrets := testStore(t)
	if saveAfterPrepare(context.Background(), &store, fixture()) != nil {
		t.Fatal("initial save failed")
	}
	if len(secrets.getInteractions) != 1 || secrets.getInteractions[0] != AllowPrompt {
		t.Fatal("interactive login did not allow native authorization")
	}
	for _, interaction := range []Interaction{NoPrompt, AllowPrompt} {
		if _, err := store.Load(interaction); err != nil || secrets.getInteractions[len(secrets.getInteractions)-1] != interaction {
			t.Fatal("restore lost the caller's authorization policy")
		}
		if store.Delete(interaction) != nil || secrets.deleteInteractions[len(secrets.deleteInteractions)-1] != interaction {
			t.Fatal("logout lost the caller's authorization policy")
		}
		if saveAfterPrepare(context.Background(), &store, fixture()) != nil {
			t.Fatal("fixture resave failed")
		}
	}
}
