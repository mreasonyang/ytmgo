# Re-login and Playback Recheck

Date: 2026-10-30, Asia/Shanghai. Assessed source: `e1ff1fcfdc943f09315e15dc9847f914a836bfcd`.

**PARTIAL: the source playback repair passes offline checks and independent anonymous full-track decoding; installed-account acceptance remains open.** This review does not modify product code.

## Source Verification

- The assessed commit is pushed to `main`; the worktree was clean at inspection.
- All 83 runtime JavaScript tests passed independently, with zero failures or skips. They exercise the current source directly, including helper-call parsing, configuration merge order, isolated Window execution, byte-array adaptation, cancellation and failure handling.
- The prepared runtime used for the independent network test matches every current `scripts/runtime/*.mjs` source file by SHA-256.
- Independent anonymous full-track decoding passed for both fixtures in 86.5 seconds overall:

| Track | Metadata duration | Decoded PCM duration | EOF | Decoder / transport errors |
| --- | ---: | ---: | --- | --- |
| 1 | 212.561 s | 212.527 s | Yes | 0 / 0 |
| 2 | 369.641 s | 369.607 s | Yes | 0 / 0 |

Full-track decoding checks finite, nonzero PCM, an end-of-stream indication, duration agreement within 0.5 seconds, and decoder/transport failures. It does not open the installed app's audio output or prove account authentication.

Receipts: [source checks](source-checks.json), [independent full-track decoding](full-decode.json), and [delivery and CI checks](delivery-checks.json). The network test ran `go test -json -count=1 -run '^TestNativeFullDecodeLive$' ./internal/player` with the anonymous live-test switch and the verified source runtime. Only fixed state categories, public artifact identifiers and numeric results are retained.

## Open Acceptance Gates

1. **The installed app is still the earlier package.** Its main executable SHA-256 is `d59e4b0c32646271b27d0767124351020b0175f83a6299742375f0c46ec4c62f`, matching the recorded installation from [Actions run 40863518962](https://github.com/mreasonyang/ytmgo/actions/runs/40863518962). The latest completed [macOS package run](https://github.com/mreasonyang/ytmgo/actions/runs/40910763322) contains `ce0fcc9`, not the assessed playback repair. A new signed/notarized package and installation are required before validating the current app.
2. **Real-account login, playback and restart persistence remain unverified on the repaired installed app.** No credential-store read, account reset or interactive sign-in was performed during this source review.
3. **Windows CI is still failing.** For the assessed commit, [run 40999534114](https://github.com/mreasonyang/ytmgo/actions/runs/40999534114) passed on macOS and Ubuntu. Windows failed at `Native helper lifecycle`. This is separate from the source parser regression; this review does not infer its root cause from the step name.

## Current Account Storage and Historical Evidence

The current implementation uses one persistent `account-session-key`, a fresh nonce for each encrypted session write, and the version-3 file format. Automatic macOS restore requests no Keychain interaction; explicit restore, sign-in and clearing may request system authorization. Missing-key creation is distinct from denied or locked storage. This is a source description, not a native Keychain acceptance result.

The [2026-10-29 repair report](../2026-10-29-relogin-repair/README.md) records the earlier `b1e4d80` implementation. Its `Store.Replace`, rotated-key and legacy-format statements describe that historical revision and do not apply to the current single-key implementation.

The remaining installed-app sequence is: package this revision through Actions with Developer ID signing and notarization; install and verify the artifact; complete real interactive sign-in; verify playback; quit normally; reopen and verify the account and playback again. A source test or CI pass cannot substitute for those observations.
