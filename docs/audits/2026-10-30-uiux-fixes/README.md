# UI/UX findings: fixes and installed-package retest

Date: 2026-10-30, Asia/Shanghai. Platform: macOS 27.0.1, Apple Silicon.
Installed target: **0.5.2**, source `77cbb37a70c579c2df5775a93421262dc6a3c211`.

**Installation and acceptance of the five audited fixes: PASS.** Source regression,
GitHub CI, downloaded-package verification and the installed native interactions
passed. The user also confirmed physical global-key activation and audible music.
Broader E2E coverage remains **PARTIAL** within the limits below.

No additional product defect was confirmed during this retest. Original account
data was retained. The app is left running, Premium restored, idle without autoplay,
in Traditional Chinese/dark mode at volume 0.7, with the original global binding.

## Findings and acceptance

| Finding | Original problem | Final change | Installed acceptance |
| --- | --- | --- | --- |
| F01, P2 | Mixed search accessibility names omitted destination type. | Row/card names include localized type and creator. | PASS: Song, Video, Artist, Album, Playlist, Podcast and Episode appear in the actual native names in English, Simplified Chinese, Traditional Chinese and Japanese. Album and video result activation were exercised. |
| F02, P3 | Short album headers hid every complete track row and omitted the creator. | Short-height headers use smaller horizontal artwork, compact spacing and linked creator metadata. | PASS: Opus at approximately 757×641 DIPs shows the artist and four complete track rows in both appearances. |
| F03, P3 | A wide playback failure repeated its full body and Retry, truncating the bottom copy. | One full explanation/Retry in the open presentation; concise transport status and a closed-panel Retry. | PASS: a controlled decoder interruption produced one full explanation and one Retry. The bottom displayed “播放失敗”. Closing the panel retained one Retry; that Retry restarted playback and the clock advanced past 35 seconds. |
| F04, P3 | The global binding was fixed, absent from help and could not be disabled. | Focused recorder, custom combinations/function keys, off switch, persistence, rollback and current binding in help. | PASS: Ctrl+P, Ctrl+Alt+P and F8 recording; invalid/reserved input; Escape cancellation; F8 and off across restarts; current help. The user confirmed Ctrl+P paused from another app and music was audible. |
| F05, P3 | An authenticated, not-yet-loaded library still asked the user to sign in. | Separate deferred-load, loading, failure and loaded-empty copy; reset loaded state with account content. | PASS: Premium restoration showed the correct not-loaded prompt, entering Library showed loading copy, and real entries replaced it. Another restart again showed the appropriate not-loaded state. |

These findings originate in the earlier local GitHub-package audit. Its personal
library screenshots remain local. This report commits only reviewed public-search,
public-album and public-video screenshots plus closed diagnostic data.

## Source regression: PASS

`go test -race -json -count=1 ./...` passed in 130.209 seconds: **841 named
test/subtest passes, zero failures, 22 opt-in skips**. Counts include parent tests
and subtests; they are not counts of independent user scenarios. See
[source tests](source-tests.json). Runtime preparation/tests, login staging tests,
Go vet, translation validation, release-script tests and whitespace validation
passed: [checks](checks.json).

Source layout coverage includes 380/420/757/960/1440 × 609 DIPs, four locales and
both appearances. Header parsing tests retain linked creators and avoid guessing
from tracks. Shortcut tests cover OS registration conflict, save failure, failed
rollback, startup rejection, cancellation, disable and persistence. Failure injection
for OS conflicts and disk-save errors is deterministic test coverage, rather than
an induced failure of another running app or the user's disk.

Reviewed synthetic evidence: [contact sheet](contact-source.jpg) and
[full-size snapshots](snapshots/). Production source did not change during the
installed retest, so the completed source/CI runs were not repeated unnecessarily.

Four live anonymous searches and album navigations also passed. Opus supplied a
linked header creator and 30 tracks in English, Simplified Chinese, Traditional
Chinese and Japanese service languages. See [test receipt](live-album-creator.json),
[observations](live-album-creator-observations.json) and
[audit-only overlay test](creator-live-test.go.txt).

## GitHub package and installation: PASS

