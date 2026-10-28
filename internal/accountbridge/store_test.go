package accountbridge

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

type memorySecrets struct {
	values  map[string][]byte
	failSet string
	max     int
}

func (m *memorySecrets) Get(k string) ([]byte, error) {
	v, ok := m.values[k]
	if !ok {
		return nil, ErrNotFound
	}
	return append([]byte(nil), v...), nil
}
func (m *memorySecrets) Set(k string, v []byte) error {
	if k == m.failSet {
		return errors.New("synthetic-denied")
	}
	if len(v) > m.max {
		m.max = len(v)
	}
	m.values[k] = append([]byte(nil), v...)
	return nil
}
func (m *memorySecrets) Delete(k string) error { delete(m.values, k); return nil }
func TestSecretStoreChunkedRoundTripAndOwnKeyDelete(t *testing.T) {
	m := &memorySecrets{values: map[string][]byte{"unrelated": []byte("synthetic-unrelated")}}
	store := Store{Secrets: m}
	if _, err := store.Load(); !errors.Is(err, ErrNotFound) {
		t.Fatal("missing secret not distinguished")
	}
	session := fixture()
	session.Cookies[0].Value = strings.Repeat("s", 9000)
	if store.Save(context.Background(), session) != nil {
		t.Fatal("save failed")
	}
	if m.max > chunkSize {
		t.Fatal("native secret size limit exceeded")
	}
	loaded, err := store.Load()
	if err != nil || len(loaded.Cookies[0].Value) != 9000 {
		t.Fatal("chunked roundtrip failed")
	}
	if len(m.values) < 5 {
		t.Fatal("large session was not chunked")
	}
	if store.Delete() != nil || len(m.values) != 1 || m.values["unrelated"] == nil {
		t.Fatal("delete crossed app key boundary or retained session")
	}
}
func TestSecretStoreFailedCommitPreservesPreviousSession(t *testing.T) {
	m := &memorySecrets{values: map[string][]byte{}}
	store := Store{Secrets: m}
	session := fixture()
	if store.Save(context.Background(), session) != nil {
		t.Fatal("initial save failed")
	}
	originalCount := len(m.values)
	m.failSet = rootKey
	changed := fixture()
	i := 3
	changed.AccountIndex = &i
	if !errors.Is(store.Save(context.Background(), changed), ErrStorage) {
		t.Fatal("failed root commit accepted")
	}
	loaded, err := store.Load()
	if err != nil || *loaded.AccountIndex != 2 || len(m.values) != originalCount {
		t.Fatal("failed save changed prior session or retained temporary chunks")
	}
}
func TestSecretStoreCanceledWriteAndCorruptIndexFailClosed(t *testing.T) {
	m := &memorySecrets{values: map[string][]byte{}}
	store := Store{Secrets: m}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if store.Save(ctx, fixture()) == nil || len(m.values) != 0 {
		t.Fatal("canceled save retained credentials")
	}
	m.values[rootKey] = []byte(`{"version":1,"slot":"../../unrelated","parts":1}`)
	if _, err := store.Load(); !errors.Is(err, ErrStorage) {
		t.Fatal("untrusted index accepted")
	}
	if !errors.Is(store.Delete(), ErrStorage) {
		t.Fatal("corrupt index used for deletion")
	}
}

func TestSecretStoreLoadPreservesExpiryError(t *testing.T) {
	m := &memorySecrets{values: map[string][]byte{}}
	store := Store{Secrets: m}
	if store.Save(context.Background(), fixture()) != nil {
		t.Fatal("synthetic save failed")
	}
	i, err := store.readIndex()
	if err != nil || i.Parts != 1 {
		t.Fatal("synthetic index unavailable")
	}
	s := fixture()
	expired := float64(time.Now().Add(-time.Minute).Unix())
	s.Cookies[0].ExpirationDate = &expired
	data, _ := json.Marshal(s)
	m.values[chunkKey(i, 0)] = data
	if _, err := store.Load(); !errors.Is(err, ErrExpired) {
		t.Fatal("secure store changed expiry into storage failure")
	}
	if store.Delete() != nil || len(m.values) != 0 {
		t.Fatal("expired app session could not be removed")
	}
}
