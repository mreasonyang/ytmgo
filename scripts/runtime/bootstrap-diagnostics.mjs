// Closed metadata vocabulary for the public bootstrap only. Never add URLs,
// headers, response text, cache bytes, player IDs or arbitrary error messages.
export const BOOTSTRAP_STEPS = new Set(['session_data', 'player_identifier', 'player_script', 'other']);
export const BOOTSTRAP_REDIRECTS = new Set(['google_challenge', 'signin', 'consent', 'same_origin', 'other']);
export const BOOTSTRAP_MIME = new Set(['html', 'json', 'javascript', 'text', 'other', 'missing']);
export const BOOTSTRAP_CATEGORIES = new Set(['init_failed', 'cache_limit', 'request_cancelled', 'request_timeout',
  'unexpected_authentication', 'unexpected_request_host', 'resolver_response_limit',
  'resolver_http_error', 'resolver_access_denied', 'resolver_rate_limited',
  'player_signature_missing', 'decipher_script_invalid', 'decipher_failed',
  'bootstrap_redirect', 'bootstrap_network_failed', 'bootstrap_network_timeout', 'bootstrap_tls_failed']);
const status = value => Number.isSafeInteger(value) && value >= 100 && value <= 599;

export function bootstrapFailureDetails(error) {
  const result = {};
  if (BOOTSTRAP_STEPS.has(error?.safeBootstrapStep)) result.bootstrapStep = error.safeBootstrapStep;
  if (status(error?.safeHTTPStatus)) result.httpStatus = error.safeHTTPStatus;
  if (BOOTSTRAP_REDIRECTS.has(error?.safeRedirect)) result.redirect = error.safeRedirect;
  if (BOOTSTRAP_MIME.has(error?.safeMIME)) result.mime = error.safeMIME;
  return result;
}

export function validBootstrapFailureDetails(reply) {
  return (!Object.hasOwn(reply, 'bootstrapStep') || BOOTSTRAP_STEPS.has(reply.bootstrapStep)) &&
    (!Object.hasOwn(reply, 'httpStatus') || status(reply.httpStatus)) &&
    (!Object.hasOwn(reply, 'redirect') || BOOTSTRAP_REDIRECTS.has(reply.redirect)) &&
    (!Object.hasOwn(reply, 'mime') || BOOTSTRAP_MIME.has(reply.mime));
}

export function bootstrapErrorDetails(reply) {
  return { safeBootstrapStep: reply.bootstrapStep, safeHTTPStatus: reply.httpStatus,
    safeRedirect: reply.redirect, safeMIME: reply.mime };
}
