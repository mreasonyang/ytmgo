# ytmGO 0.6.7 release and installed acceptance

**PASS for the verified scope below.** Checked on 2026-11-06.

- Released source: `2a864bf7fc874ae32369fe3c1b634e45a25fb298`.
- Public stable release: [v0.6.7](https://github.com/mreasonyang/ytmgo/releases/tag/v0.6.7), with 22 macOS, Windows and Linux assets.
- Installed app: `/Applications/ytmGO.app`, upgraded through Homebrew from 0.6.5 to 0.6.7. Its executable matches the independently downloaded public ARM archive; Apple signature, stapled ticket and Gatekeeper checks passed.

| Verification | Result |
| --- | --- |
| `make check` on the released source | Passed: Go tests, runtime, login, licensing, vet and 255 messages across four locales |
| `go test -race ./...` on the released source | Passed; root package 166.637 seconds |
| Release script unit tests | 40 passed |
| macOS ARM and Intel | Native build, signed/notarized package, deployment-target and installed startup checks passed |
| Windows and Linux | Native package build, installation/startup, legal resources and signed updates passed |
| Public package bytes | All release asset digests checked; downloaded macOS apps/DMGs/updater archives and desktop packages verified |
| Public updater | Released 0.6.6 upgraded to 0.6.7 from the production feed; installed bundle matched the public archive |
| Homebrew | Cask updated to 0.6.7; native ARM/Intel installation checks and local upgrade passed |

The signed Intel startup job initially failed twice at `Native stapled ticket verification failed`. The same immutable package passed the third native CI attempt. Independent local signature, ticket and Gatekeeper checks passed for both architectures. No source, signature artifact or acceptance requirement was changed for these retries.

The installed macOS ARM app used the existing signed-in account through normal UI actions:

- Short playlist: 98 initial playable rows, then 26 more main tracks, then Suggestions and Related playlists. The saved recommendation cursor remained usable after the main stream ended. A track from the continuation played successfully.
- Long playlist: 96 initial playable rows, then 95 more tracks, followed by repeated native scrolling through the remaining main pages. The last main page had 22 rows; Suggestions and Related playlists appeared after it. Playing the final main track produced a queue of 2,626 playable entries, with selected index 2,618: 2,619 main entries followed by seven suggestions. These are playable row counts after per-response filtering/deduplication, not unique-track counts; the complete queue contained 2,474 distinct video IDs.
- Large-list accessibility child metadata stopped exposing the full list during the test. Further scrolling used coordinates in the observed content area, with screenshots confirming later main pages and the final recommendation boundary. No diagnostic build or UI injection was used.
- Playback continued during pagination. The numeric probe advanced from 14.477 to 192.973 seconds without a playback failure. Seeking through the progress bar resumed playback; pausing held position at 107.952367652 seconds. A real process exit/restart restored that exact paused bookmark, and playback then resumed. Next/previous controls also played the expected queue entries.
- Lyrics displayed, video mode displayed a decoded frame, and returning to Cover kept audio playback advancing. The retained quality preference was 1080p; actual decoded pixel dimensions and semantic lyric/audio alignment were not measured.
- Premium sign-in, English, Dark appearance, volume `0.6591679453849792` and the 1080p preference survived the upgrade. After testing, the original playback queue and paused bookmark at `93.5894255` seconds were restored and observed in the installed app. The temporary probe request was removed or restored to its prior state.

Blank pages, retry behavior, visitor isolation, session invalidation and expired handles are covered by deterministic catalog regressions. The installed tests did not sign out, switch accounts, deliberately interrupt networking or mutate playlists/likes.

This is real-account UI/playback acceptance on this macOS ARM machine. Intel/Windows/Linux real-account playback, physical speaker audibility, macOS 13/14 hardware, global media keys, sleep/wake and long-session stability remain untested.

Receipts contain public distribution metadata, fixed statuses and numeric acceptance data. Account identities, playlist IDs, song lists, credentials and raw service responses are omitted.

| Workflow | Run |
| --- | --- |
| macOS release | [42161382699](https://github.com/mreasonyang/ytmgo/actions/runs/42161382699) |
| Windows/Linux packages | [42163789050](https://github.com/mreasonyang/ytmgo/actions/runs/42163789050) |
| Public 0.6.6 to 0.6.7 upgrade | [42163816318](https://github.com/mreasonyang/ytmgo/actions/runs/42163816318) |
| Homebrew cask update | [42163827858](https://github.com/mreasonyang/homebrew-taps/actions/runs/42163827858) |
| Homebrew native ARM/Intel installs | [42163889340](https://github.com/mreasonyang/homebrew-taps/actions/runs/42163889340) |
