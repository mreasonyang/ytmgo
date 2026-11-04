# ytmGO 0.6.3 release and Homebrew acceptance

2026-11-04 (Asia/Shanghai). **PASS for the macOS arm64 release, public upgrade and Homebrew installation.**

[Public v0.6.3 release](https://github.com/mreasonyang/ytmgo/releases/tag/v0.6.3) · [Packaged source a582d25](https://github.com/mreasonyang/ytmgo/commit/a582d25bb3f5b793f4d3e57160db3e4c90f86099) · [Homebrew cask](https://github.com/mreasonyang/homebrew-taps/blob/0b99f3f/Casks/ytmgo.rb)

```sh
brew install --cask mreasonyang/taps/ytmgo
```

Requires **macOS 27+ and Apple Silicon**. The release includes the pending native lyric dragging/layout/audio-clock and AAC/Opus recording-timeline fixes, together with the library/restoration/branding/license changes committed since 0.6.2. Existing unrelated audit files and research/output directories were preserved.

| Check | Result and evidence |
| --- | --- |
| Local source regression | 1,120 Go race cases passed, 28 skipped, zero failures; 19 explicit FFmpeg recording-timeline cases passed without skips. Go vet, translations, runtime, login, licensing and 23 release-script tests passed. Counts include subtests. [Source checks](source-checks.json) |
| CI | Native playback and account-login workflows passed on macOS, Windows and Linux. The macOS package workflow passed its tests, build, Developer ID signing, notarization, stapling and publication. [Workflow receipts](workflows.json) |
| Public files | Independently downloaded all six assets anonymously. GitHub asset digests and `SHA256SUMS` matched; the stable feed matched the versioned feed, and the update archive passed Ed25519 verification. App and DMG signature, stapled ticket and Gatekeeper checks passed. [Verification](public-verification.json) · [Reproduction script](verify_public.py) |
| Previous stable upgrade | An isolated copy of the public 0.6.2 bundle read the production feed and installed 0.6.3 using the maintenance CLI. The updated bundle matched the public 0.6.3 ZIP, including files, permissions and symlinks. This test initialized no account store. [Upgrade receipt](public-upgrade-receipt.json) |
| Homebrew definition | `brew style` and `brew audit --cask --online` passed. The cask pins the public ZIP SHA-256, requires arm64/macOS 27 and retains user data on ordinary uninstall. [Checks](homebrew-checks.json) |
| Real Homebrew installation | Quit the existing local 0.6.2 normally, backed up its app bundle, then ran the installation command above without `--force`. Homebrew registered 0.6.3 and installed `/Applications/ytmGO.app`. All 725 bundle entries matched the public ZIP; signature, ticket, Gatekeeper and CLI version passed. [Installation](homebrew-installation.json) |
| Installed native smoke | Settings displayed 0.6.3, Dark and English. Premium and the saved library restored. The existing 25-entry queue, index 1, 29.765-second bookmark, paused state, zero volume and previous unmuted volume were preserved. The app was left running at its installed path, paused and muted. [Native smoke](native-smoke.json) |
| Installed decoder | The public package's FFmpeg passed 26 cases, zero skips/failures: AAC/fragmented AAC/Opus recording timelines, tail range reads and PCM equality, paused tail/natural end, actual seek and EOF restart. [Decoder checks](installed-decoder-checks.json) |
| Future tap updates | The tap's six-hour scheduled/manual workflow validates public stable-release metadata and checksums without a cross-repository credential. Its first hosted run passed and left the existing 0.6.3 cask unchanged. [Workflow receipts](workflows.json) · [Maintenance](../../HOMEBREW.md) |

The original local app remains backed up at `build/release-0.6.3-install/installed-before/ytmGO.app`. Homebrew now manages the daily installation. Update with `brew update` followed by `brew upgrade --cask --greedy mreasonyang/taps/ytmgo`, or use the app's updater.

Release acceptance covers macOS arm64. This run did not repeat all previous lyric mouse/resize scenarios on the public binary, play account audio through speakers, or validate physical global shortcuts, sleep/wake or long-duration reliability. Windows/Linux CI success is separate from signed desktop-release or real-account execution acceptance on those platforms.

Diagnostics used allowlisted test/workflow metadata and `safe-processes` executable identity fields. No process argv, environment dumps, authentication stores or raw diagnostic logs were collected. The public record omits queue contents and personalized library UI text.
