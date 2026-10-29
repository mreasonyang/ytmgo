import test from 'node:test';
import assert from 'node:assert/strict';
import http from 'node:http';
import { setTimeout as delay } from 'node:timers/promises';
import { StreamBridge } from '../stream-bridge.mjs';

const KiB = 1024;
const media = (length, contentType = 'video/mp4') => ({
  url: `https://fixture.googlevideo.com/media?expire=${Math.floor(Date.now() / 1000) + 10800}&cpn=fixture`,
  length, duration: 120, contentType,
});
const rangeOf = url => url.searchParams.get('range').split('-').map(Number);
const bytesAt = (start, length) => Uint8Array.from({ length }, (_, index) => (start + index) % 251);
const waitFor = async predicate => {
  const end = Date.now() + 2000;
  while (!predicate()) {
    assert.ok(Date.now() < end, 'bounded cleanup completed');
    await delay(5);
  }
};

test('video ranges cross 512 KiB boundaries and clamp the final request exactly', async t => {
  const ranges = [];
  const length = 2 * 512 * KiB + 777;
  const bridge = new StreamBridge({ fetch: async url => {
    const [start, end] = rangeOf(url);
    ranges.push([start, end]);
    assert.ok(end - start + 1 <= 512 * KiB, 'video range remains bounded');
    assert.ok(start >= 0 && end < length, 'remote range stays within media');
    return new Response(bytesAt(start, end - start + 1), { status: 206, headers: {
      'Content-Length': String(end - start + 1), 'Content-Range': `bytes ${start}-${end}/${length}`,
    } });
  } });
  t.after(() => bridge.close());
  const url = await bridge.publish(media(length));
  const head = await fetch(url, { method: 'HEAD', headers: { Range: 'bytes=-777' } });
  assert.equal(head.headers.get('content-range'), `bytes ${length - 777}-${length - 1}/${length}`);
  assert.equal(ranges.length, 0, 'HEAD never fetches remote media');

  const response = await fetch(url, { headers: { Range: `bytes=31-${length + 999}` } });
  assert.equal(response.status, 206);
  assert.equal(response.headers.get('content-length'), String(length - 31));
  assert.equal(response.headers.get('content-range'), `bytes 31-${length - 1}/${length}`);
  const actual = Buffer.from(await response.arrayBuffer());
  assert.ok(actual.equals(Buffer.from(bytesAt(31, length - 31))), 'all cross-range bytes match');
  assert.deepEqual(ranges, [[31, 524318], [524319, 1048606], [1048607, length - 1]]);

  ranges.length = 0;
  const tail = await fetch(url, { headers: { Range: 'bytes=-777' } });
  assert.ok(Buffer.from(await tail.arrayBuffer()).equals(Buffer.from(bytesAt(length - 777, 777))), 'suffix bytes match');
  assert.deepEqual(ranges, [[length - 777, length - 1]], 'tail does not round up to a full chunk');
});

test('both audio formats retain 64 KiB sequential ranges', async t => {
  for (const contentType of ['audio/mp4', 'audio/webm']) {
    const ranges = [];
    const bridge = new StreamBridge({ fetch: async url => {
      const [start, end] = rangeOf(url);
      ranges.push([start, end]);
      return new Response(new Uint8Array(end - start + 1), { headers: { 'Content-Length': String(end - start + 1) } });
    } });
    t.after(() => bridge.close());
    const response = await fetch(await bridge.publish(media(2 * 64 * KiB + 7, contentType)));
    assert.equal((await response.arrayBuffer()).byteLength, 2 * 64 * KiB + 7);
    assert.deepEqual(ranges, [[0, 65535], [65536, 131071], [131072, 131078]]);
  }
});

