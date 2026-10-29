import assert from 'node:assert/strict';
import { registerHooks } from 'node:module';
import { guardPremiumModules } from '../../premium-module-guard.mjs';

let proofImports = 0;
registerHooks({ resolve(specifier, context, next) {
  if (/engine-proof|proof-session|vendor\/bgutil|jsdom|happy-dom|linkedom|bgutils/.test(specifier)) proofImports++;
  return next(specifier, context);
} });
const guard = guardPremiumModules();
try {
  const { PlaybackEngine } = await import('../../engine.mjs');
  const response = { contents: {}, responseContext: { serviceTrackingParams: [
    { service: 'GFEEDBACK', params: [{ key: 'logged_in', value: '1' }] }, { service: 'CSI', params: [{ key: 'yt_li', value: '1' }] },
  ] } };
  const raw = { mimeType: 'audio/webm; codecs="opus"', contentLength: '65536', approxDurationMs: '120000', url: 'synthetic', bitrate: 128000 };
  // Use the real engine and its default proof loader. Only external service
  // responses/format deciphering are synthetic; no proof mock can hide a load.
  const engine = new PlaybackEngine({
    createSession: async () => ({ session: { player: { signature_timestamp: 1 } }, actions: { async execute(path) { return { data: path === '/browse' ? response : { playabilityStatus: { status: 'OK' }, streamingData: { adaptiveFormats: [raw] } } }; } } }),
    formatFactory: async () => ({ decipher: async () => 'https://fixture.googlevideo.com/media?expire=9999999999' }),
  });
  const signal = new AbortController().signal;
  await engine.warm({ entries: [['synthetic', new ArrayBuffer(1)]], playerID: 'fixture', session: { loggedIn: true, musicPremium: true, cookie: 'SAPISID=synthetic', accountIndex: 1, onBehalfOfUser: '', revision: 1 } }, signal);
  const result = await engine.resolve('abcdefghijk', 'audio', signal);
  assert.equal(new URL(result.media.url).searchParams.has('pot'), false);
  assert.equal(result.safe.proof_ms, 0); assert.equal(result.safe.proof_init_ms, 0);
  assert.equal(proofImports, 0); assert.equal(typeof globalThis.document, 'undefined');
  for (const name of ['jsdom', 'happy-dom', 'linkedom', 'bgutils-js', '../../engine-proof.mjs', '../../proof-session.mjs', '../../vendor/bgutil/build/session_manager.js']) {
    await assert.rejects(import(name), e => e.safeCategory === 'unexpected_authentication');
  }
  engine.close();
  process.stdout.write(JSON.stringify({ passed: true }));
} catch { process.exitCode = 1; }
finally { guard.deregister(); }
