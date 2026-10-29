# Actions-Signed Installed-App Follow-up

Date: 2026-10-29, Asia/Shanghai. **Installation PASS; real-account restoration UNVERIFIED, pending the local authorization check.**

The previous ad-hoc build was an inadequate basis for claiming that the account-restoration fix had failed. A pending Keychain authorization was not ruled out. The statement that account restoration was still broken is withdrawn; this follow-up uses the actual Developer ID package from Actions.

## Artifact and Installation

- [Actions run 40863518962](https://github.com/mreasonyang/ytmgo/actions/runs/40863518962) completed successfully for commit `ef120bf2a44af140cc920df44c050013a0eb2e1d`, which includes the F01–F06 fixes from `bb579ff`.
- Mode: `notarized`; `publish_release=false`. Version: 0.5.0, arm64. No public release or update feed was changed.
- Artifact: `ytmgo-macos-arm64-notarized-11`, artifact ID `12893271750`. The App ZIP and its build metadata were verified against the Actions checksum manifest.
- Signing authority: Developer ID Application, team `2V4TX79LUZ`, matching the previously installed app. The app signature, Gatekeeper assessment and stapled notarization ticket all passed independently.
- Installed path: `/Applications/ytmgo.app`. Main executable SHA-256: `d59e4b0c32646271b27d0767124351020b0175f83a6299742375f0c46ec4c62f`. The installed executable matches the verified Actions app byte for byte.
- The previous installed bundle was preserved under ignored local diagnostics for rollback. The running app's executable path was checked with the restricted process collector.

Receipts: [build metadata](build.json), [artifact verification](artifact-verification.json), [previous installation](installed-before.json), [new installation](installed-after.json), and [installed checks](installed-validation.json).

## Account-Restoration Observation

The newly installed app displayed the restoring state and subsequently recorded the fixed category `restore_slow`. The macOS `SecurityAgent` process was running. That process observation alone does not prove the content or purpose of an authorization dialog.

Computer Use explicitly refused access to `com.apple.SecurityAgent` for safety reasons. No alternate tool was used to inspect or operate that protected UI. The user was asked to check locally for a ytmgo Keychain request and complete it on the Mac, or report that no request was visible. No password or credential value was requested in chat or read by the agent.

**Pending:** after that local check, verify that the signed installed app restores the account, exposes the expected library/membership state, plays a track, and restores again after a normal restart. Until then, the evidence supports neither a successful real-account restoration claim nor a confirmed code-defect claim.

No credential/session stores, raw logs, process argv or environments were inspected. The main app's fixed-category diagnostic is recorded in [launch-status.json](launch-status.json).
