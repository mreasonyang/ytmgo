// Package librarysnapshot keeps the last successfully loaded library for one
// saved account selection. Entries are display metadata, never continuations,
// authentication state, or media locations.
package librarysnapshot

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"ytmgo/internal/catalog"
	"ytmgo/internal/preference"
)

const maxBytes = 2 << 20
const maxItems = 500

type snapshot struct {
	Version int            `json:"version"`
	Scope   string         `json:"scope"`
	Items   []catalog.Item `json:"items"`
}

func validScope(scope string) bool {
	return len(scope) == 24 && strings.Trim(scope, "0123456789abcdef") == ""
}

func privateMode(info os.FileInfo, mode os.FileMode) bool {
	return runtime.GOOS == "windows" || info.Mode().Perm() == mode
}

func validDirectory(path string, create bool) bool {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return false
	}
	dir := filepath.Dir(path)
	if create && os.MkdirAll(dir, 0700) != nil {
		return false
	}
	info, err := os.Lstat(dir)
	return err == nil && info.IsDir() && privateMode(info, 0700)
}

// Metadata makes an independent, bounded display-only copy. Its continuation
// handle belongs to the old catalog process and must never be restored.
func Metadata(page catalog.Page) catalog.Page {
	items := make([]catalog.Item, 0, min(len(page.Items), maxItems))
	for _, item := range page.Items[:min(len(page.Items), maxItems)] {
		if item = item.LibraryMetadata(); item.ID != "" || item.BrowseID != "" {
			items = append(items, item)
		}
	}
	return catalog.Page{Items: items}
}

func Load(path, scope string) (catalog.Page, bool) {
	if !validScope(scope) || !validDirectory(path, false) {
		return catalog.Page{}, false
	}
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || !privateMode(info, 0600) || info.Size() <= 0 || info.Size() > maxBytes {
		return catalog.Page{}, false
	}
	file, err := os.Open(path)
	if err != nil {
		return catalog.Page{}, false
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !os.SameFile(info, opened) {
		return catalog.Page{}, false
	}
	var saved snapshot
	decoder := json.NewDecoder(io.LimitReader(file, maxBytes+1))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&saved) != nil || saved.Version != 1 || saved.Scope != scope || len(saved.Items) > maxItems {
		return catalog.Page{}, false
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		return catalog.Page{}, false
	}
	page := Metadata(catalog.Page{Items: saved.Items})
	if len(page.Items) != len(saved.Items) {
		return catalog.Page{}, false
	}
	return page, true
}

func Save(path, scope string, page catalog.Page) error {
	if !validScope(scope) || !validDirectory(path, true) {
		return errors.New("library_snapshot_unavailable")
	}
	if info, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) && (err != nil || !info.Mode().IsRegular() || !privateMode(info, 0600)) {
		return errors.New("library_snapshot_unavailable")
	}
	data, err := json.Marshal(snapshot{Version: 1, Scope: scope, Items: Metadata(page).Items})
	if err != nil || len(data) > maxBytes {
		return errors.New("library_snapshot_limit")
	}
	return preference.WriteFile(path, data)
}

func Remove(path string) {
	if validDirectory(path, false) {
		_ = os.Remove(path)
	}
}
