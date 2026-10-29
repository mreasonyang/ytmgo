package accountsession

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

type memorySecrets struct {
	values                  map[string][]byte
	getCalls, setCalls      int
	deleteCalls             int
	failGet, failSet        bool
	failDelete              bool
	afterGet, afterSet      func()
	unexpectedKeyWasTouched bool
}

func ownedFixtureKey(k string) bool {
	if k == sessionKeyName {
		return true
	}
	id, ok := strings.CutPrefix(k, sessionKeyName+"-")
	decoded, err := hex.DecodeString(id)
	return ok && err == nil && len(decoded) == keyIDSize
}

func (m *memorySecrets) Get(k string) ([]byte, error) {
	m.getCalls++
	m.unexpectedKeyWasTouched = m.unexpectedKeyWasTouched || !ownedFixtureKey(k)
	if m.afterGet != nil {
		m.afterGet()
	}
	if m.failGet {
		return nil, errors.New("synthetic-secret-detail-must-not-escape")
	}
	v, ok := m.values[k]
	if !ok {
		return nil, ErrNotFound
	}
	return append([]byte(nil), v...), nil
}
func (m *memorySecrets) Set(k string, v []byte) error {
	m.setCalls++
	m.unexpectedKeyWasTouched = m.unexpectedKeyWasTouched || !ownedFixtureKey(k)
	if m.failSet {
		return errors.New("synthetic-secret-detail-must-not-escape")
	}
	m.values[k] = append([]byte(nil), v...)
	if m.afterSet != nil {
		m.afterSet()
	}
	return nil
}
func (m *memorySecrets) Delete(k string) error {
	m.deleteCalls++
	m.unexpectedKeyWasTouched = m.unexpectedKeyWasTouched || !ownedFixtureKey(k)
	if m.failDelete {
		return errors.New("synthetic-secret-detail-must-not-escape")
	}
	delete(m.values, k)
	return nil
}

func testStore(t *testing.T) (Store, *memorySecrets) {
	t.Helper()
	m := &memorySecrets{values: map[string][]byte{}}
	return Store{Secrets: m, Path: filepath.Join(t.TempDir(), "account", "session.enc")}, m
}

func savedBytes(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal("cannot read synthetic encrypted session")
	}
	return data
}

func noTemporaryFiles(t *testing.T, path string) {
	t.Helper()
	files, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatal("cannot inspect synthetic session directory")
	}
	for _, file := range files {
		if strings.HasPrefix(file.Name(), ".session-") {
			t.Fatal("failed save left a temporary file")
		}
	}
}

func TestSecretStoreLargeEncryptedRoundTripAndOwnKeyDelete(t *testing.T) {
	store, m := testStore(t)
	m.values["unrelated"] = []byte("synthetic-unrelated")
	m.values["native-account-session-v1"] = []byte("synthetic-untouched-old-item")
	m.values["native-account-session-v1.old.0"] = []byte("synthetic-untouched-old-item")
	session := fixture()
	session.Cookies[0].Value = strings.Repeat("s", 9000)
	if store.Save(context.Background(), session) != nil {
		t.Fatal("save failed")
	}
	if len(m.values) != 4 || len(m.values[sessionKeyName]) != 32 || m.setCalls != 1 {
		t.Fatal("large session did not use exactly one 32-byte native secret")
	}
	data := savedBytes(t, store.Path)
	if bytes.Contains(data, []byte(session.Cookies[0].Value)) || bytes.Contains(data, []byte("SAPISID")) {
		t.Fatal("session file contains plaintext credentials")
	}
	for path, mode := range map[string]os.FileMode{store.Path: 0600, filepath.Dir(store.Path): 0700} {
		info, err := os.Stat(path)
		if err != nil || !privateStorageMode(info.Mode(), mode) {
			t.Fatal("session storage permissions are not private")
		}
	}
	restarted := Store{Secrets: m, Path: store.Path}
	loaded, err := restarted.Load()
	if err != nil || len(loaded.Cookies[0].Value) != 9000 || *loaded.AccountIndex != 2 {
		t.Fatal("new Store instance could not restore whole session")
	}
	if restarted.Delete() != nil || len(m.values) != 3 || m.values["unrelated"] == nil || m.unexpectedKeyWasTouched {
		t.Fatal("deletion retained the current key or touched another native secret")
	}
	if _, err := os.Stat(store.Path); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("deletion retained encrypted session")
	}
	if store.Delete() != nil {
		t.Fatal("repeat deletion was not idempotent")
	}
}

