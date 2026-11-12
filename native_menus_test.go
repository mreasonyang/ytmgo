package main

import (
	"fmt"
	"testing"

	"github.com/egoist/mygo/ui"
	"ytmgo/internal/catalog"
)

// chooseAppMenuItem chooses label in the app menu showing, drawn in the
// window (nativeMenuPanel), as ChooseMenuItem does in the system's.
func chooseAppMenuItem(tt *ui.Tester, label string) error {
	tt.Frame()
	item, ok := tt.AccessByRole(ui.RoleMenuItem, label)
	if !ok {
		return fmt.Errorf("no app menu item %q is shown", label)
	}
	b := item.Bounds
	tt.ClickAt(b.X+b.W/2, b.Y+b.H/2)
	tt.Frame()
	return nil
}

// Song and account menus are the app's own (design/README.md §11): in the
// window, in the theme's raised surface, not the desktop's menus.
func TestAppMenusAreDrawnInTheWindow(t *testing.T) {
	m, a := nativeViewFixture()
	m.SignedIn = true
	tt := ui.NewTester(func(c *ui.Context) { nativeView(c, m, a) }, 1440, 900)
	tt.Frame()
	if err := tt.RightClick("Instant Crush · Daft Punk"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if err := tt.ChooseMenuItem(a.Tr("nativeAddToQueue")); err == nil {
		t.Fatal("a song's menu is still the system's")
	}
	before := len(m.Queue)
	if err := chooseAppMenuItem(tt, a.Tr("nativeAddToQueue")); err != nil || len(m.Queue) != before+1 {
		t.Fatalf("in-window menu: %v, queue %d → %d", err, before, len(m.Queue))
	}
	if _, ok := tt.AccessByRole(ui.RoleMenuItem, a.Tr("shellPlay")); ok {
		t.Fatal("the menu stayed open after its choice")
	}
}

// A long playlist's pages arrive as shelves of their own; they show as one
// table, under one header, numbered straight on (not a header every 100).
func TestPlaylistPagesAreOneTable(t *testing.T) {
	m, a := nativeViewFixture()
	m.Route = "browse:VLsynthetic"
	m.Page = catalog.Page{Title: "Long", Shelves: []catalog.Shelf{{Items: m.Queue[:3]}, {Items: m.Queue[3:6]}, {Items: m.Queue[6:]}}}
	tt := ui.NewTester(func(c *ui.Context) { nativeView(c, m, a) }, 1440, 2400)
	tt.Frame()
	if n := countText(tt, a.Tr("shellColumnTitle")); n != 1 {
		t.Fatalf("%d table headers for one playlist", n)
	}
	third, fourth := rect(t, tt, m.Queue[2].Title), rect(t, tt, m.Queue[3].Title)
	if gap := fourth.Y - third.Y; gap != tokenProductRow {
		t.Fatalf("the second page starts %.0f below the first's last row, want %d", gap, tokenProductRow)
	}
	if !tt.HasText("4") || !tt.HasText("7") {
		t.Fatal("numbering restarted on a later page")
	}
}
