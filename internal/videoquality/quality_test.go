package videoquality

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultPersistenceAndFailedSave(t *testing.T) {
	path := filepath.Join(t.TempDir(), "video-quality")
	if Load(path) != 720 {
		t.Fatal("new and existing users without a preference must default to 720p")
	}
	for _, quality := range []int{1080, 360, 720} {
		if err := Save(path, quality); err != nil {
			t.Fatal(err)
		}
		if got := Load(path); got != quality {
			t.Fatalf("reloaded %dp, want %dp", got, quality)
		}
	}
	if Save(path, -1) == nil || Load(path) != 720 {
		t.Fatal("invalid write replaced the saved preference")
	}
	if Save(filepath.Join(path, "child"), 1080) == nil {
		t.Fatal("storage failure was hidden")
	}
	if err := os.WriteFile(path, []byte("corrupt"), 0600); err != nil {
		t.Fatal(err)
	}
	if Load(path) != 720 {
		t.Fatal("corrupt preference did not fall back to 720p")
	}
}
