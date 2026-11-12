package catalog

import (
	"context"
	"strings"
)

// CollectionTracks preserves the track-list order, excluding recommendation shelves.
func (p Page) CollectionTracks() []Item {
	var items []Item
	marked := false
	for _, shelf := range p.Shelves {
		if shelf.PlaylistSection != PlaylistSectionUnknown {
			marked = true
		}
		if shelf.PlaylistSection == PlaylistSectionTracks {
			items = append(items, shelf.Items...)
		}
	}
	if !marked {
		switch {
		case len(p.Shelves) == 0:
			items = p.Items
		case p.Shelves[0].Title == "":
			// Album responses can use a plain, untitled music shelf.
			items = p.Shelves[0].Items
		}
	}
	tracks := make([]Item, 0, len(items))
	for _, item := range items {
		if item.ID != "" {
			tracks = append(tracks, item)
		}
	}
	return tracks
}

// CollectionTracks uses fresh cursors, stopping before the outer recommendations.
// A repeated upstream cursor is detected even if its local handle has changed.
func (c *Client) CollectionTracks(ctx context.Context, id, params string) ([]Item, error) {
	payload, err := browsePayload(id, params)
	if err != nil {
		return nil, err
	}
	var album *albumMetadata
	if strings.HasPrefix(id, "MPRE") {
		album = &albumMetadata{}
	}
	page, err := c.loadPage(ctx, "browse", payload, "", pageCursor{}, "", album, nil, true)
	if err != nil {
		return nil, err
	}
	tracks := page.CollectionTracks()
	seen := make(map[string]bool)
	for page.Continuation != "" {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		c.mu.RLock()
		next, ok := c.continuations[page.Continuation]
		c.mu.RUnlock()
		if !ok {
			return nil, ErrExpiredContinuation
		}
		if next.cursor.section == PlaylistSectionExtras || (!next.cursor.trackShelf && next.cursor.section != PlaylistSectionTracks) {
			break
		}
		if seen[next.cursor.token] {
			return nil, ErrInvalidResponse
		}
		seen[next.cursor.token] = true
		page, err = c.Continue(ctx, page.Continuation)
		if err != nil {
			return nil, err
		}
		tracks = append(tracks, page.CollectionTracks()...)
	}
	return tracks, nil
}
