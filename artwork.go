package main

import (
	"bytes"
	"context"
	"image"
	_ "image/jpeg"
	_ "image/png"
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
	artworkEntryLimit     = 160
	artworkPixelBudget    = 32 << 20
	artworkDownloadLimit  = 4 << 20
	artworkSourcePixels   = 4 << 20
	artworkEdge           = 768
	artworkRetryBase      = 2 * time.Second
	artworkRetryMax       = time.Minute
)

// Artwork uses a bounded work queue and a 32 MiB retained pixel budget,
// including every mipmap the native renderer may create. In-flight decoding
// and renderer-owned GPU textures are separate from that cache budget.
type artEntry struct {
	bitmap   *ui.Bitmap
	tint     ui.Color // the cover's colour for the interface, when tinted
	tinted   bool
	bytes    int
	used     uint64
	cancel   context.CancelFunc
	failures int
	retryAt  time.Time
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
}

type artworkCache struct {
	ctx                                  context.Context
	mu                                   sync.Mutex
	entries                              map[artRequest]*artEntry
	lastPlayerCover                      string // source last shown, never pins pixels outside the LRU
	clock                                uint64
	bytes                                int
	queue                                []artDemand
	active, secondaryActive, largeActive int
	disk                                 *artworkDisk
	decoding                             chan struct{}
	workers                              sync.WaitGroup
	redraw                               func()
	client                               *http.Client
	retryTimer                           *time.Timer
	retryAt                              time.Time
}

func newArtworkCache(ctx context.Context, redraw func()) *artworkCache {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = netproxy.Proxy
	c := &artworkCache{
		ctx:      ctx,
		entries:  make(map[artRequest]*artEntry),
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
	entry := c.entries[key]
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
		return entry.bitmap
	}

	failed := entry != nil && time.Now().Before(entry.retryAt)
	if priority == artPlayer {
		// A queue thumbnail can paint immediately while the large cover loads.
		// Conversely, the player bar can use a prefetched large cover as-is.
		if bitmap, size := c.cachedCover(raw, edge); bitmap != nil {
			c.lastPlayerCover = raw
			if size < edge && !failed {
				f.want(key, priority)
			}
			return bitmap
		}
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
			if bitmap, _ := c.cachedCover(c.lastPlayerCover, 0); bitmap != nil {
				return bitmap
			}
		}
	}
	return nil
}

