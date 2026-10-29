# ytmgo 0.4 — Native Design Specification

ytmgo is a native MyGo desktop client for YouTube Music. Version 0.4 draws
a Spotify-style interface from Go through `ui.View`. Catalogue responses become typed Go models; audio plays
through the system output; video is decoded only when requested. Google sign-in
is the only temporary web surface. The music interface has no HTML, CSS,
JavaScript bridge, embedded music page, or compatibility implementation.

The mental model it serves is simple and constant:

> **Browse anything on the left/centre. What's playing is always at the
> bottom, and one click shows more of it.**

Every decision below exists to protect that model.

---

## 1. Design system foundations

The interface follows Spotify's desktop language: a black window frame, three
rounded near-black panels, a global bar with a centred search pill, and a
three-zone player bar. ytmgo keeps its own accent for playback state and the
primary play actions. The palette and vector icons live in `native_style.go`;
layout and type live in `native_views.go`, exercised by the MyGo
software-renderer tests in `native_views_test.go`.

### 1.1 Color roles

Dark is the primary theme. Light uses the same roles with inverted values.

| Role | Field | Dark | Light | Use |
|---|---|---|---|---|
| Base | `base` | `#000000` | `#e9e9eb` | Window frame, global bar, player bar, tab bar |
| Surface | `surface` | `#121212` | `#ffffff` | Library, page and Now Playing panels |
| Raised | `raised` | `#1f1f1f` | `#f2f2f3` | Search pill, round buttons, cards on hover, panel cards, dialogs |
| Input | `input` | `#242424` | `#ececee` | Secondary pills |
| Highlight | `highlight` | `#2a2a2a` | `#e1e1e4` | Hovered round buttons, artwork placeholders |
| Text | `text` / `secondary` / `muted` | `#fff` / `#b3b3b3` / `#8f8f8f` | `#121212` / `#5a5a5f` / `#6b6b70` | Titles / artists, meta / placeholders |
| Accent | `accent` / `onAccent` | `#ff5a4f` / `#000` | `#e8382d` / `#fff` | Active toggles, playing glyphs, collection play, Premium avatar |
| Accent text | `accentText` | `#ff5a4f` | `#b8261c` | Playing titles, Premium badge, error and selected text (≥4.5:1 on every light surface) |
| Washes | `hover` / `press` / `line` | 7% / 12% / 10% white | 5% / 9% / 10% black | Row and chip states, hairlines |

The primary call to action (sign in, play/pause) is an inverted pill or circle
in `text` on `base`. Accent is never the only carrier of state: active toggles
also get a dot, and the playing row also gets a wash and a speaker glyph.

**Header wash.** Collections blur their cover into the top of the page panel
and fade it into the surface. Pages without artwork use a stable per-route
tint from `nativeHeaderTints`. Immersive mode blurs the current track across
the whole panel. MyGo paints absolute elements above in-flow siblings, so the
page sits in an absolute layer after the wash.

### 1.2 Typography

System stack (SF Pro / PingFang SC on macOS, Noto Sans CJK on Linux,
Microsoft YaHei on Windows).

| Use | Spec |
|---|---|
| Collection title | 800 · 32–72 (5.5% of the page width), 2 lines |
| Page title, immersive title | 800 · 32 (26 compact) / 40 (28 compact) |
| Shelf heading, settings title | 800 · 24 (20 compact) |
| Now Playing title | 800 · 22 |
| Library title, section heading | 700 · 16 |
| Row and card title | 500–700 · 15 |
| Artist, meta, table headings | 400–500 · 13 |
| Player title / artist | 600 · 14 / 12 |

Time readouts occupy fixed-width slots so progress labels do not shift
adjacent controls.

### 1.3 Layout

| Region | Wide (>1180) | Medium (761–1180) | Compact (≤760) |
|---|---|---|---|
| Global bar | 64: brand, back/forward · home + search (Explore inside) · settings, account | same | 60 in-page: back, search, settings |
| Library | 300 panel, collapsible to 72 | 72 rail | Library tab |
| Page panel | rounded 8, inset 24 | same | full bleed, inset 16 |
| Now Playing | 360 docked | 380 floating | fitted sheet |
| Player | 80 three-zone bar | same | 60 mini player card + 64 tab bar |

