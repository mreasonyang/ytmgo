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
	artworkWorkers       = 4
	artworkQueueLimit    = 64
	artworkEntryLimit    = 160
	artworkPixelBudget   = 32 << 20
	artworkDownloadLimit = 4 << 20
	artworkSourcePixels  = 4 << 20
	artworkEdge          = 768
	artworkRetryBase     = 2 * time.Second
	artworkRetryMax      = time.Minute
)

// Artwork uses a bounded work queue and a 32 MiB retained pixel budget,
// including every mipmap the native renderer may create. In-flight decoding
// and renderer-owned GPU textures are separate from that cache budget.
type artEntry struct {
	bitmap   *ui.Bitmap
	bytes    int
	used     uint64
	pending  bool
	failures int
	retryAt  time.Time
}
type artRequest struct {
	url  string
	edge int
}
type artworkCache struct {
	ctx        context.Context
	mu         sync.Mutex
	entries    map[artRequest]*artEntry
	clock      uint64
	bytes      int
	queue      chan artRequest
	workers    sync.WaitGroup
	redraw     func()
	client     *http.Client
	retryTimer *time.Timer
	retryAt    time.Time
}

func newArtworkCache(ctx context.Context, redraw func()) *artworkCache {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = netproxy.Proxy
	c := &artworkCache{ctx: ctx, entries: map[artRequest]*artEntry{}, queue: make(chan artRequest, artworkQueueLimit), redraw: redraw, client: &http.Client{Transport: transport, Timeout: 15 * time.Second, CheckRedirect: func(r *http.Request, via []*http.Request) error {
		if len(via) >= 10 || !artworkURL(r.URL) {
			return http.ErrUseLastResponse
		}
		return nil
	}}}
	context.AfterFunc(ctx, func() {
		c.mu.Lock()
		if c.retryTimer != nil {
			c.retryTimer.Stop()
			c.retryTimer = nil
		}
		c.mu.Unlock()
		transport.CloseIdleConnections()
	})
	for range artworkWorkers {
		c.workers.Add(1)
		go c.work()
	}
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
func (c *artworkCache) get(raw string, requestedEdge int) ui.ImageSource {
	if c.ctx.Err() != nil {
		return nil
	}
	edge := artworkBucket(requestedEdge)
	raw = artworkRequestAt(raw, edge)
	u, err := url.Parse(raw)
	if err != nil || !artworkURL(u) {
		return nil
	}
	request := artRequest{url: raw, edge: edge}
	c.mu.Lock()
	c.clock++
	e := c.entries[request]
	if e != nil {
		e.used = c.clock
		if e.bitmap != nil {
			b := e.bitmap
			c.mu.Unlock()
			return b
		}
		if e.pending || time.Now().Before(e.retryAt) {
			c.mu.Unlock()
			return nil
		}
	}
	for e == nil && len(c.entries) >= artworkEntryLimit {
		if !c.evictOldest(artRequest{}) {
			c.mu.Unlock()
			return nil
		}
	}
	select {
	case c.queue <- request:
		if e == nil {
			e = &artEntry{used: c.clock}
			c.entries[request] = e
		}
		e.pending = true
	default:
		// Completion redraws the visible views, which can request this image
		// again once a worker has made room. Never spawn a waiting goroutine.
	}
	c.mu.Unlock()
	return nil
}

func (c *artworkCache) work() {
	defer c.workers.Done()
	for {
		select {
		case <-c.ctx.Done():
			return
		case raw := <-c.queue:
			if c.ctx.Err() != nil {
				return
			}
			c.load(raw)
		}
	}
}

func (c *artworkCache) load(request artRequest) {
	var bitmap *ui.Bitmap
	cost := 0
	func() {
		req, err := http.NewRequestWithContext(c.ctx, http.MethodGet, request.url, nil)
		if err != nil {
			return
		}
		res, err := c.client.Do(req)
		if err != nil {
			return
		}
		defer res.Body.Close()
		if res.StatusCode != http.StatusOK || res.ContentLength > artworkDownloadLimit {
			return
		}
		data, err := io.ReadAll(io.LimitReader(res.Body, artworkDownloadLimit+1))
		if err != nil || len(data) > artworkDownloadLimit {
			return
		}
		cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
		if err != nil || cfg.Width < 1 || cfg.Height < 1 || cfg.Width > 4096 || cfg.Height > 4096 || cfg.Width*cfg.Height > artworkSourcePixels {
			return
		}
		im, _, err := image.Decode(bytes.NewReader(data))
		if err != nil {
			return
		}
		w, h := im.Bounds().Dx(), im.Bounds().Dy()
		if w > request.edge || h > request.edge {
			scale := float64(request.edge) / float64(max(w, h))
			w = max(1, int(float64(w)*scale))
			h = max(1, int(float64(h)*scale))
			dst := image.NewRGBA(image.Rect(0, 0, w, h))
			draw.CatmullRom.Scale(dst, dst.Bounds(), im, im.Bounds(), draw.Src, nil)
			im = dst
		}
		bitmap = ui.NewBitmap(im)
		cost = artworkPixelCost(w, h)
	}()
	c.mu.Lock()
	e := c.entries[request]
	if e != nil {
		e.pending = false
		e.bitmap = bitmap
		c.bytes += cost - e.bytes
		e.bytes = cost
		if bitmap == nil {
			delay := min(artworkRetryBase<<e.failures, artworkRetryMax)
			e.failures = min(e.failures+1, 6)
			e.retryAt = time.Now().Add(delay)
		} else {
			e.failures = 0
			e.retryAt = time.Time{}
		}
	}
	for c.bytes > artworkPixelBudget {
		if !c.evictOldest(request) {
			break
		}
	}
	c.scheduleRetry(time.Now())
	c.mu.Unlock()
	if c.redraw != nil && c.ctx.Err() == nil {
		c.redraw()
	}
}

// One timer wakes the view when failed entries become retryable, even while
// playback is idle. Only get() queues work, so offscreen images are not retried.
// Caller holds mu; expired entries need no further wakeup until requested again.
func (c *artworkCache) scheduleRetry(now time.Time) {
	if c.redraw == nil || c.ctx.Err() != nil {
		return
	}
	next := time.Time{}
	for _, e := range c.entries {
		if !e.pending && e.bitmap == nil && e.retryAt.After(now) && (next.IsZero() || e.retryAt.Before(next)) {
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

// Caller holds mu. Pending entries are bounded by the queue and worker count.
func (c *artworkCache) evictOldest(except artRequest) bool {
	oldest, tick := artRequest{}, ^uint64(0)
	for key, entry := range c.entries {
		if key != except && !entry.pending && entry.used < tick {
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

func artworkBucket(edge int) int {
	for _, size := range [...]int{128, 384, artworkEdge} {
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
