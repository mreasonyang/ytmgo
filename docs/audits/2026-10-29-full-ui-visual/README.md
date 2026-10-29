# ytmgo Comprehensive UI and Visual Acceptance Review

Follow-up: [independent recheck of fixes at bb579ff](../2026-10-29-ui-fix-recheck/README.md). The findings and verdict below describe the original audit baseline.

Date: 2026-10-29, Asia/Shanghai. Verdict: **NO-GO**. This review identified **6 findings: 1 P1, 3 P2, and 2 P3**, plus 2 visual improvement suggestions. Product source code was not modified.

The main blocker is prolonged account restoration in the candidate app under its original bundle identity: the UI reports that the user is signed out while playback remains pending. The remaining playback and visual checks used an isolated anonymous copy of the same candidate build. Those results do not establish acceptance for a real account.

## Tested Artifacts and Evidence Levels

| Artifact | Identity and scope |
| --- | --- |
| Workspace | HEAD `eb8e462a107468690ea99f7e577f060284747457`, with 19 previously modified tracked files and additional untracked fixes. Existing native UI tests and audit probes ran against this source tree. |
| Installed app | `/Applications/ytmgo.app`, version 0.4.0, executable SHA-256 `cee7b5ecefdb844f0da17b69a82e605685b7222d673e10d4ad6a24314db578f7`. Used only for the initial appearance, language, and browsing baseline; see native screenshots 01–09. |
| Primary candidate app | `build/darwin-arm64/ytmgo.app`, version 0.4.0, executable SHA-256 `65b6733260655b70fb5411ab93dcd97eb57cf2920145d95bef34299c9052e9b9`. This matches the artifact manifest from the earlier lyrics and layout fixes. Observations under the original bundle identity are in screenshots 10–18. |
| Isolated anonymous copy | Copied from the primary candidate, with only the bundle name, identifier, and ad-hoc signature changed. Before signing, the main executable was byte-for-byte identical. It used a separate data directory; real account data was neither copied nor read. Most native UI acceptance checks are in screenshots 19–48. |
| System | macOS 27.0.1, Apple Silicon, Retina 2×. |

The app was not rebuilt during this review. Source tests, the existing candidate bundle, and the installed app have separate results. The development version 0.5.0 in the project README does not establish that the tested 0.4.0 executable contains those updates. At audit time, source fixes were uncommitted, so HEAD alone could not establish the complete contents of the local candidate bundle. Subsequent publication of those existing fixes is documented in [delivery validation](../2026-10-29-spotify-fixes/DELIVERY_VALIDATION.md); it does not represent remediation of the six findings below.

Full identities and hashes are recorded in [baseline.json](baseline.json) and [anonymous-artifact.json](anonymous-artifact.json). The 19 previously modified tracked files retained identical hashes before and after the review; see [source-preservation.json](source-preservation.json).

## Coverage and Results

| Area | Result and evidence boundary |
| --- | --- |
| Home, Explore, and category navigation | Actual anonymous loading and navigation passed. Candidate Explore and Chill category pages: [47](native/47-candidate-explore.png), [48](native/48-candidate-genre.png). |
| Home pagination | Passed in this run: Load more added sections including Low key vibes. See [before](native/29-home-page-before.png) and [after](native/30-home-page-after.png). The older pagination failure was not carried forward as a current finding. |
| Search, CJK text, artists, and albums | Actual queries and navigation passed; Back restored the query `坂本龍一`. Text was pasted, so physical IME composition was not tested. Result hierarchy remains an issue under F04. |
| Signed-out library and empty player | Entry points and messages were visible. The empty player provided Home / ⌘K guidance. Duplicate library copy is covered by F06. |
| Playback, pause, queue selection, and seeking | Playback position advanced in the anonymous copy, and queue selection changed the current track. Pointer seeking reached 169.7145 seconds and remained paused after completion; the Left key then moved the target to 166.32021 seconds. See [completed seek](native/28-seek-result.png). Speaker output was not recorded. |
| Video | Actual video frames appeared anonymously. Cover / Video switching and reopening the playback view worked. The earlier vertical clipping was not observed; undersized video in a compact short window is covered by F05. |
| Lyrics | One track showed no lyrics; another displayed lyrics with a timing-unavailable message. Scrolling worked. Existing offline tests cover synchronized highlighting, automatic following, manual browsing, and resuming follow mode. This run did not obtain a usable synchronized timeline for a live track. |
| Four languages and light/dark appearance | The candidate switched between English, Simplified Chinese, Traditional Chinese, Japanese, and light/dark appearance while keeping the window alive and retaining the current track. Playback was mostly paused during these switches. The earlier language-switch exit was not reproduced. |
| Keyboard and dialogs | Actual checks passed for ⌘K search, ⌘J panel, Escape, Tab / Shift+Tab focus cycling in Settings, and keyboard seek adjustment. Shortcut help was readable in a short window. |
| Sign-in entry and cancellation | The Google identifier screen opened. No account identifier or credentials were entered. Closing it preserved the main UI and playback; the restricted process query returned an empty array for the login helper afterward. |
| Window lifecycle | After Cmd+W, the main window could be shown again through the app entry point. This idle-state check does not establish background playback or sleep recovery. |
| Responsive layout | Actual native windows covered approximately 1440/1512 DIP widths, a 757 DIP half-screen width, and a short content area of approximately 609 DIP height. Native windows at 380/420 DIP widths were not completed; those sizes were covered by offline rendering below. |
| Account restoration and real-account library | Blocked under the original bundle identity; see F01. Standard / Premium account presentation and filtering have fixture evidence only. |

