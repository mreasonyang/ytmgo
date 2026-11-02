# Installed system playlist artwork — 2026-11-02

Result: **PASS for Liked Music and Discover Mix in `/Applications/ytmgo.app`.**

The user still saw placeholders because the November 1 system-cover candidate
was tested as a standalone app and the installed application was preserved.
The old installed binary was reproduced with placeholders for both entries.
This follow-up built from `b6742fb` plus the four artwork/parser source and test
files, preserving the committed account-avatar and saved-library restoration
updates. The same-team Developer ID signed DMG was mounted read-only, its app
verified and staged, and the previous installation retained under
`build/installed-artwork-20261102/installed-before/ytmgo.app` before replacement.
The installed executable matches the mounted package and the process executes
from `/Applications/ytmgo.app/Contents/MacOS/ytmgo`.

On its first cold launch, Premium, the saved library and the account avatar
restored. Liked Music displays the purple/pink thumbs-up cover in the sidebar,
home shortcut and library. Discover Mix displays its generated blue/purple
branded cover in the sidebar and library, and its full generated image in the
detail header. The app is left running on that detail page with playback idle.

Focused artwork/catalogue race tests, isolated `go vet ./...`, manifest hashes,
nested app signature verification, script-host `check` and FFmpeg execution
passed. The package is signed locally; it is not notarized or publicly released.
This is focused artwork acceptance; no additional playback acceptance is claimed.

- `package.json`: base commit, changed source hashes, package identity and checks.
- `installation.json`: old/new installed identity, backup and mounted-package match.
- `ui-validation.json`: installed native UI observations.

Native screenshots and accessibility state are in this chat. No screenshot
files, raw account responses, account credentials or proxy configuration are
included in this evidence.
