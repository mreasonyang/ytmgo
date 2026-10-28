# Native playback verification

## Account-aware native implementation — 2026-10-28

The source now has native account instructions, an explicit Brave extension
handoff and system-secret session storage. The app does not create a WebKit
page or read the previous WebKit cookie store. The status query parses the
server's `logged_in` and `has_unlimited_entitlement` values; it runs on restore,
login, focus and every five minutes. An unavailable response remains an error,
not a downgrade to a free account.

Startup waits for account restoration and does not warm the proof engine.
Premium selects an engine that rejects imports of jsdom and the proof provider.
Anonymous/free playback starts a proof engine on demand. Membership changes
cancel preparation and reap the previous engine while retaining the currently
playing stream. Identity changes revoke the previous account's streams.

Go tests, vet, four-language checks (185 messages) and 80/80 offline
JavaScript tests using the packaged locked dependencies passed. Go race tests passed
for the app, account bridge, catalogue and player. New gated decoder tests
cover seeks/replay during membership changes, cancelled identity changes,
private configuration IPC and truncated PCM despite successful decoder exit.
Seventeen process-scope tests and five packaging-projection tests also passed.

The new anonymous runtime completed two full-track PCM decodes:

| Track | Metadata seconds | Actual PCM seconds | Resolve / first PCM seconds |
| --- | ---: | ---: | ---: |
| 1 | 212.561 | 212.546667 | 11.951941 / 1.622410 |
| 2 | 369.641 | 369.626667 | 1.243670 / 0.901455 |

Both streams reached EOF with nonzero finite PCM and no decoder/bridge error.
This was faster-than-real-time decoding, not a speaker recording or normal-ad
acceptance. Evidence: `build/diagnostics/account-native-20261028/anonymous-full/results.jsonl`.
The first native-matrix attempt failed because an anonymous configuration GET
included `authuser=0`, causing an official redirect which policy rejected. The
anonymous request now uses the canonical URL; these full decodes followed that
fix and are not evidence that the initial failed attempt passed.

The final anonymous native matrix also passed (race-enabled, 113.72 seconds):
prepared next-track output was observed in 96.33–98.71 ms with zero new resolver
or decoder calls; unprepared selection took 2.688 seconds. Pause held its clock,
seek to 45 seconds advanced to 47.968 seconds, and simultaneous audio, prepared
track and 480×480 video succeeded. Cold output took 21.621 seconds in this run;
this is not a cold-start improvement claim. Network samples are not guarantees.
Evidence: `build/diagnostics/account-native-20261028/anonymous-matrix-final/results.jsonl`.

A 31-sample signed-out idle run, with the native account window open, measured
116.65 MiB median/peak across the full captured responsible process set (five
processes); no WebKit executable was observed. This preliminary app had the
same playback architecture but preceded final interaction fixes. It is an idle
measurement, not a playback or final-package memory claim. Evidence:
`build/diagnostics/account-native-20261028/signed-out-idle/samples.jsonl`.

### Real-account and final-package acceptance

The native app accepted an explicit Premium handoff, verified the server flags,
loaded the account's library and played a 369.641-second recording through its
natural end into the next track. Restarting that same build restored Premium.
The Premium child enforces a module guard that rejects DOM/proof imports.
The 241 steady playback samples (elapsed 60.467–300.493 seconds) measured a
whole responsible-process median of **358.98 MiB**, peak **364.22 MiB**. No
WebKit process was observed. One sample attributed 259.81 MiB to the native UI
and about 74.27 MiB to the two Node processes. This is not a claim that the
entire app uses only the helpers' memory. Quit reaped the app and both Node
processes. Evidence: `build/diagnostics/account-native-20261028/premium-app/`.
This run preceded the final canonical-URL/diagnostic/package-projection edits;
it is a whole-app measurement of that build, not of every later package.

The ordinary-account handoff and server classification passed in the native
app, but its first playback attempt failed before audio with
`stream_resolution_failed`. The runtime now requests the canonical Music
configuration URL and checks the returned account/brand identity strictly.
It also reports bounded stage categories through private IPC and diagnostics;
no dependency messages, URLs or credentials are persisted.

The canonical request then resolved an ordinary-account recording successfully,
but the first full-decode attempt stopped at 28.3935 seconds with the former
generic `audio_connection_failed` category. A fresh explicit handoff retest
completed both tracks: 212.546667 and 369.626667 seconds of PCM, correct EOF,
and zero decoder/bridge errors. Resolution/first PCM took 27.702489/1.659875
seconds and 3.505652/2.065523 seconds. This verifies ordinary-account content
playback in the packaged runtime, not native UI/Keychain or advertisement
acceptance. Both attempts remain in `standard-final-package-ephemeral/` and
`standard-final-package-ephemeral-retry/` under the same evidence root.
The first failure cannot retrospectively be classified as a transient network
error: the then-current bridge collapsed late CDN rejection and timeout alike.
The final bridge now retains a fixed error category on its private capability;
a bounded local HEAD reads it after a stream failure, without retrying the CDN
or changing Content-Length/Range behavior. Synthetic late-403, timeout,
truncation and untrusted-header tests passed.

