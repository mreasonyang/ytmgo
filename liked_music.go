package main

import (
	"slices"

	"ytmgo/internal/catalog"
)

const likedMusicRoute = "browse:VLLM"

// A page or its next cursor can predate a successful rating change. Keep
// those changes with that page until its last cursor or a fresh full read.
type likedMusicChanges []catalog.Item

func setItemRating(item *catalog.Item, id string, liked bool) {
	if item.ID == id {
		item.Liked, item.LikeKnown = liked, true
	}
}

func setPageRating(page *catalog.Page, id string, liked bool) {
	for i := range page.Items {
		setItemRating(&page.Items[i], id, liked)
	}
	for _, shelf := range page.Shelves {
		for i := range shelf.Items {
			setItemRating(&shelf.Items[i], id, liked)
		}
	}
}

// A metadata read updates the rating everywhere the song appears. Only a
// successful user mutation changes membership of the Liked Music playlist.
func (a *desktop) setLiked(id string, liked bool) {
	setItemRating(&a.model.Current, id, liked)
	if a.model.Current.ID == id {
		a.model.Liked = liked
	}
	for i := range a.model.Queue {
		setItemRating(&a.model.Queue[i], id, liked)
	}
	setPageRating(&a.model.Page, id, liked)
	for i := range a.history {
		setPageRating(&a.history[i].page, id, liked)
	}
	for i := range a.model.LibraryItems {
		setItemRating(&a.model.LibraryItems[i], id, liked)
	}
	if a.libraryPage != nil {
		setPageRating(a.libraryPage, id, liked)
	}
}

func (a *desktop) confirmLike(track catalog.Item, liked bool) {
	a.setLiked(track.ID, liked)
	track.Liked, track.LikeKnown = liked, true
	apply := func(page *catalog.Page, edits *likedMusicChanges, loading bool) {
		updateLikedMusic(page, track)
		if loading || page.Continuation != "" {
			// History entries can share the old slice. Keep the latest edit
			// to each song, in the order the user liked them.
			next := slices.DeleteFunc(slices.Clone(*edits), func(it catalog.Item) bool { return it.ID == track.ID })
			*edits = append(next, track)
		}
	}
	if a.model.Route == likedMusicRoute {
		apply(&a.model.Page, &a.likedChanges, a.model.Loading)
	}
	for i := range a.history {
		entry := &a.history[i]
		if entry.route == likedMusicRoute {
			apply(&entry.page, &entry.likedChanges, false)
		}
	}
}

func (a *desktop) reconcileLikedMusic() {
	if a.model.Route != likedMusicRoute {
		return
	}
	for _, track := range a.likedChanges {
		setPageRating(&a.model.Page, track.ID, track.Liked)
		updateLikedMusic(&a.model.Page, track)
	}
	if a.model.Page.Continuation == "" {
		a.likedChanges = nil
	}
}

func updateLikedMusic(page *catalog.Page, track catalog.Item) {
	matches := func(it catalog.Item) bool { return it.ID == track.ID }
	withoutTrack := func(rows []catalog.Item) []catalog.Item {
		if slices.ContainsFunc(rows, matches) {
			return slices.DeleteFunc(slices.Clone(rows), matches)
		}
		return rows
	}

	// A recommendation can contain an unliked song. For older pages without
	// section metadata, a heading begins the recommendations and their continuations.
	page.Shelves = slices.Clone(page.Shelves)
	firstTracks := -1
	inRecommendations, recommended := false, false
	for i := range page.Shelves {
		shelf := &page.Shelves[i]
		switch shelf.PlaylistSection {
		case catalog.PlaylistSectionTracks:
			inRecommendations = false
		case catalog.PlaylistSectionExtras:
			inRecommendations = true
		default:
			inRecommendations = inRecommendations || shelf.Title != ""
		}
		if inRecommendations {
			recommended = recommended || slices.ContainsFunc(shelf.Items, matches)
			continue
		}
		if firstTracks < 0 {
			firstTracks = i
		}
		shelf.Items = withoutTrack(shelf.Items)
	}
	if track.Liked {
		// New likes go first, even if an older continuation also contains the song.
		page.Items = slices.Concat([]catalog.Item{track}, withoutTrack(page.Items))
		if firstTracks >= 0 {
			shelf := &page.Shelves[firstTracks]
			shelf.Items = slices.Concat([]catalog.Item{track}, shelf.Items)
		} else if len(page.Shelves) > 0 {
			page.Shelves = slices.Concat([]catalog.Shelf{{Items: []catalog.Item{track}, PlaylistSection: catalog.PlaylistSectionTracks}}, page.Shelves)
		}
	} else if recommended {
		page.Items = itemsInShelfOrder(page)
	} else {
		page.Items = withoutTrack(page.Items)
	}
	page.Shelves = slices.DeleteFunc(page.Shelves, func(s catalog.Shelf) bool {
		return len(s.Items) == 0 && (s.Title == "" || s.PlaylistSection == catalog.PlaylistSectionTracks)
	})
}

// When the song remains in recommendations, its first occurrence has moved.
// Rebuild playback order from the visible shelves, within the original page's bounds.
func itemsInShelfOrder(page *catalog.Page) []catalog.Item {
	eligible := make(map[string]bool, len(page.Items))
	for _, row := range page.Items {
		eligible[row.Key()] = true
	}
	rows := make([]catalog.Item, 0, len(eligible))
	for _, shelf := range page.Shelves {
		for _, row := range shelf.Items {
			key := row.Key()
			if eligible[key] {
				rows = append(rows, row)
				delete(eligible, key)
			}
		}
	}
	return rows
}
