import test from 'node:test';
import assert from 'node:assert/strict';
import {readFileSync} from 'node:fs';
import vm from 'node:vm';

const source = readFileSync(new URL('./theme.js', import.meta.url), 'utf8');
function harness({dark = false, saved = null, native = false, nativeSaved = 'system', fail = false} = {}) {
  const root = {dataset: {}};
  const meta = {};
  const select = {value: '', disabled: false, addEventListener(name, fn) { this[name] = fn; }};
  const documentEvents = {};
  const windowEvents = {};
  const writes = [];
  const errors = [];
  const calls = [];
  const media = {matches: dark, addEventListener(_name, fn) { this.change = fn; }};
  const window = {
    matchMedia: () => media,
    localStorage: {getItem: () => saved, setItem(key, value) { if (fail) throw Error('Storage unavailable'); writes.push({key, value}); }},
    addEventListener(name, fn) { windowEvents[name] = fn; },
    ...(native ? {mygo: {async call(method, value) {
      calls.push({method, value});
      if (method === 'Service.Appearance') return nativeSaved;
      if (fail) throw Error('Save failed');
      media.matches = value === 'dark';
      media.change();
      return value;
    }}} : {}),
  };
  const document = {documentElement: root, querySelector: () => meta, getElementById: () => select,
    addEventListener(name, fn) { documentEvents[name] = fn; }, dispatchEvent(event) { errors.push(event.type); }};
  vm.runInNewContext(source, {window, document, location: {hostname: native ? 'si.kyun.ytmgo' : '127.0.0.1'}, CustomEvent: class {constructor(type) {this.type = type;}}, setTimeout});
  return {root, meta, select, media, writes, errors, calls,
    ready: () => documentEvents.DOMContentLoaded(),
    choose: async value => { select.value = value; await select.change(); },
    system: dark => { media.matches = dark; media.change(); },
    storage: value => windowEvents.storage({key: 'ytmgo.appearance', newValue: value})};
}

test('first paint follows system and explicit preferences survive browser reload', async () => {
  const system = harness({dark: true});
  assert.equal(system.root.dataset.theme, 'dark');
  system.system(false);
  assert.equal(system.root.dataset.theme, 'light');
  await system.ready();
  await system.choose('light');
  assert.deepEqual(system.writes, [{key: 'ytmgo.appearance', value: 'light'}]);
  const reload = harness({dark: true, saved: system.writes[0].value});
  assert.equal(reload.root.dataset.theme, 'light');
  assert.equal(reload.meta.content, '#f8f7f5');
  reload.system(true);
  assert.equal(reload.root.dataset.theme, 'light');
});

test('returning to system resumes automatic updates and invalid saved data falls back', async () => {
  const app = harness({saved: 'invalid', dark: true});
  await app.ready();
  assert.equal(app.select.value, 'system');
  await app.choose('light');
  app.system(true);
  assert.equal(app.root.dataset.theme, 'light');
  await app.choose('system');
  assert.equal(app.root.dataset.theme, 'dark');
  app.system(false);
  assert.equal(app.root.dataset.theme, 'light');
  app.storage('dark');
  assert.equal(app.root.dataset.theme, 'dark');
});

test('native selection restores and saves via the native service without browser storage', async () => {
  const app = harness({native: true, nativeSaved: 'dark', dark: true});
  await app.ready();
  assert.equal(app.select.value, 'dark');
  await app.choose('light');
  assert.equal(app.root.dataset.theme, 'light');
  assert.equal(app.select.value, 'light');
  assert.equal(app.writes.length, 0);
  assert.deepEqual(app.calls.map(c => c.method), ['Service.Appearance', 'Service.SetAppearance']);
  assert.equal(app.calls[1].value, 'light');
  assert.equal(app.select.disabled, false);
});

test('failed preference saves keep the previous selection and allow retry', async () => {
  for (const native of [false, true]) {
    const app = harness({native, nativeSaved: 'dark', saved: 'dark', dark: true, fail: true});
    await app.ready();
    await app.choose('light');
    assert.equal(app.root.dataset.theme, 'dark');
    assert.equal(app.select.value, 'dark');
    assert.equal(app.select.disabled, false);
    assert.deepEqual(app.errors, ['ytmgo:appearance-error']);
  }
});
