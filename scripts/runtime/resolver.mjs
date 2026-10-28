import { performance } from 'node:perf_hooks';
import { StreamBridge } from './stream-bridge.mjs';
import { BootstrapSession } from './bootstrap-session.mjs';
import { EngineSession } from './engine-session.mjs';
import { validateAccountSession } from './account-session.mjs';
export { chooseFormat, validateMediaURL } from './media-format.mjs';

const error = category => Object.assign(new Error(category), { safeCategory: category });
const elapsed = start => Math.round(performance.now() - start);
const check = signal => { if (signal?.aborted) throw error('request_cancelled'); };

// The bridge process never imports YouTube.js or the provider. Only bounded
// opaque bootstrap bytes and resolved media capabilities cross its private pipe.
export class Resolver {
  #bridge; #bootstrap; #engine; #seed; #active = false; #closed = false; #sequence = 0;
  #session; #revision = 0; #engineFactory; #bootstrapFactory;
  constructor({ bridge = new StreamBridge(), bootstrap = new BootstrapSession(), engine,
    engineFactory = session => new EngineSession({ session }), bootstrapFactory = () => new BootstrapSession() } = {}) {
    this.#bridge = bridge; this.#bootstrap = bootstrap; this.#engine = engine;
    this.#engineFactory = engineFactory; this.#bootstrapFactory = bootstrapFactory;
  }
  async configure(value, signal) {
    const session = validateAccountSession(value);
    if (this.#closed) throw error('runtime_closed');
    if (this.#active) throw error('runtime_failed');
    check(signal);
    if (session.revision < this.#revision) throw error('invalid_request');
    if (session.revision === this.#revision && this.#session &&
        ['loggedIn', 'accountIndex', 'onBehalfOfUser'].some(key => session[key] !== this.#session[key])) throw error('unexpected_authentication');
    const previousRevision = this.#revision;
    this.#revision = session.revision;
    this.#session = undefined; this.#seed = undefined;
    if (previousRevision && previousRevision !== session.revision) this.#bridge.revokeAll();
    if (previousRevision) {
      // close resolves only after the old subprocess has been reaped.
      await Promise.all([this.#engine?.close(), this.#bootstrap.close()]);
      this.#engine = undefined; this.#bootstrap = this.#bootstrapFactory();
    }
    check(signal);
    if (this.#closed) throw error('runtime_closed');
    this.#engine ??= this.#engineFactory(session);
    this.#session = session;
    return {};
  }
  release(url) { return this.#bridge.release(url); }
  async close() {
    this.#closed = true; this.#seed = this.#session = undefined;
    await Promise.all([this.#bridge.close(), this.#bootstrap.close(), this.#engine?.close()]);
  }
  async #operation(signal, run) {
    if (this.#closed) throw error('runtime_closed');
    if (this.#active) throw error('runtime_failed');
    if (!this.#session) throw error('init_failed');
    check(signal); this.#active = true;
    const start = performance.now();
    const safe = { client: 'WEB_REMIX', authenticated: this.#session.loggedIn, version: '18.3.0', sequence: ++this.#sequence,
      init_ms: 0, bootstrap_ms: 0, proof_init_ms: 0, resolve_ms: 0, decipher_ms: 0, proof_ms: 0, fetch_count: 0, player_requests: 0 };
    let result;
    try {
      if (!this.#seed) {
        const began = performance.now();
        let seed;
        try { seed = await this.#bootstrap.warm(signal); }
        catch (failure) { const result = failure?.safeCategory ? failure : error('init_failed'); result.safeStage = 'bootstrap'; throw result; }
        check(signal);
        if (this.#closed) throw error('runtime_closed');
        this.#seed = seed; safe.bootstrap_ms = elapsed(began); safe.fetch_count += seed.fetchCount;
      }
      const warm = await this.#engine.warm(this.#seed, signal); check(signal);
      safe.proof_init_ms = warm.safe.proof_init_ms;
      safe.fetch_count += warm.safe.fetch_count; safe.player_requests += warm.safe.player_requests;
      safe.init_ms = elapsed(start);
      result = await run(safe); check(signal);
      if (this.#closed) throw error('runtime_closed');
      return { ...result, safe: { ...safe, total_ms: elapsed(start) } };
    } catch (failure) {
      if (result?.media?.url) { try { this.#bridge.release(result.media.url); } catch {} }
      if (failure?.safeCategory) throw failure;
      throw error('runtime_failed');
    } finally { this.#active = false; }
  }
  warm(signal) { return this.#operation(signal, async () => ({})); }
  resolve(videoId, kind, signal) {
    return this.#operation(signal, async safe => {
      if (this.#bridge.size >= 4) throw error('capacity_limit');
      const result = await this.#engine.resolve(videoId, kind, signal); check(signal);
      const url = await this.#bridge.publish(result.media);
      if (signal?.aborted || this.#closed) { this.#bridge.release(url); throw error(signal?.aborted ? 'request_cancelled' : 'runtime_closed'); }
      const { init_ms: _init, proof_init_ms: _proof, fetch_count, player_requests, ...metrics } = result.safe;
      Object.assign(safe, metrics); safe.fetch_count += fetch_count; safe.player_requests += player_requests;
      const media = { url, duration: result.media.duration };
      if (kind === 'video') { media.width = result.media.width; media.height = result.media.height; }
      return { media };
    });
  }
}
