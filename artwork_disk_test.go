package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func artworkHTTPResponse(data []byte, headers http.Header) *http.Response {
	return &http.Response{StatusCode: 200, Proto: "HTTP/1.1", ProtoMajor: 1, ProtoMinor: 1, Header: headers, Body: io.NopCloser(bytes.NewReader(data)), ContentLength: int64(len(data))}
}

// Use a fresh loader to test persisted HTTP metadata, not the decoded pool.
func loadHTTPArtwork(t *testing.T, dir, raw string, upstream artworkTransport) *artworkCache {
	t.Helper()
	c := artworkTestCache(t, nil)
	c.disk = &artworkDisk{dir: dir}
	c.client.Transport = upstream
	requestTestArtwork(c, raw, 384)
	c.workers.Wait()
	if e := c.entry(artRequest{artworkRequestAt(raw, 384), 384}); e == nil || e.bitmap == nil {
		t.Fatal("HTTP cache did not produce a decoded image")
	}
	return c
}

func TestArtworkHTTPValidatorsAnd304(t *testing.T) {
	for _, validator := range []string{"etag", "last-modified"} {
		t.Run(validator, func(t *testing.T) {
			dir := t.TempDir()
			data := artworkTestPNG(t, 384, 192)
			const raw = "https://i.ytimg.com/vi/synthetic/hqdefault.jpg"
			requests, bodies := 0, 0
			modified := time.Now().Add(-time.Hour).UTC().Format(http.TimeFormat)
			upstream := artworkTransport(func(req *http.Request) (*http.Response, error) {
				requests++
				h := http.Header{"Cache-Control": {"public, max-age=0"}, "Date": {time.Now().UTC().Format(http.TimeFormat)}, "Age": {"10"}, "Last-Modified": {modified}}
				if validator == "etag" {
					h.Set("ETag", `"v1"`)
				}
				if requests == 1 {
					bodies++
					return artworkHTTPResponse(data, h), nil
				}
				if validator == "etag" && req.Header.Get("If-None-Match") != `"v1"` {
					t.Error("ETag was not sent as If-None-Match")
				}
				if req.Header.Get("If-Modified-Since") != modified {
					t.Error("Last-Modified was not sent as If-Modified-Since")
				}
				h.Set("Cache-Control", "public, max-age=3600")
				return &http.Response{StatusCode: 304, Header: h, Body: http.NoBody}, nil
			})
			loadHTTPArtwork(t, dir, raw, upstream)
			loadHTTPArtwork(t, dir, raw, upstream)
			loadHTTPArtwork(t, dir, raw, upstream)
			if requests != 2 || bodies != 1 {
				t.Fatalf("304/fresh cache did not reuse the body: requests=%d bodies=%d", requests, bodies)
			}
		})
	}
}

func TestArtworkHTTPChangedETagReplacesImage(t *testing.T) {
	dir := t.TempDir()
	requests := 0
	const raw = "https://i.ytimg.com/image"
	upstream := artworkTransport(func(req *http.Request) (*http.Response, error) {
		requests++
		if requests == 2 && req.Header.Get("If-None-Match") != `"v1"` {
			t.Error("old validator missing")
		}
		h := http.Header{"Cache-Control": {"public, max-age=0"}, "Etag": {fmt.Sprintf(`"v%d"`, requests)}}
		return artworkHTTPResponse(artworkTestPNG(t, requests*32, 32), h), nil
	})
	loadHTTPArtwork(t, dir, raw, upstream)
	c := loadHTTPArtwork(t, dir, raw, upstream)
	if w, _ := c.entry(artRequest{raw, 384}).bitmap.Size(); w != 64 {
		t.Fatal("changed ETag kept the old image")
	}
}

func TestArtworkHTTPNoCacheAlsoRevalidatesResidentPixels(t *testing.T) {
	dir := t.TempDir()
	data := artworkTestPNG(t, 384, 384)
	requests := 0
	const raw = "https://i.ytimg.com/image"
	upstream := artworkTransport(func(req *http.Request) (*http.Response, error) {
		requests++
		h := http.Header{"Cache-Control": {"public, no-cache, max-age=3600"}, "Etag": {`"v1"`}}
		if requests == 1 {
			return artworkHTTPResponse(data, h), nil
		}
		if req.Header.Get("If-None-Match") != `"v1"` {
			t.Error("no-cache reuse did not validate")
		}
		return &http.Response{StatusCode: 304, Header: h, Body: http.NoBody}, nil
	})
	c := loadHTTPArtwork(t, dir, raw, upstream)
	for range 5 {
		requestTestArtwork(c, raw, 384)
	}
	if requests != 1 {
		t.Fatal("repainting the displayed image made HTTP requests")
	}
	empty := artworkFrame{cache: c}
	empty.submit()
	if got := requestTestArtwork(c, raw, 384); got != nil {
		t.Fatal("resident pixels bypassed no-cache validation after leaving the viewport")
	}
	c.workers.Wait()
	if requests != 2 || c.entry(artRequest{raw, 384}).bitmap == nil {
		t.Fatal("resident image was not revalidated")
	}
}

