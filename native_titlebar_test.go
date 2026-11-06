package main

import (
	"fmt"
	"slices"
	"testing"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

func TestMainWindowTitleBarFollowsPlatform(t *testing.T) {
	for _, tc := range []struct {
		goos   string
		style  mygo.TitleBarStyle
		height int
	}{
		{"darwin", mygo.TitleBarDefault, 0},
		{"windows", mygo.TitleBarHidden, 0},
		{"linux", mygo.TitleBarHidden, nativeTopbarHeight},
	} {
		style, height := mainWindowTitleBar(tc.goos)
		if style != tc.style || height != tc.height {
			t.Errorf("%s: title bar %q %d; want %q %d", tc.goos, style, height, tc.style, tc.height)
		}
	}
}

// Window controls as Windows 11 and GNOME (buttons on either side) report
// them for a hidden title bar.
var (
	windowsTitleBar   = ui.TitleBar{Height: 32, Right: 138}
	linuxTitleBar     = ui.TitleBar{Height: nativeTopbarHeight, Right: 46}
	linuxLeftTitleBar = ui.TitleBar{Height: nativeTopbarHeight, Left: 104}
)

func nativeTitleBarFixture(bar ui.TitleBar, width, height int) (*UIModel, ViewActions, *ui.Tester, *int) {
	m, a := nativeViewFixture()
	opened := 0
	a.AppMenu = func(menu *ui.Menu) {
		if menu.Item("Playback").Chosen() {
			opened++
		}
	}
	tt := ui.NewTester(func(c *ui.Context) { nativeView(c, m, a) }, width, height)
	tt.SetTitleBar(bar)
	tt.Frame()
	return m, a, tt, &opened
}

// nativeAssertClearOfControls fails when the control labelled label is under
// the window controls of bar.
func nativeAssertClearOfControls(t *testing.T, tt *ui.Tester, bar ui.TitleBar, label string, width int) ui.Rect {
	t.Helper()
	r := nativeAssertVisible(t, tt, label, width, 900)
	if r.Y < bar.Height && (r.X < bar.Left || r.X+r.W > float32(width)-bar.Right) {
		t.Fatalf("%q at %+v is under the window controls %+v", label, r, bar)
	}
	return r
}

func TestNativeTopbarIsTheTitleBar(t *testing.T) {
	for _, tc := range []struct {
		name string
		bar  ui.TitleBar
	}{{"windows", windowsTitleBar}, {"linux", linuxTitleBar}, {"linux-left", linuxLeftTitleBar}} {
		for _, width := range []int{1100, 1440} {
			t.Run(fmt.Sprintf("%s-%d", tc.name, width), func(t *testing.T) {
				_, a, tt, opened := nativeTitleBarFixture(tc.bar, width, 900)
				for _, key := range []string{"shellAppMenu", "shellBrandHome", "shellBack", "shellForward", "shellNavHome", "shellSettings"} {
					r := nativeAssertClearOfControls(t, tt, tc.bar, a.Tr(key), width)
					if r.Y+r.H > nativeTopbarHeight {
						t.Fatalf("%s at %+v left the %v-DIP bar", key, r, nativeTopbarHeight)
					}
				}
				menu := nativeAssertClearOfControls(t, tt, tc.bar, a.Tr("shellAppMenu"), width)
				brand, _ := tt.Find(a.Tr("shellBrandHome"))
				if menu.X > brand.X || menu.X > tc.bar.Left+24 {
					t.Fatalf("application menu at %+v does not lead the bar (brand %+v)", menu, brand)
				}
				// The bar only grows as tall as the controls: no extra band.
				if search, ok := tt.Find(a.Tr("shellNavSearch")); !ok || search.Y > nativeTopbarHeight {
					t.Fatalf("search field at %+v (found %v) is not in the top bar", search, ok)
				}
				if err := tt.Click(a.Tr("shellAppMenu")); err != nil {
					t.Fatal(err)
				}
				if got := tt.Menu(); !slices.Equal(got, []string{"Playback"}) {
					t.Fatalf("application menu shows %v", got)
				}
				if err := tt.ChooseMenuItem("Playback"); err != nil {
					t.Fatal(err)
				}
				if *opened != 1 {
					t.Fatalf("application menu item chosen %d times; want once", *opened)
				}
				nativeSnapshot(t, tt, fmt.Sprintf("titlebar-%s-%d", tc.name, width))
			})
		}
	}
}

func TestNativeCompactTitleBarKeepsSearchRoom(t *testing.T) {
	const (
		inline = "inline" // the menu button leads the compact bar
		strip  = "strip"  // the menu button and the controls get a strip
		none   = "none"   // tall controls fit, the menu button does not
	)
	for _, tc := range []struct {
		name  string
		bar   ui.TitleBar
		width int
		menu  string
	}{
		{"windows-380", windowsTitleBar, 380, strip},
		{"windows-420", windowsTitleBar, 420, strip},
		{"windows-700", windowsTitleBar, 700, inline},
		{"linux-380", linuxTitleBar, 380, none},
		{"linux-500", linuxTitleBar, 500, inline},
		{"linux-left-420", linuxLeftTitleBar, 420, none},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m, a, tt, _ := nativeTitleBarFixture(tc.bar, tc.width, 900)
			m.SignedIn = true
			tt.Frame()
			back := nativeAssertClearOfControls(t, tt, tc.bar, a.Tr("shellBack"), tc.width)
			settings := nativeAssertClearOfControls(t, tt, tc.bar, a.Tr("shellSettings"), tc.width)
			got := none
			if r, ok := tt.Find(a.Tr("shellAppMenu")); ok {
				nativeAssertClearOfControls(t, tt, tc.bar, a.Tr("shellAppMenu"), tc.width)
				got = inline
				if r.Y+r.H <= back.Y {
					got = strip
				}
			}
			if got != tc.menu {
				t.Fatalf("application menu %s; want %s", got, tc.menu)
			}
			if got == strip && back.Y > tc.bar.Height+16 {
				t.Fatalf("compact bar at %+v sits below a strip taller than the controls", back)
			}
			if room := settings.X - back.X - back.W; room < nativeCompactSearchMin {
				t.Fatalf("search field has %v DIPs; want at least %v", room, nativeCompactSearchMin)
			}
			nativeSnapshot(t, tt, "titlebar-compact-"+tc.name)
		})
	}
}

