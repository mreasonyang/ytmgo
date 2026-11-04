# Repeated loading repair — 2026-11-04

Source, race tests, public catalog checks and local macOS packaging: PASS.
The installed application was not replaced. This is not signed/notarized-release
or real-account GUI acceptance evidence.

## Resulting behavior

1. An unchanged account refresh preserves the next-track preparation. Focus
   reuses successful account verification for five minutes; failed focus checks
   have a 30-second retry interval. The existing periodic check remains. An
   actual account/membership change still performs the previous cancellation
   and lease-revocation behavior. Explicit account actions remain immediate.
2. Account-avatar requests join an existing request and reuse a successful
   result for 30 minutes. Account reset clears that freshness and display data.
3. Queue, selected-track metadata and lyrics share the same in-flight `next`
   request. A completed queue response is reused for 30 seconds.
4. Lyrics text/cues, including the anonymous timing recovery result, are cached
   in the selected account/locale for ten minutes. Missing lyrics are cached for
   only 30 seconds. Transient failures are not cached. Anonymous recovery still
   starts from the original video ID and never receives private account headers,
   visitor data or account-scoped browse references.
5. Home, Explore, search, library and browse pages reuse responses for two
   minutes. Explicit Reload/Retry bypasses completed cache entries. A missing
   continuation handle forces a fresh page. Back/Forward behavior is unchanged.
6. Public proof interpreters can survive proof-environment resets and process
   restart. Only an anonymous Google interpreter URL with no query and a
   JavaScript response explicitly marked public/fresh is admitted. Cache-Control,
   Age, Date, Vary and Set-Cookie are respected through the same conservative
   freshness policy as public artwork. The cache holds one interpreter, bounded
   at 4 MiB and at most 24 hours. URL changes, expiry, corruption or explicit
   interpreter failure cause a new download. Challenge pages, programs, proof
   tokens, session validation and media URLs are never cached here.

## Boundaries

Catalog caches are memory-only and scoped to the current Client. They clear and
cancel old work when the account or language changes. A late result cannot
populate the new response generation. A successful like/unlike invalidates
queue/page metadata so rating state cannot be served from the old cache.

Each cache retains at most 64 entries, with conservative metadata accounting
budgets of 8 MiB for pages, 4 MiB for queues and 2 MiB for lyrics. These are
retained-cache accounting limits, not a bound on total process memory. Expired
entries are pruned on use and the oldest-used entries are evicted as necessary.
Oversized results can be returned without retention. Mutable slices are cloned
for consumers. Each cache allows at most 32 concurrent distinct requests.

Coalesced work has a one-minute overall limit in addition to existing per-HTTP
deadlines. Each waiter can cancel independently; the final departing waiter
cancels the shared request. No canceled or obsolete worker can reinsert its
result. There is no persistence of authenticated catalog responses.

## Validation

- `make check`: PASS (full Go tests, native runtime, login, licensing, `go vet`,
  locale validation).
- `go test -race ./internal/catalog .`: PASS, including full application tests.
- Final targeted race checks across catalog, streaming and desktop: PASS.
- Three simultaneous same-track consumers: **one** `next` request, previously
  three in the isolated audit; one lyrics browse request.
- Signed-in plain lyrics plus anonymous timing fixture: first request sequence
  uses four requests; replay uses **zero**, previously three additional
  requests. Real cue arrays are retained and caller mutations cannot alter them.
- Real unauthenticated YouTube search: first search **one** request; repeated
  search **zero**, 32 metadata items returned. Real lyrics lookup: first **two**
  requests; repeated lookup **zero**. This live sample returned no timed cues;
  timed-result reuse and account isolation are covered by synthetic fixtures.
- Account/locale changes, late replies, per-waiter cancellation, last-waiter
  cancellation, cache budgets, expiry, explicit refresh, rating invalidation,
  public HTTP freshness, script corruption and interpreter invalidation: PASS.
- Production-mode MyGo macOS arm64 bundle built to an isolated output directory;
  `codesign --verify --deep --strict` passed its ad-hoc signature. No application
  version bump, release upload, installed-app replacement or notarization.

Public metadata check (explicit opt-in, no credentials or audio):

```sh
YTMGO_TEST_PUBLIC_CATALOG_CACHE=1 \
go test ./internal/catalog -run '^TestLiveCatalogCache$' -v -count=1
```
