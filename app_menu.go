package main

import (
	"strconv"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// mainWindowTitleBar hides the main window's title bar on Linux and Windows,
// where the view's top bar takes its place and its application menu button
// the menu bar's (see native_titlebar.go). macOS keeps its title bar, and
// its menu bar is the app's, at the top of the screen.
func mainWindowTitleBar(goos string) (mygo.TitleBarStyle, int) {
	if goos == "linux" || goos == "windows" {
		// The window controls take the top bar's height: GTK centres its
		// title buttons in it, and Windows' caption buttons fill it, as in
		// WinUI's tall title bars, so their glyphs line up with the bar's.
		return mygo.TitleBarHidden, nativeTopbarHeight
	}
	return mygo.TitleBarDefault, 0
}

// appMenu fills the view's application menu button with the menu bar's
// menus, which a hidden title bar leaves no room for. Alt and F10 still
// open them, and their accelerators keep working.
func (a *desktop) appMenu(m *ui.Menu) {
	nativeMenuItems(m, a.menu().Items(), a.appMenuRole, func(it *mygo.MenuItem) { it.Click(it, a.window) })
}

// appMenuRole returns what an item with a built-in role does from the view's
// menu, or nil for roles it leaves out: editing, which has no text there,
// and those only macOS shows.
func (a *desktop) appMenuRole(role mygo.MenuRole) func() {
	switch role {
	case mygo.RoleQuit:
		return mygo.App.Quit
	case mygo.RoleClose:
		return func() { a.window.Close() }
	case mygo.RoleMinimize:
		return func() { a.window.Minimize() }
	case mygo.RoleZoom:
		return func() {
			if a.window.IsMaximized() {
				a.window.Unmaximize()
			} else {
				a.window.Maximize()
			}
		}
	case mygo.RoleToggleFullScreen:
		return func() { a.window.SetFullScreen(!a.window.IsFullScreen()) }
	}
	return nil
}

// nativeMenuItems adds items of a native menu to a view's menu: their
// labels, check marks and shortcuts, and runs the chosen one's role or
// Click. Items it has nothing to run for are left out, with submenus and
// separators that would be left empty.
func nativeMenuItems(m *ui.Menu, items []*mygo.MenuItem, role func(mygo.MenuRole) func(), choose func(*mygo.MenuItem)) {
	separate, shown := false, false
	for _, it := range items {
		if it.Type == mygo.MenuItemSeparator {
			separate = shown
			continue
		}
		if !nativeMenuItemShows(it, role) {
			continue
		}
		if separate {
			m.Separator()
			separate = false
		}
		shown = true
		if it.Submenu != nil {
			m.Submenu(it.Label, func(sub *ui.Menu) { nativeMenuItems(sub, it.Submenu, role, choose) }).Disabled(it.Disabled)
			continue
		}
		item := m.Item(it.Label).Disabled(it.Disabled)
		if it.Type == mygo.MenuItemCheckbox || it.Type == mygo.MenuItemRadio {
			item.Checked(it.Checked)
		}
		if mods, key, ok := nativeAccelerator(it.Accelerator); ok {
			item.Shortcut(mods, key)
		}
		if item.Chosen() {
			if it.Role != "" {
				role(it.Role)()
			} else {
				choose(it)
			}
		}
	}
}

func nativeMenuItemShows(it *mygo.MenuItem, role func(mygo.MenuRole) func()) bool {
	switch {
	case it.Hidden || it.Type == mygo.MenuItemSeparator:
		return false
	case it.Submenu != nil:
		for _, child := range it.Submenu {
			if nativeMenuItemShows(child, role) {
				return true
			}
		}
		return false
	case it.Role != "":
		return role(it.Role) != nil
	}
	return it.Click != nil
}

// nativeAccelerator reads a menu accelerator such as "CmdOrCtrl+Shift+H"
// as the view's modifiers and key, for the keys ytmgo's menus use.
func nativeAccelerator(accel string) (ui.Modifiers, ui.Key, bool) {
	if accel == "" {
		return 0, 0, false
	}
	parts := strings.Split(accel, "+")
	var mods ui.Modifiers
	for _, part := range parts[:len(parts)-1] {
		switch strings.ToLower(part) {
		case "cmdorctrl", "commandorcontrol":
			mods |= ui.Cmd
		case "ctrl", "control":
			mods |= ui.Ctrl
		case "cmd", "command", "super":
			mods |= ui.Super
		case "shift":
			mods |= ui.Shift
		case "alt", "option":
			mods |= ui.Alt
		default:
			return 0, 0, false
		}
	}
	name := parts[len(parts)-1]
	switch key := strings.ToUpper(name); {
	case len(key) == 1 && key[0] >= 'A' && key[0] <= 'Z':
		return mods, ui.KeyA + ui.Key(key[0]-'A'), true
	case len(key) == 1 && key[0] >= '0' && key[0] <= '9':
		return mods, ui.Key0 + ui.Key(key[0]-'0'), true
	case len(key) >= 2 && key[0] == 'F':
		if n, err := strconv.Atoi(key[1:]); err == nil && n >= 1 && n <= 12 {
			return mods, ui.KeyF1 + ui.Key(n-1), true
		}
	}
	keys := map[string]ui.Key{"left": ui.KeyLeft, "right": ui.KeyRight, "up": ui.KeyUp, "down": ui.KeyDown, "/": ui.KeySlash, "space": ui.KeySpace, "enter": ui.KeyEnter, "escape": ui.KeyEscape, "-": ui.KeyMinus}
	if key, ok := keys[strings.ToLower(name)]; ok {
		return mods, key, true
	}
	return 0, 0, false
}
