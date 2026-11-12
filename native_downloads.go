package main

import (
	"fmt"

	"github.com/egoist/mygo/ui"
	"ytmgo/internal/downloads"
)

func downloadSize(n int64) string {
	switch {
	case n >= 1<<30:
		return fmt.Sprintf("%.1f GiB", float64(n)/(1<<30))
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MiB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.0f KiB", float64(n)/(1<<10))
	default:
		return fmt.Sprintf("%d B", n)
	}
}

func nativeCurrentDownload(c *ui.Context, m *UIModel, a ViewActions, p nativePalette, size float32) {
	if a.Download == nil || m.Current.ID == "" {
		return
	}
	state := m.downloadStates[m.Current.ID]
	icon, label := "download", "nativeDownloadTrack"
	if state != "" {
		label = "nativeDownloads"
	}
	if state == downloads.Complete {
		icon = "check"
	}
	if nativeIconButton(c, icon, a.Tr(label)+" · "+m.Current.Title, p, state != "", size).Clicked() {
		if state == "" {
			a.Download(m.Current)
		} else {
			a.Navigate("downloads")
		}
	}
}

func nativeDownloads(c *ui.Context, m *UIModel, a ViewActions, p nativePalette) {
	if m.DownloadNotice != "" {
		ui.Text(c, a.Tr(m.DownloadNotice)).Role(ui.RoleStatus).TextColor(p.secondary)
	} else if m.DownloadWaiting {
		ui.Text(c, a.Tr("nativeDownloadWaiting")).Role(ui.RoleStatus).TextColor(p.secondary)
	}
	if len(m.Downloads) == 0 {
		ui.Text(c, a.Tr("nativeDownloadsEmpty")).TextColor(p.secondary)
		return
	}
	var total int64
	for _, j := range m.Downloads {
		total += j.StoredBytes()
	}
	ui.Text(c, a.Tr("nativeDownloadStorage")+" · "+downloadSize(total)).TextColor(p.secondary).FontSize(tokenSizeSmall)
	ui.Column(c).Gap(4).Children(func() {
		for _, j := range m.Downloads {
			c := c.Key(j.Item.ID)
			ui.Row(c).MinHeight(60).Padding(8, 4).Gap(8).AlignItems(ui.Center).Children(func() {
				nativeArtwork(c, j.Item.Artwork, 40, tokenRadiusArt, a, p)
				ui.Column(c).Grow(1).MinWidth(0).Gap(4).Children(func() {
					ui.Text(c, j.Item.Title).FontWeight(600).MaxLines(1).Ellipsis("…")
					detail := a.audioQualityLabel(j.Quality)
					if j.Item.Artist != "" {
						detail = j.Item.Artist + " · " + detail
					}
					ui.Text(c, detail).FontSize(tokenSizeMeta).TextColor(p.secondary).MaxLines(1).Ellipsis("…")
					ui.Text(c, downloadStatus(j, a)).FontSize(tokenSizeMeta).TextColor(p.secondary)
				})
				label, icon := "shellPause", "pause"
				switch j.State {
				case downloads.Complete:
					label, icon = "shellPlay", "play"
				case downloads.Paused:
					label, icon = "nativeDownloadResume", "download"
				case downloads.Failed:
					label, icon = "buttonRetry", "refresh"
				}
				if nativeIconButton(c, icon, a.Tr(label)+" · "+j.Item.Title, p, false, 36).Clicked() {
					if j.State == downloads.Complete {
						a.Play(j.Item)
					} else {
						a.DownloadPause(j.Item.ID, j.State == downloads.Queued || j.State == downloads.Downloading)
					}
				}
				if nativeIconButton(c, "close", a.Tr("nativeDownloadRemove")+" · "+j.Item.Title, p, false, 36).Clicked() {
					a.DownloadRemove(j.Item.ID)
				}
			})
		}
	})
}

func downloadStatus(j downloads.Entry, a ViewActions) string {
	var status string
	switch j.State {
	case downloads.Queued:
		status = a.Tr("nativeDownloadQueued")
	case downloads.Paused:
		status = a.Tr("nativeDownloadPaused")
	case downloads.Downloading:
		status = a.Tr("nativeDownloading")
	case downloads.Complete:
		return a.Tr("nativeDownloaded") + " · " + downloadSize(j.StoredBytes())
	case downloads.Failed:
		key := "nativeDownloadNetworkError"
		switch j.Failure {
		case "key":
			key = "nativeDownloadKeyError"
		case "media":
			key = "nativeDownloadMediaError"
		case "storage":
			key = "nativeDownloadStorageError"
		}
		return a.Tr(key)
	}
	if j.Total > 0 {
		return fmt.Sprintf("%s · %d%% · %s / %s", status, 100*j.Bytes/j.Total, downloadSize(j.Bytes), downloadSize(j.Total))
	}
	return status
}
