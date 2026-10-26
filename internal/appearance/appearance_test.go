package appearance

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPreferencesSurviveReloadWithoutTouchingLibrary(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "appearance")
	library := filepath.Join(dir, "library.json")
	content := []byte(`{"liked":[],"playlists":[{"name":"Keep me"}]}`)
	if err := os.WriteFile(library, content, 0600); err != nil {
		t.Fatal(err)
	}
	if got := Load(path); got != "system" {
		t.Fatalf("new user: %s", got)
	}
	for _, value := range []string{"light", "dark", "system"} {
		if err := Save(path, value); err != nil {
			t.Fatal(err)
		}
		if got := Load(path); got != value {
			t.Fatalf("reloaded %s, want %s", got, value)
		}
	}
	if err := Save(path, "invalid"); err == nil {
		t.Fatal("invalid value accepted")
	}
	if got := Load(path); got != "system" {
		t.Fatalf("invalid write replaced preference: %s", got)
	}
	got, err := os.ReadFile(library)
	if err != nil || string(got) != string(content) {
		t.Fatal("music library changed")
	}
}

func TestUnreadableOrInvalidPreferenceFallsBackAndWriteFailureIsReported(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "appearance")
	if err := os.WriteFile(path, []byte("unexpected"), 0600); err != nil {
		t.Fatal(err)
	}
	if got := Load(path); got != "system" {
		t.Fatalf("invalid preference: %s", got)
	}
	if got := Load(dir); got != "system" {
		t.Fatalf("unreadable preference: %s", got)
	}
	if err := Save(filepath.Join(path, "child"), "light"); err == nil {
		t.Fatal("write failure hidden")
	}
}