func TestNativeImmersiveHeaderIsTheTitleBar(t *testing.T) {
	for _, tc := range []struct {
		name  string
		bar   ui.TitleBar
		width int
	}{
		{"windows-1440", windowsTitleBar, 1440},
		{"linux-1100", linuxTitleBar, 1100},
		{"windows-380", windowsTitleBar, 380},
		{"linux-left-700", linuxLeftTitleBar, 700},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m, a, tt, _ := nativeTitleBarFixture(tc.bar, tc.width, 900)
			m.Immersive = true
			tt.Frame()
			for _, key := range []string{"shellAppMenu", "shellExitImmersive", "shellSettings"} {
				r := nativeAssertClearOfControls(t, tt, tc.bar, a.Tr(key), tc.width)
				if r.Y > 24 {
					t.Fatalf("%s at %+v is not in the window's top band", key, r)
				}
			}
			nativeSnapshot(t, tt, "titlebar-immersive-"+tc.name)
			if err := tt.Click(a.Tr("shellExitImmersive")); err != nil {
				t.Fatal(err)
			}
			if m.Immersive {
				t.Fatal("exit immersive did not leave the immersive view")
			}
		})
	}
}

// Without a hidden title bar (macOS, and full screen) and without an
// application menu (macOS) the bars are as they were.
func TestNativeTopbarWithoutHiddenTitleBar(t *testing.T) {
	for _, width := range []int{420, 1440} {
		m, a := nativeViewFixture()
		tt := ui.NewTester(func(c *ui.Context) { nativeView(c, m, a) }, width, 900)
		if _, ok := tt.Find(a.Tr("shellAppMenu")); ok {
			t.Fatalf("%d: application menu button shown without AppMenu", width)
		}
		m.Immersive = true
		tt.Frame()
		if r, ok := tt.Find(a.Tr("shellExitImmersive")); !ok || r.Y < 8 {
			t.Fatalf("%d: immersive header at %+v left its panel", width, r)
		}
	}
}

