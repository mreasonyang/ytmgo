package downloads

import (
	"context"
	"crypto/cipher"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sync"

	"ytmgo/internal/accountsession"
	"ytmgo/internal/audioquality"
	"ytmgo/internal/catalog"
	"ytmgo/internal/preference"
)

type State string

const (
	Queued      State = "queued"
	Downloading State = "downloading"
	Paused      State = "paused"
	Failed      State = "failed"
	Complete    State = "complete"
	Guest             = "guest"
)

type Entry struct {
	Item         catalog.Item
	Quality      audioquality.Quality
	State        State
	Bytes, Total int64
	Failure      string
}

func (e Entry) StoredBytes() int64 { return diskSize(e.Bytes) }

type record struct {
	Entry
	Asset    string
	Format   Format
	Wrapped  []byte
	HasCover bool
}

type job struct {
	record
	life        context.Context
	revoke      context.CancelFunc
	removeAfter string
}

type Remote struct {
	Format Format
	Open   func(context.Context, int64) (io.ReadCloser, error)
	Close  func()
}

type Resolve func(context.Context, string, audioquality.Quality) (Remote, error)
type Extras func(context.Context, catalog.Item) (catalog.LyricsResult, []byte)

type Manager struct {
	mu                  sync.Mutex
	ctx                 context.Context
	cancel              context.CancelFunc
	done, wake, changes chan struct{}
	root, scope         string
	online              bool
	jobs                []*job
	byID                map[string]*job
	byCover             map[string]string
	active              *job
	activeDir           string
	abort               context.CancelFunc
	resolve             Resolve
	extras              Extras
	secrets             accountsession.Secrets
	keyMu               sync.Mutex
	key                 cipher.AEAD
}

var scopeName = regexp.MustCompile("^(guest|[a-f0-9]{24})$")
var assetName = regexp.MustCompile("^[a-f0-9]{32}$")

func New(parent context.Context, root string, secrets accountsession.Secrets, resolve Resolve, extras Extras) (*Manager, error) {
	if os.MkdirAll(root, 0700) != nil || !realDirectory(root) {
		return nil, ErrStorage
	}
	ctx, cancel := context.WithCancel(parent)
	m := &Manager{ctx: ctx, cancel: cancel, root: root, secrets: secrets, resolve: resolve, extras: extras,
		done: make(chan struct{}), wake: make(chan struct{}, 1), changes: make(chan struct{}, 1)}
	scope := Guest
	if data, err := os.ReadFile(filepath.Join(root, "selection")); err == nil {
		scope = string(data)
		if !scopeName.MatchString(scope) {
			cancel()
			return nil, ErrStorage
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		cancel()
		return nil, ErrStorage
	}
	if err := m.load(scope); err != nil {
		cancel()
		return nil, err
	}
	go m.run()
	return m, nil
}

func (m *Manager) Changes() <-chan struct{} { return m.changes }

func signal(channel chan struct{}) {
	select {
	case channel <- struct{}{}:
	default:
	}
}

func realDirectory(path string) bool {
	st, err := os.Lstat(path)
	return err == nil && st.IsDir()
}

func libraryDirectory(path string) error {
	if err := os.Mkdir(path, 0700); err != nil && !errors.Is(err, os.ErrExist) {
		return ErrStorage
	}
	if !realDirectory(path) {
		return ErrStorage
	}
	return nil
}

func (m *Manager) load(scope string) error {
	var records []record
	dir := filepath.Join(m.root, scope)
	if err := libraryDirectory(dir); err != nil {
		return err
	}
	b, err := os.ReadFile(filepath.Join(dir, "index.json"))
	if !errors.Is(err, os.ErrNotExist) && (err != nil || json.Unmarshal(b, &records) != nil) {
		return ErrStorage
	}
	jobs := make([]*job, 0, len(records))
	ids := make(map[string]*job, len(records))
	assets := make(map[string]bool, len(records))
	for _, rec := range records {
		if rec.Item.PlaybackMetadata().ID == "" || !assetName.MatchString(rec.Asset) || assets[rec.Asset] || ids[rec.Item.ID] != nil {
			return ErrStorage
		}
		assets[rec.Asset] = true
		switch rec.State {
		case Downloading, Queued:
			rec.State = Queued
		case Complete, Paused, Failed:
		default:
			return ErrStorage
		}
		if rec.Format.Length < 0 || rec.Format.Length > 64<<30 {
			return ErrStorage
		}
		rec.Total = rec.Format.Length
		var size int64
		if asset, statErr := os.Lstat(filepath.Join(dir, rec.Asset)); statErr == nil && !asset.IsDir() {
			return ErrStorage
		}
		st, err := os.Lstat(filepath.Join(dir, rec.Asset, "media"))
		if err == nil {
			if !st.Mode().IsRegular() {
				return ErrStorage
			}
			size = st.Size()
		}
		rec.Bytes = committed(size, rec.Total)
		if rec.State == Complete && (!rec.Format.valid() || size != diskSize(rec.Total) || len(rec.Wrapped) != 32+recordOverhead) {
			rec.State, rec.Failure = Failed, "media"
		}
		j := &job{record: rec}
		jobs = append(jobs, j)
		ids[j.Item.ID] = j
	}
	// Removing a track commits its index first. Finish an interrupted cleanup
	// only within asset directories allocated by this library.
	if entries, readErr := os.ReadDir(dir); readErr == nil {
		for _, e := range entries {
			if e.IsDir() && assetName.MatchString(e.Name()) && !assets[e.Name()] {
				_ = os.RemoveAll(filepath.Join(dir, e.Name()))
			}
		}
	}
	m.scope, m.jobs, m.byID = scope, jobs, ids
	for _, j := range jobs {
		j.life, j.revoke = context.WithCancel(m.ctx)
	}
	m.indexCovers()
	return nil
}

func (m *Manager) Scope() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.scope
}

