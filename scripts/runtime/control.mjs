import { validateAccountSession } from './account-session.mjs';
import { PROOF_PHASES, PROOF_REASONS, RUNTIME_STAGES } from './engine-ipc.mjs';
import { BOOTSTRAP_CATEGORIES, bootstrapFailureDetails } from './bootstrap-diagnostics.mjs';
// JSONL control-plane state. No credentials, remote URLs, or SDK logs belong
// in a response. Long operations are serial; release/cancel never queue behind them.
export const CATEGORIES = new Set([
  ...BOOTSTRAP_CATEGORIES,
  'invalid_request', 'duplicate_request', 'queue_full', 'request_cancelled',
  'request_timeout', 'runtime_closed', 'runtime_failed', 'init_failed',
  'resolve_failed', 'decipher_failed', 'decipher_script_invalid',
  'cache_limit', 'unexpected_request_host', 'unexpected_authentication',
  'unexpected_client', 'resolver_access_denied', 'resolver_rate_limited',
  'resolver_http_error', 'resolver_response_limit', 'player_signature_missing',
  'login_required', 'playability_unavailable', 'drm_unsupported',
  'sabr_unsupported', 'no_audio_format', 'no_video_format', 'invalid_duration',
  'invalid_dimensions', 'invalid_media_url', 'invalid_media_format',
  'invalid_content_length', 'bridge_closed', 'bridge_start_failed',
  'capacity_limit', 'media_expired', 'invalid_capability',
  'proof_failed', 'proof_expired', 'proof_request_rejected',
  'bootstrap_cache_miss',
]);
export const safeCategory = error => CATEGORIES.has(error?.safeCategory) ? error.safeCategory : 'runtime_failed';
export function safeFailureDetails(error) {
  const result = error?.safeStage === 'bootstrap' ? bootstrapFailureDetails(error) : {};
  if (RUNTIME_STAGES.has(error?.safeStage)) result.stage = error.safeStage;
  if (PROOF_PHASES.has(error?.safePhase)) result.phase = error.safePhase;
  if (PROOF_REASONS.has(error?.safeReason)) result.reason = error.safeReason;
  return result;
}
const validID = value => Number.isSafeInteger(value) && value > 0;

export function validateRequest(request) {
  if (!request || typeof request !== 'object' || Array.isArray(request) || !validID(request.id)) return false;
  const operations = {
    warm: ['id', 'op'], resolve: ['id', 'op', 'videoId', 'kind'],
    release: ['id', 'op', 'url'], cancel: ['id', 'op', 'target'],
    configure: ['id', 'op', 'session'],
  };
  const fields = typeof request.op === 'string' && Object.hasOwn(operations, request.op) ? operations[request.op] : null;
  if (!fields || Object.keys(request).some(key => !fields.includes(key))) return false;
  if (request.op === 'resolve') return /^[A-Za-z0-9_-]{11}$/.test(request.videoId ?? '') && ['audio', 'video'].includes(request.kind);
  if (request.op === 'release') return typeof request.url === 'string' && request.url.length <= 256;
  if (request.op === 'cancel') return validID(request.target) && request.target !== request.id;
  if (request.op === 'configure') { try { validateAccountSession(request.session); return true; } catch { return false; } }
  return true;
}

export class RuntimeController {
  #resolver;
  #emit;
  #queue = [];
  #jobs = new Map();
  #recent = new Set();
  #running;
  #closed = false;
  #timeout;
  #revision = 0;

  constructor(resolver, emit, { timeoutMS = 60_000 } = {}) {
    this.#resolver = resolver;
    this.#emit = emit;
    this.#timeout = timeoutMS;
  }

  accept(request) {
    const id = validID(request?.id) ? request.id : null;
    if (!validateRequest(request)) { this.#emit({ id, ok: false, category: 'invalid_request' }); return; }
    if (this.#closed) { this.#emit({ id, ok: false, category: 'runtime_closed' }); return; }
    if (this.#recent.has(id) || this.#jobs.has(id)) { this.#emit({ id, ok: false, category: 'duplicate_request' }); return; }
    this.#recent.add(id);
    if (this.#recent.size > 4096) this.#recent.delete(this.#recent.values().next().value);
    if (request.op === 'cancel') {
      this.#cancel(this.#jobs.get(request.target), 'request_cancelled');
      this.#emit({ id, ok: true });
      return;
    }
    if (request.op === 'release') {
      try { this.#resolver.release(request.url); this.#emit({ id, ok: true }); }
      catch (error) { this.#emit({ id, ok: false, category: safeCategory(error) }); }
      return;
    }
    if (request.op === 'configure') {
      if (request.session.revision < this.#revision) { this.#emit({ id, ok: false, category: 'invalid_request' }); return; }
      this.#revision = request.session.revision;
      // Cancelling the active request retires and waits for its engine child.
      // Drop every queued request before putting the new session at the front.
      for (const previous of [...this.#jobs.values()]) this.#cancel(previous, 'request_cancelled');
    }
    if (this.#jobs.size >= 16) { this.#emit({ id, ok: false, category: 'queue_full' }); return; }
    const job = { request, controller: new AbortController(), started: false };
    job.timer = setTimeout(() => this.#cancel(job, 'request_timeout'), this.#timeout);
    this.#jobs.set(id, job);
    this.#queue.push(job);
    this.#pump();
  }

  #cancel(job, reason) {
    if (!job || job.controller.signal.aborted) return;
    job.controller.abort(reason);
    if (!job.started) {
      this.#queue = this.#queue.filter(value => value !== job);
      this.#finish(job, { id: job.request.id, ok: false, category: reason });
    }
  }

  #finish(job, response) {
    clearTimeout(job.timer);
    this.#jobs.delete(job.request.id);
    if (!this.#closed) this.#emit(response);
  }

  #pump() {
    if (this.#running || this.#closed || !this.#queue.length) return;
    const job = this.#queue.shift();
    job.started = true;
    this.#running = (async () => {
      let result;
      try {
        const { request, controller } = job;
        result = request.op === 'configure'
          ? await this.#resolver.configure(request.session, controller.signal)
          : request.op === 'warm'
          ? await this.#resolver.warm(controller.signal)
          : await this.#resolver.resolve(request.videoId, request.kind, controller.signal);
        if (controller.signal.aborted) {
          if (result?.media?.url) this.#resolver.release(result.media.url);
          this.#finish(job, { id: request.id, ok: false, category: controller.signal.reason });
        } else this.#finish(job, { id: request.id, ok: true, ...result });
      } catch (error) {
        this.#finish(job, { id: job.request.id, ok: false,
          category: job.controller.signal.aborted ? job.controller.signal.reason : safeCategory(error),
          ...(!job.controller.signal.aborted ? safeFailureDetails(error) : {}) });
      }
    })().finally(() => { this.#running = undefined; this.#pump(); });
  }

  async close() {
    this.#closed = true;
    for (const job of [...this.#jobs.values()]) this.#cancel(job, 'request_cancelled');
    // Revoke streams immediately, even while the resolver unwinds.
    const closing = this.#resolver.close();
    await this.#running;
    await closing;
  }
}
