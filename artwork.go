package main

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"

	"ytmgo/internal/catalog"
	"ytmgo/internal/downloads"
	"ytmgo/internal/netproxy"

	"github.com/bartventer/httpcache/pkg/urlkey"
	"github.com/egoist/mygo/ui"
	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp"
)

const (
	artworkWorkers        = 12
	artworkLargeLimit     = 4
	artworkSecondaryLimit = 4
	artworkDecoders       = 2
	artworkQueueLimit     = 64
	artworkSmallEntries   = 512
	artworkBrowseEntries  = 512
	artworkPlayerEntries  = 32
	artworkSmallBudget    = 16 << 20
	artworkBrowseBudget   = 48 << 20
	artworkPlayerBudget   = 16 << 20
	artworkMemoryBudget   = artworkSmallBudget + artworkBrowseBudget + artworkPlayerBudget
	artworkDownloadLimit  = 4 << 20
	artworkSourcePixels   = 4 << 20
	artworkEdge           = 768
	artworkRetryBase      = 2 * time.Second
	artworkRetryMax       = time.Minute
)

// Rendition charges include the encoded backing and, until first paint, the
// base pixels plus possible mipmaps. Preview pools charge their own pixels.
type artEntry struct {
	bitmap                *ui.Bitmap
	tint                  ui.Color // the cover's colour for the interface, when tinted
	tinted                bool
	staged                bool // a preview whose rendition has yet to paint in a browse view
	returnPreview         bool // eviction preference for a preview from the last view
	bytes                 int
	pixelBytes            int
	firstPaint            uint64
	peeked                bool // Draw may have restored CPU pixels after submit
	used                  uint64
	version               artworkVersion
	withdrawn             bool // a newer job revoked this in-flight HTTP response
	cancel                context.CancelFunc
	failures              int
	retryAt               time.Time
	freshUntil            time.Time
	local                 bool // bitmap came from the selected download, independent of HTTP freshness
	prefetchUncacheable   bool // only try again when requested for display
	prefetchDeferredBytes int  // rendition charge from a prefetch blocked by return previews
}

type artworkPool struct {
	entries              map[artRequest]*artEntry
	bytes, budget, limit int
}
type artRequest struct {
	url        string
	edge       int
	asset      *downloads.CoverAsset
	generation uint64
	avatar     bool
}

type artworkPriority uint8

const (
	artPlayer artworkPriority = iota
	artPrimary
	artSecondary
	artAvatar
	artPrefetch
	artPriorityCount
)

func (p artworkPriority) browses() bool { return p == artPrimary || p == artSecondary }

type artDemand struct {
	artRequest
	priority artworkPriority
}

// A frame records visible misses before starting work. On desktop the library
// is drawn first, but its thumbnails must not get a head start over the page.
type artworkFrame struct {
	cache      *artworkCache
	covers     *downloads.CoverIndex
	wanted     map[artRequest]artworkPriority
	order      [artPriorityCount][]artRequest
	playerEdge int // largest player cover drawn in this frame
	seen       map[artRequest]bool
	again      bool
}

type artworkCache struct {
	ctx                   context.Context
	mu                    sync.Mutex
	small, browse, player artworkPool
	smallPreviews         artworkPool
	browsePreviews        artworkPool
	frame                 uint64
	lastPlayerCover       artRequest // source last shown, never pins pixels outside the LRU
	clock                 uint64
	queue                 []artDemand
	running               map[*artEntry]*artworkLoad
	graceTimer            *time.Timer
	graceDeadline         time.Time
	disk                  *artworkDisk
	downloadCovers        *downloads.Manager
	indexed               *downloads.CoverIndex
	decoding              int
	decodeQueue           []*artworkDecodeWaiter
	workers               sync.WaitGroup
	redraw                func()
	client                *http.Client
	retryTimer            *time.Timer
	retryAt               time.Time
	visible               map[artRequest]bool
	httpOnce              sync.Once
	httpClient            *http.Client
	urlLoads              map[string]chan struct{}
}