test('paused video consumer applies backpressure without parallel prefetch and disconnect cancels it', async t => {
  let requests = 0, produced = 0, active = 0, maximumActive = 0, maximumWrite = 0, aborted = 0;
  const originalWrite = http.ServerResponse.prototype.write;
  http.ServerResponse.prototype.write = function (chunk, ...args) {
    maximumWrite = Math.max(maximumWrite, Buffer.byteLength(chunk));
    return originalWrite.call(this, chunk, ...args);
  };
  t.after(() => { http.ServerResponse.prototype.write = originalWrite; });
  const bridge = new StreamBridge({ fetch: async (url, { signal }) => {
    requests++;
    active++;
    maximumActive = Math.max(maximumActive, active);
    const [start, end] = rangeOf(url);
    let remaining = end - start + 1;
    let finished = false;
    const finish = () => { if (!finished) { finished = true; active--; } };
    signal.addEventListener('abort', () => { aborted++; }, { once: true });
    const stream = new ReadableStream({
      pull(controller) {
        if (signal.aborted) { finish(); controller.error(new Error('cancelled')); return; }
        if (remaining === 0) { finish(); controller.close(); return; }
        const size = Math.min(32 * KiB, remaining);
        remaining -= size;
        produced += size;
        controller.enqueue(new Uint8Array(size));
      },
      cancel() { finish(); },
    }, { highWaterMark: 0 });
    return new Response(stream, { headers: { 'Content-Length': String(end - start + 1) } });
  } });
  t.after(() => bridge.close());
  const url = await bridge.publish(media(64 * 1024 * KiB));
  const connection = await new Promise(resolve => {
    const request = http.get(url, response => { response.pause(); resolve({ request, response }); });
    request.on('error', () => {});
  });
  t.after(() => { connection.request.destroy(); connection.response.destroy(); });
  let previous = -1, stable = 0;
  const deadline = Date.now() + 2000;
  while (stable < 3) {
    assert.ok(Date.now() < deadline, 'paused consumer eventually stops upstream reads');
    await delay(25);
    stable = produced === previous ? stable + 1 : 0;
    previous = produced;
  }
  const stoppedAt = produced;
  const stoppedRequests = requests;
  await delay(75);
  assert.equal(produced, stoppedAt, 'no background full-media buffering');
  assert.equal(requests, stoppedRequests, 'no range prefetch while downstream is paused');
  assert.ok(produced < 16 * 1024 * KiB, 'only bounded socket/stream buffering, not the 64 MiB media');
  assert.equal(maximumActive, 1, 'one upstream range per consumer at a time');
  assert.ok(maximumWrite <= 32 * KiB, 'larger ranges do not enlarge downstream writes');
  connection.response.destroy(); connection.request.destroy();
  await waitFor(() => bridge.activeRequests === 0 && active === 0);
  assert.ok(aborted > 0, 'disconnect aborts upstream transport');
});

test('larger video ranges meet a fixed delivery budget under per-request latency', async () => {
  const length = 4 * 1024 * KiB;
  const deadlineMS = 1000;
  const run = async contentType => {
    let requests = 0, received = 0, complete = false;
    const bridge = new StreamBridge({ fetch: async (url, { signal }) => {
      requests++;
      await delay(40, undefined, { signal });
      const [start, end] = rangeOf(url);
      return new Response(new Uint8Array(end - start + 1), { headers: { 'Content-Length': String(end - start + 1) } });
    } });
    try {
      const url = await bridge.publish(media(length, contentType));
      const signal = AbortSignal.timeout(deadlineMS);
      try {
        const response = await fetch(url, { signal });
        for await (const chunk of response.body) received += chunk.byteLength;
        complete = received === length;
      } catch { /* Only completion/counters leave this synthetic test. */ }
      return { requests, received, complete };
    } finally { await bridge.close(); }
  };
  // The unchanged audio path is the 64 KiB control with the same bytes and
  // request latency; no special test-only transport option enters production.
  const small = await run('audio/mp4');
  const video = await run('video/mp4');
  assert.equal(small.complete, false, '64 KiB sequential requests cannot meet this delivery budget');
  assert.ok(small.received < length, 'small-range control ran out of delivery time');
  assert.equal(video.complete, true, '512 KiB video requests meet the same delivery budget');
  assert.equal(video.requests, 8, 'video completes in eight sequential requests');
});