func TestArtworkDiskSkipsTransientAndUncacheableResponses(t *testing.T) {
	for _, tc := range []struct {
		name, raw, control     string
		avatar, cookie, broken bool
	}{
		{"avatar", "https://lh3.googleusercontent.com/photo", "public, max-age=86400", true, false, false},
		{"query", "https://i.ytimg.com/image?token=synthetic", "public, max-age=86400", false, false, false},
		{"no-store", "https://i.ytimg.com/image", "public, no-store", false, false, false},
		{"private", "https://i.ytimg.com/image", "private, max-age=3600", false, false, false},
		{"cookie", "https://i.ytimg.com/image", "public, max-age=3600", false, true, false},
		{"invalid", "https://i.ytimg.com/image", "public, max-age=3600", false, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := artworkTestCache(t, nil)
			dir := t.TempDir()
			c.disk = &artworkDisk{dir: dir}
			c.client.Transport = artworkTransport(func(*http.Request) (*http.Response, error) {
				data := artworkTestPNG(t, 16, 16)
				if tc.broken {
					data = nil
				}
				h := http.Header{"Cache-Control": {tc.control}}
				if tc.cookie {
					h.Set("Set-Cookie", "synthetic=value")
				}
				return artworkHTTPResponse(data, h), nil
			})
			priority := artPrimary
			if tc.avatar {
				priority = artAvatar
			}
			frame := artworkFrame{cache: c}
			frame.get(tc.raw, 128, priority)
			frame.submit()
			c.workers.Wait()
			files, err := os.ReadDir(dir)
			if err != nil || len(files) != 0 {
				t.Fatal("non-reusable artwork persisted")
			}
		})
	}
}

