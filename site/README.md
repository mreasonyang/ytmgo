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
- `locales/<locale>.json` holds the copy. `go run ./site/gen` fails if a locale
  misses or adds a key, or if the template stops using one, so the four
  languages stay in step. Keys starting with `js.` are passed to `site.js`.
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

On the English page, a first-time visitor whose browser prefers Chinese or
Japanese is sent to that page; choosing a language in the switcher is
remembered and always wins.

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