[Actions run 41008270371](https://github.com/mreasonyang/ytmgo/actions/runs/41008270371)
built the assessed source as 0.5.2. Required tests, build, signing, notarization and
upload all succeeded. Release publication was disabled; the task produced an
Actions package. The public release remained
[v0.5.1](https://github.com/mreasonyang/ytmgo/releases/tag/v0.5.1) at the final check.
See [Actions](actions.json), [artifact metadata](artifacts.json),
[build identity](build.json) and [final public-release observation](public-release-final.json).

The authenticated GitHub client downloaded artifact
`ytmgo-macos-arm64-notarized-16`, ID `12955764343`, 193,249,422 bytes. All five
files match [package checksums](package-SHA256SUMS):
[download verification](download-verification.json).

The extracted app independently passed strict/deep signature verification,
Gatekeeper and stapled-ticket validation. Developer ID team `2V4TX79LUZ` matches
the previous installation. All 20 runtime scripts match source. The CLI returns
0.5.2 and the signed updater archive passes public-key verification:
[package validation](package-validation.json). The public update check passed:
[update check](update-check.json).

After normal quit, 0.5.1 was retained as a reversible bundle backup. The candidate
was staged separately and installed at `/Applications/ytmgo.app`. Its signature
and executable hash match the verified package: [installation](installation.json).
No account/session file was read, copied or deleted. The running installed app
reported Premium under version 0.5.2: [launch](launch.json).

Installed executable SHA-256:
`b1a8f9f196ccb82529b3273aaed28cc12eac673e1bb7c1daed252ee3b68ffa56`.
Local downloads and the retained backup are under
`build/diagnostics/acceptance-fixes-20261030/`.

## Additional installed regression

- **Playback and seeks:** real audio advanced beyond 115 seconds; pause/resume
  worked. A paused middle seek completed at 146.5205 seconds and a paused tail
  seek at 286.6118 seconds. Resuming reached EOF and automatically started BB;
  the next track advanced beyond 15 seconds.
- **Controlled recovery:** SIGTERM targeted only two exact FFmpeg executables in
  this app's discovered ownership scope. The fixed diagnostic recorded
  `audio_stream_failed`; UI Retry recovered playback. See
  [interruption](decoder-interruption.json), [failure category](decoder-error-status.json)
  and [native events](native/events.json). Diagnostic files retain the last recorded
  category; the later clock advance and dismissed error establish recovery, rather
  than treating that retained error record as current failure:
  [diagnostic interpretation](last-diagnostic-after-retry.json).
- **Video:** the first short spatial-audio sample showed unchanged imagery at
  35.96 and 58.008 seconds. It was inconclusive for motion. A second full concert
  video changed from a wide orchestra view at 34.104 seconds to a pianist close-up
  at 65.9547 seconds while playback continued. See
  [frame A](native/25-concert-video-frame-a.png),
  [frame B](native/26-concert-video-frame-b.png) and
  [media-only comparison](video-frame-comparison.json). Switching to cover and
  closing immersive presentation retained audio.
- **Hardware confirmation:** the temporary Ctrl+P binding remained selected while
  the user performed the requested test from another app. The user replied that
  it paused and music was audible; the native UI then showed paused at 52.303
  seconds. The original binding was subsequently restored:
  [confirmation](human-confirmation.json).
- **Lifecycle:** F8 and disabled preferences each survived normal quit/restart
  with Premium restored. A Cmd+H/Cmd+1 command roundtrip retained playing state
  and advanced by 84.69 seconds; actual OS hidden visibility was not independently
  observed. At final normal quit, the five identified main/Node/FFmpeg processes
  exited: [bounded cleanup check](quit-cleanup.json). The subsequent restart
  restored Premium and remained idle without autoplay.
- **Final state:** installed 0.5.2/hash, dark appearance, zh-Hant, volume 0.7,
  position zero and the original Command+Shift+Space binding were rechecked:
  [final state](final-state.json), [native acceptance](native-validation.json).

## Evidence and test notes

Native screenshots were reviewed through contact sheets
[1](contact-native-01.jpg), [2](contact-native-02.jpg),
[3](contact-native-03.jpg), [4](contact-native-04.jpg).
[Native events](native/events.json) retain the complete interaction observations.
Screenshots containing personal library artwork are retained only in the ignored
local diagnostics directory, indexed by [private evidence manifest](private-evidence-manifest.json).
No process arguments, environments, credential values, authentication stores or
raw logs were collected.

An initial source run used two old type-free row locators; they were corrected
and the final full race run passed. A superseded source rerun was interrupted.
During native automation, an early Ctrl+Alt+P injection preceded confirmed recorder
focus; after confirming focus, both combinations worked. An initial Simplified
Chinese assertion expected “播放列表”; the catalog correctly uses “歌单”. The
corrected catalog-based checks passed all seven types. A transport caption was
visible in the screenshot but not separately exposed as child text in AX; visual
review confirmed the concise failure status. These observations remain in the
trace and are not counted as additional confirmed product defects.

The earlier attempt was blocked by a locked Mac while 0.5.1 remained installed:
[retained blocked attempt](native-blocked-attempt.json). The user later requested
another attempt; native access succeeded and this installed retest completed.

## Remaining coverage limits

- Actual 380/420-DIP windows, spoken VoiceOver navigation and physical CJK IME
  composition remain unverified; narrow layouts and AX result names have the
  coverage described above.
- Physical media keys and physical F8 mode were not separately confirmed. The
  successful physical global shortcut was Ctrl+P. No particular audio output
  device is claimed beyond the user's audible-output confirmation.
- Real sleep/wake, network disconnect/reconnect, session expiry, account switching,
  non-Premium advertisements and multi-hour stability/memory leak testing remain
  unverified.
- Fresh interactive login, sign-out, clearing the saved session and playlist
  creation/deletion were not exercised on 0.5.2. Automatic Premium restoration
  and a nonempty real library were exercised. Genuinely empty/failed library
  states and OS registration/save failures retain fixture coverage.
- Ordinary and live synchronized lyric behavior was not re-exercised in this
  native retest. The earlier package audit exercised ordinary lyrics; synchronized
  timing remains fixture coverage because sampled live responses lacked timelines.
- Process cleanup evidence is bounded to approved named main/runtime metadata;
  no complete responsibility-scoped XPC cleanup or controlled memory benchmark
  is claimed here.
- Windows, Linux, Intel Macs and other macOS versions were not installed or tested.
