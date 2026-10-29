// SPDX-License-Identifier: Apache-2.0
// Uses the MIT BgUtils public API and index-innertube example. Attribution and
// the exact reference revision are recorded in licenses/README.md.
import vm from 'node:vm';
import { PROOF_RESPONSE_LIMIT } from './proof-policy.mjs';

const fail = (safeReason = 'session_bootstrap', safeCategory = 'proof_failed') =>
  Object.assign(new Error(safeCategory), { safeCategory, safeReason });
const record = value => value !== null && typeof value === 'object' && !Array.isArray(value);
const text = (value, limit) => typeof value === 'string' && value.length > 0 && value.length <= limit;
// Public WebPO request identifier from the MIT example, not an account secret.
const REQUEST_KEY = 'O43z0dpjhgX20SCx4KAo';
const loadLibraries = async () => {
  const [botguard, webpo, utils, dom] = await Promise.all([
    import('bgutils-js/botguard'), import('bgutils-js/webpo'), import('bgutils-js/utils'), import('jsdom'),
  ]);
  return { ...botguard, ...webpo, ...utils, ...dom };
};

// Extract an object without executing page JavaScript. Quoted braces and escaped
// quotes must not terminate it. The caller still parses it as JSON/loose JSON.
function objectArgument(html, marker) {
  const match = marker.exec(html);
  if (!match) throw fail();
  const start = match.index + match[0].length;
  if (html[start] !== '{') throw fail();
  let depth = 0, quote = '', escaped = false;
  for (let i = start; i < html.length; i++) {
    const char = html[i];
    if (quote) {
      if (escaped) escaped = false;
      else if (char === '\\') escaped = true;
      else if (char === quote) quote = '';
    } else if (char === '"' || char === "'") quote = char;
    else if (char === '{') depth++;
    else if (char === '}' && --depth === 0) return html.slice(start, i + 1);
  }
  throw fail();
}

