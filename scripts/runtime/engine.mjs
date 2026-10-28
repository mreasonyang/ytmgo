import { randomBytes } from 'node:crypto';
import { performance } from 'node:perf_hooks';
import { MemoryCache, createAccountSession, USER_AGENT } from './sdk-session.mjs';
import { validateAccountSession, validateSessionRequest, hasAcceptedAccount } from './account-session.mjs';
import { chooseFormat, validateMediaURL } from './media-format.mjs';

const MAX_RESPONSE = 16 * 1024 * 1024;
const error = category => Object.assign(new Error(category), { safeCategory: category });
const elapsed = start => Math.round(performance.now() - start);
const check = signal => { if (signal?.aborted) throw error('request_cancelled'); };
const createFormat = async raw => {
  const Format = (await import('./node_modules/youtubei.js/dist/src/parser/classes/misc/Format.js')).default;
  return new Format(raw, new Map());
};

// Only the permission-restricted child imports this module. Its SDK and the
// unmodified provider resolve youtubei.js to one ESM module graph.
export class PlaybackEngine {
  #proof; #proofFactory; #account; #createSession; #createFormat; #session; #cache; #active; #closed = false; #expiresAt = 0; #network; #stage;
  constructor({ proof, proofFactory = async () => new (await import('./engine-proof.mjs')).EngineProof(), createSession = createAccountSession, formatFactory = createFormat, network = globalThis.fetch } = {}) {
    this.#proof = proof; this.#createSession = createSession; this.#createFormat = formatFactory;
    this.#proofFactory = proofFactory;
    this.#network = network;
  }
  close() { this.#closed = true; this.#proof?.close(); this.#proof = this.#session = this.#cache = this.#account = undefined; }
  get stage() { return this.#stage; }

  async #fetch(input, init = {}) {
    const active = this.#active;
    if (!active) throw error('runtime_failed');
    check(active.signal);
    if (active.cachedInitialization) throw error('bootstrap_cache_miss');
    const original = new Request(input, init); await validateSessionRequest(original, this.#account);
    const request = new Request(original, { signal: AbortSignal.any([original.signal, active.signal]), credentials: 'omit', redirect: 'manual' });
    active.fetchCount++;
    if (new URL(request.url).pathname.endsWith('/player')) {
      active.playerRequests++;
      const body = await request.clone().json();
      if (body?.context?.client?.clientName !== 'WEB_REMIX') throw error('unexpected_client');
    }
    const response = await this.#network(request);
    if (response.status >= 300 && response.status < 400) { await response.body?.cancel(); throw error('unexpected_request_host'); }
    if (!response.ok) {
      await response.body?.cancel();
      if ([401, 403].includes(response.status)) throw error('resolver_access_denied');
      if (response.status === 429) throw error('resolver_rate_limited');
      throw error('resolver_http_error');
    }
    if (response.url && response.url !== request.url) { await response.body?.cancel(); throw error('unexpected_request_host'); }
    if (!response.body) return response;
    if (Number(response.headers.get('content-length')) > MAX_RESPONSE) {
      await response.body.cancel(); throw error('resolver_response_limit');
    }
    let size = 0;
    const bounded = response.body.pipeThrough(new TransformStream({ transform(chunk, controller) {
      size += chunk.byteLength;
      if (size > MAX_RESPONSE) throw error('resolver_response_limit');
      controller.enqueue(chunk);
    } }));
    const result = new Response(bounded, { status: response.status, statusText: response.statusText, headers: response.headers });
    Object.defineProperty(result, 'url', { value: response.url });
    return result;
  }

  async #operation(signal, category, run) {
    if (this.#closed) throw error('runtime_closed');
    if (this.#active) throw error('runtime_failed');
    check(signal);
    const active = { signal, fetchCount: 0, playerRequests: 0 };
    this.#active = active;
    const safe = { init_ms: 0, proof_init_ms: 0, resolve_ms: 0, decipher_ms: 0, proof_ms: 0 };
    try {
      const result = await run(active, safe); check(signal);
      return { ...result, expiresAt: this.#expiresAt, safe: { ...safe, fetch_count: active.fetchCount, player_requests: active.playerRequests } };
    } catch (failure) {
      const result = failure?.safeCategory ? failure : error(category);
      result.safeStage = this.#stage;
      throw result;
    }
    finally { this.#active = undefined; }
  }

  warm({ entries, playerID, session }, signal) {
    return this.#operation(signal, 'init_failed', async (active, safe) => {
      if (this.#session || typeof playerID !== 'string' || !/^[A-Za-z0-9_-]{1,128}$/.test(playerID)) throw error('init_failed');
      const began = performance.now();
      this.#stage = 'sdk_create';
      this.#account = validateAccountSession(session);
      this.#cache = new MemoryCache(entries, { strict: true });
      active.cachedInitialization = true;
      try { this.#session = await this.#createSession({ cache: this.#cache, fetch: this.#fetch.bind(this), playerID, account: this.#account }); }
      finally { active.cachedInitialization = false; }
      check(signal);
      if (!this.#account.loggedIn && (this.#session.session.logged_in || this.#session.session.cookie)) throw error('unexpected_authentication');
      if (!this.#session.session.player?.signature_timestamp) throw error('player_signature_missing');
      if (this.#account.loggedIn) {
        this.#stage = 'account_validation';
        const reply = await this.#session.actions.execute('/browse', { browseId: 'FEmusic_library_landing', client: 'YTMUSIC' });
        if (!hasAcceptedAccount(reply.data)) throw error('login_required');
      }
      check(signal);
      if (this.#account.musicPremium) {
        if (this.#proof) throw error('unexpected_authentication');
        this.#expiresAt = Date.now() + 6 * 60 * 60_000;
      } else {
        const proofBegan = performance.now();
        // A single official Music page supplies this account's binding and
        // experiment selection. Parse JSON only; never execute its HTML/JS.
        const headers = new Headers({ accept: 'text/html', 'user-agent': USER_AGENT, 'accept-language': 'en-US,en;q=0.7' });
        if (this.#account.loggedIn) {
          headers.set('cookie', this.#account.cookie);
          headers.set('x-goog-authuser', String(this.#account.accountIndex));
          if (this.#account.onBehalfOfUser) headers.set('x-goog-pageid', this.#account.onBehalfOfUser);
        }
        // The canonical page is selected through the account headers. Supplying
        // authuser in the URL can cause a canonical redirect; no redirect or
        // default-account retry is allowed. The returned identity is checked.
        this.#stage = 'proof_config_request';
        const config = await this.#fetch('https://music.youtube.com/', { headers });
        this.#stage = 'proof_config_parse';
        const { parseProofBinding } = await import('./proof-binding.mjs');
        const binding = parseProofBinding(await config.text(), this.#account);
        this.#session.session.context.client.visitorData = binding.visitorData;
        this.#stage = 'proof_init';
        this.#proof ??= await this.#proofFactory();
        check(signal);
        const proof = await this.#proof.warm(this.#session.session.context, signal, binding);
        this.#expiresAt = proof.expiresAt;
        safe.proof_init_ms = elapsed(proofBegan);
      }
      safe.init_ms = elapsed(began);
      return {};
    });
  }

  resolve(videoId, kind, signal) {
    return this.#operation(signal, 'resolve_failed', async (_active, safe) => {
      if (!this.#session) throw error('init_failed');
      if (this.#expiresAt <= Date.now() + 60_000) throw error('proof_expired');
      const yt = this.#session;
      this.#stage = 'player_request';
      const began = performance.now();
      const response = await yt.actions.execute('/player', {
        videoId, client: 'YTMUSIC', racyCheckOk: true, contentCheckOk: true,
        playbackContext: { contentPlaybackContext: {
          vis: 0, splay: false, lactMilliseconds: '-1', signatureTimestamp: yt.session.player.signature_timestamp,
        } },
      });
      safe.resolve_ms = elapsed(began);
      const data = response.data;
      if (this.#account.loggedIn && data?.responseContext?.mainAppWebResponseContext?.loggedOut === true) throw error('login_required');
      if (data?.playabilityStatus?.status !== 'OK') throw error(data?.playabilityStatus?.status === 'LOGIN_REQUIRED' ? 'login_required' : 'playability_unavailable');
      const selection = chooseFormat(data, kind);
      const duration = Number(selection.raw.approxDurationMs) / 1000 || Number(data.videoDetails?.lengthSeconds);
      if (!Number.isFinite(duration) || duration <= 0 || duration > 86400) throw error('invalid_duration');
      const decipherBegan = performance.now();
      this.#stage = 'decipher';
      let remote;
      try {
        const format = await this.#createFormat(selection.raw);
        remote = validateMediaURL(await format.decipher(yt.session.player));
      } catch (failure) { if (failure?.safeCategory) throw failure; throw error('decipher_failed'); }
      check(signal); safe.decipher_ms = elapsed(decipherBegan);
      if (remote.searchParams.has('pot')) throw error('unexpected_authentication');
      if (this.#proof) {
        this.#stage = 'proof_mint';
        const proofBegan = performance.now();
        const proof = await this.#proof.mint(videoId, signal);
        check(signal); this.#expiresAt = Math.min(this.#expiresAt, proof.expiresAt);
        remote.searchParams.set('pot', proof.token);
        safe.proof_ms = elapsed(proofBegan);
      }
      remote.searchParams.set('cpn', randomBytes(12).toString('base64url'));
      const codec = kind === 'video' ? 'avc1' : selection.raw.mimeType.includes('opus') ? 'opus' : 'mp4a.40.2';
      const media = { url: remote.toString(), duration, length: Number(selection.raw.contentLength), contentType: kind === 'video' ? 'video/mp4' : codec === 'opus' ? 'audio/webm' : 'audio/mp4' };
      if (kind === 'video') { media.width = selection.raw.width; media.height = selection.raw.height; }
      Object.assign(safe, { kind, codec, bitrate: Math.round(selection.raw.averageBitrate ?? selection.raw.bitrate ?? 0), format_count: selection.count });
      return { media };
    });
  }
}
