# ytmGO 0.0.62 release and native UI upgrade verification

**PASS for release distribution and the actual installed macOS ARM UI upgrade.** Checked on 2026-11-06.

The maintenance release advances 0.0.61 to 0.0.62 to test the production update flow. Application features and updater implementation are unchanged.

- Release application and packaging source: `a91f2bd8c0bf6f029c90f1120aa93cea132aff7d`.
- Public stable release: [v0.0.62](https://github.com/mreasonyang/ytmgo/releases/tag/v0.0.62), containing 22 assets.
- Baseline: the real `/Applications/ytmGO.app` at 0.0.61, whose executable matched the previously verified public 0.0.61 build.

## Observed UI flow

1. Before publication, **Check for Updates…** reported that 0.0.61 was the newest available version.
2. After publication, the same installed app offered **0.0.62**, explicitly showing the current version as **0.0.61**.
3. **Install Update** displayed **Downloading update…**, followed by **Ready to Relaunch**.
4. **Relaunch Now** closed the old process and automatically launched the updated application. A read-only executable-identity observer recorded this before the UI tool rebound to the new process.
5. The new Settings page displayed **Version 0.0.62**. Checking again reported that **0.0.62 was the newest available version**.

Premium login, paused position, volume, Dark appearance, English language and 1080p video preference were preserved. The position and volume comparison uses the live UI immediately before the update; the earlier preparation snapshot was captured before the release build finished and is not used for this comparison.

The updated bundle matches the independent public ARM ZIP across all files, modes and symbolic links. The installed version and CLI version both report 0.0.62; Apple signature, stapled notarization ticket and Gatekeeper checks pass. See [ui-upgrade.json](ui-upgrade.json). Detailed process and private playback receipts remain in ignored local test output; no account store, credentials or raw logs are included in this audit.

The installed app was updated exclusively through its native UI. Homebrew installation and updater maintenance commands were not used to perform this local upgrade. This is a macOS ARM interactive upgrade test; other platforms' interactive upgrades, crash recovery and sleep/wake are outside its scope.

## Distribution verification

- All 40 local release-script tests and the Go update-package/signing helper tests passed.
- Hosted native login and playback-runtime checks passed on macOS, Windows and Linux for the release source. See [source-checks.json](source-checks.json).
- macOS ARM/Intel builds, Developer ID signing, notarization/stapling and signed native startup passed. See [signed-arm64.json](signed-arm64.json), [signed-amd64.json](signed-amd64.json) and [macos-release.json](macos-release.json).
- Windows/Linux native builds, actual installer/startup checks, update signing and publication passed. See [desktop-release.json](desktop-release.json). Windows installers remain without Authenticode signing.
- Independent unauthenticated public downloads verified all 22 GitHub asset digests, release checksum manifests, all four Ed25519 update signatures and native smoke metadata. Both macOS public apps and DMGs passed Apple signature, stapled-ticket and Gatekeeper checks; all packaged Mach-O deployment targets remain macOS 13.0. See [published-release.json](published-release.json) and [public-verification.json](public-verification.json).
- Homebrew's cask update and native ARM/Intel installation checks passed. See [homebrew-update.json](homebrew-update.json), [homebrew-installs.json](homebrew-installs.json), [homebrew-arm64.json](homebrew-arm64.json) and [homebrew-amd64.json](homebrew-amd64.json). Homebrew receipt `commit` fields identify the tap workflow revision, not the application source; published build metadata binds the application to the release source above.

## Hosted checks

| Check | Run |
| --- | --- |
| macOS release and signed native startup | [42219147406](https://github.com/mreasonyang/ytmgo/actions/runs/42219147406) |
| Windows/Linux packages and signing | [42221983793](https://github.com/mreasonyang/ytmgo/actions/runs/42221983793) |
| Three-platform source/login | [42219093339](https://github.com/mreasonyang/ytmgo/actions/runs/42219093339) |
| Three-platform native playback runtime | [42219093442](https://github.com/mreasonyang/ytmgo/actions/runs/42219093442) |
| Homebrew cask update | [42222053650](https://github.com/mreasonyang/homebrew-taps/actions/runs/42222053650) |
| Homebrew ARM/Intel installs | [42222657264](https://github.com/mreasonyang/homebrew-taps/actions/runs/42222657264) |
