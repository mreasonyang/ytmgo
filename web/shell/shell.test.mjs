import test from 'node:test';
import assert from 'node:assert/strict';
import {readdirSync, readFileSync} from 'node:fs';
import vm from 'node:vm';
import {buildTestPreload, manifest, shellI18N, languages} from './bundle.mjs';

const sources = new Map();
const sourceFor = (locale = 'en', platform = 'MacIntel') => {
  const key = locale + platform;
  if (!sources.has(key)) sources.set(key, buildTestPreload('system', shellI18N(locale, {choice: locale === 'en' ? 'system' : locale})));
  return sources.get(key);
};
// Values built inside the vm realm have foreign prototypes; compare as data.
const plain = value => JSON.parse(JSON.stringify(value));

// A deliberately small DOM: enough for the shell to build, mount and sync.
// Service elements are registered by selector in `elements`.
class Node {
  constructor(tag = '#text') {
    this.tagName = tag.toUpperCase(); this.children = []; this.attributes = new Map(); this.dataset = {};
    this.listeners = {}; this.textContent = ''; this.parentElement = null; this.hidden = false;
    const classes = new Set();
    this.classList = {add: c => classes.add(c), remove: c => classes.delete(c), contains: c => classes.has(c), toggle: (c, on = !classes.has(c)) => (on ? classes.add(c) : classes.delete(c), on)};
    this.style = {props: {}, setProperty(k, v) { this.props[k] = v; }};
  }
  setAttribute(k, v) { this.attributes.set(k, String(v)); if (k === 'hidden') this.hidden = true; }
  getAttribute(k) { return this.attributes.has(k) ? this.attributes.get(k) : null; }
  hasAttribute(k) { return this.attributes.has(k); }
  removeAttribute(k) { this.attributes.delete(k); }
  append(...nodes) { for (const n of nodes) { n.parentElement = this; this.children.push(n); } }
  prepend(...nodes) { for (const n of nodes) n.parentElement = this; this.children.unshift(...nodes); }
  replaceChildren(...nodes) { this.children = []; this.append(...nodes); }
  replaceWith(n) { const list = this.parentElement.children; list[list.indexOf(this)] = n; n.parentElement = this.parentElement; }
  remove() {}
  addEventListener(type, fn) { (this.listeners[type] ||= []).push(fn); }
  removeEventListener() {}
  querySelector(sel) { return this.find(sel)[0] ?? null; }
  querySelectorAll(sel) { return this.find(sel); }
  find(sel) {
    const out = [];
    const walk = n => { for (const c of n.children) { if (c.matches(sel)) out.push(c); walk(c); } };
    walk(this);
    return out;
  }
  matches(sel) {
    const cls = sel.match(/^\.([\w-]+)$/);
    if (cls) return this.classList.contains(cls[1]);
    const attr = sel.match(/^\[([\w-]+)\]$/);
    return attr ? this.hasAttribute(attr[1]) : false;
  }
  closest() { return null; }
  contains() { return false; }
  getClientRects() { return [{}]; }
  getBoundingClientRect() { return {top: 0, left: 0, right: 0, bottom: 0, width: 0}; }
  focus() {} click() { this.clicks = (this.clicks || 0) + 1; }
  get offsetWidth() { return 0; }
  setPointerCapture() {}
}

