# ytmGO 0.6.10 track-numbering acceptance

**PASS for the verified scope below.** Checked on 2026-11-06. Release source: `1684e8e8e727c4bbea3dc2a9f01d995bcd366701`.

Consecutive untitled shelves represent successive pages of one track list. Their row indexes now include the lengths of earlier shelves. Named sections reset the offset, and the flat-page fallback still begins at 1. The counter is computed anew for each render; item keys and playback targets are retained.

The native rendering regression failed on the old implementation with `[1 2 1 2 3 1]` and passed with `[1 2 3 4 5 6]`. Tests cover playlist and album routes, wide/compact layouts, empty shelves, multiple continuations, repeated frames, independent named sections, row activation and four-digit label bounds. The released source passed `go test -race ./...` (root package 173.844 seconds), `go vet ./...`, and all 40 release-script tests.

The public ARM app was installed through Homebrew into `/Applications/ytmGO.app`, upgrading 0.6.7 to 0.6.10. Its executable matches the independently downloaded public ZIP, and its signature, stapled ticket and Gatekeeper assessment passed. Settings displayed Version 0.6.10.

Real-account native UI acceptance on this macOS ARM machine observed:

- Short playlist boundary `96, 97, 98, 99, 100 ... 107`; its last main row was numbered `124`, followed by unnumbered Suggestions and Related playlists.
- Long playlist boundary `94, 95, 96, 97, 98, 99, 100 ... 105`; later views showed four-digit numbers around `1233–1244`, `2003–2013` and `2489–2500`, with no wrapping or clipping. The final main track was numbered `2619`, followed by unnumbered recommendation sections.
- Native scrolling and screenshots were used once large-list accessibility child metadata became incomplete. The app was the public signed bundle, with no injected UI or diagnostic build.
- The account remained Premium. English, Dark appearance, 1080p preference, volume `0.6591679453849792` and paused position `93.5894255` seconds were retained. The test used navigation and scrolling only, returned to Home, and left the playback-state file unchanged.

Public macOS ARM/Intel packages passed independent asset digests, Apple signatures, stapled-ticket, Gatekeeper and Ed25519 update verification. Native signed startup and Homebrew installation passed on both architectures. The public 0.6.9 to 0.6.10 upgrade passed.

Windows and Linux native packages passed build, installation/startup, legal-resource and signed-update checks. Independent public downloads matched every digest and Ed25519 update signature. The complete public release contains 22 assets bound to the same immutable source.

This round verifies the numbering fix in the installed macOS ARM app. Blank-page rendering, section reset, compact mode and playback-row activation are covered by deterministic rendering tests. It does not repeat the full real-media acceptance from 0.6.7, validate hardware speaker audibility or lyric/audio semantic alignment, or claim real-account GUI acceptance on Intel, Windows or Linux.

Account identities, playlist IDs, song lists, credentials and raw service responses are omitted from the receipts.

| Workflow | Run |
| --- | --- |
| macOS release | [42172536656](https://github.com/mreasonyang/ytmgo/actions/runs/42172536656) |
| Windows/Linux packages | [42175463426](https://github.com/mreasonyang/ytmgo/actions/runs/42175463426) |
| Public 0.6.9 to 0.6.10 upgrade | [42175528050](https://github.com/mreasonyang/ytmgo/actions/runs/42175528050) |
| Homebrew update | [42175541584](https://github.com/mreasonyang/homebrew-taps/actions/runs/42175541584) |
| Homebrew ARM/Intel installs | [42175634548](https://github.com/mreasonyang/homebrew-taps/actions/runs/42175634548) |
