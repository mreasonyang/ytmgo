export const CHALLENGE = 'http://127.0.0.1:52963/challenge';
export const SINK = 'http://127.0.0.1:52963/session';

export const COOKIE_NAMES = Object.freeze([
  'SAPISID', 'APISID', 'SID', 'HSID', 'SSID', 'LOGIN_INFO', 'SIDCC',
  '__Secure-1PAPISID', '__Secure-3PAPISID', '__Secure-1PSID', '__Secure-3PSID',
  '__Secure-1PSIDTS', '__Secure-3PSIDTS', '__Secure-1PSIDCC', '__Secure-3PSIDCC',
  'PREF', 'VISITOR_INFO1_LIVE', 'VISITOR_PRIVACY_METADATA', 'YSC', 'CONSENT', 'SOCS',
]);
const names = new Set(COOKIE_NAMES);

export const STAGES = Object.freeze(['tab', 'account', 'store', 'cookies', 'cookie_filter',
  'recheck', 'challenge', 'post', 'ack', 'complete']);
const stages = new Set(STAGES);
const categories = new Set(['validation', 'tab_invalid', 'account_missing', 'account_invalid',
  'store_unavailable', 'cookie_invalid', 'cookie_conflict', 'cookie_limit', 'cookie_required_missing',
  'account_changed', 'browser_api_failed', 'network_or_timeout', 'post_rejected', 'ack_invalid', 'unknown']);
class PolicyError extends Error {
  constructor(category) { super('SESSION_VALIDATION_FAILED'); this.category = category; }
}
class HandoffError extends Error {
  constructor(stage, category) { super('SESSION_HANDOFF_FAILED'); this.stage = stage; this.category = category; }
}
function reject(category = 'validation') { throw new PolicyError(category); }

export function failureDescription(error) {
  if (error instanceof HandoffError && stages.has(error.stage) && categories.has(error.category)) {
    return { stage: error.stage, category: error.category };
  }
  return { stage: 'unknown', category: 'unknown' };
}

export function validateTab(tab) {
  if (!tab || !Number.isSafeInteger(tab.id) || tab.id < 0 || tab.active !== true) reject('tab_invalid');
  let url;
  try { url = new URL(tab.url); } catch { reject('tab_invalid'); }
  if (url.protocol !== 'https:' || url.hostname !== 'music.youtube.com' || url.port ||
      url.username || url.password) reject('tab_invalid');
  return tab.id;
}

export function validateAccount(raw) {
  if (!raw || typeof raw !== 'object' || raw.sessionIndex === undefined || raw.sessionIndex === null) reject('account_missing');
  const value = raw.sessionIndex;
  const index = typeof value === 'string' && /^(0|[1-9]\d?)$/.test(value) ? Number(value) : value;
  if (!Number.isInteger(index) || index < 0 || index > 99) reject('account_invalid');
  const account = { accountIndex: index };
  const delegated = raw.delegatedSessionId;
  if (delegated !== undefined && delegated !== null && delegated !== '') {
    if (typeof delegated !== 'string' || !/^[A-Za-z0-9_-]{1,128}$/.test(delegated)) reject('account_invalid');
    account.onBehalfOfUser = delegated;
  }
  return account;
}

export function filterCookies(raw, nowSeconds = Date.now() / 1000) {
  if (!Array.isArray(raw)) reject('cookie_invalid');
  const selected = [];
  const seen = new Map();
  let bytes = 0;
  for (const cookie of raw) {
    if (!cookie || !names.has(cookie.name) || typeof cookie.domain !== 'string') continue;
    // The SDK flattens these cookies into one header for both Music and www APIs.
    // Only a parent-domain cookie already valid for both hosts may cross this boundary.
    if (cookie.domain !== '.youtube.com' || cookie.path !== '/' || cookie.partitionKey) continue;
    if (cookie.expirationDate !== undefined &&
        (!Number.isFinite(cookie.expirationDate) || cookie.expirationDate <= nowSeconds)) continue;
    if (typeof cookie.value !== 'string' || !/^[\x21\x23-\x2b\x2d-\x3a\x3c-\x5b\x5d-\x7e]+$/.test(cookie.value) ||
        typeof cookie.secure !== 'boolean') reject('cookie_invalid');
    if (seen.has(cookie.name)) {
      if (seen.get(cookie.name) !== cookie.value) reject('cookie_conflict');
      continue;
    }
    seen.set(cookie.name, cookie.value);
    const item = { name: cookie.name, value: cookie.value, domain: cookie.domain,
      path: '/', secure: cookie.secure };
    if (cookie.expirationDate !== undefined) item.expirationDate = cookie.expirationDate;
    bytes += new TextEncoder().encode(cookie.name + cookie.value).length;
    if (bytes > 64 * 1024 || selected.length >= 100) reject('cookie_limit');
    selected.push(item);
  }
  if (!seen.has('SAPISID')) reject('cookie_required_missing');
  return selected;
}

