// overlays — popover, dialog and toast primitives plus the two surfaces
// built on them: Settings (progressive disclosure for everything that is
// not playback) and the keyboard shortcut sheet.
let activeLayer = null;

function closeLayer({restoreFocus = true} = {}) {
  const layer = activeLayer;
  if (!layer) return false;
  activeLayer = null;
  layer.node.classList.add('is-leaving');
  setTimeout(() => layer.node.remove(), reducedMotion.matches ? 0 : 180);
  layer.anchor?.setAttribute('aria-expanded', 'false');
  document.removeEventListener('pointerdown', layer.outside, true);
  if (restoreFocus) layer.anchor?.focus({preventScroll: true});
  return true;
}

function openLayer(node, anchor) {
  closeLayer({restoreFocus: false});
  const outside = event => {
    if (!node.contains(event.target) && !anchor?.contains(event.target)) closeLayer({restoreFocus: false});
  };
  activeLayer = {node, anchor, outside};
  anchor?.setAttribute('aria-expanded', 'true');
  document.body.append(node);
  document.addEventListener('pointerdown', outside, true);
  // Keep Tab inside the layer; Escape is handled by the keyboard module.
  node.addEventListener('keydown', event => {
    if (event.key !== 'Tab') return;
    const items = $$('button:not([disabled]), a[href], [tabindex="0"]', node).filter(isShown);
    if (!items.length) return;
    const first = items[0], last = items.at(-1);
    if (event.shiftKey && document.activeElement === first) { event.preventDefault(); last.focus(); }
    else if (!event.shiftKey && document.activeElement === last) { event.preventDefault(); first.focus(); }
  });
  requestAnimationFrame(() => ($('[aria-checked="true"], button, a[href]', node))?.focus({preventScroll: true}));
}

function popover(anchor, label, ...children) {
  const node = localized(h('div', {class: 'y-popover', role: 'dialog', 'aria-label': label}, ...children));
  const rect = anchor.getBoundingClientRect();
  node.style.top = `${rect.bottom + 8}px`;
  node.style.right = `${Math.max(8, innerWidth - rect.right)}px`;
  openLayer(node, anchor);
  return node;
}

function menuItem(iconName, label, props = {}, hint = '') {
  const tag = props.href ? 'a' : 'button';
  return h(tag, {class: 'y-menu-item', ...(tag === 'button' ? {type: 'button'} : {}), ...props},
    icon(iconName), h('span', {class: 'y-menu-label', text: label}), hint ? h('kbd', {text: hint}) : null);
}

// A WAI-ARIA radio group: arrow keys move and select, one tab stop.
function radioGroup({label, className, options, current, onSelect}) {
  let value = current;
  const buttons = options.map(option => h('button', {
    type: 'button', role: 'radio', class: option.className, 'data-value': option.value, lang: option.lang,
    onclick: () => { value = option.value; sync(); onSelect(option.value); },
  }, option.icon ? icon(option.icon) : null, h('span', {class: 'y-option-label', text: option.label}), option.hint ? h('span', {class: 'y-option-hint', text: option.hint}) : null));
  const group = h('div', {class: className, role: 'radiogroup', 'aria-label': label}, ...buttons);
  function sync() {
    buttons.forEach(button => {
      const checked = button.dataset.value === value;
      setAttr(button, 'aria-checked', String(checked));
      button.tabIndex = checked ? 0 : -1;
    });
  }
  group.addEventListener('keydown', event => {
    const step = {ArrowRight: 1, ArrowDown: 1, ArrowLeft: -1, ArrowUp: -1}[event.key];
    if (!step) return;
    event.preventDefault();
    const index = (buttons.findIndex(b => b.dataset.value === value) + step + buttons.length) % buttons.length;
    buttons[index].click();
    buttons[index].focus();
  });
  sync();
  return group;
}

function appearancePicker() {
  return radioGroup({
    label: t('shellAppearance'), className: 'y-segmented', current: store.state.appearance,
    options: [['dark', 'shellThemeDark', 'moon'], ['light', 'shellThemeLight', 'sun'], ['system', 'shellThemeSystem', 'monitor']]
      .map(([value, key, glyph]) => ({value, label: t(key), icon: glyph, className: 'y-segment'})),
    onSelect: value => { bridge.setAppearance(value); applyAppearance(value); },
  });
}

