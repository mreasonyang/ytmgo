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

// A drag across rows chooses them, either way, without playing; with Cmd
// held as it begins, besides those chosen.
func TestDraggingAcrossSongsChoosesThem(t *testing.T) {
	m, a := nativeViewFixture()
	songs := m.Queue
	m.Route = "browse:VLsynthetic"
	m.Page = catalog.Page{Title: "Long", Shelves: []catalog.Shelf{{Items: songs[:4]}, {Items: songs[4:]}}}
	played := ""
	a.Play = func(it catalog.Item) { played = it.ID }
	tt := ui.NewTester(func(c *ui.Context) { nativeView(c, m, a) }, 1440, 2000)
	tt.Frame()
	row := func(i int) ui.Rect { return rect(t, tt, songs[i].Title) }
	at := func(i int) (float32, float32) { r := row(i); return r.X + 20, r.Y + r.H/2 }
	drag := func(from, to int, mods ui.Modifiers) {
		x, y := at(from)
		tt.Press(x, y)
		// The tester's pointer events carry no modifiers: hold them after.
		tt.HoldModifiers(mods)
		tt.Frame()
		for _, step := range []int{from, to} {
			x, y = at(step)
			tt.Move(x, y+1)
			tt.HoldModifiers(mods) // a real move carries the keys held
			tt.Frame()
		}
		tt.Release(x, y+1)
		tt.HoldModifiers(0)
		tt.Frame()
	}
	drag(2, 5, 0) // across the playlist's two pages
	if n := m.selection.count(m.Route); n != 4 || !m.selection.has(m.Route, songs[2].ID) || !m.selection.has(m.Route, songs[5].ID) || played != "" {
		t.Fatalf("dragging rows 3–6 chose %d, played %q", n, played)
	}
	drag(6, 4, 0) // upwards, afresh
	if n := m.selection.count(m.Route); n != 3 || m.selection.has(m.Route, songs[2].ID) {
		t.Fatalf("dragging up chose %d, kept the earlier choice %v", n, m.selection.has(m.Route, songs[2].ID))
	}
	drag(0, 1, ui.Cmd)
	if n := m.selection.count(m.Route); n != 5 || played != "" {
		t.Fatalf("a Cmd-drag left %d chosen, played %q; want 5, none", n, played)
	}
	if err := tt.Click(nativeItemLabel(songs[7])); err != nil || played != songs[7].ID || m.selection.count(m.Route) != 0 {
		t.Fatal("after a drag, a plain click no longer plays")
	}
}

// Cmd+A chooses every song of the page; in the search field it is the
// field's.
func TestCmdAChoosesThePageSongs(t *testing.T) {
	m, a := nativeViewFixture()
	m.Route = "browse:VLsynthetic"
	m.Page = catalog.Page{Title: "Long", Shelves: []catalog.Shelf{{Items: m.Queue[:4]}, {Items: m.Queue[4:]}}}
	tt := ui.NewTester(func(c *ui.Context) { nativeView(c, m, a) }, 1440, 2000)
	tt.Frame()
	tt.Frame()
	tt.Key(ui.Cmd, ui.KeyA)
	if n := m.selection.count(m.Route); n != len(m.Queue) {
		t.Fatalf("Cmd+A chose %d of %d", n, len(m.Queue))
	}
	tt.Key(0, ui.KeyEscape)
	m.focusSearch()
	m.Query = "daft"
	tt.Frame()
	tt.Key(ui.Cmd, ui.KeyA)
	if n := m.selection.count(m.Route); n != 0 {
		t.Fatalf("Cmd+A in the search field chose %d songs", n)
	}
}

// Search results choose as a collection's table does.
func TestChoosingSearchResults(t *testing.T) {
	m, a := nativeViewFixture()
	m.Route = "search"
	m.Page = catalog.Page{Title: "Results", Shelves: []catalog.Shelf{{Title: "Songs", Items: m.Queue[:5]}}}
	queued := 0
	a.QueueAppend = func(catalog.Item) { queued++ }
	tt := ui.NewTester(func(c *ui.Context) { nativeView(c, m, a) }, 1440, 1400)
	tt.Frame()
	if err := tt.ClickWith(ui.Cmd, nativeResultLabel(m.Queue[0], m, a)); err != nil {
		t.Fatal(err)
	}
	if err := tt.ClickWith(ui.Shift, nativeResultLabel(m.Queue[2], m, a)); err != nil {
		t.Fatal(err)
	}
	if err := tt.RightClick(nativeResultLabel(m.Queue[1], m, a)); err != nil {
		t.Fatal(err)
	}
	if err := chooseAppMenuItem(tt, a.Tr("nativeAddToQueue")); err != nil || queued != 3 {
		t.Fatalf("queued %d of the 3 chosen results (%v)", queued, err)
	}
}

// In the queue, chosen songs leave it together; the playing one stays.
func TestRemovingChosenSongsFromTheQueue(t *testing.T) {
	fixture, a := nativeViewFixture()
	app := &desktop{model: *fixture, historyIndex: -1}
	m := &app.model
	app.queueIndex = 2
	m.Current = m.Queue[2]
	m.Panel, m.PanelTab = true, "queue"
	m.Page = catalog.Page{Title: "Empty"} // the queue's rows alone carry the songs' labels
	a.QueueRemove = app.removeFromQueue
	tt := ui.NewTester(func(c *ui.Context) { nativeView(c, m, a) }, 1440, 1400)
	tt.Frame()
	label := func(i int) string { return nativeItemLabel(fixture.Queue[i]) }
	for _, i := range []int{1, 2, 5} {
		if err := tt.ClickWith(ui.Cmd, label(i)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tt.RightClick(label(5)); err != nil {
		t.Fatal(err)
	}
	if err := chooseAppMenuItem(tt, a.Tr("nativeQueueRemove")); err != nil {
		t.Fatal(err)
	}
	if len(m.Queue) != len(fixture.Queue)-2 || app.queueIndex != 1 || m.Queue[app.queueIndex].ID != fixture.Queue[2].ID {
		t.Fatalf("queue %d long, playing index %d; want 2 removed and the playing song kept", len(m.Queue), app.queueIndex)
	}
}
