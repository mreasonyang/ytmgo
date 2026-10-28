// STDOUT IS PRIVATE BINARY IPC. Cache contents must never enter logs or files.
import { serialize } from 'node:v8';
import { createAnonymousSession, MemoryCache, validateAnonymousRequest } from './sdk-session.mjs';

const write = process.stdout.write.bind(process.stdout);
for (const name of ['log', 'info', 'warn', 'error', 'debug', 'trace', 'dir']) console[name] = () => {};
process.stdout.write = process.stderr.write = () => true;
const MAX_RESPONSE = 16 * 1024 * 1024;
const IPC_LIMIT = 16 * 1024 * 1024;
const CATEGORIES = new Set(['init_failed', 'cache_limit', 'request_cancelled', 'request_timeout',
  'unexpected_authentication', 'unexpected_request_host', 'resolver_response_limit',
  'resolver_http_error', 'resolver_access_denied', 'resolver_rate_limited',
  'player_signature_missing', 'decipher_script_invalid', 'decipher_failed']);
const failure = category => Object.assign(new Error(category), { safeCategory: category });
const controller = new AbortController();
let stopping = false;
let fetchCount = 0;

function stop(code = 0) {
  if (stopping) return;
  stopping = true;
  controller.abort();
  clearTimeout(timer);
  process.exit(code);
}

function respond(reply) {
  if (stopping) return;
  const payload = serialize(reply);
  if (payload.byteLength + 4 > IPC_LIMIT) { stop(2); return; }
  const header = Buffer.alloc(4);
  header.writeUInt32BE(payload.byteLength);
  write(header);
  write(payload, () => stop());
}

const timer = setTimeout(() => stop(2), 30_000);
process.on('SIGTERM', () => stop());
process.on('SIGINT', () => stop());
process.on('uncaughtException', () => stop(2));
process.on('unhandledRejection', () => stop(2));
process.stdout.on('error', () => stop(2));
process.stdin.on('error', () => stop(2));
process.stdin.once('end', () => stop());
process.stdin.resume();

async function boundedFetch(input, init = {}) {
  if (controller.signal.aborted) throw failure('request_cancelled');
  const original = new Request(input, init);
  validateAnonymousRequest(original);
  const request = new Request(original, {
    signal: AbortSignal.any([original.signal, controller.signal]), credentials: 'omit',
  });
  fetchCount++;
  const response = await fetch(request);
  if (!response.ok) {
    await response.body?.cancel();
    if ([401, 403].includes(response.status)) throw failure('resolver_access_denied');
    if (response.status === 429) throw failure('resolver_rate_limited');
    throw failure('resolver_http_error');
  }
  if (response.url) validateAnonymousRequest(new Request(response.url));
  if (!response.body) return response;
  if (Number(response.headers.get('content-length')) > MAX_RESPONSE) {
    await response.body.cancel();
    throw failure('resolver_response_limit');
  }
  let bytes = 0;
  const bounded = response.body.pipeThrough(new TransformStream({
    transform(chunk, output) {
      bytes += chunk.byteLength;
      if (bytes > MAX_RESPONSE) throw failure('resolver_response_limit');
      output.enqueue(chunk);
    },
  }));
  const result = new Response(bounded, { status: response.status, statusText: response.statusText, headers: response.headers });
  Object.defineProperty(result, 'url', { value: response.url });
  return result;
}

try {
  const cache = new MemoryCache();
  const yt = await createAnonymousSession({ cache, fetch: boundedFetch });
  if (controller.signal.aborted) throw failure('request_cancelled');
  const playerID = yt.session.player.player_id;
  if (typeof playerID !== 'string' || !/^[A-Za-z0-9_-]{1,128}$/.test(playerID) || !cache.size) throw failure('init_failed');
  respond({ ok: true, entries: cache.snapshot(), playerID, fetchCount });
} catch (error) {
  respond({ ok: false, category: CATEGORIES.has(error?.safeCategory) ? error.safeCategory : 'init_failed' });
}
