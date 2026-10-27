// sidebar — primary destinations plus the user's playlists. Collapses to an
// icon rail (user toggle on wide windows, automatic on medium ones).
const NAV_ITEMS = [
  {name: 'home', labelKey: 'shellNavHome', icon: 'home'},
  {name: 'explore', labelKey: 'shellNavExplore', icon: 'explore'},
  {name: 'library', labelKey: 'shellNavLibrary', icon: 'library'},
];

// Built node by node: the service enforces Trusted Types (no HTML strings).
function brandMark() {
  const svg = document.createElementNS(SVG_NS, 'svg');
  svg.setAttribute('viewBox', '0 0 32 32');
  svg.setAttribute('aria-hidden', 'true');
  svg.setAttribute('class', 'y-brand-mark');
  const shapes = [
    ['rect', {width: 32, height: 32, rx: 9, fill: 'var(--y-accent)'}],
    ['path', {d: 'M12.5 10.2v11.6a.9.9 0 0 0 1.37.77l9.3-5.8a.9.9 0 0 0 0-1.54l-9.3-5.8a.9.9 0 0 0-1.37.77Z', fill: 'var(--y-on-accent)'}],
    ['path', {d: 'M8 13v6', stroke: 'var(--y-on-accent)', 'stroke-width': 2.2, 'stroke-linecap': 'round'}],
  ];
  for (const [tag, attributes] of shapes) {
    const shape = document.createElementNS(SVG_NS, tag);
    for (const [key, value] of Object.entries(attributes)) shape.setAttribute(key, value);
    svg.append(shape);
  }
  return svg;
}

// Deterministic two-stop gradient per playlist name: stable, distinct, and
// never a broken image while the service has not delivered artwork.
function monogram(title, liked) {
  let hash = 0;
  for (const char of title) hash = (hash * 31 + char.codePointAt(0)) >>> 0;
  const hue = liked ? 262 : hash % 360;
  const tile = h('span', {class: 'y-tile', 'aria-hidden': 'true', style: `--h:${hue}`});
  if (liked) tile.append(icon('heart-fill'));
  else tile.textContent = [...title.trim()][0]?.toUpperCase() || '♪';
  return tile;
}

function mountSidebar() {
  const navButtons = NAV_ITEMS.map(item => h('button', {
    type: 'button', class: 'y-nav-item', 'data-route': item.name, 'data-tip': t(item.labelKey),
    onclick: () => bridge.navigate(item.name),
  }, icon(item.icon, 'y-icon y-icon-off'), icon(item.icon + '-fill', 'y-icon y-icon-on'), h('span', {class: 'y-label', text: t(item.labelKey)})));

  const collapse = iconButton('sidebar', t('shellSidebarCollapse'), {class: 'y-icon-btn y-side-toggle', onclick: () => store.set({sidebarCollapsed: !store.state.sidebarCollapsed})});
  const list = h('div', {class: 'y-library-list', role: 'list'});
  const signInButton = h('button', {type: 'button', class: 'y-btn y-btn-inverse', onclick: () => bridge.account()}, t('shellSignIn'));
  const empty = h('div', {class: 'y-library-empty'},
    h('p', {class: 'y-library-empty-title', text: t('shellLibraryEmptyTitle')}),
    h('p', {class: 'y-library-empty-body', text: t('shellLibraryEmptyBody')}),
    signInButton);

  const aside = localized(h('aside', {id: 'ytmgo-sidebar', 'aria-label': t('shellNavMain')},
    h('nav', {class: 'y-side-card y-side-nav', 'aria-label': t('shellNavMain')},
      h('div', {class: 'y-brand'},
        h('button', {type: 'button', class: 'y-brand-link', 'aria-label': t('shellBrandHome'), onclick: () => bridge.navigate('home')}, brandMark(), h('span', {class: 'y-brand-word y-label', text: t('shellBrandName')})),
        collapse),
      ...navButtons),
    h('section', {class: 'y-side-card y-library', 'aria-label': t('shellLibraryRegion')},
      h('div', {class: 'y-library-head'},
        h('button', {type: 'button', class: 'y-library-title', 'data-tip': t('shellLibraryTitle'), onclick: () => bridge.navigate('library')},
          icon('library'), h('span', {class: 'y-label', text: t('shellLibraryTitle')}))),
      list, empty)));

  store.watch(['route'], ({route}) => {
    for (const button of navButtons) setAttr(button, 'aria-current', button.dataset.route === route ? 'page' : null);
  });
  store.watch(['sidebarCollapsed'], ({sidebarCollapsed}) => {
    setData('ytmgoSidebar', sidebarCollapsed ? 'collapsed' : 'expanded');
    setAttr(collapse, 'aria-label', t(sidebarCollapsed ? 'shellSidebarExpand' : 'shellSidebarCollapse'));
    setAttr(collapse, 'data-tip', t(sidebarCollapsed ? 'shellSidebarExpand' : 'shellSidebarCollapse'));
    setAttr(collapse, 'aria-expanded', String(!sidebarCollapsed));
  });

  let rendered = '';
  store.watch(['playlists', 'signedIn'], ({playlists, signedIn}) => {
    empty.hidden = signedIn || playlists.length > 0;
    list.hidden = !empty.hidden;
    const key = playlists.map(p => p.title + '\u0000' + p.subtitle + p.liked).join('\u0001');
    if (key === rendered) return;
    rendered = key;
    list.replaceChildren(...playlists.map(({title, subtitle, liked}) => h('button', {
      type: 'button', role: 'listitem', class: 'y-lib-item', 'data-tip': title,
      onclick: () => bridge.openPlaylist(title),
    }, monogram(title, liked), h('span', {class: 'y-lib-text y-label'}, h('span', {class: 'y-lib-title', text: title}), subtitle ? h('span', {class: 'y-lib-sub', text: subtitle}) : null))));
  });

  document.body.append(aside);
}

// Compact windows: the four destinations move to a bottom tab bar, inside
// the thumb zone and directly under the mini player.
function mountTabbar() {
  const items = [...NAV_ITEMS.slice(0, 1), {name: 'search', labelKey: 'shellNavSearch', icon: 'search'}, ...NAV_ITEMS.slice(1)];
  const buttons = items.map(item => h('button', {
    type: 'button', class: 'y-tab', 'data-route': item.name,
    onclick: () => (item.name === 'search' ? bridge.focusSearch() : bridge.navigate(item.name)),
  }, icon(item.icon, 'y-icon y-icon-off'), icon(ICONS[item.icon + '-fill'] ? item.icon + '-fill' : item.icon, 'y-icon y-icon-on'), h('span', {class: 'y-tab-label', text: t(item.labelKey)})));
  const bar = localized(h('nav', {id: 'ytmgo-tabbar', 'aria-label': t('shellNavMain')}, ...buttons));
  store.watch(['route'], ({route}) => {
    for (const button of buttons) setAttr(button, 'aria-current', button.dataset.route === route ? 'page' : null);
  });
  document.body.append(bar);
}
