// Account credentials are private IPC input and memory only. Errors contain
// fixed categories; no account value is ever part of a response or diagnostic.
const failure = category => Object.assign(new Error(category), { safeCategory: category });
const record = value => value !== null && typeof value === 'object' && !Array.isArray(value);
const COOKIE_NAMES = new Set([
  'SAPISID', 'APISID', 'SID', 'HSID', 'SSID', 'LOGIN_INFO', 'SIDCC',
  '__Secure-1PAPISID', '__Secure-3PAPISID', '__Secure-1PSID', '__Secure-3PSID',
  '__Secure-1PSIDTS', '__Secure-3PSIDTS', '__Secure-1PSIDCC', '__Secure-3PSIDCC',
  'PREF', 'VISITOR_INFO1_LIVE', 'VISITOR_PRIVACY_METADATA', 'YSC', 'CONSENT', 'SOCS',
]);
export const SESSION_LINE_LIMIT = 96 * 1024;

export function validateAccountSession(value) {
  if (!record(value) || Object.keys(value).some(key => !['loggedIn', 'musicPremium', 'cookie', 'accountIndex', 'onBehalfOfUser', 'revision'].includes(key)) ||
      typeof value.loggedIn !== 'boolean' || typeof value.musicPremium !== 'boolean' ||
      typeof value.cookie !== 'string' || value.cookie.length > 64 * 1024 || /[^\x20-\x7e]/.test(value.cookie) ||
      !Number.isSafeInteger(value.accountIndex) || value.accountIndex < 0 || value.accountIndex > 99 ||
      !Number.isSafeInteger(value.revision) || value.revision <= 0 ||
      (value.onBehalfOfUser !== undefined && (typeof value.onBehalfOfUser !== 'string' || !/^[A-Za-z0-9_-]{0,128}$/.test(value.onBehalfOfUser)))) throw failure('invalid_request');
  if (!value.loggedIn) {
    if (value.musicPremium || value.cookie || value.accountIndex !== 0 || value.onBehalfOfUser) throw failure('unexpected_authentication');
  } else {
    const names = new Set();
    for (const part of value.cookie.split(';')) {
      const pair = /^([A-Za-z0-9_-]+)=([\x21\x23-\x2B\x2D-\x3A\x3C-\x5B\x5D-\x7E]+)$/.exec(part.trim());
      if (!pair || !COOKIE_NAMES.has(pair[1]) || names.has(pair[1]) || pair[2].length > 16 * 1024) throw failure('unexpected_authentication');
      names.add(pair[1]);
    }
    if (!names.has('SAPISID')) throw failure('login_required');
  }
  return { loggedIn: value.loggedIn, musicPremium: value.musicPremium, cookie: value.cookie,
    accountIndex: value.accountIndex, onBehalfOfUser: value.onBehalfOfUser ?? '', revision: value.revision };
}

function flag(response, serviceName, key) {
  const services = response?.responseContext?.serviceTrackingParams;
  if (!Array.isArray(services)) return false;
  let seen = false;
  for (const service of services) {
    if (service?.service !== serviceName) continue;
    if (!Array.isArray(service.params)) return false;
    for (const item of service.params) {
      if (item?.key !== key) continue;
      if (item.value !== '1') return false;
      seen = true;
    }
  }
  return seen;
}

export function hasAcceptedAccount(response) {
  return record(response) && !response.error && !response.responseContext?.errors &&
    response.responseContext?.mainAppWebResponseContext?.loggedOut !== true &&
    flag(response, 'GFEEDBACK', 'logged_in') && flag(response, 'CSI', 'yt_li') && record(response.contents);
}

// Only the SDK's browse/player APIs may carry an account. Bootstrap is public;
// proof has its own narrower transport. Redirects are never followed here.
export async function validateSessionRequest(request, session) {
  const target = new URL(request.url);
  if (target.protocol !== 'https:' || target.username || target.password || target.port || target.hash ||
      !['www.youtube.com', 'music.youtube.com', 'youtube.com', 'youtubei.googleapis.com', 'www.google.com'].includes(target.hostname)) throw failure('unexpected_request_host');
  if (/(?:\/att\/|\/jnn\/|\/js\/th\/|GenerateIT|CreateChallenge)/.test(target.pathname) ||
      target.searchParams.has('pot') || target.searchParams.has('po_token') || request.headers.has('proxy-authorization')) throw failure('unexpected_authentication');
  const api = ['www.youtube.com', 'music.youtube.com'].includes(target.hostname) && /^\/youtubei\/v1\/(?:browse|player)$/.test(target.pathname);
  const proofConfig = !session.musicPremium && target.hostname === 'music.youtube.com' && target.pathname === '/' && request.method === 'GET' &&
    target.search === '';
  const names = request.headers.get('cookie')?.split(';').map(part => part.trim().split('=', 1)[0]).filter(Boolean) ?? [];
  if (session.loggedIn && (api || proofConfig)) {
    if (request.headers.get('cookie') !== session.cookie || (api && !request.headers.get('authorization')) ||
        request.headers.get('x-goog-authuser') !== String(session.accountIndex) ||
        (request.headers.get('x-goog-pageid') ?? '') !== session.onBehalfOfUser) throw failure('unexpected_authentication');
  } else if (request.headers.has('authorization') || names.some(name => !['PREF', 'VISITOR_INFO1_LIVE'].includes(name))) throw failure('unexpected_authentication');
  if (api) {
    if (request.method !== 'POST') throw failure('unexpected_client');
    let body;
    try { body = await request.clone().json(); } catch { throw failure('unexpected_client'); }
    if (body?.context?.client?.clientName !== 'WEB_REMIX' || body.serviceIntegrityDimensions || body.context?.serviceIntegrityDimensions || body.poToken || body.po_token) throw failure('unexpected_client');
  }
  return api;
}
