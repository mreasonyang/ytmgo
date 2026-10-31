package main

import (
	"fmt"
	"testing"

	"github.com/egoist/mygo/ui"
	"ytmgo/internal/catalog"
)

// nativeContinuationPages are the routes nativeContent shows, each with as
// much of its kind of content as fills several screens.
func nativeContinuationPages(m *UIModel) map[string]catalog.Page {
	songs := make([]catalog.Item, 0, 60)
	for i := range 60 {
		songs = append(songs, catalog.Item{ID: fmt.Sprintf("long-%d", i), Title: fmt.Sprintf("Track %d", i), Artist: "Daft Punk", Kind: "song", Artwork: "art-0"})
	}
	cards := make([]catalog.Item, 0, 40)
	for i := range 40 {
		cards = append(cards, catalog.Item{Title: fmt.Sprintf("Album %d", i), Artist: "Daft Punk", Kind: "album", BrowseID: fmt.Sprintf("long-album-%d", i), Artwork: "art-1"})
	}
	shelves := make([]catalog.Shelf, 0, 12)
	for i := range 12 {
		items := cards[:8]
		if i%2 == 0 {
			items = m.Queue
		}
		shelves = append(shelves, catalog.Shelf{Title: fmt.Sprintf("Shelf %d", i), Items: items})
	}
	return map[string]catalog.Page{
		"home":            {Title: "Good evening", Shelves: shelves},
		"explore":         {Title: "Explore", Shelves: shelves},
		"library":         {Title: "Library", Items: cards},
		"search":          {Title: "daft punk", Items: songs},
		"browse:VLlong":   {Title: "Long playlist", Artwork: "art-2", Items: songs},
		"browse:UCartist": {Title: "Daft Punk", Artwork: "art-3", Shelves: shelves},
	}
}

func nativeContinuationTester(t *testing.T, route string) (*UIModel, *ui.Tester, *[]string, func(catalog.Page, error)) {
	t.Helper()
	m, a := nativeViewFixture()
	m.SignedIn = true
	m.Route = route
	m.Page = nativeContinuationPages(m)[route]
	m.Page.Continuation = "page-2"
	requests := []string{}
	// As desktop.loadMore: one request at a time, ignored without a token.
	a.LoadMore = func() {
		if m.Loading || m.Page.Continuation == "" {
			t.Fatalf("%s: LoadMore while loading=%v token=%q", route, m.Loading, m.Page.Continuation)
		}
		requests = append(requests, m.Page.Continuation)
		m.Loading, m.Error, m.ErrorScope = true, "", ""
	}
	tt := ui.NewTester(func(c *ui.Context) { nativeView(c, m, a) }, 1280, 800)
	finish := func(page catalog.Page, err error) {
		m.Loading = false
		if err != nil {
			m.Error, m.ErrorScope = err.Error(), errorCatalog
		} else {
			m.Page.Items = append(m.Page.Items, page.Items...)
			m.Page.Shelves = append(m.Page.Shelves, page.Shelves...)
			m.Page.Continuation = page.Continuation
		}
		tt.Frame()
		tt.Frame()
	}
	tt.Frame()
	tt.Frame()
	return m, tt, &requests, finish
}

func nativeScrollContent(t *testing.T, tt *ui.Tester, dy float32) {
	t.Helper()
	r, ok := tt.Find("Content")
	if !ok {
		t.Fatal("content scroller missing")
	}
	tt.Scroll(r.X+r.W/2, r.Y+r.H/2, 0, dy)
	tt.Frame()
	tt.Frame()
}

