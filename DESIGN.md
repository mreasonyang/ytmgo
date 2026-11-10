# ytmGO 0.4 — Native Design Specification

ytmGO is a native MyGo desktop client for YouTube Music. Version 0.4 draws
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

The visual design follows the **design guidebook** in
[`design/README.md`](design/README.md): colour roles and what coral means,
typography, the radius and spacing scales, motion, interaction and
components. Its values live in [`design/tokens.json`](design/tokens.json);
`go generate` turns them into `native_tokens_gen.go`, and
`TestDesignTokensAreGenerated` fails if that file is stale. Change a value
in the tokens, never in code, and read the guidebook before changing how
something looks or behaves. This section keeps only what is specific to the
native implementation.

The structure follows Spotify's desktop language: a black window frame,
three rounded near-black panels, a global bar with a centred search pill, a
library panel and a three-zone player bar. The palette, icons and theme live
in `native_style.go`, display-title styling in `native_fonts.go` (system font only), layout in
`native_views.go`, exercised by the MyGo software-renderer tests.

### 1.1 Native notes

- **Coral is play.** Apply `p.accent` / `p.accentText` only where something
  plays or starts playing. Playback modes use `nativeModeButton`; every
  other toggle uses `nativeIconButton`, which draws an active state in
  `text` with a dot. MyGo's theme accent is the inverted text colour, so
  built-in selected states invert.
- **Errors** use `p.attention` beside the `alert` icon and words.
- **What's playing lends its colour** (guidebook §4.4). `artworkTint`
  (artwork_tint.go) reads a cover's colour once, when the artwork cache
  decodes it; `ViewActions.ArtworkTint` hands it to the views. The Now
  Playing panel, the player bar and the mini player paint it as plain
  gradients or a mixed fill, so following the music costs nothing per frame.
  Only page backdrops blur a cover.
- **Header wash.** Collections blur their cover into the top of the page
  panel and fade it into the surface. Pages without artwork use a stable
  per-route tint from `nativeHeaderTints` (the tokens' order matters to the
  hash). Immersive mode blurs the current track across the whole panel.
  MyGo paints absolute elements above in-flow siblings, so the page sits in
  an absolute layer after the wash.
- **Display titles** (page, collection, immersive) go through
  `nativeDisplay`; measure them with `nativeDisplayMetrics`, so a layout
  budget measures the face that is drawn.
- **Feedback.** Buttons call `nativePress` (96 % while pressed); round play
  controls call `nativePlayFeedback` (105 % on hover). Rows and cards use the
  `hover` and `press` washes.
- **Layout regions** (global bar 64, library 300 or a 72 rail, panels 8
  apart, Now Playing 360 docked or 380 floating, player 80, compact mini
  player 60 and tab bar 64, targets 32 and 44 compact) come from the
  tokens' `product` measures.
- **No literal styling.** `TestNativeViewsUseDesignTokens` parses the view
  code and fails on a literal colour or a hand-made share of one (`Alpha`,
  `Mix`; translucent colours are roles), corner, type size or shadow, or on
  any literal spatial value off the 4 px grid, in every file of the package.
  Corners are radius tokens (8, 12, 16, 24; `TestRadiusScale`) or half a
  size, including those passed to the artwork helpers, and nest smaller
  inside (guidebook §7): the queue's rows are flat bands its card clips.
  Tooltips go through `nativeTip`, since MyGo's own draws a 5 px corner. Values that are
  geometry rather than design (a slider inset equal to its thumb's radius)
  are named constants derived from what they depend on.
- **Shadows** are the tokens' elevations: `Shadow(tokenShadowArt.args())`.
  Dialogs dim the window with `p.scrim`. Video plays on a black stage in
  both appearances, so the stage uses the dark palette.
- **Scroll bars** are `nativeScrollbar`'s (guidebook §10): the theme hides
  MyGo's, which show whenever the pointer is over a scroller, and leaves
  them to be grabbed. `TestScrollersDrawTheirBars` fails on a scroller
  without one.
- **Clips:** MyGo's GPU renderer rounds only the innermost clip (the
  software renderer the tests use rounds all of them), so content flush with
  a rounded container takes its corners itself rather than relying on the
  container's `Clip`.
- **Time readouts** occupy fixed-width slots, so progress labels don't shift
  adjacent controls.
- **CJK headings** keep their phrases whole: mark phrase boundaries in a
  heading's catalog message with `|` (`"你的|音乐库"`); `a.tr` turns the marks
  into word joiners (`phrases`). Mark new or reworded CJK headings; leave
  running text unmarked.
- **Scripts.** English, Simplified Chinese, Traditional Chinese and Japanese
  use MyGo's native shaping and locale selection; labels, menus and
  accessible names come from the four-language catalog. Layout tests cover
  long text and CJK content at narrow widths.
- **Icons:** one 24 px grid, 1.8 px round strokes; filled variants mark the
  active destination and the primary transport glyphs. Icon-only buttons
  carry accessible names and tooltips.

---

## 2. Native screen and interaction contract

