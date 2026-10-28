import test from 'node:test';
import assert from 'node:assert/strict';
import http from 'node:http';
import { StreamBridge, parseByteRange, STREAM_HEADERS, STREAM_ERROR_HEADER } from '../stream-bridge.mjs';

const media = (length = 160_000, duration = 7200) => ({
  url: `https://fixture.googlevideo.com/media?expire=${Math.floor(Date.now() / 1000) + 10800}&cpn=fixture`,
  length, duration, contentType: 'audio/webm',
});
const waitFor = async predicate => {
  const deadline = Date.now() + 2000;
  while (!predicate()) { assert.ok(Date.now() < deadline, 'condition completed within bound'); await new Promise(resolve => setTimeout(resolve, 2)); }
};

test('HTTP byte ranges cover suffix, clamping and unsatisfiable inputs', () => {
  assert.deepEqual(parseByteRange(undefined, 100), { start: 0, end: 99, partial: false });
  assert.deepEqual(parseByteRange('bytes=10-19', 100), { start: 10, end: 19, partial: true });
  assert.deepEqual(parseByteRange('bytes=90-999', 100), { start: 90, end: 99, partial: true });
  assert.deepEqual(parseByteRange('bytes=-10', 100), { start: 90, end: 99, partial: true });
  for (const value of ['bytes=100-', 'bytes=20-10', 'bytes=-0', 'bytes=-', 'bytes=0-1,4-5', 'bytes=9007199254740993-', 'items=0-1']) assert.equal(parseByteRange(value, 100), null);
});

test('four exact capabilities coexist; release is idempotent and never evicts another', async t => {
  let requests = 0;
  const bridge = new StreamBridge({ fetch: async () => { requests++; throw new Error('unexpected'); } });
  t.after(() => bridge.close());
  const urls = [];
  for (let i = 0; i < 4; i++) urls.push(await bridge.publish(media()));
  await assert.rejects(() => bridge.publish(media()), e => e.safeCategory === 'capacity_limit');
  for (const url of urls) assert.equal((await fetch(url, { method: 'HEAD' })).status, 200);
  assert.equal(requests, 0);
  assert.equal(bridge.release(urls[0]), true);
  assert.equal(bridge.release(urls[0]), false);
  assert.equal((await fetch(urls[0], { method: 'HEAD' })).status, 404);
  assert.equal((await fetch(urls[1], { method: 'HEAD' })).status, 200);
  await bridge.publish(media());
  assert.equal(bridge.size, 4);
  assert.throws(() => bridge.release('https://example.com/'), e => e.safeCategory === 'invalid_capability');
});

test('local range/HEAD metadata and exact 64 KiB remote URL chunks', async t => {
  const ranges = [];
  const bridge = new StreamBridge({ fetch: async (url, options) => {
    assert.equal(url.hostname, 'fixture.googlevideo.com');
    assert.equal(url.searchParams.get('cpn'), 'fixture');
    assert.equal(options.credentials, 'omit');
    assert.equal(options.headers.has('authorization'), false);
    assert.equal(options.headers.has('cookie'), false);
    assert.equal(options.headers.has('range'), false);
    assert.equal(options.headers.get('referer'), STREAM_HEADERS.referer);
    const [start, end] = url.searchParams.get('range').split('-').map(Number);
    ranges.push([start, end]);
    assert.ok(end - start + 1 <= 65536);
    const bytes = Uint8Array.from({ length: end - start + 1 }, (_, i) => (start + i) % 251);
    return new Response(bytes, { headers: { 'Content-Length': String(bytes.length) } });
  } });
  t.after(() => bridge.close());
  const url = await bridge.publish(media());
  const head = await fetch(url, { method: 'HEAD', headers: { Range: 'bytes=-10' } });
  assert.equal(head.status, 206);
  assert.equal(head.headers.get('content-range'), 'bytes 159990-159999/160000');
  assert.equal(ranges.length, 0);
  const response = await fetch(url, { headers: { Range: 'bytes=10-150009' } });
  assert.equal(response.status, 206);
  assert.equal(response.headers.get('content-length'), '150000');
  const bytes = new Uint8Array(await response.arrayBuffer());
  assert.equal(bytes.length, 150000);
  assert.equal(bytes[0], 10);
  assert.equal(bytes.at(-1), 150009 % 251);
  assert.deepEqual(ranges, [[10, 65545], [65546, 131081], [131082, 150009]]);
  assert.equal((await fetch(url, { headers: { Range: 'bytes=160000-' } })).status, 416);
  assert.equal((await fetch(url, { method: 'POST' })).status, 405);
});

test('upstream ignores range, malformed length and malformed 206 are rejected before forwarding', async t => {
  for (const headers of [{}, { 'Content-Length': '65537' }, { 'Content-Length': '65536', 'Content-Range': 'bytes 0-65535/999999' }]) {
    const bridge = new StreamBridge({ fetch: async () => new Response(new Uint8Array(65536), { headers }) });
    const url = await bridge.publish(media());
    assert.equal((await fetch(url)).status, 502);
    await bridge.close();
  }
  const bridge = new StreamBridge({ fetch: async () => new Response(new Uint8Array(65536), { status: 206, headers: { 'Content-Length': '65536' } }) });
  t.after(() => bridge.close());
  assert.equal((await fetch(await bridge.publish(media()))).status, 502);
});

