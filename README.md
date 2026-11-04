# ytmGO

**English** · [简体中文](README.zh-CN.md)

**ytmGO** is a native desktop client for YouTube Music (version baseline **0.6.3**). The user-facing brand is **ytmGO**, while the repository, release artifact prefix, bundle identifier (`si.kyun.ytmgo`), helper identifiers, and local data directories retain the technical name `ytmgo`.

The music interface is drawn natively in Go with **MyGo 0.7.1** (`ui.View`)—there is no embedded web music page, HTML/CSS/JS UI bridge, Electron runtime, or bundled Node.js. Go handles YouTube Music catalogue requests, session verification, format selection, and bounded media streaming; MyGo outputs decoded audio to the system audio device, and an on-demand native decoder renders video only while a video view is visible.

> **Release 0.6.3:** [Release notes and downloads](https://github.com/mreasonyang/ytmgo/releases/tag/v0.6.3) for **macOS arm64**, **Windows x64** and **Linux x64**. This release includes the native lyrics, library snapshot, restart restoration and ytmGO branding updates described below. See [Mac release verification](docs/audits/2026-11-04-release-0.6.3/README.md) and [Windows/Linux package verification](docs/audits/2026-11-04-desktop-release-0.6.3/README.md) for acceptance scope.

---

## Installation & Updates

### Homebrew (macOS)

```sh
brew install --cask mreasonyang/taps/ytmgo
```

Requires **macOS 27+ on Apple Silicon**. Homebrew installs the same signed and notarized app as the direct download. To update:

```sh
brew update
brew upgrade --cask --greedy mreasonyang/taps/ytmgo
```

The app also supports in-app updates. Quit ytmGO before switching installation methods; if a manually installed copy already exists, back it up and move it out of `/Applications` before installing with Homebrew. `brew uninstall --cask ytmgo` keeps account data and preferences. See [Homebrew maintenance](docs/HOMEBREW.md).

### macOS Prebuilt Release

1. Download the signed and notarized DMG from [GitHub Releases](https://github.com/mreasonyang/ytmgo/releases/latest) (requires **macOS 27+**, **Apple Silicon / `arm64`**).
2. Drag the app into a writable `/Applications` directory before launching (running directly from a read-only mounted DMG prevents in-place updates).
3. When installed in a writable location, ytmGO checks for updates automatically (earliest 10 seconds after launch, then once per day) and manually via **ytmGO → Check for Updates…**.
4. The native update window supports **Install Update**, **Later**, **Skip**, and optional automatic downloads, relaunching to replace the app bundle in place while keeping your encrypted account session, library snapshot, and preferences.
   - *Note:* Version `0.4.0` did not include an updater and requires a one-time manual installation of `0.5.0` or newer.

### Windows and Linux

| Platform | Downloads | Requirements |
| --- | --- | --- |
| Windows x64 | [Setup EXE](https://github.com/mreasonyang/ytmgo/releases/download/v0.6.3/ytmgo-setup-0.6.3-amd64.exe) · [Portable ZIP](https://github.com/mreasonyang/ytmgo/releases/download/v0.6.3/ytmgo-0.6.3-windows-amd64.zip) | WebView2 Runtime; installer is not Authenticode signed |
| Linux x64 | [DEB](https://github.com/mreasonyang/ytmgo/releases/download/v0.6.3/ytmgo_0.6.3_amd64.deb) · [tar.gz](https://github.com/mreasonyang/ytmgo/releases/download/v0.6.3/ytmgo-0.6.3-linux-amd64.tar.gz) | Ubuntu 24.04 or compatible GTK 3/WebKitGTK 4.1 2.50.2+ and Secret Service |

For the DEB, run `sudo apt install ./ytmgo_0.6.3_amd64.deb`, then launch ytmGO from the applications menu. Extract portable archives completely and keep the `runtime`, `login`, `legal` and other resources beside the executable. Both platforms have Ed25519-signed in-app update archives. See [installation and release details](docs/DESKTOP_RELEASES.md).

---

## Everyday Usage

### Browsing & Library

- **Direct Catalogue Access:** Browse **Home**, **Explore**, **Search** (`⌘K` / `Ctrl+K`), **Library** (`⌘L` / `Ctrl+L`), and **Album / Playlist / Artist** pages directly from YouTube Music.
- **Navigation & Pagination:** Scroll near the end of a page to load continuations automatically. **Back / Forward** history preserves the loaded page data for each visited view. Diagonal trackpad/wheel gestures over horizontal song or album shelves route cleanly to the dominant scroll axis.
- **Account-Scoped Library Snapshot:** After your signed-in library loads (including paginated continuations), ytmGO saves a local metadata snapshot (`account/library-snapshot.json`) bound to the current encrypted session. On restart, your saved library appears immediately while refreshing in the background; if an update fails, the saved list remains visible with an explicit **Retry** button. Unverified private entries stay disabled while signed out or offline, and signing out or switching accounts clears the snapshot.

### Playback & Restart Restoration

- **Transport Controls:** Click any song or collection play button to start listening. Controls include Play/Pause (`Space` inside the app, or `⌘P` / `Ctrl+P`), Previous/Next (`⌘←` / `⌘→` or `Ctrl+Left` / `Ctrl+Right`), seek bar with hover/drag time readout, volume (`⌘↑` / `⌘↓` or `Ctrl+Up` / `Ctrl+Down`), mute (`⇧⌘M` / `Ctrl+Shift+M`, preserving your previous unmuted volume), shuffle, repeat (**Off**, **All**, **One**), Like/Unlike, and right-click context menus (**Play**, **Add to queue**).
- **Restart Session Restore:** On normal quit (`⌘Q` / `Ctrl+Q`), ytmGO saves your current queue, selected track index, playback position bookmark, volume, and unmuted volume (`playback-session.json`). On the next launch, it restores the queue, track, volume/mute state, and exact position in a **paused** state (publishing paused system Now Playing metadata) until you explicitly press **Play**. Reconnecting a saved account after an offline launch retains the restored paused bookmark; explicit new sign-in or sign-out clears the old queue.
- **Next-Track Prefetch:** No script host or proof engine is warmed at startup. While a track plays, Go prepares at most the next queued track's stream and initial PCM so natural transitions start without re-resolving; random selection, cold start, seeks, queue changes, and account switches cancel obsolete preparation.

### Now Playing, Synchronized Lyrics & Video

- **Now Playing Views:** Click the artwork/title in the bottom player bar (or press `⌘J` / `Ctrl+J`, or click the **Lyrics** / **Queue** buttons) to open the **Now Playing** panel, or toggle full-window **Immersive Mode** (`⇧⌘F` / `Ctrl+Shift+F`).
- **Default Lyrics on First Play:** When playback starts for the first time in a session, Now Playing opens automatically on the **Lyrics** tab—unless you have already opened or dismissed Now Playing or Immersive Mode during that session.
- **Live Follow & Manual Reading:** When synchronized lyrics are available, the current cue is highlighted and kept centered inside the lyrics viewport without moving the outer panel. Scrolling with a mouse wheel, trackpad, keyboard, or scrollbar switches to manual reading and reveals a **Follow playback** button below the lyrics viewport; clicking it (or pressing `Tab` then `Return`) returns to live following. Tracks without synchronized timing display plain lyrics or an unavailable notice.
- **Drag Lyrics to Seek:** On tracks with synchronized lyrics, drag vertically inside the lyrics area to preview target cues (highlighting the selected line against a center guide with a `<time> · Release to seek` badge). Releasing the pointer seeks to that cue's start timestamp and resumes live following while preserving play or pause state. Single clicks, horizontal movement, pressing `Escape`, or losing focus cancels the gesture without seeking.
- **Expandable Lyrics Layout:** On the **Lyrics** tab (outside Immersive Mode and Video mode), click the expand button beside the **Lyrics / Queue** chips to shrink the cover art and give more vertical height to lyrics. Click again to restore the cover size; this choice is saved in `lyrics-layout` across restarts.
- **On-Demand Video:** Switch between **Cover** and **Video** in Now Playing or Immersive Mode. Video is decoded on demand by a dedicated native decoder (up to **480p / 24fps**, synchronized to the audio clock, preserving aspect ratio). Hiding or minimizing the window, closing Now Playing/Immersive Mode, or switching back to **Cover** immediately releases the video decoder while audio continues uninterrupted.

### Responsive Layout, Localization & Shortcuts

- **Adaptive Window Geometry:**
  - **Wide (`>1180` DIP):** Full collapsible library sidebar (`⌘B` / `Ctrl+B`), main page panel, and docked right-hand Now Playing panel.
  - **Medium (`761–1180` DIP):** Compact 72-DIP icon rail and floating Now Playing dialog.
  - **Compact (`≤760` DIP):** Single-column page, floating mini player card, and bottom navigation tab bar.
- **Languages & Themes:** Settings (`Settings → General`) supports **English**, **简体中文**, **繁體中文**, **日本語** (plus **Match system**), and **Dark**, **Light**, or **System** appearance. Changes redraw the active native window immediately without interrupting playback.
- **System Integration & Shortcuts:** Native application menus, system tray, OS Now Playing media controls, and a configurable global play/pause shortcut (`⇧⌘Space` / `Ctrl+Shift+Space` by default, recordable or disableable in `Settings → Keyboard`) share the same playback controller. Press `⌘/` (`Ctrl+/`) or open `Settings → Keyboard → View all` to inspect all in-app shortcuts.

---

## Account Sign-In, Security & Platform Limits

### Isolated System WebView Sign-In

- Clicking **Sign in** launches a separate, temporary Wry native WebView helper process (`WKWebView` on macOS, `WebView2` on Windows, `WebKitGTK` on Linux) showing Google's official sign-in page.
- The login helper uses a distinct **ytmGO** icon with a teal account badge and keeps its own Dock / taskbar / window-switcher entry so multi-step verification is easy to return to; clicking the main app or Dock icon while sign-in is active focuses the existing login window.
- Normal interactive sign-in has no forced application timeout. Once sign-in completes, the helper reads the selected account's cookies from the private WebView store, hands them to the main process over an anonymous parent-child pipe, and exits. The main UI and audio playback never depend on the login window.
- There is no browser extension, local HTTP login bridge, OAuth client configuration, or temporary cookie file. Closing the login window cancels sign-in and keeps any previously connected account. External links open in your system default browser.

### Passkey & Platform Authentication Limits

- **macOS (`WKWebView`):** Google passkey sign-in is **not supported** in the macOS login window. Apple requires `WKWebView` passkey relying parties to have an Associated Domains relationship with the application (or a restricted web-browser entitlement), which an independent client cannot obtain for `google.com`. Use another verification method offered by Google's sign-in page; accounts that only allow passkeys cannot currently complete sign-in on macOS. See [Apple Passkeys Documentation](https://developer.apple.com/documentation/authenticationservices/supporting-passkeys).
- **Windows (`WebView2`):** Requires Microsoft Edge WebView2 Runtime (`>= 101.0.1210.39`). While WebView2 supports FIDO2, ytmGO has not yet been live-tested against specific Google passkey providers on Windows.
- **Linux (`WebKitGTK`):** Requires WebKitGTK 4.1 `>= 2.50.2` and `libsecret` (`Secret Service`). Version `2.50.2` avoids an upstream WebKitGTK hang on Google accounts with WebAuthn second factors, though this does not establish Linux passkey support. See [WebKitGTK 2.50.2 Release Notes](https://webkitgtk.org/2025/11/19/webkitgtk2.50.2-released.html).

### Single-Key Encrypted Session Storage

- Your full session (cookies and selected account metadata) is encrypted with **AES-256-GCM** (`YTMSESS\x03` authenticated header, fresh 12-byte random nonce, ciphertext, and 16-byte tag) in `account/session.enc` under the app data directory (`0700` directory and `0600` file on macOS/Linux; per-user AppData on Windows).
- The OS secret store (macOS Keychain, Windows Credential Manager, Linux Secret Service) holds only a single 32-byte random encryption key under the fixed entry name `account-session-key`.
- Before opening the login window, ytmGO prepares this key in memory: reusing a restored key, reading `account-session-key`, or creating it only when explicitly not found. If secret-store access is denied, locked, or malformed, sign-in fails immediately before opening the login window without overwriting existing data.
- After login and YouTube membership verification succeed, ytmGO encrypts and atomically replaces `account/session.enc` using the prepared in-memory key, without making another OS secret-store call. Only this single encrypted format is read or written; there is no legacy migration or plaintext fallback.
- On macOS startup, automatic restoration reads Keychain silently without triggering a system authorization dialog (`NoPrompt`). If restoration takes longer than 12 seconds or is deferred, the UI offers **Continue without account** and **Restore account** so you can listen to public tracks anonymously while keeping your saved session intact.
- Confirming **Sign out and clear local data** stops playback, deletes `account/session.enc` first, and then deletes `account-session-key` from the OS secret store (allowing retry if key deletion fails, even after the file is gone). Signing out of ytmGO never affects your Google session in external browsers.

### Membership Polling & Service Redirects

- Account status is determined strictly from server-returned `logged_in` and `has_unlimited_entitlement` booleans on startup restoration, sign-in, window focus, and every 5 minutes. Network errors or missing fields are treated as transient errors, never as membership expiration.
- **Premium mode** never loads DOM or proof modules; **standard (non-Premium) and signed-out modes** launch the proof engine on demand when playback requires it. Same-account membership changes reap obsolete proof engines while keeping the currently playing stream; account identity changes or sign-out revoke old streams.
- When Google or YouTube returns a verification, consent, sign-in, or unknown redirect page, ytmGO explains the requirement inline and keeps ordinary **Retry** actions for catalogue and playback requests. It does not offer unverified web-verification flows, mislabel ordinary HTTP `403`/`429`/network errors as CAPTCHAs, or save/open server redirect URLs.

---

## Runtime Architecture, Network & Privacy Boundaries

- **Node-Free Native Playback Runtime:** `make build` packages the Go main executable alongside a small Rust script host (`ytmgo-playback-host`) and a minimal **FFmpeg 9.0.2** binary. Dynamic player script extraction uses pinned MIT **YouTube.js** extractor subsets and ISC **Meriyah** in a temporary **QuickJS** process that exits immediately after returning bounded transform code; signature evaluations run in a fresh QuickJS realm per call (32 MiB limit, 3-second interrupt). Anonymous/free proof generation runs MIT **BgUtils** inside an isolated, hidden, non-persistent Wry system WebView with network/media blocked by CSP. No Node binary, npm tree, jsdom, or full YouTube.js SDK is shipped. Details: [scripts/runtime/README.md](scripts/runtime/README.md).
- **Bounded Media Streaming:** Remote media URLs and account credentials stay in Go memory. FFmpeg connects only to a per-decoder random loopback capability URL passed over `stdin`. Go fetches remote media in **64 KiB** audio ranges and **512 KiB** video ranges, forwarding with **32 KiB** backpressure across at most 4 active leases/requests and without caching full tracks on disk.
- **Proxy & Artwork Policy:** Catalogue, account verification, artwork, and media requests share `internal/netproxy`. Explicit `HTTP_PROXY`, `HTTPS_PROXY`, and `NO_PROXY` environment variables take priority on all platforms; on macOS, normal launches also read system HTTP, HTTPS, and SOCKS proxy settings and bypass rules. Local decoder loopback traffic always connects directly. PAC scripts and proxy passwords are not supported. Visible artwork is prioritized and retries with exponential backoff (2s to 1m). Explicitly public-cacheable covers are stored on disk up to **64 MiB / 512 files** for up to **24 hours**; account avatars are never written to the disk cover cache.
- **Sanitized Diagnostics:** Local files under `native-diagnostics/` (`audio-status.json`, `video-status.json`, `account-status.json`) record only fixed status/failure categories, timestamps, and app version (plus closed-set stage, bootstrap step, HTTP status code, redirect category, and MIME type on playback init failures). Media URLs, headers, cookies, account identifiers, process arguments/environments, and raw logs are never written.

---

## Verification Scope & Current Caveats

Detailed design specifications live in [DESIGN.md](DESIGN.md); historical and local verification records live in [VERIFICATION.md](VERIFICATION.md) and [docs/AUTO_UPDATE_VERIFICATION.md](docs/AUTO_UPDATE_VERIFICATION.md). Current boundaries to keep in mind:

- **Platform Acceptance:** On 2026-11-04 at commit `8c88037`, both 3-platform GitHub Actions suites—[Native playback runtime](https://github.com/mreasonyang/ytmgo/actions/runs/41800458515) and [Native account login](https://github.com/mreasonyang/ytmgo/actions/runs/41800458464)—passed across macOS, Windows, and Linux. The Windows/Linux [package workflow](https://github.com/mreasonyang/ytmgo/actions/runs/41834832878) additionally builds on the corresponding OS, installs the NSIS/DEB package and verifies installed resources and startup in an empty account profile. Real signed-in playback, complete native UI interaction, hardware audio and prior-version Windows/Linux upgrades remain outside that smoke-test scope; the Mac acceptance records are separate.
- **Account Tiers & Advertising:** Anonymous full-track playback and real **YouTube Music Premium** browsing, library restoration, and playback have been verified on macOS. **Standard (free/non-Premium) accounts** and **official advertising flows** are **not** accepted: ytmGO does not implement official ad asset sequencing or ad progress reporting, does not simulate ads with artificial wait timers, and does not treat public track playback as ad-flow validation.
- **Memory Footprint:** Low-memory optimization is **not** claimed complete. The current workspace has no new whole-app memory benchmark; dated measurements for earlier builds are recorded in [VERIFICATION.md](VERIFICATION.md), and historical Node-based figures are not comparable to the current runtime.
- **Global Shortcuts & Long-Run Stability:** In-app shortcuts (`Space`, `⌘K`, etc.) and UI Tester state machines pass, but physical global shortcut key injection (`⇧⌘Space`) has not yet been confirmed end-to-end on macOS. Local E2E runs also recorded isolated transient playback errors (`audio_stream_failed`, `bootstrap_network_timeout`) that recovered via **Retry** whose upstream/network root causes remain open. Crash/force-kill persistence, sleep/wake endurance, and multi-day stability are not yet certified.

---

## Building & Development

### Prerequisites

- **All platforms:** Go **1.27.3+**, Rust/Cargo, Python 3, and a C compiler toolchain.
- **macOS:** Xcode Command Line Tools (`darwin/arm64` release packaging targets **macOS 27+**).
- **Windows:** MSYS2 UCRT64 (`gcc`, `make`, `diffutils`) for building minimal FFmpeg, plus Microsoft Edge WebView2 Runtime at runtime.
- **Linux:** `build-essential`, `pkg-config`, `nasm`, GTK 3, `libwebkit2gtk-4.1-dev` (`>= 2.50.2`), `libsoup-3.0-dev`, and `libsecret-1-dev` (plus `xvfb`, `xauth`, and `dbus-x11` for headless runtime tests).

Installed application bundles do not require Node.js, npm, Rust, or Python.

### Common Commands

```sh
# Run Go tests, runtime/login/license checks, go vet, and 4-language i18n check
make check

# Build the packaged app and DMG (macOS arm64 outputs to build/darwin-arm64/)
make build
open build/darwin-arm64/ytmGO.app

# Build the app bundle without DMG and compile cmd/update-package
make build-app

# Run in development mode (prepares build/runtime and build/login first)
make dev
```

Local `make build` produces `build/darwin-arm64/ytmGO.app` and `build/darwin-arm64/ytmgo-0.6.3-arm64.dmg` with ad-hoc app signing and an unsigned DMG. Note that when a local rebuild changes the ad-hoc code-signing identity on macOS, Keychain may ask you to re-authorize access to `account-session-key`. For GitHub Actions Developer ID signing, Apple notarization, and signed update feed publishing, see [macOS Actions Guide](docs/MACOS_ACTIONS.md).

Packaged builds also support non-interactive maintenance flags that exit before starting the music window or accessing account storage:

```sh
/path/to/ytmGO.app/Contents/MacOS/ytmGO --version
/path/to/ytmGO.app/Contents/MacOS/ytmGO --check-for-updates
/path/to/ytmGO.app/Contents/MacOS/ytmGO --install-update
```

### Repository Layout & Targeted Tests

```text
main.go / controller.go         Window lifecycle, menus, navigation, queue & request generations
updates.go                      Native updater plugin setup, menu item & CLI maintenance flags
native_views.go / native_style.go  Native MyGo screens, responsive layouts, palette & vector icons
native_settings.go              Settings & Keyboard Shortcuts sheets
native_lyrics.go / native_lyrics_drag.go  Synced lyrics follow, manual reading & drag-to-seek
lyrics_layout.go                Persistent standard/expanded lyrics layout preference
playback_session.go             Normal-quit playback bookmark & queue persistence
library_snapshot.go             Account-scoped library snapshot display & background sync
global_shortcut.go              Global play/pause shortcut recorder & OS registration
account.go                      Login helper handoff, 5-minute entitlement polling & sign-out
artwork.go                      Prioritized viewport cover loader, backoff retry & disk cache
video_controller.go             On-demand video lifecycle & 24fps frame updates
diagnostics.go                  Allowlisted audio/video/account diagnostic status files
internal/accountsession/        AES-256-GCM session file & single-key OS secret store
internal/accountlogin/          Parent-child pipe protocol & login helper process management
internal/catalog/               Typed Innertube client, browse/search/continuation, queue & lyrics
internal/librarysnapshot/       Sanitized library snapshot serialization
internal/netproxy/              Environment & macOS system proxy selection
internal/player/ / internal/streaming/   Bootstrap, QuickJS/WebView proof host, range relay & FFmpeg PCM/video
native/account-login/           Cross-platform Wry temporary Google sign-in helper (Rust)
native/playback-host/           QuickJS extractor/evaluator & non-persistent proof WebView (Rust)
locales/                        English, Simplified Chinese, Traditional Chinese & Japanese catalogs
scripts/runtime/                Locked playback runtime & minimal FFmpeg 9.0.2 builder
scripts/login/                  Login helper builder & platform binary verifier
scripts/licensing/              Bundled legal notices & LGPL/MPL corresponding source collector
scripts/release/                macOS signing, notarization, release publication & update checks
scripts/diagnostics/            Safe process-tree physical-footprint collector (no argv/env reads)
```

```sh
# Race detector across all Go packages
go test -race ./...

# Player unit tests with the prepared runtime
YTMGO_RUNTIME_DIR="$PWD/build/runtime" go test ./internal/player

# Opt-in live catalogue & playback checks
YTMGO_LIVE_CATALOG=1 go test -v -run TestLive ./internal/catalog
YTMGO_RUNTIME_DIR="$PWD/build/runtime" YTMGO_LIVE_PLAYBACK=1 go test -v -run TestNativePlaybackLive ./internal/player
YTMGO_RUNTIME_DIR="$PWD/build/runtime" YTMGO_LIVE_PREPARED=1 go test -v -run TestNativePreparedPlaybackLive ./internal/player

# Deterministic MyGo software-renderer UI snapshots
YTMGO_UI_SNAPSHOT_DIR="$PWD/build/native-screenshots" go test -run '^TestNative' .
```

Software-renderer layout snapshots test native view geometry deterministically; real-app interaction, audio device output, live network behavior, and whole-app memory measurements are verified and recorded separately.

---

## License

Original **ytmgo** code, documentation, and project assets are licensed under the [Apache License 2.0](LICENSE); see [NOTICE](NOTICE) for copyright and attribution and [CONTRIBUTING.md](CONTRIBUTING.md) for contribution terms.

Third-party dependencies, adapted code, and generated bundles containing third-party code retain their upstream licenses and copyright notices—including **FFmpeg 9.0.2** under **LGPL-2.1-or-later** and **MPL-2.0** components (such as `option-ext` when selected by the target's Rust dependency graph); see [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md).

During `make build` and `make build-app` (and verifiable via `make check-licenses`), the build system generates a target-specific `legal/` directory inside the packaged application containing upstream notices, `manifest.json`, and checksum-verified corresponding source archives plus rebuild instructions for FFmpeg and MPL-2.0 components.
