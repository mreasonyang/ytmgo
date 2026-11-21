package downloads

import (
	"context"
	"errors"
	"io"
	"maps"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"

	"ytmgo/internal/catalog"
)

const coverBytesLimit = 4 << 20

var coverSize = regexp.MustCompile(`=(?:w\d+-h\d+|s\d+)([^/?]*)$`)

func isSystemCoverURL(u *url.URL) bool {
	return !u.ForceQuery && (strings.EqualFold(u.Host, "gstatic.com") || strings.EqualFold(u.Host, "www.gstatic.com")) &&
		catalog.IsSystemArtworkURL(u)
}

// CoverURLAtSize uses the final host for supported system covers and rewrites
// the size of recognized CDN images. Other resource options stay intact.
func CoverURLAtSize(raw string, u *url.URL, edge int) string {
	if u.Host != "www.gstatic.com" && isSystemCoverURL(u) {
		// The bare host redirects here. HTTP cache entries use this final URL.
		result := *u
		result.Host = "www.gstatic.com"
		return result.String()
	}
	host := strings.ToLower(u.Hostname())
	if host != "googleusercontent.com" && !strings.HasSuffix(host, ".googleusercontent.com") &&
		host != "ggpht.com" && !strings.HasSuffix(host, ".ggpht.com") {
		return raw
	}
	path := u.EscapedPath()
	if !coverSize.MatchString(path) {
		return raw
	}
	size := strconv.Itoa(edge)
	resized := coverSize.ReplaceAllString(path, "=w"+size+"-h"+size+"$1")
	result := *u
	result.Path, _ = url.PathUnescape(resized)
	result.RawPath = resized
	return result.String()
}

func coverURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || u.Scheme != "https" || u.User != nil || u.Port() != "" {
		return raw
	}
	indexURL := *u
	indexURL.Host = strings.ToLower(u.Host)
	indexURL.Fragment, indexURL.RawFragment = "", ""
	// Do not let index cleanup publish an unsupported URL under the shared key.
	if (u.Fragment != "" || u.Host != indexURL.Host) && isSystemCoverURL(&indexURL) && !isSystemCoverURL(u) {
		return raw
	}
	return CoverURLAtSize(indexURL.String(), &indexURL, 768)
}

// ErrInvalidCover means the selected version is missing, changed or unusable.
// Other I/O errors can be retried without withdrawing the asset.
var ErrInvalidCover = errors.New("invalid download cover")

type CoverAsset struct {
	library *Manager
	job     *job
	path    string
	url     string
	file    atomic.Pointer[os.FileInfo]
}

// URL is the canonical artwork URL shared by the asset's renditions.
func (a *CoverAsset) URL() string { return a.url }

func (a *CoverAsset) current() bool {
	return a.library.Covers().source(a.url).Asset == a
}

// Reject withdraws this specific version. A repaired file can be discovered
// when the library is reopened or the track downloaded again.
func (a *CoverAsset) Reject(ctx context.Context) {
	m := a.library
	m.mu.Lock()
	defer m.mu.Unlock()
	if ctx.Err() == nil && m.ctx.Err() == nil && a.current() {
		m.removeCover(a.job)
	}
}

func (a *CoverAsset) Read(ctx context.Context) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !a.current() {
		return nil, context.Canceled
	}
	root, err := os.OpenRoot(a.library.root)
	if err != nil {
		return nil, coverReadError(err)
	}
	defer root.Close()
	dir, err := root.Lstat(filepath.Dir(a.path))
	if err != nil {
		return nil, coverReadError(err)
	}
	if !dir.IsDir() {
		return nil, ErrInvalidCover
	}
	st, err := root.Lstat(a.path)
	if err != nil {
		return nil, coverReadError(err)
	}
	bound := a.file.Load()
	if !st.Mode().IsRegular() || st.Size() <= 0 || st.Size() > coverBytesLimit || (bound != nil && !sameCoverFile(*bound, st)) {
		return nil, ErrInvalidCover
	}
	f, err := root.Open(a.path)
	if err != nil {
		return nil, coverReadError(err)
	}
	defer f.Close()
	opened, err := f.Stat()
	if err != nil {
		return nil, coverReadError(err)
	}
	if !sameCoverFile(st, opened) {
		return nil, ErrInvalidCover
	}
	data, err := io.ReadAll(io.LimitReader(f, coverBytesLimit+1))
	if err != nil {
		return nil, coverReadError(err)
	}
	still, err := root.Lstat(a.path)
	if err != nil {
		return nil, coverReadError(err)
	}
	if int64(len(data)) != st.Size() || !sameCoverFile(st, still) || !a.bindFile(st) {
		return nil, ErrInvalidCover
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !a.current() {
		return nil, context.Canceled
	}
	return data, nil
}

// Bind the first successful worker read to this publication. A later read
// cannot contribute a rendition of a different file to the same source.
func (a *CoverAsset) bindFile(st os.FileInfo) bool {
	return a.file.CompareAndSwap(nil, &st) || sameCoverFile(*a.file.Load(), st)
}