The existing command `go test -json -count=1 -run TestNative .` passed, with 173 named test/subtest events and 68 generated screenshots that were visually reviewed. See [results](workspace-native-tests.json) and [screenshots](snapshots/). The count includes parent tests and does not represent 173 independent user scenarios.

An additional audit probe was injected through a Go overlay, without changing production code or regular Go test files. Across 8 widths × 4 languages × 2 themes × 3 page modes, **all 192 critical-control geometry checks at a content height of 608 DIP passed**. Widths were 380, 420, 760, 761, 960, 1180, 1181, and 1440. Eight additional screenshots were reviewed. Geometry checks establish that the specified controls are within bounds; they do not establish correctness of every text element, image, or real interaction. The separate immersive queue artwork assertion **FAILED**, corresponding to F02. See [probe source](visual_audit_probe_test.go.txt), [results](audit-probe-results.json), and [probe contact sheet](audit-probe-contact.jpg).

## F01 · P1 · Account Restoration Remains Pending While the UI Reports Signed Out

**Evidence level: reproduced in the actual native candidate app under its original bundle identity.**

After the candidate launched, the account category in the fixed-category diagnostic remained `restoring`. At the same time, the top bar, library, and Settings showed a signed-out state and offered sign-in. Selecting a public track left the player displaying Loading for an observed interval of at least **210.5 seconds**, without account-restoration progress, an explanation of the blocked restoration, or an actionable recovery entry point. The audio diagnostic remained `ready` during this interval.

Users can interpret this account-restoration wait as a signed-out state or a track-loading failure. The isolated anonymous copy played a track with the same title, so the wait under the original identity does not establish a general failure of public playback. The specific cause of the restoration blockage remains undetermined.

