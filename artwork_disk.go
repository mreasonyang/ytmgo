package main

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"ytmgo/internal/httpfreshness"
)

const (
	artworkDiskBudget = 64 << 20
	artworkDiskFiles  = 512
	artworkDiskMaxAge = 24 * time.Hour
)

// The private cache directory holds compressed, validated public covers. The
// name hashes the source URL and the modification time is the expiry deadline.
type artworkDisk struct {
	dir string
	mu  sync.Mutex // serialize admission, pruning and atomic replacement
}

func artworkDiskKey(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || !artworkURL(u) || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" {
		return ""
	}
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

// Admit only explicit public freshness from the original, anonymous GET.
// These requests never send Origin and use the transport's default encoding.
func artworkFreshUntil(headers http.Header, requested time.Time) time.Time {
	return httpfreshness.FreshUntil(headers, requested, artworkDiskMaxAge)
}

func (d *artworkDisk) read(key string) []byte {
	if dir, err := os.Lstat(d.dir); err != nil || !dir.IsDir() {
		return nil
	}
	path := filepath.Join(d.dir, key)
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || !info.ModTime().After(time.Now()) || info.Size() <= 0 || info.Size() > artworkDownloadLimit {
		return nil
	}
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	if opened, err := f.Stat(); err != nil || !os.SameFile(info, opened) {
		return nil
	}
	data, err := io.ReadAll(io.LimitReader(f, artworkDownloadLimit+1))
	if err != nil || int64(len(data)) != info.Size() {
		return nil
	}
	return data
}

func (d *artworkDisk) write(key string, data []byte, expiry time.Time) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if !expiry.After(time.Now()) || len(data) == 0 || len(data) > artworkDownloadLimit {
		return
	}
	if os.MkdirAll(d.dir, 0700) != nil || !d.makeRoom(int64(len(data))) {
		return
	}
	f, err := os.CreateTemp(d.dir, ".write-")
	if err != nil {
		return
	}
	defer os.Remove(f.Name())
	_, writeErr := f.Write(data)
	closeErr := f.Close()
	if writeErr == nil && closeErr == nil && os.Chtimes(f.Name(), time.Now(), expiry) == nil {
		_ = os.Rename(f.Name(), filepath.Join(d.dir, key))
	}
}

// Discard expired files first, then those nearest expiry. No persistent index
// needs to be recovered after a crash. Only files from this cache are pruned.
func (d *artworkDisk) makeRoom(need int64) bool {
	if dir, err := os.Lstat(d.dir); err != nil || !dir.IsDir() {
		return false
	}
	entries, err := os.ReadDir(d.dir)
	if err != nil {
		return false
	}
	now := time.Now()
	var live []os.FileInfo
	var total int64
	for _, e := range entries {
		name := e.Name()
		if !e.Type().IsRegular() {
			continue
		}
		if strings.HasPrefix(name, ".write-") {
			// A previous process quit between creating and renaming its file.
			if os.Remove(filepath.Join(d.dir, name)) != nil {
				return false
			}
			continue
		}
		if len(name) != sha256.Size*2 || strings.Trim(name, "0123456789abcdef") != "" {
			continue
		}
		info, err := e.Info()
		if err != nil {
			return false
		}
		if !info.ModTime().After(now) {
			if os.Remove(filepath.Join(d.dir, name)) != nil {
				return false
			}
			continue
		}
		total += info.Size()
		live = append(live, info)
	}
	if total+need <= artworkDiskBudget && len(live) < artworkDiskFiles {
		return true
	}
	sort.Slice(live, func(i, j int) bool { return live[i].ModTime().Before(live[j].ModTime()) })
	for i, f := range live {
		if os.Remove(filepath.Join(d.dir, f.Name())) != nil {
			return false
		}
		total -= f.Size()
		if total+need <= artworkDiskBudget && len(live)-i-1 < artworkDiskFiles {
			return true
		}
	}
	return false
}
