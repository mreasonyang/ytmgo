package main

import (
	"bytes"
	"fmt"
	"image/color"
	"io"
	"net/http"
	"sync/atomic"
	"testing"
	"time"
	"weak"

	"github.com/egoist/mygo/ui"
)

func TestArtworkReturnPreviewsRetainInitialScreen(t *testing.T) {
	const edge, initial, count, size = 384, 13, 240, 384 << 10
	c := artworkTestCache(t, nil)
	c.disk = &artworkDisk{dir: t.TempDir()}
	blob := artworkPreviewFixture(t, edge, size, color.RGBA{R: 83, G: 156, B: 201, A: 255})
	var gets atomic.Int64
	c.client.Transport = artworkTransport(func(*http.Request) (*http.Response, error) {
		gets.Add(1)
		return artworkHTTPResponse(blob, http.Header{"Cache-Control": {"public, max-age=3600"}}), nil
	})
	raw := func(i int) string { return fmt.Sprintf("https://i.ytimg.com/cover-probe-%d", i) }
	show := func(start, stop int, record []artworkReturnCover) {
		t.Helper()
		f := c.beginFrame()
		for i := start; i < stop; i++ {
			f.get(raw(i), edge, artPrimary)
		}
		f.submit()
		c.workers.Wait()
		f = c.beginFrame()
		for i := start; i < stop; i++ {
			picture := f.get(raw(i), edge, artPrimary)
			requireArtworkSourceWidth(t, picture, edge)
			if record != nil {
				record[i-start] = artworkReturnCover{raw(i), edge, weak.Make(picture.(*ui.Bitmap))}
			}
		}
		f.submit()
	}
	var covers [initial]artworkReturnCover
	show(0, initial, covers[:])
	c.setReturnPreviews(covers[:])
	for i := initial; i < initial+count; i += 10 {
		show(i, min(i+10, initial+count), nil)
	}
	before := gets.Load()
	f := c.beginFrame()
	for i := range initial {
		key := f.key(raw(i), edge, artPrimary)
		if c.entry(key) != nil {
			t.Fatalf("return protection retained formal cover %d", i)
		}
		preview := c.browsePreviews.entries[key]
		picture := f.get(raw(i), edge, artPrimary)
		requireArtworkSourceWidth(t, picture, artworkPreviewEdge)
		if preview == nil || !preview.returnPreview || picture != preview.bitmap {
			t.Fatalf("cover %d did not paint its retained preview", i)
		}
	}
	f.submit()
	c.workers.Wait()
	for i := range initial {
		requireArtworkSourceWidth(t, c.peek(raw(i), edge), edge)
	}
	if got := gets.Load() - before; got != 0 {
		t.Fatalf("return made %d additional origin requests", got)
	}
	assertArtworkAccounting(t, c)
	t.Logf("other_covers=%d full=0 preview=%d miss=0 additional_origin=0", count, initial)
}

func TestArtworkReturnPreviewMatchesPaintedBucketAndVersion(t *testing.T) {
	const raw = "https://lh3.googleusercontent.com/return-match=s88-k"
	c := artworkTestCache(t, artworkTestPNG(t, 384, 384))
	sourceArtworkFrame(c, raw, 128, artPrimary)
	c.workers.Wait()
	shown := sourceArtworkFrame(c, raw, 384, artPrimary)
	requireArtworkSourceWidth(t, shown, 128)
	small := artworkReturnCover{raw, 384, weak.Make(shown.(*ui.Bitmap))}
	c.workers.Wait()
	large := sourceArtworkFrame(c, raw, 384, artPrimary)
	requireArtworkSourceWidth(t, large, 384)
	largeCover := artworkReturnCover{raw, 384, weak.Make(large.(*ui.Bitmap))}
	f := c.beginFrame()
	smallKey, largeKey := f.key(raw, 128, artPrimary), f.key(raw, 384, artPrimary)
	smallPreview, largePreview := c.smallPreviews.entries[smallKey], c.browsePreviews.entries[largeKey]
	if smallPreview == nil || largePreview == nil {
		t.Fatal("missing cross-bucket preview fixtures")
	}
	clock, deadline := c.clock, smallPreview.freshUntil
	c.setReturnPreviews([]artworkReturnCover{small})
	if !smallPreview.returnPreview || largePreview.returnPreview || c.clock != clock || smallPreview.freshUntil != deadline {
		t.Fatal("cross-bucket paint did not protect only its preview with original freshness and recency")
	}
	c.setReturnPreviews([]artworkReturnCover{largeCover})
	if smallPreview.returnPreview || !largePreview.returnPreview {
		t.Fatal("new return view did not replace the old protection")
	}
	wrongRaw := largeCover
	wrongRaw.raw = "https://lh3.googleusercontent.com/different=s88-k"
	c.setReturnPreviews([]artworkReturnCover{wrongRaw, {raw: raw, edge: 384}})
	if smallPreview.returnPreview || largePreview.returnPreview {
		t.Fatal("an unrelated or missing bitmap matched a preview")
	}
	formal := c.entry(largeKey)
	formal.version.request++
	c.setReturnPreviews([]artworkReturnCover{largeCover})
	if largePreview.returnPreview {
		t.Fatal("preview from a different version was protected")
	}
	formal.version = largePreview.version
	formal.freshUntil = time.Now().Add(-time.Minute)
	c.setReturnPreviews([]artworkReturnCover{largeCover})
	if largePreview.returnPreview {
		t.Fatal("stale formal bitmap granted return protection")
	}
	artworkPreviewEvict(t, c, largeKey)
	shown = c.peek(raw, 384)
	if shown == nil || shown == largePreview.bitmap {
		t.Fatal("formal fallback fixture is missing")
	}
	artworkPreviewEvict(t, c, smallKey)
	shown = c.peek(raw, 384)
	if shown != largePreview.bitmap {
		t.Fatal("preview paint fixture is missing")
	}
	previewCover := artworkReturnCover{raw, 384, weak.Make(shown.(*ui.Bitmap))}
	c.setReturnPreviews([]artworkReturnCover{previewCover})
	if !largePreview.returnPreview || smallPreview.returnPreview {
		t.Fatal("painted preview did not grant return protection")
	}
	largePreview.freshUntil = time.Time{}
	c.setReturnPreviews([]artworkReturnCover{previewCover})
	if largePreview.returnPreview {
		t.Fatal("expired preview granted return protection")
	}
}

