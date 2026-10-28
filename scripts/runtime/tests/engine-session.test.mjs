import test from 'node:test';
import assert from 'node:assert/strict';
import { spawn } from 'node:child_process';
import { fileURLToPath } from 'node:url';
import { dirname } from 'node:path';
import { EngineSession } from '../engine-session.mjs';
import { encodeFrame, FrameReader, REQUEST_LIMIT } from '../engine-ipc.mjs';

const workerURL = new URL('./fixtures/engine-worker-fixture.mjs', import.meta.url);
const anonymous = () => ({ loggedIn: false, musicPremium: false, cookie: '', accountIndex: 0, onBehalfOfUser: '', revision: 1 });
const make = options => new EngineSession({ workerURL, session: anonymous(), ...options });
const seed = (playerID = 'normal') => ({ entries: [['synthetic', new ArrayBuffer(1)]], playerID });
const fixed = category => error => error instanceof Error && error.message === category && error.safeCategory === category && error.cause === undefined;
const pid = result => Number(new URL(result.media.url).searchParams.get('fixture').split('-')[0]);
const gone = id => assert.throws(() => process.kill(id, 0), error => error.code === 'ESRCH');
const tick = () => new Promise(resolve => setImmediate(resolve));

test('one restricted engine persists across warm, audio and video; close reaps it', async () => {
  const session = make(), input = seed();
  try {
    await assert.rejects(session.resolve('abcdefghijk', 'audio'), fixed('init_failed'));
    await session.warm(input);
    const first = await session.resolve('abcdefghijk', 'audio');
    await session.warm(input);
    const second = await session.resolve('zyxwvutsrqp', 'video');
    assert.equal(pid(first), pid(second)); assert.equal(second.media.height, 360);
    assert.equal(new URL(second.media.url).searchParams.get('fixture').split('-')[1], '1');
    await session.close(); gone(pid(first));
  } finally { await session.close(); }
});

test('cancellation and timeout reject after reaping; later explicit warm can recover', async () => {
  const session = make({ timeoutMS: 150 }), input = seed();
  try {
    await session.warm(input); const first = await session.resolve('abcdefghijk', 'audio');
    const abort = new AbortController();
    const pending = assert.rejects(session.resolve('holdresolve', 'audio', abort.signal), fixed('request_cancelled'));
    await assert.rejects(session.warm(input), fixed('runtime_failed')); abort.abort(); await pending; gone(pid(first));
    await session.warm(input); const second = await session.resolve('abcdefghijk', 'audio');
    await assert.rejects(session.resolve('holdresolve', 'audio'), fixed('request_timeout')); gone(pid(second));
    await session.warm(input); await session.resolve('abcdefghijk', 'audio');
  } finally { await session.close(); }
});

test('warm cancellation, close and changed seeds do not leave an engine alive', async () => {
  const session = make();
  const abort = new AbortController();
  const warming = assert.rejects(session.warm(seed('hold'), abort.signal), fixed('request_cancelled'));
  await tick(); abort.abort(); await warming;
  await session.warm(seed()); const first = await session.resolve('abcdefghijk', 'audio');
  await session.warm(seed('fragmented')); gone(pid(first));
  const second = await session.resolve('abcdefghijk', 'audio');
  const pending = assert.rejects(session.resolve('holdresolve', 'audio'), fixed('runtime_closed'));
  const closing = session.close(); assert.equal(session.close(), closing); await closing; await pending; gone(pid(second));
  await assert.rejects(session.warm(seed()), fixed('runtime_closed'));
});

test('invalid caller input and pre-cancel leave an existing engine usable', async () => {
  const session = make(), input = seed();
  try {
    await session.warm(input); const first = await session.resolve('abcdefghijk', 'audio');
    await assert.rejects(session.warm({ entries: [], playerID: 'bad' }), fixed('init_failed'));
    await assert.rejects(session.resolve('bad', 'audio'), fixed('invalid_request'));
    await assert.rejects(session.resolve('abcdefghijk', 'audio', AbortSignal.abort()), fixed('request_cancelled'));
    assert.equal(pid(await session.resolve('abcdefghijk', 'audio')), pid(first));
  } finally { await session.close(); }
});

