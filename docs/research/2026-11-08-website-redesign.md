# Website redesign research (2026-11-08)

Review of `site/` at 41201a3 and of how strong product sites solve the same
problems. Reference captures were taken with headless Chrome at 1440 px and are
not committed (third-party content).

## 1. Why the current page reads as generic

The page is well built (fast, accessible, localized, honest), but every visual
decision is the default one, so nothing about it says *music* or *ytmGO*.

| Pattern on the page | Why it reads as template |
| --- | --- |
| Pill eyebrow → two-line headline with a gradient second line → centered 4-line lead → primary + ghost GitHub button → small note → screenshot in a fake macOS window → dot-separated "facts" row | The default hero of every v0/Tailwind landing page since 2023. |
| Blurred coral/violet radial glows behind every section | The most recognizable "AI-generated" texture. It adds no information. |
| Four feature rows with the same recipe: uppercase coral kicker → 60 px heavy headline → grey paragraph → three ✓ ticks → tilted window screenshot | No change of rhythm for 4,000 px; the eye stops reading after the second one. |
| 3×2 grid of outline-icon cards ("Thoughtful down to the details") | A "feature soup" of six equal-weight cards. Nobody remembers any of them. |
| Gradient numbers in stat cards, "01–04" numbered step cards | SaaS dashboard tropes on a music site. |
| One weight-850 system sans everywhere, tight tracking, no display face | No typographic voice. |
| Fictional covers are all the same radial "orb" gradient | Music sites live on artwork; ours looks like placeholder art. |
| Copy: Ed25519, AES-256-GCM, HTTP_PROXY, 90.81 MiB, WebKitGTK 2.50.2 | Engineer-facing details in the selling copy. They belong in specs, FAQ or docs. |
| Coral accent + giant heavy type | Close to Cider (cider.sh), the most visible competitor. We'd look like a copy of it. |
| macOS traffic lights on every screenshot, even for Linux/Windows visitors | The app hides its title bar on Linux/Windows; the frame is decoration that misrepresents it. |

## 2. What the best sites do (and what to take)

| Site | What it does well | What to take |
| --- | --- | --- |
| Apple Music (apple.com/apple-music) | The page plays: video tiles with pause buttons, and a lyrics card ("Try me") with real karaoke-style lines over a background taken from the album art. | Make the page perform the product. Take the background colour from the artwork. Give every motion a pause control. |
| Teenage Engineering (OP-1 page) | The product is shown as an object with extreme restraint, then *100 new features* as one dense centred typographic list, not cards. | Replace the icon-card grid with a dense **liner-notes/spec list**. That is where the nerd details go, and it feels generous instead of noisy. |
| Cider (cider.sh), competitor | Marquee bands of outlined and filled display type, monospace UI labels and a strong red. A confident editorial voice. | Commit to one typographic voice the same way, but **not** theirs: we should move away from coral-red and outlined marquees. |
| NTS Radio | Persistent "live now" strip, editorial grid, imagery first. | A persistent **now-playing bar** that stays on screen as you scroll. |
| Raycast | Centred hero, then the real UI rebuilt in HTML and interactive; the download button carries a tiny mono line ("macOS 13+ · Install via Homebrew"). | Rebuild a few UI moments in HTML (player bar, ⌘K search, lyrics) instead of flat screenshots. Keep the requirements to one mono line under the button. |
| Linear | Restraint: no icons, a lit perspective hero shot, sections with the title on the left and the body on the right. | Remove ornamental icons and glows; let type, spacing and one strong image carry each section. |
| mymind | A manifesto with a typographic "NO social features / NO ads / NO …" list. | Privacy as a short **manifesto**, not four numbered cards. |
| Overcast | "A normal business": a personal, plain-spoken note from the maker. | A short maker's note: who builds this, why, and the unofficial-client honesty. It earns trust better than "Privacy and security". |
| Plexamp | Playful headings ("All Gas, No 'Breaks'") and the app shown in context (car dashboard). | Show the app in a real setting (desk, laptop at night) at least once, and allow some wit in headings. |
| Sleeve (replay.software) | The same UI shown in a strip of themes with different album art, which turns colour into the feature. | Show ytmGO's header wash recoloured by different covers. It is a real feature of the app and it is beautiful. |
| Spotify's Linux download page | Plain and practical. | Move the platform details (apt, brew, WebView2, SmartScreen) to a **dedicated download page**. The landing page needs only a one-click button. |
| Daydream Player (Awwwards SOTD 2025), Poolsuite | The site *is* a tactile player with a nostalgic character. | Personality comes from making the page an instrument, not a brochure. |

## Revision (same day): no copying

