package catalog

import (
	"context"
	"errors"
	"net/http"
	"os"
	"sync/atomic"
	"testing"
	"time"
)

// Explicit opt-in. Public metadata only: no account, audio, signed URL or raw
// response enters the test output; the transport records only request counts.
func TestLiveCatalogCache(t *testing.T) {
	if os.Getenv("YTMGO_TEST_PUBLIC_CATALOG_CACHE") != "1" {
		t.Skip("public catalog validation not selected")
	}
	var requests atomic.Int32
	c := NewWithOptions(Options{HTTPClient: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.Header.Get("Authorization") != "" || req.Header.Get("Cookie") != "" {
			t.Fatal("public check received credentials")
		}
		requests.Add(1)
		return defaultTransport().RoundTrip(req)
	})}})
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	page, err := c.Search(ctx, "Get Lucky")
	if err != nil {
		t.Fatal(err)
	}
	before := requests.Load()
	if _, err = c.Search(ctx, "Get Lucky"); err != nil || requests.Load() != before {
		t.Fatal("repeat search made a request")
	}
	t.Logf("real search: first_requests=%d repeated_requests=0 items=%d", before, len(page.Items))
	var id string
	for _, it := range page.Items {
		if videoIDPattern.MatchString(it.ID) {
			id = it.ID
			break
		}
	}
	if id == "" {
		t.Fatal("public search returned no playable metadata")
	}
	first, err := c.LyricsWithTiming(ctx, id)
	if err != nil && !errors.Is(err, ErrNoLyrics) {
		t.Fatal(err)
	}
	firstRequests := requests.Load() - before
	before = requests.Load()
	second, repeatErr := c.LyricsWithTiming(ctx, id)
	if (repeatErr != nil && !errors.Is(repeatErr, ErrNoLyrics)) || requests.Load() != before || second.Text != first.Text {
		t.Fatal("repeat lyrics not reused")
	}
	t.Logf("real lyrics: first_requests=%d repeated_requests=0 timed_lines=%d", firstRequests, len(first.Lines))
}
