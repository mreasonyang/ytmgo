import test from 'node:test';
import assert from 'node:assert/strict';
import { createBootstrapFetch } from '../bootstrap-fetch.mjs';
import { bootstrapFailureDetails } from '../bootstrap-diagnostics.mjs';

const signal = () => new AbortController().signal;
const metadata = error => ({ category: error.safeCategory, ...bootstrapFailureDetails(error) });

test('public bootstrap redirects stop before a challenge and retain only fixed metadata through SDK wrapping', async () => {
  let calls = 0, cancelled = false;
  const transport = createBootstrapFetch(signal(), async request => {
    calls++; assert.equal(request.redirect, 'manual');
    assert.equal(request.credentials, 'omit');
    return new Response(new ReadableStream({ cancel() { cancelled = true; } }), {
      status: 302, headers: { location: 'https://www.google.com/sorry/index?synthetic=private', 'content-type': 'text/html' },
    });
  });
  let wrapped;
  try { await transport.fetch('https://www.youtube.com/sw.js_data'); }
  catch { wrapped = new Error('synthetic SDK wrapper, never diagnostic'); }
  assert.ok(wrapped); assert.equal(calls, 1); assert.equal(cancelled, true);
  const result = metadata(transport.failure(wrapped));
  assert.deepEqual(result, { category: 'bootstrap_redirect', bootstrapStep: 'session_data', httpStatus: 302, redirect: 'google_challenge', mime: 'html' });
  assert.equal(JSON.stringify(result).includes('synthetic'), false);
});

test('redirect destination taxonomy never follows or emits destination values', async () => {
  for (const [location, redirect] of [
    ['https://accounts.google.com/ServiceLogin?synthetic=private', 'signin'],
    ['https://consent.youtube.com/m?synthetic=private', 'consent'],
    ['/changed?synthetic=private', 'same_origin'],
    ['https://example.invalid/path?synthetic=private', 'other'],
    ['https://example.invalid/signin?synthetic=private', 'other'],
  ]) {
    let calls = 0;
    const transport = createBootstrapFetch(signal(), async () => { calls++; return new Response(null, { status: 302, headers: { location } }); });
    await assert.rejects(transport.fetch('https://www.youtube.com/iframe_api'));
    assert.equal(metadata(transport.failure(new Error('wrapped'))).redirect, redirect);
    assert.equal(calls, 1);
  }
  const absent = createBootstrapFetch(signal(), async () => new Response(new ReadableStream({
    cancel() { throw new Error('synthetic cancel details'); },
  }), { status: 302 }));
  await assert.rejects(absent.fetch('https://www.youtube.com/sw.js_data'));
  assert.deepEqual(metadata(absent.failure(new Error('wrapped'))),
    { category: 'bootstrap_redirect', bootstrapStep: 'session_data', httpStatus: 302, redirect: 'other', mime: 'missing' });
});

test('successful bootstrap response is unchanged and parse failure gets only its closed response metadata', async () => {
  const body = ")]}'\n[[synthetic-response]]";
  const transport = createBootstrapFetch(signal(), async () => new Response(body, { headers: { 'content-type': 'application/json' } }));
  const response = await transport.fetch('https://www.youtube.com/sw.js_data');
  assert.equal(response.status, 200); assert.equal(await response.text(), body); assert.equal(transport.count, 1);
  assert.deepEqual(metadata(transport.failure(new Error('synthetic parse details'))),
    { category: 'init_failed', bootstrapStep: 'session_data', httpStatus: 200, mime: 'json' });
});

test('HTTP and body-read failures preserve the first cause even when the SDK loses it', async () => {
  const denied = createBootstrapFetch(signal(), async () => new Response(null, { status: 403, headers: { 'content-type': 'text/html' } }));
  await assert.rejects(denied.fetch('https://www.youtube.com/s/player/synthetic/base.js'));
  assert.deepEqual(metadata(denied.failure(new Error('synthetic SDK error'))),
    { category: 'resolver_access_denied', bootstrapStep: 'player_script', httpStatus: 403, mime: 'html' });
  const broken = createBootstrapFetch(signal(), async () => new Response(new ReadableStream({
    pull(controller) { controller.error(Object.assign(new Error('synthetic body details'), { code: 'ECONNRESET' })); },
  }), { headers: { 'content-type': 'application/javascript' } }));
  const response = await broken.fetch('https://www.youtube.com/iframe_api');
  await assert.rejects(response.text());
  assert.deepEqual(metadata(broken.failure(new Error('synthetic SDK wrapper'))),
    { category: 'bootstrap_network_failed', bootstrapStep: 'player_identifier', httpStatus: 200, mime: 'javascript' });
});

test('TLS, network timeout and connection errors never retain raw dependency data', async () => {
  for (const [code, category] of [['UNABLE_TO_VERIFY_LEAF_SIGNATURE', 'bootstrap_tls_failed'], ['ETIMEDOUT', 'bootstrap_network_timeout'], ['ECONNRESET', 'bootstrap_network_failed']]) {
    const transport = createBootstrapFetch(signal(), async () => { throw new Error('synthetic private network detail', { cause: { code } }); });
    await assert.rejects(transport.fetch('https://www.youtube.com/sw.js_data'));
    assert.deepEqual(metadata(transport.failure(new Error('synthetic wrapper'))), { category, bootstrapStep: 'session_data' });
  }
});

test('bounded streaming rejects excess content while retaining no response bytes', async () => {
  const transport = createBootstrapFetch(signal(), async () => new Response(new Uint8Array(1), { headers: { 'content-length': String(16 * 1024 * 1024 + 1) } }));
  await assert.rejects(transport.fetch('https://www.youtube.com/iframe_api'));
  assert.equal(transport.failure(new Error('wrapped')).safeCategory, 'resolver_response_limit');
});

test('a redirected response supplied despite manual policy is rejected without accepting its content', async () => {
  const transport = createBootstrapFetch(signal(), async () => {
    const response = new Response('synthetic-private-body', { headers: { 'content-type': 'text/html' } });
    Object.defineProperty(response, 'url', { value: 'https://www.youtube.com/other' });
    return response;
  });
  await assert.rejects(transport.fetch('https://www.youtube.com/sw.js_data'));
  assert.equal(transport.failure(new Error('wrapped')).safeCategory, 'unexpected_request_host');
});
