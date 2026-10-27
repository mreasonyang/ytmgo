// icons — one 24px grid, 1.8px strokes; `fill` glyphs mark active/primary states.
const ICONS = {
  home: {d: ['M15 21v-8a1 1 0 0 0-1-1h-4a1 1 0 0 0-1 1v8', 'M3 10a2 2 0 0 1 .7-1.53l7-6a2 2 0 0 1 2.6 0l7 6A2 2 0 0 1 21 10v9a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z']},
  'home-fill': {fill: true, d: ['M3 10a2 2 0 0 1 .7-1.53l7-6a2 2 0 0 1 2.6 0l7 6A2 2 0 0 1 21 10v9a2 2 0 0 1-2 2h-4v-7a1 1 0 0 0-1-1h-4a1 1 0 0 0-1 1v7H5a2 2 0 0 1-2-2z']},
  explore: {d: ['M12 22a10 10 0 1 0 0-20 10 10 0 0 0 0 20Z', 'm16.24 7.76-2.12 6.36-6.36 2.12 2.12-6.36z']},
  'explore-fill': {fill: true, evenodd: true, d: ['M12 2a10 10 0 1 1 0 20 10 10 0 0 1 0-20Zm4.24 5.76-6.36 2.12-2.12 6.36 6.36-2.12z']},
  library: {d: ['m16 6 4 14', 'M12 6v14', 'M8 8v12', 'M4 4v16']},
  'library-fill': {fill: true, d: ['M4 3h1.5A1.5 1.5 0 0 1 7 4.5v15A1.5 1.5 0 0 1 5.5 21H4a1.5 1.5 0 0 1-1.5-1.5v-15A1.5 1.5 0 0 1 4 3Zm5.5 4H11a1.5 1.5 0 0 1 1.5 1.5v11A1.5 1.5 0 0 1 11 21H9.5A1.5 1.5 0 0 1 8 19.5v-11A1.5 1.5 0 0 1 9.5 7Zm5.1-1.9 1.4-.4a1.5 1.5 0 0 1 1.85 1.05l3.6 13.3a1.5 1.5 0 0 1-1.05 1.85l-1.4.4a1.5 1.5 0 0 1-1.85-1.05L13.55 6.95A1.5 1.5 0 0 1 14.6 5.1Z']},
  search: {d: ['m21 21-4.35-4.35', 'M11 19a8 8 0 1 0 0-16 8 8 0 0 0 0 16Z']},
  back: {d: ['m15 18-6-6 6-6']},
  forward: {d: ['m9 18 6-6-6-6']},
  'chevron-down': {d: ['m6 9 6 6 6-6']},
  play: {fill: true, d: ['M8 5.14v13.72a1 1 0 0 0 1.52.85l11.04-6.86a1 1 0 0 0 0-1.7L9.52 4.29A1 1 0 0 0 8 5.14Z']},
  pause: {fill: true, d: ['M6.5 4.5A1.5 1.5 0 0 1 8 3h1.5A1.5 1.5 0 0 1 11 4.5v15A1.5 1.5 0 0 1 9.5 21H8a1.5 1.5 0 0 1-1.5-1.5Z', 'M13 4.5A1.5 1.5 0 0 1 14.5 3H16a1.5 1.5 0 0 1 1.5 1.5v15A1.5 1.5 0 0 1 16 21h-1.5a1.5 1.5 0 0 1-1.5-1.5Z']},
  next: {fill: true, d: ['M5 5.5v13a1 1 0 0 0 1.55.83L16 13.2V18a1 1 0 0 0 2 0V6a1 1 0 0 0-2 0v4.8L6.55 4.67A1 1 0 0 0 5 5.5Z']},
  previous: {fill: true, d: ['M19 5.5v13a1 1 0 0 1-1.55.83L8 13.2V18a1 1 0 0 1-2 0V6a1 1 0 0 1 2 0v4.8l9.45-6.13A1 1 0 0 1 19 5.5Z']},
  shuffle: {d: ['M16 3h5v5', 'M4 20 21 3', 'M21 16v5h-5', 'm15 15 6 6', 'M4 4l5 5']},
  repeat: {d: ['m17 2 4 4-4 4', 'M3 11v-1a4 4 0 0 1 4-4h14', 'm7 22-4-4 4-4', 'M21 13v1a4 4 0 0 1-4 4H3']},
  'repeat-one': {d: ['m17 2 4 4-4 4', 'M3 11v-1a4 4 0 0 1 4-4h14', 'm7 22-4-4 4-4', 'M21 13v1a4 4 0 0 1-4 4H3', 'M11 10h1v4']},
  heart: {d: ['M19 14c1.49-1.46 3-3.21 3-5.5A5.5 5.5 0 0 0 16.5 3c-1.76 0-3 .5-4.5 2-1.5-1.5-2.74-2-4.5-2A5.5 5.5 0 0 0 2 8.5c0 2.3 1.5 4.05 3 5.5l7 7Z']},
  'heart-fill': {fill: true, d: ['M19 14c1.49-1.46 3-3.21 3-5.5A5.5 5.5 0 0 0 16.5 3c-1.76 0-3 .5-4.5 2-1.5-1.5-2.74-2-4.5-2A5.5 5.5 0 0 0 2 8.5c0 2.3 1.5 4.05 3 5.5l7 7Z']},
  dislike: {d: ['M17 14V2', 'M9 18.12 10 14H4.17a2 2 0 0 1-1.92-2.56l2.33-8A2 2 0 0 1 6.5 2H20a2 2 0 0 1 2 2v8a2 2 0 0 1-2 2h-2.76a2 2 0 0 0-1.79 1.11L12 22a3.13 3.13 0 0 1-3-3.88Z']},
  queue: {d: ['M21 15V6', 'M18.5 18a2.5 2.5 0 1 0 0-5 2.5 2.5 0 0 0 0 5Z', 'M12 12H3', 'M16 6H3', 'M12 18H3']},
  lyrics: {d: ['M12 2a3 3 0 0 0-3 3v7a3 3 0 0 0 6 0V5a3 3 0 0 0-3-3Z', 'M19 10v2a7 7 0 0 1-14 0v-2', 'M12 19v3']},
  volume: {d: ['M11 4.7a.7.7 0 0 0-1.2-.5L6.4 7.6A1.4 1.4 0 0 1 5.4 8H3a1 1 0 0 0-1 1v6a1 1 0 0 0 1 1h2.4a1.4 1.4 0 0 1 1 .4l3.4 3.4a.7.7 0 0 0 1.2-.5Z', 'M16 9a5 5 0 0 1 0 6', 'M19.4 18.4a9 9 0 0 0 0-12.8']},
  'volume-low': {d: ['M11 4.7a.7.7 0 0 0-1.2-.5L6.4 7.6A1.4 1.4 0 0 1 5.4 8H3a1 1 0 0 0-1 1v6a1 1 0 0 0 1 1h2.4a1.4 1.4 0 0 1 1 .4l3.4 3.4a.7.7 0 0 0 1.2-.5Z', 'M16 9a5 5 0 0 1 0 6']},
  'volume-off': {d: ['M11 4.7a.7.7 0 0 0-1.2-.5L6.4 7.6A1.4 1.4 0 0 1 5.4 8H3a1 1 0 0 0-1 1v6a1 1 0 0 0 1 1h2.4a1.4 1.4 0 0 1 1 .4l3.4 3.4a.7.7 0 0 0 1.2-.5Z', 'm22 9-6 6', 'm16 9 6 6']},
  expand: {d: ['M15 3h6v6', 'M9 21H3v-6', 'm21 3-7 7', 'm3 21 7-7']},
  collapse: {d: ['M4 14h6v6', 'M20 10h-6V4', 'm14 10 7-7', 'm3 21 7-7']},
  close: {d: ['M18 6 6 18', 'm6 6 12 12']},
  more: {fill: true, d: ['M6.5 12a1.5 1.5 0 1 1-3 0 1.5 1.5 0 0 1 3 0Z', 'M13.5 12a1.5 1.5 0 1 1-3 0 1.5 1.5 0 0 1 3 0Z', 'M20.5 12a1.5 1.5 0 1 1-3 0 1.5 1.5 0 0 1 3 0Z']},
  settings: {d: ['M20 7h-9', 'M14 17H5', 'M17 20a3 3 0 1 0 0-6 3 3 0 0 0 0 6Z', 'M7 10a3 3 0 1 0 0-6 3 3 0 0 0 0 6Z']},
  panel: {d: ['M5 3h14a2 2 0 0 1 2 2v14a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2Z', 'M15 3v18']},
  sidebar: {d: ['M5 3h14a2 2 0 0 1 2 2v14a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2Z', 'M9 3v18']},
  sun: {d: ['M12 16a4 4 0 1 0 0-8 4 4 0 0 0 0 8Z', 'M12 2v2', 'M12 20v2', 'm4.93 4.93 1.41 1.41', 'm17.66 17.66 1.41 1.41', 'M2 12h2', 'M20 12h2', 'm6.34 17.66-1.41 1.41', 'm19.07 4.93-1.41 1.41']},
  moon: {d: ['M12 3a6 6 0 0 0 9 9 9 9 0 1 1-9-9Z']},
  monitor: {d: ['M4 3h16a2 2 0 0 1 2 2v10a2 2 0 0 1-2 2H4a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2Z', 'M8 21h8', 'M12 17v4']},
  keyboard: {d: ['M4 5h16a2 2 0 0 1 2 2v10a2 2 0 0 1-2 2H4a2 2 0 0 1-2-2V7a2 2 0 0 1 2-2Z', 'M6 9h.01', 'M10 9h.01', 'M14 9h.01', 'M18 9h.01', 'M8 15h8']},
  logout: {d: ['M9 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h4', 'm16 17 5-5-5-5', 'M21 12H9']},
  user: {d: ['M19 21v-2a4 4 0 0 0-4-4H9a4 4 0 0 0-4 4v2', 'M12 11a4 4 0 1 0 0-8 4 4 0 0 0 0 8Z']},
  gear: {d: ['M12 15a3 3 0 1 0 0-6 3 3 0 0 0 0 6Z', 'M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 1 1-2.83 2.83l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 1 1-4 0v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 1 1-2.83-2.83l.06-.06A1.65 1.65 0 0 0 4.68 15a1.65 1.65 0 0 0-1.51-1H3a2 2 0 1 1 0-4h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 1 1 2.83-2.83l.06.06A1.65 1.65 0 0 0 9 4.68a1.65 1.65 0 0 0 1-1.51V3a2 2 0 1 1 4 0v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 1 1 2.83 2.83l-.06.06A1.65 1.65 0 0 0 19.4 9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 1 1 0 4h-.09a1.65 1.65 0 0 0-1.51 1Z']},
  note: {d: ['M9 18V5l12-2v13', 'M6 21a3 3 0 1 0 0-6 3 3 0 0 0 0 6Z', 'M18 19a3 3 0 1 0 0-6 3 3 0 0 0 0 6Z']},
};

