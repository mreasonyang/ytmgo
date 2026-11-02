# System playlist artwork — 2026-11-01

Result: **PASS for the two reported system-cover cases** in the standalone
macOS arm64 candidate. The current installed app was preserved and reopened
after testing; this candidate was not installed over it or publicly released.

| Item | Upstream representation | Previous loss | Native candidate result |
| --- | --- | --- | --- |
| Liked Music | White thumbs-up on purple/pink gradient; public `gstatic.com/youtube/media/ytm/images/pbg/` PNG | Static image origin missing in parser and downloader allowlists | PASS in library/sidebar; matches official web image |
| Discover Mix | Generated image from `music.youtube.com/image/<opaque>?r=<opaque>` | Host absent and query fields rejected | PASS in library/sidebar and full collection header; generated Discover Mix graphic |

The official Liked Music web page and its displayed image were inspected in
Brave. Switching accounts was unnecessary for comparing system-cover handling.
For Discover Mix, an isolated diagnostic build reduced only the thumbnail
fields to host, path category, query-key names and acceptance booleans. Image
identifiers, option values, account credentials, headers and raw response
bodies were not written to evidence. No account was switched or added.

The client already parsed these entries as playlists; this was an image-source
compatibility gap. The repair accepts the verified static artwork directory
and the exact canonical generated-image endpoint. It preserves the upstream
generated-image request and sends no account Cookie or Authorization headers.
Image byte, decoded-pixel, queue, worker and cache limits remain unchanged.
No image is selected by matching the displayed title.

Focused artwork/catalogue race tests, final URL-boundary race tests and
`go vet ./...` in the isolated snapshot passed. App, nested-code and DMG
signatures/manifests were checked; the mounted DMG executable matches the UI
tested candidate. The Developer ID team is `2V4TX79LUZ`; this local candidate
is not notarized or published. Diagnostic code was removed before the final
candidate was built.

The candidate was built from base commit
`2a7abf90a95c0d75eb2e5911948dc0e3dad35456` plus only the four artwork/parser
source and test files listed in `final-candidate.json`. Unrelated concurrent
workspace changes were excluded. The installed app changed during the earlier
comparison phase, so it was not overwritten. Its then-current executable hash
was preserved across this standalone test, and its installed process was
reopened afterward.

Evidence:

- `final-candidate.json`: final source/binary/DMG hashes and clean-build status.
- `discover-source-metadata.json`: reduced upstream image metadata, with opaque values omitted.
- `dmg-verification.json`: mounted package signature and binary identity.
- `ui-validation.json`: actual native UI observations and installed-app restoration.

Native screenshots and accessibility snapshots are retained in this chat;
no screenshot files were exported. This is focused artwork acceptance, not a
new long-duration playback acceptance.
