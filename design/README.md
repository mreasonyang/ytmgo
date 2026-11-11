# ytmGO Design Guidebook

This is how ytmGO should look, behave and speak, in the app and on the
website. It sets the target, not a description of today: where the app
differs, the gap is listed under [Adoption](#13-adoption) and the app moves
to the guidebook, never the other way round. `DESIGN.md` remains the app's
screen and runtime contract; for anything visual, this guidebook wins.

Values live in [`tokens.json`](tokens.json). The website build reads it
directly; the app will generate its constants from it when it adopts the
guidebook. Change a value there, not in code.

---

## 1. Idea

**The app is a record player.** Browse anything on the left and centre;
what's playing is always at the bottom, and one click shows more of it. The
interface stays dark and quiet so the artwork and the lyrics carry the colour.

**The website is a record.** Every section is a track on an album, introduced
by a row from the app's own track table and washed in one of the app's
header tints. A player bar follows the reader; the last track is "Press
play". The website shows the product by performing it, not by describing it.

### Lineage

ytmGO brings Spotify's desktop design language to YouTube Music; the launch
copy says so, and the design means it. What it borrows is the **language**:
the arrangement and behaviour people already know, so they can listen
without learning a new app. What makes ytmGO recognisably itself is its
own, and nothing of Spotify's identity is ever taken.

**Borrowed: structure and behaviour.**
- The window: a black frame holding rounded panels, a global bar with a
  centred search field, a library panel with filter chips, a three-zone
  player bar, and Now Playing as a side panel.
- The patterns: a numbered track table whose number becomes a play glyph on
  hover; a collection header with a large cover and a round play button;
  quick-access tiles on Home; a play button that rises on a hovered card;
  page headers washed in colour; a mini player above a tab bar in narrow
  windows.

These are layout and interaction conventions. They're described by what
they do, and drawn and built by ytmGO.

**Never borrowed: identity.**
- **Colour.** No Spotify Green (`#1DB954`, `#1ED760`) or any green standing
  in for it. ytmGO's colour is coral, and it means play.
- **Type.** Not Spotify's typefaces (Circular, Spotify Mix) or lookalikes
  chosen to resemble them. ytmGO uses the system font.
- **Marks.** Not Spotify's logo, wordmark, app icon or their shapes. ytmGO's
  mark is its coral play ring.
- **Assets.** Not Spotify's icons, illustrations, imagery, screenshots,
  sounds or animation. ytmGO draws its own interface; its icons are adapted
  from Lucide (§8).
- **Words.** Not Spotify's feature and product names ("Wrapped", "Discover
  Weekly", "Blend" and the like), slogans or marketing copy.

**Naming Spotify.** Mention Spotify only to say plainly where the design
language comes from, or that ytmGO isn't affiliated with it: in the launch
copy, the FAQ and the disclaimer. Never in a way that suggests
endorsement, partnership or that ytmGO is a Spotify product.

**Where ytmGO is its own.** Coral as the one colour of play; the playing
cover's colour leading the player (§4.4); a heavy system-font display
hierarchy; lyrics and immersive mode as first-class views; Chinese and Japanese
designed rather than fitted; and, on the website, the record. When adding
something new, look here first: a feature should feel familiar in where it
sits and how it behaves, and like ytmGO in how it looks.

## 2. Principles

1. **The music leads.** Artwork, lyrics and the playing track carry colour
   and size. Chrome is neutral and recedes.
2. **One meaning per signal.** Coral means *play*. A colour, shape or motion
   that means one thing never means another anywhere else.
3. **Restraint.** Each device appears once, for a reason. When in doubt,
   leave it out.
4. **Consistency.** One value per decision, one component per kind of
   content. A new value needs a reason, not a new number.
5. **It looks like what it does.** Whatever reacts to the pointer acts;
   whatever acts reacts like its siblings.
6. **Every language is first-class.** English, 简体中文, 繁體中文 and 日本語 are
   designed, not translated into leftover space.
7. **Plain words.** Say what it does for the listener. The engineering stays
   available, never in front.

## 3. Two layers

| | Product (app) | Brand (website, social, film) |
| --- | --- | --- |
| Purpose | Listening, all day | First impression, one visit |
| Type | System font; display titles at 800 | System font; display titles at 800, larger |
| Density | Compact, 15 px text | Generous, 17 px text |
| Motion | State changes only | State changes, one fade-in per block, the two product demonstrations |
| Colour | Neutral chrome, art-driven washes | The same palette; whole sections washed in tints |

**The replica rule.** When the website shows a piece of the app (the player
bar, a track row, the lyrics panel, chips, the round play button), it uses
the product values exactly. Brand-only pieces (the download button, section
titles, washes, points, cards, FAQ) use the brand values. Both share every
colour, the meaning of coral, the radius scale and the motion rules.

## 4. Colour

### 4.1 Roles

Dark is the primary appearance; light uses the same roles. The website is
dark only.

| Role | Dark | Light | Use |
| --- | --- | --- | --- |
| `base` | `#000000` | `#e9e9eb` | Window frame, global bar, player bar, page background |
| `surface` | `#121212` | `#ffffff` | Panels |
| `raised` | `#1f1f1f` | `#f2f2f3` | Search, round buttons, cards on hover, dialogs, popovers |
| `input` | `#242424` | `#ececee` | Secondary pills, segmented tracks |
| `highlight` | `#2a2a2a` | `#e1e1e4` | Hovered round buttons, menus, placeholders |
| `text` | `#ffffff` | `#121212` | Titles, primary labels, inverted buttons |
| `secondary` | 70 % white (`#b3b3b3` on black) | `#5a5a5f` | Artists, body copy, lead paragraphs, links, meta |
| `muted` | 56 % white (`#8f8f8f` on black) | `#6b6b70` | Notes, placeholders, table headings, sung lyric lines |
| `dim` | 34 % white | 34 % `text` | Upcoming lyric lines and unsung words only (decorative, §4.5) |
| `accent` | `#ff5a4f` | `#e8382d` | Play: fills and glyphs |
| `accentText` | `#ff5a4f` | `#b8261c` | Play: text (≥ 4.5:1 on every surface) |
| `onAccent` | `#000000` | `#ffffff` | Content on an accent fill |
| `attention` | `#ffb35c` | `#8a5300` | Errors and warnings, always with an icon and words |
| `line` | 10 % white | 10 % black | Hairlines, dividers, rings around images |
| `hover` / `press` | 7 % / 12 % white | 5 % / 9 % black | Quiet fills at rest / hovered; hovered rows |
| `track` | 30 % white | 21 % black | Unfilled progress and sliders, outline buttons |
| `selection` | 20 % white | 20 % `text` | Selected text |
| `inverseHover` | `#dbdbdb` | `#303030` | A hovered inverted button |
| `scrim` | 60 % black | 40 % black | Dims the window behind a dialog |
| `veil` | 70 % `base` | 70 % `base` | A surface over moving content: the sticky nav (with blur), the video's controls. 70 % keeps `secondary` links at 5:1 over white content |
| `pane` | 70 % `base` | 70 % `surface` | A card over the immersive ambience: the lyrics and queue card. Light takes `surface`, as `base` over a wash of its own tone vanishes; translucent text (`dim`) keeps its ratio on either, so the tone is for the card, not the words |

Dark text roles are white made translucent, so on a tint or a cover's wash
they take its colour; on black they are exactly the greys in brackets.
These are the only colours: no other share of a role, mixed by hand, is
allowed (the site's lint and the app's design test reject one). The
exceptions are the washes, whose strengths are tokens (`wash`), and
artwork.

### 4.2 What coral means

Coral is ytmGO's colour, and it means **play**.

**Coral is for:**
- The play action: the round play button of a collection, the play glyph on
  a hovered card, the website's download button (downloading is the
  visitor's "Press play").
- What's playing: the playing track's title, the equalizer or speaker
  glyph, the hovered progress bar, active playback modes (shuffle, repeat;
  always with a dot).
- The brand mark: the "GO" in the wordmark, the app icon.

**Coral is never for:** errors (use `attention`), selection (invert),
navigation state (filled icon plus `text`), account or Premium status,
success messages, focus rings, links, or decoration.

**One coral action in view.** If two coral buttons can be seen at once, one
of them is wrong. The primary action that isn't play (sign in, save,
install update) is an **inverted** button: `text` on `base`.

