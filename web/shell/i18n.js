// i18n — the shell's only source of user-visible text. music.go injects
// I18N = {locale, script, choice, languages, messages} from locales/*.json
// in the language the app shows; messages keep plain {name} slots.
const LOCALE = I18N.locale;

function t(key, params = {}) {
  const message = I18N.messages[key];
  if (message == null) return key;
  return message.replace(/\{(\w+)\}/g, (slot, name) => (name in params ? String(params[name]) : slot));
}

// Shortcut glyphs follow the platform: ⌘ ⇧ on macOS, localized key caps
// elsewhere. Single characters (K, /, arrows) are key caps, not text.
const MAC = /Mac/.test(navigator.platform);
const KEY_CAPS = {
  mod: () => (MAC ? '⌘' : t('shellKeyCtrl')),
  shift: () => (MAC ? '⇧' : t('shellKeyShift')),
  space: () => t('shellKeySpace'),
  esc: () => t('shellKeyEsc'),
};
function shortcut(...combos) {
  return combos.map(combo => combo.map(key => KEY_CAPS[key]?.() ?? key).join(' ')).join(' / ');
}

// Locale-aware joins and numbers: each language its own list separators
// and conjunction, and its own percent formatting.
const listFormat = (() => {
  try { return new Intl.ListFormat(LOCALE, {type: 'conjunction'}); } catch { return {format: items => items.join(', ')}; }
})();
const percentFormat = (() => {
  try { return new Intl.NumberFormat(LOCALE, {style: 'percent', maximumFractionDigits: 0}); } catch { return {format: v => Math.round(v * 100) + '%'}; }
})();

function greetingText(hour = new Date().getHours()) {
  return t(hour < 5 ? 'shellGreetingNight' : hour < 11 ? 'shellGreetingMorning' : hour < 13 ? 'shellGreetingNoon' : hour < 18 ? 'shellGreetingAfternoon' : 'shellGreetingEvening');
}

// Shell roots declare their language: screen readers pronounce it, and
// the browser picks the right Han glyphs (SC/TC/JP) for it.
function localized(node) {
  node.lang = LOCALE;
  return node;
}
