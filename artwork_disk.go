package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/bartventer/httpcache/store/driver"
	"ytmgo/internal/catalog"
)

const (
	artworkDiskBudget     = 512 << 20
	artworkDiskReserve    = 256 << 20
	artworkDiskEntryLimit = artworkDownloadLimit + 64<<10
	artworkRefLimit       = 16 << 10
)

var errArtworkStorage = errors.New("artwork cache admission declined")

// httpcache calls Set(responseID, body) then Set(urlKey, refs). Publish a
// representation's own reference and response together. A crash can leave
// only an uncommitted temp file. Independent Vary representations never
// overwrite each other's index.
type artworkDisk struct {
	dir          string
	freeSpace    func(string) (uint64, error)
	mu           sync.Mutex
	pending      map[string][]byte
	pendingBytes int
}

func artworkDiskKey(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || !artworkURL(u) || u.ForceQuery || u.Fragment != "" {
		return ""
	}
	if u.RawQuery != "" && !catalog.IsPublicArtworkQuery(u) {
		return ""
	}
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func artworkRecord(key string) (group, variant string) {
	base, v, hasVariant := strings.Cut(key, "#")
	if hasVariant {
		n, err := strconv.ParseUint(v, 10, 64)
		if err != nil || strconv.FormatUint(n, 10) != v {
			return "", ""
		}
	}
	return artworkDiskKey(base), v
}

func artworkRecordName(name string) (string, bool) {
	group, variant, ok := strings.Cut(name, ".")
	if !ok || len(group) != sha256.Size*2 || strings.Trim(group, "0123456789abcdef") != "" {
		return "", false
	}
	n, err := strconv.ParseUint(variant, 10, 64)
	return group, err == nil && strconv.FormatUint(n, 10) == variant
}

// Serialize filesystem operations across loaders and app instances. Nothing
// waits for HTTP or image decoding while holding this OS-released lock.
func (d *artworkDisk) lock(create bool) (*os.File, error) {
	if create {
		if err := os.MkdirAll(d.dir, 0700); err != nil {
			return nil, err
		}
	}
	info, err := os.Lstat(d.dir)
	if err != nil || !info.IsDir() {
		return nil, driver.ErrNotExist
	}
	path := filepath.Join(d.dir, ".lock")
	flags := os.O_RDWR
	if create {
		flags |= os.O_CREATE
	}
	if info, err := os.Lstat(path); err == nil && !info.Mode().IsRegular() {
		return nil, errArtworkStorage
	}
	f, err := os.OpenFile(path, flags, 0600)
	if err != nil {
		return nil, err
	}
	info, err = os.Lstat(path)
	opened, statErr := f.Stat()
	if err != nil || statErr != nil || !info.Mode().IsRegular() || !os.SameFile(info, opened) {
		f.Close()
		return nil, errArtworkStorage
	}
	if err := lockArtworkFile(f); err != nil {
		f.Close()
		return nil, err
	}
	return f, nil
}

// Each record starts with the length of its JSON index row, then the row,
// then the library's serialized HTTP response. Index reads skip image bytes.
func readArtworkRecord(path string, body bool) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() {
		return nil, driver.ErrNotExist
	}
	if info.Size() < 6 || info.Size() > 4+artworkRefLimit+artworkDiskEntryLimit {
		return nil, errArtworkStorage
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	opened, err := f.Stat()
	if err != nil || !os.SameFile(info, opened) {
		return nil, errArtworkStorage
	}
	var header [4]byte
	if _, err := io.ReadFull(f, header[:]); err != nil {
		return nil, errArtworkStorage
	}
	n := int64(binary.BigEndian.Uint32(header[:]))
	remaining := info.Size() - 4 - n
	if n <= 0 || n > artworkRefLimit || remaining <= 0 || remaining > artworkDiskEntryLimit {
		return nil, errArtworkStorage
	}
	if body {
		if _, err := f.Seek(n, io.SeekCurrent); err != nil {
			return nil, err
		}
		n = remaining
	}
	data := make([]byte, n)
	_, err = io.ReadFull(f, data)
	return data, err
}

type artworkRef struct {
	ID   string `json:"id"`
	Vary string `json:"vary"`
}

func parseArtworkRef(row json.RawMessage) artworkRef {
	var ref artworkRef
	if json.Unmarshal(row, &ref) != nil {
		return artworkRef{}
	}
	return ref
}

func (d *artworkDisk) Get(key string) ([]byte, error) {
	group, variant := artworkRecord(key)
	if group == "" {
		return nil, driver.ErrNotExist
	}
	lock, err := d.lock(false)
	if err != nil {
		return nil, driver.ErrNotExist
	}
	defer lock.Close()
	if variant != "" {
		path := filepath.Join(d.dir, group+"."+variant)
		data, err := readArtworkRecord(path, true)
		if err == nil {
			now := time.Now()
			_ = os.Chtimes(path, now, now)
		}
		return data, err
	}
	names, err := fs.Glob(os.DirFS(d.dir), group+".*")
	if err != nil {
		return nil, err
	}
	var refs []json.RawMessage
	for _, name := range names {
		path := filepath.Join(d.dir, name)
		if _, ok := artworkRecordName(name); !ok {
			continue
		}
		row, err := readArtworkRecord(path, false)
		if err != nil || parseArtworkRef(row).ID != key+"#"+name[len(group)+1:] {
			if errors.Is(err, errArtworkStorage) || err == nil {
				_ = os.Remove(path)
			}
			continue
		}
		refs = append(refs, json.RawMessage(row))
	}
	if len(refs) == 0 {
		return nil, driver.ErrNotExist
	}
	return json.Marshal(refs)
}

func (d *artworkDisk) Set(key string, data []byte) error {
	group, variant := artworkRecord(key)
	if group == "" || len(data) == 0 || len(data) > artworkDiskEntryLimit {
		return errArtworkStorage
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if variant != "" {
		if d.pending == nil {
			d.pending = make(map[string][]byte)
		}
		// Only the interval between httpcache's two Set calls needs staging.
		cost := d.pendingBytes - cap(d.pending[key]) + cap(data)
		if cost > artworkWorkers*artworkDiskEntryLimit || len(d.pending) >= artworkWorkers*2 && d.pending[key] == nil {
			return errArtworkStorage
		}
		d.pending[key] = data
		d.pendingBytes = cost
		return nil
	}
	var refs []json.RawMessage
	if json.Unmarshal(data, &refs) != nil {
		return errArtworkStorage
	}
	for _, row := range refs {
		id := parseArtworkRef(row).ID
		body := d.pending[id]
		if body == nil {
			continue
		}
		g, v := artworkRecord(id)
		if g != group || v == "" {
			continue
		}
		delete(d.pending, id)
		d.pendingBytes -= cap(body)
		if len(row) > artworkRefLimit {
			return errArtworkStorage
		}
		if err := d.write(group, v, row, body); err != nil {
			return err
		}
	}
	return nil
}

func (d *artworkDisk) write(group, variant string, ref, body []byte) error {
	lock, err := d.lock(true)
	if err != nil {
		return err
	}
	defer lock.Close()
	need := int64(4 + len(ref) + len(body))
	if !d.makeRoom(group, need) {
		return errArtworkStorage
	}
	f, err := os.CreateTemp(d.dir, ".write-")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	var header [4]byte
	binary.BigEndian.PutUint32(header[:], uint32(len(ref)))
	_, err = io.Copy(f, io.MultiReader(bytes.NewReader(header[:]), bytes.NewReader(ref), bytes.NewReader(body)))
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	// Retire a previous Vary scheme before publishing the new one. For
	// example, a formerly unvaried response must not satisfy a second
	// language once this URL starts varying by Accept-Language.
	names, err := fs.Glob(os.DirFS(d.dir), group+".*")
	if err != nil {
		return err
	}
	vary := parseArtworkRef(ref).Vary
	for _, name := range names {
		if _, ok := artworkRecordName(name); !ok || name == group+"."+variant {
			continue
		}
		path := filepath.Join(d.dir, name)
		row, readErr := readArtworkRecord(path, false)
		if readErr == nil && !strings.EqualFold(vary, parseArtworkRef(row).Vary) {
			if err := os.Remove(path); err != nil {
				return err
			}
		}
	}
	return os.Rename(f.Name(), filepath.Join(d.dir, group+"."+variant))
}

type artworkDiskGroup struct {
	id    string
	files []string
	size  int64
	used  time.Time
}

// Charge both old and incoming files during atomic replacement. Each format
// and Vary representation contributes its allocated bytes to the same limit.
func (d *artworkDisk) makeRoom(incoming string, need int64) bool {
	entries, err := os.ReadDir(d.dir)
	if err != nil {
		return false
	}
	unit, err := artworkAllocationUnit(d.dir)
	if err != nil || unit <= 0 {
		return false
	}
	charge := func(n int64) int64 { return (n + unit - 1) / unit * unit }
	groups := make(map[string]*artworkDiskGroup)
	var total int64
	for _, entry := range entries {
		name := entry.Name()
		if !entry.Type().IsRegular() {
			continue
		}
		path := filepath.Join(d.dir, name)
		id, ok := artworkRecordName(name)
		if !ok {
			// v1 stored a bare SHA-256 filename, with no HTTP validators.
			legacy := len(name) == 64 && strings.Trim(name, "0123456789abcdef") == ""
			if legacy || strings.HasPrefix(name, ".write-") {
				if os.Remove(path) != nil {
					return false
				}
				continue
			}
			// Include unknown regular files in the size ceiling; eviction only
			// owns files matching a cache format.
			info, err := entry.Info()
			if err != nil {
				return false
			}
			total += max(charge(info.Size()), artworkFileCharge(info))
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return false
		}
		g := groups[id]
		if g == nil {
			g = &artworkDiskGroup{id: id}
			groups[id] = g
		}
		g.files = append(g.files, path)
		size := max(charge(info.Size()), artworkFileCharge(info))
		g.size += size
		total += size
		if info.ModTime().After(g.used) {
			g.used = info.ModTime()
		}
	}
	freeSpace := d.freeSpace
	if freeSpace == nil {
		freeSpace = artworkFreeSpace
	}
	free, err := freeSpace(d.dir)
	if err != nil {
		return false
	}
	need = max(charge(need), artworkWriteCharge(need))
	required := uint64(need) + artworkDiskReserve
	if total+need <= artworkDiskBudget && free >= required {
		return true
	}

	// Recover headroom when crossing the byte limit. Re-read actual
	// volume space after unlinking: snapshots and open handles can retain blocks.
	byteTarget := int64(artworkDiskBudget)
	if total+need > byteTarget {
		byteTarget = byteTarget * 4 / 5
	}
	oldest := make([]*artworkDiskGroup, 0, len(groups))
	for _, g := range groups {
		if g.id != incoming {
			oldest = append(oldest, g)
		}
	}
	sort.Slice(oldest, func(i, j int) bool { return oldest[i].used.Before(oldest[j].used) })
	for _, g := range oldest {
		for _, path := range g.files {
			if os.Remove(path) != nil {
				return false
			}
		}
		total -= g.size
		if free < required {
			free, err = freeSpace(d.dir)
			if err != nil {
				return false
			}
		}
		if total+need <= byteTarget && free >= required {
			return true
		}
	}
	return false
}

func (d *artworkDisk) Delete(key string) error {
	group, variant := artworkRecord(key)
	if group == "" {
		return driver.ErrNotExist
	}
	lock, err := d.lock(false)
	if err != nil {
		return driver.ErrNotExist
	}
	defer lock.Close()
	if variant != "" {
		return os.Remove(filepath.Join(d.dir, group+"."+variant))
	}
	names, err := fs.Glob(os.DirFS(d.dir), group+".*")
	if err != nil {
		return err
	}
	for _, name := range names {
		path := filepath.Join(d.dir, name)
		if _, ok := artworkRecordName(name); ok {
			if err := os.Remove(path); err != nil {
				return err
			}
		}
	}
	return nil
}

// Conservatively round pending writes to filesystem blocks.
func artworkWriteCharge(n int64) int64 { return (n + 4095) / 4096 * 4096 }
