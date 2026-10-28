# ytmgo 0.4 — Native Design Specification

ytmgo is a native MyGo desktop client for YouTube Music. Version 0.4 keeps
the 0.3 visual system and navigation model, while drawing the interface from
Go through `ui.View`. Catalogue responses become typed Go models; audio plays
through the system output; video is decoded only when requested. Google sign-in
is the only temporary web surface. The music interface has no HTML, CSS,
JavaScript bridge, embedded music page, or compatibility implementation.

The mental model it serves is simple and constant:

> **Browse anything on the left/centre. What's playing is always at the
> bottom, and one click shows more of it.**

Every decision below exists to protect that model.

---

## 1. Design system foundations

The palette and vector icons live in `native_style.go`. The layout and type
scale live in `native_views.go`, with responsive behavior exercised by the
native MyGo software-renderer tests.

### 1.1 Color hierarchy

Dark is the primary theme. Light uses the same roles with inverted values.

| Role | Token | Dark | Light | Use |
|---|---|---|---|---|
| Base | `--y-base` | `#050506` | `#e7e7ea` | Window gutter, player bar |
| Surface 1 | `--y-surface-1` | `#121214` | `#ffffff` | Panels: sidebar cards, canvas, now playing |
| Surface 2 | `--y-surface-2` | `#1b1b1e` | `#f4f4f6` | Raised: hovered cards, empty states, segmented track |
| Surface 3 | `--y-surface-3` | `#242428` | `#ebebee` | Inputs, chips, mini player |
| Surface 4 | `--y-surface-4` | `#2f2f34` | `#e1e1e5` | Highlight: selected segment, tooltips, chip hover |
| Glass | `--y-glass` / `-strong` | 72% / 88% surface | same | Scrolled top bar, popovers, dialogs, immersive side panel |
| Text 1 | `--y-text-1` | `#f5f5f7` | `#121214` | Titles, primary labels |
| Text 2 | `--y-text-2` | `#a8a8af` | `#55555c` | Secondary labels, artists, idle icons |
| Text 3 | `--y-text-3` | `#84848c` | `#6b6b72` | Meta, timestamps, eyebrows |
| Accent | `--y-accent` | `#ff5a4f` | `#e8382d` | Active toggles, playing row, progress on hover, primary play on cards |
| On accent | `--y-on-accent` | `#0b0b0c` | `#ffffff` | Glyphs on accent |
| Lines | `--y-line` / `-strong` | 7% / 14% white | 7% / 13% black | Hairlines, input borders |
| States | `--y-hover` / `--y-press` / `--y-selected` | 6% / 10% / 12% | 4.5% / 8% / 8% | Overlay washes on any surface |

Contrast: Text 1 and Text 2 exceed WCAG AA (4.5:1) on Surfaces 1–3 in both
themes; Text 3 is reserved for non-essential meta at ≥ 4.5:1 on Surface 1.
Accent is never the sole carrier of state: active toggles also get a dot,
and the playing row also gets a tinted background plus the service's
equalizer glyph.

**Dynamic color.** Page or current-track artwork is rendered as a blurred
native image behind the content. Immersive mode expands the same backdrop.
The native palette supplies the overlay and keeps text readable.

### 1.2 Typography

System stack (SF Pro / PingFang SC on macOS, Noto Sans CJK on Linux,
Microsoft YaHei on Windows). Display sizes use the Display optical size.

| Token | Spec | Use |
|---|---|---|
| `--y-type-display` | 800 · clamp(32–56px) / 1.04 · −0.035em | Album/artist hero, immersive title |
| `--y-type-title-1` | 760 · 32 / 1.12 | Greeting, compact immersive title, dialogs' parent |
| `--y-type-title-2` | 720 · 24 / 1.2 | Shelf headings, now-playing title, dialog titles |
| `--y-type-title-3` | 680 · 18 / 1.3 | Empty-state titles, promo titles |
| `--y-type-body` | 500 · 15 / 1.45 | Running text, search input |
| `--y-type-label` | 600 · 14 / 1.3 | Navigation, buttons, row titles, menu items |
| `--y-type-caption` | 500 · 13 / 1.4 | Artists, meta, descriptions |
| `--y-type-micro` | 700 · 11 / 1.3 · +0.08em · uppercase | Eyebrows ("正在播放", section straplines) |

Time readouts occupy fixed-width slots so progress labels do not shift
adjacent controls.

### 1.3 Spacing and grid

4 px base: `--y-s-1` 4 · `-2` 8 · `-3` 12 · `-4` 16 · `-5` 20 · `-6` 24 ·
`-8` 32 · `-10` 40 · `-12` 48 · `-16` 64.

Layout geometry (also tokens, switched by breakpoint):

| Token | Wide (>1180) | Medium (761–1180) | Compact (≤760) |
|---|---|---|---|
| `--y-gap` (panel gutter) | 8 | 8 | 0 |
| `--y-sidebar-w` | 280 (72 collapsed) | 72 rail | 0 |
| `--y-panel-w` | 360, docked | 380, overlay | 100vw sheet |
| `--y-topbar-h` | 64 | 64 | 56 |
| `--y-playerbar-h` | 84 | 84 | 64 (mini) |
| `--y-tabbar-h` | 0 | 0 | 64 |
| `--y-hit` (min target) | 32 | 32 | 44 |

Canvas content is inset 32 px (16 px compact) so shelves, headers and the
greeting share one left edge.

### 1.4 Radii

