package downloads

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"testing"
	"time"

	"ytmgo/internal/accountsession"
	"ytmgo/internal/audioquality"
	"ytmgo/internal/catalog"
)

type testKeys struct{ key []byte }

func (s *testKeys) Get(name string, _ accountsession.Interaction) ([]byte, error) {
	if name != masterKeyName || s.key == nil {
		return nil, accountsession.ErrNotFound
	}
	return slices.Clone(s.key), nil
}
func (s *testKeys) Set(name string, key []byte) error {
	if name != masterKeyName {
		return ErrKey
	}
	s.key = slices.Clone(key)
	return nil
}
func (s *testKeys) Delete(string, accountsession.Interaction) error {
	return errors.New("unexpected delete")
}

var song = catalog.Item{ID: "abcdefghijk", Title: "Recorded song", Kind: "song", Artwork: "https://i.ytimg.com/vi/abcdefghijk/hqdefault.jpg"}

func audioFixture() []byte {
	return bytes.Repeat([]byte("0123456789abcdef---private-audio-content---"), 17123)
}

func remoteFixture(data []byte, version string, open func(context.Context, int64) (io.ReadCloser, error)) Remote {
	if open == nil {
		open = func(_ context.Context, start int64) (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewReader(data[start:])), nil
		}
	}
	return Remote{Format: Format{ITag: 140, Version: version, Type: "audio/mp4", Length: int64(len(data)), Duration: time.Minute}, Open: open, Close: func() {}}
}

func waitEntry(t *testing.T, m *Manager, until func(Entry) bool) Entry {
	t.Helper()
	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	for {
		rows := m.Snapshot()
		if len(rows) > 0 && until(rows[0]) {
			return rows[0]
		}
		select {
		case <-m.Changes():
		case <-deadline.C:
			t.Fatalf("download did not reach expected state: %+v", rows)
		}
	}
}

func TestEncryptedOfflineRestartRangesAndRemoval(t *testing.T) {
	dir := t.TempDir()
	keys := &testKeys{}
	data := audioFixture()
	open := func(resolve Resolve) *Manager {
		m, err := New(t.Context(), dir, keys, resolve, func(context.Context, catalog.Item) (catalog.LyricsResult, []byte) {
			return catalog.LyricsResult{Text: "lyrics"}, []byte("cover")
		})
		if err != nil {
			t.Fatal(err)
		}
		return m
	}
	m := open(func(_ context.Context, id string, q audioquality.Quality) (Remote, error) {
		if id != song.ID || q != audioquality.Saver {
			return Remote{}, ErrMedia
		}
		return remoteFixture(data, "101", nil), nil
	})
	scope := "0123456789abcdef01234567"
	if err := m.Switch(scope, false); err != nil {
		t.Fatal(err)
	}
	if err := m.Enqueue(scope, []catalog.Item{song, song}, audioquality.Saver); err != nil {
		t.Fatal(err)
	}
	m.Online(true)
	got := waitEntry(t, m, func(e Entry) bool { return e.State == Complete })
	if got.Bytes != int64(len(data)) || len(m.Snapshot()) != 1 {
		t.Fatal("incomplete or duplicate download")
	}
	m.Close()
	// Pretend this device has already been offline for four months.
	old := time.Now().Add(-120 * 24 * time.Hour)
	var mediaPath string
	if err := filepath.WalkDir(dir, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if bytes.Contains(b, data[:64]) || bytes.Contains(b, keys.key) {
			t.Fatal("plaintext audio or master key on disk")
		}
		if entry.Name() == "media" {
			mediaPath = path
		}
		return os.Chtimes(path, old, old)
	}); err != nil {
		t.Fatal(err)
	}
	m = open(func(context.Context, string, audioquality.Quality) (Remote, error) {
		t.Error("unexpected online lookup")
		return Remote{}, errTransfer
	})
	defer m.Close()
	src, _, err := m.OpenTrack(song.ID)
	if err != nil || src == nil {
		t.Fatal("offline restart failed", err)
	}
	if l, _ := m.Lyrics(song.ID); l.Text != "lyrics" || string(m.Cover(song.Artwork)) != "cover" {
		t.Fatal("offline extras missing")
	}
	get := func(r string) *httptest.ResponseRecorder {
		req := httptest.NewRequest("GET", "/audio", nil)
		req.Header.Set("Range", r)
		out := httptest.NewRecorder()
		src.ServeHTTP(out, req)
		return out
	}
	out := get("bytes=262137-262162")
	if out.Code != 206 || !bytes.Equal(out.Body.Bytes(), data[262137:262163]) {
		t.Fatal("cross-record seek failed")
	}
	if out = get("bytes=-19"); out.Code != 206 || !bytes.Equal(out.Body.Bytes(), data[len(data)-19:]) {
		t.Fatal("suffix seek failed")
	}
	if out = get(""); out.Code != 200 || !bytes.Equal(out.Body.Bytes(), data) {
		t.Fatal("full decrypted track differs")
	}
	if out = get("bytes=9000000-"); out.Code != http.StatusRequestedRangeNotSatisfiable {
		t.Fatal("invalid range was accepted")
	}
	f, err := os.OpenFile(mediaPath, os.O_RDWR, 0600)
	if err != nil {
		t.Fatal(err)
	}
	at := int64(chunkSize + recordOverhead + 20)
	var b [1]byte
	if _, err = f.ReadAt(b[:], at); err != nil {
		t.Fatal(err)
	}
	b[0] ^= 128
	if _, err = f.WriteAt(b[:], at); err != nil {
		t.Fatal(err)
	}
	f.Close()
	out = get("bytes=262144-262168")
	if out.Body.Len() != 0 || src.Err() == nil {
		t.Fatal("tampered chunk returned audio")
	}
	if entry := m.Snapshot()[0]; entry.State != Failed || entry.Failure != "media" {
		t.Fatal("damaged download cannot be retried from the manager")
	}
	if err = m.Remove(song.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(mediaPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("removed audio still on disk")
	}
	if src.Err() == nil || m.Has(song.ID) {
		t.Fatal("deleted track remains playable")
	}
}

