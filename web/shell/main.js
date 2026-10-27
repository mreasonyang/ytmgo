// main — boot, appearance, the native control surface and the sync loop.
const colorScheme = matchMedia('(prefers-color-scheme: dark)');

function applyAppearance(value) {
  if (!['light', 'dark', 'system'].includes(value)) return false;
  store.set({appearance: value});
  setData('ytmgoTheme', value === 'system' ? (colorScheme.matches ? 'dark' : 'light') : value);
  return true;
}

const TOGGLES = new Set(['toggle', 'panel', 'queue', 'lyrics', 'immersive', 'sidebar', 'shortcuts', 'like', 'mute']);
let lastToggle = {action: '', at: 0};

// The fixed vocabulary shared with the native menu, tray and global shortcut.
function runAction(action) {
  if (TOGGLES.has(action)) {
    const now = performance.now();
    if (lastToggle.action === action && now - lastToggle.at < 250) return true;
    lastToggle = {action, at: now};
  }
  switch (action) {
    case 'toggle': return bridge.toggle();
    case 'next': return bridge.next();
    case 'previous': return bridge.previous();
    case 'mute': return bridge.toggleMute();
    case 'volume-up': return bridge.setVolume(store.state.volume + .1);
    case 'volume-down': return bridge.setVolume(store.state.volume - .1);
    case 'like': return bridge.like('like');
    case 'search': return bridge.focusSearch();
    case 'home': case 'explore': case 'library': return bridge.navigate(action);
    case 'back': history.back(); return true;
    case 'forward': history.forward(); return true;
    case 'account': return bridge.account();
    case 'panel': store.state.panel ? closePanel() : showPanel(store.state.panelTab, true); return true;
    case 'queue': showPanel(0); return true;
    case 'lyrics': showPanel(1); return true;
    case 'immersive': setImmersive(!store.state.immersive); return true;
    case 'sidebar': store.set({sidebarCollapsed: !store.state.sidebarCollapsed}); return true;
    case 'shortcuts': if (activeLayer) closeLayer(); else openShortcuts(); return true;
    default: return false;
  }
}

// The canvas is the service's own content column turned into a scroll
// container. Scroll position is remembered per entry for Back/Forward.
const scrollMemory = new Map();
let content = null, lastHref = location.href, poppedState = false;

function bindContent() {
  const node = $(SEL.content);
  if (!node || node === content) return;
  content = node;
  node.setAttribute('role', 'main');
  node.setAttribute('aria-label', t('shellContent'));
  node.tabIndex = -1;
  node.prepend(localized(h('header', {id: 'ytmgo-greeting'}, h('h1', {text: greetingText()}))));
  node.addEventListener('scroll', rafThrottle(() => {
    scrollMemory.set(location.href, node.scrollTop);
    store.set({scrolled: node.scrollTop > 4});
    document.documentElement.style.setProperty('--y-scroll', Math.min(node.scrollTop, 480).toFixed(0));
  }), {passive: true});
}

// Called from sync() when the URL changes, however the service navigated.
function onNavigate() {
  const wasWatch = new URL(lastHref).pathname === '/watch';
  lastHref = location.href;
  // Opening/closing the player page does not change the canvas underneath.
  if (content && location.pathname !== '/watch' && !wasWatch) {
    const target = poppedState ? scrollMemory.get(location.href) || 0 : 0;
    // Restored pages re-render progressively; retry until they are tall enough.
    let frames = 0;
    const restore = () => {
      content.scrollTop = target;
      if (content.scrollTop < target - 1 && ++frames < 45) requestAnimationFrame(restore);
    };
    restore();
  }
  poppedState = false;
  refreshHistory();
  setText($('#ytmgo-greeting h1'), greetingText());
}

