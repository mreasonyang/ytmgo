// core — bundle guard and DOM primitives shared by every shell module.
// The shell runs only in the top-level music document. Sign-in pages and
// embedded frames keep the service's own UI and never see these globals.
if (window.top !== window || location.origin !== 'https://music.youtube.com' || window.ytmgoShell) return;
window.ytmgoShell = true;

const SVG_NS = 'http://www.w3.org/2000/svg';
const $ = (selector, scope = document) => scope.querySelector(selector);
const $$ = (selector, scope = document) => [...scope.querySelectorAll(selector)];
const clamp = (value, low, high) => Math.min(high, Math.max(low, value));
const reducedMotion = matchMedia('(prefers-reduced-motion: reduce)');

// h('button', {class: 'y-btn', onclick, 'aria-label': '…'}, child, …)
// false/null props are omitted; `text` sets textContent; on* binds listeners.
function h(tag, props = {}, ...children) {
  const node = document.createElement(tag);
  for (const [key, value] of Object.entries(props)) {
    if (value === false || value == null) continue;
    if (key === 'text') node.textContent = value;
    else if (key.startsWith('on') && typeof value === 'function') node.addEventListener(key.slice(2), value);
    else node.setAttribute(key, value === true ? '' : value);
  }
  for (const child of children.flat()) {
    if (child == null || child === false) continue;
    node.append(typeof child === 'string' ? document.createTextNode(child) : child);
  }
  return node;
}

// Writes only on change so MutationObserver-driven syncs stay idle.
function setText(node, text) { if (node && node.textContent !== text) node.textContent = text; }
function setAttr(node, name, value) {
  if (!node) return;
  if (value == null || value === false) { if (node.hasAttribute(name)) node.removeAttribute(name); return; }
  const next = value === true ? '' : String(value);
  if (node.getAttribute(name) !== next) node.setAttribute(name, next);
}
function setData(name, value) {
  const root = document.documentElement;
  if (value == null) delete root.dataset[name];
  else if (root.dataset[name] !== String(value)) root.dataset[name] = value;
}

function formatTime(seconds) {
  if (!Number.isFinite(seconds) || seconds < 0) return '0:00';
  const s = Math.floor(seconds), hours = Math.floor(s / 3600), minutes = Math.floor(s / 60) % 60;
  const ss = String(s % 60).padStart(2, '0');
  return hours ? `${hours}:${String(minutes).padStart(2, '0')}:${ss}` : `${minutes}:${ss}`;
}

function rafThrottle(fn) {
  let queued = false;
  return () => {
    if (queued) return;
    queued = true;
    requestAnimationFrame(() => { queued = false; fn(); });
  };
}

// Visible to the page only when it carries actual layout.
function isShown(node) {
  return !!node && !node.closest('[hidden], [aria-hidden="true"]') && node.getClientRects().length > 0;
}

// Screen-reader announcements for state that changes without focus (track,
// repeat mode). Created lazily so the module works before <body> exists.
let liveRegion = null;
function announce(message) {
  if (!document.body) return;
  if (!liveRegion) {
    liveRegion = localized(h('div', {id: 'ytmgo-live', class: 'y-sr', 'aria-live': 'polite', role: 'status'}));
    document.body.append(liveRegion);
  }
  liveRegion.textContent = '';
  requestAnimationFrame(() => { liveRegion.textContent = message; });
}

// Re-trigger a one-shot CSS animation class (press pops, heart bursts).
function pulse(node, className) {
  if (!node || reducedMotion.matches) return;
  node.classList.remove(className);
  void node.offsetWidth;
  node.classList.add(className);
  node.addEventListener('animationend', () => node.classList.remove(className), {once: true});
}