func newArtworkCache(ctx context.Context, redraw func()) *artworkCache {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = netproxy.Proxy
	c := &artworkCache{
		ctx:            ctx,
		small:          artworkPool{entries: make(map[artRequest]*artEntry), budget: artworkSmallBudget, limit: artworkSmallEntries},
		browse:         artworkPool{entries: make(map[artRequest]*artEntry), budget: artworkBrowseBudget, limit: artworkBrowseEntries},
		player:         artworkPool{entries: make(map[artRequest]*artEntry), budget: artworkPlayerBudget, limit: artworkPlayerEntries},
		smallPreviews:  newArtworkPreviewPool(),
		browsePreviews: newArtworkPreviewPool(),
		frame:          1,
		running:        make(map[*artEntry]*artworkLoad),
		redraw:         redraw,
		client: &http.Client{
			Transport: transport,
			Timeout:   15 * time.Second,
			CheckRedirect: func(r *http.Request, via []*http.Request) error {
				if len(via) >= 10 || !artworkURL(r.URL) {
					return http.ErrUseLastResponse
				}
				return nil
			},
		},
	}
	context.AfterFunc(ctx, func() {
		c.mu.Lock()
		c.scheduleGrace(time.Time{})
		if c.retryTimer != nil {
			c.retryTimer.Stop()
			c.retryTimer = nil
		}
		c.mu.Unlock()
		transport.CloseIdleConnections()
	})
	return c
}

// A rendition belongs to one pool. A player and a card asking for exactly
// the same rendition share its in-flight request, bitmap and charge.
func (c *artworkCache) pool(key artRequest) *artworkPool {
	if key.edge <= artworkBuckets[0] {
		return &c.small
	}
	if _, ok := c.player.entries[key]; ok {
		return &c.player
	}
	return &c.browse
}

func (c *artworkCache) admissionPool(d artDemand) *artworkPool {
	p := c.pool(d.artRequest)
	if p == &c.browse && p.entries[d.artRequest] == nil && (d.priority == artPlayer || d.priority == artPrefetch) {
		return &c.player
	}
	return p
}

func (c *artworkCache) entry(key artRequest) *artEntry { return c.pool(key).entries[key] }

func (c *artworkCache) currentCovers() *downloads.CoverIndex {
	return c.downloadCovers.Covers()
}

func (key artRequest) matches(covers *downloads.CoverIndex) bool {
	if key.avatar {
		return true
	}
	source := covers.Source(key.url)
	return key.asset == source.Asset && key.generation == source.Generation
}

func (f *artworkFrame) key(raw string, edge int, priority artworkPriority) artRequest {
	key := artRequest{edge: edge, avatar: priority == artAvatar}
	if !key.avatar {
		source := f.covers.Source(raw)
		key.asset, key.generation = source.Asset, source.Generation
		if key.asset != nil {
			key.url = key.asset.URL()
			return key
		}
	}
	key.url = artworkRequestAt(raw, edge)
	return key
}

func (f *artworkFrame) sourceCurrent(raw string) bool {
	current := f.cache.currentCovers()
	return current == f.covers || current.Source(raw) == f.covers.Source(raw)
}

func (c *artworkCache) beginFrame() artworkFrame {
	c.mu.Lock()
	defer c.mu.Unlock()
	covers := c.currentCovers()
	if c.indexed != covers {
		for _, p := range []*artworkPool{&c.small, &c.browse, &c.player, &c.smallPreviews, &c.browsePreviews} {
			for key, e := range p.entries {
				if !key.matches(covers) {
					if e.cancel != nil {
						e.cancel()
					}
					p.bytes -= e.bytes
					delete(p.entries, key)
				}
			}
		}
		c.queue = slices.DeleteFunc(c.queue, func(d artDemand) bool { return !d.matches(covers) })
		if last := c.lastPlayerCover; last.url != "" && !last.matches(covers) {
			c.lastPlayerCover = artRequest{}
		}
		c.indexed = covers
	}
	return artworkFrame{cache: c, covers: covers}
}