**Investigation pointers:** The restoration state in [account.go:197](../../../account.go#L197) remains in the controller. Account text in [native_views.go:313](../../../native_views.go#L313) distinguishes only signed out, Standard, and Premium; it has no restoring state. Review this together with playback waiting during restoration.

**Acceptance criteria:** The UI accurately represents pending restoration, canceled authorization, failed restoration, and successful restoration. A blocked wait provides a clear explanation and an actionable next step, without leaving an unexplained long-running player spinner. Verify all four languages and state after restarting the app.

Evidence: [signed-out presentation](native/18-candidate-restoring-shown-signedout.png), [persistent loading](native/17-loading-still-pending.png), and [timing and fixed diagnostic categories](account-restore-observation.json). Session files, Keychain values, argv, environments, and raw logs were not read.

## F02 · P2 · Desktop Immersive Mode Replaces Queue Artwork with Placeholders

**Evidence level: reproduced in the actual anonymous native app and an offline probe, with supporting static analysis.**

The same queue displayed thumbnails in the regular Now Playing panel. Entering desktop immersive mode replaced the right-hand queue thumbnails with generic music-note placeholders. Track names and queue actions remained available, but visual identification became harder. The same behavior appeared in several existing offline screenshots generated during this review.

The audit probe cleared the browsing page and recorded small artwork requests for tracks other than the current track. The regular docked panel made 2 requests; immersive mode made 0, and the assertion failed. This provides evidence beyond a screenshot taken during a transient network load.

**Investigation pointers:** [native_views.go:1439](../../../native_views.go#L1439) narrows `a.artViewport` for the left media scroll area, then passes the same `a` to the right-hand queue. [nativeArtworkViewport](../../../native_views.go#L864) intersects the bounds again, causing the right-hand artwork to be treated as invisible. This propagation path is consistent with the probe and screenshots.

**Acceptance criteria:** Visible tracks in the same queue show artwork in docked, floating, desktop immersive, and compact immersive layouts. Scrolling requests artwork that enters the viewport while retaining request limits for offscreen images.

Evidence: [regular panel](native/19-anon-start-loading.png), [immersive mode](native/20-anon-immersive-light.png), [request counts](queue-artwork-observation.json), and [offline comparison](snapshots/audit-queue-artwork-immersive.png).

## F03 · P2 · Small Accent Text Has Low Contrast in Light Appearance

**Evidence level: actual visual observation, offline screenshots, and calculations from source palette values.**

Light appearance uses `#e8382d` for the current track title at 15 DIP and font weight 500, as well as smaller error text. Its calculated contrast is **4.171:1** against white, **3.728:1** against `#f2f2f3`, and **3.440:1** against `#e9e9eb`. The corresponding dark-appearance accent combinations are approximately 5.36–6.83:1.

Normal-sized status text is therefore weaker in light appearance, with selection and blurred backgrounds also affecting the final result. Using the [W3C WCAG 2.2 benchmark of 4.5:1 for normal text](https://www.w3.org/WAI/WCAG22/Understanding/contrast-minimum.html) as a readability reference, these combinations need improvement. This is not a WCAG conformance certification for the native app as a whole.

**Investigation pointers:** [native_style.go:25](../../../native_style.go#L25), [native_views.go:997](../../../native_views.go#L997), and [native_views.go:1837](../../../native_views.go#L1837).

**Acceptance criteria:** Check the final foreground/background combinations for current-track titles and small error text in light appearance. Normal text meets the adopted readability benchmark while preserving the visual hierarchy of dark appearance and non-text playback icons.

Evidence: [calculated values](palette-contrast.json), [native light appearance](native/20-anon-immersive-light.png), and [offline light appearance](snapshots/1440-light-home.png). Calculations used defined colors rather than antialiased text-edge pixels from screenshots.

## F04 · P2 · Search Result Types and Section Hierarchy Are Unclear

**Evidence level: visible in both the candidate under its original identity and the isolated anonymous copy; an information architecture and UX finding.**

Searching for `Daft Punk Get Lucky` displayed the query heading followed by another large heading containing the first track title. Subsequent tracks, artists, playlists, albums, and podcasts used similar rows in a mixed list, without consistent type or section labels. Searching for `坂本龍一` similarly placed tracks and other result types under an artist heading. Users must infer destinations from artwork, subtitles, or opening the result.

A random query also returned content with no obvious relationship to the input, without explaining the match. The upstream response was not retained, so this review does not attribute those results to client fabrication or count the query as a verified live empty-result state.

**Investigation pointers:** [internal/catalog/parse.go:56](../../../internal/catalog/parse.go#L56) merges untitled rows and uses the card shelf title. [native_views.go:685](../../../native_views.go#L685) chooses a uniform list style based on the proportion of songs in the whole group. Preserve and present the actual entity types and upstream section semantics.

**Acceptance criteria:** Track, artist, album, playlist, and podcast destinations are distinguishable before opening a result. The first track title no longer implies an incorrect section grouping for mixed results. Recheck English and CJK queries.

Evidence: [search under the original identity](native/12-search-dark.png), [CJK search](native/36-cjk-search.png), and [query without an obvious matching relationship](native/40-unmatched-query-results.png).

## F05 · P3 · Video Is Visibly Undersized in a Compact Short Window

**Evidence level: reproduced in the actual anonymous native app, supported by the source layout formula.**

In an actual window of approximately 757×641 DIP, immersive video occupied only about **180×101 DIP**, leaving substantial horizontal whitespace. The title used a much wider area, and the queue required further scrolling. Actual video frames appeared, with no obvious image clipping in this sample, but the viewing area was small.

**Investigation pointers:** The compact branch in [native_views.go:1427](../../../native_views.go#L1427) budgets `artSize` for square artwork, then reduces video height to 16:9. It does not use the aspect-ratio-aware video budget already present in the desktop branch.

**Acceptance criteria:** Video makes reasonable use of available width and height in compact short windows. The mode switch, title, and fixed playback controls remain reachable. Recheck sizes around 380/420/760 DIP, long titles, and all four languages.

Evidence: [actual short-window video](native/22-compact-immersive-video.png).

## F06 · P3 · Empty and Error States Repeat the Same Explanation

**Evidence level: actual native evidence for the empty library; offline rendering evidence for duplicated playback errors.**

The signed-out library repeats the explanation that signing in reveals playlists, albums, and liked music, both below the page heading and inside the sign-in card. In compact immersive playback-error fixtures, the same error and retry action appear at the top of the content and again in the bottom transport area, where the explanation is also truncated to one line. Repeated messages consume useful space in short windows.

**Investigation pointers:** [controller.go:60](../../../controller.go#L60) writes the empty-library explanation into the page subtitle, and [native_views.go:658](../../../native_views.go#L658) renders it again. Playback errors are rendered by both [native_views.go:1426](../../../native_views.go#L1426) and [native_views.go:1797](../../../native_views.go#L1797).

**Acceptance criteria:** Each visible page has a clear primary explanation and action. When status must appear in multiple locations, reduce repeated body copy and actions to avoid redundancy and truncation in short windows.

Evidence: [anonymous library](native/46-anonymous-library.png), [English error state](snapshots/service-error-en-playback-true.png), and [Simplified Chinese error state](snapshots/service-error-zh-Hans-playback-true.png). The network was not deliberately disconnected to create a live playback error.

## Visual Improvement Suggestions, Excluded from the Finding Count

1. **Reduce the initial viewport space used by detail headers in short windows.** In the actual 757 DIP window, artist/album artwork and headings consume much of the height, leaving almost no album tracks visible initially. The text metadata for `Opus` shows only `Album · 2024`, without an explicit artist line. Reduce the hero height in short windows and retain useful artist information. See [artist](native/37-artist-compact.png) and [album](native/38-album-compact.png). This observation does not establish missing artist information across all albums.
2. **Make horizontal browsing more discoverable.** Long horizontal lists on Home and Explore primarily rely on scrolling or a trackpad. An explicit next-group or view-more entry point would help mouse users. This is a usability suggestion; partially exposed horizontal items and intentional scrolling are not themselves clipping defects.

## Areas Still Unverified

- Real Standard / Premium libraries and membership presentation after successful restoration, a complete first sign-in, account switching, and sign-out. Likes, unlikes, and playlist cloud writes were not performed.
- Automatic following and seek alignment for live synchronized lyrics. Online samples in this run had either no lyrics or no synchronized timeline.
- Native windows at 380/420 DIP widths, a complete VoiceOver pass, physical CJK IME input, and system text enlargement / increased contrast / reduced motion.
- Physical media keys and global shortcuts, extended background operation, sleep/wake recovery, a complete offline-to-online recovery cycle, long-term stability, and acoustic speaker output.
- Windows, Linux, Intel, other macOS versions, CI, installed-app updates, notarization, and public release.

The earlier 0.3.0 report served only as a checklist. Historical conclusions were not treated as current evidence. Language-switch exits, vertical video clipping, and missing compact-layout lyrics/queue entry points were not reproduced in the corresponding checks during this run. Deduplication of real-account category subtitles passed only in current fixtures and still requires a real-account recheck.

## Evidence and Handoff

- [Native screenshots](native/); [native contact sheet 1](native-contact-01.jpg), [2](native-contact-02.jpg), [3](native-contact-03.jpg), and [4](native-contact-04.jpg). Screenshots 01–09 are from the installed app, 10–18 from the candidate under its original bundle identity, and the remainder from the isolated anonymous copy, except 43, which records restoration of the original candidate's window size.
- Offline screenshot contact sheets: [1](snapshot-contact-01.jpg), [2](snapshot-contact-02.jpg), [3](snapshot-contact-03.jpg), [4](snapshot-contact-04.jpg), [5](snapshot-contact-05.jpg), and [6](snapshot-contact-06.jpg), plus the [audit probe contact sheet](audit-probe-contact.jpg).
- [Existing native test results](workspace-native-tests.json), [additional audit probe results](audit-probe-results.json), [source preservation check](source-preservation.json), [source hashes](workspace-go-source-hashes.json), [runtime observation summary](results.json), and [evidence hash manifest](SHA256SUMS).

The original candidate retained its Traditional Chinese / dark appearance settings, and its window was restored to the original wide size. Test playback was paused, and both the anonymous copy and the original candidate were quit; process-exit checks are recorded in results.json. The acceptance run added this evidence directory and the isolated copy under `build/diagnostics/full-ui-visual-20261029/`, without installing to Applications or publishing a release. The report, evidence, and previously local fixes were committed and pushed afterward; see [delivery validation](../2026-10-29-spotify-fixes/DELIVERY_VALIDATION.md) for the separate validation record.
