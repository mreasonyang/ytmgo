// playerbar — the persistent transport. Three zones: what's playing (left),
// transport + scrubber (center), context + volume (right). In compact
// windows the same DOM condenses into a thumb-zone mini player.
let kickProgress = () => {};

function artistLinks(artists) {
  return artists.flatMap(({name, linked}, index) => [
    ...(index ? [h('span', {class: 'y-sep', 'aria-hidden': 'true', text: ' · '})] : []),
    linked
      ? h('button', {type: 'button', class: 'y-link', onclick: event => { event.stopPropagation(); bridge.openArtist(name); }}, name)
      : h('span', {text: name}),
  ]);
}

function mountPlayerbar() {
  const openNowPlaying = () => (matchMedia('(max-width: 760px)').matches ? setImmersive(true) : showPanel(0, true));

  const img = h('img', {alt: '', decoding: 'async', draggable: 'false'});
  const art = h('button', {type: 'button', class: 'y-pb-art', 'aria-label': t('shellOpenNowPlaying'), onclick: openNowPlaying}, icon('note', 'y-icon y-art-fallback'), img);
  const title = h('button', {type: 'button', class: 'y-pb-title', onclick: openNowPlaying});
  const artists = h('div', {class: 'y-pb-artists'});
  const adBadge = h('span', {class: 'y-badge', hidden: true, text: t('shellAdBadge')});
  const like = iconButton('heart', t('shellLike'), {class: 'y-icon-btn y-like', 'aria-pressed': 'false'});
  like.addEventListener('click', () => { if (bridge.like('like')) pulse(like, 'is-pop'); });

  const shuffle = iconButton('shuffle', t('shellShuffle'));
  shuffle.addEventListener('click', () => { if (bridge.shuffle()) { pulse(shuffle, 'is-spin'); toast(t('shellShuffled')); } });
  const previous = iconButton('previous', t('shellPrevious'), {onclick: () => bridge.previous()});
  const play = h('button', {type: 'button', class: 'y-play', 'aria-label': t('shellPlay'), onclick: () => bridge.toggle()},
    icon('play', 'y-icon y-play-glyph'), icon('pause', 'y-icon y-pause-glyph'));
  const next = iconButton('next', t('shellNext'), {onclick: () => bridge.next()});
  const repeat = iconButton('repeat', t('shellRepeatOff'), {class: 'y-icon-btn y-toggle', onclick: () => bridge.cycleRepeat()});

  const elapsed = h('span', {class: 'y-time', text: '0:00'});
  const total = h('span', {class: 'y-time', text: '0:00'});
  const progress = createSlider({
    label: t('shellProgress'), className: 'y-progress', step: 5, largeStep: 30, format: formatTime,
    onPreview: value => setText(elapsed, formatTime(value)),
    onCommit: value => { bridge.seek(value); kickProgress(); },
  });

  const lyrics = iconButton('lyrics', t('shellLyrics'), {class: 'y-icon-btn y-toggle', 'aria-pressed': 'false', onclick: () => showPanel(1)});
  const queue = iconButton('queue', t('shellQueue'), {class: 'y-icon-btn y-toggle', 'aria-pressed': 'false', onclick: () => showPanel(0)});
  const mute = iconButton('volume', t('shellMute'), {onclick: () => bridge.toggleMute()});
  const volume = createSlider({
    label: t('shellVolume'), className: 'y-volume', step: .05, largeStep: .2, format: v => percentFormat.format(v),
    onPreview: v => bridge.setVolume(v), onCommit: v => bridge.setVolume(v),
  });
  volume.el.addEventListener('wheel', event => {
    event.preventDefault();
    bridge.setVolume(store.state.volume + (event.deltaY < 0 ? .05 : -.05));
  }, {passive: false});
  const more = iconButton('more', t('shellMore'), {class: 'y-icon-btn y-pb-more'});
  more.addEventListener('click', () => bridge.trackMenu(more));
  const expand = iconButton('expand', t('shellImmersive'), {class: 'y-icon-btn y-toggle', 'aria-pressed': 'false', onclick: () => setImmersive(!store.state.immersive)});

  const meta = h('div', {class: 'y-pb-meta'}, h('div', {class: 'y-pb-line'}, title, adBadge), artists);
  const bar = localized(h('section', {id: 'ytmgo-playerbar', 'aria-label': t('shellPlayerControls')},
    h('div', {class: 'y-pb-now'}, art, meta, like),
    h('div', {class: 'y-pb-center'},
      h('div', {class: 'y-pb-transport'}, shuffle, previous, play, next, repeat),
      h('div', {class: 'y-pb-progress'}, elapsed, progress.el, total)),
    h('div', {class: 'y-pb-side'}, lyrics, queue, h('div', {class: 'y-pb-volume'}, mute, volume.el), more, expand)));

  // Progress runs on rAF only while playing and visible; idle otherwise.
  let frame = 0;
  const tick = () => {
    frame = 0;
    const video = bridge.video();
    const time = video?.currentTime || 0;
    progress.set(time, store.state.duration);
    if (!progress.dragging) setText(elapsed, formatTime(time));
    if (store.state.playing && !document.hidden) frame = requestAnimationFrame(tick);
  };
  kickProgress = () => { if (!frame) frame = requestAnimationFrame(tick); };
  document.addEventListener('visibilitychange', kickProgress);

  store.watch(['active'], ({active}) => {
    bar.classList.toggle('is-idle', !active);
    for (const control of [shuffle, previous, play, next, repeat, like, more, expand]) setAttr(control, 'aria-disabled', active ? null : 'true');
    if (!active) { setText(title, t('shellIdleTitle')); artists.replaceChildren(h('span', {text: t('shellIdleSubtitle')})); }
  });
  let lastTitle = '';
  store.watch(['title', 'artists', 'active'], ({title: name, artists: list, active}) => {
    if (!active) return;
    setText(title, name || t('shellLoading'));
    setAttr(title, 'title', name);
    artists.replaceChildren(...artistLinks(list));
    if (name && name !== lastTitle) {
      if (lastTitle) pulse(meta, 'is-entering');
      lastTitle = name;
      announce(list.length
        ? t('shellAnnounceTrackBy', {title: name, artists: listFormat.format(list.map(a => a.name))})
        : t('shellAnnounceTrack', {title: name}));
    }
  });
  store.watch(['art'], ({art: src}) => {
    if (img.getAttribute('src') === src) return;
    art.classList.remove('has-art');
    if (!src) { img.removeAttribute('src'); return; }
    img.onload = () => art.classList.add('has-art');
    img.src = src;
  });
  store.watch(['playing', 'buffering'], ({playing, buffering}) => {
    play.dataset.state = playing ? 'playing' : 'paused';
    play.classList.toggle('is-buffering', buffering);
    setAttr(play, 'aria-label', t(playing ? 'shellPause' : 'shellPlay'));
    setAttr(play, 'data-tip', t(playing ? 'shellPause' : 'shellPlay'));
    kickProgress();
  });
  store.watch(['duration'], ({duration}) => { setText(total, formatTime(duration)); kickProgress(); });
  store.watch(['ad'], ({ad}) => { adBadge.hidden = !ad; bar.classList.toggle('is-ad', ad); });
  store.watch(['like'], ({like: status}) => {
    const liked = status === 'LIKE';
    swapIcon(like, liked ? 'heart-fill' : 'heart');
    setAttr(like, 'aria-pressed', String(liked));
    setAttr(like, 'aria-label', t(liked ? 'shellUnlike' : 'shellLike'));
    setAttr(like, 'data-tip', t(liked ? 'shellUnlike' : 'shellLike'));
  });
  let lastRepeat = null;
  store.watch(['repeat'], ({repeat: mode}) => {
    const label = t({ALL: 'shellRepeatAll', ONE: 'shellRepeatOne'}[mode] || 'shellRepeatOff');
    swapIcon(repeat, mode === 'ONE' ? 'repeat-one' : 'repeat');
    setAttr(repeat, 'aria-pressed', String(mode !== 'NONE'));
    setAttr(repeat, 'aria-label', label);
    setAttr(repeat, 'data-tip', label);
    if (lastRepeat !== null && lastRepeat !== mode) toast(label);
    lastRepeat = mode;
  });
  store.watch(['volume', 'muted'], ({volume: level, muted}) => {
    const silent = muted || level === 0;
    swapIcon(mute, silent ? 'volume-off' : level < .5 ? 'volume-low' : 'volume');
    setAttr(mute, 'aria-label', t(silent ? 'shellUnmute' : 'shellMute'));
    setAttr(mute, 'data-tip', t(silent ? 'shellUnmute' : 'shellMute'));
    volume.set(silent ? 0 : level, 1);
  });
  store.watch(['panel', 'panelTab', 'immersive'], ({panel, panelTab, immersive}) => {
    const open = panel || immersive;
    setAttr(queue, 'aria-pressed', String(open && panelTab === 0));
    setAttr(lyrics, 'aria-pressed', String(open && panelTab === 1));
    setAttr(expand, 'aria-pressed', String(immersive));
    swapIcon(expand, immersive ? 'collapse' : 'expand');
    setAttr(expand, 'aria-label', t(immersive ? 'shellExitImmersive' : 'shellImmersive'));
    setAttr(expand, 'data-tip', t(immersive ? 'shellExitImmersive' : 'shellImmersive'));
  });

  document.body.append(bar);
}
