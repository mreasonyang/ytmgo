package main

import (
	"testing"

	"github.com/egoist/mygo/ui"
	"ytmgo/internal/catalog"
	"ytmgo/internal/downloads"
)

// Songs in a collection are chosen with the mouse, as in a file list, and
// the menu of a chosen one acts on them all.
func TestChoosingSongsWithTheMouse(t *testing.T) {
	m, a := nativeViewFixture()
	songs := m.Queue
	m.Route = "browse:VLsynthetic"
	m.Page = catalog.Page{Title: "Long", Shelves: []catalog.Shelf{{Items: songs[:4]}, {Items: songs[4:]}}}
	var queued []string
	var downloaded [][]catalog.Item
	var removed []string
	played := ""
	a.Play = func(it catalog.Item) { played = it.ID }
	a.QueueAppend = func(it catalog.Item) { queued = append(queued, it.ID) }
	a.Download = func(catalog.Item) { t.Fatal("several songs went one by one") }
	a.DownloadMany = func(items []catalog.Item) { downloaded = append(downloaded, items) }
	a.DownloadPause = func(string, bool) {}
	a.DownloadRemove = func(id string) { removed = append(removed, id) }
	m.downloadStates = map[string]downloads.State{songs[3].ID: downloads.Complete}
	tt := ui.NewTester(func(c *ui.Context) { nativeView(c, m, a) }, 1440, 2000)
	tt.Frame()
	label := func(i int) string { return nativeItemLabel(songs[i]) }
	chosen := func() int { return m.selection.count(m.Route) }

	// Cmd-click adds; Shift-click extends across the playlist's pages.
	if err := tt.ClickWith(ui.Cmd, label(1)); err != nil {
		t.Fatal(err)
	}
	if err := tt.ClickWith(ui.Shift, label(5)); err != nil {
		t.Fatal(err)
	}
	if chosen() != 5 || played != "" {
		t.Fatalf("%d chosen, played %q; want songs 2–6 chosen, none played", chosen(), played)
	}
	if err := tt.ClickWith(ui.Cmd, label(2)); err != nil {
		t.Fatal(err)
	}
	if chosen() != 4 || m.selection.has(m.Route, songs[2].ID) {
		t.Fatal("Cmd-click did not take a chosen song out")
	}
	// Chosen rows show it, in the selection colour.
	tt.Frame()

	// The menu of a chosen song acts on them all, in the table's order.
	if err := tt.RightClick(label(3)); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if _, ok := tt.AccessByRole(ui.RoleMenuItem, a.TrArgs("nativeSelectedCount", map[string]any{"count": 4})); !ok {
		t.Fatal("the menu does not say how many are chosen")
	}
	if err := chooseAppMenuItem(tt, a.Tr("nativeDownloadSongs")); err != nil {
		t.Fatal(err)
	}
	if len(downloaded) != 1 || len(downloaded[0]) != 3 || downloaded[0][0].ID != songs[1].ID {
		t.Fatalf("downloaded %v, want the three chosen not yet downloaded, at once", downloaded)
	}
	if chosen() != 0 {
		t.Fatal("the choice stayed after its action")
	}

	// Again, to queue them and remove the one downloaded.
	for _, i := range []int{1, 3, 4} {
		if err := tt.ClickWith(ui.Cmd, label(i)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tt.RightClick(label(4)); err != nil {
		t.Fatal(err)
	}
	if err := chooseAppMenuItem(tt, a.Tr("nativeDownloadRemoveSongs")); err != nil {
		t.Fatal(err)
	}
	if len(removed) != 1 || removed[0] != songs[3].ID {
		t.Fatalf("removed %v, want only the downloaded one", removed)
	}
	for _, i := range []int{1, 4} {
		if err := tt.ClickWith(ui.Cmd, label(i)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tt.RightClick(label(1)); err != nil {
		t.Fatal(err)
	}
	if err := chooseAppMenuItem(tt, a.Tr("nativeAddToQueue")); err != nil {
		t.Fatal(err)
	}
	if len(queued) != 2 || queued[0] != songs[1].ID || queued[1] != songs[4].ID {
		t.Fatalf("queued %v, want the two chosen in order", queued)
	}

	// A plain click plays, as before, and lets the choice go; so does Esc.
	if err := tt.ClickWith(ui.Cmd, label(6)); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click(label(0)); err != nil {
		t.Fatal(err)
	}
	if played != songs[0].ID || chosen() != 0 {
		t.Fatalf("plain click played %q, %d still chosen", played, chosen())
	}
	if err := tt.ClickWith(ui.Cmd, label(6)); err != nil {
		t.Fatal(err)
	}
	tt.Key(0, ui.KeyEscape)
	if chosen() != 0 {
		t.Fatal("Esc did not let the choice go")
	}
	// Leaving the page lets it go too.
	if err := tt.ClickWith(ui.Cmd, label(6)); err != nil {
		t.Fatal(err)
	}
	m.Route = "home"
	tt.Frame()
	m.Route = "browse:VLsynthetic"
	tt.Frame()
	if chosen() != 0 {
		t.Fatal("a choice came back with its page")
	}
}
