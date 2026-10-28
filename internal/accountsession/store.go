package accountsession

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"time"
)

// Secrets is intentionally restricted to this application's native secret store.
// It stores only the encryption key; session data stays in an encrypted file.
type Secrets interface {
	Set(string, []byte) error
	Get(string) ([]byte, error)
	Delete(string) error
}

var ErrNotFound = errors.New("account_session_not_saved")
var ErrStorage = errors.New("account_secure_storage_unavailable")

const (
	sessionKeyName = "native-account-session-key"
	sessionKeySize = 32
	// The versioned header is also authenticated as GCM additional data.
	sessionFileHeader = "YTMSESS\x01"
	// NewGCMWithRandomNonce prepends a 12-byte nonce and appends a 16-byte tag.
	sessionFileOverhead = len(sessionFileHeader) + 12 + 16
	maxSessionFileSize  = MaxPayload + sessionFileOverhead
)

// Store keeps one native secret and one encrypted session file. Path must be an
// absolute path in a dedicated private directory. Callers serialize mutations.
type Store struct {
	Secrets Secrets
	Path    string
}

// POSIX mode bits protect the dedicated directory and ciphertext file. Windows
// does not represent ACLs through FileMode.Perm: the caller uses its per-user
// AppData directory and its native Credential Manager for the AES key. Chmod is
// not an ACL implementation, and the session remains encrypted on every OS.
func privateStorageMode(mode, expected os.FileMode) bool {
	return runtime.GOOS == "windows" || mode.Perm() == expected
}

func (s Store) validPath() bool {
	return filepath.IsAbs(s.Path) && filepath.Clean(s.Path) == s.Path && filepath.Dir(s.Path) != s.Path && filepath.Dir(s.Path) != filepath.VolumeName(s.Path)+string(filepath.Separator)
}

func privateSessionDirectory(path string, create bool) error {
	dir := filepath.Dir(path)
	if create {
		if err := os.MkdirAll(dir, 0700); err != nil {
			return ErrStorage
		}
	}
	info, err := os.Lstat(dir)
	if err != nil || !info.IsDir() || !privateStorageMode(info.Mode(), 0700) {
		return ErrStorage
	}
	return nil
}

// readSessionFile bounds reads and rejects links and special files before any
// Keychain access. An absent file is the complete fresh-install check.
func readSessionFile(path string) ([]byte, error) {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, ErrNotFound
	}
	if err != nil || !info.Mode().IsRegular() || !privateStorageMode(info.Mode(), 0600) || info.Size() <= int64(sessionFileOverhead) || info.Size() > int64(maxSessionFileSize) {
		return nil, ErrStorage
	}
	if err := privateSessionDirectory(path, false); err != nil {
		return nil, err
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, ErrStorage
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !opened.Mode().IsRegular() || !privateStorageMode(opened.Mode(), 0600) || !os.SameFile(info, opened) {
		return nil, ErrStorage
	}
	data, err := io.ReadAll(io.LimitReader(file, int64(maxSessionFileSize+1)))
	if err != nil || len(data) <= sessionFileOverhead || len(data) > maxSessionFileSize || !bytes.HasPrefix(data, []byte(sessionFileHeader)) {
		return nil, ErrStorage
	}
	return data, nil
}

func sessionCipher(key []byte) (cipher.AEAD, error) {
	if len(key) != sessionKeySize {
		return nil, ErrStorage
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, ErrStorage
	}
	aead, err := cipher.NewGCMWithRandomNonce(block)
	if err != nil {
		return nil, ErrStorage
	}
	return aead, nil
}

func openSessionFile(aead cipher.AEAD, data []byte) ([]byte, error) {
	plain, err := aead.Open(nil, nil, data[len(sessionFileHeader):], []byte(sessionFileHeader))
	if err != nil {
		return nil, ErrStorage
	}
	return plain, nil
}

