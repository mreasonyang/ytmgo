package main

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

// The app asks in its own dialog: a title, what will be lost, Cancel and
// the one inverted action; Cancel, Esc and the action each close it once.
func TestConfirmationIsTheAppsOwnDialog(t *testing.T) {
	for _, how := range []string{"action", "cancel", "escape"} {
		t.Run(how, func(t *testing.T) {
			m, a := nativeViewFixture()
			ran, canceled := 0, 0
			m.confirm = &nativeConfirmation{title: "Remove all downloads", body: "Remove every downloaded song?", action: "Remove all", run: func() { ran++ }, cancel: func() { canceled++ }}
			tt := ui.NewTester(func(c *ui.Context) { nativeView(c, m, a) }, 1440, 900)
			tt.Frame()
			for _, text := range []string{"Remove all downloads", "Remove every downloaded song?", "Remove all", a.Tr("buttonCancel")} {
				if !tt.HasText(text) {
					t.Fatalf("the dialog lacks %q", text)
				}
			}
			switch how {
			case "action":
				if err := tt.Click("Remove all"); err != nil {
					t.Fatal(err)
				}
			case "cancel":
				if err := tt.Click(a.Tr("buttonCancel")); err != nil {
					t.Fatal(err)
				}
			default:
				tt.Key(0, ui.KeyEscape)
			}
			tt.Frame()
			want := [2]int{0, 1}
			if how == "action" {
				want = [2]int{1, 0}
			}
			if got := [2]int{ran, canceled}; got != want || m.confirm != nil || tt.HasText("Remove every downloaded song?") {
				t.Fatalf("%s: ran/canceled %v, want %v; dialog still set %v", how, got, want, m.confirm != nil)
			}
		})
	}
}

// Asked from Settings (sign-out lives there), Esc answers the question and
// leaves Settings open.
func TestConfirmationOverSettingsClosesAlone(t *testing.T) {
	m, a := nativeViewFixture()
	m.Settings = true
	m.confirm = &nativeConfirmation{title: "Sign out of ytmGO", body: "Sign out?", action: "Sign out"}
	tt := ui.NewTester(func(c *ui.Context) { nativeView(c, m, a) }, 1440, 900)
	tt.Frame()
	tt.Key(0, ui.KeyEscape)
	tt.Frame()
	if m.confirm != nil || !m.Settings {
		t.Fatalf("Esc: dialog %v, settings %v; want the dialog gone and Settings kept", m.confirm != nil, m.Settings)
	}
}

// Remove all asks in the app, naming the space, and removes only once
// the action is chosen.
func TestRemoveAllAsksInTheApp(t *testing.T) {
	fixture, _ := nativeViewFixture()
	app := &desktop{model: *fixture, historyIndex: -1, ctx: t.Context()}
	app.model.Language = "en"
	library := newTestDownloads(t)
	app.downloads = library
	songs := app.model.Queue[1:3]
	songs[0].ID, songs[1].ID = "AAAAAAAAAA0", "AAAAAAAAAA1"
	app.enqueueDownloads(songs)
	app.confirmRemoveAllDownloads()
	q := app.model.confirm
	if q == nil || q.run == nil {
		t.Fatal("Remove all did not ask")
	}
	if len(library.Snapshot()) != 2 {
		t.Fatal("asking removed something")
	}
	app.model.confirm = nil
	q.run()
	if len(library.Snapshot()) != 0 {
		t.Fatal("the action did not remove them")
	}
}
