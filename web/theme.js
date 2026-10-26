/* Apply colors before the stylesheet paints; native appearance also colors the title bar. */
(() => {
  'use strict';
  const root = document.documentElement;
  const native = location.hostname === 'si.kyun.ytmgo';
  const media = window.matchMedia('(prefers-color-scheme: dark)');
  const valid = value => ['system', 'light', 'dark'].includes(value);
  const storageKey = 'ytmgo.appearance';
  let preference = 'system';
  let selector;
  if (!native) {
    try {
      const saved = window.localStorage.getItem(storageKey);
      if (valid(saved)) preference = saved;
    } catch { /* The interface still works when browser storage is unavailable. */ }
  }

  function apply(value) {
    preference = valid(value) ? value : 'system';
    const theme = preference === 'system' ? (media.matches ? 'dark' : 'light') : preference;
    root.dataset.theme = theme;
    root.dataset.appearance = preference;
    document.querySelector('meta[name="theme-color"]').content = theme === 'dark' ? '#141416' : '#f8f7f5';
    if (selector) selector.value = preference;
  }

  async function nativeCall(method, ...args) {
    const started = Date.now();
    while (!window.mygo?.call && Date.now() - started < 2500) await new Promise(resolve => setTimeout(resolve, 25));
    if (!window.mygo?.call) throw new Error('Appearance service unavailable');
    let result = await window.mygo.call(`Service.${method}`, ...args);
    // MyGo may return a JSON encoded result on some platforms.
    if (typeof result === 'string' && !valid(result)) result = JSON.parse(result);
    if (result?.error) throw new Error('Appearance could not be saved');
    return result;
  }

  apply(preference);
  media.addEventListener('change', () => { if (preference === 'system') apply(preference); });
  window.addEventListener('storage', event => {
    if (!native && (event.key === storageKey || event.key === null)) apply(event.newValue);
  });
  document.addEventListener('DOMContentLoaded', async () => {
    selector = document.getElementById('appearance-select');
    selector.value = preference;
    if (native) {
      selector.disabled = true;
      try { apply(await nativeCall('Appearance')); }
      catch { /* Retry remains possible using the appearance selector. */ }
      finally { selector.disabled = false; }
    }
    selector.addEventListener('change', async () => {
      const next = selector.value;
      if (!valid(next)) { selector.value = preference; return; }
      selector.disabled = true;
      try {
        if (native) await nativeCall('SetAppearance', next);
        else window.localStorage.setItem(storageKey, next);
        apply(next);
      } catch {
        selector.value = preference;
        document.dispatchEvent(new CustomEvent('ytmgo:appearance-error'));
      } finally { selector.disabled = false; }
    });
  });
})();