func TestArtworkHTTPCorruptStoreRefetches(t *testing.T) {
	dir := t.TempDir()
	data := artworkTestPNG(t, 32, 32)
	requests := 0
	const raw = "https://i.ytimg.com/image"
	upstream := artworkTransport(func(*http.Request) (*http.Response, error) {
		requests++
		return artworkHTTPResponse(data, http.Header{"Cache-Control": {"public, max-age=3600"}}), nil
	})
	loadHTTPArtwork(t, dir, raw, upstream)
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if e.Name() == ".lock" {
			continue
		}
		if err := os.WriteFile(filepath.Join(dir, e.Name()), []byte("corrupt fixture"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	loadHTTPArtwork(t, dir, raw, upstream)
	loadHTTPArtwork(t, dir, raw, upstream)
	if requests != 2 {
		t.Fatal("corrupt HTTP cache did not recover with one full fetch")
	}
}

func TestArtworkHTTPVarySeparatesRepresentations(t *testing.T) {
	c := artworkTestCache(t, nil)
	c.disk = &artworkDisk{dir: t.TempDir()}
	requests := 0
	c.client.Transport = artworkTransport(func(req *http.Request) (*http.Response, error) {
		requests++
		edge := 32
		if req.Header.Get("Accept-Language") == "ja" {
			edge = 64
		}
		return artworkHTTPResponse(artworkTestPNG(t, edge, 32), http.Header{"Cache-Control": {"public, max-age=3600"}, "Vary": {"Accept-Language"}}), nil
	})
	const raw = "https://i.ytimg.com/image"
	client := c.artworkHTTPClient(artDemand{artRequest: artRequest{raw, 384}})
	for _, language := range []string{"en", "ja", "en", "ja"} {
		req, _ := http.NewRequestWithContext(t.Context(), http.MethodGet, raw, nil)
		req.Header.Set("Accept-Language", language)
		res, err := client.Do(req)
		if err != nil {
			t.Fatal("Vary request failed")
		}
		data, _ := io.ReadAll(res.Body)
		res.Body.Close()
		art := c.decode(t.Context(), data, 384)
		expected := 32
		if language == "ja" {
			expected = 64
		}
		if art.bitmap == nil {
			t.Fatal("Vary image missing")
		}
		if w, _ := art.bitmap.Size(); w != expected {
			t.Fatal("Vary served another representation")
		}
	}
	if requests != 2 {
		t.Fatalf("Vary cache did not reuse each representation: %d requests", requests)
	}
}

func TestArtworkHTTPAgeExpiresAndMaxAgePrecedence(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	for _, tc := range []struct {
		name, control, age string
		expires            time.Time
		requests           int
	}{
		{"upstream-age", "public, max-age=3600", "7200", time.Time{}, 2},
		{"expired", "public", "0", now.Add(-time.Hour), 2},
		{"expires-fresh", "public", "0", now.Add(time.Hour), 1},
		{"max-age-precedes-expires", "public, max-age=3600", "0", now.Add(-time.Hour), 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			data := artworkTestPNG(t, 32, 32)
			requests := 0
			upstream := artworkTransport(func(req *http.Request) (*http.Response, error) {
				requests++
				h := http.Header{"Cache-Control": {tc.control}, "Date": {now.Format(http.TimeFormat)}, "Age": {tc.age}, "Etag": {`"v1"`}}
				if !tc.expires.IsZero() {
					h.Set("Expires", tc.expires.Format(http.TimeFormat))
				}
				if requests == 1 {
					return artworkHTTPResponse(data, h), nil
				}
				if req.Header.Get("If-None-Match") != `"v1"` {
					t.Error("stale HTTP response was not conditionally validated")
				}
				return &http.Response{StatusCode: 304, Header: http.Header{"Cache-Control": {"public, max-age=3600"}, "Date": {now.Format(http.TimeFormat)}, "Age": {"0"}}, Body: http.NoBody}, nil
			})
			for range 2 {
				loadHTTPArtwork(t, dir, "https://i.ytimg.com/image", upstream)
			}
			if requests != tc.requests {
				t.Fatalf("HTTP freshness policy mismatch: requests=%d", requests)
			}
		})
	}
}

func TestArtworkHTTPNoStoreLeavesNoReusablePixels(t *testing.T) {
	c := artworkTestCache(t, nil)
	c.disk = &artworkDisk{dir: t.TempDir()}
	requests := 0
	const raw = "https://i.ytimg.com/image"
	c.client.Transport = artworkTransport(func(*http.Request) (*http.Response, error) {
		requests++
		return artworkHTTPResponse(artworkTestPNG(t, 128, 128), http.Header{"Cache-Control": {"public, no-store"}}), nil
	})
	requestTestArtwork(c, raw, 128)
	c.workers.Wait()
	if requestTestArtwork(c, raw, 128) == nil {
		t.Fatal("currently displayed image was discarded")
	}
	empty := artworkFrame{cache: c}
	empty.submit()
	if e := c.entry(artRequest{raw, 128}); e != nil && e.bitmap != nil {
		t.Fatal("no-store pixels survived after leaving the viewport")
	}
	if requestTestArtwork(c, raw, 128) != nil {
		t.Fatal("no-store image was reused")
	}
	c.workers.Wait()
	if requests != 2 {
		t.Fatal("no-store request was served from a cache")
	}
}

func setArtworkRecord(d *artworkDisk, raw, variant string, body []byte) error {
	id := raw + "#" + variant
	if err := d.Set(id, body); err != nil {
		return err
	}
	refs, _ := json.Marshal([]map[string]string{{"id": id}})
	return d.Set(raw, refs)
}

func artworkDiskInventory(t *testing.T, dir string) (int, int64, int) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	unit, err := artworkAllocationUnit(dir)
	if err != nil || unit <= 0 {
		t.Fatal("disk inventory allocation unit unavailable")
	}
	groups := map[string]bool{}
	var total int64
	temps := 0
	for _, e := range entries {
		info, err := e.Info()
		if err != nil {
			t.Fatal(err)
		}
		if e.Name() == ".lock" {
			if info.Size() != 0 {
				t.Fatal("lock file contains data")
			}
			continue
		}
		if !info.Mode().IsRegular() {
			t.Fatal("unexpected nonregular cache fixture")
		}
		if strings.HasPrefix(e.Name(), ".write-") {
			temps++
		}
		total += max((info.Size()+unit-1)/unit*unit, artworkFileCharge(info))
		if g, ok := artworkRecordName(e.Name()); ok {
			groups[g] = true
		}
	}
	return len(groups), total, temps
}

func TestArtworkDiskRetainsMoreThan512URLsBelowByteQuota(t *testing.T) {
	if artworkDiskBudget != 512<<20 {
		t.Fatal("default artwork disk budget is not 512 MiB")
	}
	dir := t.TempDir()
	d := &artworkDisk{dir: dir, freeSpace: func(string) (uint64, error) { return 1 << 40, nil }}
	const count = 600
	raw := func(i int) string { return fmt.Sprintf("https://i.ytimg.com/byte-retention-%04d", i) }
	body := func(i int) []byte { return []byte(fmt.Sprintf("complete-body-%04d", i)) }
	started := time.Now()
	for i := range count {
		if setArtworkRecord(d, raw(i), "1", body(i)) != nil {
			t.Fatalf("record admission failed at %d", i)
		}
	}
	elapsed := time.Since(started)
	groups, total, temps := artworkDiskInventory(t, dir)
	if groups != count || total >= artworkDiskBudget || temps != 0 {
		t.Fatalf("byte-bounded retention lost URLs: groups=%d bytes=%d temporary=%d", groups, total, temps)
	}
	for _, i := range []int{0, count - 1} {
		got, err := d.Get(raw(i) + "#1")
		if err != nil || !bytes.Equal(got, body(i)) {
			t.Fatalf("below-quota record %d was evicted or changed", i)
		}
		index, err := d.Get(raw(i))
		var refs []json.RawMessage
		if err != nil || json.Unmarshal(index, &refs) != nil || len(refs) != 1 {
			t.Fatal("retained URL lost its representation index")
		}
	}
	t.Logf("retained_urls=%d charged_bytes=%d batch_write_ms=%.3f first_and_last_readable=true", groups, total, float64(elapsed.Microseconds())/1000)
}

