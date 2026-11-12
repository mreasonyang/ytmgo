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
			for i, state := range states {
				m.Downloads = append(m.Downloads, downloads.Entry{Item: items[i], State: state, Quality: audioquality.Standard, Bytes: 512, Total: 1024})
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
			click(a.Tr("nativeDownloadTrack") + " · " + m.Current.Title)
			if added != m.Current.ID {
				t.Fatal("current song download action missing")
			}
			click(a.Tr("nativeDownloads"))
			if m.Route != "downloads" || m.Page.Title != app.tr("nativeDownloads") || len(m.Page.Items) != 1 || m.Page.Items[0].ID != items[3].ID {
				t.Fatal("download manager did not open")
			}
			click(a.Tr("shellPause") + " · " + items[0].Title)
			click(a.Tr("nativeDownloadResume") + " · " + items[1].Title)
			if paused != items[0].ID || resumed != items[1].ID {
				t.Fatal("pause or resume targeted the wrong track")
			}
			click(a.Tr("buttonRetry") + " · " + items[2].Title)
			click(a.Tr("shellPlay") + " · " + items[3].Title)
			click(a.Tr("nativeDownloadRemove") + " · " + items[3].Title)
			if resumed != items[2].ID || played != items[3].ID || removed != items[3].ID {
				t.Fatal("retry, offline play or removal targeted the wrong track")
			}
			if !tt.HasText(downloadStatus(m.Downloads[3], a)) {
				t.Fatal("completed audio size is missing")
			}
			nativeSnapshot(t, tt, fmt.Sprintf("downloads-%d", width))
			m.Route = "browse:VLsynthetic"
			m.Page = catalog.Page{Title: "Collection", Items: items}
			tt.Frame()
			click(a.Tr("nativeDownloadCollection"))
			if collections != 1 {
				t.Fatal("collection did not enqueue")
			}
		})
	}
}
