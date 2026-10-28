package accountbridge

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"time"
)

// Secrets is intentionally restricted to this application's native secret store.
// No filesystem or browser credential store is accepted as a fallback.
type Secrets interface {
	Set(string, []byte) error
	Get(string) ([]byte, error)
	Delete(string) error
}

var ErrNotFound = errors.New("account_session_not_saved")
var ErrStorage = errors.New("account_secure_storage_unavailable")

const rootKey = "native-account-session-v1"
const chunkSize = 2048

type Store struct{ Secrets Secrets }
type index struct {
	Version int    `json:"version"`
	Slot    string `json:"slot"`
	Parts   int    `json:"parts"`
}

var slotPattern = regexp.MustCompile(`^[a-f0-9]{32}$`)

func (s Store) readIndex() (index, error) {
	data, err := s.Secrets.Get(rootKey)
	defer clear(data)
	if errors.Is(err, ErrNotFound) {
		return index{}, ErrNotFound
	}
	if err != nil {
		return index{}, ErrStorage
	}
	var i index
	if json.Unmarshal(data, &i) != nil || i.Version != 1 || !slotPattern.MatchString(i.Slot) || i.Parts < 1 || i.Parts > (MaxPayload+chunkSize-1)/chunkSize {
		return index{}, ErrStorage
	}
	return i, nil
}
func chunkKey(i index, part int) string { return fmt.Sprintf("%s.%s.%d", rootKey, i.Slot, part) }
func (s Store) removeChunks(i index) error {
	var failed bool
	for n := 0; n < i.Parts; n++ {
		if err := s.Secrets.Delete(chunkKey(i, n)); err != nil && !errors.Is(err, ErrNotFound) {
			failed = true
		}
	}
	if failed {
		return ErrStorage
	}
	return nil
}
func (s Store) Save(ctx context.Context, session Session) error {
	if _, err := session.HTTPCookies(time.Now()); err != nil {
		return err
	}
	old, err := s.readIndex()
	if err != nil && !errors.Is(err, ErrNotFound) {
		return ErrStorage
	}
	data, err := json.Marshal(session)
	defer clear(data)
	if err != nil || len(data) > MaxPayload {
		return ErrStorage
	}
	random := make([]byte, 16)
	if _, err = rand.Read(random); err != nil {
		return ErrStorage
	}
	i := index{Version: 1, Slot: hex.EncodeToString(random), Parts: (len(data) + chunkSize - 1) / chunkSize}
	clear(random)
	committed := false
	defer func() {
		if !committed {
			_ = s.removeChunks(i)
		}
	}()
	for n := 0; n < i.Parts; n++ {
		if ctx.Err() != nil {
			return ErrStorage
		}
		if s.Secrets.Set(chunkKey(i, n), data[n*chunkSize:min((n+1)*chunkSize, len(data))]) != nil {
			return ErrStorage
		}
	}
	if ctx.Err() != nil {
		return ErrStorage
	}
	metadata, _ := json.Marshal(i)
	if s.Secrets.Set(rootKey, metadata) != nil {
		return ErrStorage
	}
	committed = true
	if old.Parts > 0 {
		_ = s.removeChunks(old)
	}
	return nil
}
func (s Store) Load() (Session, error) {
	i, err := s.readIndex()
	if err != nil {
		return Session{}, err
	}
	data := make([]byte, 0, i.Parts*chunkSize)
	defer func() { clear(data) }()
	for n := 0; n < i.Parts; n++ {
		part, err := s.Secrets.Get(chunkKey(i, n))
		if err != nil || len(part) == 0 || len(part) > chunkSize {
			clear(part)
			return Session{}, ErrStorage
		}
		data = append(data, part...)
		clear(part)
	}
	return Decode(data, time.Now())
}
func (s Store) Delete() error {
	i, err := s.readIndex()
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if s.Secrets.Delete(rootKey) != nil {
		return ErrStorage
	}
	return s.removeChunks(i)
}
