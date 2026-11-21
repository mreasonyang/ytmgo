package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"image"
	"net/http"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"golang.org/x/image/draw"
)

const (
	artworkPreviewEdge    = 48
	artworkPreviewBudget  = 2 << 20 // per pool, about 170 square previews with all mip levels
	artworkPreviewEntries = 256
)

func newArtworkPreviewPool() artworkPool {
	return artworkPool{entries: make(map[artRequest]*artEntry), budget: artworkPreviewBudget, limit: artworkPreviewEntries}
}

func (c *artworkCache) previewPool(key artRequest) *artworkPool {
	if key.edge == artworkBuckets[0] {
		return &c.smallPreviews
	}
	return &c.browsePreviews
}

// A candidate pays for its pixels in the rendition pool until a browse view
// uses it. From then on the preview LRU owns the pixels and their charge.
func (c *artworkCache) rememberPreview(key artRequest, e *artEntry, now time.Time) {
	if !e.reusable(now) {
		return
	}
	p := c.previewPool(key)
	cached := p.entries[key]
	if e.preview == nil {
		if cached != nil && cached.loadedAt == e.loadedAt {
			c.clock++
			cached.used = c.clock
		}
		return
	}

	bitmap := e.preview
	cost := artworkPixelCost(bitmap.Size())
	e.preview = nil
	e.bytes -= cost
	c.pool(key).bytes -= cost
	if cost > p.budget || p.limit == 0 {
		return
	}
	if cached != nil {
		p.removePreview(key, cached)
	}
	for len(p.entries) >= p.limit || p.bytes+cost > p.budget {
		p.evictOldest(nil)
	}
	c.clock++
	p.entries[key] = &artEntry{bitmap: bitmap, bytes: cost, local: e.local, freshUntil: e.freshUntil, loadedAt: e.loadedAt, used: c.clock}
	p.bytes += cost
}

func (p *artworkPool) removePreview(key artRequest, e *artEntry) {
	p.bytes -= e.bytes
	delete(p.entries, key)
}

func (f *artworkFrame) cachedPreview(raw string, key artRequest, touch bool) *ui.Bitmap {
	c := f.cache
	if len(c.smallPreviews.entries)+len(c.browsePreviews.entries) == 0 {
		return nil
	}
	now := time.Now()
	for _, edge := range [...]int{key.edge, artworkEdge, 384, 128} {
		candidate := key
		if edge != key.edge {
			candidate = f.key(raw, edge, artPrimary)
		}
		p := c.previewPool(candidate)
		if e := p.entries[candidate]; e != nil {
			if !e.reusable(now) {
				p.removePreview(candidate, e)
				continue
			}
			if touch {
				c.clock++
				e.used = c.clock
			}
			return e.bitmap
		}
	}
	return nil
}

// MyGo can draw this small uncompressed bitmap without disk access or a
// decoder. The preview retains only these pixels and possible small mipmaps.
func newArtworkPreview(im image.Image, encoded []byte) *ui.Bitmap {
	bounds := im.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	long := max(w, h)
	if long > artworkPreviewEdge {
		w = max(1, w*artworkPreviewEdge/long)
		h = max(1, h*artworkPreviewEdge/long)
	}
	pixels := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.ApproxBiLinear.Scale(pixels, pixels.Bounds(), im, bounds, draw.Src, nil)
	if orientation := artworkJPEGOrientation(encoded); orientation > 1 {
		pixels = orientArtworkPreview(pixels, orientation)
	}
	return ui.NewBitmap(pixels)
}

// DecodeBitmap applies JPEG orientation. The already decoded image used by
// the scaler does not carry it, so read only the orientation tag from APP1.
func artworkJPEGOrientation(data []byte) uint16 {
	if len(data) < 2 || data[0] != 0xff || data[1] != 0xd8 {
		return 0
	}
	for data = data[2:]; len(data) >= 4 && data[0] == 0xff && data[1] != 0xda; {
		end := 2 + int(binary.BigEndian.Uint16(data[2:]))
		if end < 4 || end > len(data) {
			break
		}
		if data[1] == 0xe1 && end >= 19 && bytes.Equal(data[4:10], []byte("Exif\x00\x00")) {
			return artworkTIFFOrientation(data[10:end])
		}
		data = data[end:]
	}
	return 0
}

