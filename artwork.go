package main

import (
	"bytes"
	"context"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"ytmgo/internal/catalog"
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
	artworkDownloadLimit  = 4 << 20
	artworkSourcePixels   = 4 << 20
	artworkEdge           = 768
	artworkRetryBase      = 2 * time.Second
	artworkRetryMax       = time.Minute
)

// Cache charges include the encoded backing and, until first paint, the base
// pixels plus possible mipmaps. The GPU owns its texture lifetime.
type artEntry struct {
	bitmap     *ui.Bitmap
	tint       ui.Color // the cover's colour for the interface, when tinted
	tinted     bool
	bytes      int
	pixelBytes int
	firstPaint uint64
	used       uint64
	cancel     context.CancelFunc
	failures   int
	retryAt    time.Time
	freshUntil time.Time
}

type artworkPool struct {
	entries              map[artRequest]*artEntry
	bytes, budget, limit int
}
type artRequest struct {
	url  string
	edge int
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

type artDemand struct {
	artRequest
	priority artworkPriority
}

// A frame records visible misses before starting work. On desktop the library
// is drawn first, but its thumbnails must not get a head start over the page.
type artworkFrame struct {
	cache      *artworkCache
	wanted     map[artRequest]artworkPriority
	order      [artPriorityCount][]artRequest
	playerEdge int // largest player cover drawn in this frame
	seen       map[artRequest]bool
	again      bool
}

type artworkCache struct {
	ctx                                  context.Context
	mu                                   sync.Mutex
	small, browse, player                artworkPool
	frame                                uint64
	lastPlayerCover                      string // source last shown, never pins pixels outside the LRU
	clock                                uint64
	queue                                []artDemand
	active, secondaryActive, largeActive int
	disk                                 *artworkDisk
	decoding                             chan struct{}
	workers                              sync.WaitGroup
	redraw                               func()
	client                               *http.Client
	retryTimer                           *time.Timer
	retryAt                              time.Time
	visible                              map[artRequest]bool
	httpOnce                             sync.Once
	httpClient                           *http.Client
	urlLoads                             map[string]chan struct{}
}

func newArtworkCache(ctx context.Context, redraw func()) *artworkCache {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = netproxy.Proxy
	c := &artworkCache{
		ctx:      ctx,
		small:    artworkPool{entries: make(map[artRequest]*artEntry), budget: artworkSmallBudget, limit: artworkSmallEntries},
		browse:   artworkPool{entries: make(map[artRequest]*artEntry), budget: artworkBrowseBudget, limit: artworkBrowseEntries},
		player:   artworkPool{entries: make(map[artRequest]*artEntry), budget: artworkPlayerBudget, limit: artworkPlayerEntries},
		frame:    1,
		decoding: make(chan struct{}, artworkDecoders),
		redraw:   redraw,
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
	key := artRequest{url: artworkRequestAt(raw, edge), edge: edge}
	u, err := url.Parse(key.url)
	c.mu.Lock()
	defer c.mu.Unlock()
	if err != nil || !artworkURL(u) {
		if priority == artPlayer {
			c.lastPlayerCover = ""
		}
		return nil
	}
	if f.seen == nil {
		f.seen = make(map[artRequest]bool)
	}
	f.seen[key] = true
	entry := c.entry(key)
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
			c.lastPlayerCover = raw
		}
		f.paint(entry, priority)
		return entry.bitmap
	}

	failed := entry != nil && time.Now().Before(entry.retryAt)
	// A cover cached at another size paints at once: a smaller one while
	// this size loads, a larger one as it is. When a panel opens or the
	// window resizes, covers change size bucket; without this every one
	// blanked to a placeholder and loaded again.
	if bitmap, size := f.cachedCover(raw, edge, priority); bitmap != nil {
		if priority == artPlayer {
			c.lastPlayerCover = raw
		}
		if size < edge && !failed {
			f.want(key, priority)
		}
		return bitmap
	}
	if !failed {
		f.want(key, priority)
	}
	if priority == artPlayer {
		if failed {
			c.lastPlayerCover = ""
		} else if c.lastPlayerCover != "" && c.lastPlayerCover != raw {
			// A cold skip keeps the last displayed cover until the new one
			// arrives or fails. A worker never changes the displayed source.
			if bitmap, _ := f.cachedCover(c.lastPlayerCover, 0, priority); bitmap != nil {
				return bitmap
			}
		}
	}
	return nil
}

