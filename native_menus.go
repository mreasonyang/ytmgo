package main

import "github.com/egoist/mygo/ui"

// The app's menus (a song's, the account's) are drawn in the window as its
// other popovers are (design/README.md §11): `raised`, a hairline, the
// popover shadow and radius `control`, their items 36 high with radius
// `art`, washed under the pointer or the keys. The system's menus showed
// in the desktop's look beside the app's own. The application menu and
// text editing keep the system's.

// nativeMenuItem is one entry of an app menu.
type nativeMenuItem struct {
	label    string
	disabled bool // shown, not chosen: a status line
	divider  bool // a hairline before it starts a group
	run      func()
}

// nativeMenuPanel draws parts' menu while it is open.
func nativeMenuPanel(c *ui.Context, parts ui.MenuParts, items []nativeMenuItem, m *UIModel, p nativePalette) {
	parts.Popup(func(panel ui.Element) {
		nativeAppear(c, panel.MinWidth(200).Padding(4).Radius(tokenRadiusControl).Background(p.raised).Border(1, p.line).Shadow(tokenShadowPopover.args(p)), m.instantMotion)
		for _, it := range items {
			if it.divider {
				ui.Box(c).Height(1).Margin(4, 8).Background(p.line)
			}
			item := parts.Item().Height(36).PaddingX(12).Gap(8).Radius(tokenRadiusArt).AlignItems(ui.Center).Label(it.label).TextColor(p.text)
			if it.disabled {
				item.Disabled(true).TextColor(p.secondary)
			} else if item.Highlighted() {
				item.Background(p.hover)
			}
			item.Children(func() { ui.Text(c, it.label).FontSize(tokenSizeSmall).SingleLine() })
			if item.Clicked() && it.run != nil {
				it.run()
			}
		}
	})
}

// nativeContextMenu gives target the app menu items builds, opening where
// it is right-clicked, or for the menu key or Shift+F10.
func nativeContextMenu(c *ui.Context, target ui.Element, m *UIModel, p nativePalette, items func() []nativeMenuItem) {
	open := ui.Local(target, "context-menu", func() bool { return false })
	parts := ui.ContextMenuBase(c, target, open)
	if parts.Open() {
		nativeMenuPanel(c, parts, items(), m, p)
	}
}

// nativeMenuButton is an icon button (nativeIconButton's look) opening the
// app menu items builds below it; open holds whether it shows.
func nativeMenuButton(c *ui.Context, open *bool, name, label string, p nativePalette, hit float32, m *UIModel, disabled bool, items func() []nativeMenuItem) ui.Element {
	parts := ui.MenuBase(c, open)
	b := nativeIconFace(c, parts.Trigger.Disabled(disabled), name, label, p, *open, hit, p.text)
	if parts.Open() {
		nativeMenuPanel(c, parts, items(), m, p)
	}
	return b
}
