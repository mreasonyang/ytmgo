// Signed upstream capabilities stay in this process. Only opaque loopback
// capabilities cross the private IPC pipe. There is no disk media cache.
import http from 'node:http';
import { randomBytes } from 'node:crypto';
import { once } from 'node:events';

export const STREAM_HEADERS = Object.freeze({
  accept: '*/*', origin: 'https://www.youtube.com',
  referer: 'https://www.youtube.com', DNT: '?1',
});
const CHUNK_BYTES = 64 * 1024;
const NETWORK_IDLE_MS = 20_000;
const MAX_MEDIA_MS = 26 * 60 * 60_000;
export const STREAM_ERROR_HEADER = 'X-Ytmgo-Stream-Error';
const STREAM_ERRORS = new Set(['upstream_access_denied', 'upstream_rate_limited', 'upstream_http_error',
  'upstream_timeout', 'upstream_connection_failed', 'upstream_range_mismatch', 'upstream_truncated']);
const error = category => Object.assign(new Error(category), { safeCategory: category });

export function parseByteRange(value, total) {
  if (!Number.isSafeInteger(total) || total <= 0) return null;
  if (value == null) return { start: 0, end: total - 1, partial: false };
  if (typeof value !== 'string') return null;
  const match = /^bytes=(\d*)-(\d*)$/.exec(value.trim());
  if (!match || (!match[1] && !match[2])) return null;
  let start, end;
  if (!match[1]) {
    const suffix = Number(match[2]);
    if (!Number.isSafeInteger(suffix) || suffix <= 0) return null;
    start = Math.max(0, total - suffix);
    end = total - 1;
  } else {
    start = Number(match[1]);
    end = match[2] ? Number(match[2]) : total - 1;
    if (!Number.isSafeInteger(start) || !Number.isSafeInteger(end) || start > end || start >= total) return null;
    end = Math.min(end, total - 1);
  }
  return { start, end, partial: true };
}

export class StreamBridge {
  #server;
  #starting;
  #media = new Map();
  #handlers = new Set();
  #closed = false;
  #closing;
  #fetch;
  #idleTimeoutMS;

  constructor({ fetch = globalThis.fetch, idleTimeoutMS = NETWORK_IDLE_MS } = {}) {
    if (!Number.isSafeInteger(idleTimeoutMS) || idleTimeoutMS < 1 || idleTimeoutMS > NETWORK_IDLE_MS) throw error('invalid_request');
    this.#fetch = fetch; this.#idleTimeoutMS = idleTimeoutMS;
  }
  get size() { return this.#media.size; }
  get activeRequests() { return this.#handlers.size; }

  async #start() {
    if (this.#server) return;
    if (this.#starting) return this.#starting;
    this.#starting = (async () => {
      const server = http.createServer((request, response) => {
        this.#serve(request, response).catch(() => {
          if (response.headersSent) response.destroy();
          else { response.writeHead(502, { 'Content-Length': '0' }); response.end(); }
        });
      });
      server.requestTimeout = 30_000;
      server.headersTimeout = 10_000;
      server.keepAliveTimeout = 1000;
      server.maxHeadersCount = 32;
      server.maxConnections = 8;
      server.on('clientError', (_error, socket) => socket.destroy());
      const listening = once(server, 'listening');
      server.listen(0, '127.0.0.1');
      try { await listening; }
      catch { server.close(); throw error('bridge_start_failed'); }
      this.#server = server;
      server.on('error', () => { void this.close(); });
    })();
    try { await this.#starting; } finally { this.#starting = undefined; }
  }

