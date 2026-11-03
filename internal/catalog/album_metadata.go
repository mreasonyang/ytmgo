package catalog

// Album rows omit metadata shared with their header. Keep that context local
// to the canonical album browse and its own track-list continuation.
type albumMetadata struct {
	title, artist, artwork string
}

func completeAlbumTracks(page *Page, context albumMetadata) albumMetadata {
	if page.Title != "" {
		context.title = page.Title
	}
	if page.Creator != "" {
		context.artist = page.Creator
	}
	if page.Artwork != "" {
		context.artwork = page.Artwork
	}
	// The first untitled shelf is the album's track list. Named shelves hold
	// recommendations/other releases and must retain their own metadata.
	if len(page.Shelves) == 0 || page.Shelves[0].Title != "" {
		return context
	}
	tracks := make(map[string]bool)
	for _, item := range page.Shelves[0].Items {
		if item.ID != "" {
			tracks[item.ID] = true
		}
	}
	complete := func(item *Item) {
		if !tracks[item.ID] {
			return
		}
		if item.Artwork == "" {
			item.Artwork = context.artwork
		}
		if item.Artist == "" {
			item.Artist = context.artist
		}
		if item.Album == "" {
			item.Album = context.title
		}
	}
	for i := range page.Items {
		complete(&page.Items[i])
	}
	for i := range page.Shelves {
		for j := range page.Shelves[i].Items {
			complete(&page.Shelves[i].Items[j])
		}
	}
	return context
}
