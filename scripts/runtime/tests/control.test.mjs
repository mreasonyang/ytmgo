import test from 'node:test';
import assert from 'node:assert/strict';
import { RuntimeController, validateRequest, safeCategory, safeFailureDetails } from '../control.mjs';
import { chooseFormat, validateMediaURL } from '../resolver.mjs';
import { validateAnonymousRequest, MemoryCache } from '../sdk-session.mjs';

const tick = () => new Promise(resolve => setImmediate(resolve));
const deferred = () => { let resolve; const promise = new Promise(r => { resolve = r; }); return { promise, resolve }; };

test('failure metadata crosses JSONL only as fixed stage, phase and reason values', async () => {
  assert.deepEqual(safeFailureDetails({ safeStage: 'proof_config_parse', safePhase: 'context', safeReason: 'account_mismatch', message: 'synthetic-private' }),
    { stage: 'proof_config_parse', phase: 'context', reason: 'account_mismatch' });
  assert.deepEqual(safeFailureDetails({ safeStage: 'synthetic-private', safePhase: 'synthetic-private', safeReason: 'synthetic-private' }), {});
  const output = [];
  const control = new RuntimeController({
    async warm() { throw Object.assign(new Error('synthetic-private'), { safeCategory: 'unexpected_request_host', safeStage: 'proof_config_request' }); },
    async close() {},
  }, value => output.push(value));
  control.accept({ id: 1, op: 'warm' });
  await tick(); await control.close();
  assert.deepEqual(output, [{ id: 1, ok: false, category: 'unexpected_request_host', stage: 'proof_config_request' }]);
});

test('strict IPC schemas reject unexpected operations, inherited names, fields and IDs', () => {
  for (const value of [null, [], {}, { id: 0, op: 'warm' }, { id: 1.5, op: 'warm' },
    { id: 1, op: 'toString' }, { id: 1, op: '__proto__' }, { id: 1, op: 'warm', cookie: 'fixture' },
    { id: 1, op: 'resolve', videoId: 'short', kind: 'audio' },
    { id: 1, op: 'resolve', videoId: 'abcdefghijk', kind: 'other' },
    { id: 1, op: 'cancel', target: 1 }]) assert.equal(validateRequest(value), false);
  assert.equal(validateRequest({ id: 1, op: 'resolve', videoId: 'abcdefghijk', kind: 'video' }), true);
  assert.equal(safeCategory(new Error('untrusted diagnostic')), 'runtime_failed');
});

test('cancel and release bypass a running resolve; queued work stays serial', async () => {
  const output = [], starts = [], releases = [];
  const first = deferred();
  const resolver = {
    async warm(signal) { starts.push('warm'); await first.promise; return {}; },
    async resolve(id, kind, signal) { starts.push(id); return { media: { url: 'fixture-capability', duration: 10 } }; },
    release(url) { releases.push(url); }, async close() {},
  };
  const control = new RuntimeController(resolver, value => output.push(value));
  control.accept({ id: 1, op: 'warm' });
  control.accept({ id: 2, op: 'resolve', videoId: 'abcdefghijk', kind: 'audio' });
  control.accept({ id: 3, op: 'release', url: 'prior-capability' });
  control.accept({ id: 4, op: 'cancel', target: 2 });
  control.accept({ id: 5, op: 'cancel', target: 1 });
  assert.deepEqual(starts, ['warm']);
  assert.deepEqual(releases, ['prior-capability']);
  assert.equal(output.find(x => x.id === 2)?.category, 'request_cancelled');
  assert.equal(output.find(x => x.id === 3)?.ok, true);
  assert.equal(output.find(x => x.id === 5)?.ok, true);
  first.resolve(); await tick();
  assert.equal(output.find(x => x.id === 1)?.category, 'request_cancelled');
  assert.equal(output.filter(x => x.id === 1).length, 1);
  await control.close();
});

test('cancelled late completion releases its new capability and preserves existing lease', async () => {
  const output = [], released = [], gate = deferred();
  const control = new RuntimeController({
    async resolve() { await gate.promise; return { media: { url: 'new-capability', duration: 30 } }; },
    release(url) { released.push(url); }, async close() {},
  }, value => output.push(value));
  control.accept({ id: 1, op: 'resolve', videoId: 'abcdefghijk', kind: 'audio' });
  control.accept({ id: 2, op: 'cancel', target: 1 });
  gate.resolve(); await tick();
  assert.deepEqual(released, ['new-capability']);
  assert.equal(output.find(x => x.id === 1)?.category, 'request_cancelled');
  await control.close();
});