func artworkURL(u *url.URL) bool {
	if u.Scheme != "https" || u.User != nil || u.Port() != "" {
		return false
	}
	if catalog.IsSystemArtworkURL(u) || catalog.IsArtworkProxyURL(u) {
		return true
	}
	h := strings.ToLower(u.Hostname())
	for _, base := range []string{"googleusercontent.com", "ggpht.com", "ytimg.com"} {
		if h == base || strings.HasSuffix(h, "."+base) {
			return true
		}
	}
	return false
}

func (f *artworkFrame) get(raw string, requestedEdge int, priority artworkPriority) ui.ImageSource {
	c := f.cache
	if c.ctx.Err() != nil {
		return nil
	}
	edge := artworkBucket(requestedEdge)
	key := f.key(raw, edge, priority)
	u, err := url.Parse(key.url)
	c.mu.Lock()
	defer c.mu.Unlock()
	if err != nil || !artworkURL(u) || (!key.avatar && !f.sourceCurrent(raw)) {
		if priority == artPlayer {
			c.lastPlayerCover = artRequest{}
		}
		return nil
	}
	f.markSeen(key, priority)
	entry := c.entry(key)
	if priority == artPrefetch && entry != nil && entry.prefetchUncacheable {
		return nil
	}
	c.expirePixels(key, entry)
	if entry != nil {
		c.clock++
		entry.used = c.clock
	}
	if priority == artPlayer {
		f.playerEdge = max(f.playerEdge, edge)
	}
	if entry != nil && entry.bitmap != nil {
		if priority == artPlayer {
			c.lastPlayerCover = key
		}
		f.paint(key, entry, priority)
		return entry.bitmap
	}

	failed := entry != nil && time.Now().Before(entry.retryAt)
	// A larger cached rendition already meets the target. A smaller
	// rendition can cover the size change while the target loads.
	if bitmap, size := f.cachedCover(raw, edge, priority); bitmap != nil {
		if priority == artPlayer {
			c.lastPlayerCover = key
		}
		if size < edge && !failed {
			f.want(key, priority)
		}
		return bitmap
	}
	if !failed {
		f.want(key, priority)
	}
	if priority.browses() {
		if bitmap := f.cachedPreview(raw, key, true); bitmap != nil {
			return bitmap
		}
	}
	if priority == artPlayer {
		if failed {
			c.lastPlayerCover = artRequest{}
		} else if last := c.lastPlayerCover; last.url != "" && last.url != key.url &&
			last.matches(f.covers) && f.sourceCurrent(last.url) {
			// A cold skip keeps the last displayed cover until the new one
			// arrives or fails. A worker never changes the displayed source.
			if bitmap, _ := f.cachedCover(last.url, 0, priority); bitmap != nil {
				return bitmap
			}
		}
	}
	return nil
}

// A newly laid-out scroll container has no previous-frame bounds.
// It can paint an already fresh image without scheduling offscreen work,
// extending the rendition's recency, or treating it as an active HTTP response.
func (c *artworkCache) peek(raw string, requestedEdge int) ui.ImageSource {
	f := artworkFrame{cache: c, covers: c.currentCovers()}
	return f.peek(raw, requestedEdge)
}

