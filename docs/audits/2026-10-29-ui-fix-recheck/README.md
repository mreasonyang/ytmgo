# Independent UI Fix Recheck

Date: 2026-10-29, Asia/Shanghai. Reviewed product commit: [`bb579ff9628051d79427235d689acb525a60fa93`](https://github.com/mreasonyang/ytmgo/commit/bb579ff9628051d79427235d689acb525a60fa93).

**Signing-scope correction:** The earlier native recheck used an ad-hoc build. Its account-restoration wait does not establish a code defect, because Keychain authorization and signing identity were not controlled. A [Developer ID signed and notarized Actions build has now been installed](actions-signed/README.md); the subsequent fresh-login result is recorded below. F01 must not be described as a confirmed failed fix on the strength of the ad-hoc wait.

**Subsequent signed-app result:** Fresh interactive sign-in did not complete account acceptance: the return page went blank, the helper later exited, and the main app reported a secure-session-storage error. Source inspection confirms that saving a new login still requires reading and decrypting the old login state. This recovery dependency keeps F01 open; see the [signed-app follow-up](actions-signed/README.md#fresh-sign-in-still-depends-on-the-old-keychain-key).

**Verdict: PARTIAL. F02–F06 pass the checks below. F01's misleading status and blocked public-playback recovery are corrected, but successful real-account restoration remains unverified. The Windows CI job also fails.** This does not establish full product or release acceptance.

The checkout was clean before a fast-forward pull from `5ee89f6` to `bb579ff`. No product source was edited during this recheck. Findings refer to the [original audit](../2026-10-29-full-ui-visual/README.md).

## Findings

| Finding | Recheck | Evidence and limits |
| --- | --- | --- |
| F01: account restoration | **PARTIAL** | In the fresh native build under its original bundle identity, the top bar, Library and Settings accurately showed restoration. The slow state exposed **Continue without account**; selecting a track explained why playback was waiting. Clicking that action started the pending public track, and playback advanced past 79 seconds. **Restore account** retried restoration; a second skip remained usable. Restart again showed restoration accurately. Successful Standard/Premium restoration, canceled system authorization and a complete real-account recovery were not obtained. |
| F02: immersive queue artwork | **PASS** | Actual desktop immersive mode displayed queue thumbnails. The unchanged original audit probe now requested **7** immersive thumbnails, compared with **0** before the fix; docked mode requested 2. New fixture checks also passed for docked, floating, desktop immersive and compact immersive, including suppressing the last offscreen item. |
| F03: light accent text | **PASS** | Normal accent text now uses `#b8261c`. Independently calculated contrast is **4.829–6.302:1** across the five defined light surfaces. Existing dark/fill accents are retained. Light appearance was checked in the actual app and fixture images. This is a palette/readability check, not whole-app accessibility certification. |
| F04: search hierarchy | **PASS** | Actual English and CJK searches showed Song, Video, Artist, Album and Playlist labels before opening results. The first result no longer became a false large section heading. The parser fixture checks the card's section header; four-language view checks also passed for podcast labels. Live podcast/episode destinations were not exhaustively exercised. |
| F05: compact video size | **PASS** | A real **757×641 DIP** window showed an approximately **320×180 DIP** video area, compared with about 180×101 previously. A real video frame was obtained in that area with mode switches, title and transport reachable. Fixture checks passed at widths 380/420/757/760, short heights and four languages; source-edge tests also passed. Native 380/420 windows were not exercised. |
| F06: repeated explanations | **PASS** | The Library page no longer repeats its explanation in the page subtitle and primary card. A native deferred-account Library check confirms that page structure. The exact signed-out explanation count and single compact playback error/retry action passed fixture checks in four languages. A live audio failure was not induced. |

Native evidence: [restoring Library](native/01-restoring-library.png), [Settings](native/02-restoring-settings.png), [waiting track](native/04-track-waits-for-account.png), [immersive artwork](native/05-fallback-immersive.png), [English search](native/03-search-types-en-light.png), [CJK search](native/07-cjk-search.png), [Library](native/08-library-single-explanation.png), [explicit retry](native/09-explicit-restore-retry.png), [actual compact video frame](native/11-compact-live-video-frame.png), and [restart](native/12-restarted-restoring.png).

## Independent Validation

| Check | Result |
| --- | --- |
| `make check` | **PASS on rerun**, all eight Go packages, runtime/login checks, vet and localization check. The initial run failed `TestEmptySeekDoesNotMaskBridgeFailure`; five isolated repetitions then passed. The initial failure is retained in [check-initial.json](check-initial.json), rather than discarded. Its cause remains unproven. |
| `go test -race -json ./...` | **PASS**, 764 named pass events, no failure events. Counts include parent tests/subtests. |
| `go test -json -count=1 -run 'TestNative\|TestDeferredRestore' .` | **PASS**, 241 named pass events; 79 fixture screenshots generated. Twelve targeted fixture images are retained with a [contact sheet](review-contact.jpg). |
| Original audit overlay | **PASS**, all 192 minimum-height geometry cases and the formerly failing queue-artwork probe. Eight more fixture images were generated. The original probe source was used unchanged. |
| `make build-app` | **PASS**, freshly built local macOS arm64 app and update-package utility. |
| `codesign --verify --deep --strict` | **PASS** for the local app. This is an ad-hoc development build, not a newly signed/notarized public release. |
| Product patch whitespace | **PASS**, `git diff 5ee89f6..bb579ff --check`. |

Machine: macOS 27.0.1, Apple Silicon, Retina 2×. The tested executable at `build/darwin-arm64/ytmgo.app` is version 0.5.0, bundle identifier `si.kyun.ytmgo`, SHA-256 `78026e0db1adda2048c2782226d7cb3c94f95a7b0cc874d7ad5f6db3eddec993`. Its running executable path was checked with the restricted process collector before native acceptance. See [artifact.json](artifact.json).

Structured results: [check](check.json), [focused repetitions](focused.json), [race](race.json), [native](native.json), [original probe](probe.json), [build](build.json), [contrast calculations](contrast.json), and [artwork request counts](queue-artwork-counts.json). Raw subprocess/CI logs were not retained.

## Remaining Gates and Observations

1. **Real-account acceptance remains open.** The earlier restoration wait was in an ad-hoc build and cannot separate Keychain authorization from an application defect. The fix provides accurate state and working anonymous recovery. The Actions-signed installation is documented in the follow-up above; successful restoration still requires a completed authorization check. Credentials, session contents, Keychain values, process argv and environments were not inspected, and no cloud account writes were performed.
2. **Windows CI is red.** [Run 40860720119](https://github.com/mreasonyang/ytmgo/actions/runs/40860720119) completed with macOS and Linux passing and Windows failing. The failing assertions are `TestPackagePreservesBundleAndRejectsTampering` at `cmd/update-package/main_test.go:84` (executable permissions lost) and `TestVideoPersistentSlowFramesHaveFiniteCatchups` at `internal/player/video_startup_test.go:197` (catchup-count/timeout assertion). Those source/test files were unchanged in this UI commit; that alone does not establish the cause or absolve the failures. See [ci.json](ci.json).
3. **A live video startup timed out once.** Audio continued, and the video status category was `video_network_timeout`. Reopening the playback panel while paused obtained a real frame, subsequently visible in compact immersive mode. F05's layout is verified, but this run does not establish reliable cold video startup.

After the earlier local recheck, playback was paused; Traditional Chinese, dark appearance and the original wide window size were restored. That ad-hoc candidate was quit and was not installed into Applications. The subsequent Actions follow-up installed the verified signed app, retaining the previous installed bundle for rollback. No release was published and no product source was edited.

Evidence integrity: [SHA256SUMS](SHA256SUMS).
