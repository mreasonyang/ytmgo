package main

import (
	"github.com/egoist/mygo/ui"
	"ytmgo/locales"
)

// nativeAbout is About ytmGO, from the Help menu and macOS's app menu: the
// app's own dialog, in the confirmation's frame, rather than the system's
// alert or About panel. The icon, the wordmark (its "GO" coral, §4.2), the
// build (with DEV, the branch and the commit in development builds), what
// the app is, and one inverted OK.
func nativeAbout(c *ui.Context, m *UIModel, a ViewActions, p nativePalette, width float32) {
	if !m.About {
		return
	}
	open := true
	ui.DialogBase(c, &open, func(backdrop, panel ui.Element) {
		backdrop.Background(p.scrim)
		panel.Width(min(360, nativeGridFloor(width-48))).Padding(32, 24, 24).Radius(tokenRadiusSheet).Background(p.surface).Border(1, p.line).Label(a.Tr("menuAbout"))
		if !p.dark {
			panel.Shadow(tokenShadowDialog.args(p))
		}
		nativeAppear(c, panel, m.instantMotion)
		panel.Children(func() {
			ui.Column(c).Gap(24).AlignItems(ui.Center).Children(func() {
				ui.Column(c).Gap(12).AlignItems(ui.Center).Children(func() {
					if a.AppIcon != nil {
						ui.Image(c, a.AppIcon).Size(96, 96).Label(locales.AppName())
					}
					size := float32(tokenSizeH2)
					ui.Row(c).Role(ui.RoleHeading).Label(locales.AppName()).Children(func() {
						nativeDisplay(ui.Text(c, "ytm").FontSize(size), "ytm", size)
						nativeDisplay(ui.Text(c, "GO").FontSize(size).TextColor(p.accentText), "GO", size)
					})
					ui.Text(c, m.Version).FontSize(tokenSizeMeta).TextColor(p.secondary).TextAlign(ui.Center).Selectable()
				})
				ui.Text(c, a.Tr("nativeAbout")).LineHeight(1.4).TextColor(p.secondary).TextAlign(ui.Center)
				if nativePill(c.Key("about-ok"), a.Tr("buttonOK"), p, true).MinWidth(96).Focus().Clicked() {
					open = false
				}
			})
		})
	})
	if !open {
		m.About = false
	}
}
