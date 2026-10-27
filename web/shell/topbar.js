// topbar — history, search and account sit together over the canvas. The
// service's own search box and account controls are re-hosted here by CSS;
// the bar turns into frosted glass once content scrolls beneath it.
let refreshHistory = () => {};

function mountTopbar() {
  const back = iconButton('back', t('shellBack'), {class: 'y-icon-btn y-round', onclick: () => history.back()});
  const forward = iconButton('forward', t('shellForward'), {class: 'y-icon-btn y-round y-forward', onclick: () => history.forward()});
  const settings = iconButton('settings', t('shellSettings'), {class: 'y-icon-btn y-round', 'aria-haspopup': 'dialog', 'aria-expanded': 'false', id: 'ytmgo-settings-button'});
  settings.addEventListener('click', () => openSettings(settings));

  const bar = localized(h('div', {id: 'ytmgo-topbar'},
    h('div', {class: 'y-topbar-glass', 'aria-hidden': 'true'}),
    h('div', {class: 'y-topbar-history'}, back, forward),
    h('div', {class: 'y-topbar-actions'}, settings)));

  refreshHistory = () => {
    const nav = window.navigation;
    setAttr(back, 'aria-disabled', nav && !nav.canGoBack ? 'true' : null);
    setAttr(forward, 'aria-disabled', nav && !nav.canGoForward ? 'true' : null);
  };
  addEventListener('popstate', () => refreshHistory());
  refreshHistory();

  store.watch(['scrolled'], ({scrolled}) => bar.classList.toggle('is-scrolled', scrolled));
  document.body.append(bar);
}