function harness({origin = 'https://music.youtube.com', frame = false, early = false, locale = 'en', platform = 'MacIntel'} = {}) {
  const root = new Node('html');
  const body = new Node('body');
  const elements = new Map();
  const observers = [];
  const navigations = [];
  const events = {};
  const frames = [];
  const window = {};
  window.top = frame ? {} : window;
  const media = {matches: false, addEventListener(_, fn) { this.change = fn; }};
  const document = {
    documentElement: early ? null : root, body: null, head: new Node('head'),
    createElement: tag => new Node(tag), createElementNS: (_, tag) => new Node(tag), createTextNode: text => Object.assign(new Node(), {textContent: text}),
    getElementById: () => null,
    querySelector: sel => (Array.isArray(elements.get(sel)) ? elements.get(sel)[0] : elements.get(sel)) ?? body.querySelector(sel),
    querySelectorAll: sel => (Array.isArray(elements.get(sel)) ? elements.get(sel) : elements.has(sel) ? [elements.get(sel)] : body.querySelectorAll(sel)),
    addEventListener(name, fn) { (events[name] ||= []).push(fn); },
    removeEventListener() {},
  };
  const location = {origin, pathname: '/', href: origin + '/', assign(path) { navigations.push(path); }};
  // Mirror the native guard: ytmgo:// navigations are recorded and cancelled.
  const hrefSetter = {set href(value) { navigations.push(value); }, get href() { return location.href; }};
  class MutationObserver {
    constructor(fn) { this.fn = fn; observers.push(this); }
    observe() {} disconnect() { this.disconnected = true; }
  }
  const context = {
    window, document, location: new Proxy(location, {set(target, key, value) { if (key === 'href' && String(value).startsWith('ytmgo://')) { hrefSetter.href = value; return true; } target[key] = value; return true; }}),
    URL, MutationObserver, CustomEvent: class { constructor(type, init) { this.type = type; Object.assign(this, init); } },
    ResizeObserver: class { observe() {} }, navigator: {platform}, Intl, history: {back() {}, forward() {}},
    matchMedia: () => media, requestAnimationFrame: fn => frames.push(fn), performance: {now: () => context.now}, now: 0,
    setTimeout: () => 0, clearTimeout() {}, addEventListener() {}, innerWidth: 1440, innerHeight: 900, HTMLElement: Node,
  };
  vm.runInNewContext(sourceFor(locale, platform), context);
  const flush = () => { for (let i = 0; i < 20 && frames.length; i++) frames.splice(0).forEach(fn => fn()); };
  return {window, document, root, body, media, elements, observers, navigations, events, flush, context, api: () => window.__ytmgoTest};
}

// A mounted shell over a minimal service page.
function mounted(options = {}) {
  const app = harness(options);
  app.document.body = app.body;
  app.elements.set('ytmusic-app', new Node('ytmusic-app'));
  app.flush();
  return app;
}

test('manifest lists every shell module exactly once', () => {
  const files = readdirSync(new URL('./', import.meta.url)).filter(f => /\.(js|css)$/.test(f)).sort();
  assert.deepEqual([...manifest.styles, ...manifest.scripts].sort(), files);
  assert.equal(new Set([...manifest.styles, ...manifest.scripts]).size, files.length);
});

test('sign-in pages and embedded frames never acquire the shell, controls or observers', () => {
  for (const options of [{origin: 'https://accounts.google.com'}, {frame: true}]) {
    const app = harness(options);
    assert.equal(app.window.ytmgoControls, undefined);
    assert.equal(app.window.ytmgoShell, undefined);
    assert.equal(app.observers.length, 0);
  }
});

test('document-start injection boots once the document element exists', () => {
  const app = harness({early: true});
  assert.equal(app.window.ytmgoControls, undefined);
  app.document.documentElement = app.root;
  app.observers[0].fn();
  assert.equal(typeof app.window.ytmgoControls, 'function');
  assert.equal(app.observers[0].disconnected, true);
});

test('the shell mounts its surfaces only once the service app exists', () => {
  const app = harness();
  app.document.body = app.body;
  app.flush();
  assert.equal(app.body.children.length, 0, 'no service app yet');
  app.elements.set('ytmusic-app', new Node('ytmusic-app'));
  app.observers.at(-1).fn();
  app.flush();
  const ids = app.body.children.map(n => n.getAttribute('id')).filter(Boolean);
  for (const id of ['ytmgo-canvas', 'ytmgo-sidebar', 'ytmgo-topbar', 'ytmgo-np-surface', 'ytmgo-np', 'ytmgo-playerbar', 'ytmgo-tabbar']) assert.ok(ids.includes(id), id);
  app.observers.at(-1).fn();
  app.flush();
  assert.equal(app.body.children.filter(n => n.getAttribute('id') === 'ytmgo-sidebar').length, 1, 'idempotent');
});

