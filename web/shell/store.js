// store — one observable state object. Modules render from it; only the
// bridge (service → state) and user intents (UI → state) write to it.
function createStore(initial) {
  const state = {...initial};
  const watchers = new Set();
  return {
    state,
    set(patch) {
      const changed = Object.keys(patch).filter(key => !Object.is(state[key], patch[key]));
      if (!changed.length) return;
      for (const key of changed) state[key] = patch[key];
      // Isolate watchers: one failure must not leave the others stale, since
      // an unchanged key will never be re-announced.
      for (const watcher of [...watchers]) {
        try { watcher(changed); } catch (error) { console.error('ytmgo watcher', error); }
      }
    },
    // Runs immediately, then whenever any listed key changes.
    watch(keys, fn) {
      const watcher = changed => { if (changed.some(key => keys.includes(key))) fn(state); };
      watchers.add(watcher);
      fn(state);
      return () => watchers.delete(watcher);
    },
  };
}

const store = createStore({
  appearance: 'system',   // stored preference: system | light | dark
  route: 'home',          // home | explore | library | search | browse
  signedIn: false,
  playlists: [],          // [{title, subtitle, entry}] from the service guide
  scrolled: false,        // canvas scrolled past the top bar
  wash: '',               // artwork tinting the top of the canvas
  // Playback
  active: false, playing: false, buffering: false, ad: false, video: false,
  title: '', artists: [], art: '', like: 'INDIFFERENT', repeat: 'NONE',
  duration: 0, volume: 1, muted: false,
  // Layout intents
  panel: false,           // now-playing panel open
  panelTab: 0,            // 0 up next · 1 lyrics · 2 related
  immersive: false,
  sidebarCollapsed: false,
});
