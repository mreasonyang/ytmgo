package main

import (
	"slices"
	"testing"

	"ytmgo/internal/catalog"
)

func TestConfirmedUnlikeUpdatesLikedMusicAndHistory(t *testing.T) {
	track := catalog.Item{ID: "liked-track", Title: "Liked track", Liked: true, LikeKnown: true}
	other := catalog.Item{ID: "other"}
	tracks := []catalog.Item{track, other}
	page := catalog.Page{Items: tracks, Shelves: []catalog.Shelf{{Items: tracks}}}
	app := &desktop{historyIndex: -1, model: UIModel{Current: track, Liked: true, Queue: tracks, Page: page, Route: likedMusicRoute}}
	app.pushHistory(true)
	// Two routes and the queue can still reference the same backing array.
	app.model.Route = "browse:VLother"
	app.pushHistory(true)
	app.travel(-1)
	app.confirmLike(track, false)
	if app.model.Liked || app.model.Current.ID != track.ID || len(app.model.Queue) != 2 || app.model.Queue[0].Liked {
		t.Fatal("unliking must update the rating and keep playback and queue order")
	}
	if !slices.Equal(app.model.Page.Items, []catalog.Item{other}) || !slices.Equal(app.model.Page.Shelves[0].Items, []catalog.Item{other}) {
		t.Fatal("Liked Music retained the unliked row")
	}
	app.travel(1)
	if len(app.model.Page.Items) != 2 || app.model.Page.Items[0].ID != track.ID || app.model.Page.Items[0].Liked ||
		len(app.model.Page.Shelves[0].Items) != 2 {
		t.Fatal("another playlist lost the song or retained a stale rating")
	}
	app.travel(-1)
	if len(app.model.Page.Items) != 1 || len(app.model.Page.Shelves[0].Items) != 1 {
		t.Fatal("Back brought back the unliked row")
	}
	app.confirmLike(track, true)
	if !slices.Equal(app.model.Page.Items, tracks) || !slices.Equal(app.model.Page.Shelves[0].Items, tracks) {
		t.Fatal("re-liking did not restore exactly one copy of the song at the top")
	}
}

func TestLikedMusicReconcilesLatePages(t *testing.T) {
	track := catalog.Item{ID: "rated-track", Title: "Rated track"}
	other := catalog.Item{ID: "untouched"}
	for _, liked := range []bool{false, true} {
		for _, replace := range []bool{false, true} {
			stale := track
			stale.Liked, stale.LikeKnown = !liked, true
			rows := []catalog.Item{stale, other}
			app := &desktop{historyIndex: -1, model: UIModel{Route: likedMusicRoute, Loading: true, Page: catalog.Page{
				Continuation: "next", Items: slices.Clone(rows), Shelves: []catalog.Shelf{{Items: slices.Clone(rows)}},
			}}}
			app.pushHistory(true)
			app.confirmLike(track, liked)
			// Leave and return to the page before the next response. The stored
			// cursor can still serve data from before the mutation.
			app.model.Page = catalog.Page{}
			app.model.Route = "home"
			app.pushHistory(true)
			app.travel(-1)
			late := catalog.Page{Items: []catalog.Item{stale}, Shelves: []catalog.Shelf{
				{Items: []catalog.Item{stale}}, {Title: "Recommended", Items: []catalog.Item{stale}},
			}}
			app.finishPageContinuation(app.pageGeneration, late, replace, nil)
			got := 0
			for _, shelf := range app.model.Page.Shelves {
				for _, it := range shelf.Items {
					if it.ID == track.ID {
						if it.Liked != liked || !it.LikeKnown {
							t.Fatalf("liked=%t replace=%t: late reply restored obsolete rating", liked, replace)
						}
						if shelf.Title == "" {
							got++
						}
					}
				}
			}
			want := 0
			if liked {
				want = 1
			}
			shelves := app.model.Page.Shelves
			if got != want || shelves[len(shelves)-1].Title != "Recommended" || shelves[len(shelves)-1].Items[0].ID != track.ID ||
				len(app.likedChanges) != 0 || len(app.history[0].likedChanges) != 0 {
				t.Fatalf("liked=%t replace=%t: track rows=%d, recommendations or completed history cursor incorrect", liked, replace, got)
			}
		}
	}
}