// Write a valid header/index and a sparse body. The real quota charges logical
// size as well as allocated blocks, without physically writing 512 MiB.
func seedArtworkSparseRecord(t *testing.T, dir, raw string, size int64, used time.Time) {
	t.Helper()
	row, err := json.Marshal(map[string]string{"id": raw + "#1"})
	if err != nil || size <= 4+int64(len(row)) {
		t.Fatal("sparse reference encoding failed")
	}
	path := filepath.Join(dir, artworkDiskKey(raw)+".1")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal("sparse record create failed")
	}
	var header [4]byte
	binary.BigEndian.PutUint32(header[:], uint32(len(row)))
	_, writeErr := io.Copy(f, io.MultiReader(bytes.NewReader(header[:]), bytes.NewReader(row), strings.NewReader("sparse-body")))
	truncateErr, closeErr := f.Truncate(size), f.Close()
	if writeErr != nil || truncateErr != nil || closeErr != nil || os.Chtimes(path, used, used) != nil {
		t.Fatal("sparse record write or timestamp failed")
	}
}

func TestArtworkDisk512MiBQuotaUsesLRUAndLowWater(t *testing.T) {
	for _, replace := range []bool{false, true} {
		name := "new-url"
		if replace {
			name = "atomic-replacement"
		}
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			d := &artworkDisk{dir: dir, freeSpace: func(string) (uint64, error) { return 1 << 40, nil }}
			lock, err := d.lock(true)
			if err != nil {
				t.Fatal("sparse fixture lock initialization failed")
			}
			lock.Close()
			const size = int64(128 << 10)
			const count = (512 << 20) / size
			unit, err := artworkAllocationUnit(dir)
			if err != nil || unit <= 0 || size%unit != 0 {
				t.Fatal("sparse fixture allocation geometry unavailable")
			}
			raw := func(i int) string { return fmt.Sprintf("https://i.ytimg.com/sparse-quota-%04d", i) }
			old := time.Now().Add(-24 * time.Hour)
			for i := range int(count) {
				seedArtworkSparseRecord(t, dir, raw(i), size, old.Add(time.Duration(i)*time.Second))
			}
			groups, before, temps := artworkDiskInventory(t, dir)
			if int64(groups) != count || before != 512<<20 || temps != 0 {
				t.Fatalf("fixture did not fill real quota: groups=%d bytes=%d", groups, before)
			}
			readStarted := time.Now()
			index, err := d.Get(raw(0))
			var refs []artworkRef
			if err != nil || json.Unmarshal(index, &refs) != nil || len(refs) != 1 || refs[0].ID != raw(0)+"#1" {
				t.Fatal("sparse first index unreadable")
			}
			got, err := d.Get(refs[0].ID)
			readElapsed := time.Since(readStarted)
			if err != nil || !bytes.HasPrefix(got, []byte("sparse-body")) {
				t.Fatal("sparse first body unreadable")
			}
			// Reading makes the first-created URL recent; group 1 is now LRU.
			incoming := "https://i.ytimg.com/sparse-new"
			if replace {
				incoming = raw(0)
			}
			body := []byte("new complete replacement body")
			writeStarted := time.Now()
			if setArtworkRecord(d, incoming, "1", body) != nil {
				t.Fatal("cross-quota atomic admission failed")
			}
			writeElapsed := time.Since(writeStarted)
			groups, after, temps := artworkDiskInventory(t, dir)
			if after > artworkDiskBudget*4/5 || temps != 0 {
				t.Fatalf("quota crossing missed low-water: bytes=%d temporary=%d", after, temps)
			}
			wantRemoved := int((before + unit - artworkDiskBudget*4/5 + size - 1) / size)
			wantGroups := int(count) - wantRemoved
			if !replace {
				wantGroups++
			}
			if groups != wantGroups {
				t.Fatalf("LRU removed wrong number of URL groups: got=%d want=%d", groups, wantGroups)
			}
			for i := 1; i <= wantRemoved; i++ {
				if _, err := os.Stat(filepath.Join(dir, artworkDiskKey(raw(i))+".1")); !os.IsNotExist(err) {
					t.Fatalf("cold URL %d survived older than retained URLs", i)
				}
			}
			for _, i := range []int{0, wantRemoved + 1, int(count) - 1} {
				data, err := d.Get(raw(i) + "#1")
				if err != nil {
					t.Fatalf("recent or next-oldest URL %d was evicted", i)
				}
				if i == 0 && replace {
					if !bytes.Equal(data, body) {
						t.Fatal("atomic replacement retained old body")
					}
				} else if !bytes.HasPrefix(data, []byte("sparse-body")) {
					t.Fatal("retained sparse body changed")
				}
			}
			data, err := d.Get(incoming + "#1")
			if err != nil || !bytes.Equal(data, body) {
				t.Fatal("newest complete body missing")
			}
			belowStarted := time.Now()
			if setArtworkRecord(d, "https://i.ytimg.com/below-budget-scan", "1", body) != nil {
				t.Fatal("below-budget insert failed in populated cache")
			}
			belowElapsed := time.Since(belowStarted)
			t.Logf("mode=%s start_urls=%d start_bytes=%d retained_urls=%d retained_bytes=%d removed_urls=%d index_body_read_ms=%.3f quota_write_ms=%.3f below_budget_write_ms=%.3f", name, count, before, groups, after, wantRemoved, float64(readElapsed.Microseconds())/1000, float64(writeElapsed.Microseconds())/1000, float64(belowElapsed.Microseconds())/1000)
		})
	}
}