func (f *artworkFrame) peek(raw string, requestedEdge int) ui.ImageSource {
	c := f.cache
	c.mu.Lock()
	defer c.mu.Unlock()
	if !f.sourceCurrent(raw) {
		return nil
	}
	if len(c.small.entries)+len(c.browse.entries)+len(c.player.entries)+len(c.smallPreviews.entries)+len(c.browsePreviews.entries) == 0 {
		return nil
	}
	edge := artworkBucket(requestedEdge)
	now := time.Now()
	key := f.key(raw, edge, artPrimary)
	for _, size := range [...]int{edge, artworkEdge, 384, 128} {
		candidate := key
		if size != edge {
			candidate = f.key(raw, size, artPrimary)
		}
		e := c.entry(candidate)
		if e != nil && e.bitmap != nil && e.reusable(now) {
			e.peeked = true
			c.rememberPreview(candidate, e, now, false)
			return e.bitmap
		}
	}
	if bitmap := f.cachedPreview(raw, key, false); bitmap != nil {
		return bitmap
	}
	return nil
}

// A transition can replay a previously displayed image after its row has
// left the view. Remember that actual bitmap so submit can release pixels
// restored by the CPU renderer, including a fallback from another size.
func (f *artworkFrame) replayed(raw string, shown ui.ImageSource) {
	b, ok := shown.(*ui.Bitmap)
	if !ok {
		return
	}
	c := f.cache
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, size := range artworkBuckets {
		if e := c.entry(f.key(raw, size, artPrimary)); e != nil && e.bitmap == b {
			e.peeked = true
			return
		}
	}
}

// Caller holds mu. Look up only existing buckets; never fetch a fallback.
func (f *artworkFrame) cachedCover(raw string, except int, priority artworkPriority) (*ui.Bitmap, int) {
	c := f.cache
	now := time.Now()
	for i := len(artworkBuckets) - 1; i >= 0; i-- {
		size := artworkBuckets[i]
		if size == except {
			continue
		}
		key := f.key(raw, size, priority)
		e := c.entry(key)
		c.expirePixels(key, e)
		if e != nil && e.bitmap != nil && e.reusable(now) {
			f.markSeen(key, priority)
			c.clock++
			e.used = c.clock
			f.paint(key, e, priority)
			return e.bitmap, size
		}
	}
	return nil, 0
}

func (f *artworkFrame) markSeen(key artRequest, priority artworkPriority) {
	if priority == artPrefetch {
		return
	}
	if f.seen == nil {
		f.seen = make(map[artRequest]bool)
	}
	f.seen[key] = true
}

// Schedule a frame so the renderer sees the bitmap before CPU pixels are released.
func (f *artworkFrame) paint(key artRequest, e *artEntry, priority artworkPriority) {
	if priority.browses() {
		f.cache.rememberPreview(key, e, time.Now(), true)
	}
	if priority != artPrefetch && e.pixelBytes != 0 && e.firstPaint == 0 {
		e.firstPaint = f.cache.frame
		f.again = true
	}
}

func (p *artworkPool) releasePixels(e *artEntry) {
	e.bitmap.ReleasePixels()
	p.bytes -= e.pixelBytes
	e.bytes -= e.pixelBytes
	e.pixelBytes = 0
}

func (e *artEntry) reusable(now time.Time) bool {
	return e.local || e.freshUntil.After(now)
}

// Pixels already on screen belong to that displayed image. On a later use,
// expired or non-cacheable pixels must go through HTTP validation again.
func (c *artworkCache) expirePixels(key artRequest, e *artEntry) {
	if e == nil || e.bitmap == nil || c.visible[key] || e.reusable(time.Now()) {
		return
	}
	c.pool(key).bytes -= e.bytes
	e.bitmap, e.bytes, e.pixelBytes, e.firstPaint, e.tinted = nil, 0, 0, 0, false
}

func (f *artworkFrame) want(key artRequest, priority artworkPriority) {
	if previous, ok := f.wanted[key]; ok && previous <= priority {
		return
	}
	e := f.cache.entry(key)
	if len(f.order[priority]) < artworkQueueLimit+artworkWorkers || (e != nil && e.cancel != nil) {
		if f.wanted == nil {
			f.wanted = make(map[artRequest]artworkPriority)
		}
		f.wanted[key] = priority
		f.order[priority] = append(f.order[priority], key)
	}
}

