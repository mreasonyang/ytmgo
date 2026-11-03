package main

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

func TestArtworkDiskRequiresPublicFreshness(t *testing.T) {
	start := time.Now().UTC().Truncate(time.Second)
	header := http.Header{"Cache-Control": {"public, max-age=7200"}, "Age": {"900"}, "Date": {start.Add(-2 * time.Hour).Format(http.TimeFormat)}}
	if got := artworkFreshUntil(header, start); !got.Equal(start) {
		t.Fatal("origin date extended cache lifetime")
	}
	header.Del("Date")
	if got := artworkFreshUntil(header, start); !got.Equal(start.Add(105 * time.Minute)) {
		t.Fatal("upstream age extended cache lifetime")
	}
	for _, change := range []struct{ field, value string }{
		{"Age", "7200"}, {"Age", "invalid"}, {"Cache-Control", "max-age=7200"},
		{"Cache-Control", "public, no-store, max-age=7200"}, {"Cache-Control", "public, private, max-age=7200"},
		{"Cache-Control", "public, no-cache, max-age=7200"}, {"Cache-Control", "public, max-age=4, max-age=40"},
		{"Cache-Control", "public, max-age=oops"}, {"Vary", "*"}, {"Vary", "Cookie"},
		{"Vary", "Origin, Accept-Language"}, {"Set-Cookie", ""},
	} {
		h := header.Clone()
		h.Set(change.field, change.value)
		if artworkFreshUntil(h, start).After(start) {
			t.Fatalf("unexpected freshness for %s", change.field)
		}
	}
	header.Set("Cache-Control", "public, max-age=604800")
	header.Set("Vary", "Origin, Accept-Encoding")
	if got := artworkFreshUntil(header, start); !got.Equal(start.Add(artworkDiskMaxAge)) {
		t.Fatal("local freshness limit not applied")
	}
}

func TestArtworkDiskRestartExpiryAndCorruptFile(t *testing.T) {
	dir := t.TempDir()
	data := artworkTestPNG(t, 384, 192)
	const raw = "https://i.ytimg.com/vi/example/hqdefault.jpg"
	key := artworkDiskKey(raw)
	path := filepath.Join(dir, key)
	var downloads int
	restart := func(edge int) {
		c := artworkTestCache(t, data)
		c.disk = &artworkDisk{dir: dir}
		transport := c.client.Transport
		c.client.Transport = artworkTransport(func(r *http.Request) (*http.Response, error) {
			downloads++
			res, err := transport.RoundTrip(r)
			res.Header = http.Header{"Cache-Control": {"public, max-age=86400"}}
			return res, err
		})
		requestTestArtwork(c, raw, edge)
		c.workers.Wait()
		if c.entries[artRequest{raw, edge}].bitmap == nil {
			t.Fatal("fresh loader did not produce a bitmap")
		}
	}
	restart(128)
	b, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(b, data) {
		t.Fatal("validated compressed artwork did not reach disk")
	}
	restart(384)
	if downloads != 1 {
		t.Fatal("fresh loader downloaded the same source again")
	}
	expired := time.Now().Add(-time.Second)
	if err := os.Chtimes(path, expired, expired); err != nil {
		t.Fatal(err)
	}
	restart(128)
	if downloads != 2 {
		t.Fatal("expired cover was reused")
	}
	if err := os.WriteFile(path, []byte("broken image"), 0600); err != nil {
		t.Fatal(err)
	}
	fresh := time.Now().Add(time.Hour)
	if err := os.Chtimes(path, fresh, fresh); err != nil {
		t.Fatal(err)
	}
	restart(128)
	restart(128)
	if downloads != 3 {
		t.Fatal("corrupt file was not replaced by one successful fetch")
	}
}

func TestArtworkDiskSkipsTransientAndUncacheableResponses(t *testing.T) {
	data := artworkTestPNG(t, 16, 16)
	for _, tc := range []struct {
		name, raw        string
		priority         artworkPriority
		header           string
		redirect, broken bool
	}{
		{"avatar", "https://lh3.googleusercontent.com/photo", artAvatar, "public, max-age=86400", false, false},
		{"query", "https://music.youtube.com/image/example?r=mix", artPrimary, "public, max-age=86400", false, false},
		{"no store", "https://i.ytimg.com/image", artPrimary, "no-store", false, false},
		{"redirect", "https://i.ytimg.com/image", artPrimary, "public, max-age=86400", true, false},
		{"invalid image", "https://i.ytimg.com/image", artPrimary, "public, max-age=86400", false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := artworkTestCache(t, data)
			dir := t.TempDir()
			c.disk = &artworkDisk{dir: dir}
			c.client.Transport = artworkTransport(func(r *http.Request) (*http.Response, error) {
				if tc.redirect && r.URL.Path == "/image" {
					return &http.Response{StatusCode: 302, Header: http.Header{"Location": {"https://i.ytimg.com/redirected"}}, Body: http.NoBody, Request: r}, nil
				}
				body := data
				if tc.broken {
					body = nil
				}
				return &http.Response{StatusCode: 200, Header: http.Header{"Cache-Control": {tc.header}}, Body: io.NopCloser(bytes.NewReader(body)), ContentLength: int64(len(body)), Request: r}, nil
			})
			frame := artworkFrame{cache: c}
			frame.get(tc.raw, 128, tc.priority)
			frame.submit()
			c.workers.Wait()
			files, err := os.ReadDir(dir)
			if err != nil || len(files) != 0 {
				t.Fatal("non-reusable artwork persisted")
			}
		})
	}
}

func TestArtworkDiskBoundsBytesAndFiles(t *testing.T) {
	for _, tc := range []struct {
		name  string
		count int
		size  int64
	}{
		{"bytes", artworkDiskBudget / artworkDownloadLimit, artworkDownloadLimit}, {"files", artworkDiskFiles, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			now := time.Now()
			for i := range tc.count {
				path := filepath.Join(dir, fmt.Sprintf("%064x", i))
				f, err := os.Create(path)
				if err != nil {
					t.Fatal(err)
				}
				if err = f.Truncate(tc.size); err != nil {
					t.Fatal(err)
				}
				f.Close()
				expiry := now.Add(time.Hour + time.Duration(i)*time.Second)
				if err = os.Chtimes(path, expiry, expiry); err != nil {
					t.Fatal(err)
				}
			}
			d := artworkDisk{dir: dir}
			key := artworkDiskKey("https://i.ytimg.com/new" + strconv.Itoa(tc.count))
			d.write(key, []byte{1, 2}, now.Add(24*time.Hour))
			entries, err := os.ReadDir(dir)
			if err != nil {
				t.Fatal(err)
			}
			var size int64
			for _, e := range entries {
				info, err := e.Info()
				if err != nil {
					t.Fatal(err)
				}
				size += info.Size()
			}
			if size > artworkDiskBudget || len(entries) > artworkDiskFiles || !bytes.Equal(d.read(key), []byte{1, 2}) {
				t.Fatal("disk limits prevented admission or exceeded their budgets")
			}
			if _, err = os.Stat(filepath.Join(dir, fmt.Sprintf("%064x", 0))); !os.IsNotExist(err) {
				t.Fatal("nearest expiry was not evicted")
			}
		})
	}
}