The final package completed two anonymous full decodes: 212.546667 seconds of
PCM for 212.561 seconds of metadata, then 369.626667 for 369.641. Both reached
EOF with nonzero finite PCM and no decoder/bridge error. Resolution/first PCM
took 11.360899/1.040926 seconds and 2.224731/0.754330 seconds respectively.
Evidence: `build/diagnostics/account-native-20261028/anonymous-final-package/`.

The rebuilt app's Keychain restore triggered a macOS authorization prompt after
the ad-hoc signing identity changed. The user was away from the device, so this
restore was not completed or bypassed. Separate ephemeral browser-handoff tests
do not count as native Keychain restoration acceptance.

The first Premium lifecycle attempt completed its 212.561-second track, then
hit the test's three-minute per-track deadline while still decoding a
369.641-second recording (275.7535 seconds of PCM, no bridge error). This is a
test-enforced cancellation, not evidence of a CDN rejection. The per-track
deadline is now seven minutes, and the lifecycle scenario uses one complete
recording before and after switching; the default account test still uses two
tracks. The timed-out attempt remains in `premium-cycle-final/`.

The corrected lifecycle test passed against the delivery runtime: verified
Premium → complete 212.546667-second PCM decode → anonymous proof warm-up →
verified Premium restoration → another complete 212.546667-second decode.
Both decodes reached EOF with zero decoder/bridge errors. During the two
ten-second observation windows, the test host plus responsible helpers measured
146.18 MiB median with the anonymous proof engine, then 43.16 MiB after Premium
reconfiguration. The old proof PID disappeared and was absent throughout the
restored Premium decode; a new engine was created on demand. Restored decoding
measured 91.11 MiB median. All owned processes were gone at test completion.
These figures exclude native UI and Brave. This tests actual identity changes,
not a real billing upgrade/expiry; same-account membership changes are covered
by automated lifecycle tests. Evidence:
`build/diagnostics/account-native-20261028/premium-cycle-delivery/`.

The user confirmed that the observed volume change around natural next-track
was their manual action. It is not a product mute defect or an outstanding
acceptance item. Tests covering the volume controller, natural EOF, prepared
next, seek and account/membership changes passed; no mute fix is claimed.

Final packaging checks passed for 17 runtime modules, 137 manifest files,
locked dependencies, Node version, App strict ad-hoc signature, DMG integrity,
and identical App contents inside/outside the mounted DMG. The dependency hash
now uses `mygo-visible-v1`, matching MyGo's omission of dot-prefixed resources.
The DMG itself is unsigned: MyGo intentionally skips DMG signing in ad-hoc mode.
Neither Developer ID signing nor notarization has been performed. Evidence:
`build/diagnostics/account-native-20261028/artifact-release-candidate.json`. The earlier
manifest mismatch and its failed record remain preserved separately.

Delivery executable SHA256:
`897c9fb29e5c7c21728498117fe7069eb840a0059c8133265920c8e418e6c9ba`.
Delivery DMG SHA256:
`7017673c28dfbb739cc5f10659a1204f8f7b576f679e90fe182193dd87c0e649`.

### Three-state whole-application memory comparison (2026-10-28)

The three states were measured with fresh processes of the same diagnostic
binary, using the native UI, native audio output, and unchanged delivery
runtime/resources. Each run played the same 369.641-second recording
(`4D7u5KF7SP8`) in the immersive cover/queue layout, with repeat-one, no
next-track preparation, and volume zero. The native playback clock advanced
while muted. The window capture was 2880×1810 pixels. Each state had a
30-second warm-up after playback confirmation, followed by nominal 1 Hz memory
sampling over a fixed 120-second half-open interval `[start, start + 120)`;
`start` is the first sample marked `playback_steady`. Scheduling jitter yielded
120, 121, and 120 samples respectively, not a different requested window.

The test host was Mac16,8 with 24 GiB RAM and macOS 27.0.1. The metric is
the sum of `ri_phys_footprint` across the app's responsible processes and
descendants, including native system helpers. Brave is excluded.
Every sample used in these fixed windows had complete process coverage; no
owned WebKit process was observed. All numbers below are MiB. Peaks refer to
these same fixed playback windows, not the whole launch/sign-in run.

| Account state | Whole app median | Whole app peak | All Node median | All Node peak | Native UI median |
| --- | ---: | ---: | ---: | ---: | ---: |
| Signed out | 403.44 | 407.38 | 127.17 | 131.16 | 245.85 |
| Signed in, non-Premium | 399.19 | 403.16 | 129.41 | 132.50 | 248.05 |
| Premium | 347.48 | 351.79 | 74.95 | 78.10 | 250.64 |

In these sessions, Premium's whole-app median was **55.96 MiB
(13.87%)** below signed out and **51.71 MiB (12.95%)** below non-Premium.
The corresponding combined Node differences were **52.22 MiB (41.06%)** and
**54.46 MiB (42.08%)**. Component medians are separate statistics and need
not add up to the whole-app median.

This is a controlled playback setup with real account sessions, not a strict
isolation of JSDOM's memory cost. Signed-out Home loading failed; signed-in
handoffs opened Library, and search recommendation tails differed. Retained
pages, artwork/GPU caches, and native system-helper sets therefore differed
before the common playback view. The image cache's 32 MiB CPU-pixel budget
excludes GPU textures and in-flight decoding. The combined Node figures help
separate playback-resolution processes from UI differences, but also include
SDK/account state; neither their difference nor the whole-app difference is
claimed to be JSDOM alone. There was one successful run per state, so these are
session observations rather than repeated-run estimates or universal limits.

