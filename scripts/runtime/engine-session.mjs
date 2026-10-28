import { spawn } from 'node:child_process';
import { dirname } from 'node:path';
import { fileURLToPath } from 'node:url';
import { CATEGORIES } from './control.mjs';
import { MemoryCache } from './sdk-session.mjs';
import { validateMediaURL } from './media-format.mjs';
import { validateAccountSession } from './account-session.mjs';
import { encodeFrame, FrameReader, REQUEST_LIMIT, REPLY_LIMIT, PROOF_PHASES, PROOF_REASONS, RUNTIME_STAGES } from './engine-ipc.mjs';

const NEAR_EXPIRY_MS = 60_000;
const failure = category => {
  const safeCategory = CATEGORIES.has(category) ? category : 'runtime_failed';
  return Object.assign(new Error(safeCategory), { safeCategory });
};
const record = value => value !== null && typeof value === 'object' && !Array.isArray(value);
const zero = () => ({ init_ms: 0, proof_init_ms: 0, resolve_ms: 0, decipher_ms: 0, proof_ms: 0, fetch_count: 0, player_requests: 0 });
const numeric = new Set(['init_ms', 'proof_init_ms', 'resolve_ms', 'decipher_ms', 'proof_ms', 'fetch_count', 'player_requests', 'bitrate', 'format_count']);

function validateSafe(safe, kind) {
  if (!record(safe)) throw failure();
  for (const [key, value] of Object.entries(safe)) {
    if (numeric.has(key)) {
      if (!Number.isSafeInteger(value) || value < 0 || value > 1_000_000_000) throw failure();
    } else if (key === 'kind') { if (value !== kind) throw failure(); }
    else if (key === 'codec') { if (!(kind === 'video' ? ['avc1'] : ['opus', 'mp4a.40.2']).includes(value)) throw failure(); }
    else throw failure();
  }
  for (const key of Object.keys(zero())) if (!Object.hasOwn(safe, key)) throw failure();
  if (kind && (!Object.hasOwn(safe, 'kind') || !Object.hasOwn(safe, 'codec') || !Object.hasOwn(safe, 'bitrate') || !Object.hasOwn(safe, 'format_count'))) throw failure();
}

function validateMedia(media, kind) {
  if (!record(media) || Object.keys(media).some(key => !['url', 'duration', 'length', 'contentType', ...(kind === 'video' ? ['width', 'height'] : [])].includes(key))) throw failure();
  validateMediaURL(media.url);
  if (!Number.isFinite(media.duration) || media.duration <= 0 || media.duration > 86400 ||
      !Number.isSafeInteger(media.length) || media.length <= 0 || media.length > 64 * 1024 ** 3 ||
      !(kind === 'video' ? ['video/mp4'] : ['audio/webm', 'audio/mp4']).includes(media.contentType)) throw failure();
  if (kind === 'video' && (!Number.isSafeInteger(media.width) || !Number.isSafeInteger(media.height) || media.width <= 0 || media.width > 1920 || media.height <= 0 || media.height > 480)) throw failure();
}