test('explicit appearance survives OS changes; system follows them', () => {
  const app = harness();
  assert.equal(app.root.dataset.ytmgoTheme, 'light');
  app.window.ytmgoSetAppearance('dark');
  app.media.change();
  assert.equal(app.root.dataset.ytmgoTheme, 'dark');
  assert.equal(app.window.ytmgoSetAppearance('invalid'), false);
  app.window.ytmgoSetAppearance('system');
  app.media.matches = true;
  app.media.change();
  assert.equal(app.root.dataset.ytmgoTheme, 'dark');
  assert.equal(app.api().store.state.appearance, 'system');
});

test('account state reports display booleans only, and a sign-in control outranks an avatar', () => {
  const app = mounted();
  const visible = () => Object.assign(new Node('a'), {closest: () => null});
  assert.deepEqual(app.navigations, ['ytmgo://account/signed-out'], 'unknown state reports signed-out');
  app.elements.set('ytmusic-nav-bar a[href*="accounts.google.com"]', visible());
  app.elements.set('ytmusic-nav-bar #avatar-btn', visible());
  app.observers.at(-1).fn(); app.flush();
  assert.equal(app.api().store.state.signedIn, false);
  app.elements.delete('ytmusic-nav-bar a[href*="accounts.google.com"]');
  app.observers.at(-1).fn(); app.flush();
  assert.equal(app.api().store.state.signedIn, true);
  assert.equal(app.navigations.at(-1), 'ytmgo://account/signed-in');
  app.observers.at(-1).fn(); app.flush();
  assert.equal(app.navigations.length, 2, 'DOM churn must not resend unchanged state');
  app.elements.set('ytmusic-nav-bar #avatar-btn', Object.assign(new Node('a'), {closest: () => ({})}));
  app.observers.at(-1).fn(); app.flush();
  assert.equal(app.navigations.at(-1), 'ytmgo://account/signed-out', 'hidden avatars cannot establish signed-in');
});

test('primary destinations use the service SPA event, never a reload', () => {
  const app = harness();
  const layout = new Node('ytmusic-app-layout');
  const dispatched = [];
  layout.dispatchEvent = event => dispatched.push(event);
  assert.equal(app.window.ytmgoControls('library'), true);
  assert.deepEqual(app.navigations, ['/library'], 'falls back to a URL before the app exists');
  app.elements.set('ytmusic-app-layout', layout);
  app.window.ytmgoControls('explore');
  assert.equal(dispatched[0].type, 'yt-navigate');
  assert.equal(dispatched[0].detail.endpoint.browseEndpoint.browseId, 'FEmusic_explore');
  assert.equal(dispatched[0].bubbles, true);
  assert.equal(app.navigations.length, 1);
});

test('transport uses existing service controls and tolerates a missing player', () => {
  const app = harness();
  assert.equal(app.window.ytmgoControls('toggle'), false);
  assert.equal(app.window.ytmgoControls('next'), false);
  const play = new Node('button');
  app.elements.set('ytmusic-player-bar #play-pause-button', play);
  app.context.now = 1000;
  assert.equal(app.window.ytmgoControls('toggle'), true);
  assert.equal(play.clicks, 1);
  assert.equal(app.window.ytmgoControls('unsupported'), false);
});

test('toggles arriving from both the menu and the page within 250ms run once', () => {
  const app = harness();
  const play = new Node('button');
  app.elements.set('ytmusic-player-bar #play-pause-button', play);
  app.context.now = 5000;
  app.window.ytmgoControls('toggle');
  app.context.now = 5100;
  app.window.ytmgoControls('toggle');
  assert.equal(play.clicks, 1);
  app.context.now = 5400;
  app.window.ytmgoControls('toggle');
  assert.equal(play.clicks, 2);
  // Non-toggles (next track) are never de-duplicated.
  const next = new Node('button');
  app.elements.set('ytmusic-player-bar .next-button', next);
  app.window.ytmgoControls('next'); app.window.ytmgoControls('next');
  assert.equal(next.clicks, 2);
});

