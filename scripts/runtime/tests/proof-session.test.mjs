import test from 'node:test';
import assert from 'node:assert/strict';
import { parseLooseJSON } from 'bgutils-js/utils';
import { EngineProof } from '../engine-proof.mjs';
import { parseProofPage } from '../proof-session.mjs';

const context = () => ({ client: { clientName: 'WEB_REMIX', visitorData: 'synthetic-visitor' } });
const binding = (videoBound = true) => ({ sessionBinding: 'synthetic-visitor', videoBound });
const signal = () => new AbortController().signal;
const challenge = () => ({ program: 'fixture-program', globalName: 'fixtureBotGuard',
  interpreterUrl: { privateDoNotAccessOrElseTrustedResourceUrlWrappedValue: '//www.google.com/js/th/fixture.js' } });
const page = (bgChallenge = challenge(), config = { EVENT_ID: 'fixture-event', text: 'braces } and " quotes', LOGGED_IN: false }) =>
  `<script>ytcfg.set(${JSON.stringify(config)}); window.ytAtN(${JSON.stringify({ R: JSON.stringify({ bgChallenge }) })}); globalThis.unexpectedPageExecution = true;</script>`;
// A synthetic interpreter exercises the real, pinned BotGuardClient and
// WebPoMinter APIs, including their realm-sensitive Function/Uint8Array checks.
const interpreter = `var fixtureInterpreterLocal = true; globalThis.fixtureBotGuard = { a(program, ready) {
  if (program !== 'fixture-program' || yt.config_.EVENT_ID !== 'fixture-event' || !document.createElement || !navigator.userAgent.includes('Mozilla')) throw Error('fixture');
  ready((callback, args) => {
    args[2].push(integrity => {
      if (integrity[0] !== 1) throw Error('fixture');
      return bytes => new Uint8Array(bytes);
    });
    callback('fixture-snapshot');
  }, () => {}, () => {}, () => {});
  return [() => {}];
} };`;

function harness({ html = page(), script = interpreter, integrity = JSON.stringify(['AQID', 600, 60]), network } = {}) {
  const calls = [], failures = [];
  const axios = { create: () => ({ async request(config) {
    calls.push(config);
    assert.equal(config.proxy, false); assert.equal(config.maxRedirects, 0);
    assert.equal(config.responseType, 'text'); assert.ok(config.signal);
    assert.equal(Object.keys(config.headers).some(key => /cookie|authorization/i.test(key)), false);
    if (network) await network(config);
    const data = config.url === 'https://www.youtube.com/' ? html
      : config.url === 'https://www.google.com/js/th/fixture.js' ? script
      : config.url === 'https://jnn-pa.googleapis.com/$rpc/google.internal.waa.v1.Waa/GenerateIT' ? integrity : assert.fail('unexpected route');
    return { data };
  } }) };
  const proof = new EngineProof({ loadAxios: async () => ({ default: axios }), onFailure: (category, detail) => failures.push({ category, detail }) });
  return { proof, calls, failures };
}

test('real MIT libraries initialize once, reuse the minter and restore DOM globals on close', async () => {
  const globals = ['window', 'document', 'location', 'origin', 'navigator', 'yt', 'fixtureBotGuard'];
  const before = globals.map(key => Object.getOwnPropertyDescriptor(globalThis, key));
  const { proof, calls } = harness();
  try {
    const started = Date.now();
    const warm = await proof.warm(context(), signal(), binding());
    assert.ok(warm.expiresAt >= started + 540000 && warm.expiresAt <= Date.now() + 540000);
    for (const id of ['abcdefghijk', 'zyxwvutsrqp']) {
      const result = await proof.mint(id, signal());
      assert.equal(Buffer.from(result.token, 'base64').toString(), id);
      assert.equal(result.expiresAt, warm.expiresAt);
    }
    assert.equal(calls.length, 3);
    assert.equal(JSON.parse(calls[2].data)[1], 'fixture-snapshot');
    assert.equal(globalThis.unexpectedPageExecution, undefined);
    assert.equal(globalThis.fixtureInterpreterLocal, undefined);
    assert.equal(globalThis.yt.config_.EVENT_ID, 'fixture-event');
  } finally { proof.close(); proof.close(); }
  assert.deepEqual(globals.map(key => Object.getOwnPropertyDescriptor(globalThis, key)), before);
});

