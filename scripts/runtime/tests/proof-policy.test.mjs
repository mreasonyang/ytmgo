import test from 'node:test';
import assert from 'node:assert/strict';
import { proofRequest, proofContext, PROOF_RESPONSE_LIMIT, PROOF_REQUEST_LIMIT } from '../proof-policy.mjs';

test('proof transport bounds requests and rejects accounts, redirects and alternate routes', () => {
  const signal = new AbortController().signal;
  const agent = {};
  for (const [url, method] of [
    ['https://www.youtube.com/', 'get'],
    ['https://www.google.com/js/th/fixture.js', 'get'],
    ['https://jnn-pa.googleapis.com/fixture/GenerateIT', 'post'],
  ]) {
    const request = proofRequest({ url, method, maxRedirects: 5, timeout: 0, proxy: {} }, { signal, httpsAgent: agent });
    assert.equal(request.signal, signal);
    assert.equal(request.httpsAgent, agent);
    assert.equal(request.proxy, false);
    assert.equal(request.maxRedirects, 0);
    assert.equal(request.timeout, 15_000);
    assert.equal(request.maxContentLength, PROOF_RESPONSE_LIMIT);
    assert.equal(request.maxBodyLength, PROOF_REQUEST_LIMIT);
    assert.equal(request.validateStatus(302), false);
  }
  for (const config of [
    { url: 'http://www.youtube.com/' }, { url: 'https://example.com/' },
    { url: 'https://www.youtube.com/youtubei/v1/player' },
    { url: 'https://www.google.com/other' },
    { url: 'https://www.youtube.com/', method: 'post' },
    ...['Authorization', 'Cookie', 'Proxy-Authorization'].map(key => ({ url: 'https://www.youtube.com/', headers: { [key]: 'fixture' } })),
    { url: 'https://www.youtube.com/', auth: {} },
    { url: 'https://www.youtube.com/', socketPath: '/tmp/fixture' },
    { url: 'https://www.youtube.com/', headers: { common: { Cookie: 'fixture' } } },
  ]) assert.throws(() => proofRequest(config), { message: 'proof_request_rejected' });
});

test('proof context keeps the anonymous client and removes request/account overrides', () => {
  const input = { client: { clientName: 'WEB_REMIX', visitorData: 'fixture' }, user: { fixture: true }, request: { fixture: true } };
  const output = proofContext(input);
  assert.deepEqual(output.user, { lockedSafetyMode: false });
  assert.deepEqual(output.request, { useSsl: true });
  input.client.visitorData = 'changed';
  assert.equal(output.client.visitorData, 'fixture');
  for (const value of [{}, [], { ...input, cookie: 'fixture' }, { client: { clientName: 'WEB', visitorData: 'fixture' } }]) {
    assert.throws(() => proofContext(value), { message: 'proof_request_rejected' });
  }
});
