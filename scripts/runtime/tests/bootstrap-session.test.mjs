import test from 'node:test';
import assert from 'node:assert/strict';
import { mkdtemp, readFile, writeFile, rm, realpath } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { pathToFileURL } from 'node:url';
import { performance } from 'node:perf_hooks';
import { BootstrapSession } from '../bootstrap-session.mjs';
import { MemoryCache, MAX_CACHE_BYTES, createAnonymousSession, validateAnonymousRequest } from '../sdk-session.mjs';

const fixed = category => error => error instanceof Error && error.message === category && error.safeCategory === category && error.cause === undefined;
const tick = () => new Promise(resolve => setImmediate(resolve));
const delay = ms => new Promise(resolve => setTimeout(resolve, ms));
const pidOf = result => new DataView(result.entries[0][1]).getUint32(0);
const gone = pid => assert.throws(() => process.kill(pid, 0), error => error.code === 'ESRCH');
const source = await readFile(new URL('./fixtures/bootstrap-worker-fixture.mjs', import.meta.url), 'utf8');

async function fixture(mode = 'normal', options = {}) {
  // Node permissions resolve the entry point before checking its read scope.
  const directory = await realpath(await mkdtemp(join(tmpdir(), 'ytmgo-bootstrap-test-')));
  const path = join(directory, 'worker.mjs');
  const setMode = value => writeFile(path, source + `\nawait run(${JSON.stringify(value)});\n`);
  await setMode(mode);
  const session = new BootstrapSession({ workerURL: pathToFileURL(path), ...options });
  return { session, setMode, async close() { await session.close(); await rm(directory, { recursive: true, force: true }); } };
}

test('cache snapshots and seeds are opaque, independent and bounded', async () => {
  const cache = new MemoryCache();
  const value = new Uint8Array([1, 2, 3]);
  await cache.set('a', value); value[0] = 9;
  const snapshot = cache.snapshot();
  assert.ok(snapshot[0][1] instanceof ArrayBuffer);
  new Uint8Array(snapshot[0][1])[0] = 8;
  assert.equal(new Uint8Array(await cache.get('a'))[0], 1);
  const seeded = new MemoryCache(cache.snapshot(), { strict: true });
  await cache.remove('a');
  assert.equal(new Uint8Array(await seeded.get('a'))[0], 1);
  await assert.rejects(seeded.get('missing'), fixed('bootstrap_cache_miss'));
  assert.throws(() => seeded.seed([['a', new ArrayBuffer(1)], ['a', new ArrayBuffer(1)]]), fixed('cache_limit'));
  assert.equal(new Uint8Array(await seeded.get('a'))[0], 1); // failed seed is atomic
  for (const entries of [Array.from({ length: 5 }, (_, i) => [String(i), new ArrayBuffer(1)]),
    [['a', new ArrayBuffer(MAX_CACHE_BYTES)], ['b', new ArrayBuffer(1)]],
    [['a', new Uint8Array(1)]], [['', new ArrayBuffer(1)]], [['x'.repeat(513), new ArrayBuffer(1)]]]) {
    assert.throws(() => new MemoryCache(entries), fixed('cache_limit'));
  }
  await assert.rejects(cache.set('large', new ArrayBuffer(MAX_CACHE_BYTES + 1)), fixed('cache_limit'));
  for (const key of ['a', 'b', 'c', 'd', 'e']) await cache.set(key, new Uint8Array(1));
  assert.equal(cache.size, 4); assert.equal(cache.byteLength, 4); assert.equal(await cache.get('a'), undefined);
});

test('session arguments reject invalid input before lazy SDK initialization', async () => {
  await assert.rejects(createAnonymousSession(), fixed('init_failed'));
  await assert.rejects(createAnonymousSession({ fetch() {}, playerID: 'invalid id' }), fixed('init_failed'));
  assert.throws(() => validateAnonymousRequest(new Request('https://example.com/')), fixed('unexpected_request_host'));
  assert.throws(() => validateAnonymousRequest(new Request('https://www.youtube.com/', { headers: { authorization: 'fixture' } })), fixed('unexpected_authentication'));
});

test('successful binary IPC returns only after reaping and each explicit warm uses one new child', async () => {
  const f = await fixture('late_exit');
  try {
    let settled = false;
    const started = performance.now();
    const pending = f.session.warm().then(result => { settled = true; return result; });
    await delay(80); assert.equal(settled, false);
    const first = await pending;
    assert.ok(performance.now() - started >= 200);
    assert.deepEqual(Object.keys(first).sort(), ['entries', 'fetchCount', 'playerID']);
    assert.equal(first.playerID, 'fixture_player'); assert.equal(first.fetchCount, 3);
    gone(pidOf(first));
    await f.setMode('normal');
    const second = await f.session.warm();
    gone(pidOf(second)); assert.notEqual(pidOf(first), pidOf(second));
  } finally { await f.close(); }
});