// Serialized by chrome.scripting. This function must remain self-contained.
// Only these two scalar configuration fields may leave the page context.
export function readSelectedAccount() {
  const config = globalThis.ytcfg;
  if (!config || typeof config.get !== 'function') return { sessionIndex: null };
  const sessionIndex = config.get('SESSION_INDEX');
  const delegatedSessionId = config.get('DELEGATED_SESSION_ID');
  return {
    sessionIndex: typeof sessionIndex === 'string' || typeof sessionIndex === 'number' ? sessionIndex : null,
    delegatedSessionId: typeof delegatedSessionId === 'string' ? delegatedSessionId : null,
  };
}

async function activeTab(browser) {
  const tabs = await browser.tabs.query({ active: true, currentWindow: true });
  if (tabs.length !== 1) reject('tab_invalid');
  validateTab(tabs[0]);
  return tabs[0];
}

async function accountFromTab(browser, tabId) {
  const results = await browser.scripting.executeScript({
    target: { tabId, frameIds: [0] }, world: 'MAIN', func: readSelectedAccount,
  });
  if (!Array.isArray(results) || results.length !== 1 || results[0].frameId !== 0) reject('account_missing');
  return validateAccount(results[0].result);
}

export async function transferSession(browser, fetchImpl, onStage = () => {}) {
  let stage = 'tab';
  const enter = next => { stage = next; onStage(next); };
  try {
  enter('tab');
  const initial = await activeTab(browser);
  enter('account');
  const account = await accountFromTab(browser, initial.id);
  enter('store');
  const stores = await browser.cookies.getAllCookieStores();
  const matchingStores = stores.filter(store => Array.isArray(store.tabIds) && store.tabIds.includes(initial.id));
  if (matchingStores.length !== 1 || typeof matchingStores[0].id !== 'string') reject('store_unavailable');
  // Query only allowlisted names for the intended YouTube Music URL and tab store.
  // No Google-wide or browser-wide cookie query is performed.
  enter('cookies');
  // get() resolves this exact authorized URL's named cookie. Unlike getAll(),
  // it does not require an additional parent-domain host grant to return a
  // domain cookie already applicable to the Music URL. The strict domain/path
  // filter below remains authoritative; no broader query or retry is used.
  const records = await Promise.all(COOKIE_NAMES.map(name => browser.cookies.get({
    url: 'https://music.youtube.com/', storeId: matchingStores[0].id, name,
  })));
  enter('cookie_filter');
  const cookies = filterCookies(records.filter(Boolean));
  enter('recheck');
  const current = await activeTab(browser);
  if (current.id !== initial.id) reject('tab_invalid');
  const currentAccount = await accountFromTab(browser, initial.id);
  if (currentAccount.accountIndex !== account.accountIndex ||
      currentAccount.onBehalfOfUser !== account.onBehalfOfUser) reject('account_changed');
  const body = JSON.stringify({ cookies, ...account });
  if (new TextEncoder().encode(body).length > 128 * 1024) reject('cookie_limit');
  enter('challenge');
  const challengeResponse = await fetchImpl(CHALLENGE, { method: 'POST', headers: { 'Content-Type': 'application/json' }, credentials: 'omit',
    cache: 'no-store', redirect: 'error', referrerPolicy: 'no-referrer', signal: AbortSignal.timeout(5000) });
  if (!challengeResponse.ok) reject('post_rejected');
  const challenge = await challengeResponse.json();
  if (typeof challenge?.token !== 'string' || !/^[A-Za-z0-9_-]{43}$/.test(challenge.token)) reject('ack_invalid');
  enter('post');
  const response = await fetchImpl(SINK, {
    method: 'POST', headers: { 'Content-Type': 'application/json', 'X-Ytmgo-Handoff': challenge.token },
    credentials: 'omit', cache: 'no-store', redirect: 'error', referrerPolicy: 'no-referrer',
    signal: AbortSignal.timeout(15000), body,
  });
  if (!response.ok) reject('post_rejected');
  enter('ack');
  const result = await response.json();
  if (result?.ok !== true || result?.session_received !== true) reject('ack_invalid');
  enter('complete');
  } catch (error) {
    // Never retain the original error or its message/cause; it may include session data.
    const category = error instanceof PolicyError && categories.has(error.category) ? error.category :
      (stage === 'post' || stage === 'challenge') ? 'network_or_timeout' : stage === 'ack' ? 'ack_invalid' : 'browser_api_failed';
    throw new HandoffError(stage, category);
  }
}
