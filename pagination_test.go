package main

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"

	"github.com/egoist/mygo/ui"
	"ytmgo/internal/catalog"
)

type recoverySource struct {
	first func(context.Context, pageLocation) (catalog.Page, error)
	next  func(context.Context, string) (catalog.Page, error)
}

func (s recoverySource) Home(ctx context.Context) (catalog.Page, error) {
	return s.first(ctx, pageLocation{route: "home"})
}
func (s recoverySource) Explore(ctx context.Context) (catalog.Page, error) {
	return s.first(ctx, pageLocation{route: "explore"})
}
func (s recoverySource) Search(ctx context.Context, query string) (catalog.Page, error) {
	return s.first(ctx, pageLocation{route: "search", query: query})
}
func (s recoverySource) Library(ctx context.Context) (catalog.Page, error) {
	return s.first(ctx, pageLocation{route: "library"})
}
func (s recoverySource) BrowseWithParams(ctx context.Context, id, params string) (catalog.Page, error) {
	return s.first(ctx, pageLocation{route: "browse:" + id, params: params})
}
func (s recoverySource) Continue(ctx context.Context, handle string) (catalog.Page, error) {
	return s.next(ctx, handle)
}

func recoveryItems(start, count int) []catalog.Item {
	items := make([]catalog.Item, count)
	for i := range items {
		items[i] = catalog.Item{ID: fmt.Sprintf("track-%d", start+i), Title: fmt.Sprintf("Track %d", start+i)}
	}
	return items
}

func TestExpiredPaginationRecoversEveryRouteBeyondItsLoadedExtent(t *testing.T) {
	for _, location := range []pageLocation{{route: "home"}, {route: "explore"}, {route: "library"},
		{route: "search", query: "committed search"}, {route: "browse:VLsaved", params: "saved_params"}} {
		t.Run(location.route, func(t *testing.T) {
			previous := catalog.Page{Title: "Visible title", Items: recoveryItems(0, 4), Continuation: "expired"}
			var requests []string
			source := recoverySource{
				first: func(ctx context.Context, got pageLocation) (catalog.Page, error) {
					if got != location {
						t.Fatal("recovery changed the original route, query or browse parameters")
					}
					return catalog.Page{Title: "Fresh greeting", Creator: "Creator", Items: recoveryItems(0, 2), Continuation: "fresh-2"}, nil
				},
				next: func(ctx context.Context, handle string) (catalog.Page, error) {
					requests = append(requests, handle)
					switch handle {
					case "expired":
						return catalog.Page{}, catalog.ErrExpiredContinuation
					case "fresh-2":
						return catalog.Page{Items: recoveryItems(2, 2), Continuation: "fresh-3"}, nil
					case "fresh-3":
						return catalog.Page{Items: recoveryItems(4, 2), Continuation: "fresh-4"}, nil
					default:
						t.Fatal("recovery fetched beyond the next page")
						return catalog.Page{}, catalog.ErrInvalidID
					}
				},
			}
			page, replace, err := continuePage(context.Background(), source, location, previous)
			if err != nil || !replace || len(page.Items) != 6 || page.Title != previous.Title || page.Creator != "Creator" ||
				page.Continuation != "fresh-4" || !reflect.DeepEqual(requests, []string{"expired", "fresh-2", "fresh-3"}) {
				t.Fatal("recovery did not restore the loaded range and the next page exactly once")
			}
			for i, item := range page.Items {
				if item.ID != fmt.Sprintf("track-%d", i) {
					t.Fatal("replay duplicated or reordered tracks")
				}
			}
		})
	}
}

