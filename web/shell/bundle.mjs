// Node mirror of musicPreload() in music.go, used by tests and UI tooling.
import {readFileSync} from 'node:fs';

const dir = new URL('./', import.meta.url);
const read = name => readFileSync(new URL(name, dir), 'utf8');

export const manifest = JSON.parse(read('bundle.json'));

const catalog = tag => JSON.parse(readFileSync(new URL(`../../locales/${tag}.json`, import.meta.url), 'utf8'));
export const languages = ['en', 'ja', 'zh-Hans', 'zh-Hant'];
const scripts = {en: 'latn', ja: 'jpan', 'zh-Hans': 'hans', 'zh-Hant': 'hant'};

// Mirror of newShellI18N() in i18n.go. Shell messages are plain strings
// with {name} slots, so no MessageFormat engine is needed here. `pseudo`
// expands English ~40% with accented letters to test text expansion.
export function shellI18N(locale = 'en', {choice = 'system', pseudo = false} = {}) {
  const source = catalog('en'), own = catalog(locale);
  const messages = {};
  for (const key of Object.keys(source).filter(k => k.startsWith('shell'))) {
    messages[key] = pseudo ? pseudoLocalize(source[key]) : own[key] ?? source[key];
  }
  const names = tag => new Intl.DisplayNames([tag], {type: 'language'}).of(tag);
  return {locale, script: scripts[locale] ?? 'latn', choice, languages: languages.map(tag => ({tag, name: names(tag)})), messages};
}

const ACCENTS = {a: 'á', e: 'é', i: 'í', o: 'ö', u: 'ü', c: 'ç', n: 'ñ', A: 'Å', E: 'É', O: 'Ö', s: 'š', y: 'ý'};
function pseudoLocalize(text) {
  const body = text.replace(/\{\w+\}|./gs, part => (part.length > 1 ? part : ACCENTS[part] ?? part));
  const pad = '·'.repeat(Math.ceil(text.length * 0.4));
  return `[${body} ${pad}]`;
}

export function buildPreload(appearance = 'system', i18n = shellI18N('en')) {
  const styles = manifest.styles.map(name => read(name) + '\n').join('');
  const scripts = manifest.scripts.map(name => `/* ${name} */\n${read(name)}\n`).join('');
  return `(function ytmgoShell(STYLES, APPEARANCE, I18N) {\n'use strict';\n${scripts}\n})(${JSON.stringify(styles)}, ${JSON.stringify(appearance)}, ${JSON.stringify(i18n)});\n`;
}

// Tests only: the same bundle with its internals published on window, so
// node:test can drive the store, bridge and actions directly. Never used
// by the app (music.go has no equivalent).
export function buildTestPreload(appearance = 'system', i18n = shellI18N('en')) {
  const preload = buildPreload(appearance, i18n);
  const end = preload.lastIndexOf('\n})(');
  return preload.slice(0, end) + '\nwindow.__ytmgoTest = {store, bridge, runAction, formatTime, applyAppearance, setImmersive, showPanel, closePanel, dismissTopmost, SEL, t, shortcut, greetingText};' + preload.slice(end);
}
