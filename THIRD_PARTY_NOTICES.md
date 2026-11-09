# Third-party components

The root `LICENSE` applies to original ytmgo code, documentation and project
assets. Third-party components and copied or adapted portions retain their
own licenses and copyright notices, including when compiled or bundled into
an Apache-2.0 project. Generated `native/playback-host/assets/*.js` contain
third-party code; the root license does not relicense those portions.

## Components and attribution

| Component | License | Use and source |
| --- | --- | --- |
| Go runtime and golang.org/x modules | BSD-3-Clause, with applicable nested notices | [Go](https://go.dev/LICENSE), exact module versions in `go.mod` / `go.sum` |
| MyGo | MIT, with Apache-2.0 shader subcomponent notices | [MyGo](https://github.com/egoist/mygo), version resolved from `go.mod` |
| purego | Apache-2.0 | [purego](https://github.com/ebitengine/purego) |
| go-text/typesetting | Unlicense OR BSD-3-Clause, with applicable nested notices | [typesetting](https://github.com/go-text/typesetting) |
| YouTube.js 18.3.0 | MIT | [YouTube.js](https://github.com/LuanRT/YouTube.js); extraction algorithms and the adapted `getNsigProcessorFn` in `internal/streaming/format.go` |
| Meriyah 7.4.1 | ISC | [Meriyah](https://github.com/meriyah/meriyah); script parser |
| BgUtils 4.0.3 and its example | MIT | [BgUtils](https://github.com/LuanRT/BgUtils); native proof adapter; example Copyright (c) 2024 LuanRT, original notice in `scripts/runtime/licenses/bgutils-examples-MIT` |
| Rust dependencies, including Wry, Tao, rquickjs and QuickJS | Each dependency's original terms, commonly MIT / Apache-2.0 | Exact versions in each helper's `Cargo.lock`; generated inventories include the declared license expressions and upstream notices |
| option-ext, when selected by the target's Rust dependency graph | MPL-2.0 | [option-ext](https://crates.io/crates/option-ext); unchanged corresponding source accompanies the application |
| Bricolage Grotesque | SIL Open Font License 1.1 | [Bricolage](https://github.com/ateliertriay/bricolage); display titles. `resources/fonts/ytmgo-display.ttf` is a modified version: a static instance (weight 720, width 88, display optical size), Latin subset, OS/2 weight class 800. License text in `resources/fonts/OFL-BricolageGrotesque.txt` |
| Lucide icons, including icons derived from Feather | ISC; MIT for the Feather-derived icons | [Lucide](https://github.com/lucide-icons/lucide); the interface icons in `native_style.go` are adapted from Lucide (redrawn at a 1.8 px stroke). Notice in `resources/notices/lucide-LICENSE.txt` |
| FFmpeg 9.0.2 | LGPL-2.1-or-later for the configured build | [FFmpeg](https://ffmpeg.org/); separate executable, built without GPL or nonfree options; unchanged source, configuration and rebuild instructions accompany the application |

The JavaScript source versions and integrity checks are pinned in
`scripts/runtime/native/assets.lock.json`. Build-only tools such as esbuild
retain their upstream licenses; their executables are not shipped in the app.
System frameworks and separately installed WebView/GTK/Secret Service runtimes
remain subject to their vendors' terms and are not relicensed by this project.

Some Rust registry archives omit their workspace-level license. Original copies
from each archive's exact upstream VCS revision are retained under
`scripts/licensing/upstream/`; `upstream-notices.json` pins their source URLs,
revisions and checksums. The distribution collector checks those pins before
including the notices. All original license choices remain intact.

## Notices and corresponding source in distributions

`make build` and `make build-app` generate a target-specific `legal/` resource
directory before packaging. On macOS it is at
`<app>.app/Contents/Resources/legal/`; on Windows and Linux it is next to the
application executable. It contains:

- the project `LICENSE`, `NOTICE`, and this document;
- notices for the actual imported Go modules and both Rust helpers, including
  nested upstream notices and the Go runtime license;
- the original script dependency notices and BgUtils example attribution;
- exact, checksum-verified source archives for selected MPL components;
- the exact FFmpeg source archive, LGPL text, build configuration and a rebuild
  script. Run `python3 legal/ffmpeg/rebuild.py` in a native build environment
  with Python 3, a C compiler, `make`, and `sh` (MSYS2 UCRT64 on Windows).

The generated `manifest.json` records versions, original declared licenses,
source archive checksums and notice hashes. `make check-licenses` checks the
tree and its inputs; packaging rejects missing or changed legal resources.
When distributing a modified third-party component, also provide its modified
corresponding source under that component's original terms. Historical commits
and obsolete components keep their original licensing.