test('bad IPC and untrusted labels become fixed errors; no failure-then-success', async () => {
  for (const mode of ['malformed', 'oversize', 'wrong_id', 'extra', 'unknown_error', 'unsafe_stage', 'far_future', 'missing_safe', 'unsafe_metric', 'double', 'trailing', 'exit']) {
    const session = make();
    try { await assert.rejects(session.warm(seed(mode)), fixed('runtime_failed')); await session.warm(seed()); }
    finally { await session.close(); }
  }
  const session = make();
  try {
    await session.warm(seed('resolve_error'));
    await assert.rejects(session.resolve('abcdefghijk', 'audio'), error => fixed('proof_request_rejected')(error) && error.safePhase === 'mint' && error.safeReason === 'http' && error.safeStage === 'proof_mint');
    await assert.rejects(session.resolve('abcdefghijk', 'audio'), fixed('init_failed'));
    await session.warm(seed()); await session.resolve('abcdefghijk', 'audio');
  } finally { await session.close(); }
});

test('media and refresh deadlines are validated without retry', async () => {
  for (const [mode, category] of [['foreign_media', 'invalid_media_url'], ['bad_media', 'runtime_failed'], ['sentinel', 'decipher_failed']]) {
    const session = make(); try { await session.warm(seed(mode)); await assert.rejects(session.resolve('abcdefghijk', 'audio'), fixed(category)); } finally { await session.close(); }
  }
  const expired = make(); try { await assert.rejects(expired.warm(seed('expired')), fixed('proof_expired')); } finally { await expired.close(); }
  const session = make(), now = Date.now;
  try {
    const warm = await session.warm(seed()); const first = await session.resolve('abcdefghijk', 'audio');
    Date.now = () => warm.expiresAt - 60_000;
    await assert.rejects(session.resolve('abcdefghijk', 'audio'), fixed('proof_expired')); gone(pid(first));
  } finally { Date.now = now; await session.close(); }
});

test('permission boundary denies unrelated reads, writes, spawn, threads and addons', async () => {
  for (const mode of ['permissions', 'fragmented', 'stderr']) {
    const session = make(); try { await session.warm(seed(mode)); await session.resolve('abcdefghijk', 'audio'); } finally { await session.close(); }
  }
  const missing = make({ nodePath: '/nonexistent/ytmgo-engine-fixture' });
  try { await assert.rejects(missing.warm(seed()), fixed('runtime_failed')); } finally { await missing.close(); }
});

test('binary frame parser supports fragmented opaque cache and bounds before allocation', () => {
  const value = { entries: [['synthetic', new ArrayBuffer(64)]] };
  const frame = encodeFrame(value, REQUEST_LIMIT), reader = new FrameReader(REQUEST_LIMIT), replies = [];
  for (let i = 0; i < frame.length; i += 3) reader.push(frame.subarray(i, i + 3), reply => replies.push(reply));
  assert.deepEqual(replies, [value]); assert.equal(reader.partial, false);
  const oversized = Buffer.alloc(4); oversized.writeUInt32BE(REQUEST_LIMIT);
  assert.throws(() => reader.push(oversized, () => {}), fixed('runtime_failed'));
  reader.clear(); assert.equal(reader.partial, false);
});

test('actual restricted engine rejects invalid control before SDK/provider initialization and reaps on EOF', async () => {
  const path = fileURLToPath(new URL('../engine-worker.mjs', import.meta.url));
  for (const request of [null, { id: 1, op: 'warm', entries: [], playerID: 'fixture' }]) {
    const child = spawn(process.execPath, ['--permission', `--allow-fs-read=${dirname(path)}`, '--allow-net', path], {
      stdio: ['pipe', 'pipe', 'ignore'], env: { PATH: '/usr/bin:/bin', LANG: 'en_US.UTF-8', LC_ALL: 'en_US.UTF-8' },
    });
    const responses = [], reader = new FrameReader(128 * 1024);
    child.stdout.on('data', chunk => reader.push(chunk, reply => responses.push(reply)));
    const finished = new Promise((resolve, reject) => { child.once('close', resolve); child.once('error', reject); });
    if (request) child.stdin.write(encodeFrame(request, REQUEST_LIMIT)); else child.stdin.end();
    const code = await finished;
    if (request) { assert.equal(code, 2); assert.deepEqual(responses, [{ id: 0, ok: false, category: 'runtime_failed' }]); }
    else { assert.equal(code, 0); assert.deepEqual(responses, []); }
  }
});
