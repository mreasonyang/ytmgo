import test from 'node:test';
import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import { createHash } from 'node:crypto';
import { COOKIE_NAMES, CHALLENGE, SINK, STAGES, validateTab, validateAccount, filterCookies,
  readSelectedAccount, transferSession, failureDescription } from './session-policy.mjs';

const tab = { id: 7, active: true, url: 'https://music.youtube.com/' };
const account = { sessionIndex: '2', delegatedSessionId: 'synthetic-delegated' };
const cookie = (changes = {}) => ({ name: 'SAPISID', value: 'synthetic-only',
  domain: '.youtube.com', path: '/', secure: true, ...changes });

function fixture({ tabs = [tab, tab], accounts = [account, account],
  stores = [{ id: 'synthetic-store', tabIds: [7] }], cookies = [cookie()] } = {}) {
  let queryCount = 0;
  let scriptCount = 0;
  const queryCalls = [];
  const scriptCalls = [];
  const cookieCalls = [];
  const requests = [];
  const browser = {
    tabs: { query: async options => {
      queryCalls.push(options);
      return [tabs[queryCount++]];
    } },
    scripting: { executeScript: async options => {
      scriptCalls.push(options);
      return [{ frameId: 0, result: accounts[scriptCount++] }];
    } },
    cookies: {
      getAllCookieStores: async () => stores,
      get: async options => {
        cookieCalls.push(options);
        return cookies.find(item => item.name === options.name) ?? null;
      },
      getAll: async () => { throw new Error('GET_ALL_MUST_NOT_BE_USED'); },
    },
  };
  const fetchImpl = async (url, options) => {
    requests.push({ url, options });
    return { ok: true, json: async () => url === CHALLENGE ? { token: 's'.repeat(43) } : ({ ok: true, session_received: true }) };
  };
  return { browser, fetchImpl, queryCalls, scriptCalls, cookieCalls, requests };
}

test('only an active HTTPS YouTube Music tab is accepted', () => {
  assert.equal(validateTab(tab), 7);
  for (const url of ['https://www.youtube.com/', 'https://music.youtube.com.evil.test/',
    'http://music.youtube.com/', 'https://music.youtube.com:52963/',
    'https://synthetic@music.youtube.com/', 'brave://extensions/', '']) {
    assert.throws(() => validateTab({ ...tab, url }));
  }
  assert.throws(() => validateTab({ ...tab, active: false }));
  assert.throws(() => validateTab({ ...tab, id: undefined }));
});

test('SESSION_INDEX is required; explicit zero is accepted without a default', () => {
  assert.deepEqual(validateAccount({ sessionIndex: 0 }), { accountIndex: 0 });
  assert.deepEqual(validateAccount({ sessionIndex: '0' }), { accountIndex: 0 });
  assert.deepEqual(validateAccount({ sessionIndex: '99' }), { accountIndex: 99 });
  for (const value of [undefined, null, '', true, false, NaN, -1, 100, 1.1, '01', '2x', ' 2']) {
    assert.throws(() => validateAccount({ sessionIndex: value }));
  }
});

test('delegated account identifier is optional but strictly validated', () => {
  assert.deepEqual(validateAccount(account), { accountIndex: 2, onBehalfOfUser: 'synthetic-delegated' });
  assert.deepEqual(validateAccount({ sessionIndex: 2, delegatedSessionId: '' }), { accountIndex: 2 });
  for (const value of [{}, 42, 'bad value', 'x'.repeat(257), 'synthetic;value']) {
    assert.throws(() => validateAccount({ sessionIndex: 2, delegatedSessionId: value }));
  }
});

test('page script reads only the two named scalar config fields', () => {
  const original = globalThis.ytcfg;
  const calls = [];
  try {
    globalThis.ytcfg = { get: name => {
      calls.push(name);
      return name === 'SESSION_INDEX' ? '2' : 'synthetic-delegated';
    } };
    assert.deepEqual(readSelectedAccount(), account);
    assert.deepEqual(calls, ['SESSION_INDEX', 'DELEGATED_SESSION_ID']);
    delete globalThis.ytcfg;
    assert.deepEqual(readSelectedAccount(), { sessionIndex: null });
  } finally {
    if (original === undefined) delete globalThis.ytcfg;
    else globalThis.ytcfg = original;
  }
});