test('volume goes through the player API, clamps, and unmutes when raised', () => {
  const app = harness();
  const calls = [];
  let muted = true;
  app.elements.set('#movie_player', Object.assign(new Node('div'), {
    setVolume: v => calls.push(['set', v]), isMuted: () => muted, unMute: () => { muted = false; calls.push(['unmute']); }, mute() {},
  }));
  app.api().store.set({volume: .95});
  app.window.ytmgoControls('volume-up');
  assert.deepEqual(calls, [['set', 100], ['unmute']]);
  app.api().store.set({volume: .05});
  app.window.ytmgoControls('volume-down');
  assert.deepEqual(calls.at(-1), ['set', 0]);
});

test('playback state reads songs vs videos, ads, likes and linked artists', () => {
  const app = harness();
  const {bridge} = app.api();
  const layout = new Node('ytmusic-app-layout');
  layout.setAttribute('player-ui-state', 'INACTIVE');
  app.elements.set('ytmusic-app-layout', layout);
  app.elements.set('ytmusic-player-bar', new Node('ytmusic-player-bar'));
  assert.equal(bridge.readPlayback().active, false);
  layout.setAttribute('player-ui-state', 'MINIPLAYER');
  const host = new Node('ytmusic-player');
  host.setAttribute('playback-mode', 'ATV_PREFERRED');
  host.setAttribute('video-mode', '');
  app.elements.set('ytmusic-player', host);
  const player = new Node('div');
  app.elements.set('#movie_player', player);
  const byline = new Node('span');
  app.elements.set('ytmusic-player-bar .content-info-wrapper .byline', byline);
  app.elements.set('a', [Object.assign(new Node('a'), {textContent: 'Daft Punk'}), Object.assign(new Node('a'), {textContent: 'Discovery'})]);
  byline.querySelectorAll = () => app.elements.get('a');
  const like = new Node('ytmusic-like-button-renderer');
  like.setAttribute('like-status', 'LIKE');
  app.elements.set('ytmusic-player-bar ytmusic-like-button-renderer', like);
  let state = bridge.readPlayback();
  assert.equal(state.active, true);
  assert.equal(state.video, false, 'art-track songs keep the square frame despite video-mode');
  assert.equal(state.like, 'LIKE');
  assert.deepEqual(plain(state.artists.map(a => [a.name, a.linked])), [['Daft Punk', true], ['Discovery', true]]);
  host.setAttribute('playback-mode', 'OMV_PREFERRED');
  assert.equal(bridge.readPlayback().video, true);
  player.classList.add('ad-showing');
  state = bridge.readPlayback();
  assert.equal(state.ad, true);
  assert.deepEqual(plain(state.artists), [], 'ads never show service byline links');
});

test('artwork is upgraded to a sharp size and empty placeholders are ignored', () => {
  const app = harness();
  const {bridge} = app.api();
  assert.equal(bridge.readArt({artwork: [{sizes: '60x60', src: 'https://lh3.googleusercontent.com/a=w60-h60-l90-rj'}, {sizes: '544x544', src: 'https://lh3.googleusercontent.com/a=w544-h544-l90-rj'}]}), 'https://lh3.googleusercontent.com/a=w720-h720-l90-rj');
  app.elements.set('ytmusic-player-bar .thumbnail-image-wrapper img', {src: 'https://music.youtube.com/'});
  assert.equal(bridge.readArt(null), '');
});

test('routes map to canvas destinations; the player page is not a destination', () => {
  const {bridge} = harness().api();
  assert.deepEqual(['/', '/explore', '/library/playlists', '/search', '/watch', '/playlist', '/channel/x'].map(p => bridge.routeOf(p)),
    ['home', 'explore', 'library', 'search', 'watch', 'browse', 'browse']);
});

