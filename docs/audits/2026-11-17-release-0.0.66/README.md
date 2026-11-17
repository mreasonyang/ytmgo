# ytmGO 0.0.66 release verification

[ytmGO 0.0.66](https://github.com/mreasonyang/ytmgo/releases/tag/v0.0.66)
was published on 2026-11-17 from
`29422e3403c85caa1a2d597c90eb05ea5d78b89b`, after
[PR #2](https://github.com/mreasonyang/ytmgo/pull/2) passed its six platform
checks and merged. The merge tree matches the tested PR tree.

## Formal release acceptance

- Apple Silicon and Intel native builds, Developer ID signatures, notarization,
  stapled tickets, Gatekeeper and signed installed-startup checks passed.
- All 22 independently downloaded public files matched GitHub asset digests and
  the published checksum sets. All four platform update archives passed Ed25519
  verification; published build metadata matched the immutable release source.
- An isolated copy of the public 0.0.65 release upgraded through the production
  feed to 0.0.66. Its resulting bundle inventory matched the public 0.0.66 ZIP.
- The Homebrew cask was synchronized to 0.0.66; both architecture ZIP digests
  matched the public release. Native ARM and Intel Homebrew installations,
  helper startup, Apple signatures, tickets and deployment targets passed.
- Windows x64 and Linux x64 native builds, offline runtime tests, installer
  installation and startup in empty account profiles passed on their
  corresponding operating systems.

Machine-readable publication receipts are in
[release-verification.json](release-verification.json). The Homebrew receipt's
`commit` identifies the tap workflow revision; application provenance is bound
through the verified cask ZIP digests and release metadata.

The Windows installer is not Authenticode signed. Windows/Linux native startup
acceptance does not cover signed-in account/audio/UI E2E or long-running use.

The first three publication attempts stopped at notarization credential
validation. A protected remote check subsequently identified HTTP 403. After the
user handled the Apple Developer account agreement, the existing credentials
passed validation and publication attempt four succeeded. No credential rotation
or local certificate setup was needed for that recovery.

## Local candidate acceptance

The local checks below passed on 2026-11-17 on macOS arm64, using source
`63d4483d942e836a839f02ca2be02c7ee4ab3ae9` from `feat/audio-quality-download`.
The tested installation was an ad-hoc signed `0.0.66 DEV` package with updates
disabled. These results precede formal release signing and do not establish
notarization or cross-platform release acceptance.

- `make build-app` and installed bundle signature/resource verification passed.
- `make check` passed, including Go tests, native runtime, login and license
  checks, vet, and 300 messages in four locales.
- `go test -race -json ./...` passed: 1,788 tests/subtests, no failures,
  41 explicit skips. Skips are not counted as successful live acceptance.
- Public catalog and timed-lyrics tests passed. Three consecutive native audio
  loads, 30 seconds of sustained playback, pause, seek, resume and EOF restart
  passed.
- Official-MV content and video-during-audio tests passed. The content probe
  observed advancing 1280 x 720 frames and visual changes at two positions.
- A real encrypted download completed. A fresh process under a macOS sandbox
  denying outbound IP traffic played and sought that audio before account
  initialization.
- Installed desktop interaction covered search, lyrics, queue, pause/seek,
  next track, automatic next track at EOF, video, Immersive Mode, download
  pause/resume/completion/removal, settings, Explore and album navigation.
- A real exit/restart restored the same 12-entry queue, selected index and
  12.128-second paused bookmark, plus language, appearance and audio quality.
  The completed download survived restart and played through the Downloads UI.
- The packaged Google login window opened and cancellation returned normally.
  No credentials were entered.

The temporary download and acceptance request were removed. Default appearance,
language and audio quality were restored. No product implementation was changed
during this acceptance run. Numeric receipts and reproduction commands are in
[local-e2e.json](local-e2e.json).

Real signed-in account/Premium flows, physical media keys, sleep/wake, long-run
stability and other platforms were not covered locally. Audio verification used
the native playback clock and decoder state, not speaker audibility. Formal
release CI acceptance is recorded separately above.
