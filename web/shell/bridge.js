// bridge — the only module that knows the service's DOM. It reads playback
// and account state into the store and turns user intents into first-party
// actions (clicks on the service's own controls, its player API, or its SPA
// navigation event). It never reads cookies, storage or account identity.
const SEL = {
  app: 'ytmusic-app',
  layout: 'ytmusic-app-layout',
  content: 'ytmusic-app-layout > #content',
  bar: 'ytmusic-player-bar',
  title: 'ytmusic-player-bar .content-info-wrapper .title',
  byline: 'ytmusic-player-bar .content-info-wrapper .byline',
  barImage: 'ytmusic-player-bar .thumbnail-image-wrapper img',
  playPause: 'ytmusic-player-bar #play-pause-button',
  next: 'ytmusic-player-bar .next-button',
  previous: 'ytmusic-player-bar .previous-button',
  repeat: 'ytmusic-player-bar .repeat',
  shuffle: 'ytmusic-player-bar .shuffle',
  like: 'ytmusic-player-bar ytmusic-like-button-renderer',
  trackMenu: 'ytmusic-player-bar ytmusic-menu-renderer #button-shape button, ytmusic-player-bar ytmusic-menu-renderer button',
  player: '#movie_player',
  playerHost: 'ytmusic-player',
  video: '#movie_player video',
  searchInput: 'ytmusic-search-box input',
  signIn: 'ytmusic-nav-bar a[href*="accounts.google.com"]',
  avatar: 'ytmusic-nav-bar #avatar-btn',
  settings: 'ytmusic-nav-bar #right-content > yt-icon-button, ytmusic-nav-bar ytmusic-settings-button button',
  guideEntries: 'ytmusic-guide-renderer ytmusic-guide-entry-renderer',
  tabs: 'ytmusic-player-page #side-panel tp-yt-paper-tab',
  dialog: 'tp-yt-paper-dialog[opened]',
  heroArt: ':is(ytmusic-responsive-header-renderer, ytmusic-immersive-header-renderer, ytmusic-detail-header-renderer, ytmusic-visual-header-renderer) img',
};

// Primary destinations as first-party browse endpoints. Dispatching the
// service's own navigation event keeps the SPA (and playback) alive.
const DESTINATIONS = {
  home: {browseId: 'FEmusic_home', path: '/'},
  explore: {browseId: 'FEmusic_explore', path: '/explore'},
  library: {browseId: 'FEmusic_library_landing', path: '/library'},
};