func TestSecretStoreMissingFileNeverReadsNativeSecrets(t *testing.T) {
	store, m := testStore(t)
	m.failGet = true
	if _, err := store.Load(); err != ErrNotFound || m.getCalls != 0 {
		t.Fatal("fresh install queried native secrets or failed to distinguish absence")
	}
	m.values[sessionKeyName] = bytes.Repeat([]byte{1}, 32)
	if _, err := store.Load(); err != ErrNotFound || m.getCalls != 0 {
		t.Fatal("orphan key caused native secret access without a session file")
	}
}

func TestSecretStoreReusesKeyWithFreshNonce(t *testing.T) {
	store, m := testStore(t)
	if store.Save(context.Background(), fixture()) != nil {
		t.Fatal("initial save failed")
	}
	first := savedBytes(t, store.Path)
	key := append([]byte(nil), m.values[sessionKeyName]...)
	if store.Save(context.Background(), fixture()) != nil {
		t.Fatal("second save failed")
	}
	second := savedBytes(t, store.Path)
	start := len(sessionFileHeader)
	if bytes.Equal(first, second) || bytes.Equal(first[start:start+12], second[start:start+12]) {
		t.Fatal("repeated save reused ciphertext or nonce")
	}
	if !bytes.Equal(key, m.values[sessionKeyName]) || m.setCalls != 1 {
		t.Fatal("repeated save replaced native key")
	}
	noTemporaryFiles(t, store.Path)
}

func TestSecretStoreCorruptionAndMissingKeysFailClosed(t *testing.T) {
	for _, test := range []string{"ciphertext", "header", "truncated", "empty", "oversized", "missing-key", "wrong-key", "short-key"} {
		t.Run(test, func(t *testing.T) {
			store, m := testStore(t)
			if store.Save(context.Background(), fixture()) != nil {
				t.Fatal("initial save failed")
			}
			data := savedBytes(t, store.Path)
			switch test {
			case "ciphertext":
				data[len(data)-1] ^= 1
			case "header":
				data[0] ^= 1
			case "truncated":
				data = data[:len(data)-1]
			case "empty":
				data = nil
			case "oversized":
				data = make([]byte, maxSessionFileSize+1)
			case "missing-key":
				delete(m.values, sessionKeyName)
			case "wrong-key":
				m.values[sessionKeyName][0] ^= 1
			case "short-key":
				m.values[sessionKeyName] = []byte("synthetic-short-key")
			}
			if os.WriteFile(store.Path, data, 0600) != nil {
				t.Fatal("synthetic corruption setup failed")
			}
			if loaded, err := store.Load(); err != ErrStorage || loaded.Cookies != nil {
				t.Fatal("damaged storage returned a session or unfixed error")
			}
			if store.Save(context.Background(), fixture()) != ErrStorage || m.setCalls != 1 || !bytes.Equal(data, savedBytes(t, store.Path)) {
				t.Fatal("save replaced corrupt storage or its key")
			}
			if store.Delete() != nil {
				t.Fatal("corrupt owned session could not be deleted")
			}
		})
	}
}

