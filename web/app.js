/* ytmgo: a small, dependency-free interface backed by MyGo and YouTube's player. */
(() => {
  'use strict';

  const paths = {
    home: '<path d="m3 10 9-7 9 7v10a1 1 0 0 1-1 1h-5v-7H9v7H4a1 1 0 0 1-1-1Z"/>',
    compass: '<circle cx="12" cy="12" r="9"/><path d="m16 8-2.5 5.5L8 16l2.5-5.5Z"/>',
    heart: '<path d="M20.8 4.7a5.5 5.5 0 0 0-7.8 0L12 5.8l-1.1-1.1a5.5 5.5 0 0 0-7.8 7.8L12 21l8.8-8.5a5.5 5.5 0 0 0 0-7.8Z"/>',
    plus: '<path d="M12 5v14M5 12h14"/>',
    search: '<circle cx="10.5" cy="10.5" r="6.5"/><path d="m16 16 5 5"/>',
    'arrow-right': '<path d="M4 12h16m-6-6 6 6-6 6"/>',
    music: '<path d="M9 18V5l11-2v13M9 8l11-2"/><ellipse cx="5.5" cy="18.5" rx="3.5" ry="2.5"/><ellipse cx="16.5" cy="16.5" rx="3.5" ry="2.5"/>',
    youtube: '<rect x="2" y="5" width="20" height="14" rx="4"/><path d="m10 9 5 3-5 3Z" fill="currentColor" stroke="none"/>',
    shield: '<path d="m12 3 8 3v6c0 5-8 9-8 9s-8-4-8-9V6Z"/><path d="m8 12 3 3 5-6"/>',
    info: '<circle cx="12" cy="12" r="9"/><path d="M12 11v6M12 7v.1"/>',
    external: '<path d="M14 3h7v7m0-7-11 11M10 3H5a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h14a2 2 0 0 0 2-2v-5"/>',
    queue: '<path d="M3 5h15M3 10h15M3 15h9M3 20h9m5-5 5 3-5 3Z"/>',
    shuffle: '<path d="M3 6h3c5 0 7 12 12 12h3m-4-4 4 4-4 4M3 18h3c1.7 0 3.1-1.3 4.4-3.1M14 8.7c1.2-1.6 2.4-2.7 4-2.7h3m-4-4 4 4-4 4"/>',
    previous: '<path d="M6 5v14m13-14L8 12l11 7Z" fill="currentColor"/>',
    next: '<path d="M18 5v14M5 5l11 7-11 7Z" fill="currentColor"/>',
    play: '<path d="m8 4 13 8-13 8Z" fill="currentColor" stroke="none"/>',
    pause: '<path d="M7 5h3v14H7zm7 0h3v14h-3z" fill="currentColor" stroke="none"/>',
    repeat: '<path d="m17 2 4 4-4 4M3 11V8a2 2 0 0 1 2-2h16M7 22l-4-4 4-4m14-1v3a2 2 0 0 1-2 2H3"/>',
    volume: '<path d="m11 4-6 5H2v6h3l6 5ZM15 8a6 6 0 0 1 0 8m3-11a10 10 0 0 1 0 14"/>',
    muted: '<path d="m11 4-6 5H2v6h3l6 5Zm5 5 6 6m0-6-6 6"/>',
    close: '<path d="m6 6 12 12M6 18 18 6"/>',
    playlist: '<path d="M3 5h13M3 10h13M3 15h7m6-2v8m-4-4h8"/>',
    check: '<path d="m5 12 4 4L19 6"/>',
    retry: '<path d="M20 7v5h-5m4.7 0a8 8 0 1 0-1.2 5M20 7l-3-3"/>',
    moon: '<path d="M20.5 13a9 9 0 0 1-9.5-9.5A9 9 0 1 0 20.5 13Z"/>',
    sun: '<circle cx="12" cy="12" r="4"/><path d="M12 1v2m0 18v2M1 12h2m18 0h2M4 4l2 2m12 12 2 2M4 20l2-2M18 6l2-2"/>',
    headphones: '<path d="M3 14v-3a9 9 0 0 1 18 0v3M3 12h3v8H3a1 1 0 0 1-1-1v-6a1 1 0 0 1 1-1Zm18 0h-3v8h3a1 1 0 0 0 1-1v-6a1 1 0 0 0-1-1Z"/>',
    bolt: '<path d="m13 2-9 12h7l-1 8L21 9h-8Z"/>',
    album: '<rect x="3" y="3" width="18" height="18" rx="3"/><circle cx="12" cy="12" r="5"/><circle cx="12" cy="12" r="1"/>',
  };
  const icon = (name, filled = false) => `<svg viewBox="0 0 24 24" fill="${filled ? 'currentColor' : 'none'}" stroke="currentColor" stroke-width="1.65" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">${paths[name] || paths.music}</svg>`;
  const $ = (selector) => document.querySelector(selector);
  const esc = (value) => String(value ?? '').replace(/[&<>"']/g, (c) => ({'&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;'}[c]));
  const clone = (value) => JSON.parse(JSON.stringify(value));
  const validID = (id) => typeof id === 'string' && /^[a-zA-Z0-9_-]{11}$/.test(id);
  const time = (seconds) => {
    const value = Math.max(0, Math.floor(Number(seconds) || 0));
    const hours = Math.floor(value / 3600);
    return hours ? `${hours}:${String(Math.floor(value / 60) % 60).padStart(2, '0')}:${String(value % 60).padStart(2, '0')}` : `${Math.floor(value / 60)}:${String(value % 60).padStart(2, '0')}`;
  };
  const safeImage = (value) => {
    try { const url = new URL(value); return url.protocol === 'https:' ? url.href : ''; } catch { return ''; }
  };
  const imageSources = (original, size) => {
    const candidates = [original];
    let video = false;
    try {
      const url = new URL(original);
      const isHost = domain => url.hostname === domain || url.hostname.endsWith(`.${domain}`);
      if (isHost('googleusercontent.com') || isHost('ggpht.com')) {
        // Only the documented terminal size modifier is editable. The opaque
        // resource identifier, crop flags, query string, and fragment stay intact.
        const suffix = /=((?:w[1-9]\d*-h[1-9]\d*)|(?:s[1-9]\d*))(?=(?:-[A-Za-z0-9]+)*$)/;
        const match = url.pathname.match(suffix);
        if (match) {
          url.pathname = url.pathname.replace(suffix, match[1].startsWith('s') ? `=s${size}` : `=w${size}-h${size}`);
          candidates.unshift(url.href);
        }
      } else if (size > 160 && isHost('ytimg.com')) {
        const match = url.pathname.match(/^\/vi(?:_webp)?\/([A-Za-z0-9_-]{11})\/(?:hqdefault|mqdefault|default|sddefault|maxresdefault)\.(?:jpg|webp)$/);
        if (match) {
          video = true;
          url.search = '';
          url.pathname = `/vi/${match[1]}/sddefault.jpg`;
          candidates.unshift(url.href);
          url.pathname = `/vi/${match[1]}/maxresdefault.jpg`;
          candidates.unshift(url.href);
        }
      }
    } catch { /* Unknown thumbnail formats retain the service-provided URL. */ }
    return {candidates: [...new Set(candidates)], video};
  };
  const track = (value) => validID(value?.id) ? {id: value.id, title: String(value.title || '未命名歌曲'), artist: String(value.artist || ''), album: String(value.album || ''), duration: typeof value.duration === 'number' ? time(value.duration) : String(value.duration || ''), thumbnail: safeImage(value.thumbnail)} : null;
  const tracks = (value) => Array.isArray(value) ? value.map(track).filter(Boolean) : [];
  const normalizeLibrary = (value) => ({liked: tracks(value?.liked), playlists: Array.isArray(value?.playlists) ? value.playlists.filter(p => p && typeof p.id === 'string' && typeof p.name === 'string').map(p => ({id: p.id, name: p.name, tracks: tracks(p.tracks)})) : []});
  const state = {
    tab: 'home', query: '', playlistID: '', home: [], homeLoading: true, homeError: false, homeRevision: 0, libraryLoadRevision: 0,
    searchResults: [], searchLoading: false, searchError: false, searchRevision: 0,
    library: {liked: [], playlists: []}, confirmedLibrary: {liked: [], playlists: []}, libraryLoaded: false, libraryError: false, libraryRevision: 0, saveTail: Promise.resolve(),
    allTracks: new Map(), queue: [], queueIndex: -1, activeTrack: null, queueOpen: false, videoVisible: false,
    shuffleRemaining: [], shuffleHistory: [], shuffleCursor: -1,
    player: null, playerReady: false, playerStatus: 'idle', playerError: '', desiredID: '', duration: 0, elapsed: 0, seeking: false, seekTarget: null, seekAt: 0, volume: 70, audibleVolume: 70, muted: false, shuffle: false, repeat: 'off',
    dialogTrack: null, editingPlaylistID: '', dialogSaving: false, dialogReturnFocus: null, queueReturnFocus: null, deletePlaylistID: '', deleting: false, toastTimer: null, playerCreationTimer: null, loadRevision: 0,
  };
  let youtubePromise = null;
  const register = (items) => { items.forEach(item => state.allTracks.set(item.id, item)); return items; };
  const isLiked = (id) => state.library.liked.some(item => item.id === id);
  const metadata = (item) => [item.artist, item.album].filter(Boolean).join(' · ');
  const cover = (item, className = '', lazy = true, size = 768) => {
    if (!item.thumbnail) return `<span class="${className} cover-fallback" aria-hidden="true">${icon('music')}</span>`;
    const {candidates, video} = imageSources(item.thumbnail, size);
    return `<img class="${className}" src="${esc(candidates[0])}" data-original-src="${esc(item.thumbnail)}" data-image-candidates="${esc(JSON.stringify(candidates.slice(1)))}"${video ? ' data-video-cover="true"' : ''} alt="" ${lazy ? 'loading="lazy"' : ''} referrerpolicy="no-referrer">`;
  };
  const nextImageSource = (image) => {
    let candidates = [];
    try { const value = JSON.parse(image.dataset.imageCandidates || '[]'); if (Array.isArray(value)) candidates = value.map(safeImage).filter(Boolean); } catch { /* Invalid candidate metadata falls back to the existing placeholder. */ }
    const next = candidates.shift();
    image.dataset.imageCandidates = JSON.stringify(candidates);
    if (next) { image.src = next; return; }
    image.style.visibility = 'hidden';
    image.parentElement?.classList.add('broken-cover');
  };
  const button = (action, name, label, attrs = '') => `<button class="icon-button${name === 'heart' && attrs.includes('data-liked="true"') ? ' active' : ''}" data-action="${action}" aria-label="${esc(label)}" title="${esc(label)}" ${attrs}>${icon(name, name === 'heart' && attrs.includes('data-liked="true"'))}</button>`;

  function focusKey(element) {
    const key = Object.fromEntries(['action', 'id', 'index', 'playlist', 'nav'].filter(key => element?.dataset?.[key] !== undefined).map(key => [key, element.dataset[key]]));
    if (element?.id) key.elementID = element.id;
    return key;
  }
  function restoreFocus(root, key) {
    const target = Object.keys(key || {}).length && Array.from(root.querySelectorAll?.('button') || []).find(el => !el.disabled && Object.entries(key).every(([name, value]) => (name === 'elementID' ? el.id : el.dataset[name]) === value) && (!el.getClientRects || el.getClientRects().length));
    (target || $('#main-content')).focus({preventScroll: true});
  }
  function rememberFocus(root) {
    const key = root.contains?.(document.activeElement) ? focusKey(document.activeElement) : null;
    return () => { if (key) restoreFocus(root, key); };
  }

  async function api(method, argument) {
    if (location.hostname === 'si.kyun.ytmgo' && !window.mygo?.call) {
      const start = Date.now();
      while (!window.mygo?.call && Date.now() - start < 2500) await new Promise(resolve => setTimeout(resolve, 25));
    }
    let response;
    if (window.mygo?.call) {
      response = await window.mygo.call(`Service.${method}`, ...(argument === undefined ? [] : [argument]));
    } else {
      const route = {Home: '/api/home', Search: `/api/search?q=${encodeURIComponent(argument || '')}`, Library: '/api/library', SaveLibrary: '/api/library'}[method];
      const controller = new AbortController();
      const timeout = setTimeout(() => controller.abort(), 30000);
      try {
        const result = await fetch(route, {method: method === 'SaveLibrary' ? 'PUT' : 'GET', credentials: 'same-origin', signal: controller.signal, ...(method === 'SaveLibrary' ? {headers: {'Content-Type': 'application/json', 'X-Ytmgo-Client': '1'}, body: JSON.stringify(argument)} : {})});
        if (!result.ok) throw new Error(`Request failed (${result.status})`);
        response = await result.json();
      } finally { clearTimeout(timeout); }
    }
    if (typeof response === 'string') response = JSON.parse(response);
    if (response?.error) throw new Error('Service unavailable');
    return response;
  }

  function toast(message) {
    clearTimeout(state.toastTimer);
    $('#toast').textContent = message;
    $('#toast').hidden = false;
    state.toastTimer = setTimeout(() => { $('#toast').hidden = true; }, 3600);
  }

  async function loadHome() {
    const revision = ++state.homeRevision;
    state.homeLoading = true; state.homeError = false;
    renderMain();
    try {
      const result = await api('Home');
      if (revision !== state.homeRevision) return;
      state.home = Array.isArray(result?.sections) ? result.sections.map(section => ({title: String(section.title || '发现好音乐'), subtitle: String(section.subtitle || ''), tracks: register(tracks(section.tracks))})).filter(section => section.tracks.length) : [];
    } catch { if (revision !== state.homeRevision) return; state.homeError = true; }
    state.homeLoading = false;
    if (state.tab === 'home' || state.tab === 'discover') renderMain();
  }

  async function loadLibrary() {
    const revision = ++state.libraryLoadRevision;
    state.libraryError = false;
    try {
      const result = await api('Library');
      if (revision !== state.libraryLoadRevision) return;
      if (!result || !Array.isArray(result.liked) || !Array.isArray(result.playlists)) throw new Error('Invalid library');
      state.library = normalizeLibrary(result);
      state.confirmedLibrary = clone(state.library);
      state.libraryLoaded = true;
      register(state.library.liked);
      state.library.playlists.forEach(p => register(p.tracks));
    } catch { if (revision !== state.libraryLoadRevision) return; state.libraryError = true; state.libraryLoaded = false; }
    renderLibraryNav(); renderMain(); renderNow();
  }

  function requireLibrary() {
    if (state.libraryLoaded) return true;
    toast(state.libraryError ? '本地音乐库读取失败，请在「我的收藏」中重试。' : '音乐库正在加载，请稍后再试。');
    return false;
  }

  function saveLibrary(next, successMessage) {
    if (!requireLibrary()) return;
    state.library = normalizeLibrary(next);
    const snapshot = clone(state.library);
    const revision = ++state.libraryRevision;
    renderLibraryNav(); renderMain(); renderNow();
    state.saveTail = state.saveTail.catch(() => {}).then(async () => {
      try {
        const saved = await api('SaveLibrary', snapshot);
        if (!saved || !Array.isArray(saved.liked) || !Array.isArray(saved.playlists)) throw new Error('Invalid saved library');
        state.confirmedLibrary = normalizeLibrary(saved);
        if (revision === state.libraryRevision) {
          state.library = clone(state.confirmedLibrary);
          renderLibraryNav(); renderMain(); renderNow();
          if (successMessage) toast(successMessage);
        }
        return true;
      } catch {
        if (revision === state.libraryRevision) {
          state.library = clone(state.confirmedLibrary);
          renderLibraryNav(); renderMain(); renderNow();
        }
        toast('保存失败，改动未保存到本机。请稍后重试。');
        return false;
      }
    });
    return state.saveTail;
  }

  function toggleLike(item) {
    if (!item || !requireLibrary()) return;
    const next = clone(state.library);
    const exists = next.liked.some(t => t.id === item.id);
    next.liked = exists ? next.liked.filter(t => t.id !== item.id) : [item, ...next.liked];
    saveLibrary(next, exists ? '已从收藏移除' : '已收藏到本机');
  }

  function renderLibraryNav() {
    const restore = rememberFocus($('#playlist-nav'));
    $('#liked-count').textContent = state.libraryLoaded ? state.library.liked.length : '—';
    $('#playlist-nav').innerHTML = state.library.playlists.length ? state.library.playlists.map(p => `<button class="playlist-item${state.tab === 'playlist' && state.playlistID === p.id ? ' active' : ''}" data-playlist="${esc(p.id)}" title="${esc(p.name)}" aria-label="打开歌单 ${esc(p.name)}">${icon('album')}<span>${esc(p.name)}</span></button>`).join('') : `<p class="sidebar-empty">${state.libraryError ? '音乐库暂时无法读取<br>可在「我的收藏」重试。' : '把喜欢的声音<br>留在自己的歌单里。'}</p>`;
    document.querySelectorAll('[data-nav]').forEach(el => {
      const active = el.dataset.nav === state.tab;
      el.classList.toggle('active', active);
      if (active) el.setAttribute('aria-current', 'page'); else el.removeAttribute('aria-current');
    });
    restore();
  }

  const heading = (eyebrow, title, description = '', extra = '') => `<div class="page-heading"><div><span class="eyebrow">${esc(eyebrow)}</span><h1>${esc(title)}</h1>${description ? `<p>${esc(description)}</p>` : ''}</div>${extra}</div>`;
  const skeleton = (list = false) => list ? `<div aria-label="正在加载音乐" aria-busy="true">${Array.from({length: 6}, () => '<div class="skeleton skeleton-row"></div>').join('')}</div>` : `<div aria-label="正在加载推荐音乐" aria-busy="true"><div class="skeleton skeleton-hero"></div><div class="skeleton skeleton-heading"></div><div class="card-grid">${Array.from({length: 6}, () => '<div><div class="skeleton skeleton-art"></div><div class="skeleton skeleton-title"></div><div class="skeleton skeleton-artist"></div></div>').join('')}</div></div>`;
  const empty = (symbol, title, description, action = '', actionLabel = '') => `<div class="empty-state"><span class="empty-icon">${icon(symbol)}</span><h2>${esc(title)}</h2><p>${esc(description)}</p>${action ? `<button class="secondary-button" data-action="${action}">${icon(action.includes('retry') ? 'retry' : 'search')}${esc(actionLabel)}</button>` : ''}</div>`;
  const equalizer = '<span class="equalizer" aria-label="正在播放"><i></i><i></i><i></i></span>';

  function renderCard(item) {
    const liked = isLiked(item.id);
    return `<article class="music-card${state.activeTrack?.id === item.id ? ' current' : ''}"><button class="card-art" data-action="play" data-id="${item.id}" aria-label="播放 ${esc(item.title)}">${cover(item)}<span class="card-play">${icon('play')}</span>${item.duration ? `<span class="card-duration">${esc(item.duration)}</span>` : ''}</button><button class="card-title" data-action="play" data-id="${item.id}" title="${esc(item.title)}">${esc(item.title)}</button><p class="card-artist" title="${esc(metadata(item))}">${esc(item.artist || item.album || 'YouTube Music')}</p><button class="icon-button card-action card-queue" data-action="enqueue" data-id="${item.id}" aria-label="将 ${esc(item.title)} 加入播放队列" title="加入播放队列">${icon('plus')}</button><button class="icon-button card-action${liked ? ' active' : ''}" data-action="like" data-id="${item.id}" aria-label="${liked ? '取消收藏' : '收藏'} ${esc(item.title)}" title="${liked ? '取消收藏' : '收藏'}">${icon('heart', liked)}</button></article>`;
  }

  function renderSections(sections) {
    return sections.map((section, index) => `<section class="music-section"><div class="section-heading"><div><h2>${esc(section.title)}</h2>${section.subtitle ? `<p>${esc(section.subtitle)}</p>` : ''}</div><span class="section-index">0${index + 1} / SELECTION</span></div><div class="card-grid">${section.tracks.slice(0, 8).map(renderCard).join('')}</div></section>`).join('');
  }

  function trackRows(items) {
    return `<div class="track-list"><div class="track-list-header"><span>#</span><span>歌曲 / 歌手</span><span>时长</span><span></span></div>${items.map((item, index) => {
      const current = state.activeTrack?.id === item.id;
      const liked = isLiked(item.id);
      return `<div class="track-row${current ? ' current' : ''}" data-track="${item.id}"><button class="track-number" data-action="play" data-id="${item.id}" data-number="${String(index + 1).padStart(2, '0')}" aria-label="播放 ${esc(item.title)}">${current && state.playerStatus === 'playing' ? equalizer : String(index + 1).padStart(2, '0')}</button><button class="row-track-button" data-action="play" data-id="${item.id}" aria-label="播放 ${esc(item.title)}">${cover(item, 'row-art', true, 160)}<span class="row-meta"><strong title="${esc(item.title)}">${esc(item.title)}</strong><small>${esc(metadata(item) || 'YouTube Music')}</small></span></button><span class="track-duration">${esc(item.duration || '—')}</span><div class="row-actions">${button('like', 'heart', `${liked ? '取消收藏' : '收藏'} ${item.title}`, `data-id="${item.id}" data-liked="${liked}"`)}${button('enqueue', 'plus', '加入播放队列', `data-id="${item.id}"`)}${button(state.tab === 'playlist' ? 'remove-playlist-track' : 'add-playlist', state.tab === 'playlist' ? 'close' : 'playlist', state.tab === 'playlist' ? '从此歌单移除' : '加入本地歌单', `data-id="${item.id}"`)}</div></div>`;
    }).join('')}</div>`;
  }

  function renderMain() {
    const main = $('#main-content');
    const restore = rememberFocus(main);
    const scrollTop = main.scrollTop;
    let html = '';
    if (state.tab === 'home') {
      html = heading('A SOUNDTRACK FOR YOUR DAY', '今天，想听点什么？', '熟悉的旋律，新发现的心动。', `<span class="date-tag">${esc(new Intl.DateTimeFormat('en', {month: 'short', day: '2-digit'}).format(new Date()).toUpperCase())}</span>`);
      html += `<div class="page-tabs"><button class="chip selected" data-nav="home">为你发现</button><button class="chip" data-query="华语 流行 音乐">华语</button><button class="chip" data-query="chill relaxing music">放松</button><button class="chip" data-query="lofi focus beats">专注</button><button class="chip" data-query="workout energetic music">活力</button></div>`;
      if (state.homeLoading) html += skeleton();
      else if (state.homeError) html += empty('headphones', '音乐暂时没有抵达', '暂时无法连接 YouTube Music。检查网络连接后重试，也可以直接搜索歌曲。', 'retry-home', '重新加载');
      else if (!state.home.length) html += empty('music', '从一首喜欢的歌开始', '暂时没有获取到推荐音乐。搜索歌名或歌手，开始你的第一段旋律。', 'focus-search', '搜索音乐');
      else {
        const first = state.home[0].tracks[0];
        html += `<section class="hero" aria-label="今日音乐推荐"><div class="hero-copy"><div class="hero-kicker">${icon('headphones')}PRESS PLAY. FEEL MORE.</div><h2>把这一刻，<br>交给音乐。</h2><p class="hero-description">${esc(first.title)}${first.artist ? `<br>${esc(first.artist)}` : ''}</p><button class="primary-button" data-action="play" data-id="${first.id}">${icon('play')}开始聆听</button></div><div class="hero-art">${cover(first, '', false)}</div></section>${renderSections(state.home)}<p class="intro-note">音乐内容来自 YouTube Music。点击歌曲后，由 YouTube 官方播放器提供播放。</p>`;
      }
    } else if (state.tab === 'discover') {
      html = heading('FIND YOUR FREQUENCY', '循着心情，发现音乐', '不用想太多，选一个此刻的频率。');
      html += `<div class="discovery-intro"><button class="mood-tile" data-query="lofi focus beats"><span><strong>心无旁骛</strong><small>FOCUS / LO-FI</small></span>${icon('headphones')}</button><button class="mood-tile" data-query="jazz cafe music"><span><strong>慢一点也好</strong><small>JAZZ / CHILL</small></span>${icon('sun')}</button><button class="mood-tile" data-query="indie pop music"><span><strong>找回好心情</strong><small>INDIE / POP</small></span>${icon('bolt')}</button><button class="mood-tile" data-query="ambient night music"><span><strong>夜还很长</strong><small>AMBIENT / NIGHT</small></span>${icon('moon')}</button></div>`;
      html += state.homeLoading ? skeleton() : state.homeError ? empty('compass', '发现频道暂时离线', '选择上面的音乐方向发起搜索，或重新加载推荐。', 'retry-home', '重新加载') : renderSections(state.home);
    } else if (state.tab === 'search') {
      html = heading('FIND SOMETHING YOU LOVE', `关于「${state.query}」`, state.searchLoading ? '正在 YouTube Music 搜索…' : state.searchError ? '搜索未完成' : `${state.searchResults.length} 首搜索结果`);
      html += state.searchLoading ? skeleton(true) : state.searchError ? empty('search', '搜索暂时未能完成', '检查网络后再试一次，或试试不同的关键词。', 'retry-search', '重新搜索') : state.searchResults.length ? trackRows(state.searchResults) + '<p class="list-summary">搜索结果来自 YouTube Music · 收藏与歌单保存在本机</p>' : empty('search', '没有找到这段旋律', '试试更短的歌名、歌手名称，或者换一种拼写。', 'focus-search', '换个关键词');
    } else {
      const playlist = state.tab === 'playlist' ? state.library.playlists.find(p => p.id === state.playlistID) : null;
      const items = state.tab === 'liked' ? state.library.liked : playlist?.tracks || [];
      const title = state.tab === 'liked' ? '把喜欢，留在这里' : playlist?.name || '本地歌单';
      html = `<div class="page-heading collection-heading"><div class="collection-art">${icon(state.tab === 'liked' ? 'heart' : 'album')}</div><div style="flex:1"><span class="eyebrow">${state.tab === 'liked' ? 'YOUR FAVORITES' : 'YOUR OWN MIX'}</span><h1>${esc(title)}</h1><p>${state.tab === 'liked' ? '每一次收藏，都有自己的理由。' : `${items.length} 首歌曲 · 保存在本机`}</p></div></div>`;
      if (!state.libraryLoaded) html += state.libraryError ? empty('album', '本地音乐库读取失败', '为保护已有收藏，目前已暂停保存。重新读取成功后即可继续使用。', 'retry-library', '重新读取音乐库') : skeleton(true);
      else if (!items.length) html += empty(state.tab === 'liked' ? 'heart' : 'playlist', state.tab === 'liked' ? '为喜欢的歌，点一颗心' : '歌单的第一首，由你决定', state.tab === 'liked' ? '在歌曲旁点击爱心，就能在这里再次找到它。' : '搜索音乐后，点击歌曲旁的歌单按钮，将它收录进来。', 'focus-search', '去发现好音乐');
      else html += `<div class="collection-toolbar"><button class="primary-button" data-action="play-all">${icon('play')}播放全部</button><span>${items.length} 首歌曲</span>${playlist ? `<button class="text-button" data-action="delete-playlist">删除歌单</button>` : ''}</div>${trackRows(items)}`;
      if (playlist && state.libraryLoaded) html += '<div class="playlist-tools"><button class="text-button" data-action="rename-playlist">重命名歌单</button></div>';
      if (playlist && !items.length && state.libraryLoaded) html += '<button class="text-button" data-action="delete-playlist">删除空歌单</button>';
    }
    main.innerHTML = html;
    main.scrollTop = scrollTop;
    restore();
  }

  function navigate(tab, playlistID = '') {
    state.tab = tab; state.playlistID = playlistID;
    renderLibraryNav(); renderMain();
    $('#main-content').scrollTop = 0;
    if (state.queueOpen) toggleQueue(false);
  }

  async function search(query) {
    query = String(query).trim();
    if (!query || Array.from(query).length > 160) { toast(query ? '搜索关键词最多 160 个字符，请缩短后重试。' : '输入歌名、歌手或专辑后开始搜索。'); $('#search-input').focus(); return; }
    state.query = query; state.searchLoading = true; state.searchError = false; state.searchResults = [];
    $('#search-input').value = query;
    const revision = ++state.searchRevision;
    navigate('search');
    try {
      const response = await api('Search', query);
      if (revision !== state.searchRevision) return;
      state.searchResults = register(tracks(response?.tracks));
    } catch { if (revision === state.searchRevision) state.searchError = true; }
    if (revision === state.searchRevision) { state.searchLoading = false; if (state.tab === 'search') renderMain(); }
  }

  function visibleTracks() {
    if (state.tab === 'search') return state.searchResults;
    if (state.tab === 'liked') return state.library.liked;
    if (state.tab === 'playlist') return state.library.playlists.find(p => p.id === state.playlistID)?.tracks || [];
    return state.home.flatMap(section => section.tracks).filter((item, index, items) => items.findIndex(t => t.id === item.id) === index);
  }

  function youtubeAPI() {
    if (window.YT?.Player) return Promise.resolve(window.YT);
    if (youtubePromise) return youtubePromise;
    youtubePromise = new Promise((resolve, reject) => {
      const timeout = setTimeout(() => { youtubePromise = null; reject(new Error('Player timeout')); }, 16000);
      window.onYouTubeIframeAPIReady = () => { clearTimeout(timeout); resolve(window.YT); };
      const oldScript = document.querySelector('script[data-youtube-api]');
      if (oldScript) oldScript.remove();
      const script = document.createElement('script');
      script.src = 'https://www.youtube.com/iframe_api'; script.dataset.youtubeApi = 'true';
      script.referrerPolicy = 'strict-origin-when-cross-origin';
      script.onerror = () => { clearTimeout(timeout); youtubePromise = null; reject(new Error('Player unavailable')); };
      document.head.append(script);
    });
    return youtubePromise;
  }

  function setPlayerStatus(status, message = '') {
    state.playerStatus = status; state.playerError = message;
    if (status !== 'loading') clearTimeout(state.playerCreationTimer);
    renderNow();
    document.querySelectorAll('.track-number').forEach(el => { el.innerHTML = el.dataset.id === state.activeTrack?.id && status === 'playing' ? equalizer : el.dataset.number; });
  }

  function playerMatchesSelection(player = state.player) {
    try {
      if (!player?.getVideoUrl) return true;
      return new URL(player.getVideoUrl()).searchParams.get('v') === state.desiredID;
    } catch { return false; }
  }

  async function loadTrack(item) {
    if (!item || !validID(item.id)) return;
    const revision = ++state.loadRevision;
    if (state.player && !state.playerReady && state.playerStatus === 'error') {
      try { state.player.destroy(); } catch { /* Rebuild a player that never became ready. */ }
      state.player = null;
      const oldMount = document.getElementById('youtube-mount');
      if (oldMount) oldMount.remove();
      const mount = document.createElement('div'); mount.id = 'youtube-mount';
      $('.video-stage').append(mount);
    }
    state.activeTrack = item; state.desiredID = item.id; state.elapsed = 0; state.duration = 0; state.seeking = false; state.seekTarget = null;
    clearTimeout(state.playerCreationTimer);
    state.playerCreationTimer = setTimeout(() => {
      if (revision === state.loadRevision && state.playerStatus === 'loading') setPlayerStatus('error', '播放器连接超时。检查网络后重试，或打开 YouTube Music 继续播放。');
    }, 25000);
    setPlayerStatus('loading'); renderQueue(); renderMain(); updateTimeline();
    try {
      const YT = await youtubeAPI();
      if (revision !== state.loadRevision) return;
      if (state.player && state.playerReady) {
        state.player.loadVideoById(item.id);
        return;
      }
      if (state.player) return; // onReady will load the most recent selection.
      state.player = new YT.Player('youtube-mount', {
        width: '100%', height: '208', videoId: item.id,
        playerVars: {autoplay: 1, controls: 1, playsinline: 1, rel: 0, origin: location.origin, enablejsapi: 1},
        events: {
          onReady(event) {
            if (event.target !== state.player) return;
            state.playerReady = true;
            event.target.getIframe().setAttribute('title', 'YouTube 官方音乐播放器');
            event.target.getIframe().setAttribute('referrerpolicy', 'strict-origin-when-cross-origin');
            event.target.setVolume(state.volume);
            if (state.muted) event.target.mute();
            if (state.desiredID !== item.id) event.target.loadVideoById(state.desiredID);
            else event.target.playVideo();
          },
          onStateChange(event) {
            if ((event.target && event.target !== state.player) || !playerMatchesSelection(event.target)) return;
            if (event.data === YT.PlayerState.PLAYING) setPlayerStatus('playing');
            else if (event.data === YT.PlayerState.PAUSED) setPlayerStatus('paused');
            else if (event.data === YT.PlayerState.BUFFERING) setPlayerStatus('loading');
            else if (event.data === YT.PlayerState.CUED) setPlayerStatus('paused');
            else if (event.data === YT.PlayerState.ENDED) { setPlayerStatus('ended'); advance(1, true); }
            updateTimeline();
          },
          onError(event) {
            if ((event.target && event.target !== state.player) || !playerMatchesSelection(event.target)) return;
            const messages = {2: '这首歌的播放地址无效。请换一首或打开官方播放器。', 5: '当前系统播放器无法解码这段音频。可以在 YouTube Music 中继续播放。', 100: '这段视频已被移除，或仅限特定账号观看。', 101: '这首歌禁止在第三方客户端嵌入播放。', 150: '这首歌禁止在第三方客户端嵌入播放。', 153: 'YouTube 未能验证客户端来源。可以打开官方播放器继续聆听。'};
            setPlayerStatus('error', messages[event.data] || '这首歌暂时无法播放，可能受网络、地区或账号权限限制。');
          },
          onAutoplayBlocked(event) { if (!event?.target || event.target === state.player) setPlayerStatus('blocked', '播放未能开始。请点击播放重试，或打开播放器后继续。'); },
        },
      });
    } catch { if (revision === state.loadRevision) setPlayerStatus('error', 'YouTube 播放器未能加载。请检查网络后重试，或在官方播放器中打开。'); }
  }

  function playItem(item, preserveQueue = false, queueIndex = -1) {
    if (!item) return;
    if (preserveQueue) state.queueIndex = queueIndex >= 0 ? queueIndex : state.queue.findIndex(t => t.id === item.id);
    else {
      const source = visibleTracks();
      state.queue = source.some(t => t.id === item.id) ? [...source] : [item];
      state.queueIndex = state.queue.findIndex(t => t.id === item.id);
    }
    resetShuffleTraversal();
    loadTrack(item);
  }

  function resetShuffleTraversal() {
    state.shuffleRemaining = shuffleIndices(state.queue.map((_, index) => index).filter(index => index !== state.queueIndex));
    state.shuffleHistory = state.queueIndex >= 0 ? [state.queueIndex] : [];
    state.shuffleCursor = state.shuffleHistory.length - 1;
  }

  function shuffleIndices(indices) {
    for (let i = indices.length - 1; i > 0; i--) { const j = Math.floor(Math.random() * (i + 1)); [indices[i], indices[j]] = [indices[j], indices[i]]; }
    return indices;
  }

  function togglePlayback() {
    if (!state.activeTrack) { const first = state.queue[0] || visibleTracks()[0]; if (first) playItem(first, Boolean(state.queue.length), 0); else toast('先搜索或选择一首音乐。'); return; }
    if (!state.playerReady) { if (state.playerStatus === 'error') loadTrack(state.activeTrack); else toast('播放器正在准备，请稍候。'); return; }
    if (state.playerStatus === 'playing') state.player.pauseVideo();
    else if (state.playerStatus === 'error') loadTrack(state.activeTrack);
    else if (state.playerStatus === 'ended') { state.player.seekTo(0, true); state.player.playVideo(); }
    else state.player.playVideo();
  }

  function advance(direction, ended = false) {
    if (!state.queue.length) return;
    if (ended && state.repeat === 'one') { state.player.seekTo(0, true); state.player.playVideo(); return; }
    if (direction < 0 && state.elapsed > 3 && state.playerReady) { state.player.seekTo(0, true); state.elapsed = 0; updateTimeline(); return; }
    let index = state.queueIndex + direction;
    if (state.shuffle) {
      if (direction < 0) {
        if (state.shuffleCursor <= 0) { if (!ended) toast('已经是本次播放的第一首'); return; }
        index = state.shuffleHistory[--state.shuffleCursor];
      } else if (state.shuffleCursor < state.shuffleHistory.length - 1) {
        index = state.shuffleHistory[++state.shuffleCursor];
      } else {
        if (!state.shuffleRemaining.length) {
          if (state.repeat !== 'all') { if (!ended) toast('本轮随机队列已播放完毕'); return; }
          state.shuffleRemaining = shuffleIndices(state.queue.map((_, i) => i).filter(i => i !== state.queueIndex || state.queue.length === 1));
        }
        index = state.shuffleRemaining.shift();
        state.shuffleHistory.push(index);
        if (state.shuffleHistory.length > 500) state.shuffleHistory.shift();
        state.shuffleCursor = state.shuffleHistory.length - 1;
      }
    } else if (index < 0 || index >= state.queue.length) {
      if (state.repeat === 'all') index = index < 0 ? state.queue.length - 1 : 0;
      else { if (!ended) toast(direction > 0 ? '已经是队列中的最后一首' : '已经是队列中的第一首'); return; }
    }
    state.queueIndex = index;
    loadTrack(state.queue[index]);
  }

  function setVolume(value) {
    state.volume = Math.max(0, Math.min(100, Number(value) || 0));
    if (state.volume > 0) { state.muted = false; state.audibleVolume = state.volume; }
    if (state.playerReady) {
      state.player.setVolume(state.volume);
      if (state.muted) state.player.mute(); else state.player.unMute();
    }
    renderVolume();
  }

  function toggleMute() {
    if (state.volume === 0) { setVolume(state.audibleVolume); return; }
    state.muted = !state.muted;
    if (state.playerReady) state.muted ? state.player.mute() : state.player.unMute();
    renderVolume();
  }

  function renderVolume() {
    $('#volume').value = state.muted ? 0 : state.volume;
    $('#volume').style.setProperty('--range-progress', `${state.muted ? 0 : state.volume}%`);
    $('#mute-button').innerHTML = icon(state.muted || !state.volume ? 'muted' : 'volume');
    const label = state.muted || !state.volume ? '取消静音' : '静音';
    $('#mute-button').setAttribute('aria-label', label);
    $('#mute-button').setAttribute('title', label);
    $('#mute-button').setAttribute('aria-pressed', String(state.muted || !state.volume));
  }

  function renderNow() {
    const item = state.activeTrack;
    renderPlayerView();
    $('#now-title').textContent = item?.title || '让音乐发生';
    $('#now-artist').textContent = item ? metadata(item) || 'YouTube Music' : '选择一首歌，给此刻一个节奏。';
    $('#transport-title').textContent = item?.title || '还没有播放音乐';
    $('#transport-artist').textContent = item?.artist || (item ? 'YouTube Music' : '发现你的下一首喜欢');
    const currentCover = $('#transport-cover').dataset.track;
    if (item && currentCover !== item.id) { $('#transport-cover').innerHTML = cover(item, '', false, 160); $('#transport-cover').dataset.track = item.id; }
    for (const id of ['#now-like', '#transport-like']) {
      $(id).disabled = !item || !state.libraryLoaded;
      $(id).classList.toggle('active', Boolean(item && isLiked(item.id)));
      $(id).innerHTML = icon('heart', Boolean(item && isLiked(item.id)));
      $(id).setAttribute('aria-label', item && isLiked(item.id) ? '取消收藏当前歌曲' : '收藏当前歌曲');
    }
    const playing = state.playerStatus === 'playing';
    $('#play-button').innerHTML = icon(playing ? 'pause' : 'play');
    $('#play-button').setAttribute('aria-label', playing ? '暂停' : '播放');
    $('#previous-button').disabled = !item;
    $('#next-button').disabled = !state.queue.length;
    const labels = {idle: '待播放', loading: '正在连接', playing: '播放中', paused: '已暂停', ended: '播放结束', error: '播放受限', blocked: '等待播放'};
    $('#live-indicator').innerHTML = `<i></i>${labels[state.playerStatus] || '待播放'}`;
    $('#live-indicator').classList.toggle('playing', playing);
    $('#official-link').href = item ? `https://music.youtube.com/watch?v=${item.id}` : 'https://music.youtube.com/';
    const notice = $('#player-notice');
    notice.hidden = !['loading', 'error', 'blocked'].includes(state.playerStatus);
    notice.classList.toggle('error', ['error', 'blocked'].includes(state.playerStatus));
    const message = state.playerStatus === 'loading' ? '正在加载音乐…' : state.playerError;
    notice.innerHTML = `${icon('info')}<div><p>${esc(message)}</p>${state.playerStatus === 'error' ? '<button data-action="retry-player">重新尝试播放</button>' : ''}${state.playerStatus === 'blocked' ? '<button data-action="toggle">继续播放</button><button data-action="show-video">打开播放器</button>' : ''}</div>`;
  }

  function renderPlayerView() {
    const item = state.activeTrack;
    const artwork = $('#now-artwork');
    const key = item ? JSON.stringify([item.id, item.thumbnail]) : '';
    if (item && artwork.dataset.track !== key) {
      artwork.innerHTML = cover(item, '', false, 768);
      artwork.dataset.track = key;
      artwork.classList.remove('broken-cover');
    }
    artwork.setAttribute('aria-label', item ? `${item.title} · 歌曲封面` : '尚未选择歌曲');
    const showVideo = Boolean(item && state.videoVisible);
    artwork.hidden = showVideo;
    // Keep the same iframe mounted in both views so presentation changes never
    // restart playback, reset the queue, or lose the current seek position.
    $('#video-stage').hidden = !showVideo;
    $('#video-stage').setAttribute('aria-hidden', String(!showVideo));
    $('#video-toggle').disabled = !item;
    $('#video-toggle').textContent = showVideo ? '显示封面' : '显示视频';
    $('#video-toggle').setAttribute('aria-expanded', String(showVideo));
  }

  function queueRow(item, index, full = false) {
    const current = index === state.queueIndex;
    return `<div class="queue-item${current ? ' current' : ''}">${full ? `<span class="queue-order">${current ? icon('play') : index + 1}</span>` : ''}<button class="row-track-button" data-action="play-queue" data-index="${index}" aria-label="播放 ${esc(item.title)}">${cover(item, '', true, 160)}<span class="row-meta"><strong title="${esc(item.title)}">${esc(item.title)}</strong><small>${esc(item.artist || item.album || 'YouTube Music')}</small></span></button>${full ? button('remove-queue', 'close', current ? '当前歌曲保留在队列中' : `从队列移除 ${item.title}`, `data-index="${index}"${current ? ' disabled' : ''}`) : `<span>${esc(item.duration)}</span>`}</div>`;
  }

  function renderQueue() {
    const restore = rememberFocus($('#queue-full'));
    const scrollTop = $('#queue-full').scrollTop;
    const indices = state.shuffle ? [...state.shuffleHistory.slice(state.shuffleCursor + 1), ...state.shuffleRemaining] : state.queue.map((_, index) => index).slice(state.queueIndex + 1);
    const upcoming = indices.map(index => ({item: state.queue[index], index}));
    $('#queue-count').textContent = `${upcoming.length} 首`;
    $('#queue-preview').innerHTML = upcoming.length ? upcoming.slice(0, 4).map(({item, index}) => queueRow(item, index)).join('') : `<div class="queue-empty">${icon('queue')}<p>${state.activeTrack ? '下一段旋律，等你来选' : '好音乐，总值得期待'}</p><span>点击歌曲旁的 +，将它加入队列。</span></div>`;
    $('#drawer-count').textContent = state.queue.length;
    $('#queue-full').innerHTML = state.queue.length ? state.queue.map((item, index) => queueRow(item, index, true)).join('') : empty('queue', '队列还是空的', '搜索一首音乐，或将喜欢的歌曲加入队列。');
    $('#clear-queue').disabled = !state.queue.some((_, index) => index !== state.queueIndex);
    $('#queue-full').scrollTop = scrollTop;
    $('#next-button').disabled = !state.queue.length;
    restore();
  }

  function toggleQueue(value = !state.queueOpen) {
    if (value) state.queueReturnFocus = focusKey(document.activeElement);
    const focusInside = $('#queue-drawer').contains?.(document.activeElement);
    state.queueOpen = value;
    $('#queue-drawer').hidden = !value;
    $('#queue-button').classList.toggle('active', value);
    $('#queue-button').setAttribute('aria-expanded', String(value));
    $('#queue-button').setAttribute('aria-label', value ? '关闭播放队列' : '展开播放队列');
    if (value) { renderQueue(); $('#queue-drawer [data-action="queue"]').focus(); }
    else if (focusInside) restoreFocus(document, state.queueReturnFocus);
  }

  function updateTimeline() {
    if (state.playerReady && state.playerStatus !== 'loading' && playerMatchesSelection()) {
      try {
        const elapsed = Number(state.player.getCurrentTime()) || 0;
        if (state.seekTarget !== null && Math.abs(elapsed - state.seekTarget) > 2 && Date.now() - state.seekAt < 1500) state.elapsed = state.seekTarget;
        else { state.seekTarget = null; state.elapsed = elapsed; }
        state.duration = Number(state.player.getDuration()) || 0;
        if (state.player.getVolume && state.player.isMuted) {
          const volume = Number(state.player.getVolume());
          if (Number.isFinite(volume)) { state.volume = Math.max(0, Math.min(100, volume)); if (state.volume > 0) state.audibleVolume = state.volume; }
          state.muted = state.player.isMuted();
          renderVolume();
        }
      } catch { /* A disconnected player must not interrupt the rest of the UI. */ }
    }
    const displayed = state.seeking ? Number($('#progress').value) : state.elapsed;
    $('#elapsed-time').textContent = time(displayed);
    $('#duration-time').textContent = time(state.duration);
    $('#progress').disabled = !state.playerReady || state.duration <= 0 || ['loading', 'error'].includes(state.playerStatus);
    $('#progress').max = state.duration || 100;
    if (!state.seeking) {
      $('#progress').value = state.elapsed;
      $('#progress').style.setProperty('--range-progress', `${state.duration ? Math.min(100, state.elapsed / state.duration * 100) : 0}%`);
    }
    $('#progress').setAttribute('aria-valuetext', `${time(displayed)} / ${time(state.duration)}`);
  }

  function showPlaylistDialog(item = null, playlist = null) {
    if (!requireLibrary()) return;
    state.dialogTrack = item;
    state.editingPlaylistID = playlist?.id || '';
    state.dialogReturnFocus = focusKey(document.activeElement);
    $('#playlist-feedback').textContent = '';
    $('#dialog-title').textContent = playlist ? '重命名歌单' : item ? '加入本地歌单' : '创建你的歌单';
    $('#dialog-description').textContent = item ? item.title : '把属于同一种心情的歌，放在一起。';
    $('#playlist-options').innerHTML = item ? state.library.playlists.map(p => `<button type="button" class="playlist-choice" title="${esc(p.name)}" data-add-to-playlist="${esc(p.id)}">${icon('album')}<span>${esc(p.name)}</span><span>${p.tracks.some(t => t.id === item.id) ? '已收录' : `${p.tracks.length} 首`}</span></button>`).join('') : '';
    $('#playlist-name').value = playlist?.name || '';
    $('#playlist-name').required = true;
    $('#playlist-submit').innerHTML = `${icon(playlist ? 'check' : 'plus')}${playlist ? '保存名称' : item ? '创建新歌单并加入' : '创建歌单'}`;
    $('#playlist-dialog').showModal();
    if (!item || !state.library.playlists.length) $('#playlist-name').focus();
    if (playlist) $('#playlist-name').select();
  }

  function setDialogSaving(value) {
    state.dialogSaving = value;
    $('#playlist-dialog').setAttribute('aria-busy', String(value));
    $('#playlist-submit').disabled = value;
    $('#playlist-name').disabled = value;
    document.querySelectorAll('#playlist-dialog button').forEach(button => { button.disabled = value; });
  }

  async function addToPlaylist(playlistID, item) {
    if (!item || !requireLibrary() || state.dialogSaving) return;
    const next = clone(state.library);
    const playlist = next.playlists.find(p => p.id === playlistID);
    if (!playlist) return;
    if (playlist.tracks.some(t => t.id === item.id)) { toast('这首歌已经在歌单里了'); $('#playlist-dialog').close(); return; }
    playlist.tracks.push(item);
    setDialogSaving(true);
    const saved = await saveLibrary(next, `已加入「${playlist.name}」`);
    setDialogSaving(false);
    if (saved) $('#playlist-dialog').close();
    else $('#playlist-feedback').textContent = '保存失败，请检查本机存储后重试。你的输入已保留。';
  }

  function removeQueue(index) {
    if (!Number.isInteger(index) || index < 0 || index >= state.queue.length) return;
    if (index === state.queueIndex) { toast('当前正在播放的歌曲会保留在队列中。'); return; }
    state.queue.splice(index, 1);
    if (index < state.queueIndex) state.queueIndex--;
    const reindex = value => value > index ? value - 1 : value;
    state.shuffleRemaining = state.shuffleRemaining.filter(value => value !== index).map(reindex);
    const newCursor = state.shuffleHistory.slice(0, state.shuffleCursor + 1).filter(value => value !== index).length - 1;
    state.shuffleHistory = state.shuffleHistory.filter(value => value !== index).map(reindex);
    state.shuffleCursor = newCursor;
    renderQueue();
  }

  function handleAction(action, el) {
    const item = state.allTracks.get(el?.dataset?.id);
    if (action === 'play') playItem(item);
    else if (action === 'like') toggleLike(item);
    else if (action === 'like-current') toggleLike(state.activeTrack);
    else if (action === 'enqueue' && item) { state.queue.push(item); if (state.shuffle) state.shuffleRemaining.push(state.queue.length - 1); renderQueue(); renderNow(); toast('已加入播放队列'); }
    else if (action === 'add-playlist') showPlaylistDialog(item);
    else if (action === 'create-playlist') showPlaylistDialog();
    else if (action === 'close-dialog' && !state.dialogSaving) $('#playlist-dialog').close();
    else if (action === 'toggle') togglePlayback();
    else if ((action === 'toggle-video' || action === 'show-video') && state.activeTrack) { state.videoVisible = action === 'show-video' || !state.videoVisible; renderPlayerView(); }
    else if (action === 'next') advance(1);
    else if (action === 'previous') advance(-1);
    else if (action === 'mute') toggleMute();
    else if (action === 'queue') toggleQueue();
    else if (action === 'play-queue') { const index = Number(el.dataset.index); playItem(state.queue[index], true, index); }
    else if (action === 'remove-queue') removeQueue(Number(el.dataset.index));
    else if (action === 'clear-queue') { state.queue = state.activeTrack ? [state.activeTrack] : []; state.queueIndex = state.activeTrack ? 0 : -1; resetShuffleTraversal(); renderQueue(); if (state.queueOpen) $('#queue-drawer [data-action="queue"]').focus(); toast('已清除待播歌曲'); }
    else if (action === 'shuffle') { state.shuffle = !state.shuffle; resetShuffleTraversal(); renderQueue(); $('#shuffle-button').classList.toggle('active', state.shuffle); $('#shuffle-button').setAttribute('aria-pressed', String(state.shuffle)); $('#shuffle-button').setAttribute('aria-label', state.shuffle ? '关闭随机播放' : '开启随机播放'); toast(state.shuffle ? '随机播放已开启' : '随机播放已关闭'); }
    else if (action === 'repeat') { state.repeat = {off: 'all', all: 'one', one: 'off'}[state.repeat]; $('#repeat-button').classList.toggle('active', state.repeat !== 'off'); $('#repeat-button').setAttribute('aria-pressed', String(state.repeat !== 'off')); $('#repeat-button').setAttribute('aria-label', {off: '开启列表循环', all: '切换到单曲循环', one: '关闭循环播放'}[state.repeat]); $('#repeat-one').hidden = state.repeat !== 'one'; toast({off: '循环播放已关闭', all: '列表循环已开启', one: '单曲循环已开启'}[state.repeat]); }
    else if (action === 'focus-search') $('#search-input').focus();
    else if (action === 'retry-home') loadHome();
    else if (action === 'retry-search') search(state.query);
    else if (action === 'retry-library') loadLibrary();
    else if (action === 'retry-player' && state.activeTrack) loadTrack(state.activeTrack);
    else if (action === 'play-all') { const items = visibleTracks(); if (items.length) playItem(items[0]); }
    else if (action === 'rename-playlist') showPlaylistDialog(null, state.library.playlists.find(p => p.id === state.playlistID));
    else if (action === 'remove-playlist-track' && item && requireLibrary()) { const next = clone(state.library); const p = next.playlists.find(p => p.id === state.playlistID); if (p) { p.tracks = p.tracks.filter(t => t.id !== item.id); saveLibrary(next, '已从歌单移除'); } }
    else if (action === 'delete-playlist' && requireLibrary()) {
      const p = state.library.playlists.find(p => p.id === state.playlistID);
      if (p) { state.deletePlaylistID = p.id; $('#delete-feedback').textContent = ''; $('#delete-description').textContent = `「${p.name}」及其歌曲列表将从本机移除。你收藏的歌曲会保留。`; $('#delete-dialog').showModal(); }
    }
    else if (action === 'cancel-delete' && !state.deleting) $('#delete-dialog').close();
    else if (action === 'confirm-delete' && requireLibrary() && !state.deleting) {
      const next = clone(state.library); next.playlists = next.playlists.filter(p => p.id !== state.deletePlaylistID);
      state.deleting = true;
      document.querySelectorAll('#delete-dialog button').forEach(button => { button.disabled = true; });
      saveLibrary(next, '已删除歌单').then(saved => {
        state.deleting = false;
        document.querySelectorAll('#delete-dialog button').forEach(button => { button.disabled = false; });
        if (saved) { $('#delete-dialog').close(); navigate('liked'); $('#main-content').focus(); }
        else $('#delete-feedback').textContent = '删除未能保存，歌单已保留。请稍后重试。';
      });
    }
  }

  document.querySelectorAll('[data-icon]').forEach(el => { el.innerHTML = icon(el.dataset.icon); });
  document.addEventListener('ytmgo:appearance-error', () => toast('外观设置未能保存，请稍后重试。'));
  document.addEventListener('click', event => {
    const action = event.target.closest('[data-action]');
    if (action && !action.disabled) { handleAction(action.dataset.action, action); return; }
    const nav = event.target.closest('[data-nav]');
    if (nav) { navigate(nav.dataset.nav); return; }
    const playlist = event.target.closest('[data-playlist]');
    if (playlist) { navigate('playlist', playlist.dataset.playlist); return; }
    const query = event.target.closest('[data-query]');
    if (query) { search(query.dataset.query); return; }
    const add = event.target.closest('[data-add-to-playlist]');
    if (add) addToPlaylist(add.dataset.addToPlaylist, state.dialogTrack);
    if (event.target.closest('.brand')) { event.preventDefault(); navigate('home'); }
  });
  document.addEventListener('error', event => {
    if (event.target instanceof HTMLImageElement) nextImageSource(event.target);
  }, true);
  document.addEventListener('load', event => {
    const image = event.target;
    // YouTube may return HTTP 200 with a tiny placeholder for missing high-res
    // artwork. Only upgraded video candidates are checked; the original is final.
    if (image instanceof HTMLImageElement && image.dataset.videoCover === 'true' && image.src !== image.dataset.originalSrc && image.naturalWidth > 0 && image.naturalWidth <= 120) nextImageSource(image);
  }, true);
  $('#search-form').addEventListener('submit', event => { event.preventDefault(); search($('#search-input').value); });
  $('#volume').addEventListener('input', event => setVolume(event.target.value));
  $('#progress').addEventListener('input', event => { state.seeking = true; $('#elapsed-time').textContent = time(event.target.value); event.target.style.setProperty('--range-progress', `${state.duration ? event.target.value / state.duration * 100 : 0}%`); });
  $('#progress').addEventListener('change', event => { if (state.playerReady) { state.elapsed = Number(event.target.value); state.seekTarget = state.elapsed; state.seekAt = Date.now(); state.player.seekTo(state.elapsed, true); } state.seeking = false; updateTimeline(); });
  $('#playlist-form').addEventListener('submit', async event => {
    event.preventDefault();
    if (!requireLibrary() || state.dialogSaving) return;
    const name = $('#playlist-name').value.trim();
    if (!name || Array.from(name).length > 80) { $('#playlist-feedback').textContent = name ? '歌单名称最多 80 个字符。' : '请输入歌单名称。'; $('#playlist-name').focus(); return; }
    $('#playlist-feedback').textContent = '';
    const next = clone(state.library);
    const id = state.editingPlaylistID || `local-${window.crypto?.randomUUID?.() || `${Date.now()}-${Math.random().toString(36).slice(2, 9)}`}`;
    if (state.editingPlaylistID) { const playlist = next.playlists.find(p => p.id === id); if (!playlist) return; playlist.name = name; }
    else next.playlists.push({id, name, tracks: state.dialogTrack ? [state.dialogTrack] : []});
    setDialogSaving(true);
    const saved = await saveLibrary(next, state.editingPlaylistID ? '歌单名称已更新' : `已创建「${name}」`);
    setDialogSaving(false);
    if (saved) { $('#playlist-dialog').close(); if (!state.dialogTrack) { navigate('playlist', id); $('#main-content').focus(); } }
    else $('#playlist-feedback').textContent = '保存失败，请检查本机存储后重试。你的输入已保留。';
  });
  $('#playlist-dialog').addEventListener('cancel', event => { if (state.dialogSaving) event.preventDefault(); });
  $('#playlist-dialog').addEventListener('close', () => { if (!state.dialogSaving) restoreFocus(document, state.dialogReturnFocus); });
  $('#delete-dialog').addEventListener('cancel', event => { if (state.deleting) event.preventDefault(); });
  document.addEventListener('keydown', event => {
    const typing = event.target instanceof HTMLInputElement || event.target instanceof HTMLTextAreaElement || event.target.tagName === 'SELECT' || event.target.isContentEditable;
    if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === 'k') { if ($('#playlist-dialog').open || $('#delete-dialog').open) return; event.preventDefault(); $('#search-input').focus(); $('#search-input').select(); return; }
    // Some WebViews consume named-key menu accelerators before AppKit runs
    // them. Handle keys that reach our page; a native menu event consumes its
    // own key, so this does not dispatch the same action twice.
    const shortcut = {ArrowRight: 'next', ArrowLeft: 'previous', ArrowUp: 'volume-up', ArrowDown: 'volume-down'}[event.key];
    if ((event.metaKey || event.ctrlKey) && !event.altKey && !event.shiftKey && shortcut && !typing && !$('#playlist-dialog').open && !$('#delete-dialog').open) { event.preventDefault(); window.ytmgoControls(shortcut); return; }
    if (event.key === 'Escape') { if (state.queueOpen) toggleQueue(false); return; }
    if (typing || $('#playlist-dialog').open || $('#delete-dialog').open || event.target instanceof HTMLButtonElement || event.target instanceof HTMLAnchorElement) return;
    if (event.code === 'Space') { event.preventDefault(); togglePlayback(); }
  });
  window.ytmgoControls = (action) => {
    if (action === 'search') { if (!$('#playlist-dialog').open && !$('#delete-dialog').open) { $('#search-input').focus(); $('#search-input').select(); } }
    else if (action === 'volume-up') setVolume(state.volume + 5);
    else if (action === 'volume-down') setVolume(state.volume - 5);
    else if (['toggle', 'next', 'previous', 'mute'].includes(action)) handleAction(action);
    else return {ok: false, status: 'unknown-action'};
    return {ok: true, status: state.playerStatus};
  };
  renderVolume(); renderMain(); renderQueue();
  loadHome(); loadLibrary();
  setInterval(updateTimeline, 500);
})();
