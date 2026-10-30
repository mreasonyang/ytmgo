# Resilience acceptance — in progress

Target: macOS 27.0.1 arm64, installed ytmgo 0.5.2; recovery candidate 0.5.3.
This follow-up closes the previously omitted real network, sleep/wake and
multi-hour playback checks. It does not replace the scope and five-finding
results in [the earlier UI/UX report](../2026-10-30-uiux-fixes/README.md).

## Confirmed F06: sustained playback after sleep — FAIL on 0.5.2

The operator ran the reviewed one-shot root helper locally. It scheduled and
verified one wake event, then requested real system sleep. The kernel sleep and
wake counters changed; actual sleep lasted 46 seconds. The event was consumed;
no unrelated schedule or permanent power setting was changed. The operator
heard automatic playback immediately after waking and unlocking.

At 13:24:28 UTC, six seconds after wake, the closed audio diagnostic recorded
`audio_stream_failed`. A later native observation showed stopped playback at
57.2935 seconds and Retry. Initial audible recovery therefore did not establish
sustained recovery. See the `052-power-*`, `052-sleep-*` receipts.

The candidate retires the sleeping decoder and media leases, preserves the
position and playing/paused intent, and resolves a fresh stream on wake. Bounded
transport retries accommodate network restoration. Newer user actions and account
identity changes supersede an old wake. Video readers also retire during sleep.
The local full race run passed 856 named tests/subtests with 22 opt-in skips.
After the final preexisting-error guard, all 15 affected power race tests and vet
passed. Runtime (14+5+83), login (10), release (22) and translation checks passed.
Final-commit CI and installed 0.5.3 acceptance are pending.

## Real network disconnect/reconnect — PASS on 0.5.2 with manual retries

The active Wi-Fi interface en0 was physically disabled. An independent detached
watchdog had first proven that it could restore Wi-Fi after its parent exited.
The trial restored Wi-Fi after about 42 observed seconds (45-second watchdog).
Fresh TLS-verified HTTPS requests succeeded before and after the outage and
failed while offline. Application search and audio requests failed offline;
public search returned 27 results after retries, and same-track audio Retry
restored an advancing clock. Catalog recovery was not instantaneous on its first
retry. This is manual recovery, not automatic continuation. See `052-network-*`.

The initial TCP-only probe was rejected as Internet evidence: a local proxy
accepted TCP while Wi-Fi was off. A second, fresh TLS/HTTP trial supplies the
end-to-end outage evidence. No proxy configuration was read or changed.

## Two-hour steady playback — pending

The initial process sampling covers preparation and induced failures. It does
not count toward the required 7,200 continuous seconds on the repaired package.
The steady trial will use full responsibility/descendant process accounting,
explicit sample coverage, playback checkpoints and a fixed installed hash.
No claim of general leak freedom follows from a bounded run.