The diagnostic build replaces only account secret persistence with an initially
empty, concurrency-safe memory store, plus a separate application identity and
window title. It never calls the system secret API; fresh extension handoffs
stay in memory and are discarded at process exit. Its native rendering/player
implementation and runtime/resource bytes otherwise match the delivery build.
It is **not the distributed executable** and does not validate Keychain restore
or the normal advertisement protocol. Diagnostic executable SHA256:
`d7ac0ec5df4d3a1734a64a1d12e1569d482a6af230d0b096783ebd2fd7a4a050`.

Evidence: `build/diagnostics/account-memory-three-state-20261028/`, including
`comparison.json`, `workload.json`, `diagnostic-build.json`,
`diagnostic-source.patch`, and the `anonymous-02`, `standard-01`, and
`premium-01` samples/summaries. A credential-free summary is in
`docs/audits/2026-10-28-account-memory/comparison.json`. All 25 observed
processes, including native system helpers, were confirmed absent after normal
quit by PID/executable checks; `cleanup.json` records this in the evidence
directory. `anonymous-01` was a track-selection precheck and is explicitly
excluded from this comparison.

The subsequent Premium non-playing follow-up could not establish a valid
signed-in idle or paused window. Fresh extension handoffs passed local session
validation, but account bootstrap returned HTTP 302 before membership flags
were read. A separate public Go probe classified its redirect as a Google
challenge; no redirect was followed. A diagnostic-only experiment binding the
bootstrap to the selected account also returned 302 and was not promoted.
That failed experiment did not change production sources or delivery artifacts.
All diagnostic apps were closed. Failure evidence is retained in
`build/diagnostics/premium-idle-authenticated-20261028/blocked.json` and its
fixed-field stage records; `premium-idle-01` is explicitly excluded. These
failed-state samples do not establish Premium idle memory, and the successful
playback figures above remain playback-only results.

### Account bootstrap investigation and direct API correction (2026-10-28)

The blocked request was `GET https://music.youtube.com/`, used by the Go catalog
to scrape client version and visitor configuration. The membership query,
`POST /youtubei/v1/browse?prettyPrint=false`, had not yet run. Account verification
created a fresh catalog client each time, making the one-hour bootstrap cache
ineffective for the five-minute/focus checks. A failed HTML request was also
misreported as a reason to sign in again.

A bounded public comparison reproduced the Go HTTP/2 homepage request returning
302 with a Google challenge destination. A Python request with the same URL and
User-Agent returned 200. An explicit HTTP/1.1 Go attempt failed at the network
level; this does not establish a specific TLS, protocol, IP, or cookie cause.
The direct Go HTTP/2 WEB_REMIX browse request, using the installed youtubei.js
Music client version and no visitor header, returned 200 in 974 ms with explicit
`logged_in=false` and `music_premium=false` and no API error. Redirects were never
followed, and no challenge was solved.

The isolated direct-API candidate then accepted the authorized Premium session,
returned the server-verified `premium` category, loaded the native Library, and
passed another account refresh. Its executable SHA256 is
`28f4539709f3323edde4cf828a7f185296f66d6c93fd16a50bd95f165ba3b3e2`.
It retains the memory-only diagnostic account store; this is not Keychain
restore acceptance. Its runtime/resources are identical to the previous
delivery. Evidence is in `build/diagnostics/premium-direct-api-20261028/`.

The correction removes the catalog's homepage bootstrap and anonymous visitor
cache, and uses one WEB_REMIX version matching the pinned player SDK. It adds
no DOM library, browser runtime, alternate client, retry, or fallback. This
changes catalog/account requests only; anonymous/standard playback proof has
its own initialization and must not be described as fixed by this change.

Upstream research supports this approach:

