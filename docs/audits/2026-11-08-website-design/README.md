# Website design audit: consistency and restraint (2026-11-08)

Scope: `site/` at d5180ef, all four locales, 320–1440 px. Checked against
three principles:

- **Design consistency:** one value for each decision (radius, type size,
  weight, spacing, component) and one component for each kind of content.
- **Behavioural consistency:** the same look always means the same behaviour,
  and anything that reacts to the pointer actually does something.
- **Restraint:** each device appears once, with a reason; coral and motion are
  kept for the moments that matter.

Measured from the stylesheet before the fix: 17 corner radii, 18 font sizes
(8 of them display sizes), 6 weights of the one display face, 3 button
heights, and coral used for 7 different meanings.

## Findings

| ID | Principle | Issue | Fix |
| --- | --- | --- | --- |
| D1 | Design | 17 corner radii (2, 3, 4, 6, 7, 8, 10, 12, 16 %, 18, 22 %…) | Four tokens: 4 px small (art, rows, keycaps), 8 px controls (menus, command boxes), 12 px panels (screens, cards, lyrics), pill (buttons, chips); the app icon keeps its 22 % shape |
| D2 | Design | 18 font sizes, 8 of them display sizes | Scale: 12 player · 13 meta · 15 small · 17 body · 19 lead · lyric line · section title · hero |
| D3 | Design | Bricolage Grotesque at 650/700/720/800 weight and 85/88 % width | One cut: 720 weight, 88 % width |
| D4 | Design | Three treatments for the same content, "supporting points": a mono hint (lyrics), a hairline grid (immersive) and scroll-lit display lines (library); two lyrics points were moved into the details table | One component, `.points`: hairline-separated items, stacked beside copy or in a grid under it |
| D5 | Design | Privacy was a settings group with round icon buttons, a third look for points-with-titles | `.points` with titles |
| D6 | Design | Monospace used for sentences (hero note, lyrics hint, measurement note, phone note, footer headings) | Monospace only for numbers, times, versions and code |
| D7 | Design | Footer headings were the only uppercase text on the page | Sentence case |
| D8 | Design | Details table: rows 1–6 have title + text, rows 7–10 only a title; the Area column restated each title; the token column was half empty | `#` and title + text only; rows that repeat other sections removed |
| D9 | Design | Button heights 36 / 48 / 56 px | 48 px on the page, 36 px only in the player bar |
| D10 | Design | Standalone links underlined in some places and not in others | Links inside sentences are underlined; standalone links are not |
| B1 | Behaviour | "Download for Linux" scrolled to the download track from the hero and the player, but downloaded a file in the download track | Buttons outside the download track say "Download ytmGO" / "Download" and go to it; only buttons in the download track name a platform or file, and they download |
| B2 | Behaviour | Details-table rows lit up on hover but do nothing | Hover highlight only on rows that act (the tracklist) |
| B3 | Behaviour | Privacy icons sat in round button shapes that do nothing | Removed with D5 |
| B4 | Behaviour | Hover responses differed: pills grew 4 %, play buttons 6 %, icon buttons changed colour | Pills change colour; round play controls grow, as in the app; every control presses to 96 % |
| B5 | Behaviour | Sections appeared as blocks, but the tracklist, table and FAQ appeared row by row | Everything appears as blocks |
| B6 | Behaviour | This computer's platform was marked twice: the mirrored buttons above and a coral border and badge on its card | The buttons above, plus a quiet neutral badge |
| R1 | Restraint | Coral meant brand, primary action, playback, danger, platform highlight, copy success and the active language | Three meanings: the brand mark, the one primary action in view, and what's playing |
| R2 | Restraint | Five giant type moments: hero 136 px, album title 120, "Press play" 168, figures 92, section titles 78 | The hero is the one giant moment; album title, finale and figures use the section-title size |
| R3 | Restraint | The app icon appeared four times, twice at hero scale (album 200 px, download 132 px) | Once large (the album cover), plus the nav and footer marks |
| R4 | Restraint | Seven motion devices: sung headline, lyrics demo, scroll-lit lines, count-up figures, tilting screenshot, row staggers, equalizer | Keep the two that demonstrate the product (sung headline, lyrics demo), the playing equalizer and one plain fade-in; drop the rest |

## Rules going forward

1. Use the tokens in `site.css` `:root`. A new radius, size or weight needs a
   reason, not a new value.
2. One component per content type: section header (track row), title + body,
   media panel, `.points`, `.meters`, `.table`, disclosure.
3. Coral is for the brand mark, the primary action and playback; nothing else.
4. Something that reacts to the pointer must act; something that acts must
   react the same way as its siblings.
5. Motion shows the product (lyrics) or the state (playing); everything else
   only fades in once.
