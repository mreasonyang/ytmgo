import vm from 'node:vm';

export const MAX_CACHE_BYTES = 8 * 1024 * 1024;
export const MAX_CACHE_ENTRIES = 4;
const MAX_KEY_BYTES = 512;
export const USER_AGENT = 'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/144.0.0.0 Safari/537.36';
const failure = category => Object.assign(new Error(category), { safeCategory: category });
const validKey = key => typeof key === 'string' && key.length > 0 && Buffer.byteLength(key) <= MAX_KEY_BYTES;

function copyValue(value) {
  if (!(value instanceof ArrayBuffer) && !ArrayBuffer.isView(value)) throw failure('cache_limit');
  if (value.byteLength > MAX_CACHE_BYTES) throw failure('cache_limit');
  const bytes = value instanceof ArrayBuffer
    ? new Uint8Array(value)
    : new Uint8Array(value.buffer, value.byteOffset, value.byteLength);
  return bytes.slice().buffer;
}

// The cache contains opaque SDK bytes. It never reads or writes a disk cache.
export class MemoryCache {
  #entries = new Map();
  #bytes = 0;
  #strict;

  constructor(entries = [], { strict = false } = {}) {
    this.#strict = strict === true;
    this.seed(entries);
  }

  get size() { return this.#entries.size; }
  get byteLength() { return this.#bytes; }

  async get(key) {
    if (!validKey(key)) throw failure('cache_limit');
    const value = this.#entries.get(key);
    if (value === undefined && this.#strict) throw failure('bootstrap_cache_miss');
    return value?.slice(0);
  }

  async set(key, value) {
    if (!validKey(key)) throw failure('cache_limit');
    const copy = copyValue(value);
    if (this.#entries.has(key)) this.#bytes -= this.#entries.get(key).byteLength;
    this.#entries.delete(key);
    while (this.#entries.size && (this.#entries.size >= MAX_CACHE_ENTRIES || this.#bytes + copy.byteLength > MAX_CACHE_BYTES)) {
      const first = this.#entries.keys().next().value;
      this.#bytes -= this.#entries.get(first).byteLength;
      this.#entries.delete(first);
    }
    this.#entries.set(key, copy);
    this.#bytes += copy.byteLength;
  }

  async remove(key) {
    if (this.#entries.has(key)) this.#bytes -= this.#entries.get(key).byteLength;
    this.#entries.delete(key);
  }

  snapshot() { return [...this.#entries].map(([key, value]) => [key, value.slice(0)]); }

  seed(entries) {
    if (!Array.isArray(entries) || entries.length > MAX_CACHE_ENTRIES) throw failure('cache_limit');
    const next = new Map();
    let bytes = 0;
    for (const entry of entries) {
      if (!Array.isArray(entry) || entry.length !== 2 || !validKey(entry[0]) ||
          !(entry[1] instanceof ArrayBuffer) || next.has(entry[0])) throw failure('cache_limit');
      bytes += entry[1].byteLength;
      if (bytes > MAX_CACHE_BYTES) throw failure('cache_limit');
      next.set(entry[0], copyValue(entry[1]));
    }
    this.#entries = next;
    this.#bytes = bytes;
  }
}

export function validateAnonymousRequest(request) {
  const target = new URL(request.url);
  if (target.protocol !== 'https:' || !['www.youtube.com', 'music.youtube.com', 'youtube.com', 'youtubei.googleapis.com', 'www.google.com'].includes(target.hostname) || target.username || target.password || target.port) throw failure('unexpected_request_host');
  // These two cookies are SDK-generated bootstrap data, not browser/account data.
  const names = request.headers.get('cookie')?.split(';').map(part => part.trim()).filter(Boolean).map(part => part.split('=', 1)[0]) ?? [];
  if (request.headers.has('authorization') || names.some(name => !['PREF', 'VISITOR_INFO1_LIVE'].includes(name))) throw failure('unexpected_authentication');
}

let sdkPromise;

async function createSession({ cache = new MemoryCache(), fetch, playerID, account } = {}) {
  if (typeof fetch !== 'function' || !cache || typeof cache.get !== 'function' || typeof cache.set !== 'function' || typeof cache.remove !== 'function' ||
      (playerID !== undefined && (typeof playerID !== 'string' || !/^[A-Za-z0-9_-]{1,128}$/.test(playerID)))) throw failure('init_failed');
  const sdk = await (sdkPromise ??= import('youtubei.js'));
  sdk.Log.setLevel(sdk.Log.Level.NONE);
  sdk.Platform.shim.eval = async data => {
    if (typeof data?.output !== 'string' || data.output.length > MAX_CACHE_BYTES) throw failure('decipher_script_invalid');
    const context = vm.createContext(Object.create(null), { codeGeneration: { strings: false, wasm: false } });
    try {
      const script = new vm.Script(`(function(){${data.output}\n})()`, { filename: 'youtube-player-transform' });
      return script.runInContext(context, { timeout: 3000, breakOnSigint: true });
    } catch { throw failure('decipher_failed'); }
  };
  const yt = await sdk.Innertube.create({
    client_type: 'WEB_REMIX', user_agent: USER_AGENT, lang: 'en', location: 'US',
    retrieve_player: true, retrieve_innertube_config: false, player_id: playerID,
    generate_session_locally: false, fail_fast: true,
    enable_session_cache: true, cache, fetch,
    ...(account?.loggedIn ? { cookie: account.cookie, account_index: account.accountIndex, on_behalf_of_user: account.onBehalfOfUser || undefined } : {}),
  });
  if (!account?.loggedIn && (yt.session.logged_in || yt.session.cookie)) throw failure('unexpected_authentication');
  if (!yt.session.player?.signature_timestamp) throw failure('player_signature_missing');
  return yt;
}

export const createAnonymousSession = options => createSession({ ...options, account: undefined });
export const createAccountSession = options => createSession(options);
