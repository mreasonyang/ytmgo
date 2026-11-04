# ytmGO 0.6.5 — 2026-11-05

This release collects the outstanding playback, catalog and video-quality changes with their regression tests and existing audit evidence. Historical reports retain their original build bindings and failed observations; they are not rewritten as successful release acceptance.

## Behavior

- First use defaults to 720p. The player and Settings offer 144p, 240p, 360p, 480p, 720p and 1080p through one immediately saved preference. Track changes, window closure and restart retain the choice. Selecting another quality reloads only the video session against the audio clock.
- Video format selection and decoded frame bounds both follow the preference. A smaller source is not enlarged. A missing requested source prefers a lower available resolution; if only larger sources exist, the smallest is decoded to the selected bounds. Presentation remains at 24fps.
- Media recovery retries only undelivered bytes within bounded ranges and independently validates every media CDN redirect. Account requests continue to reject redirects.
- Native media recovery owns its read deadlines. Midstream failures preserve fixed categories and bounded status metadata instead of becoming normal end-of-track events.
- Video source resolution has its own deadline before the first-frame budget begins. Transient timing-lyrics failures return readable fallback text without retaining it as a completed long-lived cache result.
- Compressed initialization responses have independent wire/expanded limits, and catalog diagnostics retain only fixed categories, allowed stages and numeric measurements. Diagnostic version fields use the actual application version.

The earlier root-cause and repair evidence is in [cache repair](../2026-11-04-cache-core-e2e/fix-verification/README.md) and [first-frame recovery](../2026-11-04-cache-core-e2e/first-frame-followup/README.md).

## Local verification

The release preflight results are recorded in `preflight.json`, with test names and results in the accompanying JSON receipts. Required checks include `make check`, packaging tests, full Go race tests, and actual bundled FFmpeg HD/stall/cancellation checks.

- The public official MV probe produced 1280×720 frames and advanced 142 frames across two moving-clock playback samples. [Numeric evidence](quality/live-720p.json)
- The bundled decoder produced moving 1280×720, 1920×1080 and 640×360 frames, including switching down and back up.
- Four-language native UI tests cover Settings and player controls in compact and wide windows, without changing audio playback state. [Settings](quality/video-quality-settings-1440.png) · [Compact player](quality/video-quality-player-380.png)
- The local candidate was installed using the same Developer ID requirement as the existing app. Premium account restoration, English, dark appearance, volume and the paused bookmark were observed after launch. This was a local 0.6.4 candidate carrying the quality changes, not the public 0.6.5 release artifact. [Local verification](quality/local-installed-verification.json)

Temporary recordings, original captures, compiled diagnostic probes and local application backups remain outside the committed release source. The staged text scan reports matching metadata only and does not claim complete detection coverage for every kind of secret or binary/image content.

## Release acceptance

The macOS workflow builds arm64 and amd64 separately, signs/notarizes/staples both apps and DMGs, and requires native installed startup acceptance on both CPUs before publication. Windows and Linux supplement the same immutable released source through the existing desktop workflow. Public upgrade and Homebrew acceptance are recorded separately after publication.

The public [v0.6.5 release](https://github.com/mreasonyang/ytmgo/releases/tag/v0.6.5) uses immutable application source `526374e877eab14a9ef68eb558a0168325926bef`. All four update targets and the macOS ARM/Intel, Windows x64 and Linux x64 installation packages have been published. [Public asset inventory](acceptance/published-release.json)

| Verification | Result and evidence |
| --- | --- |
| macOS native packages | [Release workflow](https://github.com/mreasonyang/ytmgo/actions/runs/41882731718) passed ARM and Intel build, tests, deployment checks, Developer ID signing, notarization, stapling and signed installed startup. [ARM receipt](acceptance/signed-native-arm64.json) · [Intel receipt](acceptance/signed-native-amd64.json) |
| Public macOS packages | Both independently downloaded public ZIPs passed SHA-256, Apple signature, stapled ticket, Gatekeeper, Ed25519 update signature, architecture and macOS 13.0 minimum checks. [Receipt](acceptance/public-verification.json) |
| Public bundled decoder | The public ARM release's FFmpeg passed all seven HD/stall/cancellation checks, with no failures or skips. [Results](acceptance/public-decoder-checks.json) · [Cases](acceptance/public-decoder-cases.json) |
| Public upgrade | [0.6.4 → 0.6.5 ARM upgrade](https://github.com/mreasonyang/ytmgo/actions/runs/41888097689) installed through the production feed and matched an independently downloaded 0.6.5 ZIP. [Receipt](acceptance/public-upgrade.json) |
| Homebrew | [Cask update](https://github.com/mreasonyang/homebrew-taps/actions/runs/41888106311) and [actual ARM/Intel installs](https://github.com/mreasonyang/homebrew-taps/actions/runs/41888535051) passed. [ARM receipt](acceptance/homebrew-arm64.json) · [Intel receipt](acceptance/homebrew-amd64.json) |
| Local official installation | Homebrew upgraded `/Applications/ytmGO.app` to the public 0.6.5 artifact; signature, Gatekeeper and independent file inventory matched. Premium restoration, English, Dark, volume `0.6591679453849792`, paused bookmark `93.5894255s` and the user's saved **1080p** preference remained. [Install](acceptance/local-official-install.json) · [Observed UI](acceptance/local-official-ui.json) |
| Native login/runtime suites | [Three-platform login suite](https://github.com/mreasonyang/ytmgo/actions/runs/41883025829) passed. Windows/Linux runtime checks passed in the [original runtime run](https://github.com/mreasonyang/ytmgo/actions/runs/41883025814); [Mac runtime on Xcode 27](https://github.com/mreasonyang/ytmgo/actions/runs/41888875948) passed after routing repair. [Routing evidence](acceptance/source-and-ci-routing.json) |
| Windows/Linux packages | [Native package workflow](https://github.com/mreasonyang/ytmgo/actions/runs/41887976992) passed NSIS/DEB installation, installed-resource and empty-profile startup checks, signed update archives and publication. All eleven public desktop assets were downloaded independently; SHA-256 checksums, immutable source metadata and both Ed25519 update signatures matched. [Workflow](acceptance/desktop-package-status.json) · [Public verification](acceptance/public-desktop-verification.json) |

Two `macos-latest` runtime attempts were canceled after the hosted ARM runner could not acquire a machine; neither executed any steps. Commit `6d38d94875c940bff1ef8c7685581ce5513c64f7` routes the default Mac runtime check to the validated `xcode-27` runner, while preserving the optional `macos-latest` input. The fallback used unchanged application source; intervening changes were README documentation and workflow routing.

The first Windows packaging attempt failed during dependency metadata preparation with the fixed category `dependency metadata command failed`. Rerunning that Windows job against the same immutable source passed preparation, offline runtime tests, packaging and installed startup; the already successful Linux job was retained. No upstream cause beyond the observed preparation failure is asserted. [Original failure](acceptance/desktop-package-attempt-1-status.json)

The committed-text credential-shape scan found no matches in 416 text files. It reports metadata only and is not proof of coverage for every secret form or image/binary content. [Receipt](acceptance/committed-auth-shape-scan.json)

Physical global shortcuts, sleep/wake, macOS 13/14 hardware, signed-in Intel/Windows/Linux playback and long-running acceptance remain unverified in this release.
