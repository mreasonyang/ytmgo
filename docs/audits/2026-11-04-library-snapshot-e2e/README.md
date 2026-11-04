# Library snapshot installed-app E2E — 2026-11-04

**PASS for the library snapshot lifecycle and the application regressions below.**
This is a macOS arm64 installed-app run, not whole-product release certification.
The machine-readable evidence is in
`outputs/library-snapshot-e2e-20261104/report.json`; screenshots and native
accessibility snapshots remain local because they contain personal library data.

The tested `/Applications/ytmgo.app` executable SHA-256 is
`c98389c1af290733459ea57ea4a5123012139fa13c7eae006fb10ee4c8308c17`.
It is a Developer ID signed 0.6.2 local build; no notarization or public release
was performed. Deep strict signature verification and all 154 runtime/login
manifest checks passed.

Two failures were reproduced in the original `71993c3` installation and fixed:

- Offline startup followed by reconnecting the saved account cleared the paused
  playback queue and bookmark. Reconnecting now retains a restored bookmark;
  explicit new sign-in and sign-out retain their existing clearing behavior.
- Library pagination only updated the visible page, leaving the persisted list
  at its initial 25 records. Continuations now update the sidebar, backing page
  and snapshot. An older asynchronous save cannot overwrite a newer full list.

| Installed-app scenario | Result |
| --- | --- |
| Online load, automatic pagination, normal quit | 46 loaded records persisted |
| New process with only its network disabled | All 46 records shown; unverified private entries disabled |
| Restore network and retry account | Premium and library recover; 90-entry queue and 2:42 bookmark retained |
| Refresh while offline, then click Retry online | List retained with failure status; retry succeeds |
| Missing or corrupt JSON cache | No stale list shown; network recovery recreates the cache |
| Unsupported format or mismatched account scope | Snapshot ignored; no migration or legacy reader |
| Saved playlist, Home, Explore, Search, history | Native navigation and results verified |
| Restored track, lyrics, video selection, natural next track | Playback clock advances; next recording starts |
| Pause, keyboard seek, queue panel | Verified against native state and bounded playback samples |
| Light/dark and English/Simplified Chinese | Switched and restored |
| Sign-out confirmation | Canceled; verified account retained |
| End of test | Original queue, selection, bookmark, volume and preferences restored; fault proxy stopped |

After the fixes, `go test -race -count=1 -timeout=300s ./...` passed 1,053 test
events, with 20 opt-in test events skipped. `go vet ./...` and translation
validation passed. Offline native Rust/Go protocol tests, 19 Python runtime
tests, 10 login tests, 23 release tests and 24 diagnostic tests passed.
Anonymous live catalog, continuation and timed-lyrics checks passed separately.
Both complete anonymous recordings decoded through the installed runtime with
nonzero finite PCM, EOF and duration errors below 0.5 seconds.

Actual logout and switching between two real accounts were not executed.
Selection changes, logout clearing and late-write isolation have synthetic race
coverage. CUA pointer dragging and accessibility value setting did not produce a
verified seek target change; keyboard seek passed, while pointer-drag E2E remains
open. Physical sleep/wake, physical shortcut input, multi-day stability and
Windows/Linux runtime acceptance are outside this run.
