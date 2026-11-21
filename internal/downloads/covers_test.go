package downloads

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"ytmgo/internal/audioquality"
	"ytmgo/internal/catalog"
)

func TestCoverIndexTracksCompletedWinnersAndIgnoresProgress(t *testing.T) {
	m, err := New(t.Context(), t.TempDir(), &testKeys{}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	old, new := song, song
	old.Artwork = "https://LH3.googleusercontent.com/album=s64-k"
	new.ID, new.Artwork = "lmnopqrstuv", "https://lh3.googleusercontent.com/album=w384-h384-k"
	query := "https://lh3.googleusercontent.com/album=s768-k"
	if err := m.Enqueue(Guest, []catalog.Item{old, new}, audioquality.Saver); err != nil {
		t.Fatal(err)
	}
	empty := m.Covers()
	<-m.CoverChanges()
	for _, id := range []string{old.ID, new.ID} {
		if err := m.SetPaused(id, true); err != nil {
			t.Fatal(err)
		}
	}
	m.Online(true)
	m.Online(false)
	m.mu.Lock()
	first, second := m.byID[old.ID], m.byID[new.ID]
	m.mu.Unlock()
	m.progress(first, 100)
	if m.Covers() != empty {
		t.Fatal("online or progress replaced the cover snapshot")
	}
	select {
	case <-m.CoverChanges():
		t.Fatal("online or progress signaled a cover change")
	default:
	}
	// The later persisted row wins even if the earlier row completes last.
	complete := func(j *job) {
		m.mu.Lock()
		defer m.mu.Unlock()
		j.State, j.HasCover = Complete, true
		m.addCover(j)
	}
	complete(second)
	selected := m.Covers()
	winner := selected.Source(query)
	if winner.Asset == nil || winner.Asset.job != second || selected.Source(old.Artwork) != winner {
		t.Fatal("the same canonical URL did not select the last saved job")
	}
	complete(first)
	if m.Covers() != selected {
		t.Fatal("an earlier completion replaced the existing winner")
	}
	if err := m.Remove(new.ID); err != nil {
		t.Fatal(err)
	}
	fallback := m.Covers().Source(query)
	if fallback.Asset == nil || fallback.Asset.job != first || fallback == winner || selected.Source(query) != winner {
		t.Fatal("removing the winner failed to publish the earlier download in a new snapshot")
	}
	winner.Asset.Reject(t.Context())
	if m.Covers().Source(query) != fallback {
		t.Fatal("a stale rejection withdrew its replacement")
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	fallback.Asset.Reject(ctx)
	if m.Covers().Source(query) != fallback {
		t.Fatal("a canceled worker withdrew its asset")
	}
	if err := m.Remove(old.ID); err != nil {
		t.Fatal(err)
	}
	if remote := m.Covers().Source(query); remote.Asset != nil || remote == empty.Source(query) {
		t.Fatal("return to HTTP reused its pre-download generation")
	}
	if remote := m.Covers().Source("https://i.ytimg.com/another"); remote != empty.Source("https://i.ytimg.com/another") {
		t.Fatal("an unrelated URL changed generation")
	}
}

func TestCoverAssetDistinguishesTransientReadFailureFromLoss(t *testing.T) {
	for _, err := range []error{syscall.EIO, os.ErrPermission, context.Canceled} {
		if errors.Is(coverReadError(&os.PathError{Op: "read", Path: "cover", Err: err}), ErrInvalidCover) {
			t.Fatalf("transient or canceled read classified as permanent: %v", err)
		}
	}
	if !errors.Is(coverReadError(&os.PathError{Op: "open", Path: "cover", Err: os.ErrNotExist}), ErrInvalidCover) {
		t.Fatal("missing cover was considered retryable")
	}
	m, err := New(t.Context(), t.TempDir(), &testKeys{}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	if err := m.Enqueue(Guest, []catalog.Item{song}, audioquality.Saver); err != nil {
		t.Fatal(err)
	}
	m.mu.Lock()
	j := m.byID[song.ID]
	dir := m.assetDir(j)
	j.State, j.HasCover = Complete, true
	m.addCover(j)
	m.mu.Unlock()
	path := filepath.Join(dir, "cover")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("first"), 0600); err != nil {
		t.Fatal(err)
	}
	asset := m.Covers().Source(song.Artwork).Asset
	if body, err := asset.Read(t.Context()); err != nil || string(body) != "first" {
		t.Fatal("could not bind the selected cover", err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("replacement"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := asset.Read(t.Context()); !errors.Is(err, ErrInvalidCover) {
		t.Fatal("replacement file was read under the old generation", err)
	}
	asset.Reject(t.Context())
	if source := m.Covers().Source(song.Artwork); source.Asset != nil || !m.Has(song.ID) {
		t.Fatal("only the changed cover should be withdrawn")
	}
}
