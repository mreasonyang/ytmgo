# UI/UX findings: fixes and retest

Date: 2026-10-30, Asia/Shanghai. Target: macOS 27.0.1, Apple Silicon.

**PARTIAL: source checks and GitHub package verification PASS. Installation and
native retest are BLOCKED because the Mac is locked.**

The five findings come from the local GitHub-package audit performed earlier on
2026-10-30. Its screenshots include personal library contents and remain local.
This report retains only reviewed, non-sensitive evidence.

## Changes

| Finding | Fix | Regression coverage |
| --- | --- | --- |
| F01: mixed search accessibility names omit type | Row and card names include localized type and creator, matching the visible subtitle. | Seven entity types, rows and cards, four locales; result activation retained. |
| F02: short album header hides tracks and omits creator | Compact horizontal header below 760 DIPs height; smaller artwork/play control; preserve linked header creators without guessing from tracks. | First full row and creator visible at 380/420/757/960/1440 × 609 DIPs, four locales, both appearances; linked-creator parsing and duplicate prevention. |
| F03: wide error body and Retry repeated | One full explanation and Retry in the open presentation; concise transport status and Retry when closed. | Open/closed panel at 420/1440 DIPs in four locales; existing immersive retry coverage. |
| F04: fixed global shortcut absent from help | Focused key recorder in Settings; custom combinations or F1–F12, off switch, persisted preference, conflict/save rollback, cancel/blur/hide restoration, current binding in help. | Registration conflicts, save failure, failed rollback, startup failure, persistence, disable, same-binding/cancel, reserved menu accelerators and recorder interaction. |
| F05: signed-in sidebar says to sign in | Separate not-yet-loaded, loading, failed and loaded-empty messages; reset loaded state with account content. | Four-locale account/library state matrix; installed restart and real library loading remain pending. |

The existing global default is retained for upgrades. In-app playback still uses
Space. Physical global-key delivery with another application focused requires
separate evidence from synthetic window key events.

## Verification

`go test -race -json -count=1 ./...` passed in 130.209 seconds: 841 named
test/subtest passes, zero failures, 22 opt-in skips. Counts include parent tests
and subtests. See [source tests](source-tests.json). Runtime preparation and
tests, login staging tests, Go vet, translation validation, release-script tests
and whitespace validation all passed; see [checks](checks.json).

Reviewed synthetic snapshots: [contact sheet](contact-source.jpg) and
[full-size files](snapshots/). These exercise four locales, short albums,
shortcut settings and wide playback errors.

One initial full run failed because an existing search-layout test still located
rows using their old type-free names. Both row locators were updated to the new
accessible names. A superseded rerun was interrupted; the final complete run
above is the acceptance result.

Source, GitHub CI, downloaded package, installed application and hardware input
are separate evidence levels. No authentication stores, process arguments or
environments, raw logs, credential values or personal library screenshots are
collected in this report.

## Live anonymous catalog regression

Four live searches for the public Opus album followed by album navigation passed:
English, Simplified Chinese, Traditional Chinese and Japanese service languages.
Each response supplied a linked header creator and 30 tracks. See
[test receipt](live-album-creator.json), [observations](live-album-creator-observations.json)
and [the audit-only test source](creator-live-test.go.txt). This test was injected
with a Go overlay; it did not alter production source or use a signed-in account.

## GitHub package verification: PASS

[Actions run 41008270371](https://github.com/mreasonyang/ytmgo/actions/runs/41008270371)
built commit `77cbb37a70c579c2df5775a93421262dc6a3c211` as **0.5.2**.
All required build, test, signing, notarization and upload steps succeeded.
Release publication was disabled and its step was skipped; the current public
release remained [v0.5.1](https://github.com/mreasonyang/ytmgo/releases/tag/v0.5.1)
at verification time. See [Actions](actions.json), [artifact metadata](artifacts.json),
[build identity](build.json) and [public release observation](public-release.json).

The authenticated GitHub client downloaded artifact
`ytmgo-macos-arm64-notarized-16`, ID `12955764343`, 193,249,422 bytes.
All five files match their [package checksums](package-SHA256SUMS);
[download verification](download-verification.json) records each comparison.

The extracted application independently passed strict/deep signature verification,
Gatekeeper assessment and stapled-ticket validation. Its Developer ID team is
`2V4TX79LUZ`, matching the existing installation. All 20 runtime scripts match
current source. The version command returns 0.5.2 and the signed updater archive
passes public-key verification. See [package validation](package-validation.json).
The public update check also passed with updates enabled and no newer version
available: [update check](update-check.json).

Candidate executable SHA-256:
`b1a8f9f196ccb82529b3273aaed28cc12eac673e1bb7c1daed252ee3b68ffa56`.

Local candidate files are under
`build/diagnostics/acceptance-fixes-20261030/download/`; the extracted verified
application is under
`build/diagnostics/acceptance-fixes-20261030/unpacked/ytmgo.app`.

## Installation and native retest: BLOCKED

The native automation tool reported that the Mac was locked and required manual
unlock. An unlock request was sent to the user. No reply had arrived when this
report was saved. The running application was not terminated or replaced while
its native UI was inaccessible. The installed version therefore remains **0.5.1**.
See [baseline](baseline.json) and [native verification state](native-validation.json).

The 0.5.2 installation and the following native checks have **not run**:

- F01: compare actual mixed-result accessibility names in the installed package.
- F02: open Opus in the short native window and inspect creator and first full row.
- F03: interrupt only an app-owned decoder, check one full explanation/Retry,
  close the panel to check the concise fallback, then recover playback.
- F04: record a combination and F8, cancel invalid/reserved input, disable, inspect
  help, and restart to confirm persistence. Physical global-key activation while
  another app is focused remains a separate hardware acceptance item.
- F05: restore Premium without opening Library, inspect the deferred-load copy,
  then fetch the real library and restart.

After manual unlock, quit 0.5.1 normally, retain a reversible bundle backup,
install the verified candidate, and execute these checks. Restore the original
language, appearance, volume and shortcut preference after testing.

No new pass is claimed for spoken VoiceOver, physical IME/media keys, acoustic
speaker output, sleep/wake, real network outage/reconnect, account switching,
session expiry, live synchronized lyrics or multi-hour stability. Existing
account/session files have not been read, copied or deleted by this work.