function icon(name, className = 'y-icon') {
  const spec = ICONS[name];
  const svg = document.createElementNS(SVG_NS, 'svg');
  svg.setAttribute('viewBox', '0 0 24 24');
  svg.setAttribute('aria-hidden', 'true');
  svg.setAttribute('focusable', 'false');
  svg.setAttribute('class', className);
  svg.dataset.icon = name;
  if (spec.fill) svg.setAttribute('fill', 'currentColor');
  else {
    for (const [key, value] of [['fill', 'none'], ['stroke', 'currentColor'], ['stroke-width', '1.8'], ['stroke-linecap', 'round'], ['stroke-linejoin', 'round']]) svg.setAttribute(key, value);
  }
  for (const d of spec.d) {
    const path = document.createElementNS(SVG_NS, 'path');
    path.setAttribute('d', d);
    if (spec.evenodd) path.setAttribute('fill-rule', 'evenodd');
    svg.append(path);
  }
  return svg;
}

// Swap a button's glyph in place (play↔pause, repeat modes) without
// rebuilding the button, so focus and hover state survive.
function swapIcon(button, name) {
  const current = button.querySelector('svg.y-icon');
  if (current?.dataset.icon === name) return;
  const next = icon(name);
  if (current) current.replaceWith(next); else button.prepend(next);
}

// Icon-only control; the label is both the accessible name and the tooltip.
function iconButton(name, label, props = {}) {
  return h('button', {type: 'button', class: 'y-icon-btn', 'aria-label': label, 'data-tip': label, ...props}, icon(name));
}