`xs 4` list thumbnails · `sm 6` card art, tooltips · `md 8` cards, rows,
menu items · `lg 12` panels, now-playing art · `xl 16` popovers, dialogs ·
`pill` buttons, chips, search, segmented controls.

### 1.5 Elevation

| Level | Surface | Shadow | Examples |
|---|---|---|---|
| e0 | Base | none | Gutter, player bar |
| e1 | Surface 1 | none (separation by gutter) | Sidebar, canvas, docked panel |
| e2 | Surface 2 | `0 8 24 / 32%` | Hovered card, card art, mini art, tooltips |
| e3 | Glass strong + blur 28 + 1 px rim | `0 16 48 / 48%` | Popovers, service menus, toast, mini player |
| e4 | Glass / art | `0 24 80 / 60%` | Dialogs, overlay panel, immersive art |

Native view ordering: canvas, navigation, now-playing panel, immersive surface,
player controls, then settings and shortcut overlays. Menus are native controls.

### 1.6 Native feedback

Native controls provide focus, hover and pressed feedback. Page, panel and
appearance changes update the current view tree without destroying the window.
Audio position changes update the fixed-width transport. Video frames redraw
only while a visible video presentation is active.

### 1.7 Typography across scripts

English, Simplified Chinese, Traditional Chinese and Japanese use MyGo's
native text shaping and locale selection. Labels, menus and accessible names
come from the same four-language catalog. Text input remains a native editing
control; layout tests cover long text and CJK content at narrow widths.

### 1.8 Iconography

One 24 px grid, 1.8 px round strokes (`native_style.go`). Filled
variants mark the active destination (home/explore/library) and the
primary transport glyphs. Icon-only buttons carry native accessible names
and tooltip text.

---

## 2. Native screen and interaction contract

- Sidebar: 280 pt above 1180 pt, 72 pt rail below that breakpoint; Home,
  Explore and Library retain the same order and icons. At 760 pt and below,
  navigation moves to a fixed bottom tab bar and minimum targets become 44 pt.
- Top bar: back/forward navigation, persistent search field and settings.
  Search submits on Enter; text editing retains its normal keyboard behavior.
- Catalogue: four-song columns and album/playlist cover shelves retain service
  ordering, with horizontal scrolling. Album and playlist pages open in place.
  Continuations append to the current page; failures offer an explicit retry.
- Transport: fixed 84 pt desktop bar and 64 pt compact bar. Artwork/title opens
  Now Playing. Play/pause, previous/next, shuffle, repeat, seek and volume operate
  the same player in every presentation. Muting remembers the previous volume.
- Now Playing: 360 pt docked panel on wide windows, 380 pt overlay at medium
  widths, fitted sheet in compact windows. Queue and lyrics share one tab strip.
- Immersive: prominent artwork or contained video, title/artist, controls,
  queue/lyrics and an explicit exit. Navigation exits immersive mode first.
- Video: an explicit cover/video choice. Frames preserve source aspect ratio;
  there is no crop. Closing the panel or immersive surface stops video decoding.
  Audio continues independently. A failed video load shows an error.
- Settings: system/dark/light appearance and system/English/Japanese/Simplified
  Chinese/Traditional Chinese. Changes redraw the current native window and
  preserve playback. Explicit language choice remains distinct from system.
- Menus, tray, global shortcut and OS Now Playing send commands to the same Go
  controller. Closing the main window on macOS hides it; quitting releases
  the native player, decoder and resolver helpers.

## 3. Runtime ownership

`main.go` owns the native window and app lifecycle; `controller.go` owns
navigation, queue and asynchronous request generations; `native_views.go`
draws the UI. `internal/catalog` handles typed Innertube calls.
`internal/player` owns a persistent YouTube.js worker, bounded PCM/video
decoding and native audio output. The worker waits for an explicitly selected account mode; SDK and proof
initialization happen only when playback needs them. One next-track decoder is prepared while the current track
plays; a matching selection transfers ownership without resolving again.
Queue changes and canceled requests revoke obsolete media capabilities.
FFmpeg only reads a local relay; remote URLs and headers stay inside Node.
`artwork.go` bounds the decoded image cache and download
concurrency. Settings remain small files in the application data directory.

Acceptance requires no WebKit anywhere in the application, including sign-in.
External links use the system default browser through MyGo Shell. Account
authorization uses a user-clicked, short-lived loopback extension handoff in a
Chromium-compatible browser. The current extension does not support Safari or
Firefox; the guide lets users copy the Music URL to the browser containing the
extension. It does not select or require a browser vendor. Sessions belong to this app in the
system secret store; no WebKit store or legacy migration exists. Session data
must never enter logs, argv, environment variables or evidence files.
The service supplies logged-in and Music Premium booleans. A five-minute poll
and foreground refresh update them only after a valid response. Premium engines
must not import DOM/proof modules. Other modes create the proof engine only on
demand. Mode changes reap the previous engine; only an identity change revokes
current streams. Missing fields and failed refreshes never mean false.
Anonymous content playback does not prove Premium or normal advertisement
support. Advertisement media and actual progress events require a separate
verified protocol implementation; simulated waits are not allowed.
The package includes the worker, JavaScript runtime and decoder with pinned
versions and checksums. There is no yt-dlp or alternate-client fallback.

The historical 0.3 audit and images in `docs/audits/2026-10-27-uiux/` remain
visual reference and provenance; they do not prove native 0.4 acceptance.
