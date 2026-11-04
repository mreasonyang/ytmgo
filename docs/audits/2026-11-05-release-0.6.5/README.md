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

Physical global shortcuts, sleep/wake, macOS 13/14 hardware, signed-in Intel/Windows/Linux playback and long-running acceptance remain unverified in this release.
