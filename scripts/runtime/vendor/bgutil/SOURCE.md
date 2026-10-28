# Opaque upstream attestation client

Upstream: [Brainicism/bgutil-ytdlp-pot-provider](https://github.com/Brainicism/bgutil-ytdlp-pot-provider)

Version: **2.0.2**

Source commit: **26475e9d3665b972c4aeb83b4dbaafcf64b88438**

License: **GPL-3.0**, supplied in `LICENSE`. This notice does not change the
project-wide license or establish public-release readiness.

`src/session_manager.ts` is the complete, unmodified upstream standard client.
`build/session_manager.js` and its source map are the unmodified output of
TypeScript **6.0.3**, using the upstream compiler configuration. The protocol
configuration remains inside this opaque supplier module. It is not extracted,
displayed, separately configured, or copied into application code.

The upstream package manifest, lockfile and compiler configuration are retained
as `upstream-package.json`, `upstream-package-lock.json` and
`upstream-tsconfig.json`. To reproduce the compiled module in a separate build
directory, restore these three names to `package.json`, `package-lock.json`, and
`tsconfig.json`, keep `src/session_manager.ts`, install the locked dependencies
with installation scripts disabled, and run:

```sh
node node_modules/typescript/bin/tsc --project tsconfig.json --outDir build
```

The source map resolves to `../src/session_manager.ts`. Runtime ESM semantics
come from the surrounding `runtime/package.json` (`type: module`). No upstream
HTTP server, command-line entry point, disk token-cache entry point, or yt-dlp
plugin is shipped or executed.

Runtime dependency resolution intentionally uses the enclosing application's
single, integrity-locked dependency graph: YouTube.js **18.3.0**, BgUtils
**4.0.3**, JSDOM **29.1.1**, Axios **1.20.0**, and proxy-agent **8.0.2**. The
upstream reconstruction lock records its original dependency graph separately;
it is not installed inside this vendor directory. JSDOM's optional native
canvas peer is omitted, matching the validated diagnostic's unavailable native
canvas binding. No provider source or compiled method was edited for these
dependency bindings.

This is a third-party dependency, not an additional application playback
backend. The application must invoke it only through its bounded child-process
boundary, with a minimal environment, no persistent token cache, private IPC,
fixed diagnostics, and cancellation that terminates and reaps the child.

Packaging copies this entire directory, including the source, source map,
upstream build inputs, license, and notice into the application runtime.