func (f *artworkFrame) submit() bool {
	c := f.cache
	c.mu.Lock()
	defer c.mu.Unlock()
	if f.playerEdge == 0 {
		c.lastPlayerCover = artRequest{}
	}
	previous := c.visible
	c.visible = f.seen
	now := time.Now()
	c.updateLoads(f.wanted, now)
	for _, pool := range []*artworkPool{&c.small, &c.browse, &c.player} {
		for key, e := range pool.entries {
			if e.bitmap != nil && !e.local && e.freshUntil.IsZero() && !f.seen[key] {
				// Non-reusable image objects live only while displayed.
				pool.bytes -= e.bytes
				delete(pool.entries, key)
			} else if e.pixelBytes > 0 && (!f.seen[key] || (e.firstPaint > 0 && e.firstPaint < c.frame)) {
				pool.releasePixels(e)
			} else if e.bitmap != nil && (previous[key] || e.peeked) && !f.seen[key] {
				// The CPU renderer can have restored pixels while the image was
				// on screen. Drop that working set when it leaves the viewport.
				e.bitmap.ReleasePixels()
			}
			e.peeked = false
		}
	}
	c.frame++
	clear(c.queue)
	c.queue = c.queue[:0]
	covers := c.currentCovers()
	for tier, keys := range f.order {
		priority := artworkPriority(tier)
		for _, key := range keys {
			// A thumbnail shared by the library and the page uses the page's tier.
			if f.wanted[key] != priority || len(c.queue) >= artworkQueueLimit || !key.matches(covers) {
				continue
			}
			// Recheck prefetch after pixel release and any completion since get.
			if e := c.entry(key); e != nil && (e.cancel != nil || e.bitmap != nil || now.Before(e.retryAt) ||
				(priority == artPrefetch && c.prefetchBlocked(e, covers, now))) {
				continue
			}
			c.queue = append(c.queue, artDemand{key, priority})
		}
	}
	c.startQueued()
	return f.again
}

func (c *artworkCache) download(ctx context.Context, d artDemand) ([]byte, http.Header) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, d.url, nil)
	if err != nil {
		return nil, nil
	}
	client := c.artworkHTTPClient(d)
	if client != c.client {
		// CDNs without size parameters can serve several renditions from one URL.
		// Let an earlier size populate the HTTP cache before looking up the next.
		for {
			c.mu.Lock()
			done := c.urlLoads[d.url]
			if done == nil {
				if c.urlLoads == nil {
					c.urlLoads = make(map[string]chan struct{})
				}
				done = make(chan struct{})
				c.urlLoads[d.url] = done
				c.mu.Unlock()
				defer func() {
					c.mu.Lock()
					delete(c.urlLoads, d.url)
					close(done)
					c.mu.Unlock()
				}()
				break
			}
			c.mu.Unlock()
			select {
			case <-ctx.Done():
				return nil, nil
			case <-done:
			}
		}
	}
	res, err := client.Do(req)
	if err != nil {
		return nil, nil
	}
	defer res.Body.Close()
	// V0.14 returns the stored body and the merged headers after a 304.
	// Ordinary cache hits and stale-if-error responses carry no new authority.
	if client == c.client || res.Header.Get("X-Httpcache-Status") == "REVALIDATED" {
		c.observeArtworkResponse(ctx, res)
	}
	if res.StatusCode != http.StatusOK || res.ContentLength > artworkDownloadLimit {
		return nil, nil
	}
	data, err := io.ReadAll(io.LimitReader(res.Body, artworkDownloadLimit+1))
	if err != nil || len(data) > artworkDownloadLimit {
		return nil, nil
	}
	return data, res.Header
}

