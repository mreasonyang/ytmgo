import { validateAnonymousRequest } from './sdk-session.mjs';
import { BOOTSTRAP_CATEGORIES } from './bootstrap-diagnostics.mjs';

const MAX_RESPONSE = 16 * 1024 * 1024;
const failure = category => Object.assign(new Error(category), { safeCategory: category });
const stepFor = target => target.pathname === '/sw.js_data' ? 'session_data'
  : target.pathname === '/iframe_api' ? 'player_identifier'
  : target.pathname.startsWith('/s/player/') ? 'player_script' : 'other';
const mimeFor = value => value?.includes('text/html') ? 'html'
  : value?.includes('javascript') ? 'javascript' : value?.includes('json') ? 'json'
  : value?.startsWith('text/') ? 'text' : value ? 'other' : 'missing';

function redirectFor(location, source) {
  if (!location) return 'other';
  try {
    const target = new URL(location, source);
    if (['www.google.com', 'google.com'].includes(target.hostname) && /^\/sorry(?:\/|$)/.test(target.pathname)) return 'google_challenge';
    if (target.hostname === 'accounts.google.com' ||
        (['www.youtube.com', 'youtube.com', 'music.youtube.com', 'www.google.com', 'google.com'].includes(target.hostname) && /^\/signin(?:\/|$)/.test(target.pathname))) return 'signin';
    if (['consent.google.com', 'consent.youtube.com'].includes(target.hostname)) return 'consent';
    return target.origin === source.origin ? 'same_origin' : 'other';
  } catch { return 'other'; }
}

function networkCategory(error, signal) {
  if (signal.aborted) return 'request_cancelled';
  const code = error?.cause?.code ?? error?.code;
  if (['ETIMEDOUT', 'UND_ERR_CONNECT_TIMEOUT', 'UND_ERR_HEADERS_TIMEOUT', 'UND_ERR_BODY_TIMEOUT'].includes(code)) return 'bootstrap_network_timeout';
  if (['CERT_HAS_EXPIRED', 'DEPTH_ZERO_SELF_SIGNED_CERT', 'UNABLE_TO_VERIFY_LEAF_SIGNATURE',
    'SELF_SIGNED_CERT_IN_CHAIN', 'ERR_TLS_CERT_ALTNAME_INVALID'].includes(code)) return 'bootstrap_tls_failed';
  return 'bootstrap_network_failed';
}

// One transport per short-lived bootstrap. Only four small enum/scalar fields
// survive an error; the SDK may wrap it, so retain the first transport failure.
export function createBootstrapFetch(signal, network = globalThis.fetch) {
  let firstFailure, latest = {}, count = 0;
  const retain = (category, details = latest) => {
    const result = Object.assign(failure(category), details);
    firstFailure ??= result;
    return result;
  };
  const boundedFetch = async (input, init = {}) => {
    if (signal.aborted) throw retain('request_cancelled');
    let original;
    try {
      original = new Request(input, init);
      latest = { safeBootstrapStep: stepFor(new URL(original.url)) };
      validateAnonymousRequest(original);
    } catch (error) { throw retain(BOOTSTRAP_CATEGORIES.has(error?.safeCategory) ? error.safeCategory : 'init_failed'); }
    const details = latest;
    const request = new Request(original, {
      signal: AbortSignal.any([original.signal, signal]), credentials: 'omit', redirect: 'manual',
    });
    count++;
    let response;
    try { response = await network(request); }
    catch (error) { throw retain(networkCategory(error, request.signal), details); }
    details.safeHTTPStatus = response.status;
    details.safeMIME = mimeFor(response.headers.get('content-type'));
    if (response.status >= 300 && response.status < 400) {
      details.safeRedirect = redirectFor(response.headers.get('location'), new URL(request.url));
      const rejected = retain('bootstrap_redirect', details);
      await response.body?.cancel().catch(() => {});
      throw rejected;
    }
    if (!response.ok) {
      const rejected = retain([401, 403].includes(response.status) ? 'resolver_access_denied'
        : response.status === 429 ? 'resolver_rate_limited' : 'resolver_http_error', details);
      await response.body?.cancel().catch(() => {});
      throw rejected;
    }
    if (response.url) {
      try {
        validateAnonymousRequest(new Request(response.url));
        if (response.url !== request.url) throw failure('unexpected_request_host');
      } catch (error) {
        const rejected = retain(error.safeCategory, details);
        await response.body?.cancel().catch(() => {}); throw rejected;
      }
    }
    if (!response.body) return response;
    if (Number(response.headers.get('content-length')) > MAX_RESPONSE) {
      const rejected = retain('resolver_response_limit', details);
      await response.body.cancel().catch(() => {}); throw rejected;
    }
    let bytes = 0;
    const reader = response.body.getReader();
    const bounded = new ReadableStream({
      async pull(output) {
        try {
          const { value, done } = await reader.read();
          if (done) { reader.releaseLock(); output.close(); return; }
          bytes += value.byteLength;
          if (bytes > MAX_RESPONSE) {
            const rejected = retain('resolver_response_limit', details);
            await reader.cancel().catch(() => {});
            throw rejected;
          }
          output.enqueue(value);
        } catch (error) {
          output.error(retain(BOOTSTRAP_CATEGORIES.has(error?.safeCategory)
            ? error.safeCategory : networkCategory(error, request.signal), details));
        }
      },
      cancel() { return reader.cancel(); },
    });
    const result = new Response(bounded, { status: response.status, statusText: response.statusText, headers: response.headers });
    Object.defineProperty(result, 'url', { value: response.url });
    return result;
  };
  return {
    fetch: boundedFetch,
    get count() { return count; },
    failure(error) {
      if (firstFailure) return firstFailure;
      return Object.assign(failure(BOOTSTRAP_CATEGORIES.has(error?.safeCategory) ? error.safeCategory : 'init_failed'), latest);
    },
  };
}
