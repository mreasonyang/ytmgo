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

// Interaction controls system authorization UI where the native store supports it.
// Automatic restoration is silent; user-initiated operations may request access.
type Interaction bool

const (
	NoPrompt    Interaction = false
	AllowPrompt Interaction = true
)

// Secrets is intentionally restricted to this application's native secret store.
// It stores only the encryption key; session data stays in an encrypted file.
type Secrets interface {
	Set(string, []byte) error
	Get(string, Interaction) ([]byte, error)
	Delete(string, Interaction) error
}

var ErrNotFound = errors.New("account_session_not_saved")
var ErrStorage = errors.New("account_secure_storage_unavailable")

const (
	sessionKeyName = "account-session-key"
	sessionKeySize = 32
	// The versioned header is also authenticated as GCM additional data.
	// Only this format is accepted; older sessions require a fresh sign-in.
	sessionFileHeader = "YTMSESS\x03"
	// NewGCMWithRandomNonce prepends a 12-byte nonce and appends a 16-byte tag.
	sessionFileOverhead = len(sessionFileHeader) + 12 + 16
	maxSessionFileSize  = MaxPayload + sessionFileOverhead
)

// Store keeps one persistent native key and one encrypted session file. Path must
// be an absolute path in a dedicated private directory. The app's single-instance
// lock and serial account worker serialize access to this key and file.
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
	if err != nil || len(data) > maxSessionFileSize {
		return nil, ErrStorage
	}
	if !validSessionFile(data) {
		return nil, ErrStorage
	}
	return data, nil
}

func validSessionFile(data []byte) bool {
	return bytes.HasPrefix(data, []byte(sessionFileHeader)) && len(data) > sessionFileOverhead && len(data) <= maxSessionFileSize
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
	if !validSessionFile(data) {
		return nil, ErrStorage
	}
	size := len(sessionFileHeader)
	plain, err := aead.Open(nil, nil, data[size:], data[:size])
	if err != nil {
		return nil, ErrStorage
	}
	return plain, nil
}

// Save persists a newly verified interactive sign-in with the existing key and a
// fresh GCM nonce. Only an explicitly missing key permits creation; denied or
// locked storage must not cause rotation. The old ciphertext need not be read.
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
	if privateSessionDirectory(s.Path, true) != nil || !safeSessionTarget(s.Path) {
		return ErrStorage
	}
	key, err := s.Secrets.Get(sessionKeyName, AllowPrompt)
	defer func() { clear(key) }()
	if ctx.Err() != nil {
		return ErrStorage
	}
	if errors.Is(err, ErrNotFound) {
		clear(key)
		key = make([]byte, sessionKeySize)
		if _, err := rand.Read(key); err != nil {
			return ErrStorage
		}
		if s.Secrets.Set(sessionKeyName, key) != nil {
			return ErrStorage
		}
	} else if err != nil {
		return ErrStorage
	}
	aead, err := sessionCipher(key)
	if err != nil {
		return err
	}
	header := []byte(sessionFileHeader)
	data := append(header, aead.Seal(nil, nil, plain, header)...)
	// Keep the persistent key even if the file commit fails. A retry reuses the
	// same item, including after a process crash between key and file writes.
	return writeSessionFile(ctx, s.Path, data)
}

func safeSessionTarget(path string) bool {
	info, err := os.Lstat(path)
	return errors.Is(err, os.ErrNotExist) || err == nil && info.Mode().IsRegular() && privateStorageMode(info.Mode(), 0600)
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
	if !safeSessionTarget(path) {
		return ErrStorage
	}
	if ctx.Err() != nil || os.Rename(file.Name(), path) != nil {
		return ErrStorage
	}
	return nil
}

func (s Store) Load(interaction Interaction) (Session, error) {
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
	key, err := s.Secrets.Get(sessionKeyName, interaction)
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

func (s Store) Delete(interaction Interaction) error {
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
	// signed-out session on the next launch. The fixed name also allows retrying
	// key deletion after a failure, even when the file is already gone or corrupt.
	if err := s.Secrets.Delete(sessionKeyName, interaction); err != nil && !errors.Is(err, ErrNotFound) {
		return ErrStorage
	}
	return nil
}