// Caller holds mu. Look up only existing buckets; never fetch a fallback.
func (c *artworkCache) cachedCover(raw string, except int) (*ui.Bitmap, int) {
	for i := len(artworkBuckets) - 1; i >= 0; i-- {
		size := artworkBuckets[i]
		if size == except {
			continue
		}
		key := artRequest{artworkRequestAt(raw, size), size}
		if e := c.entries[key]; e != nil && e.bitmap != nil {
			c.clock++
			e.used = c.clock
			return e.bitmap, size
		}
	}
	return nil, 0
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

func (f *artworkFrame) submit() {
	c := f.cache
	c.mu.Lock()
	defer c.mu.Unlock()
	if f.playerEdge == 0 {
		c.lastPlayerCover = ""
	}
	if c.active == 0 && len(f.wanted) == 0 {
		return
	}
	for key, e := range c.entries {
		if _, visible := f.wanted[key]; e.cancel != nil && !visible {
			e.cancel()
			delete(c.entries, key)
		}
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
			if e := c.entries[key]; e != nil && (e.cancel != nil || e.bitmap != nil || now.Before(e.retryAt)) {
				continue
			}
			c.queue = append(c.queue, artDemand{key, priority})
		}
	}
	c.startQueued()
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
		e := c.entries[d.artRequest]
		if e == nil {
			if len(c.entries) >= artworkEntryLimit {
				c.evictOldest(artRequest{})
			}
			e = &artEntry{}
			c.entries[d.artRequest] = e
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

func (c *artworkCache) download(ctx context.Context, raw string) ([]byte, time.Time) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
	if err != nil {
		return nil, time.Time{}
	}
	start := time.Now()
	res, err := c.client.Do(req)
	if err != nil {
		return nil, time.Time{}
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK || res.ContentLength > artworkDownloadLimit {
		return nil, time.Time{}
	}
	data, err := io.ReadAll(io.LimitReader(res.Body, artworkDownloadLimit+1))
	if err != nil || len(data) > artworkDownloadLimit {
		return nil, time.Time{}
	}
	var expiry time.Time
	if res.Request == nil || res.Request.URL.String() == raw {
		expiry = artworkFreshUntil(res.Header, start)
	}
	return data, expiry
}

// decoded is a cover ready to draw, with the colour it lends the interface.
type decoded struct {
	bitmap *ui.Bitmap
	tint   ui.Color
	tinted bool
}

func (c *artworkCache) load(ctx context.Context, d artDemand) decoded {
	var key string
	if c.disk != nil && d.priority != artAvatar {
		key = artworkDiskKey(d.url)
	}
	if key != "" {
		if art := c.decode(ctx, c.disk.read(key), d.edge); art.bitmap != nil {
			return art
		}
	}
	if ctx.Err() != nil {
		return decoded{}
	}
	data, expiry := c.download(ctx, d.url)
	art := c.decode(ctx, data, d.edge)
	if key != "" && art.bitmap != nil && ctx.Err() == nil && expiry.After(time.Now()) {
		c.disk.write(key, data, expiry)
	}
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
	w, h := im.Bounds().Dx(), im.Bounds().Dy()
	if w > edge || h > edge {
		scale := float64(edge) / float64(max(w, h))
		w = max(1, int(float64(w)*scale))
		h = max(1, int(float64(h)*scale))
		dst := image.NewRGBA(image.Rect(0, 0, w, h))
		draw.CatmullRom.Scale(dst, dst.Bounds(), im, im.Bounds(), draw.Src, nil)
		im = dst
	}
	// The colour is read from the decoded pixels once, here, so drawing a
	// wash costs a gradient, not a blur, every frame.
	tint, tinted := artworkTint(im)
	return decoded{bitmap: ui.NewBitmap(im), tint: tint, tinted: tinted}
}

// tint is the colour a cover lends the interface, from whichever size of it
// has loaded.
func (c *artworkCache) tint(raw string) (ui.Color, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, size := range artworkBuckets {
		if e := c.entries[artRequest{artworkRequestAt(raw, size), size}]; e != nil && e.tinted {
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
	current := c.entries[d.artRequest] == e
	e.cancel()
	e.cancel = nil
	if current {
		e.bitmap = bitmap
		e.tint, e.tinted = art.tint, art.tinted
		if bitmap == nil {
			delay := min(artworkRetryBase<<e.failures, artworkRetryMax)
			e.failures = min(e.failures+1, 6)
			e.retryAt = time.Now().Add(delay)
		} else {
			e.failures = 0
			e.retryAt = time.Time{}
			e.bytes = artworkPixelCost(bitmap.Size())
			c.bytes += e.bytes
			for c.bytes > artworkPixelBudget {
				if !c.evictOldest(d.artRequest) {
					break
				}
			}
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
	for _, e := range c.entries {
		if e.cancel == nil && e.bitmap == nil && e.retryAt.After(now) && (next.IsZero() || e.retryAt.Before(next)) {
			next = e.retryAt
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
func (c *artworkCache) evictOldest(except artRequest) bool {
	oldest, tick := artRequest{}, ^uint64(0)
	for key, entry := range c.entries {
		if key != except && entry.cancel == nil && entry.used < tick {
			oldest, tick = key, entry.used
		}
	}
	if oldest.url == "" {
		return false
	}
	c.bytes -= c.entries[oldest].bytes
	delete(c.entries, oldest)
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
