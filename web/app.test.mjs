import test from 'node:test';
import assert from 'node:assert/strict';
import {readFileSync} from 'node:fs';
import vm from 'node:vm';

const source = readFileSync(new URL('./app.js', import.meta.url), 'utf8');
const css = readFileSync(new URL('./style.css', import.meta.url), 'utf8');
const trackA = {id: 'abcdefghijk', title: 'Track A', artist: 'Artist A', album: '', duration: '3:45', thumbnail: 'https://i.ytimg.com/vi/abcdefghijk/hqdefault.jpg'};
const trackB = {id: 'lmnopqrstuv', title: 'Track B', artist: '', album: '', duration: '4:12', thumbnail: ''};
const copy = value => JSON.parse(JSON.stringify(value));
const deferred = () => { let resolve, reject; const promise = new Promise((yes, no) => { resolve = yes; reject = no; }); return {promise, resolve, reject}; };
const settle = async () => { for (let i = 0; i < 12; i++) await Promise.resolve(); };
const imagesIn = html => [...html.matchAll(/<img\b([^>]+)>/g)].map(([, attributes]) => Object.fromEntries([...attributes.matchAll(/([\w-]+)="([^"]*)"/g)].map(([, name, value]) => [name, value.replace(/&quot;/g, '"').replace(/&#39;/g, "'").replace(/&lt;/g, '<').replace(/&gt;/g, '>').replace(/&amp;/g, '&')])));

class Element {
  constructor() { this.innerHTML = ''; this.textContent = ''; this.value = ''; this.dataset = {}; this.handlers = {}; this.attributes = {}; this.style = {setProperty() {}}; this.classList = {toggle() {}, add() {}, remove() {}}; this.disabled = false; this.hidden = false; this.scrollTop = 0; this.open = false; }
  addEventListener(name, fn) { this.handlers[name] = fn; }
  setAttribute(name, value) { this.attributes[name] = value; }
  removeAttribute(name) { delete this.attributes[name]; }
  focus() { this.focused = true; }
  select() {}
  remove() {}
  append() {}
  showModal() { this.open = true; }
  close() { this.open = false; }
  emit(name, extra = {}) { this.handlers[name]?.({preventDefault() {}, target: this, ...extra}); }
}

class ImageElement extends Element {
  constructor(attributes) {
    super();
    this.requests = [];
    this.src = attributes.src;
    this.dataset.originalSrc = attributes['data-original-src'];
    this.dataset.imageCandidates = attributes['data-image-candidates'];
    this.dataset.videoCover = attributes['data-video-cover'];
    this.naturalWidth = 768;
    this.parentElement = new Element();
  }
  set src(value) { this._src = value; this.requests.push(value); }
  get src() { return this._src; }
}

function harness(handlers = {}) {
  const elements = new Map();
  const el = selector => { if (!elements.has(selector)) elements.set(selector, new Element()); return elements.get(selector); };
  const documentEvents = {};
  const requests = [];
  const players = [];
  const timerCallbacks = [];
  const document = {querySelector: el, querySelectorAll: () => [], getElementById: id => el(`#${id}`), createElement: () => new Element(), head: {append() {}}, addEventListener: (name, fn) => { documentEvents[name] = fn; }};
  class Player {
    constructor(_id, options) { this.options = options; this.calls = []; this.duration = 225; this.currentTime = 0; this.iframe = new Element(); players.push(this); Promise.resolve().then(() => options.events.onReady({target: this})); }
    getIframe() { return this.iframe; }
    setVolume(value) { this.calls.push(['volume', value]); }
    mute() { this.calls.push(['mute']); }
    unMute() { this.calls.push(['unmute']); }
    playVideo() { this.calls.push(['play']); this.options.events.onStateChange({data: 1}); }
    pauseVideo() { this.calls.push(['pause']); this.options.events.onStateChange({data: 2}); }
    loadVideoById(value) { this.calls.push(['load', value]); }
    seekTo(value) { this.currentTime = value; this.calls.push(['seek', value]); }
    getCurrentTime() { return this.currentTime; }
    getDuration() { return this.duration; }
    destroy() { this.calls.push(['destroy']); }
  }
  const window = {
    mygo: {async call(method, ...args) {
      requests.push({method, args: copy(args)});
      if (handlers[method]) return handlers[method](...args);
      if (method === 'Service.Home') return {sections: [{title: 'Selection', subtitle: '', tracks: [trackA, trackB]}]};
      if (method === 'Service.Library') return {liked: [], playlists: []};
      if (method === 'Service.SaveLibrary') return args[0];
      if (method === 'Service.Search') return {tracks: [trackB]};
    }},
    YT: {Player, PlayerState: {PLAYING: 1, PAUSED: 2, BUFFERING: 3, CUED: 5, ENDED: 0}},
    crypto: {randomUUID: () => 'test-uuid'},
  };
  vm.runInNewContext(source, {window, document, location: {hostname: 'si.kyun.ytmgo', origin: 'https://si.kyun.ytmgo'}, URL, AbortController, setTimeout: fn => { timerCallbacks.push(fn); return timerCallbacks.length; }, clearTimeout() {}, setInterval() {}, HTMLImageElement: ImageElement, HTMLButtonElement: class {}, HTMLAnchorElement: class {}, HTMLInputElement: class {}, HTMLTextAreaElement: class {}});
  const click = (data, kind = 'action') => documentEvents.click({target: {closest(selector) { return selector === `[data-${kind}]` ? {dataset: data, disabled: false} : null; }}});
  const search = query => { el('#search-input').value = query; el('#search-form').emit('submit'); };
  return {window, el, requests, players, click, search, timerCallbacks, keyEvent: event => documentEvents.keydown({preventDefault() {}, target: {}, ...event}), imageEvent: (name, image) => documentEvents[name]({target: image})};
}

test('home shows real response tracks and never initializes a player on launch', async () => {
  const app = harness();
  await settle();
  assert.match(app.el('#main-content').innerHTML, /Track A/);
  assert.match(app.el('#main-content').innerHTML, /Track B/);
  assert.equal(app.players.length, 0);
  assert.deepEqual(app.requests.map(r => r.method), ['Service.Home', 'Service.Library']);
});

test('the empty mount placeholder rule cannot hide the real YouTube iframe', () => {
  assert.match(css, /\.video-stage div#youtube-mount:empty\{display:none\}/);
  assert.doesNotMatch(css, /\.video-stage\s+#youtube-mount:empty\{display:none\}/);
  assert.match(css, /\.video-stage>iframe\{[^}]*min-width:200px;min-height:200px/);
});

test('failed library reads block every write instead of replacing stored data', async () => {
  const app = harness({'Service.Library': () => Promise.reject(new Error('unreadable'))});
  await settle();
  app.click({action: 'like', id: trackA.id});
  app.click({action: 'create-playlist'});
  await settle();
  assert.equal(app.requests.filter(r => r.method === 'Service.SaveLibrary').length, 0);
  assert.match(app.el('#toast').textContent, /读取失败/);
  app.click({nav: 'liked'}, 'nav');
  assert.match(app.el('#main-content').innerHTML, /已暂停保存/);
});

test('rapid favorite changes serialize complete snapshots without dropping earlier clicks', async () => {
  const firstSave = deferred();
  let saveCount = 0;
  const app = harness({'Service.SaveLibrary': value => ++saveCount === 1 ? firstSave.promise : value});
  await settle();
  app.click({action: 'like', id: trackA.id});
  app.click({action: 'like', id: trackB.id});
  await settle();
  let saves = app.requests.filter(r => r.method === 'Service.SaveLibrary');
  assert.equal(saves.length, 1);
  assert.deepEqual(saves[0].args[0].liked.map(t => t.id), [trackA.id]);
  assert.equal(app.el('#liked-count').textContent, 2);
  firstSave.resolve(saves[0].args[0]);
  await settle();
  saves = app.requests.filter(r => r.method === 'Service.SaveLibrary');
  assert.equal(saves.length, 2);
  assert.deepEqual(saves[1].args[0].liked.map(t => t.id), [trackB.id, trackA.id]);
  assert.equal(app.el('#liked-count').textContent, 2);
});

test('failed writes roll the UI back to the last confirmed library', async () => {
  const app = harness({'Service.SaveLibrary': () => Promise.reject(new Error('disk full'))});
  await settle();
  app.click({action: 'like', id: trackA.id});
  await settle();
  assert.equal(app.el('#liked-count').textContent, 0);
  assert.match(app.el('#toast').textContent, /保存失败/);
});

test('a late search response cannot overwrite a newer query', async () => {
  const older = deferred();
  const newer = deferred();
  const app = harness({'Service.Search': query => query === 'first' ? older.promise : newer.promise});
  await settle();
  app.search('first');
  app.search('second');
  newer.resolve({tracks: [trackB]});
  await settle();
  older.resolve({tracks: [trackA]});
  await settle();
  const html = app.el('#main-content').innerHTML;
  assert.match(html, /second/);
  assert.match(html, /Track B/);
  assert.doesNotMatch(html, /Track A/);
});

test('untrusted track text is escaped and non-HTTPS image URLs are discarded', async () => {
  const app = harness({'Service.Search': () => ({tracks: [{...trackA, title: '<script>alert(1)</script>', thumbnail: 'javascript:alert(1)'}]})});
  await settle();
  app.search('<test>');
  await settle();
  const html = app.el('#main-content').innerHTML;
  assert.match(html, /&lt;script&gt;/);
  assert.match(html, /&lt;test&gt;/);
  assert.doesNotMatch(html, /<script>|javascript:/);
});

test('playback starts only on a user action, retains official controls, and responds to native shortcuts', async () => {
  const app = harness();
  await settle();
  app.click({action: 'play', id: trackA.id});
  await settle();
  assert.equal(app.players.length, 1);
  const player = app.players[0];
  assert.equal(player.options.videoId, trackA.id);
  assert.equal(player.options.playerVars.controls, 1);
  assert.equal(player.options.playerVars.origin, 'https://si.kyun.ytmgo');
  assert.equal(player.iframe.attributes.referrerpolicy, 'strict-origin-when-cross-origin');
  app.window.ytmgoControls('toggle');
  assert.deepEqual(player.calls.at(-1), ['pause']);
  app.window.ytmgoControls('next');
  await settle();
  assert.ok(player.calls.some(c => c[0] === 'load' && c[1] === trackB.id));
  assert.equal(app.el('#transport-title').textContent, trackB.title);
  assert.equal(app.el('#official-link').href, `https://music.youtube.com/watch?v=${trackB.id}`);
});

test('blocked playback can be retried or explicitly opened without forcing video into the listening view', async () => {
  const app = harness();
  await settle();
  app.click({action: 'play', id: trackA.id});
  await settle();
  app.players[0].options.events.onAutoplayBlocked();
  assert.equal(app.el('#player-notice').hidden, false);
  assert.equal(app.el('#video-stage').hidden, true);
  assert.match(app.el('#player-notice').innerHTML, /data-action="toggle"/);
  assert.match(app.el('#player-notice').innerHTML, /data-action="show-video"/);
  app.click({action: 'show-video'});
  assert.equal(app.el('#video-stage').hidden, false);
  app.click({action: 'toggle'});
  assert.equal(app.el('#player-notice').hidden, true);
  app.players[0].options.events.onError({data: 150});
  assert.match(app.el('#player-notice').innerHTML, /禁止.*嵌入/);
  assert.match(app.el('#player-notice').innerHTML, /retry-player/);
  assert.equal(app.el('#official-link').href, `https://music.youtube.com/watch?v=${trackA.id}`);
});

test('listening starts with artwork and optional video toggles preserve playback and seek position', async () => {
  const app = harness();
  await settle();
  assert.equal(app.el('#video-stage').hidden, true);
  assert.equal(app.el('#video-toggle').disabled, true);
  app.click({action: 'toggle-video'});
  assert.equal(app.players.length, 0);
  app.click({action: 'play', id: trackA.id});
  await settle();
  const player = app.players[0];
  player.currentTime = 82;
  player.options.events.onStateChange({data: 1});
  const calls = copy(player.calls);
  assert.equal(app.el('#now-artwork').hidden, false);
  assert.match(app.el('#now-artwork').innerHTML, /maxresdefault/);
  assert.equal(app.el('#video-stage').hidden, true);
  assert.equal(app.el('#player-notice').hidden, true);
  app.click({action: 'toggle-video'});
  assert.equal(app.el('#now-artwork').hidden, true);
  assert.equal(app.el('#video-stage').hidden, false);
  assert.equal(app.el('#video-stage').attributes['aria-hidden'], 'false');
  assert.equal(app.el('#video-toggle').attributes['aria-expanded'], 'true');
  app.click({action: 'toggle-video'});
  assert.equal(app.el('#now-artwork').hidden, false);
  assert.equal(app.el('#video-stage').hidden, true);
  assert.equal(app.el('#video-toggle').textContent, '显示视频');
  assert.deepEqual(player.calls, calls);
  assert.equal(player.currentTime, 82);
  assert.equal(app.players.length, 1);
});

test('cover mode follows track changes and missing artwork without reopening video', async () => {
  const app = harness();
  await settle();
  app.click({action: 'play', id: trackA.id});
  await settle();
  const player = app.players[0];
  // A player event must not reload an image that already used its fallback.
  app.el('#now-artwork').innerHTML = 'loaded fallback';
  player.options.events.onStateChange({data: 2});
  assert.equal(app.el('#now-artwork').innerHTML, 'loaded fallback');
  app.window.ytmgoControls('next');
  await settle();
  assert.match(app.el('#now-artwork').innerHTML, /cover-fallback/);
  assert.equal(app.el('#now-artwork').attributes['aria-label'], 'Track B · 歌曲封面');
  assert.equal(app.el('#video-stage').hidden, true);
  assert.equal(app.el('#now-artwork').hidden, false);
  assert.ok(player.calls.some(call => call[0] === 'load' && call[1] === trackB.id));
});

test('enqueueing before playback preserves the selected queue when using Play', async () => {
  const app = harness();
  await settle();
  app.click({action: 'enqueue', id: trackB.id});
  app.window.ytmgoControls('toggle');
  await settle();
  assert.equal(app.players[0].options.videoId, trackB.id);
  assert.equal(app.el('#drawer-count').textContent, 1);
});

test('shuffle without repeat exhausts a two-track queue instead of cycling forever', async () => {
  const app = harness();
  await settle();
  app.click({action: 'shuffle'});
  app.click({action: 'play', id: trackA.id});
  await settle();
  const player = app.players[0];
  player.options.events.onStateChange({data: 0});
  await settle();
  assert.deepEqual(player.calls.filter(c => c[0] === 'load'), [['load', trackB.id]]);
  player.options.events.onStateChange({data: 0});
  await settle();
  assert.deepEqual(player.calls.filter(c => c[0] === 'load'), [['load', trackB.id]]);
  assert.equal(app.el('#transport-title').textContent, trackB.title);
});

test('shuffle Previous follows listening history and Next returns along that history', async () => {
  const app = harness();
  await settle();
  app.click({action: 'shuffle'});
  app.click({action: 'play', id: trackA.id});
  await settle();
  app.window.ytmgoControls('next');
  await settle();
  app.window.ytmgoControls('previous');
  await settle();
  app.window.ytmgoControls('next');
  await settle();
  assert.deepEqual(app.players[0].calls.filter(c => c[0] === 'load'), [['load', trackB.id], ['load', trackA.id], ['load', trackB.id]]);
});

test('shuffle starts another cycle only when list repeat is enabled', async () => {
  const app = harness();
  await settle();
  app.click({action: 'shuffle'});
  app.click({action: 'repeat'});
  app.click({action: 'play', id: trackA.id});
  await settle();
  const player = app.players[0];
  player.options.events.onStateChange({data: 0});
  await settle();
  player.options.events.onStateChange({data: 0});
  await settle();
  assert.deepEqual(player.calls.filter(c => c[0] === 'load'), [['load', trackB.id], ['load', trackA.id]]);
});

test('album artwork uses 768px for hero/cards and 160px for compact uses while preserving stored URLs', async () => {
  const original = 'https://yt3.googleusercontent.com/opaque_A%2FB-120=w120-h120-l90-rj?x=a%2Fb&size=120&flag=1#view';
  const otherOriginal = 'https://lh3.ggpht.com/opaque_B=s120-c-k-c0x00ffffff-no-rj?x=1&y=2';
  const album = {...trackA, thumbnail: original};
  const other = {...trackB, thumbnail: otherOriginal};
  const app = harness({
    'Service.Home': () => ({sections: [{title: 'Albums', tracks: [album, other]}]}),
    'Service.Search': () => ({tracks: [album]}),
  });
  await settle();
  const mainImages = imagesIn(app.el('#main-content').innerHTML);
  assert.equal(mainImages[0].src, original.replace('=w120-h120', '=w768-h768'));
  assert.equal(mainImages[1].src, original.replace('=w120-h120', '=w768-h768'));
  assert.equal(mainImages[2].src, otherOriginal.replace('=s120', '=s768'));
  assert.equal(mainImages[0]['data-original-src'], original);
  app.click({action: 'play', id: album.id});
  await settle();
  assert.equal(imagesIn(app.el('#transport-cover').innerHTML)[0].src, original.replace('=w120-h120', '=w160-h160'));
  assert.equal(imagesIn(app.el('#queue-preview').innerHTML)[0].src, otherOriginal.replace('=s120', '=s160'));
  app.click({action: 'like', id: album.id});
  await settle();
  assert.equal(app.requests.find(r => r.method === 'Service.SaveLibrary').args[0].liked[0].thumbnail, original);
  app.search('album');
  await settle();
  assert.equal(imagesIn(app.el('#main-content').innerHTML)[0].src, original.replace('=w120-h120', '=w160-h160'));
});

test('existing local favorites upgrade at render time without rewriting the library', async () => {
  const original = 'https://googleusercontent.com/an_opaque_id=s120-rj?keep=%2B+%2F';
  const app = harness({'Service.Library': () => ({liked: [{...trackA, thumbnail: original}], playlists: []})});
  await settle();
  app.click({nav: 'liked'}, 'nav');
  assert.equal(imagesIn(app.el('#main-content').innerHTML)[0].src, original.replace('=s120', '=s160'));
  assert.equal(app.requests.filter(r => r.method === 'Service.SaveLibrary').length, 0);
});

test('opaque paths, lookalike hosts, and unknown video paths are never rewritten', async () => {
  const originals = [
    'https://lh3.googleusercontent.com/opaque-w120-h120-rj?x=1',
    'https://lh3.googleusercontent.com/opaque=w120-h120-rj/child',
    'https://lh3.googleusercontent.com/opaque%3Dw120-h120-rj',
    'https://googleusercontent.com.evil.test/id=w120-h120-rj',
    'https://evilgoogleusercontent.com/id=s120-c',
    'https://ggpht.com.evil.test/id=s120-c',
    'https://i.ytimg.com/other/abcdefghijk/hqdefault.jpg',
    'https://ytimg.com.evil.test/vi/abcdefghijk/hqdefault.jpg',
  ];
  const app = harness({'Service.Home': () => ({sections: [{title: 'Unmodified', tracks: originals.map((thumbnail, index) => ({...trackA, id: `x${String(index).padStart(10, '0')}`, thumbnail}))}]})});
  await settle();
  const rendered = imagesIn(app.el('#main-content').innerHTML);
  for (const original of originals) {
    const image = rendered.find(image => image['data-original-src'] === original);
    assert.ok(image);
    assert.equal(image.src, original);
    assert.deepEqual(JSON.parse(image['data-image-candidates']), []);
  }
});

test('a failed upgraded album cover retries the original exactly once before using the placeholder', async () => {
  const original = 'https://yt3.googleusercontent.com/opaque=w120-h120-l90-rj';
  const app = harness({'Service.Home': () => ({sections: [{title: 'Album', tracks: [{...trackA, thumbnail: original}]}]})});
  await settle();
  const image = new ImageElement(imagesIn(app.el('#main-content').innerHTML)[0]);
  app.imageEvent('error', image);
  assert.deepEqual(image.requests, [original.replace('=w120-h120', '=w768-h768'), original]);
  assert.notEqual(image.style.visibility, 'hidden');
  app.imageEvent('error', image);
  app.imageEvent('error', image);
  assert.equal(image.requests.length, 2);
  assert.equal(image.style.visibility, 'hidden');
});

test('video artwork falls back from maxres to JPEG sddefault to the original without loops', async () => {
  const original = 'https://i.ytimg.com/vi_webp/Qhd5_JeRQYI/hqdefault.webp?sqp=dimensions&rs=opaque';
  const album = {...trackA, id: 'Qhd5_JeRQYI', thumbnail: original};
  const app = harness({
    'Service.Home': () => ({sections: [{title: 'Video', tracks: [album]}]}),
    'Service.Search': () => ({tracks: [album]}),
  });
  await settle();
  const image = new ImageElement(imagesIn(app.el('#main-content').innerHTML)[0]);
  assert.equal(image.src, 'https://i.ytimg.com/vi/Qhd5_JeRQYI/maxresdefault.jpg');
  app.imageEvent('error', image);
  assert.equal(image.src, 'https://i.ytimg.com/vi/Qhd5_JeRQYI/sddefault.jpg');
  image.naturalWidth = 120;
  app.imageEvent('load', image);
  assert.equal(image.src, original);
  app.imageEvent('load', image); // A legitimate small original is the final fallback.
  assert.equal(image.requests.length, 3);
  assert.notEqual(image.style.visibility, 'hidden');
  app.imageEvent('error', image);
  app.imageEvent('error', image);
  assert.equal(image.requests.length, 3);
  assert.equal(image.style.visibility, 'hidden');
  app.search('video');
  await settle();
  const smallImage = imagesIn(app.el('#main-content').innerHTML)[0];
  assert.equal(smallImage.src, original);
  assert.deepEqual(JSON.parse(smallImage['data-image-candidates']), []);
});

test('clearing or removing the last queued song disables Next before any playback', async () => {
  const app = harness(); await settle();
  app.click({action: 'enqueue', id: trackA.id});
  assert.equal(app.el('#next-button').disabled, false);
  app.click({action: 'clear-queue'});
  assert.equal(app.el('#next-button').disabled, true);
  app.click({action: 'enqueue', id: trackA.id});
  app.click({action: 'remove-queue', index: '0'});
  assert.equal(app.el('#next-button').disabled, true);
});

test('zero volume can be unmuted in one action and restores the last audible volume', async () => {
  const app = harness(); await settle();
  app.el('#volume').value = '35'; app.el('#volume').emit('input');
  app.el('#volume').value = '0'; app.el('#volume').emit('input');
  assert.equal(app.el('#mute-button').attributes['aria-label'], '取消静音');
  app.click({action: 'mute'});
  assert.equal(app.el('#volume').value, 35);
  assert.equal(app.el('#mute-button').attributes['aria-label'], '静音');
});

test('a seek preview survives timeline polling until the user commits the change', async () => {
  const app = harness(); await settle(); app.click({action: 'play', id: trackA.id}); await settle();
  const player = app.players[0]; player.currentTime = 10;
  app.el('#progress').value = '90'; app.el('#progress').emit('input');
  player.options.events.onStateChange({data: 1});
  assert.equal(app.el('#elapsed-time').textContent, '1:30');
  assert.equal(app.el('#progress').value, '90');
  app.el('#progress').emit('change');
  assert.deepEqual(player.calls.at(-1), ['seek', 90]);
});

test('official player volume and mute controls are reflected in the custom transport', async () => {
  const app = harness(); await settle(); app.click({action: 'play', id: trackA.id}); await settle();
  const player = app.players[0]; player.getVolume = () => 24; player.isMuted = () => true;
  player.options.events.onStateChange({data: 1});
  assert.equal(app.el('#volume').value, 0);
  assert.equal(app.el('#mute-button').attributes['aria-pressed'], 'true');
  player.isMuted = () => false; player.options.events.onStateChange({data: 1});
  assert.equal(app.el('#volume').value, 24);
});

test('shuffle preview order is the actual next song order', async () => {
  const third = {...trackB, id: 'zyxwvutsrqp', title: 'Track C'};
  const app = harness({'Service.Home': () => ({sections:[{title:'Three',tracks:[trackA,trackB,third]}]})});
  await settle(); app.click({action:'shuffle'}); app.click({action:'play',id:trackA.id}); await settle();
  const preview = app.el('#queue-preview').innerHTML;
  const firstTitle = preview.match(/<strong[^>]*>([^<]+)<\/strong>/)[1];
  app.window.ytmgoControls('next'); await settle();
  assert.equal(app.el('#transport-title').textContent, firstTitle);
  assert.equal(app.el('#queue-count').textContent, '1 首');
});

test('valid existing long playlist names are never truncated by unrelated favorite saves', async () => {
  const name = '歌'.repeat(80);
  const app = harness({'Service.Library': () => ({liked:[],playlists:[{id:'long',name,tracks:[]}]})});
  await settle(); app.click({action:'like',id:trackA.id}); await settle();
  assert.equal(app.requests.find(r=>r.method==='Service.SaveLibrary').args[0].playlists[0].name,name);
});

test('search rejects blank or overlong input without silently truncating it', async () => {
  const app = harness(); await settle(); app.search(' '); app.search('界'.repeat(161)); await settle();
  assert.equal(app.requests.filter(r=>r.method==='Service.Search').length,0);
  assert.match(app.el('#toast').textContent,/160/);
  app.search('🎵'.repeat(160)); await settle();
  assert.equal(app.requests.find(r=>r.method==='Service.Search').args[0], '🎵'.repeat(160));
});

test('failed playlist creation keeps the dialog and the entered name available for retry', async () => {
  const app = harness({'Service.SaveLibrary':()=>Promise.reject(new Error('full'))});
  await settle(); app.click({action:'create-playlist'}); app.el('#playlist-name').value='Keep my input';
  app.el('#playlist-form').emit('submit'); await settle();
  assert.equal(app.el('#playlist-dialog').open,true);
  assert.equal(app.el('#playlist-name').value,'Keep my input');
  assert.equal(app.el('#playlist-submit').disabled,false);
  assert.match(app.el('#playlist-feedback').textContent,/输入已保留/);
  assert.doesNotMatch(app.el('#playlist-nav').innerHTML,/Keep my input/);
});

test('renaming a playlist preserves its identity and songs', async () => {
  const app = harness({'Service.Library':()=>({liked:[],playlists:[{id:'p1',name:'Old',tracks:[trackA]}]})});
  await settle(); app.click({playlist:'p1'},'playlist'); app.click({action:'rename-playlist'});
  assert.equal(app.el('#playlist-name').value,'Old');
  app.el('#playlist-name').value='New 🎵'; app.el('#playlist-form').emit('submit'); await settle();
  const saved = app.requests.find(r=>r.method==='Service.SaveLibrary').args[0].playlists[0];
  assert.equal(saved.id,'p1'); assert.equal(saved.name,'New 🎵'); assert.equal(saved.tracks[0].id,trackA.id);
  assert.equal(app.el('#playlist-dialog').open,false);
});

test('a failed playlist deletion stays in the confirmation dialog with the playlist retained', async () => {
  const app = harness({'Service.Library':()=>({liked:[],playlists:[{id:'p1',name:'Keep',tracks:[trackA]}]}),'Service.SaveLibrary':()=>Promise.reject(new Error('full'))});
  await settle(); app.click({playlist:'p1'},'playlist'); app.click({action:'delete-playlist'}); app.click({action:'confirm-delete'}); await settle();
  assert.equal(app.el('#delete-dialog').open,true);
  assert.match(app.el('#delete-feedback').textContent,/歌单已保留/);
  assert.match(app.el('#main-content').innerHTML,/Keep/);
});

test('a reused player that never starts reaches an actionable timeout', async () => {
  const app = harness(); await settle(); app.click({action:'play',id:trackA.id}); await settle();
  app.window.ytmgoControls('next'); await settle();
  app.timerCallbacks.at(-1)();
  assert.match(app.el('#player-notice').innerHTML,/连接超时/);
  assert.match(app.el('#player-notice').innerHTML,/retry-player/);
});

test('old player timing and events cannot replace the newly selected track state', async () => {
  const app = harness(); await settle(); app.click({action:'play',id:trackA.id}); await settle();
  const player = app.players[0]; player.getVideoUrl=()=>`https://www.youtube.com/watch?v=${trackA.id}`;
  player.currentTime=99;
  app.window.ytmgoControls('next'); await settle();
  player.options.events.onStateChange({data:1,target:player});
  assert.equal(app.el('#elapsed-time').textContent,'0:00');
  assert.equal(app.el('#progress').disabled,true);
  assert.match(app.el('#live-indicator').innerHTML,/正在连接/);
});

test('a late home request cannot replace a newer retry', async () => {
  const first=deferred(), second=deferred(); let count=0;
  const app=harness({'Service.Home':()=> ++count===1 ? first.promise : second.promise});
  await settle(); app.click({action:'retry-home'});
  second.resolve({sections:[{title:'New selection',tracks:[trackB]}]}); await settle();
  first.resolve({sections:[{title:'Old selection',tracks:[trackA]}]}); await settle();
  assert.match(app.el('#main-content').innerHTML,/New selection/);
  assert.doesNotMatch(app.el('#main-content').innerHTML,/Old selection/);
});

test('an older failed library retry cannot disable a later successful recovery', async () => {
  const older=deferred(); let count=0;
  const app=harness({'Service.Library':()=>{ count++; if(count===1)return Promise.reject(new Error('offline')); if(count===2)return older.promise; return {liked:[trackA],playlists:[]}; }});
  await settle(); app.click({action:'retry-library'}); app.click({action:'retry-library'}); await settle();
  older.reject(new Error('late offline')); await settle();
  app.click({action:'like',id:trackB.id}); await settle();
  assert.equal(app.el('#liked-count').textContent,2);
  assert.equal(app.requests.filter(r=>r.method==='Service.SaveLibrary').length,1);
});

test('a delayed player seek cannot flash the old elapsed time back into the transport', async () => {
  const app=harness(); await settle(); app.click({action:'play',id:trackA.id}); await settle();
  const player=app.players[0]; player.currentTime=10; player.seekTo=value=>player.calls.push(['seek',value]);
  app.el('#progress').value='90'; app.el('#progress').emit('input'); app.el('#progress').emit('change');
  assert.equal(app.el('#elapsed-time').textContent,'1:30');
  player.options.events.onStateChange({data:1});
  assert.equal(app.el('#progress').value,90);
  player.currentTime=91; player.options.events.onStateChange({data:1});
  assert.equal(app.el('#elapsed-time').textContent,'1:31');
});

test('direction key fallbacks control playback while preserving text editing and modal focus', async () => {
  const app=harness(); await settle(); app.click({action:'play',id:trackA.id}); await settle();
  app.keyEvent({key:'ArrowUp',metaKey:true}); assert.equal(app.el('#volume').value,75);
  app.keyEvent({key:'ArrowRight',metaKey:true}); await settle(); assert.equal(app.el('#transport-title').textContent,trackB.title);
  app.keyEvent({key:'ArrowUp',metaKey:true,target:{isContentEditable:true}}); assert.equal(app.el('#volume').value,75);
  app.click({action:'create-playlist'}); app.keyEvent({key:'ArrowUp',metaKey:true}); assert.equal(app.el('#volume').value,75);
});

test('single-song repeat restarts the current song without advancing the queue', async () => {
  const app=harness(); await settle(); app.click({action:'play',id:trackA.id}); await settle();
  app.click({action:'repeat'}); app.click({action:'repeat'});
  const player=app.players[0]; player.options.events.onStateChange({data:0}); await settle();
  assert.equal(app.el('#transport-title').textContent,trackA.title);
  assert.ok(player.calls.some(call=>call[0]==='seek'&&call[1]===0));
  assert.equal(player.calls.filter(call=>call[0]==='load').length,0);
});

test('list repeat wraps after the last song while clear waiting preserves the current song', async () => {
  const app=harness(); await settle(); app.click({action:'repeat'}); app.click({action:'play',id:trackB.id}); await settle();
  const player=app.players[0]; player.options.events.onStateChange({data:0}); await settle();
  assert.equal(app.el('#transport-title').textContent,trackA.title);
  app.click({action:'clear-queue'});
  assert.equal(app.el('#drawer-count').textContent,1);
  assert.equal(app.el('#queue-count').textContent,'0 首');
  assert.equal(app.el('#clear-queue').disabled,true);
  assert.match(app.el('#queue-full').innerHTML,/当前歌曲保留在队列中/);
});
