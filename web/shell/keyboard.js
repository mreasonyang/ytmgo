// keyboard — in-page shortcuts. Native menu accelerators call the same
// actions through window.ytmgoControls; runAction() de-duplicates toggles in
// case an engine delivers a key to both the menu and the document.
function isTyping(target) {
  return !!target?.closest?.('input, textarea, select, [contenteditable="true"]');
}

// Escape peels back exactly one layer, topmost first.
function dismissTopmost() {
  if (closeLayer()) return true;
  if (store.state.immersive) { setImmersive(false); return true; }
  if (store.state.panel && matchMedia('(max-width: 1180px)').matches) { closePanel(); return true; }
  return false;
}

function mountKeyboard() {
  document.addEventListener('keydown', event => {
    const typing = isTyping(event.target);
    const serviceDialog = !!$(SEL.dialog);
    const mod = (event.metaKey || event.ctrlKey) && !event.altKey;
    let action = null;

    if (event.key === 'Escape' && !typing && !serviceDialog) {
      if (dismissTopmost()) { event.preventDefault(); event.stopPropagation(); }
      return;
    }
    if (mod && !event.shiftKey) {
      action = {ArrowRight: 'next', ArrowLeft: 'previous', ArrowUp: 'volume-up', ArrowDown: 'volume-down', '[': 'back', ']': 'forward', '/': 'shortcuts', j: 'panel', b: 'sidebar'}[event.key];
      // Text fields and dialogs keep their own arrow/bracket editing keys.
      if (action && (typing || serviceDialog || activeLayer) && action !== 'shortcuts') action = null;
      if (event.key.toLowerCase() === 'k') action = 'search';
    } else if (mod && event.shiftKey && event.key.toLowerCase() === 'f') {
      action = 'immersive';
    } else if (!mod && !typing && !serviceDialog && event.key === '/') {
      action = 'search';
    }
    if (!action) return;
    event.preventDefault();
    event.stopPropagation();
    runAction(action);
  }, true);
}