test('queue bound and duplicate handling do not start extra work; close aborts active request', async () => {
  let started = 0, closed = false;
  const output = [];
  const control = new RuntimeController({
    warm(signal) { started++; return new Promise(resolve => signal.addEventListener('abort', resolve, { once: true })); },
    async close() { closed = true; },
  }, value => output.push(value));
  for (let id = 1; id <= 17; id++) control.accept({ id, op: 'warm' });
  control.accept({ id: 1, op: 'warm' });
  assert.equal(started, 1);
  assert.equal(output.find(x => x.id === 17)?.category, 'queue_full');
  assert.equal(output.find(x => x.id === 1)?.category, 'duplicate_request');
  await control.close();
  assert.equal(closed, true);
  assert.equal(started, 1);
});

test('deadline produces a fixed error and lets the next operation proceed', async () => {
  const output = [];
  const control = new RuntimeController({
    warm(signal) { return new Promise(resolve => signal.addEventListener('abort', resolve, { once: true })); },
    async close() {},
  }, value => output.push(value), { timeoutMS: 10 });
  control.accept({ id: 1, op: 'warm' });
  await new Promise(resolve => setTimeout(resolve, 25));
  assert.equal(output.find(x => x.id === 1)?.category, 'request_timeout');
  await control.close();
});

test('anonymous requests reject account authentication and foreign origins', () => {
  for (const headers of [{ authorization: 'fixture' }, { cookie: 'SAPISID=fixture' }, { cookie: 'PREF=fixture; other=fixture' }]) {
    assert.throws(() => validateAnonymousRequest(new Request('https://music.youtube.com/youtubei/v1/player', { headers })), e => e.safeCategory === 'unexpected_authentication');
  }
  assert.doesNotThrow(() => validateAnonymousRequest(new Request('https://www.youtube.com/', { headers: { cookie: 'PREF=fixture; VISITOR_INFO1_LIVE=fixture' } })));
  assert.throws(() => validateAnonymousRequest(new Request('https://example.com/')), e => e.safeCategory === 'unexpected_request_host');
  assert.throws(() => validateMediaURL('https://example.com/media'), e => e.safeCategory === 'invalid_media_url');
  assert.throws(() => validateMediaURL('https://fixture.googlevideo.com/media?sabr=1'), e => e.safeCategory === 'sabr_unsupported');
});

test('format selection respects native codecs and separates audio/video/DRM/SABR', () => {
  const audio = { mimeType: 'audio/webm; codecs="opus"', url: 'fixture', bitrate: 128000 };
  const video = { mimeType: 'video/mp4; codecs="avc1.4d401e"', width: 854, height: 480, url: 'fixture' };
  const data = { streamingData: { adaptiveFormats: [audio, video, { ...video, height: 720 }] } };
  assert.equal(chooseFormat(data, 'audio').raw, audio);
  assert.equal(chooseFormat(data, 'video').raw, video);
  assert.throws(() => chooseFormat({ streamingData: { adaptiveFormats: [{ ...audio, drmFamilies: ['fixture'] }] } }, 'audio'), e => e.safeCategory === 'drm_unsupported');
  assert.throws(() => chooseFormat({ streamingData: { serverAbrStreamingUrl: 'fixture' } }, 'video'), e => e.safeCategory === 'sabr_unsupported');
  assert.throws(() => chooseFormat({ streamingData: { adaptiveFormats: [audio] } }, 'video'), e => e.safeCategory === 'no_video_format');
});

test('media validation rejects decipher error sentinels without leaking their payload', () => {
  const base = 'https://fixture.googlevideo.com/videoplayback';
  for (const query of ['n=enhanced_except_fixture_payload', 'n=enhanced%5fexcept%5ffixture_payload']) {
    assert.throws(() => validateMediaURL(`${base}?${query}`), failure =>
      failure.safeCategory === 'decipher_failed' && failure.message === 'decipher_failed');
  }
  for (const query of ['', '?n=valid_fixture', '?other=enhanced_except_fixture_payload']) {
    assert.doesNotThrow(() => validateMediaURL(base + query));
  }
});

test('memory cache copies values and evicts bounded entries', async () => {
  const cache = new MemoryCache();
  const input = new Uint8Array([1, 2, 3]);
  await cache.set('a', input); input[0] = 9;
  assert.equal(new Uint8Array(await cache.get('a'))[0], 1);
  for (const key of ['b', 'c', 'd', 'e']) await cache.set(key, new Uint8Array(1));
  assert.equal(await cache.get('a'), undefined);
  await assert.rejects(() => cache.set('large', new ArrayBuffer(8 * 1024 * 1024 + 1)), e => e.safeCategory === 'cache_limit');
});