func TestArtworkReturnPreviewPoolPrefersCandidatesAndOrdinaryPreviews(t *testing.T) {
	const edge = 128
	c := artworkTestCache(t, artworkTestPNG(t, edge, edge))
	c.smallPreviews.limit = 3
	load := func(name string, priority artworkPriority, show bool) artRequest {
		t.Helper()
		key := artRequest{url: "https://i.ytimg.com/return-pool-" + name, edge: edge}
		sourceArtworkFrame(c, key.url, edge, priority)
		c.workers.Wait()
		if show {
			requireArtworkSourceWidth(t, sourceArtworkFrame(c, key.url, edge, artPrimary), edge)
		}
		return key
	}
	previous := load("previous", artPrimary, true)
	formal := c.entry(previous).bitmap
	c.setReturnPreviews([]artworkReturnCover{{previous.url, edge, weak.Make(formal)}})
	ordinary := load("ordinary", artPrimary, true)
	candidate := load("candidate", artPrimary, false)
	staged := c.smallPreviews.entries[candidate]
	c.setReturnPreviews([]artworkReturnCover{
		{previous.url, edge, weak.Make(formal)}, {candidate.url, edge, weak.Make(staged.bitmap)},
	})
	if !staged.staged || staged.returnPreview {
		t.Fatal("a staged candidate was protected")
	}
	prefetch := load("prefetch", artPrefetch, false)
	if c.smallPreviews.entries[candidate] != nil || c.smallPreviews.entries[prefetch] == nil {
		t.Fatal("prefetch did not replace the staged candidate")
	}
	load("visible-one", artPrimary, true)
	load("visible-two", artPrimary, true)
	if c.smallPreviews.entries[ordinary] != nil || c.smallPreviews.entries[previous] == nil {
		t.Fatal("pool did not prefer reclaiming an ordinary preview")
	}
	ignored := load("ignored-prefetch", artPrefetch, false)
	if c.smallPreviews.entries[ignored] != nil || c.smallPreviews.entries[previous] == nil {
		t.Fatal("prefetch displaced a displayed preview")
	}
	c.setReturnPreviews(nil)
	load("visible-three", artPrimary, true)
	if c.smallPreviews.entries[previous] != nil {
		t.Fatal("cleared protection still affected eviction")
	}
	assertArtworkAccounting(t, c)
}