func TestNativeContinuationLoadsNearEndOnEveryPage(t *testing.T) {
	for route := range nativeContinuationPages(&UIModel{}) {
		t.Run(route, func(t *testing.T) {
			m, tt, requests, finish := nativeContinuationTester(t, route)
			if tt.HasText("Load more") {
				t.Fatal("Load more button still shown")
			}
			if len(*requests) != 0 {
				t.Fatalf("loaded %v at the top of the page", *requests)
			}
			nativeScrollContent(t, tt, 400)
			if len(*requests) != 0 {
				t.Fatalf("loaded %v far from the end", *requests)
			}
			nativeScrollContent(t, tt, 1e6)
			if len(*requests) != 1 || (*requests)[0] != "page-2" {
				t.Fatalf("near the end requested %v, want [page-2]", *requests)
			}
			if _, ok := tt.Find("Loading…"); !ok && !tt.HasText("Loading…") {
				t.Fatal("no spinner while the next page loads")
			}
			nativeSnapshot(t, tt, "1280-continuation-"+route)
			// Frames and scrolling while it loads must not ask again.
			nativeScrollContent(t, tt, 50)
			nativeScrollContent(t, tt, 1e6)
			if len(*requests) != 1 {
				t.Fatalf("duplicate requests while loading: %v", *requests)
			}
			// A full page lands below the reader: nothing more until they near the new end.
			more := nativeContinuationPages(m)[route]
			more.Continuation = "page-3"
			finish(more, nil)
			if len(*requests) != 1 {
				t.Fatalf("requested %v before the reader reached the new end", *requests)
			}
			nativeScrollContent(t, tt, 1e6)
			if len(*requests) != 2 || (*requests)[1] != "page-3" {
				t.Fatalf("second page requested %v, want page-3", *requests)
			}
			// The last page has no token: no more requests, footer gone.
			finish(catalog.Page{Items: m.Queue[:1]}, nil)
			nativeScrollContent(t, tt, 1e6)
			if len(*requests) != 2 {
				t.Fatalf("requested %v after the last page", *requests)
			}
			if tt.HasText("Loading…") {
				t.Fatal("spinner left after the last page")
			}
		})
	}
}

func TestNativeContinuationFillsShortPage(t *testing.T) {
	m, tt, requests, finish := nativeContinuationTester(t, "search")
	_ = m
	m.Page.Items = m.Page.Items[:2]
	tt.Frame()
	tt.Frame()
	if len(*requests) != 1 {
		t.Fatalf("a page shorter than the window requested %v, want one page", *requests)
	}
	finish(catalog.Page{Items: m.Queue[:2], Continuation: "page-3"}, nil)
	finish(catalog.Page{Items: m.Queue[:2]}, nil)
	if len(*requests) != 2 || (*requests)[1] != "page-3" {
		t.Fatalf("short page requested %v, want [page-2 page-3]", *requests)
	}
}

func TestNativeContinuationFailureWaitsForRetry(t *testing.T) {
	m, tt, requests, finish := nativeContinuationTester(t, "browse:VLlong")
	nativeScrollContent(t, tt, 1e6)
	finish(catalog.Page{}, fmt.Errorf("network down"))
	for range 4 {
		tt.Frame()
	}
	if len(*requests) != 1 {
		t.Fatalf("failed page retried by itself: %v", *requests)
	}
	r, ok := tt.Find("network down")
	if !ok || r.Y < 400 {
		t.Fatalf("continuation error not shown at the end of the page: %+v %v", r, ok)
	}
	nativeSnapshot(t, tt, "1280-continuation-error")
	content, _ := tt.Find("Content")
	// Find returns the button's text, whose font height is smaller than the
	// padded button. Check visibility here; the click below verifies the action.
	if retry, ok := tt.Find("Retry"); !ok || retry.H <= 0 || retry.Y < content.Y || retry.Y+retry.H+8 > content.Y+content.H+1 {
		t.Fatalf("Retry %+v not fully inside the content %+v", retry, content)
	}
	if err := tt.Click("Retry"); err != nil {
		t.Fatal(err)
	}
	if len(*requests) != 2 || (*requests)[1] != "page-2" || len(m.Page.Items) != 60 {
		t.Fatalf("Retry requested %v with %d items, want the same page kept", *requests, len(m.Page.Items))
	}
	// Scrolling away and back retries too.
	finish(catalog.Page{}, fmt.Errorf("network down"))
	nativeScrollContent(t, tt, -2000)
	nativeScrollContent(t, tt, 1e6)
	if len(*requests) != 3 {
		t.Fatalf("returning to the end did not retry: %v", *requests)
	}
}
