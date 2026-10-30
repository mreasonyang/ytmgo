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

// Bound an object without executing page JavaScript. Quoted braces and escaped
// quotes must not terminate it. The caller still parses it as JSON/loose JSON.
function objectArgumentEnd(html, start) {
  let depth = 0, quote = '', escaped = false;
  for (let i = start; i < html.length; i++) {
    const char = html[i];
    if (quote) {
      if (escaped) escaped = false;
      else if (char === '\\') escaped = true;
      else if (char === quote) quote = '';
    } else if (char === '"' || char === "'") quote = char;
    else if (char === '{') depth++;
    else if (char === '}' && --depth === 0) return i + 1;
  }
  throw fail();
}

function* objectArguments(html, marker, parse) {
  for (let match; (match = marker.exec(html));) {
    const start = match.index + match[0].length;
    // Helper calls can precede the data-bearing calls. Only literal objects
    // are readable here; never evaluate a variable or an expression.
    if (html[start] !== '{') continue;
    const end = objectArgumentEnd(html, start);
    marker.lastIndex = end;
    let after = end;
    while (after < html.length && /\s/.test(html[after])) after++;
    if (html[after] !== ')') continue;
    let value;
    try { value = parse(html.slice(start, end)); } catch { continue; }
    if (record(value)) yield value;
  }
}

export function parseProofPage(html, parseLooseJSON) {
  if (!text(html, PROOF_RESPONSE_LIMIT)) throw fail();
  try {
    let config;
    for (const value of objectArguments(html, /\bytcfg\.set\s*\(\s*/g, JSON.parse)) {
      // Preserve set order without invoking the __proto__ setter. Reject any
      // signed-in config, even if a later update would overwrite the flag.
      if (value.LOGGED_IN === true) throw fail();
      config = { ...config, ...value };
    }
    let challenge;
    for (const value of objectArguments(html, /\bwindow\.ytAtN\s*\(\s*/g, parseLooseJSON)) {
      if (value.R?.bgChallenge === undefined) continue;
      challenge = value.R.bgChallenge;
      break;
    }
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

// The interpreter needs a complete Window realm. Adapt its callbacks and byte
// arrays at the MIT library boundary, where WebPoMinter uses host instanceof
// checks. Neither the page's scripts nor its subresources are executed/loaded.
function hostSignals(signals, window) {
  return signals.map(signal => async integrity => {
    if (typeof signal !== 'function') throw fail('youtube_integrity');
    const mint = await signal(new window.Uint8Array(integrity));
    if (typeof mint !== 'function') throw fail('youtube_integrity');
    return async binding => {
      const token = await mint(new window.Uint8Array(binding));
      if (!(token instanceof window.Uint8Array) || token.byteLength > 24 * 1024) throw fail('youtube_integrity');
      return new Uint8Array(token);
    };
  });
}

// One instance belongs to one restricted engine child. The DOM owns all
// interpreter globals; close disposes it without changing Node's globals.
export class ProofSession {
  #request; #loadLibraries; #dom; #client; #minter; #expiresAt = 0; #closed = false;
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
      runScripts: 'outside-only', pretendToBeVisual: true, virtualConsole: new VirtualConsole(),
    });
    const window = this.#dom.window;
    Object.defineProperty(window.navigator, 'userAgent', { value: USER_AGENT, configurable: true });
    window.yt = { config_: page.config };
    if (page.globalName in window) throw fail();
    // Only the allowlisted interpreter runs, with bounded synchronous startup.
    // Its Window, globalThis, DOM and built-in constructors share one realm.
    vm.runInContext(`(function () {\n${script}\n}).call(globalThis);`, this.#dom.getInternalVMContext(), { timeout: 5000 });
    const client = await BotGuardClient.create({ program: page.program, globalName: page.globalName, globalObject: window });
    if (this.#closed) { void client.shutdown().catch(() => {}); throw fail(); }
    this.#client = client;
    const webPoSignalOutput = [];
    const response = await client.snapshot({ webPoSignalOutput });
    this.#check();
    if (!text(response, 128 * 1024) || !webPoSignalOutput.length ||
        webPoSignalOutput.some(signal => typeof signal !== 'function')) throw fail('youtube_integrity');
    const payload = await this.#request({ url: buildURL('GenerateIT', false), method: 'POST', headers: getHeaders(),
      data: JSON.stringify([REQUEST_KEY, response]),
    });
    this.#check();
    let values;
    try { values = JSON.parse(payload); } catch { throw fail('youtube_integrity'); }
    if (!Array.isArray(values)) throw fail('youtube_integrity');
    const [integrityToken, estimatedTtlSecs, refreshThreshold] = values;
    if (!text(integrityToken, 24 * 1024)) throw fail('youtube_integrity');
    const mintRefreshThreshold = refreshThreshold ?? 0;
    if (!Number.isFinite(estimatedTtlSecs) || estimatedTtlSecs > 86400 ||
        !Number.isFinite(mintRefreshThreshold) || mintRefreshThreshold < 0 || estimatedTtlSecs - mintRefreshThreshold <= 60)
      throw fail('youtube_integrity', 'proof_expired');
    this.#expiresAt = Date.now() + Math.floor((estimatedTtlSecs - mintRefreshThreshold) * 1000);
    const minter = await WebPoMinter.create({ integrityToken, estimatedTtlSecs, mintRefreshThreshold }, hostSignals(webPoSignalOutput, window));
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
    this.#client = this.#minter = this.#dom = undefined;
    this.#expiresAt = 0;
  }
}