// Languages are listed by their own names (autonyms from CLDR), each tagged
// with its language so its Han glyphs render in the right regional form.
function languagePicker() {
  const resolved = I18N.languages.find(item => item.tag === LOCALE);
  return radioGroup({
    label: t('shellLanguage'), className: 'y-language-list', current: I18N.choice,
    options: [
      {value: 'system', label: t('shellLanguageSystem'), hint: resolved?.name, className: 'y-language'},
      ...I18N.languages.map(({tag, name}) => ({value: tag, label: name, lang: tag, className: 'y-language'})),
    ],
    onSelect: value => { if (value !== I18N.choice) bridge.setLanguage(value); },
  });
}

function openSettings(anchor) {
  if (activeLayer?.anchor === anchor) { closeLayer(); return; }
  const signedIn = store.state.signedIn;
  popover(anchor, t('shellSettings'),
    h('div', {class: 'y-popover-section'}, h('p', {class: 'y-eyebrow', text: t('shellAppearance')}), appearancePicker()),
    h('div', {class: 'y-popover-section'}, h('p', {class: 'y-eyebrow', text: t('shellLanguage')}), languagePicker(),
      h('p', {class: 'y-popover-note', text: t('shellLanguageNote')})),
    h('div', {class: 'y-popover-section y-menu'},
      menuItem('user', t(signedIn ? 'shellAccountManage' : 'shellAccountSignIn'), {onclick: () => { closeLayer({restoreFocus: false}); bridge.account(); }}),
      menuItem('gear', t('shellServiceSettings'), {onclick: () => { closeLayer({restoreFocus: false}); bridge.serviceSettings(anchor); }}),
      menuItem('keyboard', t('shellShortcuts'), {onclick: () => { closeLayer({restoreFocus: false}); openShortcuts(); }}, shortcut(['mod', '/']))),
    signedIn ? h('div', {class: 'y-popover-section y-menu'},
      // A real link: the native side accepts sign-out only from a user click.
      menuItem('logout', t('shellSignOut'), {href: 'ytmgo://session/signout', class: 'y-menu-item is-danger'})) : null,
    h('p', {class: 'y-popover-foot', text: t('shellFooter')}));
}

// Rows are message keys plus key-cap tokens (see shortcut() in i18n.js).
const SHORTCUTS = [
  ['shellShortcutGroupPlayback', [
    ['shellShortcutPlayPause', [['mod', 'P']]], ['shellShortcutNextPrevious', [['mod', '→'], ['mod', '←']]],
    ['shellShortcutVolume', [['mod', '↑'], ['mod', '↓']]], ['shellShortcutMute', [['mod', 'shift', 'M']]],
    ['shellShortcutGlobal', [['mod', 'shift', 'space']]]]],
  ['shellShortcutGroupInterface', [
    ['shellShortcutSearch', [['mod', 'K']]], ['shellShortcutPanel', [['mod', 'J']]], ['shellShortcutImmersive', [['mod', 'shift', 'F']]],
    ['shellShortcutSidebar', [['mod', 'B']]], ['shellShortcutDismiss', [['esc']]]]],
  ['shellShortcutGroupNavigation', [
    ['shellShortcutHome', [['mod', 'shift', 'H']]], ['shellShortcutLibrary', [['mod', 'L']]], ['shellShortcutHistory', [['mod', '['], ['mod', ']']]]]],
];

function openShortcuts() {
  const close = iconButton('close', t('shellClose'), {class: 'y-icon-btn y-round', onclick: () => closeLayer()});
  const node = localized(h('div', {class: 'y-dialog-wrap'},
    h('div', {class: 'y-dialog', role: 'dialog', 'aria-modal': 'true', 'aria-labelledby': 'ytmgo-shortcuts-title'},
      h('header', {class: 'y-dialog-head'}, h('h2', {id: 'ytmgo-shortcuts-title', text: t('shellShortcuts')}), close),
      h('div', {class: 'y-shortcut-grid'}, ...SHORTCUTS.map(([group, rows]) => h('section', {},
        h('p', {class: 'y-eyebrow', text: t(group)}),
        h('dl', {}, ...rows.flatMap(([label, combos]) => [h('dt', {text: t(label)}), h('dd', {}, h('kbd', {text: shortcut(...combos)}))]))))))));
  node.addEventListener('pointerdown', event => { if (event.target === node) closeLayer(); });
  openLayer(node, document.activeElement instanceof HTMLElement ? document.activeElement : null);
}

let toastTimer = 0;
function toast(message) {
  let node = $('#ytmgo-toast');
  if (!node) { node = localized(h('div', {id: 'ytmgo-toast', role: 'status', 'aria-live': 'polite'})); document.body.append(node); }
  node.textContent = message;
  node.classList.remove('is-visible');
  void node.offsetWidth;
  node.classList.add('is-visible');
  clearTimeout(toastTimer);
  toastTimer = setTimeout(() => node.classList.remove('is-visible'), 2200);
}