// A newly laid-out scroll container has no previous-frame bounds.
// It can paint an already fresh rendition without scheduling offscreen work,
// prolonging its lifetime, or treating it as an active HTTP response.
func (c *artworkCache) peek(raw string, requestedEdge int) ui.ImageSource {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.small.entries)+len(c.browse.entries)+len(c.player.entries) == 0 {
		return nil
	}
	edge := artworkBucket(requestedEdge)
	now := time.Now()
	for _, size := range [...]int{edge, artworkEdge, 384, 128} {
		e := c.entry(artRequest{artworkRequestAt(raw, size), size})
		if e != nil && e.bitmap != nil && e.freshUntil.After(now) {
			return e.bitmap
		}
	}
	return nil
}

// Caller holds mu. Look up only existing buckets; never fetch a fallback.
func (f *artworkFrame) cachedCover(raw string, except int, priority artworkPriority) (*ui.Bitmap, int) {
	c := f.cache
	for i := len(artworkBuckets) - 1; i >= 0; i-- {
		size := artworkBuckets[i]
		if size == except {
			continue
		}
		key := artRequest{artworkRequestAt(raw, size), size}
		e := c.entry(key)
		c.expirePixels(key, e)
		if e != nil && e.bitmap != nil {
			f.seen[key] = true
			c.clock++
			e.used = c.clock
			f.paint(e, priority)
			return e.bitmap, size
		}
	}
	return nil, 0
}

