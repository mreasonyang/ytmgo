# Bundled runtime

`prepare.py` produces `build/runtime` with official Node 26.12.1, a minimal
FFmpeg 9.0.2 decoder, the IPC worker, and integrity-locked YouTube.js 18.3.0
dependencies and licenses. npm is used only at build time. The app needs no
external resolver, Python, npm installation, or browser engine.

Run `node worker.mjs` with private stdin/stdout pipes. Never display or retain
stdout: it carries local capability URLs. stderr and SDK logging are silent.
The worker imports paths relative to its own files, so its working directory
does not matter. The native launcher must supply a minimal environment.

## JSONL IPC

Each request has a unique positive safe-integer `id` and an `op`:

- `configure`: install a validated private `session` with `loggedIn`,
  `musicPremium`, `cookie`, `accountIndex`, `onBehalfOfUser` and `revision`.
  It cancels older work, waits for child exit and clears cached resolution state.
  The account revision increases only on identity changes; those also revoke
  all media handles. Membership changes retain the currently playing handle.
- `warm`: explicitly initialize the configured WEB_REMIX engine. Production
  startup does not call it; the first playback request initializes on demand.
- `resolve`: requires an 11-character `videoId` and `kind: "audio" | "video"`.
- `release`: requires the exact `url` returned by resolve; repeated release is
  successful while this worker remains alive.
- `cancel`: requires `target`, the id of a queued or running warm/resolve.

Configure must complete before warm or resolve. Input lines are bounded to
96 KiB (including at most 64 KiB of cookies); replies remain at most 4 KiB.
Credentials stay inside private pipes and the child memory. Never print or
record an input request.

Warm and resolve run serially, with at most 16 outstanding jobs and a 60-second
deadline including queue time. Release and cancel run immediately. Cancel has
its own acknowledgment, and its target receives one `request_cancelled`
response. A late result is released before that canceled response is emitted.
Unknown fields and invalid schemas return `invalid_request`. Fixed errors are
listed in `control.mjs`; arbitrary SDK or HTTP error text never leaves the worker.

## SDK initialization process

`bootstrap-session.mjs` runs SDK/player-script initialization in a short-lived
`bootstrap-worker.mjs` child. The child returns only a bounded SDK cache and
player ID over a private binary pipe, then exits. The parent waits for process
reaping before passing the cache to the restricted engine. The cache is copied in memory,
limited to four entries / 8 MiB, and never written to disk; the complete frame
has a 16 MiB limit. Neither cache contents nor private IPC may be logged.

Restoration inside the restricted engine uses strict cache reads and rejects network access during that
phase. A cache miss returns `bootstrap_cache_miss`; it never silently repeats
expensive player-script analysis in the persistent process. Subsequent normal
player requests use the persistent session. An explicit initialization may
start a new child after cancellation or failure; there is no automatic retry.
Timeout, cancellation, malformed output and close kill and reap the child.
The child has the same read-only runtime and network permissions described
below. This moves the temporary analysis allocation into a process whose exit
returns its memory, without adding an initialization step to each track change.

## Account-aware SDK and proof engine

The root worker owns only the JSONL control plane, media bridge, short-lived
bootstrap launcher and `EngineSession` proxy. It never imports YouTube.js or
the provider. `engine-session.mjs` owns one restricted `engine-worker.mjs`
process. That process restores the selected account's SDK session from the
public bootstrap cache. A Premium engine never imports the proof provider or
jsdom; a module guard rejects those imports in that child. For anonymous/free
playback it invokes the complete, unmodified standard SessionManager API from
the pinned provider;
protocol configuration is kept internal to that dependency. Its original source,
source map, license and reconstruction information live under `vendor/bgutil`.
There is one YouTube.js dependency version throughout the runtime. Both SDK
consumers resolve the same package in this child, sharing its ESM module graph.
The proof engine retains its DOM and generator while in use. Switching modes
terminates and reaps that process before another can start; dropping JavaScript
references alone is not used as a memory-reclamation mechanism.

Both the persistent engine and the short-lived bootstrap run Node with
`--optimize-for-size`. This selects V8's memory-oriented allocation and reclamation
policy for the large DOM/SDK graph. It does not impose an application heap cap
or call forced GC. The root media bridge keeps its default engine settings.
JSDOM remains the sole DOM implementation: the Happy DOM prototype did not
pass reliable live audio acceptance and is not part of the shipped runtime.

Anonymous/free initialization reads the selected official Music page's bounded
configuration text without executing the page or creating another DOM. Its
experiment flags determine whether GVS binds to the video ID, the signed-in
account's dataSyncId, or the anonymous visitor data. Subsequent resolves reuse
the generator with that binding. Proofs stay in Node memory
and private pipes. The provider's `expiresAt` is a local cache refresh deadline,
not a server validity guarantee; media leases still use signed media URL expiry.

Warm-up and ordinary track changes reuse this engine and its generator. The
root retains one bounded bootstrap seed so an explicit request after engine
failure or local cache expiry can restore a fresh child without re-analyzing
the player script. There is no retry inside a failed operation. Canceling a
running engine request kills and reaps that child; existing media leases in
the root bridge remain valid and continue streaming. Close destroys both.