test('session-bound minting uses the selected binding with the real MIT minter', async () => {
  const { proof, calls } = harness();
  try {
    await proof.warm(context(), signal(), { sessionBinding: 'synthetic-account||', videoBound: false });
    const result = await proof.mint('abcdefghijk', signal());
    assert.equal(Buffer.from(result.token, 'base64').toString(), 'synthetic-account||');
    assert.equal(calls.some(config => JSON.stringify(config).includes('synthetic-account')), false);
  } finally { proof.close(); }
});

test('page parsing accepts escaped loose JSON and rejects malformed or alternate challenges', () => {
  const loose = `ytcfg.set({"EVENT_ID":"fixture-event"});window.ytAtN({R:'${JSON.stringify({ bgChallenge: challenge() })}',});`;
  assert.equal(parseProofPage(loose, parseLooseJSON).globalName, 'fixtureBotGuard');
  for (const html of ['missing', 'ytcfg.set({', page(null), page(challenge(), { LOGGED_IN: true }),
    page({ ...challenge(), globalName: '__proto__' }), page({ ...challenge(), program: '' }),
    ...['https://example.invalid/js/th/a.js', 'http://www.google.com/js/th/a.js', '//www.google.com/elsewhere', 'https://user@www.google.com/js/th/a.js']
      .map(url => page({ ...challenge(), interpreterUrl: { privateDoNotAccessOrElseTrustedResourceUrlWrappedValue: url } })),
    'ytcfg.set({});window.ytAtN({R:(globalThis.unexpectedPageExecution=true)});',
  ]) assert.throws(() => parseProofPage(html, parseLooseJSON), error => error.safeCategory === 'proof_failed');
  assert.equal(globalThis.unexpectedPageExecution, undefined);
});

test('invalid integrity responses and refresh deadlines fail without fallback tokens', async () => {
  for (const values of [null, {}, [], ['', 600, 60], ['AQID', '600', 60], ['AQID', 30, 0], ['AQID', 600, 600], ['AQID', 600, -1], ['AQID', 86401, 0]]) {
    const { proof, failures } = harness({ integrity: JSON.stringify(values) });
    try {
      await assert.rejects(proof.warm(context(), signal(), binding()), error => ['proof_failed', 'proof_expired'].includes(error.safeCategory) && !error.cause);
      assert.equal(failures.length, 1);
      await assert.rejects(proof.mint('abcdefghijk', signal()), { message: 'proof_request_rejected' });
    } finally { proof.close(); }
    assert.equal(globalThis.fixtureBotGuard, undefined);
  }
});

test('network errors stay fixed and abort the active request', async () => {
  const { proof, calls, failures } = harness({ network: async () => { throw Object.assign(new Error('synthetic private response'), { response: { status: 403 } }); } });
  try {
    await assert.rejects(proof.warm(context(), signal(), binding()), { message: 'proof_failed', safePhase: 'network', safeReason: 'http' });
    assert.equal(failures.length, 1); assert.equal(calls[0].signal.aborted, true);
    assert.equal(globalThis.fixtureBotGuard, undefined);
  } finally { proof.close(); }
});

test('close during a pending bootstrap prevents late DOM creation or minting', async () => {
  let release, entered;
  const gate = new Promise(resolve => { release = resolve; });
  const ready = new Promise(resolve => { entered = resolve; });
  const { proof, calls } = harness({ network: async () => { entered(); await gate; } });
  const pending = assert.rejects(proof.warm(context(), signal(), binding()), { message: 'proof_failed' });
  await ready; proof.close(); release(); await pending;
  assert.equal(calls.length, 1); assert.equal(calls[0].signal.aborted, true);
  assert.equal(globalThis.fixtureBotGuard, undefined); assert.equal(globalThis.document, undefined);
});

test('cancelling during interpreter download rejects late responses and leaves no DOM', async () => {
  let release, entered;
  const gate = new Promise(resolve => { release = resolve; });
  const ready = new Promise(resolve => { entered = resolve; });
  const controller = new AbortController();
  const { proof, calls } = harness({ network: async config => {
    if (config.url.includes('/js/th/')) { entered(); await gate; }
  } });
  try {
    const pending = assert.rejects(proof.warm(context(), controller.signal, binding()), { message: 'proof_failed' });
    await ready; controller.abort(); release(); await pending;
    assert.equal(calls.length, 2); assert.equal(calls[1].signal.aborted, true);
    assert.equal(globalThis.document, undefined);
  } finally { release(); proof.close(); }
});