test('binary bootstrap failures preserve only validated metadata and reject malformed fields', async () => {
  const f = await fixture('diagnostic_valid');
  try {
    await assert.rejects(f.session.warm(), error => fixed('bootstrap_redirect')(error) &&
      error.safeBootstrapStep === 'session_data' && error.safeHTTPStatus === 302 &&
      error.safeRedirect === 'google_challenge' && error.safeMIME === 'html');
    for (const mode of ['diagnostic_step', 'diagnostic_status_type', 'diagnostic_status_range', 'diagnostic_status_fraction',
      'diagnostic_redirect', 'diagnostic_mime', 'diagnostic_null', 'diagnostic_extra']) {
      await f.setMode(mode);
      await assert.rejects(f.session.warm(), error => fixed('init_failed')(error) && error.safeBootstrapStep === undefined && error.safeHTTPStatus === undefined);
    }
    await f.setMode('normal');
    const result = await f.session.warm();
    assert.deepEqual(Object.keys(result).sort(), ['entries', 'fetchCount', 'playerID']); gone(pidOf(result));
  } finally { await f.close(); }
});

test('concurrent warm is rejected without disturbing the active child', async () => {
  const f = await fixture('late_exit');
  try {
    const pending = f.session.warm();
    await assert.rejects(f.session.warm(), fixed('init_failed'));
    gone(pidOf(await pending));
  } finally { await f.close(); }
});

test('cancellation reaps work before rejecting and permits a later explicit warm', async () => {
  const f = await fixture('reply_hold');
  try {
    await assert.rejects(f.session.warm(AbortSignal.abort()), fixed('request_cancelled'));
    const abort = new AbortController();
    const pending = assert.rejects(f.session.warm(abort.signal), fixed('request_cancelled'));
    await delay(50); abort.abort(); await pending;
    await f.setMode('normal');
    gone(pidOf(await f.session.warm()));
  } finally { await f.close(); }
});

test('timeout terminates a child even after a complete reply and never publishes late success', async () => {
  // Include process startup under concurrent DOM tests in the budget. The held
  // reply never exits on its own, so timeout/reaping remains the assertion.
  const f = await fixture('reply_hold', { timeoutMS: 1000 });
  try {
    await assert.rejects(f.session.warm(), fixed('request_timeout'));
    await f.setMode('normal');
    gone(pidOf(await f.session.warm()));
  } finally { await f.close(); }
});

test('close is idempotent, waits for pending reap, and permanently rejects future warm', async () => {
  const f = await fixture('hold');
  try {
    const pending = assert.rejects(f.session.warm(), fixed('runtime_closed'));
    await tick();
    const closing = f.session.close(); assert.equal(f.session.close(), closing);
    await closing; await pending;
    await assert.rejects(f.session.warm(), fixed('runtime_closed'));
  } finally { await f.close(); }
});

test('invalid frames, trailing replies and nonzero exit never become success or expose raw diagnostics', async () => {
  const f = await fixture();
  try {
    for (const [mode, category] of [
      ['empty', 'init_failed'], ['bad_serialization', 'init_failed'], ['bad_schema', 'init_failed'],
      ['bad_player', 'init_failed'], ['bad_count', 'init_failed'], ['trailing', 'init_failed'],
      ['truncated', 'init_failed'], ['valid_then_failure', 'init_failed'], ['nonzero_after_reply', 'init_failed'],
      ['unknown_error', 'init_failed'], ['reported_error', 'cache_limit'], ['bad_entries', 'cache_limit'],
      ['duplicate_entries', 'cache_limit'], ['large_cache', 'cache_limit'], ['oversize_header', 'cache_limit'],
    ]) {
      await f.setMode(mode);
      await assert.rejects(f.session.warm(), fixed(category), mode);
    }
    await f.setMode('normal'); gone(pidOf(await f.session.warm()));
  } finally { await f.close(); }
});

test('fragmented output and private stderr work with read-only network-only child permissions', async () => {
  const f = await fixture();
  try {
    for (const mode of ['fragmented', 'stderr', 'permissions']) {
      await f.setMode(mode); gone(pidOf(await f.session.warm()));
    }
  } finally { await f.close(); }
});

test('spawn errors and invalid constructor options stay fixed and close remains safe', async () => {
  const session = new BootstrapSession({ nodePath: '/nonexistent/ytmgo-bootstrap-node' });
  try { await assert.rejects(session.warm(), fixed('init_failed')); }
  finally { await session.close(); }
  for (const options of [{ timeoutMS: 0 }, { timeoutMS: 30001 }, { workerURL: 'https://example.com/worker.mjs' }]) {
    assert.throws(() => new BootstrapSession(options), fixed('init_failed'));
  }
});