func TestArtworkDiskReserveIncludesPendingWrite(t *testing.T) {
	for _, mode := range []string{"reserve-only", "one-byte-short", "exact-headroom"} {
		t.Run(mode, func(t *testing.T) {
			dir := t.TempDir()
			const raw = "https://i.ytimg.com/exact-free-space"
			body := []byte("complete reserved response")
			row, _ := json.Marshal(map[string]string{"id": raw + "#1"})
			unit, err := artworkAllocationUnit(dir)
			if err != nil || unit <= 0 {
				t.Fatal("allocation unit unavailable")
			}
			need := (int64(4+len(row)+len(body)) + unit - 1) / unit * unit
			need = max(need, artworkWriteCharge(int64(4+len(row)+len(body))))
			free := uint64(artworkDiskReserve) + uint64(need)
			if mode == "reserve-only" {
				free = artworkDiskReserve
			} else if mode == "one-byte-short" {
				free--
			}
			d := &artworkDisk{dir: dir, freeSpace: func(string) (uint64, error) { return free, nil }}
			err = setArtworkRecord(d, raw, "1", body)
			groups, total, temps := artworkDiskInventory(t, dir)
			if mode == "exact-headroom" {
				if err != nil || groups != 1 || total != need {
					t.Fatal("exact reserve plus write headroom was rejected")
				}
			} else if err == nil || groups != 0 || total != 0 {
				t.Fatal("write consumed the protected free-space reserve")
			}
			if temps != 0 || d.pendingBytes != 0 {
				t.Fatal("admission retained temporary or pending data")
			}
		})
	}
}

func TestArtworkDiskConcurrentWritersMergeVaryAndKeepAtomicBodies(t *testing.T) {
	dir := t.TempDir()
	const raw = "https://i.ytimg.com/concurrent-vary"
	const writers = 24
	var group sync.WaitGroup
	var failures atomic.Int32
	for i := range writers {
		group.Go(func() {
			d := &artworkDisk{dir: dir}
			body := bytes.Repeat([]byte{byte(i + 1)}, 8192+i)
			if setArtworkRecord(d, raw, fmt.Sprint(i+1), body) != nil {
				failures.Add(1)
			}
		})
	}
	group.Wait()
	if failures.Load() != 0 {
		t.Fatal("concurrent writer failed")
	}
	d := &artworkDisk{dir: dir}
	index, err := d.Get(raw)
	if err != nil {
		t.Fatal("merged index missing")
	}
	var refs []map[string]any
	if json.Unmarshal(index, &refs) != nil || len(refs) != writers {
		t.Fatalf("lost concurrent Vary representations: %d", len(refs))
	}
	for i := range writers {
		got, err := d.Get(fmt.Sprintf("%s#%d", raw, i+1))
		if err != nil || !bytes.Equal(got, bytes.Repeat([]byte{byte(i + 1)}, 8192+i)) {
			t.Fatal("writer published a partial or different body")
		}
	}
	groups, total, temps := artworkDiskInventory(t, dir)
	if groups != 1 || temps != 0 || total > artworkDiskBudget {
		t.Fatal("variants counted as independent URL groups or left temporary files")
	}
	if d.Delete(raw) != nil {
		t.Fatal("URL group delete failed")
	}
	if _, err = d.Get(raw); err == nil {
		t.Fatal("URL index survived group delete")
	}
}

