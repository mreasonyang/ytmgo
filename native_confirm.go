package main

import "github.com/egoist/mygo/ui"

// nativeConfirmation is a question the app asks before something that
// can't be undone (design/README.md §10, §11): a dialog in the app's own
// look, the sheets' frame, rather than the system's alert, whose accent
// button and type are the desktop's. Its title says what will happen, its
// body names what will be lost, and the action is the one inverted button;
// Cancel, Esc and a click outside keep everything.
type nativeConfirmation struct {
	title, body, action string
	run, cancel         func()
}

// nativeConfirm shows m.confirm while it is set.
func nativeConfirm(c *ui.Context, m *UIModel, a ViewActions, p nativePalette, width float32) {
	q := m.confirm
	if q == nil {
		return
	}
	open, chosen := true, false
	ui.DialogBase(c, &open, func(backdrop, panel ui.Element) {
		backdrop.Background(p.scrim)
		panel.Width(min(400, nativeGridFloor(width-48))).Padding(24).Radius(tokenRadiusSheet).Background(p.surface).Border(1, p.line).Label(q.title)
		// As the sheets: on the dark scrim a shadow can't be seen.
		if !p.dark {
			panel.Shadow(tokenShadowDialog.args(p))
		}
		nativeAppear(c, panel, m.instantMotion)
		panel.Children(func() {
			ui.Column(c).Gap(24).Children(func() {
				ui.Column(c).Gap(8).Children(func() {
					size := float32(tokenSizeH3)
					nativeDisplay(ui.Text(c, q.title).FontSize(size).Role(ui.RoleHeading), q.title, size)
					ui.Text(c, q.body).LineHeight(1.4).TextColor(p.secondary)
				})
				ui.Row(c).Gap(8).Justify(ui.End).Wrap().Children(func() {
					// Cancel comes first and takes the focus: the safe answer.
					if nativePill(c.Key("confirm-cancel"), a.Tr("buttonCancel"), p, false).Focus().Clicked() {
						open = false
					}
					if nativePill(c.Key("confirm-action"), q.action, p, true).Clicked() {
						open, chosen = false, true
					}
				})
			})
		})
	})
	if open {
		return
	}
	m.confirm = nil
	if chosen {
		if q.run != nil {
			q.run()
		}
	} else if q.cancel != nil {
		q.cancel()
	}
}
