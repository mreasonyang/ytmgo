package librarysnapshot

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ytmgo/internal/catalog"
)

const scopeA = "0123456789abcdef01234567"
const scopeB = "fedcba9876543210fedcba98"

func TestSnapshotRestartAndSelectedAccount(t *testing.T) {
	path := filepath.Join(t.TempDir(), "account", "library.json")
	page := catalog.Page{Continuation: "old-process-handle", Items: []catalog.Item{{Title: "Saved playlist", BrowseID: "VLsaved", Kind: "playlist", Artwork: "https://i.ytimg.com/vi/abc123DEF_-/hqdefault.jpg", LikeKnown: true, Liked: true}}}
	if err := Save(path, scopeA, page); err != nil {
		t.Fatal(err)
	}
	loaded, ok := Load(path, scopeA)
	if !ok || len(loaded.Items) != 1 || loaded.Items[0].Title != "Saved playlist" || loaded.Items[0].Artwork != page.Items[0].Artwork || loaded.Continuation != "" || loaded.Items[0].LikeKnown || loaded.Items[0].Liked {
		t.Fatal("restart did not recover only display metadata")
	}
	for _, scope := range []string{"", scopeB, "invalid"} {
		if _, ok := Load(path, scope); ok {
			t.Fatal("another or unavailable account recovered the snapshot")
		}
	}
	if err := Save(path, scopeB, catalog.Page{}); err != nil {
		t.Fatal(err)
	}
	if _, ok := Load(path, scopeA); ok {
		t.Fatal("new selection left the old account snapshot available")
	}
	if page, ok := Load(path, scopeB); !ok || len(page.Items) != 0 {
		t.Fatal("a successfully refreshed empty library did not replace old entries")
	}
	Remove(path)
	if _, err := os.Lstat(path); !os.IsNotExist(err) {
		t.Fatal("sign-out cleanup left a snapshot")
	}
}

func TestSnapshotRejectsCorruptionAndUnexpectedData(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library.json")
	for _, data := range []string{
		"broken", `{"version":2,"scope":"` + scopeA + `","items":[]}`,
		`{"version":1,"scope":"` + scopeA + `","items":[{"title":"Bad","browseId":"../private"}]}`,
		`{"version":1,"scope":"` + scopeA + `","items":[],"cookie":"synthetic"}`,
		`{"version":1,"scope":"` + scopeA + `","items":[]} {}`,
		strings.Repeat(" ", maxBytes+1),
	} {
		if err := os.WriteFile(path, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
		if _, ok := Load(path, scopeA); ok {
			t.Fatal("invalid snapshot replaced the normal network loading path")
		}
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(t.TempDir(), "unrelated")
	if err := os.WriteFile(target, []byte("unchanged"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, path); err == nil {
		if _, ok := Load(path, scopeA); ok || Save(path, scopeA, catalog.Page{}) == nil {
			t.Fatal("snapshot followed a link")
		}
		data, _ := os.ReadFile(target)
		if string(data) != "unchanged" {
			t.Fatal("snapshot changed an unrelated target")
		}
	}
}

func TestSnapshotMetadataIsBoundedAndIndependent(t *testing.T) {
	page := catalog.Page{Items: make([]catalog.Item, maxItems+10)}
	for i := range page.Items {
		page.Items[i] = catalog.Item{Title: "Saved", BrowseID: "VLsaved", Kind: "playlist"}
	}
	saved := Metadata(page)
	if len(saved.Items) != maxItems {
		t.Fatal("library snapshot is unbounded")
	}
	page.Items[0].Title = "mutated"
	if saved.Items[0].Title != "Saved" {
		t.Fatal("worker metadata aliases the UI model")
	}
}