func artworkTIFFOrientation(tiff []byte) uint16 {
	var order binary.ByteOrder
	switch string(tiff[:2]) {
	case "II":
		order = binary.LittleEndian
	case "MM":
		order = binary.BigEndian
	default:
		return 0
	}
	offset := int(order.Uint32(tiff[4:]))
	if offset < 8 || offset > len(tiff)-2 {
		return 0
	}
	count := int(order.Uint16(tiff[offset:]))
	fields := tiff[offset+2:]
	for range min(count, len(fields)/12) {
		if order.Uint16(fields) == 0x112 && order.Uint16(fields[2:]) == 3 {
			o := order.Uint16(fields[8:])
			if o <= 8 {
				return o
			}
			break
		}
		fields = fields[12:]
	}
	return 0
}

func orientArtworkPreview(src *image.RGBA, orientation uint16) *image.RGBA {
	w, h := src.Bounds().Dx(), src.Bounds().Dy()
	size := image.Pt(w, h)
	if orientation >= 5 {
		size = image.Pt(h, w)
	}
	dst := image.NewRGBA(image.Rectangle{Max: size})
	for y := range size.Y {
		for x := range size.X {
			sx, sy := x, y
			if orientation >= 5 {
				sx, sy = y, x
			}
			switch orientation {
			case 2, 3, 7, 8:
				sx = w - 1 - sx
			}
			switch orientation {
			case 3, 4, 6, 7:
				sy = h - 1 - sy
			}
			dst.SetRGBA(x, y, src.RGBAAt(sx, sy))
		}
	}
	return dst
}

type artworkPreviewHTTPKey struct{}

// Observe authoritative headers before the HTTP cache can discard an
// unusable body or merge a 304. The HTTP cache owns all disk operations.
func (c *artworkCache) observeArtworkResponse(ctx context.Context, res *http.Response) {
	e, _ := ctx.Value(artworkPreviewHTTPKey{}).(*artEntry)
	if e == nil || !artworkPreviewWithdrawn(res) {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	covers, unlockCovers := c.downloadCovers.LockCovers()
	defer unlockCovers()
	if job := c.running[e]; job != nil && ctx.Err() == nil && !e.withdrawn &&
		c.entry(job.artRequest) == e && job.matches(covers) {
		c.revokeArtworkPreviews(job)
	}
}

func artworkPreviewWithdrawn(res *http.Response) bool {
	switch res.StatusCode {
	case http.StatusNotFound, http.StatusGone:
		return true
	case http.StatusOK:
		now := time.Now()
		return !artworkFreshUntil(res.Header, now).After(now)
	case http.StatusNotModified:
		control := res.Header.Values("Cache-Control")
		if artworkProhibitedResponse(res.Header) || (len(control) > 0 && !artworkPublicResponse(res.Header)) {
			return true
		}
		for directive := range strings.SplitSeq(strings.Join(control, ","), ",") {
			name, value, _ := strings.Cut(directive, "=")
			if strings.EqualFold(strings.TrimSpace(name), "no-cache") ||
				(strings.EqualFold(strings.TrimSpace(name), "max-age") && strings.Trim(strings.TrimSpace(value), "\"") == "0") {
				return true
			}
		}
	}
	return false
}

// Caller holds mu and the source index lock. Only the same effective HTTP
// URL and source generation are affected, including unsized CDNs whose
// several bitmap buckets use exactly the same HTTP representation.
func (c *artworkCache) revokeArtworkPreviews(job *artworkLoad) {
	key := job.artRequest
	for _, edge := range artworkBuckets {
		key.edge = edge
		p := c.previewPool(key)
		if e := p.entries[key]; e != nil && e.loadedAt <= job.started {
			p.removePreview(key, e)
		}
		p = c.pool(key)
		if e := p.entries[key]; e != nil && e.bitmap != nil && e.loadedAt <= job.started {
			e.freshUntil = time.Time{}
			if e.preview != nil {
				cost := artworkPixelCost(e.preview.Size())
				e.preview = nil
				e.bytes -= cost
				p.bytes -= cost
			}
		}
	}
	for _, older := range c.running {
		key.edge = older.edge
		if older.started < job.started && older.artRequest == key {
			older.entry.withdrawn = true
		}
	}
}
