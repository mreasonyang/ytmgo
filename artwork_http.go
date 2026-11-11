package main

import (
	"bufio"
	"bytes"
	"errors"
	"image"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"github.com/bartventer/httpcache"
	"github.com/bartventer/httpcache/pkg/urlkey"
	"github.com/bartventer/httpcache/store"
	"github.com/bartventer/httpcache/store/driver"
	"ytmgo/internal/httpfreshness"
)

func init() {
	store.Register("ytmgo-artwork", driver.DriverFunc(func(u *url.URL) (driver.Conn, error) {
		return artworkHTTPStore{&artworkDisk{dir: filepath.FromSlash(u.Query().Get("directory"))}}, nil
	}))
}

// httpcache v0.14 unconditionally stores the merged response after a 304.
// Recheck public admission at that final boundary as well as upstream.
type artworkHTTPStore struct{ *artworkDisk }

func (s artworkHTTPStore) Set(key string, data []byte) error {
	base, _, body := strings.Cut(key, "#")
	if body {
		_, wire, ok := bytes.Cut(data, []byte{'\n'})
		if !ok {
			return errArtworkStorage
		}
		res, err := http.ReadResponse(bufio.NewReader(bytes.NewReader(wire)), nil)
		if err != nil {
			return errArtworkStorage
		}
		res.Body.Close()
		if !artworkPublicResponse(res.Header) {
			_ = s.Delete(base)
			return errArtworkStorage
		}
	}
	return s.artworkDisk.Set(key, data)
}

func (c *artworkCache) artworkHTTPClient(d artDemand) *http.Client {
	if c.disk == nil || d.priority == artAvatar || artworkDiskKey(d.url) == "" {
		return c.client
	}
	c.httpOnce.Do(func() {
		client := *c.client
		dsn := (&url.URL{Scheme: "ytmgo-artwork", Host: "cache", RawQuery: url.Values{"directory": {c.disk.dir}}.Encode()}).String()
		client.Transport = httpcache.NewTransport(dsn, httpcache.WithUpstream(artworkHTTPUpstream{c}))
		c.httpClient = &client
	})
	return c.httpClient
}

// Keep the artwork-specific admission/size boundary outside the HTTP cache.
// The component handles freshness, Vary, validators and 304 merging itself.
type artworkHTTPUpstream struct{ cache *artworkCache }

var errArtworkResponse = errors.New("artwork_response_rejected")

func (u artworkHTTPUpstream) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.Header.Get("Authorization") != "" || req.Header.Get("Cookie") != "" {
		return nil, errArtworkResponse
	}
	res, err := u.cache.client.Transport.RoundTrip(req)
	if err != nil {
		return nil, err
	}
	res.Header = res.Header.Clone()
	if res.Header == nil {
		res.Header = make(http.Header)
	}
	if res.Body == nil {
		res.Body = http.NoBody
	}
	// v0.14.0 otherwise falls back to Last-Modified heuristics for max-age=0.
	// An already expired Expires prevents that fallback, preserving max-age=0
	// (and stale-while-revalidate) semantics without inventing a freshness TTL.
	for directive := range strings.SplitSeq(strings.Join(res.Header.Values("Cache-Control"), ","), ",") {
		name, value, _ := strings.Cut(strings.TrimSpace(directive), "=")
		if strings.EqualFold(strings.TrimSpace(name), "max-age") && strings.Trim(strings.TrimSpace(value), "\"") == "0" {
			res.Header.Set("Expires", "Thu, 01 Jan 1970 00:00:00 GMT")
		}
	}
	if res.StatusCode == http.StatusNotModified {
		if artworkProhibitedResponse(res.Header) || (len(res.Header.Values("Cache-Control")) > 0 && !artworkPublicResponse(res.Header)) {
			res.Header.Set("Cache-Control", "no-store")
		}
		return res, nil
	}
	if res.StatusCode != http.StatusOK || artworkDiskKey(req.URL.String()) == "" || !artworkPublicResponse(res.Header) {
		res.Header.Set("Cache-Control", "no-store")
		if res.StatusCode == http.StatusOK || res.StatusCode == http.StatusNotFound || res.StatusCode == http.StatusGone {
			_ = u.cache.disk.Delete(urlkey.Normalize(req.URL))
		}
	}
	if res.StatusCode != http.StatusOK {
		return res, nil
	}
	defer res.Body.Close()
	if res.ContentLength > artworkDownloadLimit {
		return nil, errArtworkResponse
	}
	data, err := io.ReadAll(io.LimitReader(res.Body, artworkDownloadLimit+1))
	if err != nil || len(data) > artworkDownloadLimit {
		return nil, errArtworkResponse
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || cfg.Width < 1 || cfg.Height < 1 || cfg.Width > 4096 || cfg.Height > 4096 || cfg.Width*cfg.Height > artworkSourcePixels {
		return nil, errArtworkResponse
	}
	res.Body = io.NopCloser(bytes.NewReader(data))
	res.ContentLength = int64(len(data))
	return res, nil
}

func artworkPublicResponse(headers http.Header) bool {
	if artworkProhibitedResponse(headers) {
		return false
	}
	public := false
	for directive := range strings.SplitSeq(strings.ToLower(strings.Join(headers.Values("Cache-Control"), ",")), ",") {
		name, _, _ := strings.Cut(strings.TrimSpace(directive), "=")
		switch strings.TrimSpace(name) {
		case "public":
			public = true
		}
	}
	return public
}

func artworkProhibitedResponse(headers http.Header) bool {
	for field := range strings.SplitSeq(strings.Join(headers.Values("Vary"), ","), ",") {
		if strings.TrimSpace(field) == "*" {
			return true
		}
	}
	if _, ok := headers["Set-Cookie"]; ok {
		return true
	}
	for directive := range strings.SplitSeq(strings.ToLower(strings.Join(headers.Values("Cache-Control"), ",")), ",") {
		name, _, _ := strings.Cut(strings.TrimSpace(directive), "=")
		if strings.TrimSpace(name) == "private" || strings.TrimSpace(name) == "no-store" {
			return true
		}
	}
	return false
}

// This is a conservative lifetime for decoded pixels, not a network-cache TTL.
// A miss/expired image goes through httpcache, which owns HTTP reuse decisions.
func artworkFreshUntil(headers http.Header, requested time.Time) time.Time {
	return httpfreshness.FreshUntil(headers, requested, time.Duration(1<<63-1))
}