const bridge = {
  routeOf(path) {
    if (path === '/') return 'home';
    if (path.startsWith('/explore')) return 'explore';
    if (path.startsWith('/library')) return 'library';
    if (path.startsWith('/search')) return 'search';
    if (path === '/watch' || path.startsWith('/podcast')) return 'watch';
    return 'browse';
  },

  navigate(name) {
    const target = DESTINATIONS[name];
    const host = $(SEL.layout) || $(SEL.app);
    if (!target) return false;
    if (!host) { location.assign(target.path); return true; }
    host.dispatchEvent(new CustomEvent('yt-navigate', {bubbles: true, composed: true, detail: {endpoint: {browseEndpoint: {browseId: target.browseId}}}}));
    return true;
  },

  player() { return $(SEL.player); },
  video() { return $(SEL.video) || $('video'); },
  call(name, ...args) {
    const player = this.player();
    return typeof player?.[name] === 'function' ? player[name](...args) : undefined;
  },
  click(selector) {
    const node = $(selector);
    if (!node) return false;
    node.click();
    return true;
  },

  // Service → store. Cheap: a handful of reads, called at most once per frame.
  readPlayback() {
    const layout = $(SEL.layout), bar = $(SEL.bar), player = this.player(), video = this.video();
    const uiState = layout?.getAttribute('player-ui-state') || 'INACTIVE';
    const meta = navigator.mediaSession?.metadata;
    const titleNode = $(SEL.title);
    const title = (titleNode?.title || titleNode?.textContent || meta?.title || '').trim();
    // The layout state lags on some entry paths and the service sometimes
    // re-creates its player bar; a loaded media source is authoritative.
    const active = uiState !== 'INACTIVE' || !!(video?.currentSrc || video?.src);
    const playerState = this.call('getPlayerState');
    const volume = this.call('getVolume');
    return {
      active,
      title,
      // Ads have no artist links; the sponsor name comes from media metadata.
      artists: player?.classList.contains('ad-showing') ? (meta?.artist ? [{name: meta.artist, linked: false}] : []) : this.readArtists(meta),
      art: this.readArt(meta),
      playing: !!video && !video.paused && !video.ended,
      buffering: playerState === 3,
      ad: !!player?.classList.contains('ad-showing'),
      // Songs play as art-track videos (ATV); only music videos (OMV) and ads get a 16:9 frame.
      video: !!player?.classList.contains('ad-showing') || $(SEL.playerHost)?.getAttribute('playback-mode') === 'OMV_PREFERRED',
      like: $(SEL.like)?.getAttribute('like-status') || 'INDIFFERENT',
      repeat: bar?.getAttribute('repeat-mode') || 'NONE',
      duration: this.call('getDuration') || video?.duration || 0,
      volume: Number.isFinite(volume) ? volume / 100 : (video?.volume ?? 1),
      muted: this.call('isMuted') ?? video?.muted ?? false,
    };
  },

  // Artist/album links keep their first-party targets; plain bylines fall
  // back to their first segment (the artist) without play counts.
  readArtists(meta) {
    const byline = $(SEL.byline);
    const links = byline ? $$('a', byline).filter(a => a.textContent.trim()) : [];
    if (links.length) return links.map(link => ({name: link.textContent.trim(), linked: true}));
    const text = (byline?.title || byline?.textContent || meta?.artist || '').split('•')[0].trim();
    return text ? [{name: text, linked: false}] : [];
  },
  // Links are resolved at click time: the service re-renders its byline.
  openArtist(name) {
    const link = $$('a', $(SEL.byline) || document.createElement('i')).find(a => a.textContent.trim() === name);
    if (!link) return false;
    link.click();
    return true;
  },

  readArt(meta) {
    const artwork = [...(meta?.artwork || [])];
    const largest = artwork.sort((a, b) => parseInt(b.sizes) - parseInt(a.sizes))[0]?.src;
    const fallback = $(SEL.barImage)?.src;
    const src = largest || (fallback && fallback !== location.origin + '/' ? fallback : '');
    // Square art is served at any size; request one that stays sharp at 2×.
    return src.replace(/=w\d+-h\d+/, '=w720-h720');
  },

  // Display state only, never identity. A sign-in control outranks an
  // avatar placeholder, and loading/unknown keeps sign-out hidden.
  readSignedIn() {
    return !isShown($(SEL.signIn)) && isShown($(SEL.avatar));
  },

  readPlaylists() {
    return $$(SEL.guideEntries)
      .filter(entry => !entry.hasAttribute('is-primary'))
      .map(entry => {
        const title = ($('.title', entry)?.textContent || '').trim();
        const subtitle = ($('.subtitle', entry)?.textContent || '').trim();
        // "Liked music" is recognised by its playlist id, never by its
        // (localized) title.
        const liked = entry.data?.navigationEndpoint?.browseEndpoint?.browseId === 'VLLM';
        return {title, subtitle, liked};
      })
      .filter(item => item.title);
  },

  openPlaylist(title) {
    const entry = $$(SEL.guideEntries).find(node => ($('.title', node)?.textContent || '').trim() === title);
    if (!entry) return false;
    ($('tp-yt-paper-item', entry) || entry).click();
    return true;
  },

  readHeroArt() {
    const img = $$(SEL.heroArt).find(node => node.src && isShown(node));
    return img ? img.src.replace(/=w\d+-h\d+/, '=w544-h544') : '';
  },

  // User intents → first-party controls.
  toggle() {
    if (this.click(SEL.playPause)) return true;
    const video = this.video();
    if (!video) return false;
    video.paused ? this.call('playVideo') : this.call('pauseVideo');
    return true;
  },
  // Prefer the service's buttons (they keep its queue logic); fall back to
  // the player API while the service is re-creating its bar.
  next() { return this.click(SEL.next) || this.call('nextVideo') !== undefined; },
  previous() { return this.click(SEL.previous) || this.call('previousVideo') !== undefined; },
  seek(seconds) {
    const duration = store.state.duration;
    const target = clamp(seconds, 0, duration || seconds);
    if (this.call('seekTo', target, true) === undefined && this.video()) this.video().currentTime = target;
    return true;
  },
  setVolume(value) {
    const level = clamp(value, 0, 1);
    const player = this.player();
    if (typeof player?.setVolume === 'function') {
      player.setVolume(Math.round(level * 100));
      if (level > 0 && player.isMuted?.()) player.unMute();
    } else if (this.video()) {
      this.video().volume = level;
    } else return false;
    store.set({volume: level, muted: level === 0 ? store.state.muted : false});
    return true;
  },
  toggleMute() {
    const player = this.player();
    if (typeof player?.isMuted === 'function') { player.isMuted() ? player.unMute() : player.mute(); }
    else if (this.video()) this.video().muted = !this.video().muted;
    else return false;
    store.set({muted: !store.state.muted});
    return true;
  },
  like(kind = 'like') {
    const host = $(SEL.like);
    const button = host && $(kind === 'like' ? '#button-shape-like button' : '#button-shape-dislike button', host);
    if (!button) return false;
    button.click();
    return true;
  },
  cycleRepeat() { return this.click(SEL.repeat); },
  shuffle() { return this.click(SEL.shuffle); },

  selectTab(index) {
    const tab = $$(SEL.tabs)[index];
    if (!tab || tab.hasAttribute('disabled')) return false;
    tab.click();
    return true;
  },
  tabState() {
    return $$(SEL.tabs).map(tab => ({
      label: tab.textContent.trim(),
      selected: tab.classList.contains('iron-selected') || tab.getAttribute('aria-selected') === 'true',
      disabled: tab.hasAttribute('disabled') || tab.getAttribute('aria-disabled') === 'true',
    }));
  },

  focusSearch() {
    const input = $(SEL.searchInput);
    if (!input) return false;
    $('ytmusic-search-box')?.setAttribute('opened', '');
    input.focus();
    input.select();
    return true;
  },
  account() {
    const signIn = $(SEL.signIn);
    if (signIn) { signIn.click(); return true; }
    return this.click(SEL.avatar);
  },
  serviceSettings(anchor) {
    const button = $(SEL.settings) || $(SEL.avatar);
    return button ? this.openServiceMenu(button, anchor) : false;
  },
  trackMenu(anchor) {
    const button = $(SEL.trackMenu);
    return button ? this.openServiceMenu(button, anchor) : false;
  },

  // The service anchors its dropdowns to the control that opened them. Ours
  // replace hidden service controls, so re-anchor the dropdown to our button.
  openServiceMenu(trigger, anchor) {
    trigger.click();
    if (!anchor) return true;
    let frames = 0;
    const attach = () => {
      const dropdown = $$('ytmusic-popup-container tp-yt-iron-dropdown').find(d => d.style.display !== 'none' && d.getAttribute('aria-hidden') !== 'true');
      if (!dropdown) { if (++frames < 30) requestAnimationFrame(attach); return; }
      dropdown.positionTarget = anchor;
      dropdown.noOverlap = true;
      dropdown.verticalAlign = anchor.getBoundingClientRect().top > innerHeight / 2 ? 'bottom' : 'top';
      dropdown.horizontalAlign = anchor.getBoundingClientRect().left > innerWidth / 2 ? 'right' : 'left';
      dropdown.refit?.();
    };
    requestAnimationFrame(attach);
    return true;
  },

  setAppearance(mode) { location.href = 'ytmgo://appearance/' + mode; },
  // The native side saves the choice and reloads the music view in it.
  setLanguage(choice) { location.href = 'ytmgo://language/' + choice; },
  reportAccount(signedIn) { location.href = 'ytmgo://account/' + (signedIn ? 'signed-in' : 'signed-out'); },
};
