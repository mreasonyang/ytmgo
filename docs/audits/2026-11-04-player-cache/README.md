# Public player cache repair — 2026-11-04

Source and native-runtime validation: PASS. This report does not claim that the
installed signed/notarized application has been upgraded.

## Problem and behavior

Previously, first playback in every process downloaded YouTube's public
`base.js` and extracted the signature/n transform. Account reconfiguration
also erased this account-independent result. The full script had the same
20-second request deadline as small API responses.

The resolver now persists one versioned, checksummed public transform under
`os.UserCacheDir()/ytmgo/public-player/current.json`. The record contains the
player ID, script profile, extractor schema, verification time and extracted
code/timestamp only. Account cookies, visitor data, proof tokens and media URLs
are excluded by construction. Downloading the full script and extracting it
still happen only in the existing bounded native helper/network boundaries.

- A cache checked within six hours is reused directly, including after restart.
- After six hours, a public `iframe_api` request checks the player ID. An
  unchanged ID refreshes the verification time without downloading/extracting.
- A changed ID, missing/corrupt/incompatible record, or decipher failure causes
  a fresh download on the next resolution. A failed download/extraction never
  overwrites the prior successful record.
- A transient version-check network/5xx failure may reuse a record checked less
  than seven days ago, with five minutes between retries in that process.
  Cancellation, TLS/redirect/authentication failures, malformed identifiers,
  and a known new version do not use that fallback.
- Public data survives account/proof resets; private session state and media
  leases retain their existing account-generation revocation behavior.
- Script download gets up to 60 seconds; version checking gets five seconds;
  ordinary requests remain at 20 seconds. Resolver work and the existing player
  load remain bounded at 90 seconds, and account switches/quit cancel them.
- Missing/unwritable cache storage degrades to in-memory reuse. Cache writes
  use a private temporary file, sync, and rename; incomplete files are rejected.
  File size, extracted-code size, schema, profile, ID, timestamp and checksum
  are checked before reuse. Bump the schema if the extractor contract changes.

## Validation

- `make check`: PASS, including full Go tests, native runtime tests, login and
  licensing tests, resource verification, `go vet`, and all four locales.
- `go test -race ./internal/streaming ./internal/player`: PASS.
- Regression cases cover restart reuse, account changes, same/new version,
  corrupt/oversized/incompatible records, failed refresh, stale fallback bounds,
  cancellation, unavailable storage, decipher invalidation and request budgets.
- Real public script + bundled QuickJS: initial download/extraction **4.901 s**;
  new resolver reading disk **2 ms**, **zero network requests and zero
  extractions**. The cached real transform also executed with a synthetic n
  input. Extracted code was **181,616 bytes**. These are JS-preparation timings,
  not end-to-end playback latency, and recreating a resolver is not a GUI restart
  test. No credentials, signed playback URLs, audio or account requests were used.
- A local macOS test bundle was built from the repaired source and its ad-hoc
  signature verified. It is not a notarized release. The installed application
  was observed playing and was left running without replacing it.

The first successful download still needs network access. Persistent caching
removes repeated downloads; it does not establish the cause of the previous
network slowdown or guarantee that a cold download always completes.

Reproduce the public-only live check explicitly:

```sh
YTMGO_TEST_NATIVE_HOST="$PWD/build/runtime/ytmgo-playback-host" \
YTMGO_TEST_PUBLIC_PLAYER_CACHE=1 \
go test ./internal/streaming -run '^TestPublicPlayerCacheRestart$' -count=1 -v
```