test('four consumers are supported and release cancels blocked upstream headers', async t => {
  let entered = 0, aborted = 0;
  const bridge = new StreamBridge({ fetch: async (_url, { signal }) => {
    entered++;
    return new Promise((_resolve, reject) => signal.addEventListener('abort', () => { aborted++; reject(new Error('cancelled')); }, { once: true }));
  } });
  t.after(() => bridge.close());
  const urls = [], pending = [];
  for (let i = 0; i < 4; i++) { const url = await bridge.publish(media()); urls.push(url); pending.push(fetch(url)); }
  await waitFor(() => entered === 4);
  assert.equal(bridge.activeRequests, 4);
  assert.equal((await fetch(urls[0])).status, 503);
  bridge.release(urls[0]);
  assert.equal((await pending[0]).status, 410);
  await waitFor(() => aborted === 1);
  assert.equal((await fetch(urls[1], { method: 'HEAD' })).status, 200);
  const remaining = Promise.allSettled(pending.slice(1));
  await bridge.close();
  await remaining;
  assert.equal(aborted, 4);
  assert.equal(bridge.activeRequests, 0);
  assert.equal(bridge.size, 0);
  await bridge.close();
});

test('slow downstream applies backpressure and close reclaims the blocked stream', async t => {
  let requests = 0;
  const bridge = new StreamBridge({ fetch: async (url, { signal }) => {
    requests++;
    if (signal.aborted) throw new Error('cancelled');
    const [a, b] = url.searchParams.get('range').split('-').map(Number);
    return new Response(new Uint8Array(b - a + 1), { headers: { 'Content-Length': String(b - a + 1) } });
  } });
  t.after(() => bridge.close());
  const url = await bridge.publish(media(64 * 1024 * 1024));
  const received = new Promise(resolve => {
    const request = http.get(url, response => { response.pause(); resolve({ request, response }); });
    request.on('error', () => {});
  });
  const connection = await received;
  await new Promise(resolve => setTimeout(resolve, 40));
  assert.ok(requests < 1024, 'does not buffer the whole media while the consumer is paused');
  await bridge.close();
  connection.response.destroy(); connection.request.destroy();
  assert.equal(bridge.activeRequests, 0);
});

test('unknown media expiry is refused; a two-hour episode is accepted', async () => {
  const bridge = new StreamBridge();
  await assert.rejects(() => bridge.publish({ ...media(), url: 'https://fixture.googlevideo.com/media' }), e => e.safeCategory === 'media_expired');
  const url = await bridge.publish(media());
  assert.equal((await fetch(url, { method: 'HEAD' })).status, 200);
  await bridge.close();
});

test('post-header failures expose only a bounded fixed HEAD status without replaying upstream', async () => {
  for (const [mode, category] of [
    ['denied', 'upstream_access_denied'], ['timeout', 'upstream_timeout'],
    ['truncated', 'upstream_truncated'], ['range', 'upstream_range_mismatch'],
    ['connection', 'upstream_connection_failed'],
  ]) {
    let calls = 0;
    const bridge = new StreamBridge({ idleTimeoutMS: 50, fetch: async (_url, { signal }) => {
      calls++;
      if (calls === 1) return new Response(new Uint8Array(65536), { headers: { 'Content-Length': '65536' } });
      await new Promise(resolve => setTimeout(resolve, 5));
      if (mode === 'denied') return new Response('synthetic-private-response', { status: 403 });
      if (mode === 'timeout') return new Promise((_resolve, reject) => signal.addEventListener('abort', () => reject(new Error('synthetic-private-timeout')), { once: true }));
      if (mode === 'truncated') return new Response(new Uint8Array(16), { headers: { 'Content-Length': '65536' } });
      if (mode === 'range') return new Response(new Uint8Array(16), { headers: { 'Content-Length': '16' } });
      throw new Error('synthetic-private-connection');
    } });
    try {
      const url = await bridge.publish(media(131072));
      await assert.rejects(async () => { const response = await fetch(url); await response.arrayBuffer(); });
      await waitFor(() => bridge.activeRequests === 0);
      const head = await fetch(url, { method: 'HEAD' });
      assert.equal(head.status, 200); assert.equal(head.headers.get('content-length'), '131072');
      assert.equal(head.headers.get(STREAM_ERROR_HEADER), category);
      assert.equal(calls, 2);
      assert.equal(bridge.release(url), true);
      const gone = await fetch(url, { method: 'HEAD' });
      assert.equal(gone.status, 404); assert.equal(gone.headers.get(STREAM_ERROR_HEADER), null);
    } finally { await bridge.close(); }
  }
});
