# ytmGO's copy of MyGo's updater

This is `github.com/egoist/mygo/plugins/updater` from MyGo v0.8.0 (MIT,
see LICENSE), copied so the update window can follow ytmGO's design
guidebook: MyGo's window hard-codes its own look, and its frontend is an
internal package an app can't supply. The update logic (checks,
sessions, prompts, skip and later, install, relaunch) and its tests are
MyGo's, unchanged but for import paths and `internal/wait` (from MyGo's
`internal/wait`, used by the tests). ytmGO's changes:

- `native/look.go`: `native.Style`, the app's look for the window.
- `native/window.go`: uses the Look when set (theme, surface, the release
  notes on raised, pill buttons with the default one inverted) and keeps
  its spacing on the 4 px grid.
- `internal/frontend/frontend.go`: `Background` is ytmGO's surface.

When MyGo is upgraded, compare `plugins/updater` with this copy and carry
its fixes over.

---

# updater

The update window of [MyGo](https://github.com/egoist/mygo) apps, in the
manner of Sparkle: it checks for updates in the background, shows the
release notes of a new version and offers to install it, skip it or remind
the user later, downloads it with a progress bar and relaunches the app into
it. It is all Go, with no npm package.

```go
import "ytmgo/internal/updater"

mygo.Use(updater.Plugin) // or updater.New(updater.Options{Interval: ..., Icon: ...})
```

and "Check for Updates…" in the app's menu:

```go
{Label: "My App", Submenu: []*mygo.MenuItem{
	{Role: mygo.RoleAbout},
	updater.MenuItem(),
	mygo.Separator(),
	{Role: mygo.RoleQuit},
}},
```

Apps whose windows all show native UI use `native.Plugin` (package
`ytmgo/internal/updater/native`) instead: the same window
drawn in native UI, with no webview.

It speaks the user's language (English, Chinese, Dutch, French, German,
Italian, Japanese, Korean, Polish, Portuguese, Russian, Spanish, Turkish,
Ukrainian), and `Options.Strings` changes its texts or adds languages.

The app must be built with updates: see
[auto-updates](https://github.com/egoist/mygo/blob/main/docs/updates.md).
[The plugin's documentation](https://github.com/egoist/mygo/blob/main/docs/plugins/updater.md)
lists the options, the languages and the preferences
(`SetAutomaticChecks`, `SetAutomaticDownloads`, and `OnChange` to hear
them change) apps can offer.