let boundVideo = null;
function bindVideo(schedule) {
  const video = bridge.video();
  if (!video || video === boundVideo) return;
  boundVideo = video;
  for (const type of ['play', 'pause', 'playing', 'waiting', 'durationchange', 'volumechange', 'seeked', 'ended', 'loadedmetadata']) {
    video.addEventListener(type, () => { schedule(); kickProgress(); });
  }
}

const sameList = (a, b, key) => a.length === b.length && a.every((item, i) => key(item) === key(b[i]));
let mounted = false, reportedAccount = null;

function mountShell() {
  mounted = true;
  const wash = h('img', {class: 'y-canvas-wash', alt: '', decoding: 'async'});
  document.body.append(h('div', {id: 'ytmgo-canvas', 'aria-hidden': 'true'}, wash));
  mountSidebar();
  mountTopbar();
  mountNowPlaying();
  mountPlayerbar();
  mountTabbar();
  store.watch(['wash'], ({wash: src}) => {
    if (!src) { wash.classList.remove('is-loaded'); return; }
    if (wash.getAttribute('src') === src) return;
    wash.classList.remove('is-loaded');
    wash.onload = () => wash.classList.add('is-loaded');
    wash.src = src;
  });
}

function sync(schedule) {
  if (!document.body || !$(SEL.app)) return;
  if (!mounted) mountShell();
  bindContent();
  if (location.href !== lastHref) onNavigate();
  bindVideo(schedule);
  attachCover();

  const playback = bridge.readPlayback();
  if (sameList(playback.artists, store.state.artists, a => a.name + a.linked)) delete playback.artists;
  store.set(playback);

  const signedIn = bridge.readSignedIn();
  store.set({signedIn});
  if (signedIn !== reportedAccount) { reportedAccount = signedIn; bridge.reportAccount(signedIn); }

  const playlists = bridge.readPlaylists();
  if (!sameList(playlists, store.state.playlists, p => p.title + '\u0000' + p.subtitle + p.liked)) store.set({playlists});

  const route = bridge.routeOf(location.pathname);
  if (route !== 'watch') store.set({route});
  setData('ytmgoRoute', route === 'watch' ? store.state.route : route);

  const selected = bridge.tabState().findIndex(tab => tab.selected);
  if (selected >= 0) store.set({panelTab: selected});

  store.set({wash: bridge.readHeroArt() || store.state.art});

  const search = $(SEL.searchInput);
  if (search && search.placeholder !== t('shellSearchPlaceholder')) search.placeholder = t('shellSearchPlaceholder');
  const signIn = $(SEL.signIn);
  if (signIn && signIn.textContent.trim() !== t('shellSignIn')) signIn.textContent = t('shellSignIn');
}

function boot() {
  const root = document.documentElement;
  applyAppearance(APPEARANCE);
  setData('ytmgoScript', I18N.script);
  colorScheme.addEventListener('change', () => applyAppearance(store.state.appearance));
  const style = document.createElement('style');
  style.id = 'ytmgo-shell-style';
  style.textContent = STYLES;
  (document.head || root).append(style);

  window.ytmgoSetAppearance = applyAppearance;
  window.ytmgoControls = runAction;
  mountKeyboard();

  const schedule = rafThrottle(() => sync(schedule));
  new MutationObserver(schedule).observe(root, {
    childList: true, subtree: true, attributes: true,
    attributeFilter: ['hidden', 'aria-hidden', 'class', 'like-status', 'repeat-mode', 'player-ui-state', 'video-mode', 'inert', 'aria-selected', 'disabled', 'src', 'title'],
  });
  addEventListener('yt-navigate-finish', schedule);
  addEventListener('popstate', () => { poppedState = true; });
  document.addEventListener('DOMContentLoaded', schedule, {once: true});
  schedule();
}

// Preloads run at document start, possibly before <html> exists.
if (document.documentElement) boot();
else {
  const waiting = new MutationObserver(() => {
    if (!document.documentElement) return;
    waiting.disconnect();
    boot();
  });
  waiting.observe(document, {childList: true});
}