func TestSecretStoreFailedAndCanceledSavePreservesPrevious(t *testing.T) {
	store, m := testStore(t)
	if store.Save(context.Background(), fixture()) != nil {
		t.Fatal("initial save failed")
	}
	original := savedBytes(t, store.Path)
	changed := fixture()
	i := 3
	changed.AccountIndex = &i
	m.failGet = true
	if store.Save(context.Background(), changed) != ErrStorage {
		t.Fatal("native key error was not normalized")
	}
	m.failGet = false
	ctx, cancel := context.WithCancel(context.Background())
	m.afterGet = cancel
	if store.Save(ctx, changed) != ErrStorage {
		t.Fatal("save committed after cancellation during native key access")
	}
	m.afterGet = nil
	restoreDirectory := blockSessionLocation(t, store.Path)
	err := store.Save(context.Background(), changed)
	restoreDirectory()
	if err != ErrStorage || !bytes.Equal(original, savedBytes(t, store.Path)) || m.setCalls != 1 {
		t.Fatal("failed or canceled save changed previous ciphertext or key")
	}
	loaded, err := store.Load()
	if err != nil || *loaded.AccountIndex != 2 {
		t.Fatal("previous session no longer loads")
	}
	noTemporaryFiles(t, store.Path)
}

func TestSecretStoreCanceledInitialSaveAndFailedKeyCreation(t *testing.T) {
	store, m := testStore(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if store.Save(ctx, fixture()) != ErrStorage || m.getCalls != 0 || m.setCalls != 0 {
		t.Fatal("already canceled save accessed native secrets")
	}
	m.failSet = true
	if store.Save(context.Background(), fixture()) != ErrStorage || len(m.values) != 0 {
		t.Fatal("failed native key creation was accepted")
	}
	m.failSet = false
	ctx, cancel = context.WithCancel(context.Background())
	m.afterSet = cancel
	if store.Save(ctx, fixture()) != ErrStorage {
		t.Fatal("save committed after cancellation during key creation")
	}
	if _, err := os.Stat(store.Path); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("failed first save committed a session file")
	}
	m.afterSet = nil
	key := append([]byte(nil), m.values[sessionKeyName]...)
	sets := m.setCalls
	if store.Save(context.Background(), fixture()) != nil || m.setCalls != sets || !bytes.Equal(key, m.values[sessionKeyName]) {
		t.Fatal("unused valid key was not reused")
	}
}

// Cancel at the final write check to exercise temporary-file cleanup after data
// has been written, synced, and closed, before the atomic rename.
type cancelBeforeCommit struct {
	context.Context
	cancel context.CancelFunc
	checks int
}

func (c *cancelBeforeCommit) Err() error {
	c.checks++
	if c.checks == 2 {
		c.cancel()
	}
	return c.Context.Err()
}

func TestSessionFileCanceledCommitPreservesPrevious(t *testing.T) {
	store, _ := testStore(t)
	if store.Save(context.Background(), fixture()) != nil {
		t.Fatal("initial save failed")
	}
	original := savedBytes(t, store.Path)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if writeSessionFile(&cancelBeforeCommit{Context: ctx, cancel: cancel}, store.Path, []byte("synthetic-replacement")) != ErrStorage || !bytes.Equal(original, savedBytes(t, store.Path)) {
		t.Fatal("cancel before rename changed the committed file")
	}
	noTemporaryFiles(t, store.Path)
}

func TestSecretStoreRejectsUnsafePaths(t *testing.T) {
	for _, kind := range []string{"empty", "relative", "file-symlink", "directory-symlink", "directory-target", "public-directory"} {
		t.Run(kind, func(t *testing.T) {
			store, m := testStore(t)
			if os.MkdirAll(filepath.Dir(store.Path), 0700) != nil {
				t.Fatal("path setup failed")
			}
			switch kind {
			case "empty":
				store.Path = ""
			case "relative":
				store.Path = "session.enc"
			case "file-symlink":
				target := filepath.Join(t.TempDir(), "other")
				if os.WriteFile(target, []byte("synthetic-unrelated-file"), 0600) != nil {
					t.Fatal("target setup failed")
				}
				if err := os.Symlink(target, store.Path); err != nil {
					if runtime.GOOS == "windows" {
						t.Skip("Windows symlink creation requires Developer Mode or privileges")
					}
					t.Fatal("symlink setup failed")
				}
			case "directory-symlink":
				dir := filepath.Dir(store.Path)
				if os.Remove(dir) != nil {
					t.Fatal("directory setup failed")
				}
				if err := os.Symlink(t.TempDir(), dir); err != nil {
					if runtime.GOOS == "windows" {
						t.Skip("Windows symlink creation requires Developer Mode or privileges")
					}
					t.Fatal("directory symlink setup failed")
				}
			case "directory-target":
				if os.Mkdir(store.Path, 0700) != nil {
					t.Fatal("directory target setup failed")
				}
			case "public-directory":
				if runtime.GOOS == "windows" {
					t.Skip("POSIX mode bits do not describe Windows ACLs")
				}
				if os.Chmod(filepath.Dir(store.Path), 0755) != nil {
					t.Fatal("permission setup failed")
				}
			}
			if store.Save(context.Background(), fixture()) != ErrStorage || m.getCalls != 0 || m.setCalls != 0 {
				t.Fatal("unsafe path reached native secret access")
			}
		})
	}
}

