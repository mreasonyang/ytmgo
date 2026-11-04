package catalog

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sync/atomic"
	"testing"
	"time"
)

func waitCache(t *testing.T, predicate func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for !predicate() {
		if time.Now().After(deadline) {
			t.Fatal("cache worker did not reach expected state")
		}
		time.Sleep(time.Millisecond)
	}
}

func TestConcurrentTrackQueueLyricsShareOneNext(t *testing.T) {
	var next, browse atomic.Int32
	release := make(chan struct{})
	client := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/youtubei/v1/next" {
			next.Add(1)
			select {
			case <-release:
			case <-r.Context().Done():
				return
			}
			_ = json.NewEncoder(w).Encode(queueResponse(true))
		} else {
			browse.Add(1)
			_ = json.NewEncoder(w).Encode(timedLyricsFixture(timedLyricFixture("Synthetic", "0", "1000")))
		}
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	done := make(chan error, 3)
	go func() { _, e := client.Queue(ctx, "abc123DEF_-"); done <- e }()
	go func() { _, e := client.Track(ctx, "abc123DEF_-"); done <- e }()
	go func() { _, e := client.LyricsWithTiming(ctx, "abc123DEF_-"); done <- e }()
	waitCache(t, func() bool {
		client.mu.RLock()
		defer client.mu.RUnlock()
		f := client.queues.flights["abc123DEF_-"]
		return f != nil && f.waiters == 3
	})
	close(release)
	for range 3 {
		if err := <-done; err != nil {
			t.Fatal(err)
		}
	}
	if next.Load() != 1 || browse.Load() != 1 {
		t.Fatal("duplicate same-track requests")
	}
	t.Logf("three simultaneous consumers: next=%d lyrics_browse=%d", next.Load(), browse.Load())
}

func TestLyricsCacheReusesPublicTimingAndOwnsItsLines(t *testing.T) {
	var requests atomic.Int32
	client := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.URL.Path == "/youtubei/v1/next" {
			_ = json.NewEncoder(w).Encode(queueResponse(true))
			return
		}
		if r.Header.Get("Authorization") != "" {
			_ = json.NewEncoder(w).Encode(plainLyricsFixture())
			return
		}
		_ = json.NewEncoder(w).Encode(timedLyricsFixture(timedLyricFixture("Synthetic timing", "0", "1000")))
	})
	if err := client.SetSession(syntheticCookies(), 0, ""); err != nil {
		t.Fatal(err)
	}
	first, err := client.LyricsWithTiming(context.Background(), "abc123DEF_-")
	if err != nil || len(first.Lines) != 1 {
		t.Fatal("initial timing failed")
	}
	first.Lines[0].Text = "caller mutation"
	before := requests.Load()
	second, err := client.LyricsWithTiming(context.Background(), "abc123DEF_-")
	if err != nil || second.Lines[0].Text != "Synthetic timing" || requests.Load() != before {
		t.Fatal("lyrics were refetched or shared mutable data")
	}
	t.Logf("public timing recovery: first_requests=%d replay_requests=%d", before, requests.Load()-before)
	client.mu.Lock()
	client.lyricResults.entries["timed:abc123DEF_-"].expires = time.Now().Add(-time.Second)
	client.mu.Unlock()
	if _, err = client.LyricsWithTiming(context.Background(), "abc123DEF_-"); err != nil || requests.Load() != before+3 {
		t.Fatal("expired timing not refreshed")
	}
}

func TestPageCacheRefreshExpiryLocaleIdentityAndRating(t *testing.T) {
	var calls atomic.Int32
	client := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.URL.Path == "/youtubei/v1/like/like" {
			_ = json.NewEncoder(w).Encode(object{"responseContext": object{}})
			return
		}
		_ = json.NewEncoder(w).Encode(catalogResponse(responsiveSong("abc123DEF_-", "Original")))
	})
	ctx := context.Background()
	page, err := client.Home(ctx)
	if err != nil {
		t.Fatal(err)
	}
	page.Items[0].Title = "mutated"
	page.Shelves[0].Items[0].Title = "mutated"
	page, err = client.Home(ctx)
	if err != nil || calls.Load() != 1 || page.Items[0].Title != "Original" || page.Shelves[0].Items[0].Title != "Original" {
		t.Fatal("page cache not isolated")
	}
	if _, err = client.Home(Refresh(ctx)); err != nil || calls.Load() != 2 {
		t.Fatal("explicit refresh used cache")
	}
	client.mu.Lock()
	for _, e := range client.pages.entries {
		e.expires = time.Now().Add(-time.Second)
	}
	client.mu.Unlock()
	if _, err = client.Home(ctx); err != nil || calls.Load() != 3 {
		t.Fatal("expired page used cache")
	}
	client.SetLanguage("ja")
	if _, err = client.Home(ctx); err != nil || calls.Load() != 4 {
		t.Fatal("language reused prior page")
	}
	if err = client.SetSession(syntheticCookies(), 0, ""); err != nil {
		t.Fatal(err)
	}
	if _, err = client.Home(ctx); err != nil || calls.Load() != 5 {
		t.Fatal("account reused prior page")
	}
	if err = client.Like(ctx, "abc123DEF_-", true); err != nil {
		t.Fatal(err)
	}
	if _, err = client.Home(ctx); err != nil || calls.Load() != 7 {
		t.Fatal("rating mutation retained page")
	}
	client.ClearSession()
	if _, err = client.Home(ctx); err != nil || calls.Load() != 8 {
		t.Fatal("sign-out reused private page")
	}
}

