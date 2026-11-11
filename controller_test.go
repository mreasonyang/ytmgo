package main

import (
	"context"
	"fmt"
	"testing"
	"ytmgo/internal/catalog"
)

func TestExpiredContinuationKeepsPageAndPlaybackUntilReload(t *testing.T) {
	for _, err := range []error{catalog.ErrExpiredContinuation, catalog.ErrSessionChanged} {
		t.Run(err.Error(), func(t *testing.T) {
			m, actions := nativeViewFixture()
			m.Page.Continuation = "expired-page"
			m.SignedIn = true
			a := &desktop{model: *m, pageGeneration: 7}
			a.model.setContinuationFailure(fmt.Errorf("wrapped: %w", err), actions.TrArgs)
			if a.model.Loading || a.model.errorMessage.key != "nativeCatalogPageExpired" || !nativeContinuationExpired(&a.model) {
				t.Fatal("invalidated pagination did not offer a page reload")
			}
			if len(a.model.Page.Items) != len(m.Page.Items) || a.model.Page.Continuation != "expired-page" ||
				a.model.Current != m.Current || a.model.Playing != m.Playing || a.model.Position != m.Position || len(a.model.Queue) != len(m.Queue) || !a.model.SignedIn {
				t.Fatal("pagination failure changed existing content, playback or account state")
			}
			// Neither automatic nor direct LoadMore may retry this dead cursor.
			a.loadMore()
			if a.model.Loading || !nativeContinuationExpired(&a.model) {
				t.Fatal("expired cursor was submitted again")
			}
		})
	}
}

func TestStaleExpiredContinuationCannotChangeNewerPage(t *testing.T) {
	a := &desktop{pageGeneration: 8, model: UIModel{Loading: true, Page: catalog.Page{Title: "new page", Continuation: "new-cursor"}}}
	a.finishContinuation(7, catalog.Page{}, catalog.ErrExpiredContinuation)
	if !a.model.Loading || a.model.Error != "" || a.model.Page.Title != "new page" || a.model.Page.Continuation != "new-cursor" {
		t.Fatal("stale pagination failure changed the new page")
	}
}

func TestPauseInvalidatesQueuedResume(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	a := &desktop{resumeCancel: cancel}
	a.pause()
	if ctx.Err() != context.Canceled || a.resumeCancel != nil {
		t.Fatal("pause did not invalidate the queued resume")
	}
	a.pause()
}

func TestPlayableQueueKeepsOrderAndExcludesBrowseDestinations(t *testing.T) {
	items := []catalog.Item{{BrowseID: "album", Title: "Album"}, {ID: "first"}, {BrowseID: "artist", Title: "Artist"}, {ID: "second"}}
	queue := playableItems(items)
	if len(queue) != 2 || queue[0].ID != "first" || queue[1].ID != "second" {
		t.Fatal("mixed search results must enqueue only tracks, in order")
	}
	queue[0].Title = "changed"
	if items[1].Title != "" {
		t.Fatal("queue must own its items")
	}
}

func TestPreparedCandidateFollowsQueueAndRepeatMode(t *testing.T) {
	queue := []catalog.Item{{ID: "first"}, {ID: "second"}, {ID: "third"}}
	for _, tc := range []struct {
		index, repeat int
		want          string
	}{
		{0, 0, "second"}, {1, 0, "third"}, {2, 0, ""}, {2, 1, "first"}, {0, 2, ""}, {-1, 1, ""}, {3, 1, ""},
	} {
		if got := nextPreparedTrack(queue, tc.index, tc.repeat).ID; got != tc.want {
			t.Fatalf("index=%d repeat=%d candidate=%q want=%q", tc.index, tc.repeat, got, tc.want)
		}
	}
	queue[1], queue[2] = queue[2], queue[1]
	if got := nextPreparedTrack(queue, 0, 0).ID; got != "third" {
		t.Fatal("prepared candidate ignored shuffled order")
	}
	if got := nextPreparedTrack(nil, 0, 1).ID; got != "" {
		t.Fatal("empty queue prepared a track")
	}
}

func TestRatingUpdateKeepsCurrentAndQueueConsistent(t *testing.T) {
	a := &desktop{model: UIModel{Current: catalog.Item{ID: "current"}, Queue: []catalog.Item{{ID: "current"}, {ID: "other"}}, Page: catalog.Page{Items: []catalog.Item{{ID: "current"}}, Shelves: []catalog.Shelf{{Items: []catalog.Item{{ID: "current"}}}}}}}
	a.setLiked("current", true)
	if !a.model.Liked || !a.model.Current.LikeKnown || !a.model.Queue[0].Liked || !a.model.Page.Items[0].Liked || !a.model.Page.Shelves[0].Items[0].Liked || a.model.Queue[1].LikeKnown {
		t.Fatal("authoritative rating must update matching items without affecting unrelated tracks")
	}
	a.setLiked("other", true)
	if !a.model.Liked || !a.model.Queue[1].Liked {
		t.Fatal("a completed mutation for an earlier track must not replace the current rating")
	}
}

func TestArtworkResolutionIsSourceAware(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"https://lh3.googleusercontent.com/a=w120-h120-l90-rj", "https://lh3.googleusercontent.com/a=w768-h768-l90-rj"},
		{"https://yt3.ggpht.com/a=s120-c-k-c0x00ffffff-no-rj", "https://yt3.ggpht.com/a=w768-h768-c-k-c0x00ffffff-no-rj"},
		{"https://example.com/a=w120-h120", "https://example.com/a=w120-h120"},
		{"https://i.ytimg.com/vi/id/hqdefault.jpg", "https://i.ytimg.com/vi/id/hqdefault.jpg"},
	} {
		if got := artworkRequest(tc.in); got != tc.want {
			t.Errorf("unexpected artwork request: %s", got)
		}
	}
}

func TestPendingLibraryFollowsNavigationAndAccount(t *testing.T) {
	page := catalog.Page{Items: []catalog.Item{{Title: "Saved playlist"}}}
	for _, action := range []string{"stay on Home", "leave Library", "change account"} {
		t.Run(action, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			pending := &libraryRequest{cancel: cancel}
			a := &desktop{ctx: ctx, libraryRequest: pending, historyIndex: -1,
				model: UIModel{SignedIn: true, Route: "home", Page: catalog.Page{Title: "Home"}}}
			if action == "change account" {
				a.clearAccountContent()
				a.finishLibrary(pending, page, nil)
				if ctx.Err() == nil || len(a.model.LibraryItems) != 0 || a.model.LibraryLoaded {
					t.Fatal("old request survived the account change")
				}
				return
			}
			if action == "leave Library" {
				a.navigate("library")
				if a.libraryRequest != pending || !a.model.Loading {
					t.Fatal("navigation did not join the background request")
				}
				a.history = []historyEntry{{route: "home", page: catalog.Page{Title: "Home"}}, {route: "explore"}}
				a.historyIndex = 1
				a.travel(-1)
			}
			a.finishLibrary(pending, page, nil)
			if !a.model.LibraryLoaded || len(a.model.LibraryItems) != 1 {
				t.Fatal("sidebar did not receive the saved playlist")
			}
			if a.model.Route != "home" || a.model.Page.Title != "Home" || len(a.model.Page.Items) != 0 {
				t.Fatal("the sidebar response replaced Home")
			}
		})
	}
}
