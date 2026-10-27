// nowplaying — one surface, two shapes. Docked, it is the right-hand panel
// (artwork, track, then the service's Up next / Lyrics / Related tabs).
// Immersive, the same surface expands over the whole window on a blurred
// artwork backdrop. The service player page is laid into this frame by CSS,
// so the queue, lyrics and video stay first-party and keep working.
let userClosedPanel = false;

// Songs stream as art-track videos whose frame crops badly; for them the
// original square artwork is laid over the player (see nowplaying.css).
const cover = document.createElement('img');
cover.className = 'ytmgo-cover';
cover.alt = '';
cover.decoding = 'async';
cover.onload = () => cover.classList.add('is-loaded');
function attachCover() {
  const host = $(SEL.playerHost);
  if (host && cover.parentElement !== host) host.append(cover);
}

// Toggle semantics: the button for the visible tab closes the panel; any
// other tab button switches to its tab without closing.
function showPanel(tab, forceOpen = false) {
  const {panel, panelTab, immersive} = store.state;
  if (!forceOpen && panel && panelTab === tab && !immersive) {
    store.set({panel: false});
    userClosedPanel = true;
    return;
  }
  if (!immersive) store.set({panel: true});
  store.set({panelTab: tab});
  bridge.selectTab(tab);
}

function closePanel() {
  store.set({panel: false});
  userClosedPanel = true;
}

// Expanding and collapsing morph the artwork and title between their docked
// and immersive positions where the engine supports view transitions.
function setImmersive(on) {
  if (on === store.state.immersive || (on && !store.state.active)) return;
  const apply = () => store.set({immersive: on});
  if (document.startViewTransition && !reducedMotion.matches) document.startViewTransition(apply);
  else apply();
  if (on) requestAnimationFrame(() => $('#ytmgo-np .y-np-collapse')?.focus({preventScroll: true}));
}

function mountNowPlaying() {
  const backdrop = h('img', {class: 'y-np-backdrop', alt: '', 'aria-hidden': 'true', decoding: 'async'});
  const collapse = iconButton('chevron-down', t('shellExitImmersive'), {class: 'y-icon-btn y-round y-np-collapse', onclick: () => setImmersive(false)});
  const expand = iconButton('expand', t('shellImmersive'), {class: 'y-icon-btn y-round y-np-expand', onclick: () => setImmersive(true)});
  const close = iconButton('close', t('shellClosePanel'), {class: 'y-icon-btn y-round y-np-close', onclick: closePanel});
  const title = h('h2', {class: 'y-np-title'});
  const artists = h('div', {class: 'y-np-artists'});
  const like = iconButton('heart', t('shellLike'), {class: 'y-icon-btn y-like', 'aria-pressed': 'false'});
  like.addEventListener('click', () => { if (bridge.like('like')) pulse(like, 'is-pop'); });
  const more = iconButton('more', t('shellMore'));
  more.addEventListener('click', () => bridge.trackMenu(more));

  // Two fixed layers sandwich the service player page: the surface below it
  // (background, backdrop, empty state) and the controls above it.
  const surface = h('div', {id: 'ytmgo-np-surface'},
    h('div', {class: 'y-np-backdrop-wrap', 'aria-hidden': 'true'}, backdrop),
    h('div', {class: 'y-np-empty'},
      icon('note', 'y-icon y-np-empty-icon'),
      h('p', {class: 'y-np-empty-title', text: t('shellEmptyTitle')}),
      h('p', {class: 'y-np-empty-body', text: t('shellEmptyBody', {shortcut: shortcut(['mod', 'K'])})})));
  // Compact windows draw their own artwork (the service switches its player
  // page to a phone layout there); wider windows use the service player.
  const compactArt = h('img', {class: 'y-np-art', alt: '', decoding: 'async'});
  const np = h('aside', {id: 'ytmgo-np', 'aria-label': t('shellNowPlaying')},
    compactArt,
    h('header', {class: 'y-np-head'},
      collapse,
      h('span', {class: 'y-np-eyebrow', text: t('shellNowPlaying')}),
      h('div', {class: 'y-np-head-actions'}, expand, close)),
    h('div', {class: 'y-np-meta'},
      h('div', {class: 'y-np-text'}, title, artists),
      h('div', {class: 'y-np-meta-actions'}, like, more)));
  const scrim = h('div', {id: 'ytmgo-scrim', 'aria-hidden': 'true', onclick: () => (store.state.immersive ? setImmersive(false) : closePanel())});

  store.watch(['panel', 'immersive'], ({panel, immersive}) => {
    setData('ytmgoPanel', panel ? 'open' : 'closed');
    setData('ytmgoView', immersive ? 'immersive' : 'browse');
    np.inert = surface.inert = !(panel || immersive);
  });
  store.watch(['active'], ({active}) => {
    setData('ytmgoActive', String(active));
    // First playback of a session opens the panel where it can dock.
    if (active && !userClosedPanel && matchMedia('(min-width: 1181px)').matches) store.set({panel: true});
    if (!active && store.state.immersive) store.set({immersive: false});
  });
  store.watch(['title', 'artists'], ({title: name, artists: list}) => {
    setText(title, name);
    artists.replaceChildren(...artistLinks(list));
    pulse(np.querySelector('.y-np-meta'), 'is-entering');
  });
  store.watch(['art'], ({art}) => {
    cover.classList.remove('is-loaded');
    for (const img of [backdrop, cover, compactArt]) {
      if (art) img.src = art; else img.removeAttribute('src');
    }
  });
  store.watch(['video'], ({video}) => setData('ytmgoMedia', video ? 'video' : 'audio'));
  store.watch(['like'], ({like: status}) => {
    const liked = status === 'LIKE';
    swapIcon(like, liked ? 'heart-fill' : 'heart');
    setAttr(like, 'aria-pressed', String(liked));
    setAttr(like, 'aria-label', t(liked ? 'shellUnlike' : 'shellLike'));
  });

  // Crossing below the docking width collapses an open panel rather than
  // letting it suddenly cover the canvas; widening again restores it.
  const dock = matchMedia('(min-width: 1181px)');
  let reopenOnDock = false;
  dock.addEventListener('change', ({matches}) => {
    if (!matches && store.state.panel) { store.set({panel: false}); reopenOnDock = true; }
    else if (matches && reopenOnDock) { store.set({panel: true}); reopenOnDock = false; }
  });

  document.body.append(scrim, localized(surface), localized(np));
}