### 4.3 Header tints and washes

Eight tints colour pages that have no artwork, chosen per page from a stable
hash so a page always keeps its tint: blue `#2d46b9`, violet `#8d67ab`, teal
`#1e7a6b`, rust `#b0472b`, steel `#477d95`, brown `#7d4b32`, slate `#5179a1`
and raspberry `#a13d63`.

In the app a page's wash runs from the tint at `wash.dark` (50 %; light
32 %) at the top to nothing. The website's sections are the brand's, and
richer: the tint at `wash.brandTop` (70 %) over black, through
`wash.brandMid` (34 %) at 260 px into the background. 70 % is the most
that keeps `secondary` text at 4.5:1 at the top on every tint. Pages with artwork wash the same way in their
cover's colour (§4.4), never in a blurred copy of the cover, which turns
blotchy; a cover without colour leaves the page neutral. Only immersive
mode, where the cover is the subject, fills the view with its blur. A wash
belongs to its page's header, so it **scrolls away with the header**; it
never stays pinned while a list scrolls through it. What is fixed to a
viewport is only what describes that viewport: edge fades (shown while
there is more beyond the edge), the immersive ambience, and the washes of
panels that don't scroll (the player bar, a docked Now Playing panel); a
floating Now Playing sheet scrolls, so its wash scrolls away like a page's.
Scrolling edges follow three conventions: a list under a fixed header (the
library, a sheet) gets a hairline under the header once content has scrolled
beneath it and a fade at its bottom while more lies below; a panel that
scrolls whole (the page, the floating Now Playing sheet), whose header
scrolls away with it, fades at its bottom the same way, its top being the
panel's own edge; a list inside a card (lyrics, queue) fades at whichever
edge has more beyond it. Rows are
never cut off at an edge without one of these. The website gives each track one tint; the intro and the download
share rust, so the record starts and ends in the same light.

