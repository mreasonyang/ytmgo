package main

import (
	"context"
	"path/filepath"
	"strings"

	"ytmgo/internal/catalog"
	"ytmgo/internal/downloads"
)

func (a *desktop) initDownloads(dataDir string) {
	if a.player == nil {
		return
	}
	m, err := downloads.New(a.ctx, filepath.Join(dataDir, "downloads-v1"), nativeAccountSecrets{}, a.player.DownloadSource, a.downloadExtras)
	if err != nil {
		a.model.DownloadNotice = "nativeDownloadStorageError"
		return
	}
	a.downloads = m
	a.player.AttachDownloads(m)
	a.art.pinned = m.Cover
	a.refreshDownloads()
}

func (a *desktop) selectDownloads(scope string, forget bool) {
	if a.downloads == nil || scope == "" {
		return
	}
	if scope != a.downloads.Scope() && a.cancelCollections != nil {
		a.cancelCollections()
		a.collectionCtx, a.cancelCollections, a.pendingCollections = nil, nil, 0
		a.model.DownloadNotice, a.model.collectionDownloads = "", nil
	}
	if err := a.downloads.Switch(scope, forget); err != nil {
		a.model.DownloadNotice = "nativeDownloadStorageError"
	}
	a.refreshDownloads()
}

func (a *desktop) downloadsOnline(loggedIn bool, scope string) {
	if a.downloads == nil {
		return
	}
	mine := a.downloads.Scope()
	a.downloads.Online((loggedIn && scope == mine) || (!loggedIn && mine == downloads.Guest))
}

func (a *desktop) refreshDownloads() {
	if a.downloads != nil {
		a.model.Downloads = a.downloads.Snapshot()
		a.model.DownloadWaiting = false
		canTransfer := a.downloads.CanTransfer()
		a.model.downloadStates = make(map[string]downloads.State, len(a.model.Downloads))
		for _, j := range a.model.Downloads {
			a.model.downloadStates[j.Item.ID] = j.State
			if j.State == downloads.Queued && !canTransfer {
				a.model.DownloadWaiting = true
			}
		}
	}
	if a.model.Route != "downloads" {
		return
	}
	page := catalog.Page{Title: a.tr("nativeDownloads")}
	for _, entry := range a.model.Downloads {
		if entry.State == downloads.Complete {
			it := entry.Item
			page.Items = append(page.Items, it)
		}
	}
	a.model.Page = page
}

// Downloading stays where it was asked for: the control that asked shows
// the progress, and the library gains its Downloads entry.
func (a *desktop) enqueueDownloads(items []catalog.Item) {
	list := make([]catalog.Item, 0, len(items))
	for _, it := range items {
		it.Artwork = artworkRequest(it.Artwork)
		list = append(list, it)
	}
	a.model.DownloadNotice = ""
	err := a.downloads.Enqueue(a.downloads.Scope(), list, a.model.AudioQuality)
	if err == nil {
		err = a.resumeDownloads(list)
	}
	if err != nil {
		a.model.DownloadNotice = "nativeDownloadStorageError"
	}
	a.refreshDownloads()
}

// resumeDownloads restarts the paused and failed among items: asking to
// download them again means now.
func (a *desktop) resumeDownloads(items []catalog.Item) error {
	for _, it := range items {
		if state := a.model.downloadStates[it.ID]; state == downloads.Paused || state == downloads.Failed {
			if err := a.downloads.SetPaused(it.ID, false); err != nil {
				return err
			}
		}
	}
	return nil
}

func (a *desktop) downloadItem(it catalog.Item) { a.enqueueDownloads([]catalog.Item{it}) }

func (a *desktop) downloadPage() {
	if a.collectionCtx == nil {
		a.collectionCtx, a.cancelCollections = context.WithCancel(a.ctx)
	}
	ctx := a.collectionCtx
	a.pendingCollections++
	location, scope, q := a.currentPageLocation(), a.downloads.Scope(), a.model.AudioQuality
	route := a.model.Route
	a.model.DownloadNotice = "nativeDownloadPreparing"
	if a.model.collectionDownloads == nil {
		a.model.collectionDownloads = map[string]string{}
	}
	a.model.collectionDownloads[route] = "nativeDownloadPreparing"
	go func() {
		items, err := a.catalog.CollectionTracks(ctx, strings.TrimPrefix(location.route, "browse:"), location.params)
		if ctx.Err() != nil {
			return
		}
		for i := range items {
			items[i].Artwork = artworkRequest(items[i].Artwork)
		}
		if err == nil {
			err = a.downloads.Enqueue(scope, items, q)
		}
		a.post(func() {
			if ctx.Err() != nil || a.downloads.Scope() != scope {
				return
			}
			a.pendingCollections--
			if a.pendingCollections == 0 && a.model.DownloadNotice == "nativeDownloadPreparing" {
				a.model.DownloadNotice = ""
			}
			delete(a.model.collectionDownloads, route)
			if err == nil {
				a.refreshDownloads()
				if err = a.resumeDownloads(items); err != nil {
					a.model.DownloadNotice = "nativeDownloadStorageError"
				}
			} else {
				a.model.DownloadNotice = "nativeDownloadCollectionError"
				a.model.collectionDownloads[route] = "nativeDownloadCollectionError"
			}
			a.refreshDownloads()
		})
	}()
}

func (a *desktop) pauseDownload(id string, pause bool) {
	a.model.DownloadNotice = ""
	if err := a.downloads.SetPaused(id, pause); err != nil {
		a.model.DownloadNotice = "nativeDownloadStorageError"
	}
	a.refreshDownloads()
}
func (a *desktop) removeDownload(id string) {
	a.model.DownloadNotice = ""
	if a.downloads.Has(id) && a.model.Current.ID == id {
		a.cancelResume()
		if a.playCancel != nil {
			a.playCancel()
		}
		if a.seekCancel != nil {
			a.seekCancel()
		}
		a.playGeneration++
		a.loadPending, a.model.PlaybackLoading, a.model.Playing = false, false, false
		_ = a.player.Stop()
		a.restoredTrack = true
		a.publishPausedTrack()
	}
	if a.preparedID == id {
		if a.prepareCancel != nil {
			a.prepareCancel()
		}
		a.preparedID = ""
		a.player.ClearPrepared()
	}
	if err := a.downloads.Remove(id); err != nil {
		a.model.DownloadNotice = "nativeDownloadStorageError"
	}
	a.refreshDownloads()
}

func (a *desktop) trackLyrics(ctx context.Context, id string) (catalog.LyricsResult, error) {
	if a.downloads != nil {
		if lyrics, ok := a.downloads.Lyrics(id); ok && lyrics.Text != "" {
			return lyrics, nil
		}
	}
	return a.catalog.LyricsWithTiming(ctx, id)
}

func (a *desktop) downloadExtras(ctx context.Context, it catalog.Item) (catalog.LyricsResult, []byte) {
	lyricsDone := make(chan catalog.LyricsResult, 1)
	go func() { lyrics, _ := a.catalog.LyricsWithTiming(ctx, it.ID); lyricsDone <- lyrics }()
	var cover []byte
	if it.Artwork != "" && artworkDiskKey(it.Artwork) != "" {
		data, h := a.art.download(ctx, artDemand{artRequest: artRequest{url: it.Artwork, edge: artworkEdge}, priority: artSecondary})
		if artworkPublicResponse(h) {
			cover = data
		}
	}
	select {
	case lyrics := <-lyricsDone:
		return lyrics, cover
	case <-ctx.Done():
		return catalog.LyricsResult{}, cover
	}
}