func TestSecretStoreDeleteFailuresAreReported(t *testing.T) {
	store, m := testStore(t)
	if store.Save(context.Background(), fixture()) != nil {
		t.Fatal("initial save failed")
	}
	restoreDirectory := blockSessionLocation(t, store.Path)
	err := store.Delete()
	restoreDirectory()
	if err != ErrStorage || m.deleteCalls != 0 {
		t.Fatal("failed file removal was hidden or deleted key first")
	}
	m.failDelete = true
	if store.Delete() != ErrStorage {
		t.Fatal("native key deletion error was hidden")
	}
	gets := m.getCalls
	if _, err := store.Load(); err != ErrNotFound || m.getCalls != gets {
		t.Fatal("failed key deletion restored signed-out session")
	}
	m.failDelete = false
	if store.Delete() != nil || len(m.values) != 0 {
		t.Fatal("could not retry native key deletion")
	}
}

// Replacing the session file by a directory models an unavailable storage
// location on every OS, without treating chmod as a Windows ACL.
func blockSessionLocation(t *testing.T, path string) func() {
	t.Helper()
	backup := path + ".synthetic-held"
	if os.Rename(path, backup) != nil || os.Mkdir(path, 0700) != nil {
		t.Fatal("cannot block synthetic session location")
	}
	return func() {
		t.Helper()
		if os.Remove(path) != nil || os.Rename(backup, path) != nil {
			t.Fatal("cannot restore synthetic session location")
		}
	}
}

// Public POSIX permissions must be rejected before the encryption key is read.
// Windows relies on the per-user AppData ACL and native credential store instead.
func TestSecretStoreRejectsPublicCiphertextPermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX mode bits do not describe Windows ACLs")
	}
	store, secrets := testStore(t)
	if store.Save(context.Background(), fixture()) != nil {
		t.Fatal("save failed")
	}
	reads := secrets.getCalls
	if os.Chmod(store.Path, 0644) != nil {
		t.Fatal("mode setup failed")
	}
	if _, err := store.Load(); err != ErrStorage || secrets.getCalls != reads {
		t.Fatal("public ciphertext reached native secret access")
	}
	if os.Chmod(store.Path, 0600) != nil {
		t.Fatal("mode restore failed")
	}
}

func TestSecretStoreLoadPreservesExpiryError(t *testing.T) {
	store, m := testStore(t)
	if store.Save(context.Background(), fixture()) != nil {
		t.Fatal("initial save failed")
	}
	session := fixture()
	expired := float64(time.Now().Add(-time.Minute).Unix())
	session.Cookies[0].ExpirationDate = &expired
	plain, err := json.Marshal(session)
	if err != nil {
		t.Fatal("synthetic session serialization failed")
	}
	aead, err := sessionCipher(m.values[sessionKeyName])
	if err != nil {
		t.Fatal("synthetic cipher setup failed")
	}
	data := append([]byte(sessionFileHeader), aead.Seal(nil, nil, plain, []byte(sessionFileHeader))...)
	if os.WriteFile(store.Path, data, 0600) != nil {
		t.Fatal("synthetic expiry setup failed")
	}
	if _, err := store.Load(); err != ErrExpired {
		t.Fatal("expiry was changed into a storage failure")
	}
	if store.Delete() != nil || len(m.values) != 0 {
		t.Fatal("expired session could not be removed")
	}
}