  async publish({ url, length, contentType, duration }) {
    if (this.#closed) throw error('bridge_closed');
    if (this.size >= 4) throw error('capacity_limit');
    if (!Number.isSafeInteger(length) || length <= 0 || length > 64 * 1024 ** 3) throw error('invalid_content_length');
    if (!Number.isFinite(duration) || duration <= 0 || duration > 86400) throw error('invalid_duration');
    if (!['audio/webm', 'audio/mp4', 'video/mp4'].includes(contentType)) throw error('invalid_media_format');
    const remote = new URL(url);
    if (remote.protocol !== 'https:' || !remote.hostname.endsWith('.googlevideo.com') || remote.port || remote.username || remote.password || remote.hash) throw error('invalid_media_url');
    const expires = Number(remote.searchParams.get('expire')) * 1000;
    const lifetime = Math.min(MAX_MEDIA_MS, expires - Date.now() - 30_000);
    if (!Number.isFinite(lifetime) || lifetime <= 0) throw error('media_expired');
    await this.#start();
    if (this.#closed) throw error('bridge_closed');
    if (this.size >= 4) throw error('capacity_limit');
    const path = '/' + randomBytes(32).toString('hex');
    const media = { url, length, contentType, duration, path, failure: '', controller: new AbortController() };
    media.timer = setTimeout(() => this.#revoke(path), lifetime);
    media.timer.unref();
    this.#media.set(path, media);
    return `http://127.0.0.1:${this.#server.address().port}${path}`;
  }

  #revoke(path) {
    const media = this.#media.get(path);
    if (!media) return false;
    this.#media.delete(path);
    clearTimeout(media.timer);
    media.controller.abort();
    media.url = '';
    return true;
  }

  release(value) {
    let url;
    try { url = new URL(value); } catch { throw error('invalid_capability'); }
    if (url.protocol !== 'http:' || url.hostname !== '127.0.0.1' || url.username || url.password || url.search || url.hash || !/^\/[a-f0-9]{64}$/.test(url.pathname)) throw error('invalid_capability');
    if (!this.#server || url.port !== String(this.#server.address()?.port)) throw error('invalid_capability');
    return this.#revoke(url.pathname);
  }

  revokeAll() {
    for (const path of this.#media.keys()) this.#revoke(path);
  }

  async #serve(request, response) {
    const media = this.#media.get(request.url);
    const stop = (status, extra = {}) => { response.writeHead(status, { 'Content-Length': '0', ...extra }); response.end(); };
    if (this.#closed || !media) { stop(404); return; }
    if (request.method !== 'GET' && request.method !== 'HEAD') { stop(405, { Allow: 'GET, HEAD' }); return; }
    const range = parseByteRange(request.headers.range, media.length);
    if (!range) { stop(416, { 'Content-Range': `bytes */${media.length}` }); return; }
    const headers = {
      'Accept-Ranges': 'bytes', 'Content-Type': media.contentType,
      'Content-Length': String(range.end - range.start + 1), 'Cache-Control': 'no-store',
    };
    if (range.partial) headers['Content-Range'] = `bytes ${range.start}-${range.end}/${media.length}`;
    if (request.method === 'HEAD') {
      // Preserve Content-Length/range semantics. A bounded, local HEAD reads
      // the first fixed failure after a previously started stream is cut.
      // It never performs an upstream request or returns upstream details.
      if (STREAM_ERRORS.has(media.failure)) headers[STREAM_ERROR_HEADER] = media.failure;
      response.writeHead(range.partial ? 206 : 200, headers); response.end(); return;
    }
    if (this.#handlers.size >= 4) { stop(503); return; }
    const controller = new AbortController();
    const signal = AbortSignal.any([controller.signal, media.controller.signal]);
    let finished;
    const task = new Promise(resolve => { finished = resolve; });
    this.#handlers.add(task);
    const disconnected = () => { if (!response.writableEnded) controller.abort(); };
    response.once('close', disconnected);
    request.once('aborted', disconnected);
    let reader, timedOut = false;
    const recordFailure = category => {
      if (!media.failure && STREAM_ERRORS.has(category)) media.failure = category;
    };
    const timed = async operation => {
      const timer = setTimeout(() => { timedOut = true; controller.abort(); }, this.#idleTimeoutMS);
      try { return await operation(); } finally { clearTimeout(timer); }
    };
    try {
      for (let offset = range.start; offset <= range.end;) {
        if (signal.aborted) throw error('stream_cancelled');
        const end = Math.min(range.end, offset + CHUNK_BYTES - 1);
        const remote = new URL(media.url);
        remote.searchParams.set('range', `${offset}-${end}`);
        const upstream = await timed(() => this.#fetch(remote, {
          headers: new Headers(STREAM_HEADERS), signal, credentials: 'omit',
        }));
        if (![200, 206].includes(upstream.status) || !upstream.body) {
          await upstream.body?.cancel();
          const category = [401, 403].includes(upstream.status) ? 'upstream_access_denied'
            : upstream.status === 429 ? 'upstream_rate_limited' : 'upstream_http_error';
          recordFailure(category);
          if (!response.headersSent) { stop([401, 403].includes(upstream.status) ? 403 : upstream.status === 429 ? 429 : 502); return; }
          throw error(category);
        }
        // A URL range can return 200. Require the exact bounded body length.
        const expected = end - offset + 1;
        const length = upstream.headers.get('content-length');
        const contentRange = upstream.headers.get('content-range');
        if (!/^\d+$/.test(length ?? '') || Number(length) !== expected ||
            (contentRange && contentRange !== `bytes ${offset}-${end}/${media.length}`) ||
            (upstream.status === 206 && !contentRange)) {
          await upstream.body.cancel();
          throw error('upstream_range_mismatch');
        }
        reader = upstream.body.getReader();
        if (!response.headersSent) response.writeHead(range.partial ? 206 : 200, headers);
        let received = 0;
        for (;;) {
          const { value, done } = await timed(() => reader.read());
          if (done) break;
          received += value.byteLength;
          if (received > expected) throw error('upstream_range_mismatch');
          for (let index = 0; index < value.byteLength; index += 32 * 1024) {
            if (signal.aborted) throw error('stream_cancelled');
            if (!response.write(value.subarray(index, Math.min(value.byteLength, index + 32 * 1024)))) await once(response, 'drain', { signal });
          }
        }
        reader.releaseLock();
        reader = undefined;
        if (received !== expected) throw error('upstream_truncated');
        offset = end + 1;
      }
      response.end();
    } catch (failure) {
      if (timedOut) recordFailure('upstream_timeout');
      else if (!signal.aborted) {
        const code = failure?.cause?.code ?? failure?.code;
        const timeout = ['ETIMEDOUT', 'UND_ERR_CONNECT_TIMEOUT', 'UND_ERR_HEADERS_TIMEOUT', 'UND_ERR_BODY_TIMEOUT'].includes(code);
        recordFailure(STREAM_ERRORS.has(failure?.safeCategory) ? failure.safeCategory
          : timeout ? 'upstream_timeout' : 'upstream_connection_failed');
      }
      if (response.headersSent) response.destroy();
      else stop(signal.aborted ? 410 : 502);
    } finally {
      controller.abort();
      await reader?.cancel().catch(() => {});
      response.off('close', disconnected);
      request.off('aborted', disconnected);
      this.#handlers.delete(task);
      finished();
    }
  }

  close() {
    if (this.#closing) return this.#closing;
    this.#closed = true;
    this.#closing = (async () => {
      for (const path of this.#media.keys()) this.#revoke(path);
      await this.#starting?.catch(() => {});
      const server = this.#server;
      if (server) await new Promise(resolve => { server.close(resolve); server.closeAllConnections(); });
      await Promise.allSettled([...this.#handlers]);
      this.#server = undefined;
    })();
    return this.#closing;
  }
}
