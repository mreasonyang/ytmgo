# ytmgo 0.3 — Design Specification

ytmgo is a desktop client for YouTube Music. The 0.3 shell is a ground-up
rewrite: ytmgo owns every piece of chrome (navigation, top bar, transport,
now-playing, settings), and the service supplies only what is genuinely
its own: the catalogue canvas, the queue, lyrics, and the player itself.

The mental model it serves is simple and constant:

> **Browse anything on the left/centre. What's playing is always at the
> bottom, and one click shows more of it.**

Every decision below exists to protect that model.

---

## 1. Design system foundations

All values live in `web/shell/tokens.css`. No other file hard-codes a
color, radius, duration or z-index.

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

**Dynamic color.** Instead of extracting a palette from the artwork (which
would need cross-origin pixel reads), the current page's header art, or
else the now-playing art, is blurred 90 px and masked into the top of the
canvas (`.y-canvas-wash`). It fades with scroll. Immersive mode uses the
same technique full-bleed (`.y-np-backdrop`).

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

Time readouts use `font-variant-numeric: tabular-nums` so the progress
labels never jitter.

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

z-index ladder: canvas 1 · chrome 20 · panel 30 · immersive 40 · player
bar 50 · overlays 60. Service popups (≈100–2200) intentionally stay above
the shell so their menus and dialogs always work.

### 1.6 Motion

| Token | Value | Use |
|---|---|---|
| `--y-d-instant` | 90 ms | Press-down |
| `--y-d-fast` | 160 ms | Hover color, exits, tooltips |
| `--y-d-base` | 240 ms | State changes, entrances |
| `--y-d-slow` | 380 ms | Panel slide, card play-button rise, track change |
| `--y-d-xslow` | 520 ms | Immersive morph, backdrop fade |
| `--y-ease-out` | `cubic-bezier(.2,.8,.2,1)` | Default decelerate |
| `--y-ease-emphasized` | `cubic-bezier(.2,0,0,1)` | Layout transitions |
| `--y-ease-spring` | `linear()` spring, single 16% overshoot (cubic fallback) | Press release, pops, entrances |

Rules: only `transform` and `opacity` animate per-frame (the progress fill
is `scaleX`, the knob is `translate`). Exits are faster than entrances. The
canvas never animates its width (that would re-flow the service's DOM every
frame); the docked panel slides in over an instant re-flow instead. Under
`prefers-reduced-motion` every duration collapses to 1 ms and all keyframe
and view-transition animations are disabled.

### 1.7 Typography across scripts

The type tokens are composites of per-script variables, switched by
`:root[data-ytmgo-script]` (`latn`, `hans`, `hant`, `jpan`, set from the
interface language):

| Variable | Latin | CJK | Why |
|---|---|---|---|
| `--y-font-han` | SC families | SC / TC / JP families | Shared Han code points (骨, 直, 角…) have regional glyph forms; each script names its own fonts first (PingFang SC/TC, Hiragino Sans, Noto Sans CJK SC/TC/JP, Microsoft YaHei/JhengHei, Yu Gothic) |
| `--y-lh-body` / `-caption` / `-label` | 1.45 / 1.4 / 1.3 | 1.7 / 1.6 / 1.45 | Ideographs are full-height; they need more leading to stay legible |
| `--y-lh-display` / `-title` / `-heading` | 1.04 / 1.15 / 1.3 | 1.2 / 1.3 / 1.4 | |
| `--y-track-display` / `-tight` | −0.035 / −0.025em | 0 | Negative tracking collides ideographs |
| `--y-w-display` | 800 | 700 | CJK system fonts top out near semibold; avoids smeared faux bold |
| `--y-size-micro` / `--y-track-micro` | 11px / 0.08em | 12px / 0.04em | Dense glyphs need the extra pixel |

