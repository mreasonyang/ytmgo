import https from 'node:https';
import { proofContext, proofRequest } from './proof-policy.mjs';
import { PROOF_REASONS } from './engine-ipc.mjs';

const failed = category => Object.assign(new Error(category), { safeCategory: category });

// Used only inside the restricted engine child. All proof requests pass through
// this policy boundary; the independent session uses the MIT BgUtils API.
export class EngineProof {
  #manager; #context; #binding; #active; #failed = false; #closed = false;
  #agent = new https.Agent({ keepAlive: true, maxSockets: 2, maxFreeSockets: 1 });
  #axios; #onFailure; #loadAxios; #loadSession;
  #phase = 'init'; #failureDetail;
  constructor({ onFailure = () => {}, loadAxios = () => import('axios'), loadSession = () => import('./proof-session.mjs') } = {}) {
    this.#onFailure = onFailure; this.#loadAxios = loadAxios; this.#loadSession = loadSession;
  }

  #terminal(category, reason = 'other') {
    const first = !this.#failed;
    this.#failed = true;
    this.#active?.abort();
    if (first) {
      this.#failureDetail = { safePhase: this.#phase, safeReason: PROOF_REASONS.has(reason) ? reason : 'other' };
      this.#onFailure(category, this.#failureDetail);
    }
    return Object.assign(failed(category), this.#failureDetail);
  }

  async #initialize() {
    this.#phase = 'init';
    const { default: axios } = await this.#loadAxios();
    this.#axios = axios.create();
    const { ProofSession } = await this.#loadSession();
    if (this.#failed || this.#closed || this.#active?.signal.aborted) throw this.#terminal('proof_failed');
    this.#manager = new ProofSession({ request: config => this.#request(config) });
    await this.#manager.initialize();
  }

  async #request(config) {
    this.#phase = 'network';
    if (!this.#active || this.#failed || this.#closed || this.#active.signal.aborted) throw this.#terminal('proof_request_rejected');
    let request;
    try { request = proofRequest(config, { signal: this.#active.signal, httpsAgent: this.#agent }); }
    catch (failure) { throw this.#terminal('proof_request_rejected', failure.safeReason); }
    try {
      const response = await this.#axios.request({ ...request, responseType: 'text', transformResponse: [data => data] });
      if (this.#failed || this.#closed || this.#active?.signal.aborted) throw this.#terminal('proof_failed');
      this.#phase = 'mint'; return response.data;
    } catch (failure) {
      throw this.#terminal('proof_failed', failure?.code === 'ERR_ACCESS_DENIED' ? 'permission' : failure?.response ? 'http' : 'other');
    }
  }

  async #generate(binding, signal) {
    if (this.#closed || this.#failed || this.#active || signal?.aborted) throw failed('proof_request_rejected');
    const active = new AbortController(); this.#active = active;
    const abort = () => active.abort();
    signal?.addEventListener('abort', abort, { once: true });
    try {
      if (!this.#manager) await this.#initialize();
      if (signal?.aborted || this.#failed || this.#closed) throw this.#terminal('proof_failed');
      this.#phase = 'mint';
      const result = await this.#manager.mint(binding);
      if (signal?.aborted || this.#failed || this.#closed) throw this.#terminal('proof_failed');
      if (typeof result?.token !== 'string' || !result.token || result.token.length > 24 * 1024) throw this.#terminal('proof_failed');
      this.#phase = 'deadline';
      const expiry = result.expiresAt;
      if (!Number.isSafeInteger(expiry) || expiry <= Date.now() + 60_000 || expiry > Date.now() + 24 * 60 * 60_000) throw this.#terminal('proof_expired');
      return { token: result.token, expiresAt: expiry };
    } catch (error) {
      throw this.#terminal(['proof_failed', 'proof_expired', 'proof_request_rejected'].includes(error?.safeCategory) ? error.safeCategory : 'proof_failed', error?.safeReason);
    } finally { signal?.removeEventListener('abort', abort); this.#active = undefined; }
  }

  async warm(context, signal, binding) {
    if (this.#context) throw failed('proof_request_rejected');
    this.#phase = 'context';
    try { this.#context = proofContext(context); }
    catch (failure) { throw this.#terminal('proof_request_rejected', failure.safeReason); }
    if (!binding || typeof binding.sessionBinding !== 'string' || !binding.sessionBinding || binding.sessionBinding.length > 4096 ||
        typeof binding.videoBound !== 'boolean') throw this.#terminal('proof_request_rejected', 'authentication');
    this.#binding = { sessionBinding: binding.sessionBinding, videoBound: binding.videoBound };
    const result = await this.#generate(this.#binding.sessionBinding, signal);
    return { expiresAt: result.expiresAt };
  }
  mint(videoId, signal) {
    if (!this.#context || !/^[A-Za-z0-9_-]{11}$/.test(videoId ?? '')) throw failed('proof_request_rejected');
    return this.#generate(this.#binding.videoBound ? videoId : this.#binding.sessionBinding, signal);
  }
  close() {
    if (this.#closed) return;
    this.#closed = true; this.#active?.abort();
    try { this.#manager?.close(); } catch {}
    this.#axios = undefined;
    this.#context = this.#manager = this.#binding = undefined; this.#agent.destroy();
  }
}
