package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"runtime"
	"sync"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/plugins/updater"
	updatenative "github.com/egoist/mygo/plugins/updater/native"
	"ytmgo/locales"
)

var updaterSetup sync.Once

func setupUpdater() {
	if buildKind != "release" {
		return
	}
	updaterSetup.Do(func() { mygo.Use(updatenative.Plugin) })
}

func applicationVersion() string {
	return currentBuildIdentity().Version
}

func applicationBuildLabel() string { return currentBuildIdentity().displayVersion() }

func updateMenuItem() *mygo.MenuItem {
	if buildKind != "release" {
		return &mygo.MenuItem{Label: locales.MenuDevelopmentUpdatesDisabled(), Disabled: true}
	}
	setupUpdater()
	return updater.MenuItem()
}

// applicationMenuWithUpdates is macOS's app menu. Its About opens the app's
// own About (nativeAbout), as the Help menu's does, not the system panel.
func applicationMenuWithUpdates(newUpdateItem func() *mygo.MenuItem, showAbout func()) *mygo.MenuItem {
	about := &mygo.MenuItem{Label: locales.MenuAbout(), Click: func(*mygo.MenuItem, *mygo.Window) { showAbout() }}
	return &mygo.MenuItem{Role: mygo.RoleAppMenu, Submenu: []*mygo.MenuItem{
		about, newUpdateItem(), mygo.Separator(),
		{Role: mygo.RoleServices}, mygo.Separator(), {Role: mygo.RoleHide},
		{Role: mygo.RoleHideOthers}, {Role: mygo.RoleUnhide}, mygo.Separator(), {Role: mygo.RoleQuit},
	}}
}

func helpMenuWithUpdates(items []*mygo.MenuItem, newUpdateItem func() *mygo.MenuItem) *mygo.MenuItem {
	// MyGo hides RoleAppMenu and its children outside macOS.
	if runtime.GOOS != "darwin" {
		items = append(items, mygo.Separator(), newUpdateItem())
	}
	return &mygo.MenuItem{Label: locales.MenuHelp(), Submenu: items}
}

type updateCommands struct {
	current string
	enabled func() bool
	check   func(context.Context) (*mygo.Update, error)
	install func(context.Context, *mygo.Update) error
}

// Maintenance commands run before the music app, its single-instance lock or
// account store starts. Installation targets only the executing app bundle.
// This also permits update acceptance in an isolated copy of a released app.
func runUpdateCommand() (bool, int) {
	if len(os.Args) > 1 && os.Args[1] == "--build-info" {
		if len(os.Args) != 2 {
			return true, 1
		}
		_ = json.NewEncoder(os.Stdout).Encode(currentBuildIdentity())
		return true, 0
	}
	return handleUpdateCommand(os.Args[1:], os.Stdout, updateCommands{
		current: applicationBuildLabel(), enabled: updatesEnabled, check: mygo.Updater.Check,
		install: func(ctx context.Context, update *mygo.Update) error { return update.Install(ctx, nil) },
	})
}

func handleUpdateCommand(args []string, out io.Writer, client updateCommands) (bool, int) {
	if len(args) == 0 || (args[0] != "--version" && args[0] != "--check-for-updates" && args[0] != "--install-update") {
		return false, 0
	}
	result := map[string]any{"version": client.current}
	write := func(code int, category string) (bool, int) {
		if category != "" {
			result["error"] = category
		}
		_ = json.NewEncoder(out).Encode(result)
		return true, code
	}
	if len(args) != 1 {
		return write(1, "unexpected_arguments")
	}
	if args[0] == "--version" {
		_, _ = fmt.Fprintln(out, client.current)
		return true, 0
	}
	result["enabled"] = client.enabled()
	if result["enabled"] != true {
		return write(1, "updates_disabled")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	update, err := client.check(ctx)
	cancel()
	if err != nil {
		return write(1, "update_check_failed")
	}
	result["available"] = update != nil
	if update == nil {
		return write(0, "")
	}
	result["available_version"] = update.Version
	if args[0] == "--install-update" {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
		err := client.install(ctx, update)
		cancel()
		if err != nil {
			return write(1, "update_install_failed")
		}
		// macOS replaces the bundle after this process exits. Do not start a
		// music window or load the user's account as a side effect of this CLI.
		result["install_staged"] = true
	}
	return write(0, "")
}