func TestArtworkDiskLowVolumeSpaceStopsAdmissionAndRechecks(t *testing.T) {
	for _, mode := range []string{"unreclaimed", "reclaimed", "unavailable"} {
		t.Run(mode, func(t *testing.T) {
			dir := t.TempDir()
			d := &artworkDisk{dir: dir}
			const first = "https://i.ytimg.com/low-space-first"
			if setArtworkRecord(d, first, "1", []byte("first-body")) != nil {
				t.Fatal("initial fixture failed")
			}
			calls := 0
			d.freeSpace = func(string) (uint64, error) {
				calls++
				if mode == "unavailable" {
					return 0, errors.New("synthetic volume failure")
				}
				if mode == "reclaimed" && calls > 1 {
					return artworkDiskReserve + 1<<20, nil
				}
				return artworkDiskReserve - 1, nil
			}
			const next = "https://i.ytimg.com/low-space-next"
			err := setArtworkRecord(d, next, "1", []byte("next-body"))
			if mode == "reclaimed" {
				if err != nil || calls < 2 {
					t.Fatal("freed space was not rechecked before admission")
				}
			} else if err == nil {
				t.Fatal("low or unavailable free space admitted a response")
			}
			_, total, temps := artworkDiskInventory(t, dir)
			if total > artworkDiskBudget || temps != 0 || d.pendingBytes != 0 {
				t.Fatal("failed admission retained temporary or pending body bytes")
			}
		})
	}
}

func TestArtworkDiskReclaimsAbandonedTemporaryAndLegacyFiles(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{".write-abandoned", fmt.Sprintf("%064x", 1)} {
		if os.WriteFile(filepath.Join(dir, name), []byte("abandoned fixture"), 0600) != nil {
			t.Fatal("fixture write failed")
		}
	}
	d := &artworkDisk{dir: dir}
	if setArtworkRecord(d, "https://i.ytimg.com/cleanup", "1", []byte("complete body")) != nil {
		t.Fatal("cleanup prevented admission")
	}
	groups, _, temps := artworkDiskInventory(t, dir)
	if groups != 1 || temps != 0 {
		t.Fatal("abandoned temporary file survived admission")
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 2 {
		t.Fatal("legacy opaque file survived migration cleanup")
	}
}

func TestArtworkDiskWriterProcess(t *testing.T) {
	dir, variant := os.Getenv("YTMGO_ARTWORK_DISK_FIXTURE"), os.Getenv("YTMGO_ARTWORK_DISK_VARIANT")
	if dir == "" || variant == "" {
		t.Skip("isolated subprocess fixture")
	}
	d := &artworkDisk{dir: dir}
	if setArtworkRecord(d, "https://i.ytimg.com/process-vary", variant, bytes.Repeat([]byte(variant), 8192)) != nil {
		t.Fatal("fixture commit failed")
	}
}

func TestArtworkDiskMultiProcessWriters(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal("test executable unavailable")
	}
	dir := t.TempDir()
	const count = 8
	children := make([]*exec.Cmd, count)
	for i := range count {
		child := exec.CommandContext(t.Context(), executable, "-test.run=^TestArtworkDiskWriterProcess$")
		// No inherited credentials, auth stores, process inspection or raw logs.
		child.Env = []string{"YTMGO_ARTWORK_DISK_FIXTURE=" + dir, "YTMGO_ARTWORK_DISK_VARIANT=" + fmt.Sprint(i+1)}
		child.Stdout, child.Stderr = io.Discard, io.Discard
		if child.Start() != nil {
			t.Fatal("fixture child failed to start")
		}
		children[i] = child
	}
	for _, child := range children {
		if child.Wait() != nil {
			t.Fatal("fixture child failed")
		}
	}
	d := &artworkDisk{dir: dir}
	index, err := d.Get("https://i.ytimg.com/process-vary")
	var refs []json.RawMessage
	if err != nil || json.Unmarshal(index, &refs) != nil || len(refs) != count {
		t.Fatal("process writers lost a Vary representation")
	}
	for i := range count {
		variant := fmt.Sprint(i + 1)
		body, err := d.Get("https://i.ytimg.com/process-vary#" + variant)
		if err != nil || !bytes.Equal(body, bytes.Repeat([]byte(variant), 8192)) {
			t.Fatal("process writer published partial body")
		}
	}
	groups, _, temps := artworkDiskInventory(t, dir)
	if groups != 1 || temps != 0 {
		t.Fatal("process writes left temporary or duplicate groups")
	}
}

