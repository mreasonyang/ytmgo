import test from 'node:test';
import assert from 'node:assert/strict';
import { spawn } from 'node:child_process';
import { fileURLToPath } from 'node:url';
import { RuntimeController } from '../control.mjs';
import { Resolver } from '../resolver.mjs';
import { EngineSession } from '../engine-session.mjs';
import { PlaybackEngine } from '../engine.mjs';
import { validateAccountSession, validateSessionRequest, hasAcceptedAccount } from '../account-session.mjs';
import { MemoryCache } from '../sdk-session.mjs';

const anonymous = (revision = 1) => ({ loggedIn: false, musicPremium: false, cookie: '', accountIndex: 0, onBehalfOfUser: '', revision });
const account = (musicPremium = true, revision = 1) => ({ ...anonymous(revision), loggedIn: true, musicPremium, cookie: 'SAPISID=synthetic-fixture', accountIndex: 2 });
const seed = () => ({ entries: [['synthetic', new ArrayBuffer(1)]], playerID: 'normal', fetchCount: 0 });
const accepted = () => ({ contents: {}, responseContext: { serviceTrackingParams: [
  { service: 'GFEEDBACK', params: [{ key: 'logged_in', value: '1' }] }, { service: 'CSI', params: [{ key: 'yt_li', value: '1' }] },
] } });
const tick = () => new Promise(resolve => setImmediate(resolve));
const fixed = category => error => error?.safeCategory === category && error.message === category && !error.cause;
const deferred = () => { let resolve; const promise = new Promise(r => { resolve = r; }); return { promise, resolve }; };

test('account schema bounds private cookies and rejects contradictory or ambiguous session state', () => {
  assert.deepEqual(validateAccountSession(anonymous()), anonymous());
  assert.deepEqual(validateAccountSession(account()), account());
  for (const bad of [null, { ...anonymous(), musicPremium: true }, { ...anonymous(), cookie: 'SAPISID=synthetic' },
    { ...account(), cookie: 'SAPISID=synthetic\n' }, { ...account(), cookie: 'SAPISID=synthetic; SAPISID=other' },
    { ...account(), cookie: 'unapproved=synthetic' }, { ...account(), accountIndex: -1 },
    { ...account(), revision: 0 }, { ...account(), extra: 'synthetic' }, { ...account(), onBehalfOfUser: 'invalid value' },
  ]) assert.throws(() => validateAccountSession(bad));
});

test('configure preempts active and queued requests and waits before new account work', async () => {
  const output = [], events = [], gate = deferred();
  const control = new RuntimeController({
    async warm(signal) { events.push('warm'); await new Promise(resolve => signal.addEventListener('abort', resolve, { once: true })); events.push('aborted'); await gate.promise; events.push('reaped'); },
    async configure(value) { assert.equal(value.revision, 2); events.push('configure'); return {}; },
    async resolve() { events.push('resolve'); return {}; }, async close() {},
  }, reply => output.push(reply));
  control.accept({ id: 1, op: 'warm' });
  control.accept({ id: 2, op: 'resolve', videoId: 'abcdefghijk', kind: 'audio' });
  control.accept({ id: 3, op: 'configure', session: account(true, 2) });
  control.accept({ id: 4, op: 'resolve', videoId: 'abcdefghijk', kind: 'audio' });
  await tick(); assert.deepEqual(events, ['warm', 'aborted']);
  gate.resolve(); await tick(); await tick();
  assert.deepEqual(events, ['warm', 'aborted', 'reaped', 'configure', 'resolve']);
  assert.equal(output.find(x => x.id === 1)?.category, 'request_cancelled');
  assert.equal(output.find(x => x.id === 2)?.category, 'request_cancelled');
  assert.equal(output.find(x => x.id === 3)?.ok, true);
  control.accept({ id: 5, op: 'configure', session: anonymous(1) });
  assert.equal(output.find(x => x.id === 5)?.category, 'invalid_request');
  assert.equal(JSON.stringify(output).includes('synthetic-fixture'), false);
  await control.close();
});

