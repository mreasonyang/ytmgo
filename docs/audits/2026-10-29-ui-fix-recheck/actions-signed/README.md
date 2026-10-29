# Actions-Signed Installed-App Follow-up

Date: 2026-10-29, Asia/Shanghai. **Installation PASS; account acceptance NO-GO after the fresh sign-in attempt described below.** Successful account restoration remains unverified.

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

That initial observation did not establish a code defect. The subsequent fresh sign-in attempt provides the additional evidence below.

No credential/session stores, raw logs, process argv or environments were inspected. The main app's fixed-category diagnostic is recorded in [launch-status.json](launch-status.json).

## Fresh Sign-in Still Depends on the Old Keychain Key

The user completed the interactive Google sign-in in the installed Actions build. The login helper returned to `music.youtube.com/` and visibly displayed a blank white page, with no usable page controls. Its accessibility tree contained only the YouTube Music document. The helper subsequently exited. The main app still showed a secure-session-storage error and did not become signed in; its fixed account diagnostic updated to `session_failed` at `2026-10-29T11:45:09Z`. The user also reported another authorization prompt. The protected system dialog was not inspected, so its exact text and OSStatus are not established.

Source inspection confirms a concrete recovery limitation: [Store.Save](../../../../internal/accountsession/store.go#L142) reads the previous encrypted session file, then unconditionally calls `Secrets.Get` for the existing `native-account-session-key` at line 150. When a prior file exists, lines 176–180 also require decrypting that old file before writing the newly authenticated session. [The fresh-login controller](../../../../account.go#L279) uses this same save operation after verification.

Therefore, **a fresh sign-in is not an independent recovery path when the old Keychain key is inaccessible**. New credentials cannot be saved until that old key can be read; retrying sign-in can encounter the same authorization requirement again. The earlier suggestion that signing in again would bypass the old storage dependency was incorrect.

This does not prove the precise reason macOS requested authorization. It does establish the dependency in source and the failed installed-app recovery outcome. The blank return page is another observed UX problem; its rendering/navigation cause was not established and must not be attributed to Keychain without further evidence.

Required follow-up: provide an explicit, safe way for a newly verified sign-in to replace unusable local login state without first decrypting the old state, preserving transactional save behavior. Then verify fresh sign-in, playback and normal restart in a new Actions-signed installation. No local login data was deleted and no product code was changed in this review.
