# GitHub Package E2E and UI/UX Acceptance

Date: 2026-10-30, Asia/Shanghai. Platform: macOS 27.0.1, Apple Silicon.

**Installation and exercised core macOS paths: PASS. UI/UX acceptance: PARTIAL,
with five findings (one P2 and four P3).** The app was reinstalled from a
GitHub notarized Actions package, then upgraded through its native updater to
the public GitHub **0.5.1** release. Detailed coverage limits remain below.

The primary acceptance target is the Developer ID signed and notarized
GitHub Actions artifact for source `58fd1c77cb1b2b181c4097e4c8fd7503c39049fa`,
from [run 41002530405](https://github.com/mreasonyang/ytmgo/actions/runs/41002530405).
The existing public 0.5.0 release predates the current playback repair. This run
uses `publish_release=false` and does not publish a new release or update feed.

## Verified Installation

[Actions](actions.json) completed successfully. Artifact
`ytmgo-macos-arm64-notarized-14` (ID `12954645580`) was downloaded with the
authenticated GitHub client. Its [build metadata](build.json) identifies the
assessed commit. Every entry in [package-SHA256SUMS](package-SHA256SUMS)
matched the downloaded file; see [download verification](download-verification.json).

The extracted App passed independent strict/deep signature verification,
Gatekeeper assessment and stapled-ticket validation. Team `2V4TX79LUZ` matches
the previous installed app. All 20 packaged runtime `.mjs` files match current
source byte for byte. See [package validation](package-validation.json).

The old app was quit normally and preserved under ignored local diagnostics.
The replacement at `/Applications/ytmgo.app` matches the verified package,
including executable SHA-256
`b977114f63316b93d70e6f814a8eb8d04f826576c70e5df27451cefc28c430c0`.
Its installed signature also passes. Receipt: [installation.json](installation.json).
Account and preference data were not deleted or copied into the evidence.

On launch the new app displayed the saved-account recovery explanation and
Restore / Sign In actions. Actual anonymous Home browsing worked. The official
Google identifier page opened through the installed native login helper;
cancelling it retained the main window and the saved-account explanation,
and a metadata-only executable check found no remaining login helper. A fresh
window was opened for the user's real authentication. No field values from
that authentication window are retained.

## Evidence Boundaries

- The previous installed executable is recorded in [baseline.json](baseline.json).
  Its fixed account diagnostic was `restore_slow`; see
  [before-status.json](before-status.json).
- Source race tests, software-rendered UI snapshots, live anonymous catalog
  calls, packaged runtime tests and installed native interactions are separate
  evidence levels. None substitutes for a successful real-account login.
- Existing account data is retained. Authentication stores, process argv and
  environments, raw application logs and credential values are not inspected.
- Only closed diagnostic categories, public artifact identities, numeric
  measurements and reviewed native screenshots belong in this report.

## Acceptance Checklist

| Area | Planned native acceptance |
| --- | --- |
| Installation | Download provenance, SHA-256, bundle signature, notarization ticket, Gatekeeper, installed/running executable identity |
| Account | Automatic restoration; actionable failure; native sign-in and cancellation; successful restoration after normal quit/restart |
| Catalog | Home and continuation, Explore, Latin/CJK search, entity types, artist/album/playlist detail, back/forward and library filters |
| Playback | Start, sustained advancing clock, pause/resume, middle and tail seek, next/previous, queue selection, shuffle/repeat, mute/volume |
| Video and lyrics | Advancing frames during audio; cover/video switching; hiding the presentation; lyrics availability and scrolling; synchronized follow where supplied |
| Presentation | Wide, medium and minimum native windows, long text, four locales, light/dark appearance, modal scrolling and focus |
| Input/accessibility | Accessible names and state, keyboard search/navigation/playback, focus cycling and dialog dismissal |
| Lifecycle/resources | Hidden-window playback, normal quit and helper cleanup, restart persistence, responsibility-scoped memory observations |
| Recovery | Safe cancellation and retry paths visible in the real application; deterministic error fixtures recorded separately |

## Source and Live Catalog Checks

Live anonymous catalog tests passed: search returned 31 items, Explore 110,
Home 30, and Home continuation added 22 items. Both `TestLiveCatalog` and
`TestLiveHomeContinuationContext` passed; see [live-catalog.json](live-catalog.json).

`go test -race -json -count=1 ./...` passed independently in 129.559 seconds.
There were 753 named test/subtest pass events and eight package pass events,
with zero failure events. These counts include parent tests and subtests and
are not counts of independent user scenarios. Twenty-two opt-in tests were
skipped in this source run. The separately invoked live catalog tests above
are not included in its pass count. Receipt: [source-tests.json](source-tests.json).

All 79 generated software-renderer snapshots were reviewed through the
contact sheets: [1](contact-source-01.jpg), [2](contact-source-02.jpg),
[3](contact-source-03.jpg), [4](contact-source-04.jpg),
[5](contact-source-05.jpg), [6](contact-source-06.jpg),
[7](contact-source-07.jpg). Full-size source snapshots are in [snapshots](snapshots/).
Coverage includes 380/420/760/1100/1440-DIP layouts, four locales, both
appearances, account restore/slow/deferred states, queue artwork in four
presentations, compact errors, synchronized lyric fixtures, aspect-ratio
containment and long immersive metadata. Assertions cover more size/state
combinations than the retained snapshots. This establishes source behavior
under fixtures and does not establish installed-app or screen-reader acceptance.

The five findings and explicit coverage limits below distinguish working
functional paths from remaining UI/UX and accessibility gaps.

## Installed Native and Packaged Runtime Results

The user completed authentication in the installed Google login helper.
The app reported the fixed `premium` category and displayed the authenticated
library. The helper exited. [real-login.json](real-login.json) includes only
closed account state and the main app's responsibility/descendant process scope.
Independent packaged-runtime test processes are excluded from that scope.

| Area | Observed result |
| --- | --- |
| Real library | Initial 24 entries grew to 45 after Load more. Artist filtering and clearing worked. The Likes playlist opened with real account tracks. |
| Navigation | English and CJK search, artist and album details, and return to the preserved query worked. Search results visibly show their entity type; the accessibility omission is F01 below. |
| Premium playback | Tracks loaded and the clock advanced through navigation and presentation changes. Pause/resume and middle seek worked. A paused middle seek reached 99.420 s; Right adjusted it to 101.408 s. |
| Premium tail seek | Two real tracks successfully sought to 195.751 s and 217.665 s while paused. The first resumed, reached EOF and advanced to the next queue item. The second repeated the same track at EOF with Repeat one, then advanced past 18 s. The historical tail-seek timeout was not reproduced. |
| Queue and transport | Queue selection, restarting the current track with Previous, shuffle toggling and Repeat off/all/one worked. Mute set volume to zero; unmute restored the chosen 0.3 volume. Keyboard volume changes worked. Original settings will be restored at completion. |
| Cloud like round trip | A previously unliked track changed to Remove like after the request, then returned to Like after cancellation of that rating. The original unliked state was restored. This did not alter an existing liked track. |
| Video | Real video frames visibly changed while the audio clock advanced from 39.971 s to 112.419 s. Wide, half-screen and short-window presentations were exercised. Cover switching and closing the presentation retained audio playback. Queue thumbnails appeared. |
| Live lyrics | Three tracks provided plain lyrics and the explicit timing-unavailable explanation. Plain lyrics scrolling worked. One additional track had no lyrics and displayed the empty state. No live synchronized timeline was obtained; synchronized follow/seek remains fixture-only in this run. |
| Locales and appearance | English, Japanese, Simplified Chinese and Traditional Chinese switched in the existing window. Light/dark changes retained the selected track. A further English/light switch during playback retained the playing state. |
| Keyboard and dialogs | Cmd+K focused search; Cmd+A replacement and Enter submission worked with Latin/CJK text. Tab/Shift+Tab moved within Settings and Escape dismissed it. Short-window Settings content scrolled to languages and shortcut help. |
| Decoder interruption | SIGTERM targeted only two exact FFmpeg executables owned by this app. The UI displayed a retryable error and the fixed diagnostic became `audio_stream_failed`. One Retry restarted the same track and the clock advanced past 23 s while Premium remained present. This is decoder-process recovery, not a real network outage test. |
| Updates | The installed maintenance command successfully checked the public feed and reported no newer version at the time. The downloaded update archive passed independent Ed25519 verification using the public configuration from its own commit. No update was installed in this check. |

Receipts: [native events](native-events.json), [decoder interruption](decoder-interruption.json),
[fixed error category](decoder-interruption-status.json), [update check](update-check.json),
and [update signature verification](update-package-verification.json).

The source checkout advanced concurrently to 0.5.1 during this audit. An initial
update verification attempt therefore used a mismatched local version and failed.
Repeating with public configuration extracted from the package's immutable
`58fd1c7` commit passed. This was a verification-input mismatch, not a defective
package signature. Installed-app observations in this section remain bound to
the verified 0.5.0 executable above.

The opt-in runtime tests used `/Applications/ytmgo.app/Contents/Resources/runtime`,
not a source runtime. Full-track decoding, native EOF restart and video driven
by real anonymous audio all passed in 146.887 s overall. The two full decodes
produced 212.527 s and 369.607 s of finite, nonzero PCM, reached EOF, and had
zero decoder/transport errors. At 73.125 s and 117.625 s, the video-during-audio
subtests also passed. These tests exercise the installed dependencies through
the Go test harness; the native UI observations above are separate evidence.
See [packaged-runtime tests](packaged-runtime-tests.json) and
[full-decode metrics](packaged-full-decode/results.jsonl).

## Findings

### F01 — P2 — Search result accessibility names omit the destination type

Actual native search results visibly distinguish Artist, Song, Playlist and
Album, but their accessibility button labels omit those types. For example,
the visible `Song` result is exposed as `Merry Christmas Mr. Lawrence`, and
the visible `Album` result as `Opus · Ryuichi Sakamoto`. The labels make it
harder for a screen-reader user to distinguish opening a collection from
starting playback in the mixed result list.

The native accessibility tree and [the visible result list](native/14-cjk-search-light.png)
were compared directly. `nativeItemLabel` in `native_views.go` returns only
title and optional artist, while `nativeResultSubtitle` separately adds the
localized type to visible text. Row/card labels use the former function.
This is accessibility-tree evidence with source support; a full spoken
VoiceOver session was not performed.

Acceptance: include the localized entity type in the accessible name or
description of mixed search results and verify the actual native tree for
songs, videos, artists, albums, playlists and podcasts in all four locales.

### F02 — P3 — Short album detail headers leave no complete track row visible

Opening Opus in an actual approximately 757×641-DIP window fills the initial
viewport with artwork, title, `Album • 2024` and the play button. No complete
track row is visible above the fixed player. The header also has no separate
artist line; the album's creator was visible in the preceding search result.
This is a usability/density finding for the observed short layout, not a
failure to load the album or reach its tracks by scrolling.

Evidence: [short album header](native/15-short-album-header.png).
Acceptance: reduce the header's short-window height and preserve useful creator
information so the first track is discoverable immediately after opening it.

### F03 — P3 — Wide playback errors repeat the body and truncate the bottom copy

After the controlled decoder interruption, the same error body and Retry
action appeared in both the docked Now Playing panel and the bottom player.
The bottom copy was truncated even at full width. Both actions were usable;
the retry recovered playback. The earlier compact error fixtures still pass,
so this observation is specific to the actual wide docked layout.

Evidence: [native decoder error](native/21-decoder-interruption-error.png).
Acceptance: keep a primary explanation in the open panel and use concise status
in the transport, preserving a discoverable retry action when the panel closes.

### F04 — P3 — Global playback shortcut is fixed and missing from shortcut help

The app registers Cmd+Shift+Space as a fixed global playback shortcut. Settings
provides no customization control. The actual keyboard-shortcuts dialog lists
Space for in-app playback and the other local combinations, but omits this
global combination. The user questioned the complexity of the three-key
binding during acceptance; physical-key operation was not confirmed.

Evidence: [actual shortcut help](native/27-shortcut-help-global-omitted.png),
the `global_shortcut_ux` event, and the literal registration in `main.go`.
Acceptance: expose and distinguish the global binding in Settings, support a
user-chosen binding with conflict feedback, and verify physical-key operation
with another application focused. The failed synthetic key delivery alone
does not diagnose a broken global registration.

### F05 — P3 — Signed-in empty sidebar still instructs the user to sign in

After the public 0.5.1 update restored Premium and loaded personalized Home,
the sidebar displayed `建立你的音樂庫` with `登入後查看你的 YouTube Music 音樂庫。`.
It remained visible until the Library route was explicitly opened and fetched
the entries. The library then loaded successfully. This is incorrect empty /
not-yet-loaded copy for an authenticated account, not failed session recovery.

Evidence: [Premium with the sign-in explanation](native/31-public-051-start.png)
and [the subsequently loaded library](native/32-public-051-library.png).
`nativeLibraryPromptText` explicitly selects `nativeLibraryHint` when SignedIn
is true; that string instructs sign-in in all four locale catalogs.
Acceptance: distinguish loading, a genuinely empty signed-in library and a
signed-out account, and use account-appropriate copy and actions.

## Public 0.5.1 Upgrade

A separate concurrent task published
[v0.5.1](https://github.com/mreasonyang/ytmgo/releases/tag/v0.5.1) during this
acceptance run. This audit did not trigger that release. The app's actual
startup check offered it; Later returned to the restored Premium app, and
the native menu's manual check offered it again. The automatic-install option
was left off. Install downloaded the real public package and presented the
restart-ready window; Restart replaced the app and launched 0.5.1.

An independently downloaded public ZIP matched both its checksum manifest and
GitHub asset digest. Its Developer ID signature, Gatekeeper assessment and
stapled ticket passed. The app installed by the updater is byte-for-byte equal
to that ZIP's main executable, SHA-256
`2214521b90dc4b3b3f5b645df0636fe23e331e9ea0a6d2ad8c088689cd0d5c15`.
The installed app independently passes those three system checks as well.

The final public package is from commit
`52790e0161c534f391eb620bf956bc60649b1df0`. Between the primary matrix revision
and this release, the UI, controller, account, catalog, player and runtime
script source paths are unchanged. Changes are the login-helper Dock behavior,
version identifiers and documentation. The full primary matrix was not repeated
after the upgrade; the actual public-package account, library, playback and
video paths receive a separate critical-path recheck.

Receipts: [public release assets](release051-assets.json),
[public build metadata](release051-build.json), [ZIP validation](release051-validation.json),
[installed updater result](installed051-validation.json),
[source comparison](release051-source-comparison.json),
[account restoration](upgrade051-account.json) and
[post-upgrade feed check](upgrade051-update-check.json).

The public-package recheck restored Premium without credentials, fetched the
real library, played past 86 seconds and displayed real video frames. Opening
and cancelling its changed native login helper preserved Premium and advancing
playback. No known login/WebKit helper remained afterward. A final normal Quit
terminated all eight recorded owned processes, and another cold launch restored
Premium and the original preferences without auto-playing a track.

Additional receipts: [release Actions](release051-actions.json),
[before the final Quit](upgrade051-pre-quit.json),
[final Quit](upgrade051-quit.json) and [final state](final-state.json).

Final handoff: `/Applications/ytmgo.app` is public 0.5.1, open on its restored
Premium library in Traditional Chinese/dark, volume 0.7, with no track
auto-playing and no login helper running. This audit changed no product code.
Reports and evidence are local under this directory; they were not pushed or
uploaded by this audit.

## Evidence Index

There are 36 native screenshots and 79 source-renderer snapshots. Native
contact sheets: [1](contact-native-01.jpg), [2](contact-native-02.jpg),
[3](contact-native-03.jpg). Individual screenshots are in [native](native/).
The source contact sheets are linked under Source and Live Catalog Checks.
The final [SHA-256 manifest](SHA256SUMS) binds the retained report and evidence;
[integrity checks](integrity.json) record local link and count validation.

## Normal Quit, Restart and Resource Observation

Closing the main window retained playback. On reopening, the same track's
position advanced from 23.437 s to 107.085 s. Native Cmd+P then paused it.
Preferences were restored to Traditional Chinese, dark and volume 0.7.

Normal Quit terminated all eight previously recorded owned processes, including
all five app/runtime executables. No recorded system helpers remained at the
check. See [before Quit](pre-quit-processes.json) and [after Quit](normal-quit.json).
Relaunch restored the fixed `premium` category without repeating interactive
login. No track auto-played. A fresh selection then played past 27.832 s.
See [normal restart](normal-restart.json),
[restored account](native/26-normal-restart-premium.png) and
[restart playback](native/28-restart-playback.png).

The original main PID was observed for approximately 1,250 seconds at nominal
1 Hz with transitive responsibility/descendant ownership. There were 1,252
records, of which 1,229 had complete process/metric coverage. Incomplete rows
were excluded rather than counted as zero. The complete mixed-interaction
median was 401.15 MiB; the observed peak was 1,016.62 MiB during login handoff.
The observations include authentication, browsing, image loading, audio/video,
paused seeks and recovery. Operator phase labels lag some UI transitions and
are not controlled steady workloads. These figures are not a leak result or
a controlled comparison with earlier builds or other apps.

Evidence: [memory summary](memory-summary.json) and
[safe numeric samples](memory-samples.jsonl). No argv, environment or
credential-store contents were read for attribution.

## Coverage Limits and Automation Observations

- Native interactions reached full width, approximately 757×901 DIP, and
  approximately 757×641 DIP. The OS Window menu changed those sizes successfully.
  Attempts to drag the window to 380/420 DIP did not change its size through
  this automation interface. Those widths have passing source layout fixtures,
  but no completed real-window acceptance in this run.
- A synthetic progress drag committed its starting location, while direct
  coordinate clicks and Right-key seeks worked. Window drags also failed to
  move to their destinations. This is retained as an automation limitation /
  unresolved drag observation, not promoted to a confirmed application defect.
  Accessibility `setValue` likewise did not adjust the float sliders; keyboard
  volume/seek and pointer clicks were exercised successfully.
- The registered global Cmd+Shift+Space did not activate under automated key
  delivery. A physical-key check was requested. This observation must not be
  silently treated as a pass or as a diagnosed registration failure.
- Full spoken VoiceOver navigation, physical CJK IME composition, physical
  media keys, acoustic speaker-output confirmation, sleep/wake, network disconnect/reconnect, account switching,
  Standard/non-Premium advertisements, session expiry and multi-hour stability
  remain unverified. A real network outage was not simulated by changing the
  computer's global network settings.
- Account sign-out, clearing the saved session, and playlist creation/deletion
  were not exercised. Existing account data was preserved. A destructive-flow
  pass is not inferred from showing its entry point or passing fixture tests.
- The runtime decoder interruption, plain-lyrics fallback and actual native
  retry were tested. Live synchronized highlighting/auto-follow remains open
  because none of the three sampled lyric responses supplied a timeline.
- Scope is macOS 27.0.1 arm64. Windows, Linux, Intel Macs and other macOS versions
  were not installed or exercised by this audit.
