package main

import (
	"context"
	"testing"

	"ytmgo/internal/catalog"
)

func TestFailedNavigationRetryPreservesOriginAndForwardHistory(t *testing.T) {
	a := &desktop{ctx: context.Background(), historyIndex: -1,
		model: UIModel{SignedIn: true, Route: "home", Page: catalog.Page{Title: "Home"}}}
	a.pushHistory(true)
	first := &libraryRequest{cancel: func() {}}
	a.libraryRequest = first
	a.navigate("library")
	if len(a.history) != 2 || !a.model.CanBack {
		t.Fatal("a pending destination hid its origin")
	}
	a.finishLibrary(first, catalog.Page{}, catalog.ErrUnavailable)
	if !a.history[1].needsReload || a.history[0].route != "home" {
		t.Fatal("failed destination replaced its origin")
	}
	retry := &libraryRequest{cancel: func() {}}
	a.libraryRequest = retry
	a.reload()
	a.finishLibrary(retry, catalog.Page{Items: []catalog.Item{{BrowseID: "VLsaved", Title: "Saved", Kind: "playlist"}}}, nil)
	if len(a.history) != 2 || !a.model.CanBack || a.history[1].needsReload {
		t.Fatal("successful Retry did not settle the existing destination")
	}
	a.travel(-1)
	if a.model.Route != "home" || a.model.Page.Title != "Home" || !a.model.CanForward {
		t.Fatal("Back lost the origin")
	}
	a.travel(1)
	if a.model.Route != "library" || len(a.model.Page.Items) != 1 || a.model.Page.Items[0].Title != "Saved" {
		t.Fatal("Forward lost the recovered page")
	}
}

func TestInvalidatedHistoryReloadsAndCancelsPreviousPagination(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	pending := &libraryRequest{cancel: func() {}}
	a := &desktop{ctx: context.Background(), historyIndex: 1, pageGeneration: 4, pageCancel: cancel, libraryRequest: pending,
		history: []historyEntry{{route: "library", page: catalog.Page{Continuation: "old-language-cursor"}}, {route: "home"}},
		model:   UIModel{SignedIn: true, Route: "home"}}
	a.invalidateHistory()
	a.travel(-1)
	if ctx.Err() == nil || !a.model.Loading || a.model.Page.Continuation != "" || pending.pageGeneration != a.pageGeneration {
		t.Fatal("Back reused an invalidated page or kept the previous pagination alive")
	}
	if len(a.history) != 2 || a.historyIndex != 0 || !a.model.CanForward {
		t.Fatal("refreshing a historical destination changed history ordering")
	}
	a.finishLibrary(pending, catalog.Page{Items: []catalog.Item{{Title: "Fresh language", BrowseID: "VLfresh"}}, Continuation: "fresh-cursor"}, nil)
	if a.history[0].needsReload || a.model.Page.Continuation != "fresh-cursor" {
		t.Fatal("fresh historical page did not receive its new pagination")
	}
}

func TestRatingUpdatesSurviveHistoryAndLibraryRestoration(t *testing.T) {
	old := func() catalog.Item { return catalog.Item{ID: "abc123DEF_-", Title: "Synthetic", LikeKnown: true} }
	page := func() catalog.Page {
		return catalog.Page{Items: []catalog.Item{old()}, Shelves: []catalog.Shelf{{Items: []catalog.Item{old()}}}}
	}
	library := page()
	a := &desktop{historyIndex: 1, libraryPage: &library,
		history: []historyEntry{{route: "browse:VLsaved", page: page()}, {route: "search", page: page()}},
		model:   UIModel{Route: "search", Current: old(), Queue: []catalog.Item{old()}, Page: page(), LibraryItems: []catalog.Item{old()}}}
	for _, liked := range []bool{true, false} {
		a.setLiked("abc123DEF_-", liked)
		a.travel(-1)
		for _, item := range []catalog.Item{a.model.Page.Items[0], a.model.Page.Shelves[0].Items[0], a.model.Current,
			a.model.Queue[0], a.model.LibraryItems[0], a.libraryPage.Items[0], a.history[1].page.Items[0]} {
			if !item.LikeKnown || item.Liked != liked {
				t.Fatal("navigation restored a rating older than the confirmed mutation")
			}
		}
		a.travel(1)
	}
}

func TestPaginationAndRetryKeepCommittedSearchAndBrowseParameters(t *testing.T) {
	for _, location := range []pageLocation{{route: "search", query: "submitted"}, {route: "browse:VLsaved", params: "original_params"}} {
		a := &desktop{historyIndex: 0, pageGeneration: 1,
			history: []historyEntry{{route: location.route, query: location.query, params: location.params}},
			model:   UIModel{Route: location.route, Query: "unsubmitted draft", Page: catalog.Page{Continuation: "next"}}}
		a.finishContinuation(1, catalog.Page{Items: recoveryItems(0, 2), Continuation: "next-2"}, nil)
		if a.currentPageLocation() != location {
			t.Fatal("pagination overwrote the displayed page's origin with unsubmitted input")
		}
	}
}
