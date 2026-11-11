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

func TestLikeRestartsConcurrentPageAndLyricsWithoutSessionFailure(t *testing.T) {
	for _, operation := range []string{"home", "lyrics"} {
		t.Run(operation, func(t *testing.T) {
			entered := make(chan struct{})
			release := make(chan struct{})
			defer close(release)
			var reads atomic.Int32
			client := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/youtubei/v1/like/like" {
					_ = json.NewEncoder(w).Encode(object{"responseContext": object{}})
					return
				}
				var payload object
				_ = json.NewDecoder(r.Body).Decode(&payload)
				if r.URL.Path == "/youtubei/v1/next" || operation == "home" {
					if reads.Add(1) == 1 {
						close(entered)
						select {
						case <-r.Context().Done():
						case <-release:
						}
						return
					}
				}
				if r.URL.Path == "/youtubei/v1/next" {
					_ = json.NewEncoder(w).Encode(queueResponse(true))
				} else if operation == "home" {
					_ = json.NewEncoder(w).Encode(catalogResponse(responsiveSong("abc123DEF_-", "Fresh")))
				} else {
					_ = json.NewEncoder(w).Encode(timedLyricsFixture(timedLyricFixture("Fresh lyrics", "0", "1000")))
				}
			})
			if err := client.SetSession(syntheticCookies(), 0, ""); err != nil {
				t.Fatal("synthetic setup failed")
			}
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			done := make(chan error, 1)
			go func() {
				if operation == "home" {
					_, err := client.Home(ctx)
					done <- err
				} else {
					_, err := client.LyricsWithTiming(ctx, "abc123DEF_-")
					done <- err
				}
			}()
			select {
			case <-entered:
			case <-ctx.Done():
				t.Fatal("fixture read did not start")
			}
			if err := client.Like(ctx, "abc123DEF_-", true); err != nil {
				t.Fatal("synthetic Like failed")
			}
			select {
			case err := <-done:
				if err != nil || reads.Load() != 2 {
					t.Fatalf("affected read was not transparently restarted: error_type=%T reads=%d", err, reads.Load())
				}
			case <-ctx.Done():
				t.Fatal("affected read did not recover within its caller deadline")
			}
		})
	}
}

func TestMutationRetrySharesFreshReadAndRejectsLateCachePublication(t *testing.T) {
	client := New()
	entered, release := make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	load := func(context.Context) (Page, error) {
		if calls.Add(1) == 1 {
			close(entered)
			<-release // A late transport result must never replace the fresh cache.
			return Page{Title: "Old"}, nil
		}
		return Page{Title: "Fresh"}, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	read := func() (Page, error) {
		return memoized(client, ctx, &client.pages, "shared", time.Minute, 1<<20, pageBytes, load)
	}
	done := make(chan error, 2)
	for range 2 {
		go func() {
			page, err := read()
			if err == nil && page.Title != "Fresh" {
				err = ErrInvalidResponse
			}
			done <- err
		}()
	}
	<-entered
	waitCache(t, func() bool {
		client.mu.RLock()
		defer client.mu.RUnlock()
		return client.pages.flights["shared"].waiters == 2
	})
	client.mu.Lock()
	client.pages.invalidate()
	client.mu.Unlock()
	close(release)
	for range 2 {
		if err := <-done; err != nil {
			t.Fatal("shared mutation retry returned an obsolete result or failure")
		}
	}
	page, err := read()
	if err != nil || page.Title != "Fresh" || calls.Load() != 2 {
		t.Fatal("retry was duplicated or the old flight contaminated the cache")
	}
}

func TestMutationRetryStillHonorsCancellationLanguageAndAccountBoundaries(t *testing.T) {
	for _, change := range []string{"cancel", "language", "account"} {
		t.Run(change, func(t *testing.T) {
			client := New()
			entered, release := make(chan struct{}), make(chan struct{})
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			done := make(chan error, 1)
			go func() {
				_, err := memoized(client, ctx, &client.pages, "boundary", time.Minute, 1<<20, pageBytes, func(context.Context) (Page, error) {
					close(entered)
					<-release
					return Page{Title: "Old identity or locale"}, nil
				})
				done <- err
			}()
			<-entered
			client.mu.Lock()
			client.pages.invalidate()
			client.mu.Unlock()
			want := ErrSessionChanged
			switch change {
			case "cancel":
				cancel()
				want = context.Canceled
			case "language":
				client.SetLanguage("ja")
			case "account":
				if err := client.SetSession(syntheticCookies(), 0, ""); err != nil {
					t.Fatal("synthetic account setup failed")
				}
			}
			close(release)
			if err := <-done; !errors.Is(err, want) {
				t.Fatal("mutation retry crossed a cancellation, language or account boundary")
			}
		})
	}
}
