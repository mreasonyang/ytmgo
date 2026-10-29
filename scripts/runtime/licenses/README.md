# Vendored dependency notices

`@bufbuild/protobuf` 2.16.0 declares `(Apache-2.0 AND BSD-3-Clause)` but omits
the root Apache license from its npm archive. The build copies both notices.

- Apache-2.0: https://github.com/bufbuild/protobuf-es/blob/b3b232f7f466eda27c66e8f63c82f930578b6cc6/LICENSE
  (the `v2.16.0` tag).
- BSD-3-Clause: the leading notice in the integrity-locked package's
  `dist/esm/wire/varint.js`, retaining its Google copyright and conditions.

The following npm archives also omit a standalone notice. These copies keep
their origin explicit:

- `agent-base` 6.0.2 and `https-proxy-agent` 5.0.1: the complete MIT license
  section from each integrity-locked npm README, including its copyright.
- `saxes` 6.0.0: the upstream license at the package's published gitHead,
  https://github.com/lddubeau/saxes/blob/211fa0ebec9b628affc09219199639887174bfc3/LICENSE.

All remaining dependency licenses come directly from their integrity-locked
npm archives, including `decimal.js`'s British-spelled `LICENCE.md`. Node and
FFmpeg notices come from their verified source distributions.

## Independent proof session

`proof-session.mjs` is licensed under Apache-2.0; its license text is included
as `proof-session-APACHE-2.0`. Its integration uses the public MIT BgUtils API
and the MIT example at this exact revision:

- https://github.com/LuanRT/BgUtils/blob/f842f8a14435a35800ff469d315bb1a7e6d7a64a/examples/index-innertube.ts
- https://github.com/LuanRT/BgUtils/blob/f842f8a14435a35800ff469d315bb1a7e6d7a64a/LICENSE

The example's attribution, Copyright (c) 2024 LuanRT, and MIT permission notice
are preserved in `bgutils-examples-MIT`. The runtime library itself remains
pinned to `bgutils-js` 4.0.3; its MIT notice is also copied from the verified
npm package. The new session does not copy or translate the removed GPL
provider implementation.

The GPL provider's source, compiled output, source map and reconstruction inputs
are removed from the current source tree and build. Incremental preparation
removes its previous vendor directory and notices. The unused `proxy-agent`
dependency was removed as well, including the transitive `quickjs-wasi` and
`proxy-agent-negotiate` packages that previously had notice gaps.

These changes concern the proof implementation and its dependencies. They do
not relicense other project files or third-party components. FFmpeg remains
LGPL-2.1-or-later with its own distribution obligations. Historical Git commits
containing the GPL provider retain their original licensing.