The references above inform principles only: the page should perform the
product, use restraint and keep one voice. None of their signature devices are
reused. Every visual device on the site comes from ytmGO's own interface: the
player bar, the track table, the synced-lyrics view, the header wash with the
app's tints (`native_style.go`), the settings group and the keycaps. So the
"NO …" manifesto and the dense spec wall in §3 were replaced by the app's
settings group (privacy) and its collection table (the details). The launch
pitch and the measured figures from 41201a3 are kept word for word.

Built in this pass: album structure with 9 tracks, a sung headline, the page
player, an interactive lyrics panel, lines lit by scroll, tracks washed in the
app's tints, Bricolage Grotesque and JetBrains Mono, and download details folded
under one disclosure. Not yet done: new fictional cover art and a video loop.

## 3. Recommended direction: "The page is a record"

One idea organizes everything: **scrolling the site is playing an album.**

- **A persistent player bar** (real HTML, ytmGO's own 3-zone layout) is pinned
  to the bottom of the viewport. Its "now playing" title is the current
  section, its progress bar is scroll progress, and ⏮/⏭ jump between sections.
  It doubles as the download CTA ("Download for Linux" sits where the volume
  slider would be).
- **Sections are tracks.** Each is introduced like a tracklist row
  (`02 Lyrics that keep time 3:12`) instead of a coral kicker. There are 6–7
  tracks instead of 11 equal sections, and the page gets about 40% shorter.
- **Hero = a synced lyric.** The headline is sung: a karaoke highlight sweeps
  word by word across "Your music deserves more than a browser tab", the same
  way the app highlights lyrics. It demonstrates the key feature in the first
  second. Below it, a full-bleed render of the app without fake chrome, with
  the page background washed in the colour of the playing cover.
- **Colour comes from the artwork.** Each track/section takes its background
  wash from "its" cover, as the app's header wash does. The page shifts from
  amber to teal to violet as you scroll; coral stays only as the brand/CTA colour.
- **Real art direction for the fictional catalogue.** Replace the radial-orb
  covers with a designed fictional label: around 12 covers in distinct styles
  (Swiss typographic, risograph halftone, photographic gradient, collage), so
  the screenshots look like a real record collection. The covers come from
  the Go generator, so this also improves the README shots and the promo film.
- **One display typeface** with character, self-hosted (CSP is `self`). It
  should pair with system CJK fonts, since display faces don't cover
  zh/ja; candidates are Bricolage Grotesque, Instrument Sans/Serif or Unbounded.
  A monospace face is for tracklist numbers and spec lines only.

### Proposed running order

1. **Hero:** synced-lyric headline, auto-detected download button with
   one mono requirements line, and a full-bleed app render.
2. **Lyrics:** an interactive HTML demo: lines advance on a timer, and
   clicking a line seeks to it ("drag through the lyrics" made real).
3. **Immersive & video:** a looping muted video captured from the real app
   (the Xvfb capture route already works), with a pause control.
4. **Your library / any window:** scroll-driven sequence (CSS scroll-driven
   animations, no library) that morphs wide → rail → mini player, plus a
   Sleeve-style strip of the header wash under different covers.
5. **Manifesto:** "No ads. No tracking. No browser extension. No login
   server. Just your music." Then one paragraph on encryption and sign-out, and a
   link to details.
6. **Liner notes:** a TE-style dense list of 30–40 details in small mono:
   60 fps, 90 MiB paused, Ed25519 updates, ⌘K, media keys, proxies, four
   languages, light/dark, Premium or free… The engineering credibility stays,
   but as texture, not as the pitch.
7. **Maker's note + FAQ + footer** (disclaimer kept verbatim).

A separate **/download** page per locale holds the full platform cards
(apt/brew commands, WebView2, SmartScreen, notarization).

### Copy principles

Lead with feelings and moments ("Quit mid-song. It waits at the same second.");
one idea per sentence; no algorithm names above the liner notes. This matches
the direction already chosen for the launch film.

## 4. Constraints to keep

- Generator contract: every visible string comes from `locales/*.json`, and the
  key sets stay in sync across the four locales.
- CSP `self` + api.github.com means self-hosted fonts, no CDNs and no third-party JS.
- The page must work without JS (static fallback for the lyric demo and the player bar).
- `prefers-reduced-motion` disables the karaoke sweep, the scroll morphs and the video autoplay.
- Fictional catalogue only: no real artists, covers or lyrics.
- CJK: no negative tracking, and headings fall back to the system CJK heavy weights.

## 5. Order of work

1. **Quick wins (small):** remove glows, gradient text, tick lists, icon cards
   and fake window chrome; introduce the display face; rewrite copy; cut to the
   7-track structure; move platform details to /download.
2. **Signature pieces (medium):** synced-lyric hero, persistent player bar,
   artwork-driven section colours, liner notes, manifesto.
3. **Assets (medium):** new fictional cover set in the Go shot generator;
   re-render shots; record the immersive video loop.
4. **Polish:** scroll-driven window morph, cover-recolour strip.