type canceledArtworkBody struct {
	ctx     context.Context
	started chan struct{}
	once    sync.Once
}

func (b *canceledArtworkBody) Read([]byte) (int, error) {
	b.once.Do(func() { close(b.started) })
	<-b.ctx.Done()
	return 0, b.ctx.Err()
}
func (*canceledArtworkBody) Close() error { return nil }

func TestArtworkHTTPCanceledBodyCannotPoisonCache(t *testing.T) {
	c := artworkTestCache(t, nil)
	dir := t.TempDir()
	c.disk = &artworkDisk{dir: dir}
	started := make(chan struct{})
	data := artworkTestPNG(t, 384, 384)
	var attempts atomic.Int32
	c.client.Transport = artworkTransport(func(req *http.Request) (*http.Response, error) {
		res := artworkHTTPResponse(data, http.Header{"Cache-Control": {"public, max-age=3600"}})
		if attempts.Add(1) == 1 {
			res.Body = &canceledArtworkBody{ctx: req.Context(), started: started}
		}
		return res, nil
	})
	const raw = "https://i.ytimg.com/canceled-body"
	requestTestArtwork(c, raw, 384)
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("response body did not begin")
	}
	f := artworkFrame{cache: c}
	f.submit()
	c.workers.Wait()
	groups, _, temps := artworkDiskInventory(t, dir)
	if groups != 0 || temps != 0 {
		t.Fatal("canceled partial body persisted")
	}
	requestTestArtwork(c, raw, 384)
	c.workers.Wait()
	if requestTestArtwork(c, raw, 384) == nil || attempts.Load() != 2 {
		t.Fatal("canceled cache did not recover with one full body")
	}
	assertArtworkAccounting(t, c)
}

func TestArtworkHTTPNoStore304DoesNotLeaveReusableBody(t *testing.T) {
	dir := t.TempDir()
	data := artworkTestPNG(t, 32, 32)
	requests, bodies := 0, 0
	const raw = "https://i.ytimg.com/no-store-304"
	upstream := artworkTransport(func(req *http.Request) (*http.Response, error) {
		requests++
		if requests == 2 {
			if req.Header.Get("If-None-Match") != `"v1"` {
				t.Error("304 fixture was not conditional")
			}
			return &http.Response{StatusCode: 304, Header: http.Header{"Cache-Control": {"no-store"}}, Body: http.NoBody}, nil
		}
		bodies++
		return artworkHTTPResponse(data, http.Header{"Cache-Control": {"public, max-age=0"}, "Etag": {`"v1"`}}), nil
	})
	for range 3 {
		loadHTTPArtwork(t, dir, raw, upstream)
	}
	if requests != 3 || bodies != 2 {
		t.Fatalf("304 no-store reused prohibited body: requests=%d bodies=%d", requests, bodies)
	}
}

func TestArtworkHTTPPublicWithdrawalReplacesWithoutPersisting(t *testing.T) {
	for _, status := range []int{200, 304} {
		for _, control := range []string{"no-store", "private, max-age=3600"} {
			t.Run(fmt.Sprintf("%d-%s", status, strings.Split(control, ",")[0]), func(t *testing.T) {
				dir := t.TempDir()
				requests, bodies := 0, 0
				data := artworkTestPNG(t, 32, 32)
				const raw = "https://i.ytimg.com/public-withdrawal"
				upstream := artworkTransport(func(req *http.Request) (*http.Response, error) {
					requests++
					if requests == 3 && (req.Header.Get("If-None-Match") != "" || req.Header.Get("If-Modified-Since") != "") {
						t.Error("withdrawn public response retained validator")
					}
					h := http.Header{"Cache-Control": {"public, max-age=0"}, "Etag": {`"v1"`}}
					if requests == 2 {
						h.Set("Cache-Control", control)
						if status == 304 {
							return &http.Response{StatusCode: 304, Header: h, Body: http.NoBody}, nil
						}
					}
					bodies++
					return artworkHTTPResponse(data, h), nil
				})
				for range 3 {
					loadHTTPArtwork(t, dir, raw, upstream)
				}
				expected := 3
				if status == 304 {
					expected = 2
				}
				if requests != 3 || bodies != expected {
					t.Fatal("withdrawn public response remained reusable")
				}
			})
		}
	}
}

func TestArtworkHTTPStorageFailureStillDisplaysImage(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "not-a-directory")
	if os.WriteFile(path, []byte("fixture"), 0600) != nil {
		t.Fatal("storage failure fixture failed")
	}
	requests := 0
	data := artworkTestPNG(t, 384, 384)
	upstream := artworkTransport(func(*http.Request) (*http.Response, error) {
		requests++
		return artworkHTTPResponse(data, http.Header{"Cache-Control": {"public, max-age=3600"}}), nil
	})
	for range 2 {
		loadHTTPArtwork(t, path, "https://i.ytimg.com/storage-failure", upstream)
	}
	if requests != 2 {
		t.Fatal("failed storage did not degrade to ordinary download")
	}
}

