# Installed artwork candidate — 2026-11-01

Result: **PARTIAL**. The two previously blank video thumbnails now render,
including after a cold restart. Liked Music and Discover Mix still show
placeholders in the library/sidebar; their cause was not established by this
package test, so this is not acceptance of every missing cover.

The local 0.6.0 arm64 candidate was built from the current workspace with the
artwork proxy/retry changes. Its Developer ID team matches the previous install
(`2V4TX79LUZ`). App, embedded-code and DMG signature checks, runtime/login
manifests and native-host/FFmpeg smoke checks passed. The signed DMG was mounted
read-only, its app staged and verified, then installed at `/Applications/ytmgo.app`.
The installed executable matches the hash in the mounted candidate. This local
candidate has not been notarized or published.

| Native UI check | Result |
| --- | --- |
| Premium account restoration, first launch and cold restart | PASS; no re-login |
| Saved library | PASS |
| Hong Kong and Marijuana Girl Vol.2 covers | PASS; both launches |
| Daft Punk search artist/song/album artwork | PASS |
| Player bar, immersive large cover and queue covers | PASS |
| Covers entering view after home-page scrolling | PASS after loading |
| Playback and pause | PASS; progress observed through 73.85 seconds |
| Liked Music and Discover Mix library/sidebar covers | FAIL; placeholders remain |

UI evidence consists of native screenshots and accessibility snapshots in this
chat; screenshot files were not exported. Playback was a short smoke check,
not a long-duration stability run. No network settings were changed for these
package checks. Failed-download retry recovery remains supported by the earlier
synthetic tests; no outage was injected into the installed app.

Machine-readable records:

- `package.json`: artifact/source hashes and signing/manifest checks.
- `install-stage.json`: mounted DMG and staged app verification.
- `installation.json`: installed identity and previous-app backup path.
- `installed-recheck.json`: post-restart installed identity and process checks.
- `ui-validation.json`: focused UI results and remaining failures.

The candidate remains installed and open, with playback stopped. The previous
app is retained at `build/artwork-20261101/installed-before/ytmgo.app`.
