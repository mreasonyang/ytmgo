import test from 'node:test';
import assert from 'node:assert/strict';
import { parseProofBinding } from '../proof-binding.mjs';
import { EngineProof } from '../engine-proof.mjs';
import { PlaybackEngine } from '../engine.mjs';

const account = { loggedIn: true, musicPremium: false, cookie: 'SAPISID=synthetic', accountIndex: 2, onBehalfOfUser: '', revision: 1 };
const config = () => ({ LOGGED_IN: true, SESSION_INDEX: '2', DATASYNC_ID: 'synthetic-account||', VISITOR_DATA: 'synthetic-visitor',
  WEB_PLAYER_CONTEXT_CONFIGS: { music: { serializedExperimentFlags: 'other=true&html5_generate_content_po_token=false' } } });
const html = value => `<!doctype html><script>ytcfg.set(${JSON.stringify(value)});</script>`;
const signal = () => new AbortController().signal;
const category = value => error => error.safeCategory === value;

test('ordinary account binding is from selected server session, never anonymous visitor', () => {
  const binding = parseProofBinding(html(config()), account);
  assert.equal(binding.sessionBinding, 'synthetic-account||'); assert.equal(binding.videoBound, false);
  const enabled = config(); enabled.WEB_PLAYER_CONTEXT_CONFIGS.music.serializedExperimentFlags = 'html5_generate_content_po_token=true';
  assert.equal(parseProofBinding(html(enabled), account).videoBound, true);
  const noExperiment = config(); delete noExperiment.WEB_PLAYER_CONTEXT_CONFIGS;
  assert.equal(parseProofBinding(html(noExperiment), account).videoBound, false);
  const delegated = { ...config(), DATASYNC_ID: 'synthetic-channel||synthetic-primary' };
  assert.equal(parseProofBinding(html(delegated), { ...account, onBehalfOfUser: 'synthetic-channel' }).sessionBinding, delegated.DATASYNC_ID);
  for (const value of [{ ...config(), LOGGED_IN: false }, { ...config(), SESSION_INDEX: 0 }, { ...config(), DATASYNC_ID: undefined },
    { ...config(), DATASYNC_ID: 'invalid space||' }, delegated, { ...config(), VISITOR_DATA: undefined },
  ]) assert.throws(() => parseProofBinding(html(value), account), category('proof_request_rejected'));
  const anonymous = parseProofBinding(html({ LOGGED_IN: false, VISITOR_DATA: 'anonymous-visitor' }), { loggedIn: false });
  assert.equal(anonymous.sessionBinding, 'anonymous-visitor'); assert.equal(anonymous.videoBound, false);
});

test('provider mints exact account or video bindings with no account cookie transfer', async () => {
  for (const videoBound of [false, true]) {
    const bindings = [];
    const axios = { interceptors: { request: { use() {}, eject() {} }, response: { use() {}, eject() {} } } };
    class SessionManager {
      async generatePoToken(binding, proxy, bypass, address, tls, challenge, context) {
        bindings.push(binding); assert.equal(context.cookie, undefined); assert.equal(context.user.onBehalfOfUser, undefined);
        return { contentBinding: binding, poToken: 'synthetic-proof', expiresAt: new Date(Date.now() + 120000) };
      }
      invalidateCaches() {}
    }
    const proof = new EngineProof({ loadAxios: async () => ({ default: axios }), loadProvider: async () => ({ SessionManager }) });
    try {
      await proof.warm({ client: { clientName: 'WEB_REMIX', visitorData: 'synthetic-visitor' } }, signal(), { sessionBinding: 'synthetic-account||', videoBound });
      await proof.mint('abcdefghijk', signal()); await proof.mint('zyxwvutsrqp', signal());
      assert.deepEqual(bindings, videoBound ? ['synthetic-account||', 'abcdefghijk', 'zyxwvutsrqp'] : ['synthetic-account||', 'synthetic-account||', 'synthetic-account||']);
    } finally { proof.close(); }
  }
});

