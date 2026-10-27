// slider — shared scrubber for playback progress and volume.
// Paints with a single --p custom property (fill scaleX + knob translate),
// so per-frame updates stay on the compositor. Pointer drags preview
// locally and commit once; keyboard follows the WAI-ARIA slider pattern.
function createSlider({label, className = '', step, largeStep, format = String, onPreview, onCommit}) {
  const fill = h('div', {class: 'y-slider-fill'});
  const knob = h('div', {class: 'y-slider-knob'});
  const tip = h('div', {class: 'y-slider-tip', 'aria-hidden': 'true'});
  const el = h('div', {class: `y-slider ${className}`, role: 'slider', tabindex: '0', 'aria-label': label, 'aria-valuemin': '0'},
    h('div', {class: 'y-slider-track'}, fill), knob, tip);
  let max = 0, value = 0, dragging = false, spoken = '';

  new ResizeObserver(([entry]) => el.style.setProperty('--w', entry.contentRect.width + 'px')).observe(el);

  function paint(v) { el.style.setProperty('--p', max > 0 ? clamp(v / max, 0, 1).toFixed(4) : 0); }
  function speak(v) {
    const text = format(v);
    if (text === spoken) return;
    spoken = text;
    setAttr(el, 'aria-valuenow', Math.round(v));
    setAttr(el, 'aria-valuetext', max > 0 ? `${text} / ${format(max)}` : text);
  }
  function fromPointer(event) {
    const rect = el.getBoundingClientRect();
    return clamp((event.clientX - rect.left) / rect.width, 0, 1) * max;
  }
  function showTip(v, event) {
    const rect = el.getBoundingClientRect();
    tip.textContent = format(v);
    tip.style.setProperty('--x', clamp(event.clientX - rect.left, 0, rect.width) + 'px');
  }

  el.addEventListener('pointerdown', event => {
    if (event.button !== 0 || max <= 0) return;
    dragging = true;
    el.setPointerCapture(event.pointerId);
    el.classList.add('is-dragging');
    value = fromPointer(event);
    paint(value);
    showTip(value, event);
    onPreview?.(value);
  });
  el.addEventListener('pointermove', event => {
    if (max <= 0) return;
    const v = fromPointer(event);
    showTip(v, event);
    if (!dragging) return;
    value = v;
    paint(value);
    onPreview?.(value);
  });
  const finish = event => {
    if (!dragging) return;
    dragging = false;
    el.classList.remove('is-dragging');
    if (event.type === 'pointerup') onCommit(value);
  };
  el.addEventListener('pointerup', finish);
  el.addEventListener('pointercancel', finish);
  el.addEventListener('keydown', event => {
    if (max <= 0) return;
    const delta = {ArrowRight: step, ArrowUp: step, ArrowLeft: -step, ArrowDown: -step, PageUp: largeStep, PageDown: -largeStep}[event.key];
    let next;
    if (delta !== undefined) next = value + delta;
    else if (event.key === 'Home') next = 0;
    else if (event.key === 'End') next = max;
    else return;
    event.preventDefault();
    event.stopPropagation();
    value = clamp(next, 0, max);
    paint(value);
    speak(value);
    onCommit(value);
  });

  return {
    el,
    get dragging() { return dragging; },
    set(v, newMax = max) {
      if (newMax !== max) { max = newMax; setAttr(el, 'aria-valuemax', Math.round(max)); }
      setAttr(el, 'aria-disabled', max > 0 ? null : 'true');
      if (dragging) return;
      value = v;
      paint(v);
      speak(v);
    },
  };
}