- Account restoration: while a saved session is read and verified, the top
  bar, library and Settings say the account is being restored and do not offer
  sign-in; a track selected meanwhile explains that it waits for the account.
  On macOS automatic restoration reads without system authorization UI; an
  explicit restore or sign-in may request access to the existing key. After 12 s a remaining verification wait is
  explained with "Continue without account". A failed or skipped restore keeps
  the saved session, plays anonymously and offers "Restore account" beside
  sign-in; polling never rereads the store in that state. Settings also offers
  explicit clearing of unrestorable account data through the sign-out action.
  A newly verified sign-in reuses the persistent key, generates a fresh GCM nonce
  and atomically replaces the encrypted session without reading the old file.
  Failed commits preserve the old file and the key. Only a missing key permits
  creation; locked, denied or malformed keys fail without replacement. A key
  created before a failed first file commit stays available for retry. Only one
  encrypted session format is read and written; no migration or fallback exists.
- Account: three states. Signed out shows an inverted "Sign in" pill and
  sign-in prompts in the library panel and on the Library route. Signed in
  shows a muted avatar; Music Premium shows an inverted avatar and a "Premium"
  badge on wide windows. The avatar's menu states the plan and offers account
  management, settings and sign-out; Settings repeats the plan. The view only
  reads `SignedIn` and `MusicPremium`; detection and transitions stay in
  `account.go`.
- Global bar: back/forward history, Home, the persistent search field and
  Explore. Search submits on Enter; ⌘K focuses it from anywhere.
- Window header (Linux, Windows): the main window hides the system title bar
  (`mainWindowTitleBar`), so the global bar, the compact bar or the immersive
  header is the title bar. It drags the window, keeps clear of the window
  controls (`c.TitleBar()`) and leads with a ⋯ application menu holding the
  menu bar's menus; Alt and F10 still open those, and their shortcuts keep
  working. The window controls take the bar's 64: GTK centres its buttons in
  it and Windows' caption buttons fill it, so their glyphs line up with the
  bar's. Narrow compact windows move the search field to a row below the
  controls. macOS keeps its title bar and the app's menu bar.
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
- Settings: one sheet (640 wide; full-window at 760 and below, title 20) of
  titled cards — Account, General, Keyboard, About. Each row has an icon
  tile, a name and a one-line explanation, with its control on the right;
  below 520 DIPs of content the control spans the row under the text. A
  hairline under the title appears once content scrolls beneath it and a
  fade marks more below; sheets open at the top. Account states the plan
  (the status names Premium; no repeated badge) and offers sign-in/manage or
  the restore actions, under the text when a restore notice shows; sign-out
  is a separate destructive row, inset so its focus ring shows. Appearance
  is a segmented radio group (one Tab stop, arrows choose, single-line
  labels); language is a drop-down listing each language in itself. The
  global shortcut field shows key caps in platform order (⌃ ⌥ ⇧ ⌘ on macOS)
  and records in place when clicked; recording ends on a chord, Escape,
  Cancel or when focus leaves it. Off, the row offers Record shortcut by
  name. "View all" opens Keyboard shortcuts, which shares the sheet, spells
  chords for the platform, and whose Configure returns with the Keyboard card
  in view. Changes redraw the current native window and preserve playback.
- Menus, tray, global shortcut and OS Now Playing send commands to the same Go
  controller. Closing the main window on macOS hides it; quitting releases
  the native player, decoder and resolver helpers.

## 3. Runtime ownership

`main.go` owns the native window and app lifecycle; `controller.go` owns
navigation, queue and asynchronous request generations; `native_views.go`
draws the UI. `internal/catalog` handles typed Innertube calls.
`internal/player` owns bounded PCM/video decoding and native audio output.
`internal/streaming` owns protocol bootstrap, account headers, format selection,
proof lifetimes and direct HTTPS ranges in Go. A small native host runs the
extracted signature code in QuickJS; anonymous/free proof uses a hidden,
nonpersistent system WebView. Neither script environment receives account
cookies. Initialization is lazy and waits for an explicitly selected account mode. One next-track decoder is prepared while the current track
plays; a matching selection transfers ownership without resolving again.
Queue changes and canceled requests revoke obsolete media capabilities.
FFmpeg only reads one Go loopback relay; remote URLs and headers stay in Go.
`artwork.go` bounds the decoded image cache and download
concurrency. Settings remain small files in the application data directory.

External links use the system default browser through MyGo Shell. Account
authorization uses a separate, temporary native WebView login process. The helper
returns the selected YouTube session through private pipes; the app verifies it
with the service before saving. There is no browser-extension or HTTP handoff.
A single random 32-byte AES key belongs to this app in the system secret store
under the fixed name `account-session-key`; Cookies are never stored there.
The session file contains the authenticated `YTMSESS\x03` header, a fresh 12-byte
nonce, ciphertext and a 16-byte GCM tag, with no key ID. It is stored at
`account/session.enc` under the app data directory using atomic replacement, a
private directory and an owner-only file. Missing ciphertext returns signed out
without touching the secret store. Signing out removes the ciphertext before
deleting the key, and retries key deletion even when the file is already absent
or damaged. Confirmed expiry or server sign-out also clears the saved account;
network and authorization errors retain it. Session data must never enter logs,
argv, environment variables or evidence files.

The key lifecycle follows Chromium's
[KeychainPassword implementation](https://github.com/chromium/chromium/blob/main/components/os_crypt/common/keychain_password_mac.mm):
read one stable item and create it only on an explicit not-found result. As in
[Electron safeStorage](https://www.electronjs.org/docs/latest/api/safe-storage),
the OS protects the encryption key while application data stays encrypted on disk;
temporary key-store unavailability is an error, not a reason to replace the key.
The app's single-instance lock and serial account worker protect key/file writes.
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