// decoded is a cover ready to draw, with the colour it lends the interface.
type decoded struct {
	bitmap       *ui.Bitmap
	preview      *ui.Bitmap
	encodedBytes int
	tint         ui.Color
	tinted       bool
	freshUntil   time.Time
	bodyID       uint64
	local        bool
	invalid      bool
}

func (c *artworkCache) load(ctx context.Context, d artDemand, e *artEntry) decoded {
	if ctx.Err() != nil || !d.matches(c.currentCovers()) {
		return decoded{}
	}
	if asset := d.asset; asset != nil {
		data, err := asset.Read(ctx)
		if err != nil {
			if errors.Is(err, downloads.ErrInvalidCover) {
				asset.Reject(ctx)
			}
			return decoded{}
		}
		art := c.decodePixels(ctx, e, data, d.edge, time.Time{}, true)
		if art.invalid {
			asset.Reject(ctx)
		}
		art.local = art.bitmap != nil
		return art
	}
	if !d.avatar {
		if c.downloadCovers != nil {
			ctx = context.WithValue(ctx, artworkSourceKey{}, d.artRequest)
		}
		ctx = context.WithValue(ctx, artworkPreviewHTTPKey{}, e)
	}
	data, headers := c.download(ctx, d)
	art := c.decodeResponse(ctx, e, data, headers, d)
	if art.invalid && ctx.Err() == nil && !d.avatar && c.disk != nil &&
		artworkDiskKey(d.url) != "" && d.matches(c.currentCovers()) {
		u, _ := url.Parse(d.url)
		_ = c.disk.Delete(urlkey.Normalize(u))
		if headers.Get("X-From-Cache") == "1" && ctx.Err() == nil {
			data, headers = c.download(ctx, d)
			art = c.decodeResponse(ctx, e, data, headers, d)
		}
	}
	return art
}

func (c *artworkCache) decode(ctx context.Context, data []byte, edge int) decoded {
	return c.decodePixels(ctx, nil, data, edge, time.Time{}, false)
}

func (c *artworkCache) decodeResponse(ctx context.Context, e *artEntry, data []byte, headers http.Header, d artDemand) decoded {
	freshUntil := artworkFreshUntil(headers, time.Now())
	art := c.decodePixels(ctx, e, data, d.edge, freshUntil, !d.avatar && !freshUntil.IsZero())
	art.freshUntil = freshUntil
	if art.bitmap != nil && !d.avatar {
		art.bodyID = artworkBodyID(data)
	}
	return art
}