// get and submit run on the view thread. Schedule one further frame so that
// the renderer has seen the bitmap before we release its CPU-side pixels.
func (f *artworkFrame) paint(e *artEntry, priority artworkPriority) {
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

// Pixels already on screen belong to that displayed image. On a later use,
// expired or non-cacheable pixels must go through HTTP validation again.
func (c *artworkCache) expirePixels(key artRequest, e *artEntry) {
	if e == nil || e.bitmap == nil || c.visible[key] || e.freshUntil.After(time.Now()) {
		return
	}
	c.pool(key).bytes -= e.bytes
	e.bitmap, e.bytes, e.pixelBytes, e.firstPaint, e.tinted = nil, 0, 0, 0, false
}

func (f *artworkFrame) want(key artRequest, priority artworkPriority) {
	if previous, ok := f.wanted[key]; ok && previous <= priority {
		return
	}
	if len(f.order[priority]) < artworkQueueLimit+artworkWorkers {
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
		c.lastPlayerCover = ""
	}
	previous := c.visible
	c.visible = f.seen
	for _, pool := range []*artworkPool{&c.small, &c.browse, &c.player} {
		for key, e := range pool.entries {
			if _, visible := f.wanted[key]; e.cancel != nil && !visible {
				e.cancel()
				pool.bytes -= e.bytes
				delete(pool.entries, key)
			} else if e.bitmap != nil && e.freshUntil.IsZero() && !f.seen[key] {
				// Non-reusable image objects live only while displayed.
				pool.bytes -= e.bytes
				delete(pool.entries, key)
			} else if e.pixelBytes > 0 && (!f.seen[key] || (e.firstPaint > 0 && e.firstPaint < c.frame)) {
				pool.releasePixels(e)
			} else if e.bitmap != nil && previous[key] && !f.seen[key] {
				// The CPU renderer can have restored pixels while the image was
				// on screen. Drop that working set when it leaves the viewport.
				e.bitmap.ReleasePixels()
			}
		}
	}
	c.frame++
	if c.active == 0 && len(f.wanted) == 0 {
		return f.again
	}
	clear(c.queue)
	c.queue = c.queue[:0]
	now := time.Now()
	for tier, keys := range f.order {
		priority := artworkPriority(tier)
		for _, key := range keys {
			// A thumbnail shared by the library and the page uses the page's tier.
			if f.wanted[key] != priority || len(c.queue) >= artworkQueueLimit {
				continue
			}
			if e := c.entry(key); e != nil && (e.cancel != nil || e.bitmap != nil || now.Before(e.retryAt)) {
				continue
			}
			c.queue = append(c.queue, artDemand{key, priority})
		}
	}
	c.startQueued()
	return f.again
}

// Caller holds mu. The secondary limit leaves room for page covers even if
// the library finishes loading before the catalogue response arrives.
func (c *artworkCache) startQueued() {
	for len(c.queue) > 0 && c.active < artworkWorkers && c.ctx.Err() == nil {
		// Small visible thumbnails can use the remaining slots while large
		// cards share the link. Preserve display order among eligible covers.
		index := slices.IndexFunc(c.queue, func(d artDemand) bool {
			return (d.edge <= 128 || c.largeActive < artworkLargeLimit) &&
				(d.priority <= artPrimary || c.secondaryActive < artworkSecondaryLimit)
		})
		if index < 0 {
			return
		}
		d := c.queue[index]
		c.queue = slices.Delete(c.queue, index, index+1)
		pool := c.admissionPool(d)
		e := pool.entries[d.artRequest]
		if e == nil {
			if len(pool.entries) >= pool.limit && !pool.evictOldest(c.visible) {
				continue
			}
			e = &artEntry{}
			pool.entries[d.artRequest] = e
		}
		ctx, cancel := context.WithCancel(c.ctx)
		e.cancel = cancel
		c.clock++
		e.used = c.clock
		c.active++
		if d.edge > 128 {
			c.largeActive++
		}
		if d.priority > artPrimary {
			c.secondaryActive++
		}
		c.workers.Go(func() {
			c.finish(d, e, c.load(ctx, d))
		})
	}
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
	encodedBytes int
	tint         ui.Color
	tinted       bool
	freshUntil   time.Time
}

func (c *artworkCache) load(ctx context.Context, d artDemand) decoded {
	if ctx.Err() != nil {
		return decoded{}
	}
	data, headers := c.download(ctx, d)
	requested := time.Now()
	art := c.decode(ctx, data, d.edge)
	if art.bitmap == nil && len(data) > 0 && ctx.Err() == nil && c.disk != nil && d.priority != artAvatar && artworkDiskKey(d.url) != "" {
		u, _ := url.Parse(d.url)
		_ = c.disk.Delete(urlkey.Normalize(u))
		if headers.Get("X-From-Cache") == "1" && ctx.Err() == nil {
			data, headers = c.download(ctx, d)
			art = c.decode(ctx, data, d.edge)
		}
	}
	art.freshUntil = artworkFreshUntil(headers, requested)
	return art
}

func (c *artworkCache) decode(ctx context.Context, data []byte, edge int) decoded {
	if len(data) == 0 {
		return decoded{}
	}
	select {
	case c.decoding <- struct{}{}:
		defer func() { <-c.decoding }()
	case <-ctx.Done():
		return decoded{}
	}
	if ctx.Err() != nil {
		return decoded{}
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || cfg.Width < 1 || cfg.Height < 1 || cfg.Width > 4096 || cfg.Height > 4096 || cfg.Width*cfg.Height > artworkSourcePixels {
		return decoded{}
	}
	im, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return decoded{}
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
	}
	if ctx.Err() != nil {
		return decoded{}
	}
	bitmap, err := ui.DecodeBitmap(data)
	if err != nil {
		return decoded{}
	}
	return decoded{bitmap: bitmap, encodedBytes: cap(data), tint: tint, tinted: tinted}
}

// tint is the colour a cover lends the interface, from whichever size of it
// has loaded.
func (c *artworkCache) tint(raw string) (ui.Color, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, size := range artworkBuckets {
		if e := c.entry(artRequest{artworkRequestAt(raw, size), size}); e != nil && e.tinted {
			return e.tint, true
		}
	}
	return ui.Color{}, false
}

func (c *artworkCache) finish(d artDemand, e *artEntry, art decoded) {
	bitmap := art.bitmap
	c.mu.Lock()
	c.active--
	if d.edge > 128 {
		c.largeActive--
	}
	if d.priority > artPrimary {
		c.secondaryActive--
	}
	// A canceled request may return after the same cover has become visible
	// again. Only the entry that started this load can publish its result.
	pool := c.pool(d.artRequest)
	current := pool.entries[d.artRequest] == e
	e.cancel()
	e.cancel = nil
	if current {
		pixels := 0
		if bitmap != nil {
			pixels = artworkPixelCost(bitmap.Size())
			charge := art.encodedBytes + pixels
			for pool.bytes+charge > pool.budget {
				if !pool.evictOldest(c.visible) {
					bitmap = nil
					break
				}
			}
		}
		if bitmap == nil {
			delay := min(artworkRetryBase<<e.failures, artworkRetryMax)
			e.failures = min(e.failures+1, 6)
			e.retryAt = time.Now().Add(delay)
		} else {
			e.bitmap = bitmap
			e.tint, e.tinted = art.tint, art.tinted
			e.freshUntil = art.freshUntil
			e.failures = 0
			e.retryAt = time.Time{}
			e.pixelBytes = pixels
			e.bytes = art.encodedBytes + pixels
			pool.bytes += e.bytes
		}
		c.scheduleRetry(time.Now())
	}
	c.startQueued()
	c.mu.Unlock()
	if current && c.redraw != nil && c.ctx.Err() == nil {
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

var artworkSize = regexp.MustCompile(`=(?:w\d+-h\d+|s\d+)([^/?]*)$`)

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
	h := strings.ToLower(u.Hostname())
	if strings.HasSuffix(h, "googleusercontent.com") || strings.HasSuffix(h, "ggpht.com") {
		size := strconv.Itoa(edge)
		return artworkSize.ReplaceAllString(raw, "=w"+size+"-h"+size+"$1")
	}
	return raw
}