- [NewPipeExtractor #872](https://github.com/TeamNewPipe/NewPipeExtractor/issues/872)
  specifically discusses avoiding HTML configuration extraction to reduce
  reCAPTCHA. Its [closing change](https://github.com/TeamNewPipe/NewPipeExtractor/commit/efd3d8540114b379819e6e78b387b655907314d3)
  moves WEB initialization to service-worker data; it is not a demonstrated
  fix for this exact Music/Go 302.
- [NewPipeExtractor #547](https://github.com/TeamNewPipe/NewPipeExtractor/issues/547#issuecomment-1140507701)
  records a maintainer's historical resolution after moving requests to
  InnerTube. This is not a guarantee against future service challenges.
- [YouTube.js Session](https://github.com/LuanRT/YouTube.js/blob/main/src/core/Session.ts)
  uses service-worker configuration and an SDK Music version, rather than
  requiring Music homepage HTML for that version.
- [ytmusicapi context initialization](https://github.com/sigma67/ytmusicapi/blob/main/ytmusicapi/helpers.py#L31)
  constructs its WEB_REMIX version independently; it can still fetch the
  homepage for visitor data, so it is not immune to homepage failures.

Research date: 2026-10-28. NewPipeExtractor had 2,030 stars and a YouTube code
fix on October 22; YouTube.js had 5,399 stars and released 18.3.0 on October 23;
ytmusicapi had 3,091 stars, a code fix on October 1 and test updates on October
12. No exact resolved upstream issue was found proving why this particular Go
request received a challenge while Python succeeded.

The production correction additionally distinguishes service/network errors
from explicitly invalid or expired sessions. Only the latter request account
reconnection. Packaging checks the Go Music protocol version against the actual
installed SDK and stops on disagreement; there is no automatic version change.
`go test -race ./...` and `make check` passed, including 80 runtime tests, five
manifest tests, five protocol-version tests, vet, and 186 localized messages.
`make build` produced the updated ad-hoc signed arm64 app and DMG. No installed
Keychain restore or public release is claimed by these checks.
The bundle's protocol/dependency manifests, strict deep app signature, DMG
integrity and embedded-app equality passed. The DMG itself remains unsigned.
Evidence: `build/diagnostics/premium-direct-api-20261028/artifact-direct-api.json`.
Updated executable SHA256:
`6a44936a87a03a0544cbafbedd62209fcd4b3e90983445e3cf970f15531f2fd0`.
Updated DMG SHA256:
`6c3782fc49c1632a311296bf3a47ba73c72052dd08eab3cbfee985948082b0d0`.
These replace the previous files at `build/darwin-arm64/`; earlier hashes in
this document remain historical evidence. Additional public checks of the
corrected catalog returned 30 anonymous Home items and 10 search items. One
Home continuation request completed but produced no parsed items, so this is
not a full live pagination acceptance claim.

The valid Premium idle follow-up used a fresh sign-in, the native Library page,
and no playback before the measurement. After at least 30 seconds of settling,
the half-open 120-second window contained 120 complete samples: **143.69 MiB
median, 144.21 MiB peak** across the app, descendants and responsible helpers.
Component medians were UI 75.86 MiB, Node 22.05 MiB, system helpers 45.78 MiB,
and FFmpeg 0. No WebKit or Brave process was in scope. This is one diagnostic
run with memory-only session storage, not a repeated-run or installed-app bound.
The seven process identities observed across the full run were absent after
normal quit. Portable results are in
`docs/audits/2026-10-28-account-memory/premium-idle.json`.

The subsequent attempt to measure pause-after-play did not start audio:
the separate player SDK initialization returned `init_failed`. An account-free
SDK probe also failed, and a public `/sw.js_data` status probe returned 302.
The exact redirect destination and parser failure were not established for
that SDK run. These are distinct from the corrected catalog request and are
not evidence of a Premium entitlement failure. No valid paused window was
recorded, and this catalog correction does not claim current end-to-end
playback acceptance.

### Player SDK bootstrap investigation follow-up (2026-10-28)

The original native diagnostic was re-read before relaunch. It contained both
`Category=init_failed` and **`Stage=bootstrap`**. The prior investigation missed
the stage field. `Resolver.configure()` saves the account without making a
request; the first warm/resolve runs the public `BootstrapSession` before
restoring the seed and passing the account to `EngineSession`. Consequently,
this failure preceded account validation, proof initialization, and the player
request. The public `/sw.js_data` request does not receive the saved account.

The old fetch wrapper followed redirects and the SDK could wrap its errors,
reducing the result to `init_failed`. The separate public 302 observation is
consistent with a bootstrap failure but does **not** prove the exact HTTP
response or redirect destination of that earlier native attempt. Neither an
IP/TLS cause nor an entitlement failure has been established.

The original runtime subsequently initialized successfully without a code
change. An isolated observation using the same Node permission launcher saw
three HTTP 200 responses with no redirects: session data, player identifier,
and player script. The session-data prefix/JSON/shape and player cache were
valid. The same native diagnostic executable (`28f4539709f3323edde4cf828a7f185296f66d6c93fd16a50bd95f165ba3b3e2`)
accepted Premium, started playback on its first attempt in that process, and
advanced through 39.181333333, 158.189333333, and 174.581333333 seconds before
normal pause. This was muted playback/clock validation, not an acoustic
recording. It demonstrates recovery of the previously failing build; it does
not establish that the intermittent upstream failure is eliminated.

The correction makes bootstrap fetch redirects explicit (`redirect=manual`)
and preserves the first transport failure through SDK wrapping, binary IPC,
the resolver, player state, and native diagnostics. Optional metadata is
limited to four closed fields: bootstrap substep, HTTP status, redirect class,
and MIME class. Network, timeout, TLS, and redirect categories are distinct.
Unknown/malformed metadata is rejected; successful replies and non-bootstrap
failures cannot carry these fields. Response bodies remain bounded streams.
No response text, headers, URLs, query values, account data, or SDK cache is
logged. There is no automatic retry, alternate client, or local session fallback.

`go test -race ./...` and `make check` passed, including 89 runtime/extension
tests, five manifest tests, five protocol-version tests, vet, and 186 localized
messages. Synthetic tests cover redirects, SDK-wrapped transport failures,
body-read failures, cancellation, and invalid metadata across both IPC
boundaries. One public bootstrap with the new implementation also succeeded
using the production permission launcher, with three requests. `make build`
produced the updated native app and DMG.

The updated bundle passed the dependency/protocol manifests, all 19 runtime
module comparisons, strict deep app signature, DMG integrity, and embedded-app
equality checks. The DMG remains unsigned. Evidence is
`build/diagnostics/sdk-bootstrap-20261028/artifact-bootstrap.json`.
Executable SHA256:
`90d238c0ee2ab0cd212b0f01d2295e659fa769cee850019b0eef999175d7cd46`.
DMG SHA256:
`42794f2643abe8d751ec41b9db3f90287c019881fc23d41241b8e3d43a0855ae`.
These replace the artifact files above; this does not establish installed
Keychain restoration or a public release.

A new isolated native diagnostic built from these tested sources also passed
its first Premium playback attempt. Its runtime and complete resource tree
match the formal bundle; only the account store (memory-only), application
identity, and window title differ. Executable SHA256:
`519a81eac28772768ef6e1859847bb5f9c99aecc863d82da14484eb0bdbb91dc`.
The service verified Premium and the native clock advanced through 18.776,
37.677333333, 42.925333333, and 52.024 seconds, before normal pause at
64.074666666 seconds. Volume was zero; no acoustic recording or full-track
completion is claimed. Signed-in Library, search, and Home loaded. The initial
anonymous Home showed a load error before sign-in; its HTTP cause was not
captured and anonymous Home reliability is not established by this test.
All five observed app/descendant PIDs were absent after normal quit. The
portable investigation record is
`docs/audits/2026-10-28-account-memory/sdk-bootstrap-investigation.json`.

The relevant upstream behavior is documented in
[YouTube.js 18.3.0 Session](https://github.com/LuanRT/YouTube.js/blob/v18.3.0/src/core/Session.ts)
and [PR 1066](https://github.com/LuanRT/YouTube.js/pull/1066), which introduced
fail-fast initialization because locally generated session data after a
server-data failure could break playback. Disabling fail-fast is not used as
a workaround. [PR 1276](https://github.com/LuanRT/YouTube.js/pull/1276) fixes an
optional configuration field and is already in the pinned SDK; it does not
prove a fix for this 302 observation. Research date: 2026-10-28. No exact
resolved upstream issue establishing this intermittent failure's cause was
found.

The successful original-runtime replay also completed a valid pause-after-play
measurement on the native Library page. Its first half-open 120-second paused
window had 120 complete samples: **176.95 MiB median, 179.06 MiB peak** across
the app, descendants, and responsible helpers. Component medians were UI
80.92 MiB, Node 73.96 MiB, FFmpeg 3.44 MiB, and system helpers 18.63 MiB.
No WebKit or Brave process was in scope. Playback position remained
174.581333333 seconds through the window, and all seven observed processes
were absent after normal quit. This is one memory-only diagnostic account
run of the prior runtime; it is separate from the fresh-before-play idle
measurement above. Portable evidence:
`docs/audits/2026-10-28-account-memory/premium-paused.json`.

Local request and native observations are under
`build/diagnostics/sdk-bootstrap-20261028/`. The previous exact HTTP cause
remains unknown; current successful initialization cannot retroactively prove it.

The measurements and hashes below describe earlier builds only.

## Historical anonymous implementation and measurements

The following records predate the account-aware implementation above. They
remain useful as baseline evidence; they do not establish current package
behavior, account support or memory use.

### Earlier implementation

- Native MyGo browsing and controls retain the original UI/UX. One prepared
  next-track lease and PCM decoder transfer ownership without resolving or
  starting a decoder on selection. Queue changes, seek, stop and close cancel
  obsolete preparation. Audio and on-demand video share the resolver.
- The outer Node process owns control messages and the bounded media bridge.
  It does not import YouTube.js, JSDOM or the proof provider. One restricted
  persistent engine owns the YouTube.js 18.3.0 anonymous WEB_REMIX session,
  signature processing, JSDOM and proof generator. SDK and proof no longer have
  separate persistent processes or duplicated module graphs.
- Initial anonymous Session/Player analysis runs in a short-lived bootstrap
  process. At most four opaque SDK cache entries, up to 8 MiB in total, cross a
  private binary pipe bounded to 16 MiB. The child exits and is reaped before
  the shared engine restores the cache with the same SDK and player ID.
  Restoration rejects initialization network requests as `bootstrap_cache_miss`;
  it cannot silently repeat player analysis. Normal player requests resume
  after restoration.
- Both `bootstrap-session.mjs` and `engine-session.mjs` launch Node with
  `--optimize-for-size`. This uses V8's memory-oriented policy without a heap
  cap, forced GC, slower UI updates or a change of DOM implementation. JSDOM
  29.1.1 remains the sole DOM. The Happy DOM diagnostic failed real audio
  verification and was not promoted; an isolated warm success was insufficient.
- The shared engine uses the complete, unmodified standard provider 2.0.2 at
  commit `26475e9d3665b972c4aeb83b4dbaafcf64b88438`. Warm-up initializes its
  generator; subsequent audio/video requests bind proofs to individual IDs.
  Provider cache refresh is local policy, not a server guarantee of proof
  validity. Refresh does not revoke existing media leases; signed media expiry
  still bounds them.
- Bootstrap and engine requests have 30-second deadlines and one active request
  per session. Engine binary IPC is bounded to 16 MiB requests and 128 KiB replies.
  Children use a minimal environment with runtime-only read and network
  permissions. Cancellation, malformed output, timeout and close terminate and
  reap the owned child. The native parent terminates its process group on exit.
  No disk proof cache, credentials, raw dependency logs, argv or environment
  contents enter diagnostics.
- The bridge supports four independent media capabilities/consumers, 64 KiB
  upstream ranges and 32 KiB writes with backpressure. No full-song cache is
  retained. Go and FFmpeg receive only local bounded stream endpoints.
- yt-dlp, client switching, old-player fallback and artificial advertisement
  delays are absent from the playback source and package.

### Earlier automated and playback checks

`make check` passed, including Go tests, vet and 172 messages in four languages.
All **43 Node offline tests** passed. The current app and DMG were rebuilt with
`make build`. Build success is separate from the partial whole-app checks below.

The race-enabled native playback matrix passed after both bootstrap and engine
received `--optimize-for-size`. Evidence:
`build/diagnostics/jsdom-replacement-20261027/compact-candidate/summary.json`
and `results.jsonl` in the same directory.

| Scenario | Current single network run |
| --- | ---: |
| Completely cold first Load | 11.285 s |
| Cold first observed output | 11.586 s |
| Background next-track preparation, three samples | 1.567–1.702 s |
| Prepared Load ownership transfer, three samples | 0.952–123.394 ms |
| Prepared native output observation, three samples | 97.608–234.620 ms |
| Unprepared random selection in existing session | 1.602 s Load / 1.697 s output |
| Fresh startup warm-up | 6.266 s |
| First Load after that warm-up | 1.694 s |

Output observation requires nonzero decoded PCM, playing state and at least
50 ms of native audio-clock advance. It is not a speaker recording or acoustic
latency. Every prepared handoff made zero new resolver and decoder calls. The
matrix exercised a 35-second prepared hold alongside active audio and 480×480
video, three prepared selections, random selection, pause, seek, close and a
fresh-player warm/load. Pause held position; a seek to 45 s advanced to 47.957 s
after approximately 3 s.

These are individual network samples, not guarantees of constant switching
latency. Cold output still took 11.586 s; this run does not establish a cold-start
improvement. The UI's natural-end polling remains 350 ms; sample-accurate gapless
playback has not been established.

The shared engine with `--optimize-for-size` also completed two whole-track
decodes, faster than real time. Track 1 produced 212.547 s of nonzero PCM against
212.561 s metadata; track 2 produced 369.627 s against 369.641 s. Both decoder
EOF/exit checks passed. Resolution took 0.931/0.856 s and first decoded PCM took
0.702/0.657 s. This test used the same persistent-engine code; the bootstrap
flag was added afterward and then covered by the complete matrix above. It is
not evidence of full-duration real-time listening. Evidence:
`build/diagnostics/jsdom-replacement-20261027/engine-size-full/results.jsonl`.

### Earlier playback memory comparison

The metric is the sum of captured-process `ri_phys_footprint`, covering the
Go race-test process and its Node/FFmpeg descendants. The original collector
filtered executable names before attribution and did not capture all responsible
system services. It also excludes application UI and artwork. These are matching
filtered scopes, not complete responsible-process totals. PID/start identity and
safe libproc sampling are used; argv, environments and credential stores are not read.

Both runs use the shared SDK/proof engine and JSDOM. The baseline has the default
V8 policy; the current candidate adds `--optimize-for-size` to engine and
bootstrap. Compare matching rows only:

| Matching scope / metric | Shared-engine baseline | Current compact runtime |
| --- | ---: | ---: |
| Active playback median | 261.62 MiB | 200.38 MiB |
| Active playback peak | 277.42 MiB | 206.97 MiB |
| Held prepared-track median | 273.51 MiB | 200.41 MiB |
| Held prepared-track peak | 277.42 MiB | 206.97 MiB |
| Entire sampled run median | 230.62 MiB | 199.80 MiB |
| Entire sampled run peak | 292.96 MiB | 225.19 MiB |

Active playback has 64 baseline / 63 current samples; the prepared-hold subset
has 32 / 33; the entire run has 85 / 86. Playback phases and the entire run have
different scopes and must not be interchanged. Sequential network runs also
have different timings; this is not a randomized performance benchmark or a
long-term leak test. No current whole-app total can be inferred from this table.

Evidence directories under `build/diagnostics/jsdom-replacement-20261027/`:
`merged-jsdom-baseline/summary.json` and `compact-candidate/summary.json`.

## JSDOM replacement and lazy-loading experiments — not promoted

Happy DOM's lower import footprint did not translate into reliable proof/audio
acceptance. Its clean candidate failed before PCM with `proof_failed`; a single
successful warm-up did not establish a working replacement. JSDOM remains the
only production DOM implementation, without a replacement or fallback path.

A separate JSDOM lazy-require prototype preserved the real CSS and selector
implementations, postponing their module imports until first use. It changed
12 dependency files; the other 5,454 runtime files were byte-identical. All
37 offline DOM/CSS cases passed with equal results. In three alternating offline
pairs, ordinary DOM construction footprint fell from 77.62 to 60.82 MiB. CSS use
loaded the module graph back to 937 modules; first selector/CSS use added roughly
66/47 ms. Two private CommonJS exports became getters, so this was a dependency
patch, not an entirely unchanged JSDOM package.

One complete live playback matrix also passed, including nonzero PCM, three
prepared transitions, concurrent video, random selection, controls and shutdown.
However, active-playback median was **192.90 MiB**, only **7.48 MiB (3.7%)** below
the unpatched compact runtime's 200.38 MiB; active peak was 202.40 MiB. The full
sampled-run median/peak were 190.91/207.14 MiB. Cold output was 23.047 s, prepared
output 97–100 ms, random output 9.727 s and fresh warm-up 11.093 s. These are
single network runs, not proof that lazy loading caused the latency differences.

The small observed playback benefit did not justify maintaining the 12-file
private patch or establish the desired whole-app improvement. **It was not
promoted or packaged.** No additional variants were added. Evidence:
`build/diagnostics/jsdom-lazy-20261028/REPORT.md`, `behavior-final/summary.json`,
`tree-verification.json`, and
`build/diagnostics/jsdom-replacement-20261027/lazy-jsdom-candidate/summary.json`.

### Earlier whole-app acceptance — partial, 2026-10-28

The exact locally built bundle identified below was launched with its real
native window. Signed-out search and muted playback were observed at the same
1440×940-DIP window size. The baseline was a separate diagnostic copy containing
the frozen shared-engine runtime without the size-policy flags. Its unsigned
application executable was byte-identical to the candidate; both bundles passed
deep/strict ad-hoc signature verification. This is not an installed-app test.

The baseline's marked playback interval contained 108 samples: captured-process
footprint median **399.91 MiB**, peak **401.07 MiB**. The candidate's fresh-process
marked interval contained 122 samples: median **411.80 MiB**, peak **415.82 MiB**.
Component medians were respectively 259.86/279.70 MiB for the native main process,
133.69/122.42 MiB for both Node processes combined, and 6.30/6.50 MiB for FFmpeg.
Component medians are separate statistics and need not sum to the total median.
No owned WebKit process was observed during these signed-out playback runs.

**Scope correction, 2026-10-28:** the original collector enumerated only executable
names matching ytmgo, Node, FFmpeg, yt-dlp and WebKit before checking responsibility.
It could therefore miss responsible native system helpers such as Metal compiler
and system view services. The figures above are incomplete application totals.
The earlier MyGo Cloud Music 0.1.19 measurement included those helpers (playback
main-process median 242.47 MiB; attributed total 288.53 MiB), so the two totals
must not be treated as identical-scope measurements. Historical omitted process
memory cannot be reconstructed or assigned a guessed amount. Collector repair
does not retroactively validate the old totals; a complete comparison needs new
sampling. The main-process and captured Node/FFmpeg figures remain observations.
The collector has now been changed to PID-only responsibility discovery plus
recursive descendant discovery, then `safe-processes --pid` for selected metadata.
Unexpected metadata-collector errors stop sampling instead of silently omitting
a process. Seventeen offline fixtures passed; the libproc adapter and a new
complete application comparison have not yet been exercised live.

**These samples do not establish a whole-app memory reduction.** Online Home and
search results were not frozen, and the candidate's final observation was a
different queue track. Its official-audio selection initially showed 4:08 and
progress 10.168 s; the final observation showed the next 10-minute item at
81.848 s. The first decoder disappeared during sampling. This prevents accepting
the entire interval as a matched same-track comparison. An earlier candidate
launch also produced `stream_resolution_failed`; retries and prior tracks make
that process unsuitable as a clean baseline. These failures are not hidden by
the successful isolated playback matrix.

Evidence is in `build/diagnostics/jsdom-replacement-20261027/app-summary.json`,
`app-baseline/`, `app-candidate/`, `app-candidate-steady/` and
`app-candidate-fresh/`. The last two candidate runs must not be conflated. Both
apps were quit. Metadata-only PID/start-identity checks confirmed that all 17
observed owned process identities were gone (`app-cleanup.json`). No user
installation was replaced.

Minimum-width, complete accessibility, physical media keys and all four
languages still require real-window coverage. Automated native view tests do
not replace those checks.

## Historical app-window run — previous package, incomplete process coverage

The previous package in `build/diagnostics/runtime-final-proof-20261027/app/`
passed signed-out Home/search, native audio, next track, mouse seek, pause,
queue, video/artwork switching, wide and half-screen layouts, and light/dark
appearance checks. No login was performed. Its five observed identities were
gone after quit. These results predate the shared-engine and V8-policy changes.

Its four-minute sample included the actual UI/artwork process, Node and FFmpeg.
Across 241 samples, median was **466.48 MiB** and peak **600.80 MiB**. Sampling
began after warm-up; manually marked phases include brief transitions. No owned
WebKit process was observed. A later paused snapshot was 286.09 MiB, which is
not an idle-memory guarantee. These are historical measurements, not the
current package's footprint or a controlled old-WebKit comparison.
The name-filtered collector also did not establish coverage of all responsible
native system helpers in this run; its totals must not be presented as complete.

That run established mouse seek, but AX `setValue` and keyboard attempts did
not establish it. MyGo 0.7.1's Darwin AX value setter dispatches text edits, not
slider assignments; a separate increment/decrement path was not exercised.
Minimum-width, complete accessibility, physical media keys and all-language
acceptance remained open.

## Backdrop optimization evidence — 2026-10-27

The current backdrop renders the existing cover crop and blur on two
quarter-size canvases, then uses `Painter.ImageRegion` at the final size.
Parent rounded clip, opacity and effective 70-DIP blur are preserved. No
dependency fork, slower progress updates or compatibility path was added.

The isolated Metal comparison used the same native view, eight synthetic
768-square artworks, a 1440×940-DIP window, Retina 2× and 350-ms progress updates.
It created no catalogue, player, account, tray or runtime helpers. Each row
has a 20-second stable window and 20 footprint samples; runs were sequential.
Go/Metal allocation counters are not added to physical footprint.

| Renderer process only | Original backdrop | Current layer |
| --- | ---: | ---: |
| Normal page median | 276.99 MiB | 265.53 MiB |
| Immersive page median | 308.45 MiB | 282.38 MiB |
| Paused/idle median | 105.67 MiB | 109.53 MiB |

Normal-page canvas allocation fell from 18.62 to 2.47 MiB; immersive sampling
fell from 35.84 to 7.22 MiB, including briefly retained normal-page layers.
Image textures stayed at 6.53 MiB. These allocation figures are not physical
memory savings. Both runs showed idle GPU trim; no idle-memory reduction is
claimed. All fixture processes were confirmed gone afterward.

Fifteen software comparisons against the original had maximum channel
difference 2/255 and mean absolute error 0.095–0.332/255; parent-clip exterior
pixels were unchanged. Five same-instance invalidation checks matched fresh
renders. The backdrop was also inspected in real Metal windows. A discarded
Scale(4) variant increased footprint and is absent from production.

An earlier whole-app follow-up was blocked by an anonymous bootstrap redirect
classified only as `www.google.com` / `sorry`, consistent with a verification
page. Its cause was not established; no redirect was followed or bypassed.
Empty/error-page samples were excluded. That historical interruption is not
presented as the status of the current acceptance run.

Evidence: `build/diagnostics/runtime-memory-opt-20261027/`, including
`isolated-renderer-fixture/comparison.json`, `backdrop-layer-comparison.json`
and `backdrop-layer-invalidation.json`.

## Earlier playback evidence and failures

The earlier bootstrap-isolated runtime, before SDK/proof merging, recorded a
native matrix median/peak of 300.84/375.66 MiB. Its preceding runtime recorded
328.70/576.51 MiB for that workload. These archived measurements belong to
different implementations and are not substituted for the current matching
baseline comparison. Evidence: `build/diagnostics/runtime-final-proof-20261027/`
and `build/diagnostics/runtime-production-proof-20261027/`.

The earlier `TestNativeEndedRestartLive` passed immediate tail seek, actual EOF
and restart without resolving again. Proof-free prototypes had returned 403 on
middle/tail reads; a controlled same-URL proof/no-proof comparison established
that proof affected access, not an advertising timer. One historical prototype
matrix failed after closing its first player without an established cause.
Current passing matrices do not retroactively explain those failures.

## Remaining application acceptance

- The account-aware implementation at the top of this file replaces the old
  login and playback paths. Its real-account, restoration and whole-app memory
  results must be validated independently of the historical records above.
- Normal advertising media, sequencing, interactions/progress reporting and
  transitions are not implemented. Content PCM does not validate advertisements.
- Current whole-app acceptance is partial in its dedicated section above;
  the matched comparison and candidate queue-transition behavior are unresolved.
  Native tests and historical screenshots do not establish all current UI/UX.
- The provider is GPL-3.0. Original source/maps/reconstruction files and notices
  ship with the package. Licensing-material gaps remain recorded in notices;
  public distribution has not been accepted. No public release, installation,
  Developer ID signing or notarization is claimed by this document.

## Historical local artifact

`make build` completed for `build/darwin-arm64/ytmgo.app` and
`build/darwin-arm64/ytmgo-0.4.0-arm64.dmg`, containing the shared SDK/proof engine,
JSDOM, both Node size-policy flags and the backdrop layer. Target: arm64,
minimum macOS 27, ad-hoc signing.

Build record: `build/diagnostics/jsdom-replacement-20261027/artifact.json`.
Version 0.4.0 was built at **2026-10-28 00:06:29.760971 +08:00**. This artifact
has **not been installed**; the older installed application is not evidence for
this working tree.

| Artifact | SHA-256 |
| --- | --- |
| App executable | `b3146047315c67ae3fb2b375d2ca56ba0b9fcd86a2e506aaa0caeb6bfcd5a56d` |
| DMG, 43,407,641 bytes | `a143212b10bd662d7f3bf35a08b76aa21dcb42857fe9cd744b07e16e5b388b44` |

Package review confirmed that all **14 runtime `.mjs` files match their source**
and that the runtime manifest checksums pass. The package contains Node 26.12.1
and JSDOM 29.1.1; both bootstrap and engine use `--optimize-for-size`. The old
separate proof helper and Happy DOM are absent. Deep, strict code-signature
verification passed for the ad-hoc-signed app. These package checks do not
establish installation, notarization or completion of the whole-app UI and
memory acceptance above.
