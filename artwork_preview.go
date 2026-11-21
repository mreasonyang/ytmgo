package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"hash/maphash"
	"image"
	"net/http"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"golang.org/x/image/draw"
)

type artworkVersion struct {
	request uint64
	body    uint64
}

type artworkAuthority struct {
	artworkVersion
	freshUntil time.Time
}

// An older request can supply pixels for the accepted body, but only while
// the newer response authorizes reuse. Caller holds the cache lock.
func (job *artworkLoad) responseVersion(art *decoded, now time.Time) (artworkVersion, bool) {
	if job.newer.request == 0 {
		return artworkVersion{request: job.started, body: art.bodyID}, true
	}
	if job.responseWithdrawn || art.bitmap == nil || job.newer.body != art.bodyID || !job.newer.freshUntil.After(now) {
		return artworkVersion{}, false
	}
	art.freshUntil = job.newer.freshUntil
	return job.newer.artworkVersion, true
}

var artworkBodySeed = maphash.MakeSeed()

// Identify the original HTTP representation across sizes and disk reloads.
func artworkBodyID(data []byte) uint64 { return maphash.Bytes(artworkBodySeed, data) }

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

// Both staged and displayed previews borrow the space left by the three
// rendition pools. Only formal bitmap admission can reclaim from both pools.
func (c *artworkCache) artworkRoom() int {
	return artworkMemoryBudget - c.small.bytes - c.browse.bytes - c.player.bytes -
		c.smallPreviews.bytes - c.browsePreviews.bytes
}

func (c *artworkCache) makeArtworkRoom(charge int) bool {
	for charge > c.artworkRoom() {
		pool := &c.smallPreviews
		key, victim := pool.oldestPreview()
		otherKey, other := c.browsePreviews.oldestPreview()
		if other != nil && evictPreviewFirst(other, victim) {
			pool, key, victim = &c.browsePreviews, otherKey, other
		}
		if victim == nil {
			return false
		}
		pool.removePreview(key, victim)
	}
	return true
}

func evictPreviewFirst(e, other *artEntry) bool {
	return other == nil || (e.staged && !other.staged) || (e.staged == other.staged && e.used < other.used)
}

func (p *artworkPool) oldestPreview() (key artRequest, oldest *artEntry) {
	for candidate, e := range p.entries {
		if evictPreviewFirst(e, oldest) {
			key, oldest = candidate, e
		}
	}
	return
}

func (c *artworkCache) stagePreview(key artRequest, art decoded, version artworkVersion, visibleBrowse bool) {
	p := c.previewPool(key)
	if old := p.entries[key]; old != nil {
		if old.version.request > version.request {
			return
		}
		if old.version.body == version.body && !old.staged {
			old.version, old.freshUntil = version, art.freshUntil
			return
		}
		p.removePreview(key, old)
	}
	cost := artworkPixelCost(art.preview.Size())
	if cost > p.budget || cost > c.artworkRoom()+p.bytes || p.limit == 0 {
		return
	}
	for len(p.entries) >= p.limit || p.bytes+cost > p.budget || cost > c.artworkRoom() {
		victimKey, victim := p.oldestPreview()
		if victim == nil || (!victim.staged && !visibleBrowse) {
			return
		}
		p.removePreview(victimKey, victim)
	}
	c.clock++
	p.entries[key] = &artEntry{
		bitmap: art.preview, bytes: cost, local: art.local, freshUntil: art.freshUntil,
		version: version, used: c.clock, staged: true,
	}
	p.bytes += cost
}

func (c *artworkCache) rememberPreview(key artRequest, e *artEntry, now time.Time, touch bool) {
	preview := c.previewPool(key).entries[key]
	if preview == nil || !e.reusable(now) || preview.version.body != e.version.body {
		return
	}
	if preview.staged && preview.version.request != e.version.request {
		return
	}
	if preview.staged || touch {
		preview.staged = false
		c.clock++
		preview.used = c.clock
	}
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
			if e.staged {
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
		job.responseWithdrawn = true
		if job.newer.request == 0 {
			c.revokeArtworkPreviews(job)
		}
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

// A newly accepted response supplies the deadline for every rendition of its
// body at this URL. A changed body invalidates older renditions, even without
// enough room to stage a new preview. Distinct sized CDN URLs stay separate.
func (c *artworkCache) acceptArtworkBody(key artRequest, version artworkVersion, freshUntil time.Time) {
	for _, edge := range artworkBuckets {
		key.edge = edge
		p := c.previewPool(key)
		if old := p.entries[key]; old != nil && old.version.request <= version.request {
			if old.version.body != version.body {
				p.removePreview(key, old)
			} else {
				old.version, old.freshUntil = version, freshUntil
			}
		}
		if old := c.entry(key); old != nil && old.bitmap != nil && old.version.request < version.request {
			if old.version.body == version.body {
				old.version, old.freshUntil = version, freshUntil
			} else {
				old.freshUntil = time.Time{}
			}
		}
	}
	for _, older := range c.running {
		key.edge = older.edge
		if older.started < version.request && older.newer.request < version.request && older.artRequest == key {
			older.newer = artworkAuthority{version, freshUntil}
		}
	}
}

// Caller holds mu and the source index lock. Only the same effective HTTP
// URL and source generation are affected, including unsized CDNs whose
// several bitmap buckets use exactly the same HTTP representation.
func (c *artworkCache) revokeArtworkPreviews(job *artworkLoad) {
	key := job.artRequest
	for _, edge := range artworkBuckets {
		key.edge = edge
		p := c.previewPool(key)
		if e := p.entries[key]; e != nil && e.version.request <= job.started {
			p.removePreview(key, e)
		}
		if e := c.entry(key); e != nil && e.bitmap != nil && e.version.request <= job.started {
			e.freshUntil = time.Time{}
		}
	}
	for _, older := range c.running {
		key.edge = older.edge
		if older.started < job.started && older.artRequest == key {
			older.entry.withdrawn = true
		}
	}
}
