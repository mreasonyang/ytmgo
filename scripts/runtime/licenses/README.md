# Runtime notices

Original ytmgo code uses the root Apache-2.0 license. Third-party components and
adapted portions retain their original terms; see `THIRD_PARTY_NOTICES.md`.

The current runtime contains only the pinned YouTube.js 18.3.0 extraction code
(MIT), Meriyah 7.4.1 (ISC), BgUtils 4.0.3 (MIT), the native Rust closure including
QuickJS, and the minimal LGPL FFmpeg build. The three script notices here are
unchanged copies from the integrity-pinned package archives. The Go URL-transform
adapter derives from YouTube.js `getNsigProcessorFn` and retains its MIT notice.

The proof adapter follows the MIT BgUtils example at:
https://github.com/LuanRT/BgUtils/blob/f842f8a14435a35800ff469d315bb1a7e6d7a64a/examples/index-innertube.ts
Its original Copyright (c) 2024 LuanRT and complete permission notice are in
`bgutils-examples-MIT`.

`make licenses` collects current notices for both helpers, the actual imported
Go modules and scripts, plus the unchanged MPL source archives and FFmpeg source,
configuration and rebuild instructions. The complete `build/legal` tree is
copied into application distributions and verified before packaging.

Other notice files in this directory belong to historical Node/jsdom and
`proof-session.mjs` implementations. They are retained for provenance and are
not copied into current bundles. The removed GPL provider is not a current
component; historical commits retain their original licensing.