func TestArtworkReturnPreviewFormalAdmissionAtTotalBudget(t *testing.T) {
	const raw = "https://i.ytimg.com/return-player-pressure"
	const edge = 128
	data := artworkTestPNG(t, edge, edge)
	encoded, err := io.ReadAll(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	charge := cap(encoded) + artworkPixelCost(edge, edge)
	for _, scenario := range []struct {
		name    string
		mark    bool
		expired bool
	}{
		{"ordinary", false, false},
		{"already-expired", true, true},
		{"visible-upgrade", true, false},
		{"protection-cleared", true, false},
		{"budget-freed", true, false},
		{"protection-expired", true, false},
		{"no-store-on-resume", true, false},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			c := artworkTestCache(t, nil)
			var requests atomic.Int64
			c.client.Transport = artworkTransport(func(*http.Request) (*http.Response, error) {
				count := requests.Add(1)
				policy := "public, max-age=3600"
				if scenario.name == "no-store-on-resume" && count >= 3 {
					policy = "no-store"
				}
				return artworkHTTPResponse(data, http.Header{"Cache-Control": {policy}}), nil
			})
			sourceArtworkFrame(c, raw, edge, artPrimary)
			c.workers.Wait()
			shown := sourceArtworkFrame(c, raw, edge, artPrimary).(*ui.Bitmap)
			key := artRequest{url: raw, edge: edge}
			if scenario.mark {
				c.setReturnPreviews([]artworkReturnCover{{raw, edge, weak.Make(shown)}})
				if !c.smallPreviews.entries[key].returnPreview {
					t.Fatal("missing protected preview")
				}
			}
			artworkPreviewEvict(t, c, key)
			if scenario.expired {
				c.smallPreviews.entries[key].freshUntil = time.Time{}
			}
			// Represent settled charges at the production limits. The incoming
			// rendition will fit if it can reclaim this displayed preview.
			for _, fixture := range []struct {
				pool  *artworkPool
				key   artRequest
				bytes int
			}{
				{&c.small, artRequest{url: "small", edge: 128}, artworkSmallBudget},
				{&c.browse, artRequest{url: "browse", edge: 384}, artworkBrowseBudget},
				{&c.player, artRequest{url: "player", edge: 768}, artworkPlayerBudget - charge},
			} {
				fixture.pool.entries[fixture.key] = &artEntry{local: true, bytes: fixture.bytes}
				fixture.pool.bytes = fixture.bytes
			}
			const next = "https://i.ytimg.com/return-pressure-prefetch"
			nextKey := artRequest{url: next, edge: 768}
			pending := c.beginFrame()
			pending.get(next, 768, artPrefetch)
			sourceArtworkFrame(c, next, 768, artPrefetch)
			c.workers.Wait()
			entry := c.player.entries[nextKey]
			if entry == nil {
				t.Fatal("prefetch did not create a player-pool entry")
			}
			if scenario.mark && !scenario.expired {
				if c.smallPreviews.entries[key] == nil || entry.bitmap != nil || entry.failures != 0 ||
					!entry.retryAt.IsZero() || entry.prefetchDeferredBytes != charge {
					t.Fatal("deferral reclaimed a protected preview or counted as a load failure")
				}
				pending.submit()
				for range 3 {
					sourceArtworkFrame(c, next, 768, artPrefetch)
					c.workers.Wait()
				}
				if got := requests.Load(); got != 2 {
					t.Fatalf("deferred prefetch ran again: %d requests", got)
				}
				priority := artPrefetch
				switch scenario.name {
				case "visible-upgrade":
					priority = artPlayer
				case "protection-cleared", "no-store-on-resume":
					c.setReturnPreviews(nil)
				case "budget-freed":
					if !c.browse.evictOldest(c.visible) {
						t.Fatal("could not free formal cache space")
					}
				case "protection-expired":
					c.smallPreviews.entries[key].freshUntil = time.Now().Add(-time.Second)
				}
				sourceArtworkFrame(c, next, 768, priority)
				c.workers.Wait()
				if got := requests.Load(); got != 3 {
					t.Fatalf("expected the first eligible frame to retry: %d requests", got)
				}
				if scenario.name == "no-store-on-resume" {
					for range 3 {
						sourceArtworkFrame(c, next, 768, artPrefetch)
						c.workers.Wait()
					}
					if requests.Load() != 3 || entry.bitmap != nil || !entry.prefetchUncacheable || entry.prefetchDeferredBytes != 0 {
						t.Fatal("no-store result was reused or repeatedly prefetched")
					}
					assertArtworkAccounting(t, c)
					return
				}
			}
			if entry.bitmap == nil || entry.prefetchDeferredBytes != 0 {
				t.Fatal("formal rendition was not admitted")
			}
			preview := c.smallPreviews.entries[key]
			if scenario.name == "budget-freed" {
				if preview == nil || !preview.returnPreview {
					t.Fatal("prefetch reclaimed protection despite having room")
				}
			} else if preview != nil || c.artworkRoom() != 0 {
				t.Fatal("formal admission did not reclaim the preview at the total budget")
			}
			assertArtworkAccounting(t, c)
		})
	}
}