### 4.4 What's playing lends its colour

The playing track's cover colours the player: the Now Playing panel washes
down from it, the player bar glows with it behind the track and fades out
before the transport, and the compact mini player is filled with it, more
weakly (28 % dark, 16 % light) since it holds text: its secondary text keeps
4.5:1 on every colour the picker can return, which a test checks. The
colour is read once from the cover when it loads: the hue carrying the most
colour (vivid, bright pixels outweigh large dark ones; near-black, near-white
and grey don't count), normalised to one brightness and a bounded saturation
so text stays readable on any cover. A cover without colour lends none, and
the chrome stays neutral. Washes are 50 % (dark) or 32 % (light) of the tint,
the same strength as an app page's header wash (`wash` in the tokens); the player
bar's is 70 % of that. Coral is never replaced by a cover's colour:
play stays coral.

### 4.5 Contrast

Text is at least 4.5:1 against what's behind it, large display text and
interface glyphs 3:1. Dimmed states (upcoming lyric lines, disabled
controls) may go lower only when the same text is readable elsewhere or
becomes readable by itself. Colour is never the only carrier of state.

## 5. Typography

### 5.1 Faces

**ytmGO uses the system font, always.** It ships no typefaces: the app
registers none and the website loads none.

| Face | Use |
| --- | --- |
| **System UI** (SF Pro, Segoe UI, Noto Sans; PingFang, Microsoft YaHei/JhengHei, Hiragino) | All text, display titles included. Titles are the same face at the display weight (800) and leading (1.05; 1.18 in CJK), with the platform's own spacing and optical sizes: no added tracking |
| **System monospace** (SF Mono, Cascadia Mono, Menlo, Consolas) | Numbers that line up: track numbers, durations, times, versions, code. Never sentences |

The system face reads natively on every platform, covers every script
without fallbacks, and can't be shadowed, mis-cut or mis-licensed; what
makes ytmGO's type its own is the scale, weight and hierarchy, not a face.

### 5.2 Scale

| Token | Size | Product | Brand |
| --- | --- | --- | --- |
| `micro` | 12 | Player artist, badges | Player bar labels, badges |
| `meta` | 13 | Artists, meta, table headings, time readouts | Notes, footer, mono numbers |
| `small` | 15 | Row and card titles, buttons, player title | Labels, points, table text, buttons |
| `body` | 17 | Library title, section headings | Running text, FAQ questions |
| `lead` | 19 | — | Section introductions |
| `h3` | 24 | Shelf headings, Now Playing title | Card titles |
| `h2` | 32 | Page titles, immersive title | — |
| `line` | 22–30 | Lyric lines | Lyric lines in the demo |
| `title` | 32–72 | Collection title | Section titles |
| `hero` | 54–136 | — | The one hero headline |

Weights: 400 text, 600 labels, 700 headings, 800 display. Leading:
1.05 display, 1.2 headings, 1.35 interface, 1.6 running text (1.8 in CJK).

**Every heading at `h3` and above is a display title**, set at the display
weight and leading: page, shelf, sheet and empty-state titles alike. Quiet
labels (a panel's name, a title bar's) are `meta`/`body` at 600–700. Nothing
is letter-spaced: the system face's own spacing is right at every size.

**One giant moment per surface.** The website has one hero size; the app's
largest type is a collection title.

### 5.3 Chinese and Japanese

- Headings never break inside a word: they keep phrases whole
  (`word-break: keep-all`) and break only at spaces, punctuation and phrase
  marks. In locale strings a `|` marks a phrase boundary
  (`你的音乐，|值得不止一个|浏览器标签页`). The website turns marks into
  `<wbr>`; the app joins each phrase's characters with word joiners.
- Running text breaks anywhere, with strict kinsoku, 1.8 leading, +2 %
  tracking and automatic CJK–Latin spacing.
- Display sizes step down (`sizeCJK`): full-width glyphs are about twice as
  wide as the condensed Latin face.
- No negative tracking, no faux italics.
- Text carries its `lang`, so each script gets its own fonts; Japanese kana
  never falls back to Chinese fonts.
- Names stay whole: a no-break space joins "YouTube Music".

## 6. Space and layout

**The base unit is 4 px.** Every spatial value is a positive multiple of
4: padding, margins, gaps, component widths and heights, icon sizes and
their containers, and positioning offsets. Exempt: 1 px borders, hairlines
and dividers, and shadows' blur and spread. A size computed at run time
(an icon from its button, a cover from the window) is rounded to the grid.
Prefer the scale **4, 8, 12, 16, 24, 32, 48, 72, 104, 144**; other
multiples (20, 28, 40) are allowed where the scale can't fit. The app's
design guard and the website's lint fail on an off-grid value.

**Product** (from `tokens.json` → `product`):

| Region | Wide (> 1180) | Medium (761–1180) | Compact (≤ 760) |
| --- | --- | --- | --- |
| Global bar | 64 | 64 | 60, in-page |
| Library | 300, collapsible to a 72 rail | 72 rail | Library tab |
| Page panel | radius 16, inset 24 | same | full bleed, inset 16 |
| Now Playing | 360 docked | 380 floating | sheet |
| Player | 80, three zones | 80 | 60 mini player + 64 tab bar |

Panels sit 8 apart on the black frame. Targets are at least 32 with a
pointer and 44 for touch.

**Brand:** content width 1240 (1320 for wide media), 144 between tracks
(104 compact), 72 between a title block and its content (48 compact),
24 between items.

## 7. Shape and depth

Corners come from one scale in 4 px steps: **8, 12, 16, 24, 36, 44**, and no
other value. Four of them have roles; 36 and 44 are free for larger
surfaces when a design needs them.

| Token | Radius | Use |
| --- | --- | --- |
| `art` | 8 | Artwork up to 64 px, thumbnails, keycaps, icon tiles, segments, items inside menus, tooltips |
| `control` | 12 | Rows, tiles, cards, menus and popovers, inputs, command boxes, artwork above 64 px |
| `panel` | 16 | Panels, the mini player, groups inside a sheet, the lyrics panel, website cards |
| `sheet` | 24 | Dialogs and sheets |
| `pill` | capsule | Buttons, chips, search, badges, progress bars |
| round | 50 % | Icon buttons, play buttons, avatars, artist art |
| `icon` | 22 % | The app icon, everywhere it appears |

A capsule and a circle are shapes, not radii: their corner is half their
height, whatever that is. The app icon's corner is a share of its size.

**Nesting:** a shape inside another has a smaller corner, smaller by no more
than the space between them: outer > inner, and outer − inner ≤ padding.
So the roles step up as they hold one another: art 8 in a row 12 in a panel
16, a tile 8 in a group 16 in a sheet 24. Where a chain would need one step
more than the space allows, the inner shapes give up their own corners: a
list inside a card (the Now Playing queue) is made of flat bands that the
card clips, as the Settings groups are. A shape flush with its container
(the art of a quick-access tile) takes the container's corners on the
sides it shares (12, 0, 0, 12), never 0 with the container's clip left to
round it: MyGo's GPU renderer rounds only the innermost clip, so an inner
clip squares the outer corners off. A focus ring nests around its element the same
way: element radius plus the ring's offset.

A screenshot nests around the panels in it at the scale it is shown:
showing a 1440 px window at scale s, its corner must exceed 16s by no more
than 8s (the frame gap), so it is 16 from 960 px wide, 12 from 720, and 8
below; the phone shot's frame is 16 from 280, 12 from 212. Below about
480 px (a phone's viewport) the panels inside draw at 4 or less and nothing
on the scale nests; the frame stays at the floor, 8.

**Depth:** a shadow must separate something that floats or lifts; it is
never decoration. Shadows are black at an opacity tuned on dark surfaces;
light appearance draws them at `lightScale` (38 %) of it, since black on
white reads far heavier.
- **Shadowed:** a collection's cover and the immersive cover (`art`),
  floating panels (`hero`), popovers, tooltips, the play button that rises
  over a hovered card's art, and a card's art while hovered.
- **Not shadowed:** cards at rest (spacing separates them), the collection's
  coral play button (its fill is its affordance), and dialogs in dark
  appearance, where the scrim and hairline already set them apart and a
  shadow on the scrim can't be seen.
- **Edges before shadows:** on a light page, a cover gets a 1 px `line`
  border, since a pale cover on white needs an edge a light shadow can't
  give. Everything else is flat, separated by `line` hairlines or surface
  steps.

## 8. Iconography

One 24 px grid, 1.8 px round strokes, adapted from Lucide (ISC; its
Feather-derived icons MIT, notice in `resources/notices`). New icons come
from the same set, redrawn to the same stroke, or are drawn to match it;
never from another app's interface. **One glyph, one meaning:** the
diagonal expand arrows always mean immersive mode; a panel that grows or
shrinks in place uses vertical chevrons, and a control whose glyph changes
with its state needs no active dot. Filled variants mark the active
destination and the primary transport glyphs. **Optical, not geometric, centring:** a
glyph is centred by where the eye puts its weight, between its bounding
box and its ink's centroid. Asymmetric glyphs carry their correction in
their path (play right of centre, the heart 1 unit low, shuffle 1.5 units
left). **Optical corrections never break the 4 px grid:** they live in a
glyph's own geometry (its path, viewBox or clip), never in an off-grid
padding or offset. A leading download icon's viewBox is shifted 2 units
so its ink sits as far from the pill's edge as the label's does; digits and
wordmarks are centred by their cap height (`text-box: trim-both cap
alphabetic`), not their line box; a chevron is a glyph centred on the line,
where the system font's cap height also centres.
Check by measuring rendered ink, not by eye. Icons label actions; they
don't decorate. Icon-only buttons always have an accessible name and a
tooltip. The website uses icons only on actions and platform names.

## 9. Motion

| Token | Value | Use |
| --- | --- | --- |
| `fast` | 150 ms | Hover and press colour, small state changes |
| `base` | 250 ms | Menus, disclosures, panel changes |
| `slow` | 700 ms | Lyrics scrolling, fade-ins, a cover's colour blending to the next |
| `morph` | 400 ms | Long journeys across the window: into and out of immersive mode, a collection's cover into its page |
| `ease` | `cubic-bezier(.2, .7, .2, 1)` | Everything that eases |

- **Motion shows the product or its state.** The product animates state:
  lyrics following the song, progress, the playing equalizer, panels opening.
  Nothing moves for decoration.
- **The website may add only:** the sung headline and the lyrics demo
  (they *are* the product), and one fade-in per block as it scrolls into
  view. No count-ups, no tilts, no parallax, no per-row staggers.
- **Scrolling the product does** glides over `slow` along `ease`: the
  lyrics to each sung line, a page back to its top when its tab is chosen
  again (at the top, choosing it reloads), the website to a section, a
  track or its top (both logos). The keys that page (arrows, Page Up/Down,
  Home/End) glide over `base`, as browsers smooth them, and leave a
  focused control the keys it uses. What the reader moves directly (a
  wheel, a drag, the website player's seek) follows them at once, and
  grabbing a glide stops it. A view that appears or relays out arrives at
  its place without gliding there.
- **Continuity: what persists moves; it never vanishes and reappears.**
  When a state changes, the elements both states share carry the eye from
  one to the other:
  - Lyrics expand: the cover shrinks into the thumbnail beside the title
    while the title, the switch and the card move to their places
    (`base`); restoring runs it back.
  - Now Playing docks by sliding in from the right edge, where the
    controls that open it are, and slides back out; the page keeps its
    width until the panel has gone, then eases to fill the room. Floating,
    it rises from the bottom and sinks back.
  - Immersive mode: the cover grows from the one that was showing (the
    panel's or the player bar's) and shrinks back into the one that will;
    the lyrics card travels with it; the library and the global bar fade as
    the page grows over their room (`morph`). The cover and the card fly
    over the window, never cut by the column or panel they land in, and
    each goes one way, with no swing past its place: docked Now Playing,
    their destination on the way back, fades in where it is rather than
    sliding in under them.
  - Opening a collection, its header shows at once from the card, and the
    card's cover travels into it (`morph`); Back carries it home.
  - Settings and Keyboard shortcuts are one sheet: its frame takes the
    next page's width as the pages slide, Settings from the left.
  - Track changes: covers cross-fade, titles rise 8 into place over the
    last fading, and cover colours blend (`slow`). Cover and video are one
    frame that changes shape. Small swaps (a play glyph, a card's view, a
    chip's fill, the website's Copy) cross-fade over `fast`; popovers drop
    4 into place as they fade in.
- Only opacity and transforms animate, except a shared element's frame
  (a cover or a card on its journey) and a sheet's or panel's size, which
  morph.
- Reduced motion, the website player's pause, and no JavaScript all show
  the final state immediately. Nothing is ever hidden waiting for motion.

## 10. Interaction

- **Scroll bars** show as a list scrolls and as the pointer comes over it,
  and fade after a second at rest, wherever the pointer rests; on the
  bar's own strip they stay, brighter (`muted` for `track`), to be grabbed.
- **Hover only on what acts.** Rows and cards that open something get the
  `hover` wash; text and tables that don't, get nothing.
- **Hover response by kind:** capsule buttons change colour (to
  `inverseHover`, `highlight` or `press`); play controls, round or the
  coral capsule, grow to 105 %, as coral never shifts; icon buttons go from `secondary` to `text`;
  rows wash. Every **button** (capsules, chips, icon and play buttons)
  presses to 96 %; rows and cards answer with the `press` wash, since a
  whole row shrinking would jump.
- **Button hierarchy:** accent (play, one in view) → inverted (primary) →
  outline (secondary) → plain link (tertiary). Brand buttons are 48 high,
  the player bar's 36; product pills 36, chips 32.
- **Links:** underlined inside sentences, plain when they stand alone.
- **Labels tell the truth.** A button that downloads names the file or
  platform; a button that only navigates says where it goes ("Download
  ytmGO" leads to the download section).
- **Destructive actions are neutral:** signing out or clearing data is
  `text`, never coral (play) or `attention` (errors); the confirmation
  names what will be lost.
- **Selection is inversion:** a selected chip, tab or segment is `text` on
  `base`. A playing item is coral.
- **Focus** is a 2 px `text` ring, 3 px out, on every focusable element.
- **The website player** is the app's player bar: it shows the section
  you're in, its clock runs from 0:00 at the top to the total at the bottom,
  ⏮ ⏭ move between tracks, the seek bar scrolls, and its play button pauses
  every loop on the page.

## 11. Components

**Product:**
- **Track row:** 56 high, radius `control`, 40 art (`art` radius); in the
  Now Playing card a flat band across the card. Number,
  title over artist, album, duration in mono. The playing indicator has its
  own slot, so columns line up whether a row plays or not, and a list whose
  rows all come from one album (an album's own page) drops the Album column. The number becomes a play glyph
  on hover and an equalizer while playing; the title turns coral.
- **Library row:** 64 high, 48 art (round for artists), title over
  "kind · owner". The open collection is washed and its title is `text`
  (navigation state isn't coral).
- **Quick-access tile:** 64 high, art filling its height, radius `control`.
- **Card:** 164 art (140 compact), title, two-line subtitle; the play button
  rises on hover.
- **Collection header:** 232 art (192 medium) beside the collection title,
  then the 56 coral play button.
- **Player bar:** 80 high, three zones: now playing (56 art), transport
  (white round play/pause, icon buttons, progress with mono times),
  and output (lyrics, queue, volume, more, expand); washed with the playing
  cover's colour behind the track (§4.4).
- **Immersive layout:** the media column (cover or video, the switch, the
  title and artist, centred) and the lyrics card are one group, centred in
  the stage, 64 apart; the card runs from the media's top to the artist's
  line. The media takes the height the text leaves; the column is the
  wider of the media and the title's own width, which wraps past 560.
- **Now Playing panel:** washed from the top in the playing cover's colour;
  the track title is a display title. Lyrics are a first-class view, so the
  cover never takes first claim on the height: it gets 45 % of the space it
  shares with the lyrics (at least 120, at most the panel's width). Lyrics
  first, the cover becomes a 56 thumbnail in one row with the title, artist
  and like, the Cover/Video switch beneath, and the lyrics take the rest; a
  video keeps its size. The reader's choice and reading position survive
  switching. A video takes the panel's full width (16:9) at any height; the
  switch is there only for a track that has one.
- **Video controls:** over the video's corner (8 in) under the pointer, 32
  icon buttons on the stage's `veil`: fill the play view (a toggle, with its
  dot) and full screen. Full screen is the black stage, the video as large
  as it fits, and the transport over a scrim fading from clear to `veil`
  up from its foot (a band would cut the picture), shown while the pointer
  moves or playback pauses; it fades in and out over `fast`.
- **Chips** 32, capsule, `hover` fill, inverted when selected. **Pills** 36.
- **Lyrics:** the current line is `text` and slightly larger, sung lines
  `muted`, upcoming lines `dim`; it follows playback until you scroll, then offers
  "Follow playback". Lines scrolled past the view's top or bottom fade out
  over 24 rather than being cut, and an edge fades only while more lies
  beyond it.
- **Dialogs and sheets:** radius `sheet`, `raised`, one inverted primary
  action; their groups `panel`, rows in a group flat or `control`, icon
  tiles `art`.
- **Tooltips:** inverted, `micro` text, radius `art`, the `tooltip`
  elevation, 8 from their anchor. A slider's value (time, volume) is one,
  anchored on the thumb rather than the taller hit area: shown under the
  pointer and while dragging, and for a second after a key moves it, the
  volume shortcuts included.

**Brand:**
- **Track head:** the section's row from the app's track table: number,
  title, duration, a hairline below. Coral title and equalizer while the
  reader is in it.
- **Title + body:** a display title, then a `lead` paragraph in
  `secondary`, no wider than 46 characters.
- **Media panel:** a real screenshot (its corner nests at its shown size,
  §7) or a live replica, radius `panel`,
  with a hairline ring. No drop shadow: on the black page one can't be
  seen, and the ring does the separating. No fake window chrome. A replica
  follows the app's surfaces: the lyrics demo is the Now Playing panel,
  washed from the top in its playing cover's colour (§4.4), not in its
  section's tint. Website shadows use the elevation tokens
  (`--shadow-*`), and only on what floats, as the language menu.
- **Points:** supporting details, separated by hairlines, stacked beside the
  copy or in a grid beneath it. Optional `body`-size bold titles.
- **Figures:** a display-size number over a `small` label; with a note on
  how it was measured.
- **Table:** the app's collection table: a mono number, then a title over a
  description.
- **Disclosure:** a hairline-separated summary with a chevron; it opens
  smoothly.
- **Download:** one coral button for this computer's file, one outline
  button for the alternative, the requirements as a note; every platform
  behind a disclosure.

## 12. Words

- **Voice:** warm, plain and specific. Short sentences, one idea each.
  Lead with what the listener gets ("Quit mid-song, and it waits at the
  same second"), not how it's built.
- **Engineering details** (Ed25519, AES-256-GCM, proxies, memory figures)
  belong in details, notes and FAQ, never in headlines or leads.
- **Names:** ytmGO (lower-case "ytm", capital "GO"); YouTube Music, with a
  no-break space; YouTube Music Premium; Apple silicon; macOS, Windows,
  Linux; WebKitGTK.
- **Interface labels** are quoted with the language's own marks:
  “Follow playback” (en), “跟随播放” (zh-Hans), 「跟隨播放」 (zh-Hant),
  「再生に合わせる」 (ja).
- **Numbers:** digits for all figures; tabular digits wherever numbers sit in
  a column; units as the app shows them (MiB, fps).
- **Every string exists in all four languages.** The website build fails if
  a key is missing, extra or unused.
- **The disclaimer** ("an unofficial open-source client … not affiliated
  with Google LLC, YouTube or Spotify") appears on every page.

## 13. Adoption

**Website:** complies. Its build generates every token from `tokens.json`
and fails if `site.css` hard-codes a colour, radius or font size outside the
marked artwork rules.

**App:** adopted on the `app-guidebook-redesign` branch:

| Area | Before | Now |
| --- | --- | --- |
| Tokens | Hand-written in `native_style.go` | Generated from `tokens.json` (`native_tokens_gen.go`), checked by a test |
| Coral | Also errors, selection, Premium, open library item, every active toggle, the hovered volume slider | Play only; errors `attention` with an icon, selection inverted, Premium neutral, non-playback toggles `text` with a dot |
| Colour shares | Text at 20, 30, 86 %, base at 35 %, coral at 25 %, made in the views | Roles only: `track`, `selection`, `inverseHover`, `veil`, `pane`, `dim`; lyric lines `muted` / `dim` |
| Radii | 4, 5, 6, 8, 10, 12, 14 | The scale 8, 12, 16, 24: `art` 8, `control` 12, `panel` 16, `sheet` 24, every nest smaller inside |
| Type | Base 14; 11, 14, 16, 20, 22 in use | The scale: 12, 13, 15, 17, 22, 24, 32, title |
| Display titles | System font 800, ad hoc | System font at the display weight and leading, through one helper (`nativeDisplay`) |
| Feedback | Native defaults | Buttons press to 96 %; play controls grow 105 % |

**CJK phrase marks:** adopted. The app's catalog marks its headings with
`|` like the website's locales; `phrases` (i18n_phrases.go) joins each
phrase's characters with invisible word joiners (U+2060), which MyGo's line
breaker honours, so headings break only at the marks. Catalogue names from
YouTube Music can't be marked and break as the system breaks them, as on the
website.

## 14. Changing the guidebook

1. Change `tokens.json` and this file in the same commit, with the reason.
2. Rebuild the website (`go run ./site/gen`) and look at it in all four
   languages and at phone width.
3. A new value needs a reason that an existing one can't serve. A new
   component needs a kind of content that no existing one fits.
4. Record design reviews under `docs/audits/`.