func TestPaginationRecoveryHandlesChangedSessionEmptyPagesAndShorterLists(t *testing.T) {
	for _, shortened := range []bool{false, true} {
		previous := catalog.Page{Title: "Playlist", Items: recoveryItems(0, 4), Continuation: "invalidated"}
		calls := 0
		source := recoverySource{
			first: func(context.Context, pageLocation) (catalog.Page, error) {
				if shortened {
					return catalog.Page{Items: recoveryItems(0, 2)}, nil
				}
				return catalog.Page{Continuation: "empty"}, nil
			},
			next: func(ctx context.Context, handle string) (catalog.Page, error) {
				calls++
				if handle == "invalidated" {
					return catalog.Page{}, catalog.ErrSessionChanged
				}
				return catalog.Page{Items: recoveryItems(0, 6)}, nil
			},
		}
		page, replace, err := continuePage(context.Background(), source, pageLocation{route: "home"}, previous)
		want, wantCalls := 6, 2
		if shortened {
			want, wantCalls = 2, 1
		}
		if err != nil || !replace || len(page.Items) != want || calls != wantCalls || page.Continuation != "" {
			t.Fatal("recovery could not finish an empty page or a shortened list")
		}
	}
}

func TestInvalidContinuationResponseRecoversHomeShelves(t *testing.T) {
	previous := catalog.Page{Title: "Visible greeting", Items: recoveryItems(0, 4), Shelves: make([]catalog.Shelf, 4), Continuation: "rejected"}
	var requests []string
	source := recoverySource{
		first: func(_ context.Context, location pageLocation) (catalog.Page, error) {
			if location.route != "home" {
				t.Fatal("recovery changed the displayed route")
			}
			requests = append(requests, "home")
			return catalog.Page{Title: "Fresh greeting", Items: recoveryItems(0, 2), Shelves: make([]catalog.Shelf, 2), Continuation: "fresh"}, nil
		},
		next: func(_ context.Context, handle string) (catalog.Page, error) {
			requests = append(requests, handle)
			if handle == "rejected" {
				return catalog.Page{}, fmt.Errorf("wrapped: %w", catalog.ErrInvalidResponse)
			}
			return catalog.Page{Items: recoveryItems(2, 3), Shelves: make([]catalog.Shelf, 3)}, nil
		},
	}
	page, replace, err := continuePage(context.Background(), source, pageLocation{route: "home"}, previous)
	if err != nil || !replace || page.Title != previous.Title || len(page.Items) != 5 || len(page.Shelves) != 5 || page.Continuation != "" ||
		!reflect.DeepEqual(requests, []string{"rejected", "home", "fresh"}) {
		t.Fatal("unusable continuation did not rebuild Home through its loaded extent")
	}
}

func TestInvalidContinuationRecoveryStopsOnRepeatedInvalidResponse(t *testing.T) {
	for _, freshFirstPageFails := range []bool{false, true} {
		m, _ := nativeViewFixture()
		m.Page.Continuation = "rejected"
		previous := m.Page
		firsts, nexts := 0, 0
		source := recoverySource{
			first: func(context.Context, pageLocation) (catalog.Page, error) {
				firsts++
				if freshFirstPageFails {
					return catalog.Page{}, catalog.ErrInvalidResponse
				}
				return catalog.Page{Continuation: "fresh"}, nil
			},
			next: func(context.Context, string) (catalog.Page, error) {
				nexts++
				return catalog.Page{}, catalog.ErrInvalidResponse
			},
		}
		page, replace, err := continuePage(context.Background(), source, pageLocation{route: "home"}, previous)
		wantNexts := 2
		if freshFirstPageFails {
			wantNexts = 1
		}
		if !errors.Is(err, catalog.ErrInvalidResponse) || firsts != 1 || nexts != wantNexts {
			t.Fatal("repeated invalid response was hidden or recovery exceeded one replay")
		}
		a := &desktop{model: *m, historyIndex: -1, pageGeneration: 3}
		a.finishPageContinuation(3, page, replace, err)
		if !reflect.DeepEqual(a.model.Page, previous) || a.model.Current != m.Current || a.model.Position != m.Position || !a.model.Playing || a.model.Error == "" {
			t.Fatal("failed replay discarded existing content, playback or its error")
		}
	}
}

