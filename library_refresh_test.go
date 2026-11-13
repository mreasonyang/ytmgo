package main

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/egoist/mygo/ui"
	"ytmgo/internal/catalog"
)

func TestLibraryHistoryRejoinsRefresh(t *testing.T) {
	old := catalog.Page{Items: []catalog.Item{{Title: "Old playlist"}}, Continuation: "old-next"}
	fresh := catalog.Page{Items: []catalog.Item{{Title: "Fresh playlist"}}, Continuation: "new-next"}
	for _, forwardFirst := range []bool{true, false} {
		t.Run(fmt.Sprintf("forward-before-reply-%t", forwardFirst), func(t *testing.T) {
			request := &libraryRequest{}
			a := &desktop{ctx: context.Background(), historyIndex: -1, libraryPage: &old, libraryRequest: request,
				model: UIModel{Route: "home", SignedIn: true, LibraryLoaded: true, LibraryItems: old.Items}}
			a.pushHistory(true)
			a.navigate("library")
			a.travel(-1)
			if forwardFirst {
				a.travel(1)
				if a.model.Page.Continuation != "" || a.libraryRequest != request || request.pageGeneration != a.pageGeneration {
					t.Fatal("Forward did not rejoin the pending Library refresh")
				}
			}
			a.finishLibrary(request, fresh, nil)
			if !forwardFirst {
				if a.model.Route != "home" || len(a.model.Page.Items) != 0 {
					t.Fatal("Library reply replaced Home")
				}
				a.travel(1)
			}
			if a.model.Page.Items[0].Title != "Fresh playlist" || a.model.LibraryItems[0].Title != "Fresh playlist" ||
				a.model.Page.Continuation != "new-next" || len(a.history) != 2 {
				t.Fatal("Forward restored stale Library contents or pagination")
			}
		})
	}
}

func TestAccountPageLoadsAtPriorScrolledRoute(t *testing.T) {
	a := &desktop{ctx: context.Background(), historyIndex: -1,
		model: UIModel{Route: "library", SignedIn: true, pageScroll: &ui.ScrollState{Y: 200, MaxY: 480}}}
	a.pushHistory(true)
	a.clearAccountContent()
	if a.model.pageScroll != nil {
		t.Fatal("clearing the old account retained its scroll state")
	}
	// A redraw can attach a scroller before asynchronous account work finishes.
	a.model.pageScroll = &ui.ScrollState{Y: 120, MaxY: 350}
	request := &libraryRequest{}
	a.libraryRequest = request
	a.openRoute("library")
	if len(a.history) != 1 || request.pageGeneration != a.pageGeneration || !a.model.Loading {
		t.Fatal("account navigation scrolled the old page instead of loading the new account")
	}
	a.finishLibrary(request, catalog.Page{Items: []catalog.Item{{Title: "New account playlist"}}}, nil)
	if a.model.Loading || len(a.model.Page.Items) != 1 || a.model.Page.Items[0].Title != a.model.LibraryItems[0].Title {
		t.Fatal("account's Library reply did not reach both the sidebar and the main panel")
	}
}

func TestLibraryRefreshKeepsExtentAndFilter(t *testing.T) {
	items := make([]catalog.Item, 46)
	for i := range items {
		items[i] = catalog.Item{Kind: "playlist", BrowseID: fmt.Sprintf("VL%02d", i), Title: fmt.Sprintf("Saved %02d", i)}
	}
	items[25].Kind = "album"
	old := catalog.Page{Items: items, Shelves: []catalog.Shelf{{Items: items[:25]}, {Items: items[25:]}}}
	m, actions := nativeViewFixture()
	m.Route, m.SignedIn, m.LibraryLoaded, m.LibraryItems = "home", true, true, items
	request := &libraryRequest{}
	a := &desktop{ctx: context.Background(), historyIndex: -1, model: *m, libraryPage: &old, libraryRequest: request}
	actions.LoadMore, actions.Navigate = nil, a.navigate
	tt := ui.NewTester(func(c *ui.Context) { nativeView(c, &a.model, actions) }, 1280, 820)
	if err := tt.Click(actions.Tr("shellFilterAlbums")); err != nil {
		t.Fatal(err)
	}
	// Wide, the panel is the library; the page is reached by route.
	a.navigate("library")
	tt.Frame()
	source := recoverySource{
		first: func(context.Context, pageLocation) (catalog.Page, error) {
			return catalog.Page{Items: items[:25], Shelves: old.Shelves[:1], Continuation: "second"}, nil
		},
		next: func(_ context.Context, handle string) (catalog.Page, error) {
			if handle != "second" {
				t.Fatalf("unexpected continuation %q", handle)
			}
			if len(a.model.Page.Items) != 46 || len(a.model.LibraryItems) != 46 || a.model.Page.Continuation != "" {
				t.Fatal("first refresh page was exposed before reaching the loaded extent")
			}
			return catalog.Page{Items: items[25:], Shelves: old.Shelves[1:], Continuation: "new-next"}, nil
		},
	}
	page, err := loadLibraryExtent(a.ctx, source, len(old.Items))
	if err != nil {
		t.Fatal(err)
	}
	a.finishLibrary(request, page, nil)
	tt.Frame()
	_, filtered := tt.Find(actions.Tr("shellFilterClear"))
	if !filtered || len(a.model.Page.Items) != 46 || len(a.model.Page.Shelves) != 2 || a.model.Page.Continuation != "new-next" {
		t.Fatal("refresh changed the selected filter, item range, shelf layout or cursor")
	}
}

func TestLibraryExtentFailureAndEnd(t *testing.T) {
	old := catalog.Page{Items: recoveryItems(0, 4), Continuation: "old-next"}
	for _, failure := range []error{nil, catalog.ErrUnavailable, context.Canceled} {
		request := &libraryRequest{}
		a := &desktop{ctx: context.Background(), historyIndex: -1, libraryPage: &old, libraryRequest: request,
			model: UIModel{Route: "home", SignedIn: true, LibraryLoaded: true, LibraryItems: old.Items}}
		a.pushHistory(true)
		a.navigate("library")
		a.travel(-1)
		source := recoverySource{
			first: func(context.Context, pageLocation) (catalog.Page, error) {
				return catalog.Page{Items: recoveryItems(8, 1), Continuation: "second"}, nil
			},
			next: func(context.Context, string) (catalog.Page, error) {
				return catalog.Page{Items: recoveryItems(9, 1)}, failure
			},
		}
		page, err := loadLibraryExtent(a.ctx, source, len(old.Items))
		if !errors.Is(err, failure) {
			t.Fatalf("expected %v, got %v", failure, err)
		}
		a.finishLibrary(request, page, err)
		a.travel(1)
		if failure != nil {
			if len(page.Items) != 0 || len(a.model.LibraryItems) != 4 || len(a.model.Page.Items) != 4 || a.model.LibraryErrorKey == "" {
				t.Fatal("partial refresh discarded the last complete library")
			}
		} else if len(a.model.LibraryItems) != 2 || len(a.model.Page.Items) != 2 || a.model.Page.Items[0].ID != "track-8" {
			t.Fatal("genuinely shorter library did not replace the old list")
		}
		if a.model.Page.Continuation != "" {
			t.Fatal("failed or exhausted refresh exposed a stale cursor")
		}
	}
}