func TestSignOutDuringDownloadRemovesOnlyAccountAssets(t *testing.T) {
	root := t.TempDir()
	started := make(chan struct{})
	data := audioFixture()
	resolve := func(context.Context, string, audioquality.Quality) (Remote, error) {
		return remoteFixture(data, "100", func(ctx context.Context, _ int64) (io.ReadCloser, error) {
			close(started)
			<-ctx.Done()
			return nil, ctx.Err()
		}), nil
	}
	m, err := New(t.Context(), root, &testKeys{}, resolve, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	if err = m.Enqueue(Guest, []catalog.Item{song}, audioquality.Saver); err != nil {
		t.Fatal(err)
	}
	scope := "abababababababababababab"
	if err = m.Switch(scope, false); err != nil {
		t.Fatal(err)
	}
	if err = m.Enqueue(scope, []catalog.Item{song}, audioquality.Best); err != nil {
		t.Fatal(err)
	}
	m.Online(true)
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("transfer did not start")
	}
	if err = m.Switch(Guest, true); err != nil {
		t.Fatal(err)
	}
	m.Close()
	if _, err = os.Stat(filepath.Join(root, scope)); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("signed-out account retained ciphertext or a wrapped key")
	}
	if rows := m.Snapshot(); len(rows) != 1 || rows[0].Quality != audioquality.Saver || rows[0].State != Queued {
		t.Fatal("guest queue was altered by account sign-out")
	}
}

func TestPausedRestartAndRepresentationChange(t *testing.T) {
	for _, change := range []bool{false, true} {
		name := "same version"
		if change {
			name = "new version"
		}
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			keys := &testKeys{}
			data := audioFixture()
			started := make(chan struct{})
			resolve := func(context.Context, string, audioquality.Quality) (Remote, error) {
				return remoteFixture(data, "101", func(ctx context.Context, _ int64) (io.ReadCloser, error) {
					r, w := io.Pipe()
					go func() { _, _ = w.Write(data[:chunkSize+77]); close(started); <-ctx.Done(); w.CloseWithError(ctx.Err()) }()
					return r, nil
				}), nil
			}
			m, err := New(t.Context(), dir, keys, resolve, nil)
			if err != nil {
				t.Fatal(err)
			}
			m.Online(true)
			if err = m.Enqueue(Guest, []catalog.Item{song}, audioquality.Standard); err != nil {
				t.Fatal(err)
			}
			select {
			case <-started:
			case <-time.After(5 * time.Second):
				t.Fatal("download didn't start")
			}
			if err = m.SetPaused(song.ID, true); err != nil {
				t.Fatal(err)
			}
			m.Close()
			version := "101"
			want := int64(chunkSize)
			if change {
				version = "102"
				want = 0
				data = bytes.Repeat([]byte{42}, len(data))
			}
			var mu sync.Mutex
			var ranges []int64
			m, err = New(t.Context(), dir, keys, func(ctx context.Context, id string, q audioquality.Quality) (Remote, error) {
				return remoteFixture(data, version, func(ctx context.Context, at int64) (io.ReadCloser, error) {
					mu.Lock()
					ranges = append(ranges, at)
					mu.Unlock()
					return io.NopCloser(bytes.NewReader(data[at:])), nil
				}), nil
			}, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer m.Close()
			if row := m.Snapshot()[0]; row.State != Paused || row.Bytes != chunkSize {
				t.Fatalf("bad restart progress %+v", row)
			}
			m.Online(true)
			if err = m.SetPaused(song.ID, false); err != nil {
				t.Fatal(err)
			}
			waitEntry(t, m, func(e Entry) bool { return e.State == Complete })
			mu.Lock()
			if len(ranges) != 1 || ranges[0] != want {
				t.Errorf("unexpected resume ranges %v", ranges)
			}
			mu.Unlock()
			src, _, err := m.OpenTrack(song.ID)
			if err != nil {
				t.Fatal(err)
			}
			out := httptest.NewRecorder()
			src.ServeHTTP(out, httptest.NewRequest("GET", "/audio", nil))
			if !bytes.Equal(out.Body.Bytes(), data) {
				t.Fatal("mixed representations")
			}
			if err = m.Switch("aaaaaaaaaaaaaaaaaaaaaaaa", false); err != nil {
				t.Fatal(err)
			}
			if len(m.Snapshot()) != 0 || src.Err() == nil {
				t.Fatal("stale scope remained accessible")
			}
			if err = m.Enqueue(Guest, []catalog.Item{song}, audioquality.Best); !errors.Is(err, context.Canceled) {
				t.Fatal("stale scope accepted new work")
			}
		})
	}
}
