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
  the social card (default `https://mreasonyang.github.io/ytmgo/`); page links
  and assets are relative, so the site also works from any sub-path.

On the English page, a first-time visitor whose browser prefers Chinese or
Japanese is sent to that page; choosing a language in the switcher is
remembered and always wins.