test('resolver replaces and reaps real engines across Premium/proof changes and revokes only changed accounts', async () => {
  const modes = [], upstream = [], revoked = [], released = [];
  let bootstraps = 0;
  const bootstrapFactory = () => ({ async warm() { bootstraps++; return seed(); }, async close() {} });
  const resolver = new Resolver({
    bootstrap: bootstrapFactory(), bootstrapFactory,
    engineFactory: session => { modes.push(session.musicPremium); return new EngineSession({ session, workerURL: new URL('./fixtures/engine-worker-fixture.mjs', import.meta.url) }); },
    bridge: { size: 0, async publish(media) { upstream.push(media); return `capability-${upstream.length}`; }, release(value) { released.push(value); }, revokeAll() { revoked.push(true); }, async close() {} },
  });
  const pidAt = index => Number(new URL(upstream[index].url).searchParams.get('fixture').split('-')[0]);
  const gone = pid => assert.throws(() => process.kill(pid, 0), e => e.code === 'ESRCH');
  try {
    await assert.rejects(resolver.warm(), fixed('init_failed'));
    await resolver.configure(account(true)); await resolver.resolve('abcdefghijk', 'audio');
    await assert.rejects(resolver.configure({ ...account(), accountIndex: 3 }), fixed('unexpected_authentication'));
    await assert.rejects(resolver.configure(anonymous(1)), fixed('unexpected_authentication'));
    await resolver.configure(account(false)); gone(pidAt(0));
    assert.deepEqual(revoked, []); assert.deepEqual(released, []);
    await resolver.resolve('abcdefghijk', 'audio');
    await resolver.configure(account(true)); gone(pidAt(1)); await resolver.resolve('abcdefghijk', 'audio');
    await resolver.configure(anonymous(2)); gone(pidAt(2)); assert.equal(revoked.length, 1);
    await resolver.resolve('abcdefghijk', 'audio');
    await assert.rejects(resolver.configure(account(true, 1)), fixed('invalid_request'));
    assert.deepEqual(modes, [true, false, true, false]); assert.equal(bootstraps, 4);
  } finally { await resolver.close(); }
  gone(pidAt(3));
});

test('configure supersession cannot briefly start work for a cancelled session', async () => {
  const output = [], starts = [], gate = deferred();
  const control = new RuntimeController({
    async configure(session, signal) { starts.push(session.revision); if (session.revision === 1) await gate.promise; if (signal.aborted) throw Object.assign(new Error('request_cancelled'), { safeCategory: 'request_cancelled' }); return {}; },
    async warm() { starts.push('warm'); return {}; }, async close() {},
  }, reply => output.push(reply));
  control.accept({ id: 1, op: 'configure', session: account(true, 1) });
  control.accept({ id: 2, op: 'warm' });
  control.accept({ id: 3, op: 'configure', session: account(false, 2) });
  control.accept({ id: 4, op: 'warm' });
  gate.resolve(); await tick(); await tick();
  assert.deepEqual(starts, [1, 2, 'warm']);
  assert.equal(output.find(x => x.id === 2)?.category, 'request_cancelled');
  await control.close();
});

test('WEB_REMIX authentication requires two server signals; membership metadata does not gate it', async () => {
  assert.equal(hasAcceptedAccount(accepted()), true);
  for (const reply of [{ contents: {} }, { ...accepted(), error: {} }, { ...accepted(), contents: null },
    { contents: {}, responseContext: { serviceTrackingParams: [{ service: 'GFEEDBACK', params: [{ key: 'logged_in', value: '0' }] }] } },
    { ...accepted(), responseContext: { ...accepted().responseContext, mainAppWebResponseContext: { loggedOut: true } } },
  ]) assert.equal(hasAcceptedAccount(reply), false);
  let proofCreated = 0;
  const engine = new PlaybackEngine({ proofFactory() { proofCreated++; throw new Error('should not import proof'); },
    createSession: async () => ({ session: { logged_in: true, cookie: 'synthetic', player: { signature_timestamp: 1 } }, actions: { execute: async () => ({ data: { contents: {} } }) } }),
  });
  try { await assert.rejects(engine.warm({ ...seed(), session: account() }, new AbortController().signal), fixed('login_required')); assert.equal(proofCreated, 0); }
  finally { engine.close(); }
});