test('ordinary engine fetches selected official account config before proof; redirects never forward cookies', async () => {
  const accepted = { contents: {}, responseContext: { serviceTrackingParams: [
    { service: 'GFEEDBACK', params: [{ key: 'logged_in', value: '1' }] }, { service: 'CSI', params: [{ key: 'yt_li', value: '1' }] },
  ] } };
  for (const redirect of [false, true]) {
    let calls = 0, proofCalled = 0;
    const engine = new PlaybackEngine({
      createSession: async () => ({ session: { player: { signature_timestamp: 1 }, context: { client: { clientName: 'WEB_REMIX', visitorData: 'old-anonymous-visitor' } } }, actions: { execute: async () => ({ data: accepted }) } }),
      network: async request => {
        calls++; assert.equal(request.url, 'https://music.youtube.com/');
        assert.equal(request.headers.get('cookie'), account.cookie); assert.equal(request.headers.get('x-goog-authuser'), '2'); assert.equal(request.redirect, 'manual');
        return redirect ? new Response(null, { status: 302, headers: { location: 'https://example.invalid/never-requested' } }) : new Response(html(config()));
      },
      proof: { async warm(context, _signal, binding) { proofCalled++; assert.equal(context.client.visitorData, 'synthetic-visitor'); assert.equal(binding.sessionBinding, 'synthetic-account||'); return { expiresAt: Date.now() + 120000 }; }, close() {} },
    });
    try {
      const work = engine.warm({ entries: [['synthetic', new ArrayBuffer(1)]], playerID: 'fixture', session: account }, signal());
      if (redirect) await assert.rejects(work, error => category('unexpected_request_host')(error) && error.safeStage === 'proof_config_request'); else await work;
      assert.equal(calls, 1); assert.equal(proofCalled, redirect ? 0 : 1);
    } finally { engine.close(); }
  }
});

test('canonical account config cannot select another account or brand before proof creation', async () => {
  const selected = { ...account, onBehalfOfUser: 'synthetic-channel' };
  const accepted = { contents: {}, responseContext: { serviceTrackingParams: [
    { service: 'GFEEDBACK', params: [{ key: 'logged_in', value: '1' }] }, { service: 'CSI', params: [{ key: 'yt_li', value: '1' }] },
  ] } };
  for (const [changes, reason] of [
    [{ SESSION_INDEX: '0', DATASYNC_ID: 'synthetic-channel||synthetic-primary' }, 'account_mismatch'],
    [{ DATASYNC_ID: 'synthetic-other-channel||synthetic-primary' }, 'brand_mismatch'],
    [{ LOGGED_IN: false }, 'login_unverified'],
  ]) {
    let calls = 0, proofCalls = 0;
    const engine = new PlaybackEngine({
      createSession: async () => ({ session: { player: { signature_timestamp: 1 }, context: { client: { clientName: 'WEB_REMIX' } } }, actions: { execute: async () => ({ data: accepted }) } }),
      network: async request => {
        calls++; assert.equal(request.url, 'https://music.youtube.com/');
        assert.equal(request.headers.get('x-goog-authuser'), '2');
        assert.equal(request.headers.get('x-goog-pageid'), 'synthetic-channel');
        return new Response(html({ ...config(), ...changes }));
      },
      proofFactory: async () => { proofCalls++; throw new Error('synthetic-not-reached'); },
    });
    try {
      await assert.rejects(engine.warm({ entries: [['synthetic', new ArrayBuffer(1)]], playerID: 'fixture', session: selected }, signal()),
        error => error.safeCategory === 'proof_request_rejected' && error.safeStage === 'proof_config_parse' && error.safeReason === reason && error.message === 'proof_request_rejected');
      assert.equal(calls, 1); assert.equal(proofCalls, 0);
    } finally { engine.close(); }
  }
});