test('cookie filtering accepts only exact parent domain and excludes other paths and names', () => {
  const selected = filterCookies([
    cookie(), cookie({ name: 'unapproved', value: 'synthetic-unapproved' }),
    cookie({ domain: '.google.com', value: 'synthetic-google' }),
    cookie({ domain: '.youtube.com.evil.test', value: 'synthetic-evil' }),
    cookie({ domain: '.studio.youtube.com', value: 'synthetic-studio' }),
    cookie({ domain: 'youtube.com', value: 'synthetic-host-only' }),
    cookie({ domain: 'music.youtube.com', value: 'synthetic-music' }),
    cookie({ domain: '.music.youtube.com', value: 'synthetic-music-domain' }),
    cookie({ domain: 'www.youtube.com', value: 'synthetic-www' }),
    cookie({ domain: '.www.youtube.com', value: 'synthetic-www-domain' }),
    cookie({ path: '/account', value: 'synthetic-path' }),
  ]);
  assert.deepEqual(selected, [cookie()]);
});

test('only approved fields are serialized for accepted cookies', () => {
  assert.deepEqual(filterCookies([cookie({ storeId: 'synthetic-store', httpOnly: true,
    sameSite: 'unspecified', session: true, ignored: 'synthetic-ignored' })]), [cookie()]);
});

test('expired, malformed-expiry, and partitioned cookies are excluded', () => {
  assert.deepEqual(filterCookies([
    cookie(), cookie({ name: 'SID', expirationDate: 10 }),
    cookie({ name: 'HSID', expirationDate: NaN }),
    cookie({ name: 'SSID', partitionKey: { topLevelSite: 'https://synthetic.test' } }),
  ], 20), [cookie()]);
  assert.deepEqual(filterCookies([cookie({ expirationDate: 30 })], 20), [cookie({ expirationDate: 30 })]);
});

test('same-value duplicate cookies are deduplicated; ambiguous values fail closed', () => {
  assert.deepEqual(filterCookies([cookie(), cookie()]), [cookie()]);
  assert.throws(() => filterCookies([cookie(), cookie({ value: 'synthetic-other' })]));
});

test('missing SAPISID cannot silently use another account mechanism', () => {
  assert.throws(() => filterCookies([cookie({ name: '__Secure-3PAPISID' })]));
  assert.throws(() => filterCookies([]));
  for (const domain of ['youtube.com', 'music.youtube.com', 'www.youtube.com']) {
    assert.throws(() => filterCookies([cookie({ domain })]));
  }
});

test('invalid cookie values, secure flags and excessive sizes are rejected', () => {
  for (const value of ['', 'a;b', 'a\nb', 'a\rb', 'a b', 'a\u0000b', 'a"b', 'a,b', 'a\\b', 'a\u0080b', 2]) {
    assert.throws(() => filterCookies([cookie({ value })]));
  }
  assert.throws(() => filterCookies([cookie({ secure: undefined })]));
  assert.throws(() => filterCookies([cookie({ value: 'x'.repeat(64 * 1024) })]));
});