Panels sit 8 apart on the black frame. Minimum hit targets are 32 on desktop
and 44 in compact windows.

### 1.4 Components

- **Library row:** 64 high, 48 cover (round for artists), title and
  "kind · owner". The open collection is washed and its title accented.
  Filter chips (playlists, albums, artists, songs) appear when the library holds
  at least two kinds; a selected chip hides the others and gains a clear button.
- **Quick-access tiles:** signed-in Home leads with up to eight library items in
  a 4- or 2-column grid of 64-high tiles.
- **Card:** 164 cover (140 compact), title and two-line subtitle; playable
  cards raise an accent play button on hover.
- **Track row:** 56 high, 40 cover. Collections number their rows under a
  `#  Title  Album  ⏱` heading; the number becomes a play glyph on hover and a
  speaker while playing.
- **Collection header:** 232 cover (192 medium) beside the display title, then
  a 56 accent play button that starts the first track or toggles the page's
  own playback.
- **Chips and pills:** chips are 32-high translucent capsules, inverted when
  selected; pills are 36-high buttons with the primary form inverted.

### 1.5 Radii and elevation

`4` thumbnails and table covers · `6` rows, cards' covers, tiles · `8` panels,
cards, Now Playing art · `12` dialogs · capsule for pills, chips, search and
round buttons. Shadows are used only on covers, the collection play button,
dialogs and the floating Now Playing panel.

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

- Account restoration: while a saved session is read and verified, the top
  bar, library and Settings say the account is being restored and do not offer
  sign-in; a track selected meanwhile explains that it waits for the account.
  After 12 s the wait is explained (the system may be asking for access to the
  saved key) with "Continue without account". A failed or skipped restore keeps
  the saved session, plays anonymously and offers "Restore account" beside
  sign-in; polling never rereads the store in that state.
- Account: three states. Signed out shows an inverted "Sign in" pill and
  sign-in prompts in the library panel and on the Library route. Signed in
  shows a neutral avatar; Music Premium shows an accent avatar and a "Premium"
  badge on wide windows. The avatar's menu states the plan and offers account
  management, settings and sign-out; Settings repeats the plan. The view only
  reads `SignedIn` and `MusicPremium`; detection and transitions stay in
  `account.go`.
- Global bar: back/forward history, Home, the persistent search field and
  Explore. Search submits on Enter; ⌘K focuses it from anywhere.
- Search: results lead their subtitle with their type (song, video, artist,
  album, playlist, podcast, episode); artist rows use round artwork. A top-result
  card is headed by its section header, never by the result's own title.
- Catalogue: song shelves scroll horizontally in four-row columns, other
  shelves as cards, with service ordering. Album and playlist pages open in
  place with a numbered table. Continuations append to the current page;
  failures offer an explicit retry.
- Transport: the player bar's artwork/title opens Now Playing. Play/pause,
  previous/next, shuffle, repeat, seek and volume operate the same player in
  every presentation. Queue and lyrics buttons open Now Playing on that tab and
  close it when it is already showing. Muting remembers the previous volume.
- Now Playing: cover or video, title and like, then a card with Queue and
  Lyrics chips. Floating and compact forms add their own transport.
- Immersive: prominent artwork or contained video, title/artist, the queue and
  lyrics card, and an explicit exit. Navigation exits immersive mode first.
- Video: an explicit cover/video choice. Frames preserve source aspect ratio;
  there is no crop. Landscape video keeps the cover's height budget and uses
  the extra width, in compact and desktop immersive alike. Closing the panel or immersive surface stops video decoding.
  Audio continues independently. A failed video load shows an error.
- Settings: account status and actions, system/dark/light appearance and
  system/English/Japanese/Simplified Chinese/Traditional Chinese. Changes
  redraw the current native window and preserve playback.
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
extension. It does not select or require a browser vendor. A single 32-byte key belongs to
this app in the system secret store. The entire session is authenticated-encrypted
with AES-256-GCM in `account/session.enc` under the app data directory, using
atomic replacement, a private directory and an owner-only file. Missing ciphertext
returns signed out without touching the secret store. Signing out removes the
ciphertext before deleting the key. Old chunked sessions are not read or migrated;
those users reconnect their account. No WebKit store or legacy fallback exists. Session data
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
