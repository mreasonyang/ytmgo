// Runs only in the top-level music document. Account pages keep Google's UI.
(function mount() {
  'use strict';
  if (window.top !== window || location.origin !== 'https://music.youtube.com') return;
  if (!document.documentElement) {
    const ready = new MutationObserver(() => {
      if (document.documentElement) { ready.disconnect(); mount(); }
    });
    ready.observe(document, {childList: true});
    return;
  }
  if (window.ytmgoMusicShell) return;
  window.ytmgoMusicShell = true;
  let mode = YTMGO_APPEARANCE;
  const root = document.documentElement;
  const media = matchMedia('(prefers-color-scheme: dark)');
  function element(tag, attributes = {}, children = []) {
    const node = document.createElement(tag);
    for (const [key, value] of Object.entries(attributes)) node.setAttribute(key, value);
    for (const child of children) node.append(typeof child === 'string' ? document.createTextNode(child) : child);
    return node;
  }
  function icon(name) {
    const paths = {
      home: ['m3 10 9-7 9 7v10H3z', 'M9 20v-7h6v7'],
      explore: ['M21 12a9 9 0 1 1-18 0 9 9 0 0 1 18 0', 'm16 8-2.5 5.5L8 16l2.5-5.5z'],
      library: ['M4 4v16M9 4v16m5-15 5 14M3 20h18'],
      search: ['M17 10.5a6.5 6.5 0 1 1-13 0 6.5 6.5 0 0 1 13 0', 'm16 16 5 5'],
    };
    const svg = document.createElementNS('http://www.w3.org/2000/svg', 'svg');
    for (const [key, value] of Object.entries({viewBox:'0 0 24 24',fill:'none',stroke:'currentColor','stroke-width':'1.7','stroke-linecap':'round','stroke-linejoin':'round','aria-hidden':'true'})) svg.setAttribute(key,value);
    for (const d of paths[name]) { const path = document.createElementNS('http://www.w3.org/2000/svg', 'path'); path.setAttribute('d', d); svg.append(path); }
    return svg;
  }
  function applyAppearance(value) {
    if (!['light', 'dark', 'system'].includes(value)) return false;
    mode = value;
    root.dataset.ytmgoTheme = value === 'system' ? (media.matches ? 'dark' : 'light') : value;
    const select = document.getElementById('ytmgo-appearance');
    if (select) select.value = value;
    return true;
  }
  window.ytmgoSetAppearance = applyAppearance;
  media.addEventListener('change', () => applyAppearance(mode));
  applyAppearance(mode);
  const style = document.createElement('style');
  style.id = 'ytmgo-music-style';
  style.textContent = YTMGO_STYLES;
  (document.head || root).append(style);
  function route(path) {
    // Clicking the first-party navigation element preserves the playback session.
    const original = [...document.querySelectorAll('ytmusic-guide-renderer a[href]')]
      .find(link => { const url = new URL(link.href); return url.origin === location.origin && url.pathname === path; });
    if (original) original.click();
    else location.assign(path);
  }
  function click(selector) {
    const button = document.querySelector(selector);
    if (!button) return false;
    button.click();
    return true;
  }
  window.ytmgoControls = action => {
    const player = document.querySelector('video, audio');
    switch (action) {
      case 'toggle': return click('ytmusic-player-bar #play-pause-button');
      case 'next': return click('ytmusic-player-bar .next-button');
      case 'previous': return click('ytmusic-player-bar .previous-button');
      case 'mute': if (player) { player.muted = !player.muted; return true; } return false;
      case 'volume-up': case 'volume-down':
        if (player) { player.volume = Math.max(0, Math.min(1, player.volume + (action === 'volume-up' ? .1 : -.1))); return true; } return false;
      case 'search': {
        click('ytmusic-search-box .search-icon');
        const input = document.querySelector('ytmusic-search-box input');
        input?.focus(); input?.select(); return !!input;
      }
      case 'home': route('/'); return true;
      case 'explore': route('/explore'); return true;
      case 'library': route('/library'); return true;
      case 'account': {
        const signIn = document.querySelector('ytmusic-nav-bar a[href*="accounts.google.com"]');
        if (signIn) { signIn.click(); return true; }
        return click('ytmusic-nav-bar #avatar-btn');
      }
      default: return false;
    }
  };
  // WebKit may deliver named-key accelerators to the document before AppKit.
  document.addEventListener('keydown', event => {
    const action = {ArrowRight:'next', ArrowLeft:'previous', ArrowUp:'volume-up', ArrowDown:'volume-down'}[event.key];
    if (!action || !(event.metaKey || event.ctrlKey) || event.altKey || event.shiftKey) return;
    if (event.target.closest?.('input, textarea, select, [contenteditable="true"], [role="dialog"]') || document.querySelector('tp-yt-paper-dialog[opened]')) return;
    event.preventDefault();
    event.stopPropagation();
    window.ytmgoControls(action);
  }, true);
  let pending = false;
  let reportedAccountState = null;
  function syncAccountState() {
    // Inspect only the official navigation controls, never cookies or account
    // identity. Unknown/loading states keep the destructive action hidden.
    const signIn = document.querySelector('ytmusic-nav-bar a[href*="accounts.google.com"]');
    const avatar = document.querySelector('ytmusic-nav-bar #avatar-btn');
    const visible = node => !!node && !node.closest('[hidden], [aria-hidden="true"]') && node.getClientRects().length > 0;
    const signedIn = !visible(signIn) && visible(avatar);
    const signOut = document.getElementById('ytmgo-signout');
    if (signOut && signOut.hidden === signedIn) signOut.hidden = !signedIn;
    if (signedIn !== reportedAccountState) {
      reportedAccountState = signedIn;
      location.href = 'ytmgo://account/' + (signedIn ? 'signed-in' : 'signed-out');
    }
  }
  function update() {
    pending = false;
    if (!document.body || !document.querySelector('ytmusic-app')) return;
    const signIn = document.querySelector('ytmusic-nav-bar a[href*="accounts.google.com"]');
    if (signIn && signIn.textContent !== '登录') {
      signIn.textContent = '登录';
      signIn.setAttribute('aria-label', '登录 Google 账号');
    }
    const search = document.querySelector('ytmusic-search-box input');
    if (search && search.placeholder !== '搜索歌曲、艺人、专辑或歌单') search.placeholder = '搜索歌曲、艺人、专辑或歌单';
    if (!document.getElementById('ytmgo-sidebar')) {
      const aside = document.createElement('aside');
      aside.id = 'ytmgo-sidebar';
      aside.setAttribute('aria-label', 'ytmgo 导航');
      aside.append(
        element('a', {class:'ytmgo-brand', href:'/', 'data-route':'/'}, [
          element('span', {class:'ytmgo-mark','aria-hidden':'true'}, ['y', element('span',{},['•'])]),
          element('span',{},['ytmgo',element('small',{},['音乐，随你。'])])
        ]),
        element('nav', {'aria-label':'音乐导航'}, [
          element('a', {href:'/', 'data-route':'/'}, [icon('home'),'首页']),
          element('button', {'data-action':'search'}, [icon('search'),'搜索',element('span',{class:'ytmgo-key'},['⌘ K'])]),
          element('a', {href:'/explore', 'data-route':'/explore'}, [icon('explore'),'发现']),
          element('a', {href:'/library', 'data-route':'/library'}, [icon('library'),'音乐库'])
        ]),
        element('div',{class:'ytmgo-library-note'},[
          element('span',{class:'ytmgo-eyebrow'},['YOUR COLLECTION']),
          element('p',{},['喜欢的音乐，',element('br'),'都在一起。']),
          element('span',{},['歌单、专辑与收藏',element('br'),'在音乐库中随时找到。'])
        ]),
        element('div',{class:'ytmgo-sidebar-bottom'},[
          element('a',{id:'ytmgo-signout',class:'ytmgo-signout',hidden:'',href:'ytmgo://session/signout','aria-label':'退出登录并清除本机账号数据'},['退出登录 / 清除账号数据']),
          element('label',{for:'ytmgo-appearance'},['外观']),
          element('select',{id:'ytmgo-appearance','aria-label':'外观'},[
            element('option',{value:'system'},['跟随系统']),element('option',{value:'light'},['浅色']),element('option',{value:'dark'},['深色'])
          ]),
          element('small',{},['音乐与账号由 YouTube Music 提供'])
        ])
      );
      aside.addEventListener('click', event => {
        const link = event.target.closest('[data-route]');
        if (link) { event.preventDefault(); route(link.dataset.route); }
        const button = event.target.closest('[data-action]');
        if (button) window.ytmgoControls(button.dataset.action);
      });
      aside.querySelector('select').addEventListener('change', event => {
        location.href = 'ytmgo://appearance/' + event.target.value;
      });
      document.body.append(aside);
      applyAppearance(mode);
    }
    syncAccountState();
    const path = location.pathname;
    root.dataset.ytmgoRoute = path === '/' ? 'home' : (path === '/watch' || path.startsWith('/podcast/')) ? 'player' : 'browse';
    document.querySelectorAll('#ytmgo-sidebar nav [data-route]').forEach(link => {
      const active = link.dataset.route === path || (link.dataset.route === '/library' && path.startsWith('/library'));
      if (active) { if (link.getAttribute('aria-current') !== 'page') link.setAttribute('aria-current', 'page'); }
      else link.removeAttribute('aria-current');
    });
    const content = document.querySelector('ytmusic-app-layout > #content');
    if (content && !document.getElementById('ytmgo-welcome')) {
      const welcome = document.createElement('header');
      welcome.id = 'ytmgo-welcome';
      welcome.append(
        element('div',{},[element('span',{class:'ytmgo-eyebrow'},['MAKE ROOM FOR MUSIC']),element('h1',{},['今天，想听点什么？']),element('p',{},['熟悉的旋律，新的发现。'])]),
        element('span',{class:'ytmgo-welcome-art','aria-hidden':'true'},Array.from({length:5},()=>element('i')))
      );
      content.prepend(welcome);
    }
  }
  const observer = new MutationObserver(() => {
    if (!pending) { pending = true; requestAnimationFrame(update); }
  });
  observer.observe(root, {childList: true, subtree: true, attributes: true, attributeFilter: ['hidden', 'aria-hidden', 'style', 'class']});
  document.addEventListener('yt-navigate-finish', update);
  document.addEventListener('DOMContentLoaded', update, {once: true});
  update();
})();
