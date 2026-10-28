package main

import (
	"context"
	"testing"
	"ytmgo/internal/catalog"
)

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
		if got := nextPreparedID(queue, tc.index, tc.repeat); got != tc.want {
			t.Fatalf("index=%d repeat=%d candidate=%q want=%q", tc.index, tc.repeat, got, tc.want)
		}
	}
	queue[1], queue[2] = queue[2], queue[1]
	if got := nextPreparedID(queue, 0, 0); got != "third" {
		t.Fatal("prepared candidate ignored shuffled order")
	}
	if got := nextPreparedID(nil, 0, 1); got != "" {
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
