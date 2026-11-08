# ytmGO website

The official website: one static page per language (English, 简体中文,
繁體中文, 日本語), with no build-time JavaScript tooling and no third-party
requests except the GitHub releases API that fills in the download links.

```sh
make site          # build into site/dist
make site-shots    # re-render the app screenshots (needs Python with Pillow)
python3 -m http.server 8000 --directory site/dist
```

- `template.html` is the page; every visible string is a `{{t "key"}}` lookup.
  The page is laid out as an album in the app's own visual language: each
  section is a track (`section.track` with `data-time`) under a row from the
  app's track table, washed in one of the app's header tints (`tint-*`, the
  colours in `native_style.go`). A fixed player bar, the app's player bar,
  follows the scroll, skips between tracks, and its play button pauses every
  animation. Without JavaScript the bar is hidden and everything is static.
- Motion is opacity and transforms only. Content eases up as it scrolls into
  view (`.rv`, added by `site.js` only when motion is on), the measured
  figures count up, disclosures open smoothly, and the hero screenshot settles
  with scroll. Reduced motion, the player's pause and no JavaScript all show
  everything in place; the pause freezes only the decorative loops.
- `assets/fonts/` holds Latin subsets of Bricolage Grotesque (headings) and
  JetBrains Mono (numbers and times), both under the SIL Open Font License
  (texts beside them). Other scripts fall back to the system fonts.
- `locales/<locale>.json` holds the copy. `go run ./site/gen` fails if a locale
  misses or adds a key, or if the template stops using one, so the four
  languages stay in step. Keys starting with `js.` are passed to `site.js`.
- Chinese and Japanese headings never break inside a word: they break only at
  spaces, punctuation and `|` phrase marks in the locale strings (for example
  `"你的音乐，|值得不止一个|浏览器标签页"`). Headings use `{{wbr "key"}}`, which
  turns each `|` into `<wbr>`; `{{t "key"}}` and the script's strings drop the
  marks. When you add or reword a CJK heading, mark its phrase boundaries, and
  use a no-break space (`\u00a0`) to keep a name like YouTube Music together.
- `assets/shots/<locale>/` are screenshots of the real interface, rendered by
  `TestSiteScreenshots` (`site_shots_test.go`) through MyGo's software renderer
  with a fictional catalogue: invented artists, titles and lyrics with
  generated covers. Re-render them after visible UI changes.
- Download buttons point at the releases page and are upgraded in the browser
  to the newest release that carries each platform's file, since a release
  does not always include every platform.
- `-base` sets the absolute URL used for canonical, `hreflang`, the sitemap and
  the social card (default `https://ytmgo.kyun.si/`); page links and assets are
  relative, so previews work from any address.
- The generator also writes Cloudflare's `_headers`: a Content-Security-Policy
  that allows only the site's own files plus the GitHub API, the usual
  hardening headers, and a week of caching for screenshots.
- The JavaScript URL includes a content hash, so a new deployment loads the
  updated script even when a visitor has an earlier version cached.

The root URL restores a language chosen in the switcher; without a valid saved
choice, it follows the first supported browser language. Redirects keep query
parameters and fragments. Direct links to a localized page stay in that language.

Run the language regression checks with Node.js (not required to build the site):

```sh
node --test site/tests/language.test.mjs
```

## Deploying on Cloudflare Pages

The site deploys through Cloudflare Pages' Git integration: Cloudflare builds
from this repository on every push to `main` that touches `site/`.

| Setting | Value |
| --- | --- |
| Production branch | `main` |
| Framework preset | None |
| Build command | `go run ./site/gen` |
| Build output directory | `site/dist` |
| Root directory | *(empty: the repository root)* |
| Environment variable | `GO_VERSION` = the `go` version in `go.mod` |
| Build watch paths | Include `site/*` |
| Custom domain | `ytmgo.kyun.si` |

The build needs only Go's standard library, so it downloads no modules and
runs nothing else from the repository.