Text rules for CJK interfaces: `line-break: strict` (kinsoku — no line
starts with 。」、), `text-autospace` (space between CJK and Latin runs),
`text-spacing-trim: space-first` (trim full-width punctuation at line
starts), and `font-feature-settings: "palt"` on headings (proportional
punctuation, so 「」、 don't leave holes in large type). Headings and short
bodies use `text-wrap: balance`. Every shell root carries `lang`, so
screen readers pronounce it correctly and the engine picks regional glyphs
even for fallback fonts.

### 1.8 Iconography

One 24 px grid, 1.8 px round strokes (`web/shell/icons.js`). Filled
variants mark the active destination (home/explore/library) and the
primary transport glyphs. Icon-only buttons always carry `aria-label`,
which doubles as the delayed tooltip text.

---

## 2. Component architecture

```
web/shell/
  bundle.json     manifest: style + script order (single source of truth)
  tokens.css      design tokens + breakpoints + reduced motion
  base.css        primitives: buttons, icon buttons, tooltips, focus, keyframes
  shell.css       geometry; sidebar; top bar; canvas; compact tab bar; service chrome
  playerbar.css   transport, slider, mini player, compact full transport
  nowplaying.css  panel + immersive frame; service player page laid into it
  overlays.css    popover, dialog, toast, service menus/promos
  canvas.css      the service's content re-skinned onto tokens
  core.js         guard, h(), DOM helpers, announce(), pulse()
  i18n.js         t(), shortcut(), Intl list/percent formats, greeting, localized()
  icons.js        icon set, iconButton(), swapIcon()
  store.js        observable state; isolated watchers
  bridge.js       ONLY module that knows the service DOM: reads state, performs intents
  slider.js       shared scrubber (progress, volume)
  sidebar.js      navigation + playlists; compact tab bar
  topbar.js       history, settings; hosts the service search/account
  playerbar.js    transport
  nowplaying.js   panel / immersive / compact sheet
  overlays.js     settings popover, shortcuts dialog, toast
  keyboard.js     in-page shortcuts, Escape layering
  main.js         boot, appearance, action vocabulary, sync loop, scroll memory
```

`music.go` (and its Node mirror `bundle.mjs`) concatenates the scripts into
one closure, `(function ytmgoShell(STYLES, APPEARANCE, I18N) { … })`, injected at
document start into the top-level music document only.

**Data flow.** One direction:

```
service DOM ──(MutationObserver, video events, rAF-throttled)──▶ bridge.read*() ──▶ store.set()
                                                                                       │
UI modules ◀──────────────── store.watch(keys) ◀───────────────────────────────────────┘
UI intent (click/key/menu) ──▶ runAction()/bridge.* ──▶ service control, player API or SPA event
```

UI modules never query the service; the bridge never renders. This keeps
the inevitable service-markup changes contained to `bridge.js`,
`canvas.css` and the player-page section of `nowplaying.css`.

**Native boundary (unchanged security model).** The page has no IPC
bridge. It can only:
- report a display boolean: `ytmgo://account/signed-in|signed-out`;
- request one of three appearances: `ytmgo://appearance/{dark,light,system}`;
- request an interface language: `ytmgo://language/{system,en,ja,zh-Hans,zh-Hant}`;
- open the native sign-out confirmation with a real user click on
  `ytmgo://session/signout`.

Go drives the page through one function, `window.ytmgoControls(action)`,
over a fixed vocabulary: `toggle next previous mute volume-up volume-down
like search home explore library back forward account panel queue lyrics
immersive sidebar shortcuts`. Toggles arriving twice within 250 ms (native
menu accelerator + in-page key) run once.

---

## 3. Screen layouts

### 3.1 Wide (> 1180 px) — three panels on a base

```
┌─ base ──────────────────────────────────────────────────────────────────┐
│ ┌ Sidebar ─┐ ┌ Canvas ─────────────────────────────┐ ┌ Now playing ──┐ │
│ │ ● ytmgo ◧│ │ ‹ ›  [ 想听什么？        ]   登录  ⚙ │ │ 正在播放  ⤢ ✕ │ │
│ │ ⌂ 首页   │ │                                     │ │ ┌───────────┐ │ │
│ │ ◎ 探索   │ │ 晚上好                               │ │ │  artwork  │ │ │
│ │ ⫼ 音乐库 │ │ (chips)                              │ │ └───────────┘ │ │
│ └──────────┘ │ Shelf ───────────────────── ‹ ›     │ │ Title      ♡⋯ │ │
│ ┌──────────┐ │ ▢ ▢ ▢ ▢ ▢                           │ │ Artist        │ │
│ │ 你的音乐库│ │ Shelf ...                           │ │ [Up next|Lyr] │ │
│ │ playlists│ │                                     │ │ queue …       │ │
│ └──────────┘ └─────────────────────────────────────┘ └───────────────┘ │
│ [art] Title ♡      ⇄  ⏮  (▶)  ⏭  ↻                 🎤 ☰ 🔊━━ ⋯ ⤢     │
│       Artist       0:42 ━━━━━━━━●──────── 3:58                          │
└─────────────────────────────────────────────────────────────────────────┘
```

- **Sidebar**: two stacked cards. Top: brand (doubles as Home) + collapse
  toggle + three destinations. Bottom: the account's playlists, mirrored
  from the service guide with deterministic gradient monograms; signed out,
  an inline "建立你的音乐库" card with a sign-in button replaces the list.
- **Canvas**: the service content column, made a rounded scroll container.
  The top bar floats over it and turns to glass once content scrolls under
  it. The artwork wash tints its top.
- **Now playing panel**: docks on first playback. It is the service player
  page, laid out vertically: artwork (square for songs, 16:9 for music
  videos and ads), ytmgo's title/artists/like/⋯ row, then Up next / Lyrics /
  Related as a segmented control.
- **Player bar**: on the base, full width, three zones.

### 3.2 Medium (761–1180 px)

Sidebar becomes a 72 px icon rail with right-side tooltips. The panel no
longer docks: it floats over the canvas with a scrim (Esc or scrim click
closes it). Crossing below 1181 px closes an open panel; crossing back
re-docks it.

### 3.3 Compact (≤ 760 px) — thumb zone

```
┌──────────────────────────┐
│ ‹ [ search ]   登录   ⚙  │
│ 晚上好                    │
│ canvas …                 │
│                          │
│ ┌──────────────────────┐ │  ← mini player (art · title · ♡ · ▶/❚❚),
│ │▣ Title / Artist  ♡ ❚❚│ │     2 px progress hairline on its edge
│ └──────────────────────┘ │
│  ⌂     ⌕     ◎     ⫼     │  ← tab bar: Home · Search · Explore · Library
└──────────────────────────┘
```

Tapping the mini player opens the compact immersive sheet: artwork, title,
artists, like/⋯, and the mini player unfolds into the full transport
(scrubber with times, ⇄ ⏮ ▶ ⏭ ↻ on a 64 px primary button). All targets
are ≥ 44 px and sit in the bottom third of the screen. The window minimum
is now 380 × 640 so this layout is reachable on desktop.

### 3.4 Immersive

The same three layers expand over the whole window above the player bar:
blurred, slowly drifting artwork backdrop; artwork left at the largest size
that fits; display-size title and artists centered beneath it; queue/lyrics
on a frosted column on the right. The artwork and title morph between
docked and immersive positions via View Transitions where the engine
supports them, and cross-fade otherwise.

### 3.5 Page headers (album / playlist)

The service's two-column album layout becomes Spotify-style: square art
(160–232 px) on the left, strapline, display-size title, meta, description
and actions stacked on the right, bottom-aligned; the track list runs full
width beneath.

---

## 4. Internationalization

Languages: **English** (source), **简体中文**, **繁體中文** (Taiwan usage, not
character-converted), **日本語**.

### 4.1 One source of truth

```
locales/en.json  ja.json  zh-Hans.json  zh-Hant.json     ARB + ICU MessageFormat
        │
        ├─ mygo generate ─▶ locales/catalog_gen.go        typed Go accessors (menus, dialogs, windows, sign-out page)
        └─ i18n.go newShellI18N() ─▶ I18N argument of the injected shell
                                      {locale, script, choice, languages[{tag, autonym}], messages{shell*}}
```

- Native strings: `locales.MenuPlayback()`, `locales.AboutMessage(version)`, …
- Shell strings: keys prefixed `shell`. Go formats them in the shown
  language with each argument left as a literal `{name}` slot; the shell's
  `t(key, params)` fills the slots. Shell messages may only use plain
  arguments (no plural/select) — enforced by a Go test.
- Key caps are tokens, not text: `shortcut(['mod', 'shift', 'space'])`
  renders `⌘ ⇧ Space` on macOS and `Ctrl Shift 空格` on Linux/Windows in
  Simplified Chinese.
- Lists and percentages use `Intl.ListFormat` / `Intl.NumberFormat` in the
  shown language ("A, B, and C" / "A、B和C" / "A、B、C").
- Language names are CLDR autonyms (`i18n.Autonym`), each rendered with its
  own `lang` so 繁體中文 and 日本語 use their own glyphs inside a
  Simplified Chinese menu.

### 4.2 Choosing the language

`Match system` (default) lets MyGo match the OS languages against the
catalogs by CLDR distance (zh-TW, zh-HK → 繁體中文; zh-CN, zh-SG → 简体中文;
anything unmatched → English). An explicit choice is offered in two places:
**Settings → Language** in the shell, and **View → Language** in the native
menu. The choice is stored in the `language` file beside `appearance` and
applied with `App.SetLocale`.

On change: native menus, tray menu and window titles are rebuilt, and the
music view reloads, so that the shell *and* the service's catalogue canvas
(loaded with the matching `hl`: en / ja / zh-CN / zh-TW) switch together.
The settings popover says so before you choose.

The page can request only `ytmgo://language/{system,en,ja,zh-Hans,zh-Hant}`
through the same navigation guard as appearance.

### 4.3 Text expansion

The same UI ranges from compact CJK to English that is often 1.5–2× wider
("播放面板" → "Now Playing panel"). The layout absorbs it:

- One-line slots (navigation, rows, player title, tab bar, panel heading)
  have `min-width: 0` and ellipsize; their full text is the tooltip or
  accessible name.
- Free-flowing slots wrap: menu items, buttons (`min-height`, not
  `height`), the empty states, the toast, tooltips (max 260px), and
  appearance segments (two centred, balanced lines rather than truncation).
- Containers size to content within bounds: the settings popover is
  `max-content` between 280 and 360px and scrolls past the viewport height;
  the shortcut sheet's columns are `auto-fit, minmax(220px, 1fr)`.
- Nothing is sized to a string. No CSS selects on visible text or
  `aria-label` values.
- A pseudo-locale (English, accented, +40% length) in the test harness
  (`shellI18N('en', {pseudo: true})`) checks the layout under expansion;
  natively, `MYGO_LOCALE=en-XA` does the same for menus and dialogs.

### 4.4 Guarantees (tests)

- Every catalog has every message with identical `{placeholders}`
  (`node --test`, and `mygo i18n check`); every shell message is
  translated, not falling back to English (`go test`).
- Every `'shell…'` key used in code exists, and every shell message is used.
- No CJK text outside `locales/` in Go, JS or CSS; no user-facing string
  literals in shell code (`text:`, `aria-label`, `data-tip`, `label`,
  `placeholder`, `toast()`, `announce()`, `setText()`).
- Each mounted language sets `data-ytmgo-script`, puts `lang` on every
  shell root, and speaks its catalog.

---

## 5. Interaction specifications

### 5.1 Start listening (1 click)

| Step | State change | Feedback |
|---|---|---|
| Click a card's play button / a row | Service starts playback | Card play button springs up on hover (translateY 8→0, spring); playing row tints accent-soft with accent title |
| First playback of the session | `active` false→true | Player bar fills in (title, artists, art fades in 380 ms); panel docks (wide only) with a 380 ms spring slide; screen reader hears "正在播放：Title — Artist" |
| Canvas | unchanged | The service's player page no longer hijacks the canvas: you keep browsing where you were |

### 5.2 Control playback (always 1 click / 1 key)

- **Play/pause**: 40 px white circle; hover ×1.07, press ×0.92 (90 ms),
  release on spring. The play and pause glyphs cross-morph (outgoing
  rotates −90° and shrinks, incoming springs in). Buffering draws a
  rotating ring around it.
- **Seek**: 4 px track. Hover turns the fill accent, the knob scales in on
  a spring, and a tooltip previews the time under the cursor. While
  dragging, the track thickens to 6 px and only the local preview updates;
  the service seeks once on release. Keyboard: ←/→ ±5 s, PgUp/PgDn ±30 s,
  Home/End.
- **Volume**: same slider (±5% per arrow, ±20% per page, scroll wheel ±5%);
  the icon steps through off/low/high.
- **Like**: heart → filled accent heart with a 1.28× pop; `aria-pressed`.
- **Repeat**: cycles off → all → one; the icon swaps to repeat-one, an
  accent dot marks "on", and a toast plus a live-region announcement name
  the new mode.
- **Shuffle**: the service shuffles the queue (an action, not a mode); the
  icon spins once and a toast confirms.

### 5.3 See more of what's playing (progressive disclosure)

1. **Glance**: player bar (always visible).
2. **Panel** (1 click on art/title, Queue or Lyrics; `⌘J`): queue, lyrics,
   related. The Queue and Lyrics buttons toggle their own tab and switch
   between tabs without closing.
3. **Immersive** (⤢, `⌘⇧F`): full-window artwork and queue/lyrics.
4. **More actions** (⋯): the service's own track menu (add to playlist,
   go to album, share…), re-anchored to ytmgo's button.

