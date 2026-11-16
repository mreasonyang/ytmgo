package audioquality

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSavedPreferenceAndFallback(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audio-quality")
	if got := Load(path); got != Best {
		t.Fatalf("missing preference = %q, want %q", got, Best)
	}
	for _, q := range []Quality{Best, Standard, Saver} {
		if err := Save(path, q); err != nil {
			t.Fatal(err)
		}
		if got := Load(path); got != q {
			t.Fatalf("reloaded %q, want %q", got, q)
		}
	}
	if err := Save(path, "invalid"); err == nil || Load(path) != Saver {
		t.Fatal("invalid choice changed the saved preference")
	}
	if err := os.WriteFile(path, []byte("unknown"), 0600); err != nil {
		t.Fatal(err)
	}
	if got := Load(path); got != Best {
		t.Fatalf("invalid preference = %q, want %q", got, Best)
	}
}
