import https from 'node:https';
import { proofContext, proofRequest } from './proof-policy.mjs';
import { PROOF_REASONS } from './engine-ipc.mjs';

const failed = category => Object.assign(new Error(category), { safeCategory: category });

// Used only inside the restricted engine child. The provider stays unmodified;
// one process owns its module graph, DOM, generator, anonymous context and SDK.
export class EngineProof {
  #manager; #context; #binding; #active; #failed = false; #closed = false;
  #agent = new https.Agent({ keepAlive: true, maxSockets: 2, maxFreeSockets: 1 });
  #axios; #requestInterceptor; #responseInterceptor; #onFailure; #loadAxios; #loadProvider;
  #phase = 'init'; #failureDetail;
  constructor({ onFailure = () => {}, loadAxios = () => import('axios'), loadProvider = () => import('./vendor/bgutil/build/session_manager.js') } = {}) {
    this.#onFailure = onFailure; this.#loadAxios = loadAxios; this.#loadProvider = loadProvider;
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
    this.#axios = axios;
    this.#requestInterceptor = axios.interceptors.request.use(config => {
      this.#phase = 'network';
      if (!this.#active || this.#failed || this.#closed) throw this.#terminal('proof_request_rejected');
      try { return proofRequest(config, { signal: this.#active.signal, httpsAgent: this.#agent }); }
      catch (failure) { throw this.#terminal('proof_request_rejected', failure.safeReason); }
    });
    this.#responseInterceptor = axios.interceptors.response.use(response => {
      if (this.#failed || this.#closed || this.#active?.signal.aborted) throw this.#terminal('proof_failed');
      this.#phase = 'mint'; return response;
    }, failure => { throw this.#terminal('proof_failed', failure?.code === 'ERR_ACCESS_DENIED' ? 'permission' : failure?.response ? 'http' : 'other'); });
    const { SessionManager } = await this.#loadProvider();
    this.#manager = new SessionManager(false, undefined);
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
      const result = await this.#manager.generatePoToken(binding, '', false, undefined, false, undefined, this.#context);
      if (signal?.aborted || this.#failed || this.#closed) throw this.#terminal('proof_failed');
      if (result?.contentBinding !== binding || typeof result.poToken !== 'string' || !result.poToken || result.poToken.length > 24 * 1024) throw this.#terminal('proof_failed');
      this.#phase = 'deadline';
      const expiry = result.expiresAt instanceof Date ? result.expiresAt.getTime() : NaN;
      if (!Number.isSafeInteger(expiry) || expiry <= Date.now() + 60_000 || expiry > Date.now() + 24 * 60 * 60_000) throw this.#terminal('proof_expired');
      return { token: result.poToken, expiresAt: expiry };
    } catch (error) {
      throw this.#terminal(['proof_failed', 'proof_expired', 'proof_request_rejected'].includes(error?.safeCategory) ? error.safeCategory : 'proof_failed');
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
    try { this.#manager?.invalidateCaches(); } catch {}
    if (this.#axios) {
      this.#axios.interceptors.request.eject(this.#requestInterceptor);
      this.#axios.interceptors.response.eject(this.#responseInterceptor);
    }
    this.#context = this.#manager = this.#binding = undefined; this.#agent.destroy();
  }
}
