import test from 'node:test';
import assert from 'node:assert/strict';
import { PlaybackEngine } from '../engine.mjs';
import { EngineProof } from '../engine-proof.mjs';
import { Resolver } from '../resolver.mjs';

const fixed = category => error => error?.safeCategory === category && error.message === category && error.cause === undefined;
const signal = () => new AbortController().signal;
const anonymous = () => ({ loggedIn: false, musicPremium: false, cookie: '', accountIndex: 0, onBehalfOfUser: '', revision: 1 });
const seed = () => ({ session: anonymous(), entries: [['synthetic', new ArrayBuffer(3)]], playerID: 'fixture', fetchCount: 2 });
const zero = () => ({ init_ms: 0, proof_init_ms: 0, resolve_ms: 0, decipher_ms: 0, proof_ms: 0, fetch_count: 0, player_requests: 0 });
const deferred = () => { let resolve; const promise = new Promise(r => { resolve = r; }); return { promise, resolve }; };

test('cached engine initialization denies any network fallback and preserves strict misses', async () => {
  let proofCalled = false;
  for (const category of ['bootstrap_cache_miss', 'network']) {
    const engine = new PlaybackEngine({ proof: { warm() { proofCalled = true; }, close() {} }, createSession: async options => {
      assert.equal(options.playerID, 'fixture');
      assert.equal((await options.cache.get('synthetic')).byteLength, 3);
      if (category === 'bootstrap_cache_miss') await options.cache.get('missing');
      else await options.fetch('https://www.youtube.com/');
    } });
    try { await assert.rejects(engine.warm(seed(), signal()), fixed('bootstrap_cache_miss')); }
    finally { engine.close(); }
  }
  assert.equal(proofCalled, false);
});

test('one anonymous session and proof generator serve multiple tracks and selected formats', async () => {
  let creates = 0, warms = 0, deciphers = 0;
  const minted = [], requests = [];
  const player = { signature_timestamp: 123 };
  const data = { playabilityStatus: { status: 'OK' }, streamingData: { adaptiveFormats: [
    { mimeType: 'audio/webm; codecs="opus"', url: 'synthetic', contentLength: '65536', approxDurationMs: '120000', bitrate: 128000 },
    { mimeType: 'video/mp4; codecs="avc1.4d401e"', url: 'synthetic', contentLength: '131072', approxDurationMs: '120000', bitrate: 256000, width: 640, height: 360 },
  ] } };
  const context = { client: { clientName: 'WEB_REMIX', visitorData: 'anonymous-fixture' } };
  const engine = new PlaybackEngine({
    network: async request => {
      assert.equal(request.url, 'https://music.youtube.com/'); assert.equal(request.headers.has('cookie'), false); assert.equal(request.redirect, 'manual');
      return new Response('ytcfg.set(' + JSON.stringify({ LOGGED_IN: false, VISITOR_DATA: 'anonymous-fixture', WEB_PLAYER_CONTEXT_CONFIGS: { music: { serializedExperimentFlags: 'html5_generate_content_po_token=true' } } }) + ');');
    },
    createSession: async options => { creates++; await options.cache.get('synthetic'); return { session: { player, context, logged_in: false }, actions: { async execute(path, request) { requests.push(request); return { data }; } } }; },
    proof: { async warm(value) { warms++; assert.equal(value, context); return { expiresAt: Date.now() + 120000 }; }, async mint(id) { minted.push(id); return { token: 'synthetic-proof', expiresAt: Date.now() + 120000 }; }, close() {} },
    formatFactory: async () => ({ async decipher(value) { assert.equal(value, player); deciphers++; return 'https://fixture.googlevideo.com/media?expire=9999999999'; } }),
  });
  try {
    await engine.warm(seed(), signal());
    const first = await engine.resolve('abcdefghijk', 'audio', signal());
    const second = await engine.resolve('zyxwvutsrqp', 'video', signal());
    assert.equal(first.media.contentType, 'audio/webm'); assert.equal(second.media.width, 640);
    assert.equal(first.safe.codec, 'opus'); assert.equal(second.safe.codec, 'avc1');
    assert.equal(new URL(first.media.url).searchParams.get('pot'), 'synthetic-proof');
    assert.match(new URL(first.media.url).searchParams.get('cpn'), /^[A-Za-z0-9_-]{16}$/);
    assert.equal(creates, 1); assert.equal(warms, 1); assert.equal(deciphers, 2);
    assert.deepEqual(minted, ['abcdefghijk', 'zyxwvutsrqp']); assert.ok(requests.every(x => x.client === 'YTMUSIC'));
    await assert.rejects(engine.warm(seed(), signal()), fixed('init_failed'));
  } finally { engine.close(); }
});