test('handoff uses the exact sink, selected tab store and allowlisted-name queries', async () => {
  const f = fixture();
  await transferSession(f.browser, f.fetchImpl);
  assert.equal(f.requests.length, 2);
  assert.equal(f.requests[0].url, CHALLENGE);
  assert.equal(f.requests[0].options.method, 'POST');
  assert.equal(f.requests[0].options.body, undefined);
  const request = f.requests[1];
  assert.equal(request.url, SINK);
  assert.equal(request.url, 'http://127.0.0.1:52963/session');
  assert.equal(request.options.redirect, 'error');
  assert.equal(request.options.credentials, 'omit');
  assert.equal(request.options.cache, 'no-store');
  assert.deepEqual(request.options.headers, { 'Content-Type': 'application/json', 'X-Ytmgo-Handoff': 's'.repeat(43) });
  assert.deepEqual(JSON.parse(request.options.body), {
    cookies: [cookie()], accountIndex: 2, onBehalfOfUser: 'synthetic-delegated',
  });
  assert.deepEqual(f.cookieCalls, COOKIE_NAMES.map(name => ({
    url: 'https://music.youtube.com/', storeId: 'synthetic-store', name,
  })));
  assert.ok(f.scriptCalls.every(call => call.world === 'MAIN' && call.target.tabId === 7 &&
    call.target.frameIds.length === 1 && call.target.frameIds[0] === 0));
});

test('tab change during collection prevents transmission', async () => {
  const f = fixture({ tabs: [tab, { ...tab, id: 8 }] });
  await assert.rejects(transferSession(f.browser, f.fetchImpl));
  assert.equal(f.requests.length, 0);
});

test('account index or delegated account change prevents transmission', async () => {
  for (const changed of [{ ...account, sessionIndex: '3' },
    { ...account, delegatedSessionId: 'synthetic-other' }]) {
    const f = fixture({ accounts: [account, changed] });
    await assert.rejects(transferSession(f.browser, f.fetchImpl));
    assert.equal(f.requests.length, 0);
  }
});

test('missing current account config prevents any cookie query or transmission', async () => {
  const f = fixture({ accounts: [{ sessionIndex: null }] });
  await assert.rejects(transferSession(f.browser, f.fetchImpl));
  assert.equal(f.cookieCalls.length, 0);
  assert.equal(f.requests.length, 0);
});

test('missing or ambiguous tab cookie store prevents cookie collection', async () => {
  for (const stores of [[], [{ id: 'a', tabIds: [9] }],
    [{ id: 'a', tabIds: [7] }, { id: 'b', tabIds: [7] }]]) {
    const f = fixture({ stores });
    await assert.rejects(transferSession(f.browser, f.fetchImpl));
    assert.equal(f.cookieCalls.length, 0);
    assert.equal(f.requests.length, 0);
  }
});

test('server failure and invalid acknowledgment are not reported as success', async () => {
  for (const response of [{ ok: false }, { ok: true, json: async () => ({ ok: true }) }]) {
    const f = fixture();
    await assert.rejects(transferSession(f.browser, async () => response));
  }
});

test('progress reports only fixed stage names without account or cookie metadata', async () => {
  const f = fixture();
  const progress = [];
  await transferSession(f.browser, f.fetchImpl, stage => progress.push(stage));
  assert.deepEqual(progress, STAGES);
});

test('failure descriptions identify missing config and rejected cookie scope safely', async () => {
  const cases = [
    { options: { accounts: [{ sessionIndex: null }] }, expected: { stage: 'account', category: 'account_missing' } },
    { options: { accounts: [{ sessionIndex: true }] }, expected: { stage: 'account', category: 'account_invalid' } },
    { options: { stores: [] }, expected: { stage: 'store', category: 'store_unavailable' } },
    { options: { cookies: [cookie({ domain: 'music.youtube.com' })] },
      expected: { stage: 'cookie_filter', category: 'cookie_required_missing' } },
  ];
  for (const { options, expected } of cases) {
    const f = fixture(options);
    await assert.rejects(transferSession(f.browser, f.fetchImpl), error => {
      assert.deepEqual(failureDescription(error), expected);
      assert.deepEqual(Object.keys(error), ['stage', 'category']);
      assert.equal(error.message, 'SESSION_HANDOFF_FAILED');
      assert.equal(error.cause, undefined);
      return true;
    });
  }
});

