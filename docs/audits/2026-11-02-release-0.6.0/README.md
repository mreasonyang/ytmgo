# ytmgo 0.6.0 release — 2026-11-02

Result: **PASS — published stable release and public 0.5.1 → 0.6.0 upgrade.**

- Release: https://github.com/mreasonyang/ytmgo/releases/tag/v0.6.0
- Source/tag: `c15f02a005f25f97d3dd09e355119dad2a3c58cc`.
- Build/sign/notarize/publish: https://github.com/mreasonyang/ytmgo/actions/runs/41380657462
- Public upgrade: https://github.com/mreasonyang/ytmgo/actions/runs/41381505029
- Upgrade receipt artifact: `ytmgo-public-upgrade-receipt-3`.

The package workflow passed its Go, runtime/login, translation and packaging
checks, built the App, completed Developer ID signing, Apple notarization,
stapling, Gatekeeper checks and Ed25519 update signing, verified the uploaded
asset digests, then published all six assets. The tag resolves to the exact
source commit above. Public metadata downloads without authentication passed;
the stable update feed matches the versioned 0.6.0 feed, and GitHub asset
digests match the published checksum list. Release notes describe the shipped
changes and preserve the source link.

The remote upgrade check ran on macOS 27.0.1/arm64 using the public 0.5.1 ZIP
and production update feed. It downloaded and installed the signed update,
then checked the 0.6.0 version, Apple signature, stapled ticket and Gatekeeper.
The resulting bundle matches an independently downloaded public 0.6.0 ZIP,
including file bytes, permissions and symbolic links. The receipt records the
source and asset identities. It used isolated bundles and maintenance commands;
no account store was initialized and the local installed app was not replaced.
This release check does not repeat native updater-window, account or playback
acceptance. Earlier installed local artwork acceptance is recorded separately
in `../2026-11-02-installed-system-artwork/`.

Evidence contains only public package metadata and structured workflow/upgrade
results. No raw diagnostic logs or signing credentials were retrieved.
