# ytmGO 0.6.9 release verification

**PASS for the distribution scope below.** Checked on 2026-11-06.

- Release source: `dfef9af2d8cbe91800fc390480c5433953383e61`, pinned at `release/0.6.9-source`.
- Menu and global-shortcut fix: `b14b90810303f20c8ca40d7116be844f76ca8aee`.
- [macOS release workflow](https://github.com/mreasonyang/ytmgo/actions/runs/42167781850).

## Changes and source checks

Windows and Linux receive **Help → Check for Updates…** using the existing native updater. macOS retains the update action in its application menu. Regression tests inspect the complete menu after MyGo applies platform visibility and ensure exactly one update entry is reachable.

Global playback bindings now reserve platform menu shortcuts: Windows/Linux F11, Windows Ctrl+Y, and macOS Control+Command+F, Option+Command+H and Option+Shift+Command+V. Tests cover menu accelerators, recorder rejection and fallback from previously saved conflicts. Other important navigation, playback, account, editing and window actions remain reachable.

The 0.6.9 source passed the complete three-platform Go suite, app compilation and login-helper checks ([run](https://github.com/mreasonyang/ytmgo/actions/runs/42167761561)), plus native playback runtime checks on all three platforms ([run](https://github.com/mreasonyang/ytmgo/actions/runs/42167761436)). Windows/Linux application cross-builds, four-language UI regressions, vet and i18n checks also passed. Release-script tests passed all 40 cases.

## Unpublished 0.6.8 failure and correction

[0.6.8 release attempt](https://github.com/mreasonyang/ytmgo/actions/runs/42166620806) failed in the Intel **Go tests and vet** step before signing or publication. `TestAccessRecoveryBudgetReturnsAfterHealthyPlayback` waited for 200 ms of actual audio progress under a two-second wall-clock deadline and reported `player lifecycle transition timed out` at `access_recovery_test.go:152`.

The recovery-policy tests now supply controlled heard-progress samples after the real recovery transition. They check the exact production threshold, reject premature replenishment, verify paused/seeked playback cannot replenish the budget, and retain the bounded fresh-resolution checks. Production sampling still reads the native output's heard position. Native playback and package smoke checks remain enabled.

Both affected tests passed 100 consecutive local repetitions each; the complete player race suite passed. The release workflow also repeats both tests 25 times per macOS architecture before the full Go suite. Both ARM and Intel passed that source-test step in the 0.6.9 attempt.

0.6.8 was not published. Its source pin remains associated with its original failed attempt; 0.6.9 uses the corrected immutable source.

## Distribution verification

- [Public 0.6.9 release](https://github.com/mreasonyang/ytmgo/releases/tag/v0.6.9): 22 macOS, Windows and Linux assets bound to the immutable source above.
- macOS ARM/Intel: signing, notarization, stapling, deployment targets and native installed startup passed. Independently downloaded apps, DMGs and updater archives passed digests, signatures, tickets, Gatekeeper and Ed25519 verification.
- [Windows/Linux packages](https://github.com/mreasonyang/ytmgo/actions/runs/42169471322): native build, legal resources, actual installer/startup checks and signed update archives passed. Independent downloads matched every asset digest and updater signature.
- [Public 0.6.7 to 0.6.9 ARM upgrade](https://github.com/mreasonyang/ytmgo/actions/runs/42169485976): passed; the upgraded bundle matched the public ZIP. The public ARM app reports 0.6.9 with its updater enabled and no newer release available.
- [Homebrew cask update](https://github.com/mreasonyang/homebrew-taps/actions/runs/42169498944): passed, version 0.6.9.
- [Homebrew ARM/Intel installation](https://github.com/mreasonyang/homebrew-taps/actions/runs/42169536145): both passed on attempt 2. Intel's first attempt failed at the stapled-ticket check. The same public Intel package passed independent signature, ticket and Gatekeeper rechecks and prepublication native Intel startup. Fresh-runner verification then passed with the original tag and archive. The cause of this runner discrepancy is not established; its failure receipt is retained. Published assets were not replaced.

Native package startup, menu-model regressions and real-account GUI acceptance are separate scopes. Windows/Linux real-account playback and manual update-window interaction, physical global keys, macOS 13/14 hardware and long-running stability are not covered by this release record.