The child receives a minimal environment, runtime-directory read access and
network permission, with no write/addon/spawn/worker permission. This is an
ordinary resource boundary, not a sandbox claim for malicious JavaScript.
Requests use exact service origins and bounded bootstrap, interpreter and
integrity routes. Account headers are accepted only on the selected YouTube
API and proof-configuration requests. The proof provider itself receives no
cookies. Redirects, alternate challenge routes, socket overrides and oversized
bodies are rejected. The
private binary protocol uses one length-prefixed V8 frame per request/reply,
limited to 16 MiB requests (including the at-most-8-MiB seed) and 128 KiB
replies. IDs, fields, numeric metrics, media destinations, formats and expiry
are validated. One pending operation has a 30-second deadline. Cancellation,
timeout, provider failure, invalid IPC and close kill and reap the child. The
root media bridge survives engine replacement. Native shutdown kills its
owned process group as well. Upstream media URLs and proof values may travel
only across these private Node pipes; they never enter native IPC or logs.

The provider is GPL-3.0. Bundled notices explicitly record upstream license
material gaps; a local build is not public-distribution acceptance.

Successful responses are `{id, ok:true, media?, safe?}`. Resolve returns
`media: {url, duration}` (seconds), plus integer `width` and `height` for video.
Only an opaque `http://127.0.0.1:<port>/<random>` capability is returned; upstream
URLs and headers remain inside Node. `safe` contains numeric timings/counts and
fixed client/version/codec labels. The native parent may discard it.

The bridge retains at most four independent handles and supports four active
HTTP consumers. Publishing never revokes an existing handle. The fifth publish
returns `capacity_limit`; callers must release handles they no longer need,
including results from superseded requests. EOF, termination, and explicit
release cancel associated upstream requests and free the listener/handlers.

## Transport and evidence boundary

Every upstream request uses the same deciphered URL, a 64 KiB URL `range`, its
`cpn`, and the pinned SDK's stream headers. Bodies must match the requested byte
length. Forwarding uses 32 KiB writes and respects downstream backpressure.
There is no full-file cache, retry client, or alternate playback backend.
Headers and upstream reads have a 20-second idle timeout; waiting for a paused
local consumer has no network-idle timer or short overall request deadline.
Handles expire 30 seconds before the actual signed URL expiry, with a 26-hour
absolute upper bound. Preparing a handle does not start a simulated playback
timer. Missing or expired signatures fail explicitly.

Audio supports clear Opus/AAC formats; video selects clear H.264 MP4 up to 480p.
DRM, SABR-only responses, login requirements, and unsupported formats are
explicit errors. The worker has no credential-store access; its parent supplies
the verified account over private IPC. It does not calculate advertisement
timing, impose an artificial startup delay, or implement advertisement playback
and progress reporting. Anonymous content transport success is not evidence of
normal advertising behavior or authenticated account benefits.

Run `make test-runtime` for the runtime and account-extension offline tests;
`make check` includes this target. Preparation installs the locked dependencies
only in `build/runtime`, using empty npm configuration and disabled install
scripts. It may download public build dependencies; the tests themselves use
injected synthetic upstream data and loopback HTTP, with no YouTube or real
account access. No source-tree `node_modules` installation is required.

The runner uses the bundled Node, temporarily stages the runtime tests beside
the built modules, and removes its own staging directory on success or failure.
An existing staging directory is an error, never overwritten. Only numeric test
counts and a fixed result category are printed. Do not run packaging concurrently
with this target; after it exits, test files are absent from the runtime bundle.

`manifest.json` declares `node_modules_hash_policy: "mygo-visible-v1"`.
`runtime_manifest.py` computes that digest over MyGo's resource projection:
dot-prefixed entries at every depth are omitted, matching MyGo 0.7.1's copy
rules. The same function verifies a prepared runtime or the bundled runtime;
the manifest must not be checked using an unfiltered npm installation tree.
`make test-runtime` includes synthetic tests for this copy/hash agreement.
Node and FFmpeg are signed before the manifest is written. Changing to a
distribution signing identity requires checking the final nested-code signing
order before generating hashes; never edit a manifest inside a sealed App.

After preparing the runtime, the native prepared-playback acceptance test can
be explicitly run by the operator controlling the measurement workload:

```sh
YTMGO_RUNTIME_DIR="$PWD/build/runtime" YTMGO_LIVE_PREPARED=1 YTMGO_LIVE_EVIDENCE="$PWD/build/diagnostics/runtime-memory-opt-20261027/merged-engine-live" go test -race -count=1 -run '^TestNativePreparedPlaybackLive$' ./internal/player
```

The evidence directory must be fresh. This performs real anonymous networking
and native audio, so do not run it alongside UI or memory comparison workloads.
It validates cold/warm loads, prepared handoff, concurrent audio/video,
pause/seek and cleanup. Full-track decoding and real application UI acceptance
remain separate gates. Module sharing is a structural change, not a measured
memory-saving claim until the same workload has been sampled.