test('root resolver preserves one seed and existing bridge leases across engine failures', async () => {
  let bootstraps = 0, warms = 0, resolves = 0, failNext = false, closed = 0;
  const published = [], released = [], input = seed();
  const engine = {
    async warm(value) { assert.equal(value, input); warms++; return { safe: zero() }; },
    async resolve() { resolves++; if (failNext) { failNext = false; throw Object.assign(new Error('proof_failed'), { safeCategory: 'proof_failed' }); } return { media: { url: 'synthetic-private-upstream', duration: 120, length: 65536, contentType: 'audio/webm' }, safe: { ...zero(), kind: 'audio', codec: 'opus', bitrate: 128000, format_count: 1 } }; },
    async close() { closed++; },
  };
  const bridge = { size: 0, async publish(media) { published.push(media); return `synthetic-capability-${published.length}`; }, release(url) { released.push(url); }, async close() { closed++; } };
  const resolver = new Resolver({ engine, bridge, bootstrap: { async warm() { bootstraps++; return input; }, async close() { closed++; } } });
  await resolver.configure(anonymous(), signal());
  await resolver.warm(signal());
  const first = await resolver.resolve('abcdefghijk', 'audio', signal());
  failNext = true;
  await assert.rejects(resolver.resolve('zyxwvutsrqp', 'audio', signal()), fixed('proof_failed'));
  assert.deepEqual(released, []);
  const third = await resolver.resolve('abcdefghijk', 'audio', signal());
  assert.equal(bootstraps, 1); assert.equal(warms, 4); assert.equal(resolves, 3);
  assert.notEqual(first.media.url, third.media.url); assert.deepEqual(Object.keys(third.media).sort(), ['duration', 'url']);
  resolver.release(first.media.url); assert.deepEqual(released, [first.media.url]);
  await resolver.close(); assert.equal(closed, 3);
});

test('late cancelled publish is revoked without closing already active streams', async () => {
  const gate = deferred(), released = [], abort = new AbortController();
  const resolver = new Resolver({
    bootstrap: { async warm() { return seed(); }, async close() {} },
    engine: { async warm() { return { safe: zero() }; }, async resolve() { return { media: {}, safe: zero() }; }, async close() {} },
    bridge: { size: 1, async publish() { await gate.promise; return 'new-capability'; }, release(url) { released.push(url); }, async close() {} },
  });
  await resolver.configure(anonymous(), signal());
  const pending = assert.rejects(resolver.resolve('abcdefghijk', 'audio', abort.signal), fixed('request_cancelled'));
  await new Promise(resolve => setImmediate(resolve)); abort.abort(); gate.resolve(); await pending;
  assert.deepEqual(released, ['new-capability']); await resolver.close();
});

test('proof network failure is terminal even when a session catches it and returns success', async () => {
  let invalidated = 0, generated = 0;
  const callbacks = [];
  const axios = { create: () => ({ async request(config) {
    assert.equal(config.proxy, false); assert.equal(config.maxRedirects, 0);
    if (generated > 1) throw new Error('synthetic private transport error');
    return { data: 'synthetic' };
  } }) };
  class ProofSession {
    constructor({ request }) { this.request = request; }
    async initialize() {}
    async mint() {
      generated++;
      try { await this.request({ url: 'https://www.youtube.com/', method: 'GET', headers: {} }); } catch {}
      return { token: 'synthetic-proof', expiresAt: Date.now() + 120000 };
    }
    close() { invalidated++; }
  }
  const proof = new EngineProof({ onFailure: category => callbacks.push(category), loadAxios: async () => ({ default: axios }), loadSession: async () => ({ ProofSession }) });
  try {
    await proof.warm({ client: { clientName: 'WEB_REMIX', visitorData: 'anonymous-fixture' } }, signal(), { sessionBinding: 'anonymous-fixture', videoBound: true });
    await assert.rejects(proof.mint('abcdefghijk', signal()), fixed('proof_failed'));
    await assert.rejects(proof.mint('abcdefghijk', signal()), fixed('proof_request_rejected'));
    assert.deepEqual(callbacks, ['proof_failed']); assert.equal(generated, 2);
  } finally { proof.close(); }
  assert.equal(invalidated, 1);
});