test('keyboard: arrows reach playback, text fields and dialogs keep their keys, / focuses search', () => {
  const app = harness();
  const next = new Node('button');
  app.elements.set('ytmusic-player-bar .next-button', next);
  const keydown = app.events.keydown[0];
  const press = (key, extra = {}) => { const e = {key, metaKey: false, ctrlKey: false, altKey: false, shiftKey: false, target: {closest: () => null}, prevented: false, preventDefault() { this.prevented = true; }, stopPropagation() {}, ...extra}; keydown(e); return e; };
  press('ArrowRight', {metaKey: true});
  assert.equal(next.clicks, 1);
  press('ArrowRight', {metaKey: true, target: {closest: () => ({})}});
  assert.equal(next.clicks, 1, 'typing keeps arrows');
  app.elements.set('tp-yt-paper-dialog[opened]', new Node('div'));
  press('ArrowRight', {metaKey: true});
  assert.equal(next.clicks, 1, 'service dialogs keep arrows');
  app.elements.delete('tp-yt-paper-dialog[opened]');
  const input = Object.assign(new Node('input'), {select() { this.selected = true; }});
  app.elements.set('ytmusic-search-box input', input);
  assert.equal(press('/').prevented, true);
  assert.equal(input.selected, true);
  assert.equal(press('/', {target: {closest: () => ({})}}).prevented, false, 'typing a slash stays a slash');
});

test('Escape peels one layer at a time: immersive, then an overlaid panel', () => {
  const app = mounted();
  const {store, dismissTopmost, setImmersive} = app.api();
  store.set({active: true});
  setImmersive(true);
  assert.equal(app.root.dataset.ytmgoView, 'immersive');
  assert.equal(dismissTopmost(), true);
  assert.equal(app.root.dataset.ytmgoView, 'browse');
  store.set({panel: true});
  // Docked panels (wide windows) are part of the layout, not a layer.
  assert.equal(dismissTopmost(), false);
  assert.equal(store.state.panel, true);
});

test('panel buttons toggle their own tab and switch between tabs without closing', () => {
  const app = mounted();
  const {store, showPanel} = app.api();
  const tabs = [new Node('tab'), new Node('tab')];
  app.elements.set('ytmusic-player-page #side-panel tp-yt-paper-tab', tabs);
  showPanel(0);
  assert.equal(store.state.panel, true);
  showPanel(1);
  assert.equal(store.state.panel, true);
  assert.equal(tabs[1].clicks, 1);
  showPanel(1);
  assert.equal(store.state.panel, false, 'same tab closes');
  assert.equal(app.root.dataset.ytmgoPanel, 'closed');
});

test('immersive mode requires something playing', () => {
  const app = mounted();
  const {store, setImmersive} = app.api();
  setImmersive(true);
  assert.equal(store.state.immersive, false);
});

test('time formatting covers hours and invalid input', () => {
  const {formatTime} = harness().api();
  assert.deepEqual([formatTime(0), formatTime(65.9), formatTime(3725), formatTime(NaN), formatTime(-1)], ['0:00', '1:05', '1:02:05', '0:00', '0:00']);
});

test('a failing watcher cannot starve the others of a change', () => {
  const app = harness();
  const {store} = app.api();
  app.context.console = {error() {}};
  const seen = [];
  store.watch(['title'], () => { if (store.state.title === 'boom') throw new Error('render failed'); });
  store.watch(['title'], ({title}) => seen.push(title));
  store.set({title: 'boom'});
  assert.deepEqual(seen.slice(-1), ['boom']);
});

test('a loaded media source counts as active even when the layout state lags', () => {
  const app = harness();
  const {bridge} = app.api();
  const layout = new Node('ytmusic-app-layout');
  layout.setAttribute('player-ui-state', 'INACTIVE');
  app.elements.set('ytmusic-app-layout', layout);
  // No service player bar at all: it is re-created at times during playback.
  assert.equal(bridge.readPlayback().active, false);
  app.elements.set('#movie_player video', Object.assign(new Node('video'), {currentSrc: 'blob:https://music.youtube.com/x', paused: false}));
  assert.equal(bridge.readPlayback().active, true);
});

// ── i18n ──────────────────────────────────────────────────────────────
const shellDir = new URL('./', import.meta.url);
const readSource = name => readFileSync(new URL(name, shellDir), 'utf8');
const catalogs = Object.fromEntries(languages.map(tag => [tag, JSON.parse(readFileSync(new URL(`../../locales/${tag}.json`, import.meta.url), 'utf8'))]));
const shellKeys = Object.keys(catalogs.en).filter(k => k.startsWith('shell'));