**Escape** peels back exactly one layer: popover/dialog → immersive →
overlaid panel. A docked panel is layout, not a layer, so Escape leaves it.

### 5.4 Find something (≤ 2 steps)

`⌘K` or `/` focuses search from anywhere (a slash typed in a field stays a
slash). The service's suggestions open as a glass sheet fused to the
field. Enter shows results; filter chips sit directly beneath. Back/Forward
(`⌘[`/`⌘]`) restore each page's scroll position.

### 5.5 Navigate

Sidebar and tab-bar destinations dispatch the service's own SPA navigation
event, so navigation never reloads the page or interrupts audio. The
active destination switches to its filled icon and `aria-current="page"`.

### 5.6 Settings and account (hidden until needed)

⚙ opens one popover:

- **Appearance**: Dark · Light · System as a radiogroup (arrow keys move
  and apply). Persisted natively and applied instantly.
- **Language**: Match system (showing what it resolves to) and each
  language by its own name; a note says the music view reloads.
- **Google account**: sign in / manage.
- **YouTube Music settings**: the service's own menu, re-anchored.
- **Keyboard shortcuts** (`⌘/`).
- **Sign out and clear local data…** (only when signed in; opens the
  native confirmation).

The sign-out window uses the same tokens: brand mark, panel surface, a
spinner while clearing, then a pill action ("重试清理" / "返回首页").