func TestPaginationRecoveryFailuresKeepContentAndPlayback(t *testing.T) {
	m, _ := nativeViewFixture()
	m.Page.Continuation = "expired"
	previous := m.Page
	for _, failure := range []error{context.DeadlineExceeded, context.Canceled, catalog.ErrUnavailable} {
		source := recoverySource{
			first: func(context.Context, pageLocation) (catalog.Page, error) { return catalog.Page{}, failure },
			next: func(context.Context, string) (catalog.Page, error) {
				return catalog.Page{}, catalog.ErrExpiredContinuation
			},
		}
		page, replace, err := continuePage(context.Background(), source, pageLocation{route: "home"}, previous)
		if !errors.Is(err, failure) {
			t.Fatal("recovery hid its network/cancellation failure")
		}
		a := &desktop{model: *m, historyIndex: -1, pageGeneration: 3}
		a.finishPageContinuation(3, page, replace, err)
		if !reflect.DeepEqual(a.model.Page, previous) || a.model.Current != m.Current || a.model.Position != m.Position || !a.model.Playing {
			t.Fatal("failed replay discarded usable content or changed playback")
		}
		if nativeContinuationExpired(&a.model) {
			t.Fatal("a temporary recovery failure asked the reader to discard the list")
		}
	}
}

func TestPaginationRecoveryDoesNotLoopOnEmptyContinuations(t *testing.T) {
	source := recoverySource{
		first: func(context.Context, pageLocation) (catalog.Page, error) {
			return catalog.Page{Continuation: "empty"}, nil
		},
		next: func(ctx context.Context, handle string) (catalog.Page, error) {
			if handle == "expired" {
				return catalog.Page{}, catalog.ErrExpiredContinuation
			}
			return catalog.Page{Continuation: "empty"}, nil
		},
	}
	_, _, err := continuePage(context.Background(), source, pageLocation{route: "home"}, catalog.Page{Continuation: "expired"})
	if !errors.Is(err, catalog.ErrInvalidResponse) {
		t.Fatal("empty recovery did not terminate within its replay budget")
	}
}

func TestRecoveredPaginationKeepsNativeScrollAndPlayback(t *testing.T) {
	m, actions := nativeViewFixture()
	m.Route = "browse:VLlong"
	m.Page = nativeContinuationPages(m)[m.Route]
	m.Page.Continuation = "old"
	a := &desktop{model: *m, historyIndex: -1, pageGeneration: 2}
	loads := 0
	actions.LoadMore = func() { loads++; a.model.Loading = true }
	tt := ui.NewTester(func(c *ui.Context) { nativeView(c, &a.model, actions) }, 1280, 800)
	tt.Frame()
	tt.Frame()
	nativeScrollContent(t, tt, 1e6)
	if loads != 1 || a.model.pageScroll == nil {
		t.Fatal("initial pagination did not start")
	}
	scroll, y := a.model.pageScroll, a.model.pageScroll.Y
	recovered := a.model.Page
	recovered.Items = append(append([]catalog.Item(nil), recovered.Items...), recoveryItems(60, 20)...)
	recovered.Continuation = "fresh"
	a.finishPageContinuation(2, recovered, true, nil)
	tt.Frame()
	tt.Frame()
	if a.model.pageScroll != scroll || a.model.pageScroll.Y != y || len(a.model.Page.Items) != 80 || loads != 1 ||
		a.model.Current != m.Current || !a.model.Playing || a.model.Position != m.Position || tt.HasText(actions.Tr("nativeCatalogPageExpired")) {
		t.Fatal("successful replay reset scrolling, playback or requested another page too early")
	}
	a.finishPageContinuation(1, catalog.Page{Title: "stale"}, true, nil)
	if a.model.Page.Title != recovered.Title {
		t.Fatal("stale recovery replaced a newer page")
	}
}
