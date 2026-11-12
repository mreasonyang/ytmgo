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
			if err := chooseAppMenuItem(tt, a.Tr("nativeDownloadTrack")); err != nil {
				t.Fatal(err)
			}
			if added != m.Queue[4].ID || m.Route != "browse:VLsynthetic" {
				t.Fatalf("download song added %q and moved to %q", added, m.Route)
			}
			removed = ""
			if err := tt.RightClick(nativeItemLabel(items[3])); err != nil {
				t.Fatal(err)
			}
			if err := chooseAppMenuItem(tt, a.Tr("nativeDownloadRemove")); err != nil {
				t.Fatal(err)
			}
			if removed != items[3].ID {
				t.Fatal("a downloaded song's menu does not remove it")
			}
			toggle := func(key string) string {
				label := a.Tr(key)
				if key != "nativeDownloadCollection" {
					label += " · " + a.Tr("nativeShowDownloads")
				}
				return label + " · Collection"
			}
			click(toggle("nativeDownloadCollection"))
			if collections != 1 || m.Route != "browse:VLsynthetic" {
				t.Fatal("collection did not enqueue in place")
			}
			for _, it := range m.Page.Items {
				m.downloadStates[it.ID] = downloads.Queued
			}
			tt.Frame()
			click(toggle("nativeDownloading"))
			if collections != 1 || m.Route != "downloads" {
				t.Fatal("a collection under way does not lead to Downloads")
			}
			m.Route = "browse:VLsynthetic"
			m.Page = catalog.Page{Title: "Collection", Items: m.Queue[:5]}
			for _, it := range m.Page.Items {
				m.downloadStates[it.ID] = downloads.Complete
			}
			tt.Frame()
			if _, ok := tt.Find(toggle("nativeDownloaded")); !ok {
				t.Fatal("a downloaded collection does not say so")
			}
		})
	}
}

// The player bar keeps to its zones (§11): downloading the playing song is
// in its More menu, as in other players, not a button beside Like.
func TestPlayerBarLeavesDownloadToItsMenu(t *testing.T) {
	m, a := nativeViewFixture()
	var added string
	a.Download = func(item catalog.Item) { added = item.ID }
	a.DownloadPause, a.DownloadRemove = func(string, bool) {}, func(string) {}
	tt := ui.NewTester(func(c *ui.Context) { nativeView(c, m, a) }, 1440, 900)
	tt.Frame()
	for _, key := range []string{"nativeDownloadTrack", "nativeDownloading", "nativeDownloaded"} {
		if _, ok := tt.Find(a.Tr(key) + " · " + m.Current.Title); ok {
			t.Fatal("the player bar grew a download button")
		}
	}
	if err := tt.Click(a.Tr("shellMore")); err != nil {
		t.Fatal(err)
	}
	if err := chooseAppMenuItem(tt, a.Tr("nativeDownloadTrack")); err != nil {
		t.Fatal(err)
	}
	if added != m.Current.ID || m.Route != "home" {
		t.Fatalf("More › Download song added %q, route %q", added, m.Route)
	}
}

// A downloaded song is marked in any track list, before its artist.
func TestTrackRowsMarkDownloadedSongs(t *testing.T) {
	m, a := nativeViewFixture()
	tt := ui.NewTester(func(c *ui.Context) { nativeView(c, m, a) }, 1440, 900)
	tt.Frame()
	before := len(tt.Texts())
	song := m.Page.Items[2]
	artist := rect(t, tt, song.Title)
	m.downloadStates = map[string]downloads.State{song.ID: downloads.Complete}
	tt.Frame()
	if after := rect(t, tt, song.Title); after.X != artist.X || len(tt.Texts()) != before {
		t.Fatal("the mark moved the title or changed the text")
	}
	m.downloadStates[song.ID] = downloads.Queued
	tt.Frame()
}
