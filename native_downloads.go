package main

import (
	"fmt"
	"math"
	"strings"

	"github.com/egoist/mygo/ui"
	"ytmgo/internal/catalog"
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

// nativeDownloadsShown reports whether the library offers its Downloads
// entry: once something is downloaded or asked for, not before.
func nativeDownloadsShown(m *UIModel, a ViewActions) bool {
	return a.DownloadPause != nil && (len(m.Downloads) > 0 || m.Route == "downloads" || m.DownloadNotice != "")
}

// nativeDownloadsEntry is the library row for Downloads: a 48 tile with the
// download glyph, washed while its page is open, as an open collection is.
func nativeDownloadsEntry(c *ui.Context, m *UIModel, a ViewActions, p nativePalette, full bool) {
	label := a.Tr("nativeDownloads")
	b := ui.ButtonBase(c.Key("downloads-entry")).Height(tokenProductLibraryRow).Padding(8).Gap(12).Radius(tokenRadiusControl).Justify(ui.Start).Label(label)
	nativeTip(c, b, label, p)
	if !full {
		b.Padding(4).Justify(ui.Center)
	}
	if m.Route == "downloads" {
		b.Selected(true).Background(p.press).TextColor(p.text)
	} else if b.Hovered() {
		b.Background(p.hover)
	}
	b.Children(func() {
		ui.Box(c).Size(48, 48).Shrink(0).Radius(tokenRadiusArt).Background(p.highlight).TextColor(p.text).Center().Children(func() {
			ui.Icon(c, nativeIcons["download"]).Size(24, 24)
		})
		if !full {
			return
		}
		ui.Column(c).Grow(1).MinWidth(0).Gap(4).Children(func() {
			ui.Text(c, label).FontSize(tokenSizeSmall).FontWeight(500).MaxLines(1).Ellipsis("…")
			ui.Text(c, nativeDownloadsSummary(m, a)).FontSize(tokenSizeMeta).TextColor(p.secondary).MaxLines(1).Ellipsis("…")
		})
	})
	if b.Clicked() {
		a.Navigate("downloads")
	}
}

// nativeDownloadsSummary is the entry's subtitle: what is under way, else
// the space the downloads take.
func nativeDownloadsSummary(m *UIModel, a ViewActions) string {
	var total int64
	for _, j := range m.Downloads {
		if j.State == downloads.Queued || j.State == downloads.Downloading {
			return a.Tr("nativeDownloading")
		}
		total += j.StoredBytes()
	}
	return downloadSize(total)
}

// nativeCollectionDownload is the collection's download toggle, beside its
// play button: an arrow while some of its songs aren't downloaded, a ring
// filling as they download, then the filled glyph. Under way or done, it
// leads to Downloads.
func nativeCollectionDownload(c *ui.Context, m *UIModel, a ViewActions, p nativePalette, items []catalog.Item) {
	if a.DownloadCollection == nil || !(strings.HasPrefix(m.Route, "browse:MPRE") || strings.HasPrefix(m.Route, "browse:VL")) || len(items) == 0 {
		return
	}
	note := m.collectionDownloads[m.Route]
	entries := make(map[string]downloads.Entry, len(m.Downloads))
	for _, j := range m.Downloads {
		entries[j.Item.ID] = j
	}
	missing, active, done := false, note == "nativeDownloadPreparing", float32(0)
	for _, it := range items {
		switch m.downloadStates[it.ID] {
		case downloads.Complete:
			done++
		case downloads.Queued, downloads.Downloading:
			active = true
			if j := entries[it.ID]; j.Total > 0 {
				done += float32(j.Bytes) / float32(j.Total)
			}
		default: // not asked for, paused or failed: the arrow (re)starts them
			missing = true
		}
	}
	icon, key := "download-circle", "nativeDownloadCollection"
	ring := note == "nativeDownloadPreparing" || (active && !missing)
	switch {
	case ring:
		icon, key = "download-arrow", "nativeDownloading"
	case !missing:
		icon, key = "download-fill", "nativeDownloaded"
	}
	label := a.Tr(key)
	if key != "nativeDownloadCollection" {
		label += " · " + a.Tr("nativeShowDownloads")
	}
	const hit, glyph = 48, 32
	b := ui.ButtonBase(c.Key("collection-download")).Size(hit, hit).Shrink(0).Radius(hit / 2).Label(label + " · " + m.Page.Title).TextColor(p.secondary)
	nativeTip(c, b, label, p)
	if !missing || b.Hovered() {
		b.TextColor(p.text)
	}
	nativePress(b)
	if ring {
		fraction := max(0, min(1, done/float32(len(items))))
		track, fill := p.track, p.text
		b.Draw(func(painter *ui.Painter, r ui.Rect) {
			cx, cy, radius := r.X+r.W/2, r.Y+r.H/2, float32(glyph)*9/24
			painter.StrokePath(new(ui.Path).Circle(cx, cy, radius), 2, track)
			if fraction > 0 {
				// From twelve o'clock, clockwise.
				arc := new(ui.Path).MoveTo(cx, cy-radius)
				for k := 1; k <= 64; k++ {
					t := float64(k) / 64 * 2 * math.Pi
					arc.LineTo(cx+radius*float32(math.Sin(t)), cy-radius*float32(math.Cos(t)))
				}
				painter.StrokePath(arc.Cap(ui.RoundCap).Trim(0, fraction), 2, fill)
			}
		})
	}
	b.Children(func() { ui.Icon(c, nativeIcons[icon]).Size(glyph, glyph) })
	if b.Clicked() {
		if missing && !ring {
			a.DownloadCollection()
		} else {
			a.Navigate("downloads")
		}
	}
	if note == "nativeDownloadCollectionError" {
		nativeDownloadAlert(c, a.Tr(note), p).Shrink(1).MinWidth(0)
	}
}

// nativeDownloadAlert is an error in words beside the attention glyph.
func nativeDownloadAlert(c *ui.Context, text string, p nativePalette) ui.Element {
	return ui.Row(c).Gap(8).AlignItems(ui.Center).Role(ui.RoleAlert).Children(func() {
		ui.Icon(c, nativeIcons["alert"]).Size(16, 16).Shrink(0).TextColor(p.attention)
		ui.Text(c, text).FontSize(tokenSizeMeta).TextColor(p.text).MinWidth(0)
	})
}

func nativeDownloads(c *ui.Context, m *UIModel, a ViewActions, p nativePalette, compact bool) {
	switch m.DownloadNotice {
	case "":
		if m.DownloadWaiting {
			ui.Text(c, a.Tr("nativeDownloadWaiting")).Role(ui.RoleStatus).FontSize(tokenSizeMeta).TextColor(p.secondary)
		}
	case "nativeDownloadPreparing":
		ui.Text(c, a.Tr(m.DownloadNotice)).Role(ui.RoleStatus).FontSize(tokenSizeMeta).TextColor(p.secondary)
	default:
		nativeDownloadAlert(c, a.Tr(m.DownloadNotice), p)
	}
	if len(m.Downloads) == 0 {
		ui.Text(c, a.Tr("nativeDownloadsEmpty")).TextColor(p.secondary)
		return
	}
	var total int64
	for _, j := range m.Downloads {
		total += j.StoredBytes()
	}
	ui.Text(c, a.Tr("nativeDownloadStorage")+" · "+downloadSize(total)).FontSize(tokenSizeMeta).TextColor(p.secondary)
	ui.Column(c).Children(func() {
		for _, j := range m.Downloads {
			nativeDownloadRow(c.Key(j.Item.ID), j, m, a, p, compact)
		}
	})
}

// nativeDownloadRow is a track row (§11): it plays on click, washes on
// hover and marks the playing song, with the download's state in the album's
// place and its controls at the end.
func nativeDownloadRow(c *ui.Context, j downloads.Entry, m *UIModel, a ViewActions, p nativePalette, compact bool) {
	item := j.Item
	playing := item.ID != "" && item.ID == m.Current.ID
	pad := float32(16)
	if compact {
		pad = 8
	}
	row := ui.Row(c).Height(tokenProductRow).PaddingX(pad).Gap(8).Radius(tokenRadiusControl).AlignItems(ui.Center)
	if playing {
		row.Background(p.hover)
	}
	failed := j.State == downloads.Failed
	status := downloadStatus(j, a)
	row.Children(func() {
		// The row's wash answers its play target, as a track row's does.
		b := ui.ButtonBase(c).Grow(1).Basis(0).MinWidth(0).Height(tokenProductRow).Gap(16).Justify(ui.Start).Label(nativeItemLabel(item))
		hovered := b.Hovered()
		if hovered {
			row.Background(p.press)
		}
		b.Children(func() {
			ui.Row(c).Grow(1).Basis(0).MinWidth(0).Gap(12).AlignItems(ui.Center).Children(func() {
				a.collectionArt(c, item, 40, tokenRadiusArt, m, p)
				ui.Column(c).Grow(1).MinWidth(0).Gap(4).Children(func() {
					t := ui.Text(c, item.Title).FontSize(tokenSizeSmall).FontWeight(500).MaxLines(1).Ellipsis("…")
					if playing {
						t.TextColor(p.accentText)
					}
					subtitle := item.Artist
					if compact && j.State != downloads.Complete {
						subtitle = status
					}
					ui.Row(c).MinWidth(0).Gap(4).AlignItems(ui.Center).Children(func() {
						if compact && failed {
							ui.Icon(c, nativeIcons["alert"]).Size(16, 16).Shrink(0).TextColor(p.attention)
						}
						artist := ui.Text(c, subtitle).FontSize(tokenSizeMeta).TextColor(p.secondary).MaxLines(1).Ellipsis("…")
						if hovered {
							artist.TextColor(p.text)
						}
					})
				})
			})
			if !compact {
				ui.Row(c).Grow(0.75).Basis(0).MinWidth(0).Gap(8).AlignItems(ui.Center).Children(func() {
					if failed {
						ui.Icon(c, nativeIcons["alert"]).Size(16, 16).Shrink(0).TextColor(p.attention)
					}
					ui.Text(c, status).FontSize(tokenSizeMeta).TextColor(p.secondary).MaxLines(1).Ellipsis("…")
				})
				ui.Box(c).Size(16, 16).Shrink(0).Children(func() {
					if playing {
						ui.Icon(c, nativeIcons["volume"]).Size(16, 16).TextColor(p.accent)
					}
				})
				duration := ""
				if item.Duration > 0 {
					duration = nativeTime(item.Duration.Seconds())
				}
				ui.Text(c, duration).Width(48).Shrink(0).FontSize(tokenSizeMeta).TextColor(p.secondary).TextAlign(ui.End)
			}
		})
		b.ContextMenu(nativeTrackMenu(item, m, a))
		if b.Clicked() {
			a.Play(item)
		}
		// The download's own control: pause what moves, resume what waits,
		// retry what failed. A finished one has none; the row plays it.
		label, icon := "", ""
		switch j.State {
		case downloads.Queued, downloads.Downloading:
			label, icon = "nativeDownloadPause", "download-pause"
		case downloads.Paused:
			label, icon = "nativeDownloadResume", "download"
		case downloads.Failed:
			label, icon = "buttonRetry", "refresh"
		}
		ui.Box(c).Size(32, 32).Shrink(0).Children(func() {
			if icon != "" && nativeIconButton(c, icon, a.Tr(label)+" · "+item.Title, p, false, 32).Clicked() {
				a.DownloadPause(item.ID, j.State == downloads.Queued || j.State == downloads.Downloading)
			}
		})
		if nativeIconButton(c, "close", a.Tr("nativeDownloadRemove")+" · "+item.Title, p, false, 32).Clicked() {
			a.DownloadRemove(item.ID)
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
		return a.audioQualityLabel(j.Quality) + " · " + downloadSize(j.StoredBytes())
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
	if j.Total > 0 && j.Bytes > 0 {
		return fmt.Sprintf("%s · %d%%", status, 100*j.Bytes/j.Total)
	}
	return status
}
