# UI/UX findings: fixes and retest

Date: 2026-10-30, Asia/Shanghai. Target: macOS 27.0.1, Apple Silicon.

**Source verification: PASS. Installed-package acceptance: pending.**

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
| F05: signed-in sidebar says to sign in | Separate not-yet-loaded, loading, failed and loaded-empty messages; reset loaded state with account content. | Four-locale account/library state matrix; installed restart and real library loading planned. |

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
are separate evidence levels. Exact package provenance and native observations
will be added after verification. Existing account data is preserved; no authentication stores,
process arguments/environments or raw logs are collected.
