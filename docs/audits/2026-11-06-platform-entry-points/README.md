# Desktop menu and shortcut reachability review

Source fix: `b14b90810303f20c8ca40d7116be844f76ca8aee`, based on `9cadeb1` (0.6.7). Framework: MyGo 0.7.1.

## Findings and fixes

1. **Manual updates were unreachable on Windows and Linux.** MyGo marks `RoleAppMenu` hidden outside macOS, including its custom children. `updates.go` placed its only update action there. The same code is present in the 0.6.3 tag; this was not introduced by 0.6.6. Windows/Linux now receive a normal Help submenu item using the same native updater callback. macOS keeps the update action in its application menu. Each platform has exactly one reachable update entry.
2. **Custom global playback keys could take over existing menu shortcuts.** The old guard covered Command/Control shortcuts with no extra Control/Alt modifier, omitting platform-specific combinations. Windows/Linux allowed F11 (full screen); Windows also allowed Ctrl+Y (redo). macOS allowed Control+Command+F (full screen), Option+Command+H (hide others), and Option+Shift+Command+V (paste and match style). These combinations are now reserved in the recorder and preference validation. Previously saved conflicts fall back to the default playback shortcut on startup.

The additional menu review found visible alternatives for About, Quit, Close, Minimize, editing, account actions, navigation, playback, volume, panel/lyrics, immersive view, sidebar, shortcuts and language selection. The other hidden application-menu roles provide macOS-specific system behavior. The main window does not enable automatic menu-bar hiding. Settings, account and immersive/compact controls retain their existing UI regression coverage.

## Local validation

- `go test ./... -count=1 -timeout=180s`: passed.
- Final menu, saved-shortcut, recorder and updater command regressions: passed.
- Settings, responsive, account-avatar and immersive UI regressions: passed.
- `go vet ./...`: passed.
- `go tool mygo i18n check`: passed, 255 messages in four languages.
- Windows amd64 and Linux amd64 application cross-builds with `CGO_ENABLED=0`: passed.
- `git diff --check`: passed.

The new accelerator regression first failed on the three macOS conflicts above, then passed after the guard was fixed. Menu tests construct the complete application menu and inspect it after MyGo applies platform visibility; only the updater item is supplied as a fixture so the test does not require an application event loop or contact the release feed.

## Native CI

- [Native account login](https://github.com/mreasonyang/ytmgo/actions/runs/42165977047): **passed on macOS, Windows and Linux**, including the complete Go suite, the new menu/shortcut regressions, application compilation and native helper lifecycle checks.
- [Native playback runtime](https://github.com/mreasonyang/ytmgo/actions/runs/42165977062): macOS passed; Windows and Linux remained in progress at the review snapshot. No playback-runtime code was changed by this fix.

This is source, menu-model and UI regression validation. It does not claim installed Windows/Linux GUI interaction, a real in-place upgrade, hardware audio acceptance or a newly published release. Published 0.6.7 packages still contain the old behavior.