func TestSharedRequestCancellationAndLatePublication(t *testing.T) {
	for _, mode := range []string{"one_waiter", "all_waiters", "account", "language"} {
		t.Run(mode, func(t *testing.T) {
			client := New()
			var m responseCache[int]
			started, release, stopped := make(chan struct{}), make(chan struct{}), make(chan struct{})
			load := func(ctx context.Context) (int, error) {
				close(started)
				defer close(stopped)
				select {
				case <-release:
					return 7, nil
				case <-ctx.Done():
					return 0, ctx.Err()
				}
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			first := make(chan error, 1)
			go func() {
				_, e := memoized(client, ctx, &m, "same", time.Minute, 1024, func(int) int { return 8 }, load)
				first <- e
			}()
			<-started
			if mode == "one_waiter" {
				second := make(chan error, 1)
				go func() {
					v, e := memoized(client, context.Background(), &m, "same", time.Minute, 1024, func(int) int { return 8 }, load)
					if e == nil && v != 7 {
						e = errors.New("wrong result")
					}
					second <- e
				}()
				waitCache(t, func() bool { client.mu.RLock(); defer client.mu.RUnlock(); return m.flights["same"].waiters == 2 })
				cancel()
				if !errors.Is(<-first, context.Canceled) {
					t.Fatal("waiter did not cancel")
				}
				select {
				case <-stopped:
					t.Fatal("one waiter canceled other consumers")
				default:
				}
				close(release)
				if e := <-second; e != nil {
					t.Fatal(e)
				}
			} else if mode == "all_waiters" {
				cancel()
				if !errors.Is(<-first, context.Canceled) {
					t.Fatal("waiter did not cancel")
				}
				select {
				case <-stopped:
				case <-time.After(time.Second):
					t.Fatal("orphaned shared request")
				}
			} else {
				// Reset the same cache as the client lifecycle does, and change
				// identity/locale while a response is still in flight.
				client.mu.Lock()
				m.reset()
				client.mu.Unlock()
				if mode == "account" {
					_ = client.SetSession(syntheticCookies(), 0, "")
				} else {
					client.SetLanguage("ja")
				}
				if e := <-first; !errors.Is(e, ErrSessionChanged) {
					t.Fatal("old result survived lifecycle reset")
				}
			}
			client.mu.RLock()
			entries := len(m.entries)
			client.mu.RUnlock()
			if mode != "one_waiter" && entries != 0 {
				t.Fatal("canceled worker repopulated cache")
			}
		})
	}
}

func TestResponseCacheLimitsErrorsAndNegativeLyrics(t *testing.T) {
	c := New()
	var m responseCache[string]
	calls := 0
	load := func(context.Context) (string, error) { calls++; return "value", nil }
	for _, key := range []string{"one", "two", "three"} {
		if _, e := memoized(c, context.Background(), &m, key, time.Minute, 150, func(s string) int { return len(s) }, load); e != nil {
			t.Fatal(e)
		}
	}
	if len(m.entries) != 1 || m.bytes > 150 {
		t.Fatal("response budget exceeded")
	}
	if _, e := memoized(c, context.Background(), &m, "large", time.Minute, 150, func(string) int { return 1000 }, load); e != nil {
		t.Fatal(e)
	}
	if m.entries["large"] != nil {
		t.Fatal("oversized response retained")
	}
	for range 2 {
		_, _ = memoized(c, context.Background(), &m, "failure", time.Minute, 150, func(string) int { return 0 }, func(context.Context) (string, error) { calls++; return "", ErrUnavailable })
	}
	if m.entries["failure"] != nil {
		t.Fatal("transient failure cached")
	}
	before := calls
	for range 2 {
		_, _ = memoized(c, context.Background(), &m, "no_lyrics", time.Minute, 150, func(string) int { return 0 }, func(context.Context) (string, error) { calls++; return "", ErrNoLyrics })
	}
	if calls != before+1 || time.Until(m.entries["no_lyrics"].expires) > 30*time.Second {
		t.Fatal("negative lyric cache missing or unbounded")
	}
}