test('actual SDK selected-account request is checked before network and account headers cannot leave API origins', async () => {
  const sdk = await import('youtubei.js'); sdk.Log.setLevel(sdk.Log.Level.NONE);
  const { default: Session } = await import('../node_modules/youtubei.js/dist/src/core/Session.js');
  const { serialize } = await import('../node_modules/youtubei.js/dist/src/utils/BinarySerializer.js');
  const selected = account(); let calls = 0;
  const transport = async (input, init) => {
    const request = new Request(input, init); await validateSessionRequest(request, selected);
    calls++; assert.equal(request.headers.get('x-goog-authuser'), '2');
    return new Response(JSON.stringify(accepted()), { headers: { 'content-type': 'application/json' } });
  };
  const cache = new MemoryCache([['innertube_session_data', serialize({ library_version: 18, account_index: 0,
    context: { client: { clientName: 'WEB_REMIX', clientVersion: 'synthetic', hl: 'en', gl: 'US' }, user: {}, request: {} }, api_key: 'synthetic-key', api_version: 'v1' })]], { strict: true });
  const session = await Session.create({ client_type: 'WEB_REMIX', account_index: selected.accountIndex, cookie: selected.cookie,
    fetch: transport, cache, enable_session_cache: true, retrieve_player: false, retrieve_innertube_config: false });
  assert.equal(session.account_index, 2);
  assert.equal(hasAcceptedAccount((await session.actions.execute('/browse', { browseId: 'FEmusic_library_landing', client: 'YTMUSIC' })).data), true);
  assert.equal(calls, 1);
  for (const url of ['https://www.youtube.com/', 'https://www.google.com/', 'https://fixture.googlevideo.com/media', 'https://www.youtube.com/youtubei/v1/other']) {
    await assert.rejects(validateSessionRequest(new Request(url, { headers: { cookie: selected.cookie, authorization: 'synthetic' } }), selected));
  }
  const api = (headers = {}) => new Request('https://music.youtube.com/youtubei/v1/player', { method: 'POST', headers: { cookie: selected.cookie, authorization: 'synthetic', 'x-goog-authuser': '2', ...headers }, body: JSON.stringify({ context: { client: { clientName: 'WEB_REMIX' } } }) });
  await assert.rejects(validateSessionRequest(api({ 'x-goog-authuser': '0' }), selected), fixed('unexpected_authentication'));
  await assert.rejects(validateSessionRequest(api(), anonymous()), fixed('unexpected_authentication'));
});

test('real restricted Premium process warms and resolves with a guarded module graph and no proof injection', async () => {
  const path = fileURLToPath(new URL('./fixtures/premium-engine-fixture.mjs', import.meta.url));
  const root = fileURLToPath(new URL('../', import.meta.url));
  const child = spawn(process.execPath, ['--permission', `--allow-fs-read=${root}`, path], { stdio: ['ignore', 'pipe', 'pipe'], env: { PATH: '/usr/bin:/bin' } });
  let output = '', stderr = 0;
  child.stdout.on('data', data => { output += data.toString(); }); child.stderr.on('data', data => { stderr += data.length; });
  const code = await new Promise((resolve, reject) => { child.once('close', resolve); child.once('error', reject); });
  assert.equal(code, 0); assert.equal(stderr, 0); assert.deepEqual(JSON.parse(output), { passed: true });
});