test('raw browser and network exception messages cannot enter reported status', async () => {
  const f = fixture();
  f.browser.cookies.get = async () => { throw new Error('synthetic-sensitive-exception'); };
  await assert.rejects(transferSession(f.browser, f.fetchImpl), error => {
    assert.deepEqual(failureDescription(error), { stage: 'cookies', category: 'browser_api_failed' });
    assert.doesNotMatch(String(error) + JSON.stringify(error), /synthetic-sensitive-exception/);
    return true;
  });
  const g = fixture();
  await assert.rejects(transferSession(g.browser, async () => { throw new Error('synthetic-network-detail'); }), error => {
    assert.deepEqual(failureDescription(error), { stage: 'challenge', category: 'network_or_timeout' });
    assert.doesNotMatch(String(error) + JSON.stringify(error), /synthetic-network-detail/);
    return true;
  });
  assert.deepEqual(failureDescription({ stage: 'synthetic-untrusted', category: 'synthetic-untrusted' }),
    { stage: 'unknown', category: 'unknown' });
});

test('named get succeeds for authorized Music URL without getAll parent-domain access', async () => {
  const f = fixture();
  let getAllCalls = 0;
  // Model Chromium getAll's extra cookie-origin host check returning no parent cookies.
  f.browser.cookies.getAll = async () => { getAllCalls++; return []; };
  await transferSession(f.browser, f.fetchImpl);
  assert.equal(getAllCalls, 0);
  assert.equal(f.requests.length, 2);
  assert.equal(JSON.parse(f.requests[1].options.body).cookies[0].domain, '.youtube.com');
});

test('named get returning host-only or missing required cookie never broadens or retries', async () => {
  for (const cookies of [[], [cookie({ domain: 'youtube.com' })],
    [cookie({ domain: 'music.youtube.com' })], [cookie({ domain: 'www.youtube.com' })],
    [cookie({ partitionKey: { topLevelSite: 'https://music.youtube.com' } })]]) {
    const f = fixture({ cookies });
    let getAllCalls = 0;
    f.browser.cookies.getAll = async () => { getAllCalls++; return [cookie()]; };
    await assert.rejects(transferSession(f.browser, f.fetchImpl), error => {
      assert.deepEqual(failureDescription(error), { stage: 'cookie_filter', category: 'cookie_required_missing' });
      return true;
    });
    assert.equal(getAllCalls, 0);
    assert.equal(f.requests.length, 0);
    assert.ok(f.cookieCalls.every(call => call.url === 'https://music.youtube.com/' &&
      call.storeId === 'synthetic-store' && COOKIE_NAMES.includes(call.name)));
  }
});

test('manifest pins only public key and the intended minimal permissions', async () => {
  const manifest = JSON.parse(await readFile(new URL('./manifest.json', import.meta.url), 'utf8'));
  assert.deepEqual(manifest.permissions, ['cookies', 'scripting', 'activeTab']);
  assert.deepEqual(manifest.host_permissions, [
    'https://music.youtube.com/*', 'https://www.youtube.com/*', 'http://127.0.0.1:52963/*',
  ]);
  assert.equal(manifest.background, undefined);
  assert.equal(manifest.content_scripts, undefined);
  const id = [...createHash('sha256').update(Buffer.from(manifest.key, 'base64')).digest().subarray(0, 16)]
    .map(byte => String.fromCharCode(97 + (byte >> 4), 97 + (byte & 15))).join('');
  assert.equal(id, 'llinflbckniejeiomanmglbdganiccfk');
});

test('popup starts unchecked, uses a one-attempt guard, and never logs errors', async () => {
  const html = await readFile(new URL('./popup.html', import.meta.url), 'utf8');
  const js = await readFile(new URL('./popup.mjs', import.meta.url), 'utf8');
  assert.match(html, /id="confirm-account" type="checkbox">/);
  assert.match(html, /id="transfer" type="button" disabled/);
  assert.match(js, /confirmation\.checked = false/);
  assert.match(js, /if \(attempted \|\| !confirmation\.checked\) return/);
  assert.match(js, /attempted = true/);
  assert.doesNotMatch(js, /console\.|localStorage|sessionStorage|chrome\.storage|error\.message|error\.stack|error\.cause/);
  assert.match(js, /failureDescription\(error\)/);
});