func (m *Manager) indexCovers() {
	m.byCover = make(map[string]string)
	for _, j := range m.jobs {
		if j.HasCover && j.State == Complete {
			m.byCover[j.Item.Artwork] = filepath.Join(m.assetDir(j), "cover")
		}
	}
}

// Switch publishes the new library before deleting explicitly abandoned account
// data. The guest library can be selected again after signing out.
func (m *Manager) Switch(scope string, forget bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !scopeName.MatchString(scope) {
		return ErrStorage
	}
	if scope == m.scope {
		return nil
	}
	oldScope, oldJobs := m.scope, m.jobs
	m.online = false
	if m.abort != nil {
		m.abort()
	}
	for _, j := range oldJobs {
		j.revoke()
	}
	m.scope, m.jobs, m.byID, m.byCover = "", nil, nil, nil
	signal(m.changes)
	if err := preference.WriteFile(filepath.Join(m.root, "selection"), []byte(scope)); err != nil {
		return ErrStorage
	}
	if err := m.load(scope); err != nil {
		return err
	}
	if forget && oldScope != "" && oldScope != Guest {
		oldDir := filepath.Join(m.root, oldScope)
		if m.active != nil && filepath.Dir(m.activeDir) == oldDir {
			m.active.removeAfter = oldDir
		} else if err := os.RemoveAll(oldDir); err != nil {
			return ErrStorage
		}
	}
	return nil
}

func (m *Manager) CanTransfer() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.online
}

func (m *Manager) Online(on bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.online = on
	if !on && m.abort != nil {
		m.abort()
		if j := m.active; j != nil && m.byID[j.Item.ID] == j && j.State == Downloading {
			j.State = Queued
		}
	}
	signal(m.wake)
	signal(m.changes)
}

func (m *Manager) save() error {
	records := make([]record, len(m.jobs))
	for i, j := range m.jobs {
		records[i] = j.record
	}
	b, err := json.Marshal(records)
	if err != nil || preference.WriteFile(filepath.Join(m.root, m.scope, "index.json"), b) != nil {
		return ErrStorage
	}
	return nil
}

