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
- `quickjs-wasi` 2.2.0: its npm metadata declares MIT, but its npm archive and
  release source do not include the wrapper notice. The wrapper's upstream
  notice was added later in
  https://github.com/vercel-labs/quickjs-wasi/blob/066475d399936cd40a38cabde6935d5f7b080dec/LICENSE.
  It is retained with this timing distinction; this does not independently
  establish license completeness for the earlier archive.
- The QuickJS-NG notice is from the exact submodule used by that 2.2.0 release,
  https://github.com/quickjs-ng/quickjs/blob/dec012362bd93876449f3ecff4f835b2eba89bab/LICENSE.
  The bundled mbedTLS notice is from
  https://github.com/vercel-labs/quickjs-wasi/blob/cc1fea4a6a4ac1d960e0db68d35e1459064a1a23/extensions/crypto/mbedtls/LICENSE.
- `proxy-agent-negotiate` 1.1.0: the npm manifest declares MIT but supplies no
  copyright/license text. Its source release also lacks a package notice.
  The supplied NOTICE records that gap and the exact release source. It is
  not a fabricated copyright grant or a claim of public-release readiness.

All remaining dependency licenses come directly from their integrity-locked
npm archives, including `decimal.js`'s British-spelled `LICENCE.md`. Node and
FFmpeg notices come from their verified source distributions. The complete
upstream provider client, GPL-3.0 license, and source reconstruction inputs are
retained in `vendor/bgutil`; they do not change the project's overall license.

The two notice gaps described above and the overall GPL distribution obligations
remain release review items. Preparing a local runtime is not public release.
