package main

import (
	"context"
	"embed"
	"encoding/json"
	"log"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/i18n"
	"ytmgo/internal/language"
	"ytmgo/internal/navigation"
	"ytmgo/internal/session"
	"ytmgo/locales"
)

const version = "0.3.0"

//go:embed web/shell locales/en.json resources/icon.png resources/tray.png
var assets embed.FS

type desktop struct {
	window           *mygo.Window
	tray             *mygo.Tray
	quitting         bool
	account          *mygo.Window
	prefs            *Preferences
	languagePath     string
	sessionMarker    string
	logoutWindow     *mygo.Window
	logoutBusy       bool
	logoutBlocked    bool
	logoutComplete   bool
	logoutLoading    bool
	logoutNextHTML   string
	signedIn         bool
	accountItem      *mygo.MenuItem
	signOutItem      *mygo.MenuItem
	accountSeparator *mygo.MenuItem
}

func main() {
	// Preserve the CLI's separate packaged production/development identities.
	if !mygo.App.IsPackaged() {
		mygo.App.SetName("ytmgo Dev")
	}
	if !mygo.App.RequestSingleInstanceLock() {
		return
	}
	dataDir, err := mygo.App.Path(mygo.PathUserData)
	if err != nil {
		log.Fatal("ytmgo could not locate its application data directory")
	}
	a := &desktop{prefs: &Preferences{path: filepath.Join(dataDir, "appearance")}, languagePath: filepath.Join(dataDir, "language"), sessionMarker: filepath.Join(dataDir, "signout-pending")}
	mygo.Theme.SetSource(mygo.ThemeSource(a.prefs.Appearance()))
	mygo.App.SetLanguages(language.Supported...)
	a.applyLanguage()
	mygo.App.SetMenu(a.menu())
	mygo.App.OnLocaleChanged(func(string) { a.relocalize() })
	mygo.App.OnBeforeQuit(func(*mygo.QuitEvent) { a.quitting = true })
	mygo.App.OnActivate(func(bool) { a.show() })
	mygo.App.OnSecondInstance(func([]string, string) { a.show() })
	mygo.App.OnWillQuit(func(*mygo.QuitEvent) { mygo.GlobalShortcut.UnregisterAll() })
	mygo.App.WhenReady(func() {
		pending, pendingErr := session.Pending(a.sessionMarker)
		if pending || pendingErr != nil {
			a.clearAccountData()
		} else {
			a.openMusic()
		}
		icon, _ := assets.ReadFile("resources/tray.png")
		a.tray, err = mygo.NewTray(mygo.TrayOptions{ID: "si.kyun.ytmgo.player", Icon: icon, IconIsTemplate: true, ToolTip: locales.AppName(), Menu: a.trayMenu()})
		if err != nil {
			log.Print("ytmgo: tray unavailable; window and Dock controls remain available")
		}
		if err = mygo.GlobalShortcut.Register("CmdOrCtrl+Shift+Space", func() { a.control("toggle") }); err != nil {
			log.Print("ytmgo: global playback shortcut unavailable; use the Playback menu")
		}
	})
	if err = mygo.App.Run(); err != nil {
		log.Fatal("ytmgo could not start its desktop runtime")
	}
}

func (a *desktop) show() {
	if a.logoutBlocked && a.logoutWindow != nil && !a.logoutWindow.IsDestroyed() {
		a.logoutWindow.Show()
		a.logoutWindow.Focus()
		return
	}
	if a.window != nil && !a.window.IsDestroyed() {
		a.window.Show()
		a.window.Restore()
		a.window.Focus()
	}
}

// control forwards one word of the shell's fixed action vocabulary
// (web/shell/main.js runAction) to the music page.
func (a *desktop) control(action string) {
	if a.logoutBlocked {
		a.show()
		return
	}
	if a.window == nil || a.window.IsDestroyed() {
		return
	}
	w := a.window
	encoded, _ := json.Marshal(action)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_, _ = w.Page().EvalContext(ctx, `location.origin === 'https://music.youtube.com' && window.ytmgoControls?.(`+string(encoded)+`)`)
	}()
}

func (a *desktop) external(raw string) {
	if !navigation.External(raw) {
		return
	}
	go func() {
		var err error
		if runtime.GOOS == "darwin" {
			// The workspace's browser preference is Brave. Do not silently choose
			// another browser if it is absent. No URL is printed to diagnostics.
			err = exec.Command("open", "-b", "com.brave.Browser", raw).Run()
		} else {
			err = mygo.Shell.OpenExternal(raw)
		}
		if err != nil {
			_, _ = mygo.Dialog.Message(mygo.MessageOptions{Parent: a.window, Title: locales.LinkErrorTitle(), Message: locales.LinkErrorMessage(), Detail: locales.LinkErrorDetail(), Buttons: []string{locales.ButtonOK()}})
		}
	}()
}

func item(label, shortcut string, fn func()) *mygo.MenuItem {
	return &mygo.MenuItem{Label: label, Accelerator: shortcut, Click: func(*mygo.MenuItem, *mygo.Window) { fn() }}
}

