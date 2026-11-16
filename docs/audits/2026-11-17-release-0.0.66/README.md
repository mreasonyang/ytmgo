# ytmGO 0.0.66 release candidate verification

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
release CI must independently build, sign, notarize and smoke-test its artifacts.
