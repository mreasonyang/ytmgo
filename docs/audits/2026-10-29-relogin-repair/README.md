# Re-login Recovery Repair and Installed-App Acceptance

Date: 2026-10-29, Asia/Shanghai. **Source validation PASS; Actions packaging and installed-account acceptance pending.** This report will be updated with the actual signed artifact and native results before acceptance is claimed.

## Problem and Change

A newly authenticated Google session previously had to read the old Keychain key and decrypt the previous session before it could be saved. When access to that old key was denied, signing in again hit the same failure. The signed-app failure is documented in the [preceding review](../2026-10-29-ui-fix-recheck/actions-signed/README.md#fresh-sign-in-still-depends-on-the-old-keychain-key).

Fresh interactive sign-in now uses `Store.Replace`: it creates an independent random 32-byte key in the native credential store, authenticates a random key identifier in the encrypted file header, then atomically replaces the session file. It neither reads nor deletes the old Keychain item and does not decrypt the old session. Existing storage remains intact on a failed or canceled commit; only the fresh attempt's key is removed. A normal restart loads the key selected by the committed header. Existing version-1 files remain readable.

On macOS, automatic account-key reads disable optional Keychain interaction for the duration of the read and restore the previous policy afterward. All account secret operations are serialized around that process-wide flag. An inaccessible/locked item fails closed and offers sign-in again rather than waiting behind another authorization dialog. New-key writes still use the native credential store; no access permission or credential protection is bypassed. The unavailable retired key is left untouched. Sign-out removes the current encrypted session and active key.

Restore failure and slow-wait text was updated in English, Japanese, Simplified Chinese and Traditional Chinese. Opening a fresh sign-in clears stale restore errors.

## Local Validation

| Check | Result |
| --- | --- |
| `make check` | PASS: all eight Go packages, runtime/login checks, vet and localization check. |
| `go test -race -json ./...` | PASS: 776 named pass events, no failure events; counts include parent tests/subtests. |
| Focused recovery tests | PASS for old-key access denial, missing key, corrupt old file, restart round-trip, current-key deletion, canceled/failed key creation and file commit, key rotation and header tampering. |
| Native read-policy tests | PASS using synthetic callbacks only: reads occur with interaction disabled, prior policy is restored after success/failure, and unavailable policy setup never queries a credential. |
| Final localization check | PASS: 213 messages in four languages. |
| Whitespace | PASS. |

Receipts: [full check](check.json), [race](race.json), and [focused tests](focused.json). No raw command output or credential values are retained.

## Delivery and Acceptance

The required delivery is an Actions `notarized` build with Developer ID signing, installed at `/Applications/ytmgo.app`, followed by real interactive sign-in, playback and normal restart with the account still signed in. Public release publication is disabled.

| Gate | Result |
| --- | --- |
| Actions source revision and run | Pending dispatch. |
| Developer ID signature, notarization and local verification | Pending artifact. |
| Installed artifact matches Actions | Pending installation. |
| Fresh real-account sign-in | Pending user interaction in the installed build. |
| Playback while signed in | Pending. |
| Normal restart retains account and playback works | Pending. |

The user enters Google credentials and any interactive verification directly. The agent does not inspect credential stores, secret values, session contents, raw authentication logs, process argv or environments. Evidence will contain fixed state categories, artifact identities and public test-track observations only.
