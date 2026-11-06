# ytmGO 0.0.61 release verification

**PASS for the distribution scope below.** Checked on 2026-11-06.

The project restarts its public version series at **0.0.61**, retaining the current application features and fixes. The version changed in both the bundle configuration and executable fallback. Desktop release resolution and update signing now accept stable versions outside the former 0.6.x restriction. The Homebrew generator accepts the explicit 0.6.10 to 0.0.61 reset and continues rejecting other downgrades.

- Application release source: `6d31b0a93bfdf60026a4825736f81f783bb7efe9`, pinned at `release/0.0.61-source`.
- Desktop packaging/signing tools: `86d9757aa4d2aedd97e2bd6d15068c5130ed5d44`. The desktop workflow builds the application from the release source while using this separately pinned tools revision.
- Public release: [v0.0.61](https://github.com/mreasonyang/ytmgo/releases/tag/v0.0.61), containing 22 assets.

## Verification

- All 40 release-script tests, nine Homebrew generator tests and the Go update-package/signing tests passed. Regression coverage exercises 0.0.61 release provenance, signed Windows/Linux archives, tampering, invalid version strings and the exact Homebrew reset.
- The latest three-platform Go/login checks and native playback-runtime checks passed. An earlier Windows lifecycle fixture timed out on its first startup, while the next four attempts in that job passed. The latest complete three-platform run passed; the initial category-only receipt is retained without raw logs.
- macOS ARM/Intel native builds, Developer ID signing, Apple notarization/stapling and signed installed startup passed. Independent public downloads passed GitHub asset digests, release checksum files, all four Ed25519 update signatures, and app/DMG Apple signature, ticket and Gatekeeper checks. All packaged Mach-O deployment targets remain macOS 13.0.
- Windows/Linux packages passed native build, actual installer/startup and signed-update checks. Their public assets matched all digests and signatures. Windows installers remain without Authenticode signing.
- Homebrew generated the verified 0.0.61 cask and passed native ARM/Intel installation checks. The `commit` fields in the Homebrew receipts identify the tap workflow revision; public build metadata binds the application to the release source above.
- The local ARM app was reinstalled through Homebrew. Its executable matches the independently downloaded public ZIP, reports 0.0.61, passes Apple signature/ticket/Gatekeeper checks, and starts as a new process. Its updater is enabled and reports no newer release.

This release verifies version numbering and distribution. It does not repeat full real-account playback/UI E2E or establish real hardware acceptance on macOS 13/14, Intel, Windows or Linux.

## Previous releases

All 13 former 0.5.x/0.6.x GitHub Releases were deleted at the operator's request. No backup is retained. Git tags, commits and historical verification records are preserved. The final public release list contains only v0.0.61. No existing-install migration release was requested or created.

## Hosted checks

| Check | Run |
| --- | --- |
| macOS release and signed startup | [42196431078](https://github.com/mreasonyang/ytmgo/actions/runs/42196431078) |
| Windows/Linux packages and signing | [42199311756](https://github.com/mreasonyang/ytmgo/actions/runs/42199311756) |
| Three-platform source/login | [42197715599](https://github.com/mreasonyang/ytmgo/actions/runs/42197715599) |
| Three-platform native playback runtime | [42197715769](https://github.com/mreasonyang/ytmgo/actions/runs/42197715769) |
| Homebrew cask update | [42199392045](https://github.com/mreasonyang/homebrew-taps/actions/runs/42199392045) |
| Homebrew ARM/Intel installs | [42199574961](https://github.com/mreasonyang/homebrew-taps/actions/runs/42199574961) |