// The unrestricted bridge never imports the SDK/provider. The shared engine
// has the former proof child's read-only runtime and network permissions.
export class EngineSession {
  #worker; #node; #timeout; #child; #busy = false; #closed = false; #closing; #sequence = 0; #session;
  constructor({ workerURL = new URL('./engine-worker.mjs', import.meta.url), nodePath = process.execPath, timeoutMS = 30_000, session } = {}) {
    try {
      const url = workerURL instanceof URL ? workerURL : new URL(workerURL);
      if (url.protocol !== 'file:' || url.search || url.hash || typeof nodePath !== 'string' || !nodePath || !Number.isSafeInteger(timeoutMS) || timeoutMS <= 0 || timeoutMS > 30_000) throw failure();
      this.#worker = fileURLToPath(url); this.#node = nodePath; this.#timeout = timeoutMS;
      this.#session = validateAccountSession(session);
    } catch { throw failure('init_failed'); }
  }
  #enter(signal) {
    if (this.#closed) throw failure('runtime_closed');
    if (signal?.aborted) throw failure('request_cancelled');
    if (this.#busy || (signal != null && (typeof signal.addEventListener !== 'function' || typeof signal.removeEventListener !== 'function'))) throw failure('runtime_failed');
    this.#busy = true;
  }
  async warm(seed, signal) {
    this.#enter(signal);
    try {
      if (!record(seed) || Object.keys(seed).some(key => !['entries', 'playerID', 'fetchCount'].includes(key)) ||
          typeof seed.playerID !== 'string' || !/^[A-Za-z0-9_-]{1,128}$/.test(seed.playerID) || !Array.isArray(seed.entries) || !seed.entries.length) throw failure('init_failed');
      let child = this.#child;
      if (child?.ready && !child.retiring && child.seed === seed && child.expiresAt - Date.now() > NEAR_EXPIRY_MS) return { expiresAt: child.expiresAt, safe: zero() };
      // Validate before retiring a healthy child. The cache contains no SDK
      // code execution here, only bounded opaque byte copies.
      new MemoryCache(seed.entries, { strict: true });
      if (child) await this.#retire(child, 'proof_expired');
      if (this.#closed) throw failure('runtime_closed');
      if (signal?.aborted) throw failure('request_cancelled');
      child = this.#launch(); child.seed = seed;
      const reply = await this.#request(child, { op: 'warm', entries: seed.entries, playerID: seed.playerID, session: this.#session }, signal);
      if (this.#closed || signal?.aborted || child.retiring || this.#child !== child) {
        await this.#retire(child, signal?.aborted ? 'request_cancelled' : 'runtime_closed');
        throw failure(signal?.aborted ? 'request_cancelled' : 'runtime_closed');
      }
      child.ready = true; child.expiresAt = reply.expiresAt;
      return { expiresAt: reply.expiresAt, safe: reply.safe };
    } finally { this.#busy = false; }
  }
  async resolve(videoId, kind, signal) {
    this.#enter(signal);
    try {
      if (!/^[A-Za-z0-9_-]{11}$/.test(videoId ?? '') || !['audio', 'video'].includes(kind)) throw failure('invalid_request');
      const child = this.#child;
      if (!child?.ready || child.retiring) throw failure('init_failed');
      if (child.expiresAt - Date.now() <= NEAR_EXPIRY_MS) { await this.#retire(child, 'proof_expired'); throw failure('proof_expired'); }
      const reply = await this.#request(child, { op: 'resolve', videoId, kind }, signal);
      if (this.#closed || signal?.aborted || child.retiring || this.#child !== child) {
        await this.#retire(child, signal?.aborted ? 'request_cancelled' : 'runtime_closed');
        throw failure(signal?.aborted ? 'request_cancelled' : 'runtime_closed');
      }
      child.expiresAt = Math.min(child.expiresAt, reply.expiresAt);
      return { media: reply.media, safe: reply.safe, expiresAt: child.expiresAt };
    } finally { this.#busy = false; }
  }
  #launch() {
    let processChild;
    // The DOM/SDK module graph benefits from V8's memory-oriented heap policy.
    try { processChild = spawn(this.#node, ['--optimize-for-size', '--permission', `--allow-fs-read=${dirname(this.#worker)}`, '--allow-net', this.#worker], {
      stdio: ['pipe', 'pipe', 'ignore'], env: { PATH: '/usr/bin:/bin', LANG: 'en_US.UTF-8', LC_ALL: 'en_US.UTF-8' }, windowsHide: true,
    }); } catch { throw failure('init_failed'); }
    const child = { process: processChild, ready: false, retiring: false, reaped: false, seed: null, pending: null, expiresAt: 0, reader: new FrameReader(REPLY_LIMIT), category: 'runtime_failed' };
    child.closed = new Promise(resolve => processChild.once('close', () => {
      child.reaped = true; child.ready = false; child.seed = null; child.reader.clear();
      if (this.#child === child) this.#child = undefined;
      const pending = child.pending; child.pending = undefined;
      if (pending) { pending.cleanup(); pending.reject(Object.assign(failure(child.category), child.failureDetail)); }
      resolve();
    }));
    this.#child = child;
    for (const emitter of [processChild, processChild.stdin, processChild.stdout]) emitter.on('error', () => { void this.#retire(child, 'runtime_failed'); });
    processChild.stdout.on('data', chunk => this.#receive(child, chunk));
    return child;
  }
  #request(child, request, signal) {
    const id = ++this.#sequence;
    let frame;
    try { frame = encodeFrame({ id, ...request }, REQUEST_LIMIT); }
    catch { return this.#retire(child, 'cache_limit').then(() => { throw failure('cache_limit'); }); }
    return new Promise((resolve, reject) => {
      const abort = () => { void this.#retire(child, 'request_cancelled'); };
      const timer = setTimeout(() => { void this.#retire(child, 'request_timeout'); }, this.#timeout);
      child.pending = { id, op: request.op, kind: request.kind, resolve, reject, cleanup() { clearTimeout(timer); signal?.removeEventListener('abort', abort); } };
      signal?.addEventListener('abort', abort, { once: true });
      if (signal?.aborted) { abort(); return; }
      if (this.#closed || child.retiring) { void this.#retire(child, 'runtime_closed'); return; }
      child.process.stdin.write(frame, error => { if (error) void this.#retire(child, 'runtime_failed'); });
    });
  }
  #receive(child, chunk) {
    if (child.retiring || child.reaped) return;
    try {
      const replies = [];
      child.reader.push(chunk, reply => { replies.push(reply); if (replies.length > 1) throw failure(); });
      if (!replies.length) return;
      if (child.reader.partial) throw failure();
      const reply = replies[0], pending = child.pending;
      if (!pending || !record(reply) || reply.id !== pending.id || typeof reply.ok !== 'boolean') throw failure();
      if (!reply.ok) {
        if (Object.keys(reply).some(key => !['id', 'ok', 'category', 'phase', 'reason', 'stage'].includes(key)) || !CATEGORIES.has(reply.category) ||
            (reply.phase != null && !PROOF_PHASES.has(reply.phase)) || (reply.reason != null && !PROOF_REASONS.has(reply.reason)) ||
            (reply.stage != null && !RUNTIME_STAGES.has(reply.stage))) throw failure();
        child.failureDetail = { safePhase: reply.phase, safeReason: reply.reason, safeStage: reply.stage };
        void this.#retire(child, reply.category); return;
      }
      if (Object.keys(reply).some(key => !['id', 'ok', 'expiresAt', 'safe', ...(pending.op === 'resolve' ? ['media'] : [])].includes(key))) throw failure();
      const remaining = reply.expiresAt - Date.now();
      if (!Number.isSafeInteger(reply.expiresAt) || remaining > 24 * 60 * 60_000) throw failure();
      if (remaining <= NEAR_EXPIRY_MS) { void this.#retire(child, 'proof_expired'); return; }
      validateSafe(reply.safe, pending.kind);
      if (pending.op === 'resolve') validateMedia(reply.media, pending.kind);
      child.pending = undefined; pending.cleanup(); pending.resolve(reply);
    } catch (error) { void this.#retire(child, error?.safeCategory); }
  }
  #retire(child, category) {
    if (!child.retiring && !child.reaped) {
      child.retiring = true; child.ready = false; child.seed = null; child.category = category;
      child.reader.clear(); child.process.kill('SIGKILL'); child.process.stdin.destroy(); child.process.stdout.destroy();
    }
    return child.closed;
  }
  close() {
    if (this.#closing) return this.#closing;
    this.#closed = true; this.#session = undefined;
    this.#closing = this.#child ? this.#retire(this.#child, 'runtime_closed') : Promise.resolve();
    return this.#closing;
  }
}