func (m *Manager) Enqueue(scope string, items []catalog.Item, q audioquality.Quality) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if scope == "" || scope != m.scope || m.ctx.Err() != nil {
		return context.Canceled
	}
	n := len(m.jobs)
	for _, it := range items {
		it = it.PlaybackMetadata()
		if it.ID == "" || m.byID[it.ID] != nil {
			continue
		}
		var id [16]byte
		rand.Read(id[:])
		j := &job{record: record{Entry: Entry{Item: it, Quality: q.Normalize(), State: Queued}, Asset: hex.EncodeToString(id[:])}}
		j.life, j.revoke = context.WithCancel(m.ctx)
		m.jobs = append(m.jobs, j)
		m.byID[it.ID] = j
	}
	if n == len(m.jobs) {
		return nil
	}
	if err := m.save(); err != nil {
		for _, j := range m.jobs[n:] {
			j.revoke()
			delete(m.byID, j.Item.ID)
		}
		m.jobs = m.jobs[:n]
		return err
	}
	signal(m.wake)
	signal(m.changes)
	return nil
}

func (m *Manager) SetPaused(id string, pause bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	j := m.byID[id]
	if j == nil || j.State == Complete {
		return nil
	}
	next := Queued
	if pause {
		next = Paused
	}
	old, failure := j.State, j.Failure
	j.State, j.Failure = next, ""
	if err := m.save(); err != nil {
		j.State, j.Failure = old, failure
		return err
	}
	if m.active == j && m.abort != nil {
		m.abort()
	}
	signal(m.wake)
	signal(m.changes)
	return nil
}

func (m *Manager) Remove(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	j := m.byID[id]
	if j == nil {
		return nil
	}
	for i, entry := range m.jobs {
		if entry != j {
			continue
		}
		m.jobs = append(m.jobs[:i], m.jobs[i+1:]...)
		if err := m.save(); err != nil {
			m.jobs = append(m.jobs, nil)
			copy(m.jobs[i+1:], m.jobs[i:])
			m.jobs[i] = j
			return err
		}
		break
	}
	delete(m.byID, id)
	m.indexCovers()
	j.revoke()
	if m.active == j && m.abort != nil {
		j.removeAfter = m.assetDir(j)
		m.abort()
		signal(m.changes)
		return nil
	}
	signal(m.changes)
	if err := os.RemoveAll(m.assetDir(j)); err != nil {
		return ErrStorage
	}
	return nil
}

func (m *Manager) assetDir(j *job) string { return filepath.Join(m.root, m.scope, j.Asset) }

func (m *Manager) Snapshot() []Entry {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Entry, len(m.jobs))
	for i, j := range m.jobs {
		out[i] = j.Entry
	}
	return out
}

func (m *Manager) Has(id string) bool {
	if m == nil {
		return false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	j := m.byID[id]
	return j != nil && j.State == Complete
}

func (m *Manager) OpenTrack(id string) (*Source, Format, error) {
	m.mu.Lock()
	j := m.byID[id]
	if j == nil || j.State != Complete {
		m.mu.Unlock()
		return nil, Format{}, nil
	}
	life := j.life
	s := &Source{damage: func() { m.damaged(j, life) }, life: life, dir: m.assetDir(j), format: j.Format, bind: binding(m.scope, j.Asset, id, j.Format)}
	wrapped := j.Wrapped
	m.mu.Unlock()
	master, err := m.master(false)
	if err != nil {
		return nil, Format{}, err
	}
	s.aead, err = unwrap(master, wrapped, s.bind)
	if err != nil {
		s.damaged()
		return nil, Format{}, err
	}
	if err = s.life.Err(); err != nil {
		return nil, Format{}, err
	}
	return s, s.format, nil
}

func (m *Manager) damaged(j *job, life context.Context) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.byID[j.Item.ID] != j || j.life != life || j.State != Complete {
		return
	}
	j.revoke()
	j.life, j.revoke = context.WithCancel(m.ctx)
	j.State, j.Failure = Failed, "media"
	m.indexCovers()
	_ = m.save()
	signal(m.changes)
}

func (m *Manager) Lyrics(id string) (catalog.LyricsResult, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	j := m.byID[id]
	if j == nil || j.State != Complete {
		return catalog.LyricsResult{}, false
	}
	var result catalog.LyricsResult
	if b, err := os.ReadFile(filepath.Join(m.assetDir(j), "lyrics.json")); err == nil {
		_ = json.Unmarshal(b, &result)
	}
	return result, true
}

func (m *Manager) Cover(raw string) []byte {
	if raw == "" {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if path := m.byCover[raw]; path != "" {
		b, _ := os.ReadFile(path)
		return b
	}
	return nil
}

func (m *Manager) Close() { m.cancel(); <-m.done }
