# Native playback runtime

`prepare.py` builds the same runtime layout on macOS, Windows and Linux:
`ytmgo-playback-host[.exe]`, `ffmpeg[.exe]`, licenses and a content manifest.
There is no Node binary, npm installation, jsdom, Axios, or full YouTube.js SDK
in the app. The packager rejects stale legacy files and replaces the output
only after the new tree has passed checksum verification.

## Ownership

`internal/streaming` handles session bootstrap, selected-account authentication,
Innertube requests, proof configuration, format selection, URL transformation,
proof expiry, media leases, cancellation and bounded HTTPS range requests.
Account cookies remain in Go memory and are sent only to music.youtube.com.
Signed media URLs stay in Go; FFmpeg receives an opaque per-decoder loopback
capability through stdin. The former second HTTP relay is gone.

The native helper has three private JSON-line modes:

- `extract`: load only the pinned MIT YouTube.js extraction algorithms and ISC
  Meriyah parser; return extracted code and timestamp, then exit. A small plain
  AST clone adapter supplies Meriyah's host structuredClone requirement; the
  parser retains start/end offsets without redundant per-node range arrays.
- `eval`: QuickJS with a fresh realm per call, 32 MiB interpreter allocation
  bound and a three-second execution interrupt. No filesystem, network, process
  or Node module APIs are registered.
- `dom`: a hidden, nonpersistent Wry system WebView for MIT BgUtils. Go supplies
  the allowlisted interpreter and integrity response. Page scripts, images and
  subresources from YouTube HTML are not loaded. CSP disables network/media,
  audible APIs are disabled, and callbacks return through private IPC. A fixed
  local document keeps the required HTTPS origin: WebKit base URLs on macOS /
  Linux, and a dedicated temporary-folder host mapping on Windows. It does not
  load the remote page; subsequent navigation is denied.

These are resource/process boundaries, not a claim of an OS sandbox. The host
has fixed failure categories, bounded input/output and a watchdog. Graceful EOF
permits WebView cleanup; cancellation also has a bounded forced-termination
fallback. macOS explicitly selects a nonpersistent WKWebsiteDataStore, Windows
uses a private temporary WebView2 profile, and Linux uses incognito WebKitGTK.

Premium never creates the proof WebView. Same-account membership changes retain
current media leases but dispose of obsolete script/proof state. Identity
changes revoke every old lease. Expiry disposes of idle hosts. A maximum of four
leases and four active media requests covers current audio, next-track prefetch,
on-demand video and transitions. Audio ranges are 64 KiB, video ranges 512 KiB;
forwarding uses 32 KiB with backpressure and no whole-track cache.

## Build

Install Go, Rust/Cargo, Python 3 and native C build tools. Linux also requires
GTK 3/WebKitGTK 4.1 development packages. Windows FFmpeg builds under MSYS2
UCRT64 with gcc and make; WebView2 is the same prerequisite as the login helper.
The native application is cross-platform, but each platform's full bundle and
real playback require native-host acceptance.

`native/assets.lock.json` pins three public source archives by SHA-512. The
build-only Go esbuild module bundles about 150 KB of script assets into the
native executable. It does not run npm or package-install scripts. Cargo.lock
pins the Rust closure; FFmpeg 9.0.2 source is SHA-256 verified and built with the
existing limited codec/protocol configuration. License texts and build metadata
are copied into the runtime. QuickJS needs no V8/JIT signing entitlement.

```sh
python3 scripts/runtime/prepare.py
python3 scripts/runtime/prepare.py --verify
python3 -m unittest discover -s scripts/runtime -p 'test_*.py'
python3 scripts/runtime/run_tests.py
```

The last command runs only offline native protocol/DOM fixtures and streaming
unit tests. It never resolves a track, starts FFmpeg or opens an audio device.
Linux runs it under `xvfb-run -a dbus-run-session --`. The three-platform
`native-playback.yml` workflow performs these checks without credentials.

Public anonymous initialization tests are separately opt-in through
`YTMGO_TEST_ANONYMOUS_WARM=1` and `YTMGO_TEST_NATIVE_HOST`. They exercise public
session/script/proof initialization only, never `/player`, media bytes or audio.
They do not establish real playback acceptance. Existing live playback tests
remain explicitly gated and must not run while the user has prohibited playback.

## Acceptance state

This migration is a candidate, not an installed or published release. See the
2026-10-31 entry in the root verification record for measured package sizes,
quiet checks, outstanding native-platform checks and deferred real playback /
whole-application memory measurements. Historical Node measurements must not be
reported as measurements of this implementation.