func (c *artworkCache) decodePixels(ctx context.Context, e *artEntry, data []byte, edge int, freshUntil time.Time, preview bool) decoded {
	if len(data) == 0 || !c.takeDecoder(ctx, e, freshUntil) {
		return decoded{}
	}
	defer c.releaseDecoder()
	if ctx.Err() != nil {
		return decoded{}
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || cfg.Width < 1 || cfg.Height < 1 || cfg.Width > 4096 || cfg.Height > 4096 || cfg.Width*cfg.Height > artworkSourcePixels {
		return decoded{invalid: true}
	}
	im, _, err := image.Decode(bytes.NewReader(data))
	if ctx.Err() != nil {
		return decoded{}
	}
	if err != nil {
		return decoded{invalid: true}
	}
	tint, tinted := artworkTint(im)
	w, h := im.Bounds().Dx(), im.Bounds().Dy()
	if w > edge || h > edge {
		scale := float64(edge) / float64(max(w, h))
		w = max(1, int(float64(w)*scale))
		h = max(1, int(float64(h)*scale))
		dst := image.NewRGBA(image.Rect(0, 0, w, h))
		draw.CatmullRom.Scale(dst, dst.Bounds(), im, im.Bounds(), draw.Src, nil)
		// MyGo restores from these exact bytes if its GPU texture is lost.
		// Keep oversized non-resizable CDN originals out of the restore path.
		var encoded bytes.Buffer
		if dst.Opaque() {
			err = jpeg.Encode(&encoded, dst, &jpeg.Options{Quality: 90})
		} else {
			err = png.Encode(&encoded, dst)
		}
		if err != nil {
			return decoded{}
		}
		data = encoded.Bytes()
		im = dst
	}
	if ctx.Err() != nil {
		return decoded{}
	}
	bitmap, err := ui.DecodeBitmap(data)
	if err != nil {
		return decoded{invalid: true}
	}
	art := decoded{bitmap: bitmap, encodedBytes: cap(data), tint: tint, tinted: tinted}
	if preview {
		art.preview = newArtworkPreview(im, data)
	}
	return art
}

// tint is the colour a cover lends the interface, from whichever size of it
// has loaded.
func (c *artworkCache) tint(raw string) (ui.Color, bool) {
	f := artworkFrame{cache: c, covers: c.currentCovers()}
	return f.tint(raw)
}

func (f *artworkFrame) tint(raw string) (ui.Color, bool) {
	c := f.cache
	c.mu.Lock()
	defer c.mu.Unlock()
	if !f.sourceCurrent(raw) {
		return ui.Color{}, false
	}
	now := time.Now()
	for _, size := range artworkBuckets {
		key := f.key(raw, size, artPrimary)
		e := c.entry(key)
		// A non-cacheable response may colour the cover while it is displayed.
		if e != nil && e.tinted && (e.reusable(now) || c.visible[key] || f.seen[key]) {
			return e.tint, true
		}
	}
	return ui.Color{}, false
}

func (c *artworkCache) finish(d artDemand, e *artEntry, art decoded) {
	bitmap := art.bitmap
	c.mu.Lock()
	job := c.running[e]
	now := time.Now()
	wanted := job.graceUntil.IsZero()
	version, allowed := job.responseVersion(&art, now)
	if e.withdrawn || job.ctx.Err() != nil || !allowed ||
		(!wanted && (!job.graceUntil.After(now) || bitmap == nil || (!art.local && !art.freshUntil.After(now)))) {
		c.abandonLoad(job)
	}
	delete(c.running, e)
	// A canceled request may return after the same cover has become visible
	// again. Only the entry that started this load can publish its result.
	pool := c.pool(d.artRequest)
	current := pool.entries[d.artRequest] == e
	e.cancel()
	e.cancel = nil
	e.prefetchDeferredBytes = 0
	covers, unlockCovers := c.downloadCovers.LockCovers()
	if current && !d.matches(covers) {
		pool.bytes -= e.bytes
		delete(pool.entries, d.artRequest)
		current = false
	}
	uncacheable := bitmap != nil && !art.local && !art.freshUntil.After(now)
	if current && bitmap != nil && !uncacheable && d.asset == nil && !d.avatar && job.newer.request == 0 {
		c.acceptArtworkBody(d.artRequest, version, art.freshUntil)
	}
	if current && wanted && job.priority == artPrefetch && uncacheable {
		// Remember this outcome in the LRU so prefetch stays idle. A visible
		// request can still load the image immediately for display.
		e.prefetchUncacheable = true
		current = false
	}
	if current {
		pixels, charge := 0, 0
		deferred := false
		if bitmap != nil {
			// submit releases pixels on the view thread. Until then a background
			// completion must pay their full cost without displacing useful covers.
			pixels = artworkPixelCost(bitmap.Size())
			charge = art.encodedBytes + pixels
			for pool.bytes+charge > pool.budget {
				if !wanted || !pool.evictOldest(c.visible) {
					bitmap = nil
					break
				}
			}
			if bitmap != nil {
				var room bool
				room, deferred = c.makeArtworkRoom(charge, wanted && job.priority < artPrefetch, covers, now)
				if !room {
					bitmap = nil
				}
			}
		}
		if deferred && wanted {
			// Space reserved for a return preview is a scheduling decision.
			e.prefetchDeferredBytes = charge
			e.retryAt = time.Time{}
			current = false
		} else if bitmap == nil {
			if wanted {
				delay := min(artworkRetryBase<<e.failures, artworkRetryMax)
				e.failures = min(e.failures+1, 6)
				e.retryAt = now.Add(delay)
			} else {
				pool.bytes -= e.bytes
				delete(pool.entries, d.artRequest)
			}
		} else {
			e.bitmap = bitmap
			e.version = version
			e.tint, e.tinted = art.tint, art.tinted
			e.freshUntil = art.freshUntil
			e.local = art.local
			e.prefetchUncacheable = false
			e.failures = 0
			e.retryAt = time.Time{}
			e.pixelBytes = pixels
			e.bytes = charge
			pool.bytes += e.bytes
			if art.preview != nil && !uncacheable {
				c.stagePreview(d.artRequest, art, version, wanted && job.priority.browses())
			}
		}
		if wanted {
			c.scheduleRetry(now)
		}
	}
	unlockCovers()
	c.startQueued()
	c.mu.Unlock()
	if current && wanted && c.redraw != nil && c.ctx.Err() == nil {
		c.redraw()
	}
}

// One timer wakes the view when failed entries become retryable, even while
// playback is idle. Only visible views request work, so offscreen images are not retried.
// Caller holds mu; expired entries need no further wakeup until requested again.
func (c *artworkCache) scheduleRetry(now time.Time) {
	if c.redraw == nil || c.ctx.Err() != nil {
		return
	}
	next := time.Time{}
	for _, pool := range []*artworkPool{&c.small, &c.browse, &c.player} {
		for _, e := range pool.entries {
			if e.cancel == nil && e.bitmap == nil && e.retryAt.After(now) && (next.IsZero() || e.retryAt.Before(next)) {
				next = e.retryAt
			}
		}
	}
	if c.retryTimer != nil {
		if next.Equal(c.retryAt) {
			return
		}
		c.retryTimer.Stop()
		c.retryTimer = nil
	}
	c.retryAt = next
	if next.IsZero() {
		return
	}
	var timer *time.Timer
	timer = time.AfterFunc(time.Until(next), func() {
		c.mu.Lock()
		if c.retryTimer != timer {
			c.mu.Unlock()
			return
		}
		c.retryTimer = nil
		c.scheduleRetry(time.Now())
		c.mu.Unlock()
		if c.ctx.Err() == nil {
			c.redraw()
		}
	})
	c.retryTimer = timer
}

// Caller holds mu. In-flight loads keep their entries until they finish or are canceled.
func (p *artworkPool) evictOldest(visible map[artRequest]bool) bool {
	oldest, tick := artRequest{}, ^uint64(0)
	for key, entry := range p.entries {
		if !visible[key] && entry.cancel == nil && entry.used < tick {
			oldest, tick = key, entry.used
		}
	}
	if oldest.url == "" {
		return false
	}
	p.bytes -= p.entries[oldest].bytes
	delete(p.entries, oldest)
	return true
}

func artworkPixelCost(w, h int) int {
	cost := 0
	for {
		cost += 4 * w * h
		if w == 1 && h == 1 {
			return cost
		}
		w, h = max(1, (w+1)/2), max(1, (h+1)/2)
	}
}

// Request one deterministic high-resolution image from image CDN formats that
// define a size suffix. Unrecognized source URLs are left untouched.
func artworkRequest(raw string) string {
	return artworkRequestAt(raw, artworkEdge)
}

var artworkBuckets = [...]int{128, 384, artworkEdge}

func artworkBucket(edge int) int {
	for _, size := range artworkBuckets {
		if edge <= size {
			return size
		}
	}
	return artworkEdge
}

func artworkRequestAt(raw string, edge int) string {
	u, err := url.Parse(raw)
	if err != nil || !artworkURL(u) {
		return raw
	}
	return downloads.CoverURLAtSize(raw, u, edge)
}