### 5.7 Keyboard map

| Action | Keys |
|---|---|
| Play / pause | ⌘P · ⌘⇧Space (global) · Space (service) |
| Next / previous | ⌘→ / ⌘← |
| Volume / mute | ⌘↑ / ⌘↓ · ⌘⇧M |
| Search | ⌘K · / |
| Panel / immersive / sidebar | ⌘J · ⌘⇧F · ⌘B |
| Home / library | ⌘⇧H · ⌘L |
| Back / forward | ⌘[ · ⌘] |
| Shortcut sheet | ⌘/ |
| Close one layer | Esc |

(Ctrl on Linux/Windows; the in-app sheet shows the right glyphs per platform.)

### 5.8 Accessibility

- Landmarks: `nav` (sidebar, tab bar), `main` (canvas, focusable for skip),
  `aside` (now playing), `section` (player controls).
- Every icon button has an accessible name; toggles expose `aria-pressed`;
  the appearance picker is a `radiogroup`; popovers are `dialog`s with focus
  moved in, Tab trapped, and focus returned to the opener on close.
- Sliders follow the WAI-ARIA slider pattern with `aria-valuetext` such as
  "1:23 / 3:58", updated once per second rather than every frame.
- A polite live region announces track changes and repeat modes.
- Focus: 2 px Text-1 ring with a 5 px base-color halo, keyboard-only.
- Hidden layers are `inert` and `visibility: hidden` after their exit
  motion, so nothing invisible is focusable or clickable.
- Reduced motion is honored everywhere (tokens, keyframes, view transitions).
