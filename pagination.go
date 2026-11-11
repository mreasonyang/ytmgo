package main

import (
	"context"
	"errors"
	"strings"

	"ytmgo/internal/catalog"
)

type pageSource interface {
	Home(context.Context) (catalog.Page, error)
	Explore(context.Context) (catalog.Page, error)
	Search(context.Context, string) (catalog.Page, error)
	BrowseWithParams(context.Context, string, string) (catalog.Page, error)
	Library(context.Context) (catalog.Page, error)
	Continue(context.Context, string) (catalog.Page, error)
}

type pageLocation struct{ route, query, params string }

const maxReplayPages = 256

// The search field can contain an unsubmitted draft, and focusing it clears
// browseParams. Recovery must use the request that produced the displayed page.
func (a *desktop) currentPageLocation() pageLocation {
	if a.historyIndex >= 0 && a.historyIndex < len(a.history) {
		entry := a.history[a.historyIndex]
		if entry.route == a.model.Route {
			return pageLocation{entry.route, entry.query, entry.params}
		}
	}
	return pageLocation{a.model.Route, a.model.Query, a.browseParams}
}

func (location pageLocation) load(ctx context.Context, source pageSource) (catalog.Page, error) {
	switch {
	case location.route == "home":
		return source.Home(ctx)
	case location.route == "explore":
		return source.Explore(ctx)
	case location.route == "search":
		return source.Search(ctx, location.query)
	case location.route == "library":
		return source.Library(ctx)
	case strings.HasPrefix(location.route, "browse:"):
		return source.BrowseWithParams(ctx, strings.TrimPrefix(location.route, "browse:"), location.params)
	default:
		return catalog.Page{}, catalog.ErrInvalidID
	}
}

// A visible page can outlive its bounded, identity-scoped cursor cache. Replay
// fresh pages until the old extent and one more page are available. The caller
// keeps the old content and scroll container throughout; partial/failed replay
// is never published. Network deadlines and this bound also stop empty loops.
func continuePage(ctx context.Context, source pageSource, location pageLocation, previous catalog.Page) (catalog.Page, bool, error) {
	page, err := source.Continue(ctx, previous.Continuation)
	if !errors.Is(err, catalog.ErrExpiredContinuation) && !errors.Is(err, catalog.ErrSessionChanged) {
		return page, false, err
	}
	page, err = location.load(catalog.Refresh(ctx), source)
	if err == nil {
		page, err = extendPage(ctx, source, page, maxReplayPages, func(p catalog.Page) bool {
			return len(p.Items) >= len(previous.Items) && len(p.Shelves) >= len(previous.Shelves) &&
				(len(p.Items) > len(previous.Items) || len(p.Shelves) > len(previous.Shelves))
		})
	}
	if err != nil {
		// Another invalidation can be retried as a fresh recovery.
		if errors.Is(err, catalog.ErrExpiredContinuation) || errors.Is(err, catalog.ErrSessionChanged) {
			err = catalog.ErrUnavailable
		}
		return catalog.Page{}, true, err
	}
	// The title is part of the scroll container's identity. A Home greeting
	// changing while idle must not send the reader back to the top.
	page.Title = previous.Title
	return page, true, nil
}

// The displayed library and its sidebar filters share the last loaded extent.
// Publish a refresh once it covers that extent or reaches the end of the library.
func loadLibraryExtent(ctx context.Context, source pageSource, items int) (catalog.Page, error) {
	page, err := source.Library(ctx)
	if err != nil {
		return catalog.Page{}, err
	}
	// Even one item per page suffices within items requests. Allow the same
	// budget as cursor recovery for empty intermediate pages.
	return extendPage(ctx, source, page, items+maxReplayPages, func(p catalog.Page) bool { return len(p.Items) >= items })
}

func extendPage(ctx context.Context, source pageSource, page catalog.Page, limit int, enough func(catalog.Page) bool) (catalog.Page, error) {
	for fetched := 1; page.Continuation != "" && !enough(page); fetched++ {
		if fetched >= limit {
			return catalog.Page{}, catalog.ErrInvalidResponse
		}
		if err := ctx.Err(); err != nil {
			return catalog.Page{}, err
		}
		next, err := source.Continue(ctx, page.Continuation)
		if err != nil {
			return catalog.Page{}, err
		}
		page.Items = append(page.Items, next.Items...)
		page.Shelves = append(page.Shelves, next.Shelves...)
		page.Continuation = next.Continuation
	}
	return page, nil
}