func TestNativeMenuItemsMirrorTheMenuBar(t *testing.T) {
	var log []string
	click := func(name string) func(*mygo.MenuItem, *mygo.Window) {
		return func(*mygo.MenuItem, *mygo.Window) { log = append(log, name) }
	}
	menu := mygo.NewMenu([]*mygo.MenuItem{
		{Role: mygo.RoleAppMenu, Submenu: []*mygo.MenuItem{{Role: mygo.RoleServices}}},
		{Label: "File", Submenu: []*mygo.MenuItem{{Label: "Show", Accelerator: "CmdOrCtrl+1", Click: click("show")}, {Role: mygo.RoleClose}, mygo.Separator(), {Role: mygo.RoleQuit}}},
		{Role: mygo.RoleEditMenu},
		{Label: "Language", Submenu: []*mygo.MenuItem{
			{Label: "English", Type: mygo.MenuItemRadio, Checked: true, Click: click("en")},
			mygo.Separator(), mygo.Separator(),
			{Label: "Hidden", Hidden: true, Click: click("hidden")},
		}},
		mygo.Separator(),
		{Label: "Help", Submenu: []*mygo.MenuItem{{Label: "About", Click: click("about")}}},
	})
	roles := func(role mygo.MenuRole) func() {
		if role == mygo.RoleQuit {
			return func() { log = append(log, "quit") }
		}
		return nil
	}
	quit := menu.Items()[1].Submenu[3].Label
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Button(c, "Menu").Menu(func(m *ui.Menu) {
			nativeMenuItems(m, menu.Items(), roles, func(it *mygo.MenuItem) { it.Click(it, nil) })
		})
	}, 300, 200)
	for _, tc := range []struct {
		path []string
		want string
	}{{[]string{"File", "Show"}, "show"}, {[]string{"File", quit}, "quit"}, {[]string{"Language", "English"}, "en"}, {[]string{"Help", "About"}, "about"}} {
		if err := tt.Click("Menu"); err != nil {
			t.Fatal(err)
		}
		// The Edit menu has nothing to edit and the app menu is macOS's.
		if got, want := tt.Menu(), []string{"File", "Language", "-", "Help"}; !slices.Equal(got, want) {
			t.Fatalf("menu %v; want %v", got, want)
		}
		log = nil
		if err := tt.ChooseMenuItem(tc.path...); err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(log, []string{tc.want}) {
			t.Fatalf("%v ran %v; want %s", tc.path, log, tc.want)
		}
	}
}

func TestNativeAccelerator(t *testing.T) {
	for _, tc := range []struct {
		accel string
		mods  ui.Modifiers
		key   ui.Key
		ok    bool
	}{
		{"CmdOrCtrl+Shift+H", ui.Cmd | ui.Shift, ui.KeyH, true},
		{"CmdOrCtrl+1", ui.Cmd, ui.Key1, true},
		{"CmdOrCtrl+Right", ui.Cmd, ui.KeyRight, true},
		{"CmdOrCtrl+/", ui.Cmd, ui.KeySlash, true},
		{"F11", 0, ui.KeyF11, true},
		{"Alt+F4", ui.Alt, ui.KeyF4, true},
		{"", 0, 0, false},
		{"F13", 0, 0, false},
		{"Hyper+K", 0, 0, false},
	} {
		mods, key, ok := nativeAccelerator(tc.accel)
		if mods != tc.mods || key != tc.key || ok != tc.ok {
			t.Errorf("%q: %v %v %v; want %v %v %v", tc.accel, mods, key, ok, tc.mods, tc.key, tc.ok)
		}
	}
}
