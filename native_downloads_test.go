package main

import (
	"fmt"
	"testing"

	"github.com/egoist/mygo/ui"
	"ytmgo/internal/audioquality"
	"ytmgo/internal/catalog"
	"ytmgo/internal/downloads"
)

func TestNativeDownloadManagement(t *testing.T) {
	for _, width := range []int{380, 1440} {
		t.Run(fmt.Sprint(width), func(t *testing.T) {
			fixture, a := nativeViewFixture()
			app := &desktop{model: *fixture, historyIndex: -1}
			m := &app.model
			a.Navigate = app.navigate
			m.Route = "library"
			items := m.Queue[:4]
			states := []downloads.State{downloads.Queued, downloads.Paused, downloads.Failed, downloads.Complete}
			m.downloadStates = map[string]downloads.State{}
			for i, state := range states {
				m.Downloads = append(m.Downloads, downloads.Entry{Item: items[i], State: state, Quality: audioquality.Standard, Bytes: 512, Total: 1024})
				m.downloadStates[items[i].ID] = state
			}
			var added, paused, resumed, removed, played string
			a.Download = func(item catalog.Item) { added = item.ID }
			a.DownloadPause = func(id string, pause bool) {
				if pause {
					paused = id
				} else {
					resumed = id
				}
			}
			a.DownloadRemove = func(id string) { removed = id }
			a.Play = func(item catalog.Item) { played = item.ID }
			collections := 0
			a.DownloadCollection = func() { collections++ }
			tt := ui.NewTester(func(c *ui.Context) { nativeView(c, m, a) }, width, 900)
			click := func(label string) {
				t.Helper()
				nativeAssertVisible(t, tt, label, width, 900)
				if err := tt.Click(label); err != nil {
					t.Fatal(err)
				}
			}
			// The library leads to them: its panel, or a compact Library page.
			click(a.Tr("nativeDownloads"))
			if m.Route != "downloads" || m.Page.Title != app.tr("nativeDownloads") || len(m.Page.Items) != 1 || m.Page.Items[0].ID != items[3].ID {
				t.Fatal("download manager did not open")
			}
			click(a.Tr("nativeDownloadPause") + " · " + items[0].Title)
			click(a.Tr("nativeDownloadResume") + " · " + items[1].Title)
			if paused != items[0].ID || resumed != items[1].ID {
				t.Fatal("pause or resume targeted the wrong track")
			}
			click(a.Tr("buttonRetry") + " · " + items[2].Title)
			// A finished download is a track row: it plays, and has no
			// download control of its own.
			click(nativeItemLabel(items[3]))
			if _, ok := tt.Find(a.Tr("shellPlay") + " · " + items[3].Title); ok {
				t.Fatal("a finished download keeps a separate play button")
			}
			click(a.Tr("nativeDownloadRemove") + " · " + items[3].Title)
			if resumed != items[2].ID || played != items[3].ID || removed != items[3].ID {
				t.Fatal("retry, offline play or removal targeted the wrong track")
			}
			if !tt.HasText(downloadStatus(m.Downloads[2], a)) {
				t.Fatal("the failure is not shown")
			}
			if width == 1440 && !tt.HasText(downloadStatus(m.Downloads[3], a)) {
				t.Fatal("the downloaded quality and size are missing")
			}
			nativeSnapshot(t, tt, fmt.Sprintf("downloads-%d", width))

			// A collection offers its songs while some aren't downloaded;
			// asking for one stays on the page.
			m.Route = "browse:VLsynthetic"
			m.Page = catalog.Page{Title: "Collection", Items: m.Queue[:5]}
			tt.Frame()
			if err := tt.RightClick(nativeItemLabel(m.Queue[4])); err != nil {
				t.Fatal(err)
			}
			if err := tt.ChooseMenuItem(a.Tr("nativeDownloadTrack")); err != nil {
				t.Fatal(err)
			}
			if added != m.Queue[4].ID || m.Route != "browse:VLsynthetic" {
				t.Fatalf("download song added %q and moved to %q", added, m.Route)
			}
			removed = ""
			if err := tt.RightClick(nativeItemLabel(items[3])); err != nil {
				t.Fatal(err)
			}
			if err := tt.ChooseMenuItem(a.Tr("nativeDownloadRemove")); err != nil {
				t.Fatal(err)
			}
			if removed != items[3].ID {
				t.Fatal("a downloaded song's menu does not remove it")
			}
			click(a.Tr("nativeDownloadCollection"))
			if collections != 1 || m.Route != "browse:VLsynthetic" {
				t.Fatal("collection did not enqueue in place")
			}
			for _, it := range m.Page.Items {
				m.downloadStates[it.ID] = downloads.Queued
			}
			tt.Frame()
			if tt.HasText(a.Tr("nativeDownloadCollection")) || !tt.HasText(a.Tr("nativeDownloading")) {
				t.Fatal("a collection under way still offers the download")
			}
			for _, it := range m.Page.Items {
				m.downloadStates[it.ID] = downloads.Complete
			}
			tt.Frame()
			if tt.HasText(a.Tr("nativeDownloadCollection")) || !tt.HasText(a.Tr("nativeDownloaded")) {
				t.Fatal("a downloaded collection does not say so")
			}
		})
	}
}

// Beside Like, in the player bar and the mini player: it downloads the
// playing song in place, then leads to Downloads, its glyph and dot showing
// the state as Like's do.
func TestPlayingSongDownloadButton(t *testing.T) {
	for _, width := range []int{420, 1440} {
		t.Run(fmt.Sprint(width), func(t *testing.T) {
			m, a := nativeViewFixture()
			m.downloadStates = map[string]downloads.State{}
			a.Download = func(item catalog.Item) { m.downloadStates[item.ID] = downloads.Queued }
			a.DownloadPause, a.DownloadRemove = func(string, bool) {}, func(string) {}
			tt := ui.NewTester(func(c *ui.Context) { nativeView(c, m, a) }, width, 900)
			tt.Frame()
			click := func(key string) {
				t.Helper()
				label := a.Tr(key) + " · " + m.Current.Title
				nativeAssertVisible(t, tt, label, width, 900)
				if err := tt.Click(label); err != nil {
					t.Fatal(err)
				}
			}
			click("nativeDownloadTrack")
			if m.downloadStates[m.Current.ID] != downloads.Queued || m.Route != "home" {
				t.Fatalf("download state %q, route %q", m.downloadStates[m.Current.ID], m.Route)
			}
			click("nativeDownloading")
			if m.Route != "downloads" {
				t.Fatal("a download under way does not lead to Downloads")
			}
			m.Route = "home"
			m.downloadStates[m.Current.ID] = downloads.Complete
			tt.Frame()
			click("nativeDownloaded")
			if m.Route != "downloads" {
				t.Fatal("a downloaded song does not lead to Downloads")
			}
		})
	}
}
