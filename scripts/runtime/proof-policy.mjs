// Network contract for the anonymous proof helper. There is one bootstrap
// route; account requests, alternate challenge routes and redirects are refused.
export const PROOF_RESPONSE_LIMIT = 16 * 1024 * 1024;
export const PROOF_REQUEST_LIMIT = 256 * 1024;
const rejected = (safeReason = 'policy') => Object.assign(new Error('proof_request_rejected'), { safeCategory: 'proof_request_rejected', safeReason });

export function proofRequest(config, { signal, httpsAgent } = {}) {
  if (!config || typeof config.url !== 'string' || config.url.length > 8192 || config.auth || config.socketPath) throw rejected();
  let target;
  try { target = new URL(config.url, config.baseURL); } catch { throw rejected(); }
  if (target.protocol !== 'https:' || target.port || target.username || target.password || target.hash) throw rejected();
  const method = String(config.method ?? 'get').toUpperCase();
  const allowed =
    (target.hostname === 'www.youtube.com' && target.pathname === '/' && method === 'GET') ||
    (target.hostname === 'www.google.com' && target.pathname.startsWith('/js/th/') && method === 'GET') ||
    (target.hostname === 'jnn-pa.googleapis.com' && target.pathname.endsWith('/GenerateIT') && method === 'POST');
  if (!allowed) throw rejected(
    !['www.youtube.com', 'www.google.com', 'jnn-pa.googleapis.com'].includes(target.hostname) ? 'origin' :
    target.pathname.includes('/att/get') || target.pathname.endsWith('/Create') ? 'challenge' : 'route');
  for (const [key, value] of Object.entries(config.headers ?? {})) {
    if (value != null && typeof value === 'object') throw rejected('authentication');
    if (value != null && ['authorization', 'proxy-authorization', 'cookie'].includes(key.toLowerCase())) throw rejected('authentication');
  }
  return { ...config, url: target.toString(), baseURL: undefined,
    proxy: false, socketPath: undefined, httpAgent: undefined, httpsAgent, transport: undefined,
    adapter: 'http', maxRedirects: 0, timeout: 15_000, signal,
    maxContentLength: PROOF_RESPONSE_LIMIT, maxBodyLength: PROOF_REQUEST_LIMIT,
    validateStatus: status => status >= 200 && status < 300,
  };
}

export function proofContext(value) {
  if (!value || typeof value !== 'object' || Array.isArray(value) ||
      Object.keys(value).some(key => !['client', 'user', 'request'].includes(key))) throw rejected();
  const client = value.client;
  if (!client || client.clientName !== 'WEB_REMIX' ||
      typeof client.visitorData !== 'string' || !client.visitorData || client.visitorData.length > 4096) throw rejected();
  if (JSON.stringify(value).length > 24 * 1024) throw rejected();
  // The parent creates this from an explicitly anonymous SDK session. Omit
  // account/request overrides even if a future caller tries to add them.
  return { client: structuredClone(client), user: { lockedSafetyMode: false }, request: { useSsl: true } };
}