func sameCoverFile(a, b os.FileInfo) bool {
	return os.SameFile(a, b) && a.Size() == b.Size() && a.ModTime().Equal(b.ModTime())
}

func coverReadError(err error) error {
	if errors.Is(err, os.ErrNotExist) {
		return ErrInvalidCover
	}
	return err
}

// CoverSource identifies both local cover publications and returns to HTTP.
type CoverSource struct {
	Asset      *CoverAsset
	Generation uint64
}

// CoverIndex is an immutable view of selected sources. Remote generations
// left by a removal are retained until switching libraries, so a frame can
// detect even a complete HTTP -> local -> HTTP cycle between two reads.
type CoverIndex struct {
	byURL      map[string]CoverSource
	remote     uint64
	revocation uint64
}

func (c *CoverIndex) Source(raw string) CoverSource {
	if c == nil {
		return CoverSource{}
	}
	if source, ok := c.byURL[raw]; ok {
		return source
	}
	if len(c.byURL) == 0 {
		return CoverSource{Generation: c.remote}
	}
	return c.source(coverURL(raw))
}

func (c *CoverIndex) source(key string) CoverSource {
	if source, ok := c.byURL[key]; ok {
		return source
	}
	return CoverSource{Generation: c.remote}
}

func (c *CoverIndex) Revocation() uint64 {
	if c == nil {
		return 0
	}
	return c.revocation
}

func (m *Manager) Covers() *CoverIndex {
	if m == nil {
		return nil
	}
	return m.covers.Load()
}

// LockCovers lets a consumer validate and publish its own cache entry in
// one operation. This lock never requires the download manager's state lock.
func (m *Manager) LockCovers() (*CoverIndex, func()) {
	if m == nil {
		return nil, func() {}
	}
	m.coverMu.RLock()
	return m.Covers(), m.coverMu.RUnlock
}

func (m *Manager) CoverChanges() <-chan struct{} { return m.coverChanges }

func (m *Manager) publishCovers(index *CoverIndex) {
	m.coverMu.Lock()
	m.covers.Store(index)
	m.coverMu.Unlock()
	signal(m.coverChanges)
}

func (m *Manager) clearCovers() {
	next := m.Covers().Revocation() + 1
	m.coverJobs = nil
	m.publishCovers(&CoverIndex{remote: next, revocation: next})
}

// Only library selection walks all jobs. Incremental updates touch just the
// affected URL's candidates and copy the published map if its winner changes.
func (m *Manager) loadCovers() {
	old := m.Covers()
	next := &CoverIndex{byURL: make(map[string]CoverSource), revocation: old.Revocation()}
	if old != nil {
		next.remote = old.remote
	}
	m.coverJobs = make(map[string][]*job)
	for _, j := range m.jobs {
		if j.State == Complete && j.HasCover && j.coverURL != "" {
			m.coverJobs[j.coverURL] = append(m.coverJobs[j.coverURL], j)
			next.byURL[j.coverURL] = CoverSource{Asset: m.coverAsset(j), Generation: next.revocation + 1}
		}
	}
	if len(next.byURL) > 0 {
		next.revocation++
		m.publishCovers(next)
	} else if old == nil {
		m.publishCovers(next)
	}
}

func (m *Manager) coverAsset(j *job) *CoverAsset {
	return &CoverAsset{library: m, job: j, path: filepath.Join(m.scope, j.Asset, "cover"), url: j.coverURL}
}

func (m *Manager) addCover(j *job) {
	if !j.HasCover || j.coverURL == "" {
		return
	}
	m.coverJobs[j.coverURL] = append(m.coverJobs[j.coverURL], j)
	winner := m.Covers().source(j.coverURL).Asset
	if winner == nil || winner.job.coverOrder < j.coverOrder {
		m.selectCover(j.coverURL, j)
	}
}

func (m *Manager) removeCover(j *job) {
	candidates := m.coverJobs[j.coverURL]
	i := slices.Index(candidates, j)
	if i < 0 {
		return
	}
	candidates = slices.Delete(candidates, i, i+1)
	if len(candidates) == 0 {
		delete(m.coverJobs, j.coverURL)
	} else {
		m.coverJobs[j.coverURL] = candidates
	}
	if winner := m.Covers().source(j.coverURL).Asset; winner != nil && winner.job == j {
		var replacement *job
		for _, candidate := range candidates {
			if replacement == nil || candidate.coverOrder > replacement.coverOrder {
				replacement = candidate
			}
		}
		m.selectCover(j.coverURL, replacement)
	}
}

func (m *Manager) selectCover(key string, j *job) {
	old := m.Covers()
	next := &CoverIndex{byURL: maps.Clone(old.byURL), remote: old.remote, revocation: old.revocation + 1}
	if next.byURL == nil {
		next.byURL = make(map[string]CoverSource)
	}
	source := CoverSource{Generation: next.revocation}
	if j != nil {
		source.Asset = m.coverAsset(j)
	}
	next.byURL[key] = source
	m.publishCovers(next)
}