export function parseProofPage(html, parseLooseJSON) {
  if (!text(html, PROOF_RESPONSE_LIMIT)) throw fail();
  try {
    const config = JSON.parse(objectArgument(html, /\bytcfg\.set\s*\(\s*/));
    const attestation = parseLooseJSON(objectArgument(html, /\bwindow\.ytAtN\s*\(\s*/));
    const challenge = attestation?.R?.bgChallenge;
    if (!record(config) || config.LOGGED_IN === true || !record(challenge) ||
        !text(challenge.program, 8 * 1024 * 1024) || !text(challenge.globalName, 128) ||
        !/^[A-Za-z_$][A-Za-z0-9_$]*$/.test(challenge.globalName) ||
        ['__proto__', 'constructor', 'prototype'].includes(challenge.globalName)) throw fail();
    const interpreter = challenge.interpreterUrl?.privateDoNotAccessOrElseTrustedResourceUrlWrappedValue;
    if (!text(interpreter, 8192)) throw fail('interpreter_route');
    const url = new URL(interpreter, 'https://www.youtube.com/');
    if (url.origin !== 'https://www.google.com' || !url.pathname.startsWith('/js/th/') || url.username || url.password || url.hash)
      throw fail('interpreter_route');
    return { config, program: challenge.program, globalName: challenge.globalName, interpreterURL: url.toString() };
  } catch (error) { throw fail(error?.safeReason); }
}

// One instance belongs to one restricted engine child. Use the host realm, as
// in the MIT example: WebPoMinter checks Function/Uint8Array with instanceof.
// JSDOM never executes page scripts or loads subresources. Restore globals on
// close; engine replacement also terminates and reaps the owning process.
export class ProofSession {
  #request; #loadLibraries; #dom; #client; #minter; #expiresAt = 0; #closed = false; #globals = new Map();
  constructor({ request, libraries = loadLibraries }) { this.#request = request; this.#loadLibraries = libraries; }
  #check() { if (this.#closed) throw fail(); }

  async initialize() {
    this.#check();
    const { BotGuardClient, WebPoMinter, buildURL, getHeaders, parseLooseJSON, USER_AGENT, JSDOM, VirtualConsole } = await this.#loadLibraries();
    this.#check();
    const html = await this.#request({ url: 'https://www.youtube.com/', headers: {
      accept: 'text/html', 'accept-language': 'en-US,en;q=0.7', 'user-agent': USER_AGENT,
    } });
    this.#check();
    const page = parseProofPage(html, parseLooseJSON);
    const script = await this.#request({ url: page.interpreterURL, headers: { 'user-agent': USER_AGENT } });
    this.#check();
    if (!text(script, PROOF_RESPONSE_LIMIT)) throw fail('interpreter_route');
    this.#dom = new JSDOM('<!doctype html><html lang="en"><head></head><body></body></html>', {
      url: 'https://www.youtube.com/', referrer: 'https://www.youtube.com/',
      pretendToBeVisual: true, virtualConsole: new VirtualConsole(),
    });
    const window = this.#dom.window;
    Object.defineProperty(window.navigator, 'userAgent', { value: USER_AGENT, configurable: true });
    window.yt = { config_: page.config };
    for (const [key, value] of Object.entries({ window, document: window.document, location: window.location,
      origin: window.origin, navigator: window.navigator, yt: window.yt })) {
      this.#globals.set(key, Object.getOwnPropertyDescriptor(globalThis, key));
      Object.defineProperty(globalThis, key, { value, writable: true, configurable: true });
    }
    if (page.globalName in globalThis) throw fail();
    this.#globals.set(page.globalName, undefined);
    // Match the example's function scope so interpreter-local declarations do
    // not become permanent worker globals, while bounding synchronous startup.
    vm.runInThisContext(`(function () {\n${script}\n}).call(globalThis);`, { timeout: 5000 });
    const client = await BotGuardClient.create({ program: page.program, globalName: page.globalName, globalObject: globalThis });
    if (this.#closed) { void client.shutdown().catch(() => {}); throw fail(); }
    this.#client = client;
    const webPoSignalOutput = [];
    const response = await client.snapshot({ webPoSignalOutput });
    this.#check();
    if (!text(response, 128 * 1024)) throw fail('youtube_integrity');
    const payload = await this.#request({ url: buildURL('GenerateIT', false), method: 'POST', headers: getHeaders(),
      data: JSON.stringify([REQUEST_KEY, response]),
    });
    this.#check();
    let values;
    try { values = JSON.parse(payload); } catch { throw fail('youtube_integrity'); }
    if (!Array.isArray(values)) throw fail('youtube_integrity');
    const [integrityToken, estimatedTtlSecs, mintRefreshThreshold = 0] = values;
    if (!text(integrityToken, 24 * 1024) || !Number.isFinite(estimatedTtlSecs) || estimatedTtlSecs > 86400 ||
        !Number.isFinite(mintRefreshThreshold) || mintRefreshThreshold < 0 || estimatedTtlSecs - mintRefreshThreshold <= 60)
      throw fail('youtube_integrity', 'proof_expired');
    this.#expiresAt = Date.now() + Math.floor((estimatedTtlSecs - mintRefreshThreshold) * 1000);
    const minter = await WebPoMinter.create({ integrityToken, estimatedTtlSecs, mintRefreshThreshold }, webPoSignalOutput);
    this.#check();
    this.#minter = minter;
  }

  async mint(binding) {
    this.#check();
    if (!this.#minter || this.#expiresAt <= Date.now() + 60_000) throw fail('youtube_integrity', 'proof_expired');
    const token = await this.#minter.mintAsWebsafeString(binding);
    this.#check();
    return { token, expiresAt: this.#expiresAt };
  }
  close() {
    if (this.#closed) return;
    this.#closed = true;
    if (this.#client) void this.#client.shutdown().catch(() => {});
    this.#dom?.window.close();
    for (const [key, descriptor] of this.#globals) {
      if (descriptor) Object.defineProperty(globalThis, key, descriptor);
      else Reflect.deleteProperty(globalThis, key);
    }
    this.#globals.clear();
    this.#client = this.#minter = this.#dom = undefined;
    this.#expiresAt = 0;
  }
}
