import { spawn } from 'node:child_process';
import { dirname } from 'node:path';
import { fileURLToPath } from 'node:url';
import { DefaultDeserializer } from 'node:v8';
import { MemoryCache } from './sdk-session.mjs';

export const BOOTSTRAP_IPC_LIMIT = 16 * 1024 * 1024;
const CATEGORIES = new Set(['init_failed', 'cache_limit', 'request_cancelled', 'request_timeout',
  'unexpected_authentication', 'unexpected_request_host', 'resolver_response_limit',
  'resolver_http_error', 'resolver_access_denied', 'resolver_rate_limited',
  'player_signature_missing', 'decipher_script_invalid', 'decipher_failed']);
const failure = category => {
  const safeCategory = CATEGORIES.has(category) || category === 'runtime_closed' ? category : 'init_failed';
  return Object.assign(new Error(safeCategory), { safeCategory });
};
const record = value => value !== null && typeof value === 'object' && !Array.isArray(value);

function decode(chunks, bytes) {
  try {
    const frame = Buffer.concat(chunks, bytes);
    if (frame.length < 5 || frame.readUInt32BE(0) !== frame.length - 4) throw failure('init_failed');
    const reader = new DefaultDeserializer(frame.subarray(4));
    if (!reader.readHeader()) throw failure('init_failed');
    const reply = reader.readValue();
    let trailing = false;
    try { reader.readRawBytes(1); trailing = true; } catch {}
    if (trailing || !record(reply) || typeof reply.ok !== 'boolean') throw failure('init_failed');
    if (!reply.ok) {
      if (Object.keys(reply).some(key => !['ok', 'category'].includes(key)) || !CATEGORIES.has(reply.category)) throw failure('init_failed');
      throw failure(reply.category);
    }
    if (Object.keys(reply).some(key => !['ok', 'entries', 'playerID', 'fetchCount'].includes(key)) ||
        typeof reply.playerID !== 'string' || !/^[A-Za-z0-9_-]{1,128}$/.test(reply.playerID) ||
        !Number.isSafeInteger(reply.fetchCount) || reply.fetchCount < 0 || reply.fetchCount > 100 ||
        !Array.isArray(reply.entries) || !reply.entries.length) throw failure('init_failed');
    const cache = new MemoryCache(reply.entries);
    return { entries: cache.snapshot(), playerID: reply.playerID, fetchCount: reply.fetchCount };
  } catch (error) { throw failure(error?.safeCategory); }
}

// Each explicit warm owns one short-lived child and returns only after reaping.
// There is no automatic retry or supplier state in the parent process.
export class BootstrapSession {
  #worker;
  #node;
  #timeout;
  #active;
  #closed = false;
  #closing;

  constructor({ workerURL = new URL('./bootstrap-worker.mjs', import.meta.url), nodePath = process.execPath, timeoutMS = 30_000 } = {}) {
    try {
      const url = workerURL instanceof URL ? workerURL : new URL(workerURL);
      if (url.protocol !== 'file:' || url.search || url.hash || typeof nodePath !== 'string' || !nodePath ||
          !Number.isSafeInteger(timeoutMS) || timeoutMS <= 0 || timeoutMS > 30_000) throw failure('init_failed');
      this.#worker = fileURLToPath(url);
      this.#node = nodePath;
      this.#timeout = timeoutMS;
    } catch { throw failure('init_failed'); }
  }

  warm(signal) {
    if (this.#closed) return Promise.reject(failure('runtime_closed'));
    if (this.#active) return Promise.reject(failure('init_failed'));
    if (signal?.aborted) return Promise.reject(failure('request_cancelled'));
    if (signal != null && (typeof signal.addEventListener !== 'function' || typeof signal.removeEventListener !== 'function')) return Promise.reject(failure('init_failed'));
    let child;
    try {
      child = spawn(this.#node, [
        '--optimize-for-size', '--permission', `--allow-fs-read=${dirname(this.#worker)}`, '--allow-net', this.#worker,
      ], {
        stdio: ['pipe', 'pipe', 'ignore'], windowsHide: true,
        env: { PATH: '/usr/bin:/bin', LANG: 'en_US.UTF-8', LC_ALL: 'en_US.UTF-8' },
      });
    } catch { return Promise.reject(failure('init_failed')); }
    const active = { child, chunks: [], bytes: 0, expected: null, prefix: Buffer.alloc(4), prefixBytes: 0, category: null };
    this.#active = active;
    active.reaped = new Promise(resolve => { active.finishReaping = resolve; });
    const stop = category => {
      if (active.category) return;
      active.category = category;
      active.chunks = [];
      child.kill('SIGKILL');
      child.stdin.destroy();
      child.stdout.destroy();
    };
    active.stop = stop;
    const abort = () => stop('request_cancelled');
    const timer = setTimeout(() => stop('request_timeout'), this.#timeout);
    const result = new Promise((resolve, reject) => {
      child.once('close', code => {
        clearTimeout(timer);
        signal?.removeEventListener('abort', abort);
        if (this.#active === active) this.#active = undefined;
        active.finishReaping();
        try {
          if (active.category) throw failure(active.category);
          if (code !== 0 || active.expected === null || active.expected !== active.bytes) throw failure('init_failed');
          resolve(decode(active.chunks, active.bytes));
        } catch (error) { reject(failure(error?.safeCategory)); }
        finally { active.chunks = []; }
      });
      child.on('error', () => stop('init_failed'));
      child.stdin.on('error', () => stop('init_failed'));
      child.stdout.on('error', () => stop('init_failed'));
      child.stdout.on('data', chunk => {
        if (active.category) return;
        active.bytes += chunk.length;
        if (active.bytes > BOOTSTRAP_IPC_LIMIT) { stop('cache_limit'); return; }
        if (active.prefixBytes < 4) {
          const count = Math.min(4 - active.prefixBytes, chunk.length);
          chunk.copy(active.prefix, active.prefixBytes, 0, count);
          active.prefixBytes += count;
          if (active.prefixBytes === 4) {
            active.expected = active.prefix.readUInt32BE(0) + 4;
            if (active.expected > BOOTSTRAP_IPC_LIMIT) { stop('cache_limit'); return; }
            if (active.expected < 5) { stop('init_failed'); return; }
          }
        }
        if (active.expected !== null && active.bytes > active.expected) { stop('init_failed'); return; }
        active.chunks.push(chunk);
      });
    });
    signal?.addEventListener('abort', abort, { once: true });
    if (signal?.aborted) abort();
    return result;
  }

  close() {
    if (this.#closing) return this.#closing;
    this.#closed = true;
    const active = this.#active;
    active?.stop('runtime_closed');
    this.#closing = active?.reaped ?? Promise.resolve();
    return this.#closing;
  }
}
