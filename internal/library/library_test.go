package library

import (
	"os"
	"path/filepath"
	"testing"
	"ytmgo/internal/catalog"
)

func TestRoundTripAndValidationPreservesSavedData(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library.json")
	store := NewStore(path)
	initial, err := store.Load()
	if err != nil || initial.Liked == nil || initial.Playlists == nil {
		t.Fatal("missing library should be empty", err)
	}
	value := Empty()
	value.Liked = append(value.Liked, catalog.Track{ID: "dQw4w9WgXcQ", Title: "Test", Thumbnail: "https://i.ytimg.com/vi/dQw4w9WgXcQ/hqdefault.jpg"})
	if err = store.Save(value); err != nil {
		t.Fatal(err)
	}
	loaded, err := NewStore(path).Load()
	if err != nil || len(loaded.Liked) != 1 {
		t.Fatal("persistence failed", err)
	}
	value.Liked[0].ID = "../invalid"
	if store.Save(value) == nil {
		t.Fatal("accepted invalid id")
	}
	loaded, err = store.Load()
	if err != nil || loaded.Liked[0].ID != "dQw4w9WgXcQ" {
		t.Fatal("bad write replaced saved data")
	}
	stat, _ := os.Stat(path)
	if stat.Mode().Perm() != 0600 {
		t.Fatal("library is not private")
	}
}

func TestCorruptionAndUntrustedCover(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library.json")
	_ = os.WriteFile(path, []byte("broken"), 0600)
	if _, err := NewStore(path).Load(); err == nil {
		t.Fatal("ignored corruption")
	}
	if err := NewStore(path).Save(Empty()); err == nil {
		t.Fatal("overwrote a corrupt library")
	}
	data, _ := os.ReadFile(path)
	if string(data) != "broken" {
		t.Fatal("failed recovery destroyed original data")
	}
	v := Empty()
	v.Liked = []catalog.Track{{ID: "dQw4w9WgXcQ", Title: "Test", Thumbnail: "https://ytimg.com.evil.test/a.jpg"}}
	if NewStore(path).Save(v) == nil {
		t.Fatal("accepted untrusted cover")
	}
}

func TestIncompleteJSONNeverBecomesAnEmptyLibrary(t *testing.T) {
	for _, raw := range []string{`null`, `{}`, `{"liked":[]}`, `{"liked":[],"playlists":null}`} {
		t.Run(raw, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "library.json")
			if err := os.WriteFile(path, []byte(raw), 0600); err != nil {
				t.Fatal(err)
			}
			store := NewStore(path)
			if _, err := store.Load(); err == nil {
				t.Fatal("incomplete data was accepted")
			}
			if err := store.Save(Empty()); err == nil {
				t.Fatal("incomplete original was overwritten")
			}
			data, _ := os.ReadFile(path)
			if string(data) != raw {
				t.Fatal("original changed")
			}
		})
	}
}