func (s Store) Save(ctx context.Context, session Session) error {
	if ctx.Err() != nil || !s.validPath() || s.Secrets == nil {
		return ErrStorage
	}
	if _, err := session.HTTPCookies(time.Now()); err != nil {
		if errors.Is(err, ErrExpired) {
			return ErrExpired
		}
		return ErrStorage
	}
	plain, err := json.Marshal(session)
	defer clear(plain)
	if err != nil || len(plain) > MaxPayload {
		return ErrStorage
	}
	if err := privateSessionDirectory(s.Path, true); err != nil {
		return err
	}
	previous, err := readSessionFile(s.Path)
	if err != nil && !errors.Is(err, ErrNotFound) {
		return ErrStorage
	}
	fileExists := err == nil
	if ctx.Err() != nil {
		return ErrStorage
	}
	key, err := s.Secrets.Get(sessionKeyName)
	defer func() { clear(key) }()
	if ctx.Err() != nil {
		return ErrStorage
	}
	if errors.Is(err, ErrNotFound) && !fileExists {
		clear(key)
		key = make([]byte, sessionKeySize)
		if _, err := rand.Read(key); err != nil {
			return ErrStorage
		}
		if ctx.Err() != nil || s.Secrets.Set(sessionKeyName, key) != nil {
			return ErrStorage
		}
		// If a first save fails after this point, keep the unused key and reuse it
		// next time. Rolling it back could remove a key needed by another caller.
	} else if err != nil {
		return ErrStorage
	}
	if ctx.Err() != nil {
		return ErrStorage
	}
	aead, err := sessionCipher(key)
	if err != nil {
		return err
	}
	if fileExists {
		old, err := openSessionFile(aead, previous)
		clear(old)
		if err != nil {
			return err
		}
	}
	data := append([]byte(sessionFileHeader), aead.Seal(nil, nil, plain, []byte(sessionFileHeader))...)
	return writeSessionFile(ctx, s.Path, data)
}

func writeSessionFile(ctx context.Context, path string, data []byte) error {
	if ctx.Err() != nil || privateSessionDirectory(path, false) != nil {
		return ErrStorage
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".session-*.tmp")
	if err != nil {
		return ErrStorage
	}
	defer func() {
		_ = file.Close()
		_ = os.Remove(file.Name())
	}()
	if n, err := file.Write(data); err != nil || n != len(data) {
		return ErrStorage
	}
	if file.Sync() != nil || file.Close() != nil {
		return ErrStorage
	}
	// Never replace a link or special file, including one introduced while a
	// native secret-store prompt was open.
	info, err := os.Lstat(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) || err == nil && (!info.Mode().IsRegular() || !privateStorageMode(info.Mode(), 0600)) {
		return ErrStorage
	}
	if ctx.Err() != nil || os.Rename(file.Name(), path) != nil {
		return ErrStorage
	}
	return nil
}

func (s Store) Load() (Session, error) {
	if !s.validPath() {
		return Session{}, ErrStorage
	}
	data, err := readSessionFile(s.Path)
	if err != nil {
		return Session{}, err
	}
	if s.Secrets == nil {
		return Session{}, ErrStorage
	}
	key, err := s.Secrets.Get(sessionKeyName)
	defer clear(key)
	if err != nil {
		return Session{}, ErrStorage
	}
	aead, err := sessionCipher(key)
	if err != nil {
		return Session{}, err
	}
	plain, err := openSessionFile(aead, data)
	defer clear(plain)
	if err != nil {
		return Session{}, err
	}
	session, err := Decode(plain, time.Now())
	if errors.Is(err, ErrExpired) {
		return Session{}, ErrExpired
	}
	if err != nil {
		return Session{}, ErrStorage
	}
	return session, nil
}

func (s Store) Delete() error {
	if !s.validPath() || s.Secrets == nil {
		return ErrStorage
	}
	info, err := os.Lstat(s.Path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return ErrStorage
	}
	if err == nil {
		if !info.Mode().IsRegular() || !privateStorageMode(info.Mode(), 0600) || privateSessionDirectory(s.Path, false) != nil || os.Remove(s.Path) != nil {
			return ErrStorage
		}
	}
	// Remove the file first: a failure deleting the key must not restore a
	// signed-out session on the next launch.
	if err := s.Secrets.Delete(sessionKeyName); err != nil && !errors.Is(err, ErrNotFound) {
		return ErrStorage
	}
	return nil
}