func TestLikedMusicPendingFirstPageAndLastRemoval(t *testing.T) {
	track := catalog.Item{ID: "pending", Liked: true, LikeKnown: true}
	app := &desktop{model: UIModel{Route: likedMusicRoute, Loading: true}}
	app.confirmLike(track, false)
	app.model.Page = catalog.Page{Items: []catalog.Item{track}, Shelves: []catalog.Shelf{{Items: []catalog.Item{track}}}}
	app.reconcileLikedMusic()
	if len(app.model.Page.Items) != 0 || len(app.model.Page.Shelves) != 0 || len(app.likedChanges) != 0 {
		t.Fatal("an in-flight first page restored the last removed song")
	}
	app.confirmLike(track, true)
	if !slices.Equal(app.model.Page.Items, []catalog.Item{track}) {
		t.Fatal("re-liking could not populate an empty playlist")
	}
}

func TestUnlikedRecommendationUsesItsVisibleQueuePosition(t *testing.T) {
	first, last := catalog.Item{ID: "first"}, catalog.Item{ID: "last"}
	track, recommended := catalog.Item{ID: "rated", LikeKnown: true, Liked: true}, catalog.Item{ID: "recommended"}
	page := catalog.Page{
		Items: []catalog.Item{first, track, last, recommended},
		Shelves: []catalog.Shelf{
			{Items: []catalog.Item{first, track, last}},
			{Title: "Recommended", Items: []catalog.Item{recommended, track}},
		},
		Continuation: "more-recommendations",
	}
	app := &desktop{model: UIModel{Page: page, Route: likedMusicRoute}}
	app.confirmLike(track, false)
	app.finishPageContinuation(app.pageGeneration, catalog.Page{
		Items: []catalog.Item{track}, Shelves: []catalog.Shelf{{Items: []catalog.Item{track}}},
	}, false, nil)
	if shelves := app.model.Page.Shelves; len(shelves) != 3 || len(shelves[2].Items) != 1 {
		t.Fatal("an untitled recommendation continuation lost its song")
	}
	app.playItem(first)
	track.Liked = false
	want := []catalog.Item{first, last, recommended, track}
	if !slices.Equal(app.model.Queue, want) {
		t.Fatalf("playing the first track created a queue out of shelf order: %+v", app.model.Queue)
	}
}

func TestLikedMusicUsesPlaylistCursorForUntitledSections(t *testing.T) {
	track := catalog.Item{ID: "song", LikeKnown: true}
	app := &desktop{model: UIModel{Route: likedMusicRoute, Page: catalog.Page{
		Items: []catalog.Item{track},
		Shelves: []catalog.Shelf{
			{PlaylistSection: catalog.PlaylistSectionExtras, Items: []catalog.Item{track}},
			{PlaylistSection: catalog.PlaylistSectionTracks, Items: []catalog.Item{track}},
		},
	}}}
	app.confirmLike(track, false)
	shelves := app.model.Page.Shelves
	if len(shelves) != 1 || shelves[0].PlaylistSection != catalog.PlaylistSectionExtras ||
		!slices.Equal(shelves[0].Items, []catalog.Item{track}) || !slices.Equal(app.model.Page.Items, []catalog.Item{track}) {
		t.Fatal("unliking removed an anonymous recommendation or retained a playlist track")
	}
	app.confirmLike(track, true)
	shelves = app.model.Page.Shelves
	if len(shelves) != 2 || shelves[0].PlaylistSection != catalog.PlaylistSectionTracks || len(app.model.Page.Items) != 1 {
		t.Fatal("re-liking did not restore the main playlist ahead of its recommendations")
	}
}