test('every shell message exists in every language with the same {slots}', () => {
  const slots = text => [...text.matchAll(/\{(\w+)\}/g)].map(m => m[1]).sort().join();
  for (const tag of languages) {
    for (const key of shellKeys) {
      assert.equal(typeof catalogs[tag][key], 'string', `${tag}: ${key} missing`);
      assert.equal(slots(catalogs[tag][key]), slots(catalogs.en[key]), `${tag}: ${key} placeholders differ`);
    }
  }
});

test('every message key the shell uses exists, and every shell message is used', () => {
  const code = manifest.scripts.map(readSource).join('\n');
  const used = new Set([...code.matchAll(/'(shell[A-Z]\w*)'/g)].map(m => m[1]));
  for (const key of used) assert.ok(shellKeys.includes(key), `unknown message ${key}`);
  for (const key of shellKeys) assert.ok(used.has(key), `unused message ${key}`);
});

test('no user-facing literal strings in shell code: all text goes through t()', () => {
  const offenders = [];
  for (const name of manifest.scripts.filter(n => n !== 'i18n.js')) {
    readSource(name).split('\n').forEach((line, i) => {
      if (/^\s*\/\//.test(line)) return;
      const literal = /(?:\btext|'aria-label'|'data-tip'|\blabel|placeholder)\s*:\s*['`][^'`]*\p{L}/u.test(line)
        || /\b(?:toast|announce)\(\s*['`]/.test(line)
        || /\bsetText\([^,]+,\s*['`][^'`]*\p{L}/u.test(line);
      if (literal) offenders.push(`${name}:${i + 1}: ${line.trim()}`);
    });
  }
  assert.deepEqual(offenders, []);
});

test('t() fills slots, leaves unknown slots visible, and falls back to the key', () => {
  const {t} = harness({locale: 'zh-Hant'}).api();
  assert.equal(t('shellAnnounceTrack', {title: 'Beat It'}), '正在播放：Beat It');
  assert.equal(t('shellAnnounceTrack'), '正在播放：{title}');
  assert.equal(t('shellNope'), 'shellNope');
});

test('shortcut key caps follow the platform and the language', () => {
  assert.equal(harness({locale: 'ja'}).api().shortcut(['mod', 'shift', 'space']), '⌘ ⇧ Space');
  assert.equal(harness({locale: 'zh-Hans', platform: 'Linux x86_64'}).api().shortcut(['mod', 'shift', 'space'], ['esc']), 'Ctrl Shift 空格 / Esc');
});

test('greetings come from the catalog for each time of day', () => {
  const {greetingText} = harness({locale: 'ja'}).api();
  assert.deepEqual([3, 8, 12, 15, 21].map(greetingText), ['夜更かしのお供に', 'おはようございます', 'こんにちは', 'こんにちは', 'こんばんは']);
});

test('a mounted shell speaks its language and declares it on every root', () => {
  for (const locale of languages) {
    const app = mounted({locale});
    assert.equal(app.root.dataset.ytmgoScript, shellI18N(locale).script);
    const roots = app.body.children.filter(n => ['ytmgo-sidebar', 'ytmgo-topbar', 'ytmgo-np', 'ytmgo-np-surface', 'ytmgo-playerbar', 'ytmgo-tabbar'].includes(n.getAttribute('id')));
    assert.equal(roots.length, 6);
    for (const root of roots) assert.equal(root.lang, locale, `${root.getAttribute('id')} lang`);
    const sidebar = roots.find(n => n.getAttribute('id') === 'ytmgo-sidebar');
    assert.equal(sidebar.getAttribute('aria-label'), catalogs[locale].shellNavMain);
  }
});

test('the language picker asks the native side for exactly the chosen language', () => {
  const app = mounted({locale: 'ja'});
  app.navigations.length = 0;
  const {bridge} = app.api();
  bridge.setLanguage('zh-Hant');
  assert.deepEqual(app.navigations, ['ytmgo://language/zh-Hant']);
});
