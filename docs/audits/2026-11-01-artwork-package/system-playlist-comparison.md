# Official system-playlist artwork comparison — 2026-11-01

Scope: compare the official YouTube Music web UI in Brave with ytmgo's remaining
Liked Music / Discover Mix placeholders. The completed repair and native UI
acceptance are recorded in `../2026-11-01-system-playlist-artwork/README.md`.

## Confirmed: Liked Music

The official `https://music.youtube.com/playlist?list=LM` page shows a white
thumbs-up symbol on a purple-to-pink gradient square and labels the collection
as an automatic playlist. Opening that displayed image through the browser's
image context menu identifies this public resource:

`https://gstatic.com/youtube/media/ytm/images/pbg/liked-songs-delhi-1200.png`

The browser displayed it as a 1200×1200 PNG. It is a system-playlist cover,
not an album cover or the generic missing-image placeholder.

Two temporary, local execution probes used that observed public URL and the
existing catalogue renderer shape. Their output was:

```text
parsed_kind=playlist observed_official_system_cover_retained=false
observed_official_system_cover_download_allowed=false
```

The temporary probe files were removed after execution. These are defect
reproductions, not passing acceptance of the image behavior.

The current `internal/catalog/parse.go:safeArtwork` accepts `ytimg.com`,
`googleusercontent.com` and `ggpht.com` but discards this `gstatic.com` address.
The native downloader's `artwork.go:artworkURL` independently rejects the same
source. The item can therefore have the correct playlist kind while losing its
system cover before any image request is queued. Proxy selection and retry
cannot recover a URL that was discarded at that boundary.

## Confirmed: Discover Mix uses a different image service

The user correctly clarified that checking system-cover rendering does not
require switching accounts. No account was switched or added. Bounded metadata
from the app's normal catalogue client established that Discover Mix uses
`https://music.youtube.com/image/<opaque>?r=<opaque>`, with one `r` option. The
image identifier and option value were not retained in evidence. Both the
unrecognized host and blanket query rejection discarded this cover.

The final candidate preserves that upstream image request without rewriting
the opaque path or option. Its native library/sidebar and detail header now
show generated Discover Mix artwork: a blue/purple branded frame with image
material and the Discover Mix label. The image material can differ between
catalogue responses; the client does not replace it with an album cover.

The repair retains upstream system-playlist/mix artwork and its playlist
semantics, with narrow allowances for both the static directory and the exact
generated-image endpoint. It neither guesses an image from a title nor
substitutes an album cover. Both reported system covers passed native UI
acceptance in the clean, signed standalone candidate.
