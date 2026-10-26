import test from 'node:test';
import assert from 'node:assert/strict';
import {readFileSync} from 'node:fs';
import vm from 'node:vm';

const source = readFileSync(new URL('./music.js', import.meta.url), 'utf8')
  .replace('YTMGO_APPEARANCE', '"system"').replace('YTMGO_STYLES', '""');

function harness({origin = 'https://music.youtube.com', frame = false, early = false} = {}) {
  const root = {dataset: {}, append() {}};
  const elements = new Map();
  const links = [];
  const observers = [];
  const navigations = [];
  const events = {};
  const window = {};
  window.top = frame ? {} : window;
  const media = {matches: false, addEventListener(_, fn) { this.change = fn; }};
  const document = {
    documentElement: early ? null : root, body: null, head: {append() {}},
    createElement() { return {}; }, getElementById() { return null; },
    querySelector(selector) { return elements.get(selector) ?? null; },
    querySelectorAll() { return links; }, addEventListener(name, fn) { events[name] = fn; },
  };
  const location = {origin, pathname: '/', assign(path) { navigations.push(path); }, set href(path) { navigations.push(path); }};
  class MutationObserver {
    constructor(fn) { this.fn = fn; observers.push(this); }
    observe() {} disconnect() { this.disconnected = true; }
  }
  vm.runInNewContext(source, {window, document, location, URL, MutationObserver,
    matchMedia: () => media, requestAnimationFrame: fn => fn()});
  return {window, document, root, media, elements, links, observers, navigations, events};
}

test('account pages and embedded frames never acquire music controls or observers', () => {
  for (const options of [{origin: 'https://accounts.google.com'}, {frame: true}]) {
    const app = harness(options);
    assert.equal(app.window.ytmgoControls, undefined);
    assert.equal(app.observers.length, 0);
  }
});

test('sign-out stays hidden until a visible account avatar replaces the sign-in control', () => {
  const app = harness();
  const signOut = {hidden: true};
  const visible = () => ({closest:()=>null, getClientRects:()=>[{}], textContent:'登录'});
  app.document.body = {};
  app.elements.set('ytmusic-app', {});
  app.document.getElementById = id => id==='ytmgo-sidebar' ? {} : id==='ytmgo-signout' ? signOut : null;
  const update = app.events['yt-navigate-finish'];
  update();
  assert.equal(signOut.hidden, true, 'loading must not expose sign-out');
  app.elements.set('ytmusic-nav-bar a[href*="accounts.google.com"]',visible());
  app.elements.set('ytmusic-nav-bar #avatar-btn',visible());
  update();
  assert.equal(signOut.hidden,true,'a sign-in button overrides an avatar placeholder');
  assert.deepEqual(app.navigations,['ytmgo://account/signed-out']);
  app.elements.delete('ytmusic-nav-bar a[href*="accounts.google.com"]');
  update();
  assert.equal(signOut.hidden,false);
  assert.equal(app.navigations.at(-1),'ytmgo://account/signed-in');
  update();
  assert.equal(app.navigations.length,2,'DOM churn must not resend unchanged state');
  app.elements.set('ytmusic-nav-bar #avatar-btn',{closest:()=>({}),getClientRects:()=>[{}]});
  update();
  assert.equal(signOut.hidden,true,'hidden avatars cannot establish logged-in state');
  assert.equal(app.navigations.at(-1),'ytmgo://account/signed-out');
});

test('document-start injection mounts once after the document element exists', () => {
  const app = harness({early: true});
  assert.equal(app.window.ytmgoControls, undefined);
  app.document.documentElement = app.root;
  app.observers[0].fn();
  assert.equal(typeof app.window.ytmgoControls, 'function');
  assert.equal(app.observers[0].disconnected, true);
  app.observers[0].fn();
  assert.equal(app.observers.length, 2);
});

test('explicit appearance survives OS changes and returning to system follows them', () => {
  const app = harness();
  assert.equal(app.root.dataset.ytmgoTheme, 'light');
  app.window.ytmgoSetAppearance('dark');
  app.media.change();
  assert.equal(app.root.dataset.ytmgoTheme, 'dark');
  assert.equal(app.window.ytmgoSetAppearance('invalid'), false);
  app.window.ytmgoSetAppearance('system');
  assert.equal(app.root.dataset.ytmgoTheme, 'light');
  app.media.matches = true;
  app.media.change();
  assert.equal(app.root.dataset.ytmgoTheme, 'dark');
});

test('sidebar navigation uses the first-party SPA link without reloading the player', () => {
  const app = harness();
  let clicks = 0;
  app.links.push({href: 'https://other.example/library', click() { throw Error('wrong origin'); }});
  app.links.push({href: 'https://music.youtube.com/library', click() { clicks++; }});
  app.window.ytmgoControls('library');
  assert.equal(clicks, 1);
  assert.deepEqual(app.navigations, []);
  app.window.ytmgoControls('explore');
  assert.deepEqual(app.navigations, ['/explore']);
});

test('transport actions use existing service controls and tolerate a missing player', () => {
  const app = harness();
  assert.equal(app.window.ytmgoControls('toggle'), false);
  assert.equal(app.window.ytmgoControls('volume-up'), false);
  let clicks = 0;
  app.elements.set('ytmusic-player-bar #play-pause-button', {click() { clicks++; }});
  assert.equal(app.window.ytmgoControls('toggle'), true);
  assert.equal(clicks, 1);
  assert.equal(app.window.ytmgoControls('unsupported'), false);
});

test('volume adjustments clamp and mute does not recreate or seek the player', () => {
  const app = harness();
  const player = {volume: .95, muted: false, currentTime: 45};
  app.elements.set('video, audio', player);
  app.window.ytmgoControls('volume-up');
  assert.equal(player.volume, 1);
  player.volume = .05;
  app.window.ytmgoControls('volume-down');
  assert.equal(player.volume, 0);
  app.window.ytmgoControls('mute');
  assert.equal(player.muted, true);
  assert.equal(player.currentTime, 45);
});

test('arrow shortcuts reach playback while text editing and dialogs keep their keys', () => {
  const app = harness();
  let clicks = 0;
  app.elements.set('ytmusic-player-bar .next-button', {click() { clicks++; }});
  const event = {key:'ArrowRight', metaKey:true, target:{closest:()=>null}, preventDefault() {}, stopPropagation() {}};
  app.events.keydown(event);
  assert.equal(clicks, 1);
  app.events.keydown({...event, target:{closest:()=>({})}});
  assert.equal(clicks, 1);
  app.elements.set('tp-yt-paper-dialog[opened]', {});
  app.events.keydown(event);
  assert.equal(clicks, 1);
});