func TestArtworkReturnPreviewWithdrawalAndReplacement(t *testing.T) {
	const raw = "https://i.ytimg.com/return-revoked"
	const edge = 384
	key := artRequest{url: raw, edge: edge}
	first := artworkTestPNG(t, edge, edge)
	replacement := artworkPreviewFixture(t, edge, 0, color.RGBA{R: 30, G: 150, A: 255})
	for _, name := range []string{"withdrawn", "replaced"} {
		t.Run(name, func(t *testing.T) {
			c := artworkTestCache(t, nil)
			var gets atomic.Int64
			c.client.Transport = artworkTransport(func(*http.Request) (*http.Response, error) {
				body, status := first, http.StatusOK
				if gets.Add(1) > 1 {
					if name == "withdrawn" {
						status = http.StatusGone
					} else {
						body = replacement
					}
				}
				res := artworkHTTPResponse(body, http.Header{"Cache-Control": {"public, max-age=3600"}})
				res.StatusCode = status
				return res, nil
			})
			sourceArtworkFrame(c, raw, edge, artPrimary)
			c.workers.Wait()
			sourceArtworkFrame(c, raw, edge, artPrimary)
			artworkPreviewEvict(t, c, key)
			shown := c.peek(raw, edge).(*ui.Bitmap)
			cover := artworkReturnCover{raw, edge, weak.Make(shown)}
			c.setReturnPreviews([]artworkReturnCover{cover})
			old := c.browsePreviews.entries[key]
			if !old.returnPreview || sourceArtworkFrame(c, raw, edge, artPrimary) != shown {
				t.Fatal("could not display the protected preview before reloading")
			}
			c.workers.Wait()
			next := c.browsePreviews.entries[key]
			if name == "withdrawn" {
				if next != nil || c.peek(raw, edge) != nil {
					t.Fatal("HTTP withdrawal left a protected preview available")
				}
			} else {
				if next == nil || next == old || next.returnPreview {
					t.Fatal("replacement inherited the old preview's protection")
				}
				sourceArtworkFrame(c, raw, edge, artPrimary)
				c.setReturnPreviews([]artworkReturnCover{cover})
				if next.returnPreview {
					t.Fatal("old bitmap pointer matched the replacement")
				}
			}
			if got := gets.Load(); got != 2 {
				t.Fatalf("got %d origin requests, want the cold load and one reload", got)
			}
			assertArtworkAccounting(t, c)
		})
	}
}

func TestArtworkReturnPreviewProtectionEndsWithSourceGeneration(t *testing.T) {
	const raw, edge = artworkSourceURL, 128
	data := artworkTestPNG(t, edge, edge)
	m, _ := artworkSourceLibrary(t, map[string][]byte{artworkSourceID: data})
	completeArtworkSource(t, m, artworkSourceID, raw)
	c := artworkTestCache(t, data)
	c.downloadCovers = m
	sourceArtworkFrame(c, raw, edge, artPrimary)
	c.workers.Wait()
	shown := sourceArtworkFrame(c, raw, edge, artPrimary).(*ui.Bitmap)
	cover := artworkReturnCover{raw, edge, weak.Make(shown)}
	f := c.beginFrame()
	oldKey := f.key(raw, edge, artPrimary)
	old := c.smallPreviews.entries[oldKey]
	c.setReturnPreviews([]artworkReturnCover{cover})
	if old == nil || !old.returnPreview {
		t.Fatal("downloaded cover did not get a return preview")
	}
	if err := m.Remove(artworkSourceID); err != nil {
		t.Fatal(err)
	}
	c.setReturnPreviews([]artworkReturnCover{cover})
	if old.returnPreview || c.peek(raw, edge) != nil {
		t.Fatal("withdrawn source remained eligible for return protection")
	}
	sourceArtworkFrame(c, raw, edge, artPrimary)
	c.workers.Wait()
	sourceArtworkFrame(c, raw, edge, artPrimary)
	f = c.beginFrame()
	next := c.smallPreviews.entries[f.key(raw, edge, artPrimary)]
	if c.smallPreviews.entries[oldKey] != nil || next == nil || next.returnPreview {
		t.Fatal("the new source generation inherited a previous preview or preference")
	}
	assertArtworkAccounting(t, c)
}

func TestArtworkReturnPreviewProtectionIsBounded(t *testing.T) {
	const edge = 128
	c := artworkTestCache(t, artworkTestPNG(t, edge, edge))
	items := make([]artworkReturnCover, artworkReturnCoverLimit+1)
	for i := range items {
		raw := fmt.Sprintf("https://i.ytimg.com/return-limit-%d", i)
		sourceArtworkFrame(c, raw, edge, artPrimary)
		c.workers.Wait()
		shown := sourceArtworkFrame(c, raw, edge, artPrimary).(*ui.Bitmap)
		items[i] = artworkReturnCover{raw, edge, weak.Make(shown)}
	}
	c.setReturnPreviews(items)
	protected := 0
	for _, e := range c.smallPreviews.entries {
		if e.returnPreview {
			protected++
		}
	}
	if protected != artworkReturnCoverLimit || len(c.smallPreviews.entries) != len(items) {
		t.Fatalf("protected %d of %d existing previews", protected, len(c.smallPreviews.entries))
	}
}
