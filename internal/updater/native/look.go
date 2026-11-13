package native

import "github.com/egoist/mygo/ui"

// Look is an app's own look for the update window, in place of MyGo's (a
// copy kept by ytmGO for this hook; see ../LICENSE): ytmGO's design
// guidebook gives it the app's theme, its surface, the release notes on
// raised, and pill buttons, the default one inverted. Style nil keeps
// MyGo's look.
type Look struct {
	Theme                             *ui.Theme
	Background, Panel, Line           ui.Color // the window, the release notes, their hairline
	Primary, PrimaryText, PrimaryOver ui.Color // the default button, and under the pointer
	Button, ButtonOver                ui.Color // the other buttons' fill
	Title, Small                      float32  // font sizes of the title and the fine print
	PillHeight, PillRadius            float32
	NotesRadius                       float32
	Press                             func(ui.Element) // a button's press feedback
}

// Style returns the window's Look for a dark or light appearance. Set it
// before the plugin is used.
var Style func(dark bool) Look