func TestArtworkHTTPVarySchemeChangeRetiresOldRepresentations(t *testing.T) {
	for _, status := range []int{200, 304} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			c := artworkTestCache(t, nil)
			c.disk = &artworkDisk{dir: t.TempDir()}
			requests, bodies := 0, 0
			c.client.Transport = artworkTransport(func(req *http.Request) (*http.Response, error) {
				requests++
				h := http.Header{"Cache-Control": {"public, max-age=0"}, "Vary": {"Accept-Language"}, "Etag": {`"old"`}}
				edge := 32
				if req.Header.Get("Accept-Language") == "ja" {
					edge = 64
				}
				if requests >= 3 {
					h.Set("Vary", "Accept-Encoding")
					h.Set("Cache-Control", "public, max-age=3600")
					h.Set("Etag", `"new"`)
					edge = 96
					if status == 304 {
						return &http.Response{StatusCode: 304, Header: h, Body: http.NoBody}, nil
					}
				}
				bodies++
				return artworkHTTPResponse(artworkTestPNG(t, edge, 32), h), nil
			})
			const raw = "https://i.ytimg.com/vary-scheme-change"
			client := c.artworkHTTPClient(artDemand{artRequest: artRequest{raw, 384}})
			for i, lang := range []string{"en", "ja", "en", "ja"} {
				req, _ := http.NewRequestWithContext(t.Context(), http.MethodGet, raw, nil)
				req.Header.Set("Accept-Language", lang)
				req.Header.Set("Accept-Encoding", "identity")
				res, err := client.Do(req)
				if err != nil {
					t.Fatal("Vary transition request failed")
				}
				data, _ := io.ReadAll(res.Body)
				res.Body.Close()
				art := c.decode(t.Context(), data, 384)
				if art.bitmap == nil {
					t.Fatal("Vary transition image missing")
				}
				want := 32
				if i == 1 {
					want = 64
				}
				if i >= 2 && status == 200 {
					want = 96
				}
				if w, _ := art.bitmap.Size(); w != want {
					t.Fatalf("Vary transition served wrong representation: got=%d want=%d", w, want)
				}
			}
			wantBodies := 3
			if status == 304 {
				wantBodies = 2
			}
			if requests != 3 || bodies != wantBodies {
				t.Fatalf("obsolete Vary scheme was not retired: requests=%d bodies=%d", requests, bodies)
			}
		})
	}
}

func TestArtworkResidentExpiresFreshnessOnWarmReturn(t *testing.T) {
	for _, tc := range []struct {
		name, control string
		expires       time.Duration
		fresh         bool
	}{
		{"expires-only", "public", time.Hour, true},
		{"max-age-zero-precedes-expires", "public, max-age=0", time.Hour, false},
		{"expired", "public", -time.Hour, false},
		{"no-cache", "public, no-cache", time.Hour, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := artworkTestCache(t, nil)
			c.disk = &artworkDisk{dir: t.TempDir()}
			requests, bodies := 0, 0
			data := artworkTestPNG(t, 384, 384)
			c.client.Transport = artworkTransport(func(req *http.Request) (*http.Response, error) {
				requests++
				h := http.Header{"Cache-Control": {tc.control}, "Expires": {time.Now().Add(tc.expires).UTC().Format(http.TimeFormat)}, "Date": {time.Now().UTC().Format(http.TimeFormat)}, "Etag": {`"v1"`}}
				if requests > 1 {
					if req.Header.Get("If-None-Match") != `"v1"` {
						t.Error("stale resident cover did not validate")
					}
					h.Set("Cache-Control", "public, max-age=3600")
					return &http.Response{StatusCode: 304, Header: h, Body: http.NoBody}, nil
				}
				bodies++
				return artworkHTTPResponse(data, h), nil
			})
			const raw = "https://i.ytimg.com/resident-expires"
			requestTestArtwork(c, raw, 384)
			c.workers.Wait()
			original := requestTestArtwork(c, raw, 384)
			f := artworkFrame{cache: c}
			f.submit()
			got := requestTestArtwork(c, raw, 384)
			if tc.fresh {
				if got != original || got == nil {
					t.Fatal("fresh Expires-only pixels did not retain bitmap on return")
				}
			} else if got != nil {
				t.Fatal("stale resident image bypassed HTTP validation")
			}
			c.workers.Wait()
			want := 2
			if tc.fresh {
				want = 1
			}
			if requests != want || bodies != 1 {
				t.Fatalf("resident freshness body reuse mismatch: requests=%d bodies=%d", requests, bodies)
			}
		})
	}
}
