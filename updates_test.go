package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"runtime"
	"strings"
	"testing"

	"github.com/egoist/mygo"
	"ytmgo/locales"
)

func TestUpdateMenuVisibleOnCurrentPlatform(t *testing.T) {
	a := &desktop{}
	// Use the complete menu after MyGo has applied platform role visibility.
	// Only the updater's own item is a fixture: its translations need App.Run.
	const label = "Check for Updates…"
	menu := a.menuWithUpdates(func() *mygo.MenuItem {
		return &mygo.MenuItem{Label: label, Click: func(*mygo.MenuItem, *mygo.Window) {}}
	})
	var paths [][]*mygo.MenuItem
	var walk func([]*mygo.MenuItem, []*mygo.MenuItem)
	walk = func(items []*mygo.MenuItem, parents []*mygo.MenuItem) {
		for _, item := range items {
			if item.Hidden || item.Disabled {
				continue
			}
			path := append(append([]*mygo.MenuItem(nil), parents...), item)
			if item.Label == label {
				paths = append(paths, path)
			}
			walk(item.Submenu, path)
		}
	}
	walk(menu.Items(), nil)
	if len(paths) != 1 {
		t.Fatalf("%s: got %d reachable update items, want 1", runtime.GOOS, len(paths))
	}
	path := paths[0]
	if len(path) != 2 || path[1].Click == nil {
		t.Fatal("update item must be an actionable child of a top-level menu")
	}
	if runtime.GOOS == "darwin" {
		if path[0].Role != mygo.RoleAppMenu {
			t.Fatal("macOS update item must be in the application menu")
		}
	} else if path[0].Role != "" || path[0].Label != locales.MenuHelp() {
		t.Fatal("Windows/Linux update item must be in the ordinary Help menu")
	}
}

func TestDesktopMenuActionsRemainReachable(t *testing.T) {
	for _, signedIn := range []bool{false, true} {
		a := &desktop{model: UIModel{SignedIn: signedIn}}
		// Rebuilding after an account or language change must retain all entries.
		for range 2 {
			menu := a.menuWithUpdates(func() *mygo.MenuItem {
				return &mygo.MenuItem{Label: "Check for Updates…", Click: func(*mygo.MenuItem, *mygo.Window) {}}
			})
			labels, roles := map[string]bool{}, map[mygo.MenuRole]bool{}
			var walk func([]*mygo.MenuItem)
			walk = func(items []*mygo.MenuItem) {
				for _, item := range items {
					if item.Hidden || item.Disabled {
						continue
					}
					if item.Click != nil || item.Role != "" {
						labels[item.Label], roles[item.Role] = true, true
					}
					walk(item.Submenu)
				}
			}
			walk(menu.Items())
			for _, label := range []string{locales.MenuShowApp(), locales.MenuAbout(), locales.MenuShortcuts(), locales.MenuSignIn(), locales.MenuPlayPause(), locales.MenuNext(), locales.MenuPrevious(), locales.MenuLike(), locales.MenuVolumeUp(), locales.MenuVolumeDown(), locales.MenuMute(), locales.MenuHome(), locales.MenuExplore(), locales.MenuLibrary(), locales.MenuPanel(), locales.MenuLyrics(), locales.MenuImmersive(), locales.MenuSidebar(), locales.MenuLanguageSystem(), "Check for Updates…"} {
				if !labels[label] {
					t.Errorf("%s signed-in=%t: action %q is unreachable", runtime.GOOS, signedIn, label)
				}
			}
			if labels[locales.MenuSignOut()] != signedIn {
				t.Error("sign-out visibility does not match account state")
			}
			for _, role := range []mygo.MenuRole{mygo.RoleQuit, mygo.RoleClose, mygo.RoleMinimize, mygo.RoleToggleFullScreen, mygo.RoleCopy, mygo.RolePaste} {
				if !roles[role] {
					t.Errorf("%s: required menu role %s is unreachable", runtime.GOOS, role)
				}
			}
		}
	}
}

func TestUpdateCommands(t *testing.T) {
	for _, test := range []struct {
		name, arg                                    string
		enabled, available, checkError, installError bool
		code, checks, installs                       int
		category                                     string
	}{
		{name: "version", arg: "--version"},
		{name: "disabled", arg: "--check-for-updates", code: 1, category: "updates_disabled"},
		{name: "current", arg: "--check-for-updates", enabled: true, checks: 1},
		{name: "available", arg: "--check-for-updates", enabled: true, available: true, checks: 1},
		{name: "check_failure", arg: "--check-for-updates", enabled: true, checkError: true, checks: 1, code: 1, category: "update_check_failed"},
		{name: "install", arg: "--install-update", enabled: true, available: true, checks: 1, installs: 1},
		{name: "install_failure", arg: "--install-update", enabled: true, available: true, installError: true, checks: 1, installs: 1, code: 1, category: "update_install_failed"},
		{name: "no_install_when_current", arg: "--install-update", enabled: true, checks: 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			var out bytes.Buffer
			checks, installs := 0, 0
			client := updateCommands{current: "0.5.0-rc.1", enabled: func() bool { return test.enabled },
				check: func(ctx context.Context) (*mygo.Update, error) {
					checks++
					if _, ok := ctx.Deadline(); !ok {
						t.Fatal("unbounded check")
					}
					if test.checkError {
						return nil, errors.New("private fixture diagnostic")
					}
					if test.available {
						return &mygo.Update{Version: "0.5.0"}, nil
					}
					return nil, nil
				}, install: func(ctx context.Context, update *mygo.Update) error {
					installs++
					if _, ok := ctx.Deadline(); !ok {
						t.Fatal("unbounded install")
					}
					if update.Version != "0.5.0" {
						t.Fatal("wrong update")
					}
					if test.installError {
						return errors.New("private fixture diagnostic")
					}
					return nil
				}}
			handled, code := handleUpdateCommand([]string{test.arg}, &out, client)
			if !handled || code != test.code || checks != test.checks || installs != test.installs {
				t.Fatalf("unexpected command result: %v %d checks=%d installs=%d", handled, code, checks, installs)
			}
			if strings.Contains(out.String(), "private fixture") {
				t.Fatal("raw diagnostic leaked")
			}
			if test.arg == "--version" {
				if out.String() != "0.5.0-rc.1\n" {
					t.Fatal("wrong version")
				}
				return
			}
			var result map[string]any
			if err := json.Unmarshal(out.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			if test.category != "" && result["error"] != test.category {
				t.Fatal("missing error category")
			}
			if test.available && !test.checkError && result["available_version"] != "0.5.0" {
				t.Fatal("wrong available version")
			}
			if test.installs == 1 && !test.installError && result["install_staged"] != true {
				t.Fatal("staging not reported")
			}
		})
	}
}

func TestUpdateCommandsDoNotInterceptNormalLaunch(t *testing.T) {
	for _, args := range [][]string{nil, {"--unrelated"}, {"-psn_0_1"}} {
		if handled, _ := handleUpdateCommand(args, &bytes.Buffer{}, updateCommands{}); handled {
			t.Fatal("normal launch intercepted")
		}
	}
	var out bytes.Buffer
	if handled, code := handleUpdateCommand([]string{"--install-update", "extra"}, &out, updateCommands{}); !handled || code != 1 {
		t.Fatal("accepted extra arguments")
	}
}