// shell brings the window forward, then runs a shell action.
func (a *desktop) shell(action string) func() {
	return func() { a.show(); a.control(action) }
}

func (a *desktop) menu() *mygo.Menu {
	a.accountItem = item(locales.MenuSignIn(), "", a.shell("account"))
	a.signOutItem = item(locales.MenuSignOut(), "", a.requestSignOut)
	a.signOutItem.Hidden = true
	a.accountSeparator = mygo.Separator()
	a.accountSeparator.Hidden = true
	return mygo.NewMenu([]*mygo.MenuItem{
		{Role: mygo.RoleAppMenu},
		{Label: locales.MenuFile(), Submenu: []*mygo.MenuItem{item(locales.MenuShowApp(), "CmdOrCtrl+1", a.show), {Role: mygo.RoleClose}, mygo.Separator(), {Role: mygo.RoleQuit}}},
		{Role: mygo.RoleEditMenu},
		{Label: locales.MenuPlayback(), Submenu: []*mygo.MenuItem{
			item(locales.MenuPlayPause(), "CmdOrCtrl+P", func() { a.control("toggle") }),
			item(locales.MenuNext(), "CmdOrCtrl+Right", func() { a.control("next") }),
			item(locales.MenuPrevious(), "CmdOrCtrl+Left", func() { a.control("previous") }),
			mygo.Separator(),
			item(locales.MenuLike(), "", func() { a.control("like") }),
			mygo.Separator(),
			item(locales.MenuVolumeUp(), "CmdOrCtrl+Up", func() { a.control("volume-up") }),
			item(locales.MenuVolumeDown(), "CmdOrCtrl+Down", func() { a.control("volume-down") }),
			item(locales.MenuMute(), "CmdOrCtrl+Shift+M", func() { a.control("mute") }),
		}},
		{Label: locales.MenuView(), Submenu: []*mygo.MenuItem{
			item(locales.MenuSearch(), "CmdOrCtrl+K", a.shell("search")),
			item(locales.MenuHome(), "CmdOrCtrl+Shift+H", a.shell("home")),
			item(locales.MenuExplore(), "", a.shell("explore")),
			item(locales.MenuLibrary(), "CmdOrCtrl+L", a.shell("library")),
			mygo.Separator(),
			item(locales.MenuPanel(), "CmdOrCtrl+J", a.shell("panel")),
			item(locales.MenuLyrics(), "", a.shell("lyrics")),
			item(locales.MenuImmersive(), "CmdOrCtrl+Shift+F", a.shell("immersive")),
			item(locales.MenuSidebar(), "CmdOrCtrl+B", a.shell("sidebar")),
			mygo.Separator(),
			{Label: locales.MenuLanguage(), Submenu: a.languageMenu()},
			mygo.Separator(),
			{Role: mygo.RoleReload}, {Role: mygo.RoleResetZoom}, {Role: mygo.RoleZoomIn}, {Role: mygo.RoleZoomOut}, {Role: mygo.RoleToggleFullScreen},
		}},
		{Role: mygo.RoleWindowMenu},
		{Label: locales.MenuAccount(), Submenu: []*mygo.MenuItem{a.accountItem, a.accountSeparator, a.signOutItem}},
		{Label: locales.MenuHelp(), Submenu: []*mygo.MenuItem{
			item(locales.MenuShortcuts(), "CmdOrCtrl+/", a.shell("shortcuts")),
			item(locales.MenuAbout(), "", func() {
				go func() {
					_, _ = mygo.Dialog.Message(mygo.MessageOptions{Parent: a.window, Title: locales.MenuAbout(), Message: locales.AboutMessage(version), Detail: locales.AboutDetail(), Buttons: []string{locales.ButtonOK()}})
				}()
			}),
			item(locales.ServiceName(), "", func() { a.external(navigation.Home) }),
		}},
	})
}

// languageMenu lists "Match System" and each language named in itself.
func (a *desktop) languageMenu() []*mygo.MenuItem {
	choice := language.Load(a.languagePath)
	radio := func(label, value string) *mygo.MenuItem {
		return &mygo.MenuItem{Label: label, Type: mygo.MenuItemRadio, Checked: choice == value, Click: func(*mygo.MenuItem, *mygo.Window) { a.setLanguage(value) }}
	}
	items := []*mygo.MenuItem{radio(locales.MenuLanguageSystem(), language.System), mygo.Separator()}
	for _, tag := range language.Supported {
		items = append(items, radio(i18n.Autonym(tag), tag))
	}
	return items
}

func (a *desktop) trayMenu() *mygo.Menu {
	return mygo.NewMenu([]*mygo.MenuItem{
		item(locales.TrayOpen(), "", a.show), mygo.Separator(),
		item(locales.MenuPlayPause(), "", func() { a.control("toggle") }),
		item(locales.MenuNext(), "", func() { a.control("next") }),
		item(locales.MenuPrevious(), "", func() { a.control("previous") }),
		item(locales.MenuLike(), "", func() { a.control("like") }),
		mygo.Separator(), item(locales.TrayQuit(), "", func() { mygo.App.Quit() }),
	})
}
