package main

import (
	"github.com/egoist/mygo/ui"
	"ytmgo/internal/catalog"
	"ytmgo/internal/downloads"
)

// A collection's songs can be chosen with the mouse, as in a file list:
// Cmd-click (Ctrl-click on Linux and Windows) adds a song or takes it out,
// Shift-click chooses from the last one clicked; a plain click plays, as
// before, and lets the choice go, as does Esc or leaving the page. The
// menu of a chosen song acts on them all.

// nativeTable is the track table a row belongs to: a collection's pages,
// in order, without copying them.
type nativeTable struct{ parts [][]catalog.Item }

func (t *nativeTable) each(fn func(i int, it catalog.Item)) {
	i := 0
	for _, items := range t.parts {
		for _, it := range items {
			fn(i, it)
			i++
		}
	}
}

// at is the song at row i, or none.
func (t *nativeTable) at(i int) catalog.Item {
	for _, items := range t.parts {
		if i < len(items) {
			if i < 0 {
				break
			}
			return items[i]
		}
		i -= len(items)
	}
	return catalog.Item{}
}

// nativeSelection is the songs chosen on one page, by ID.
type nativeSelection struct {
	route  string
	ids    map[string]bool
	anchor int // the row Shift extends from
}

func (s *nativeSelection) has(route, id string) bool {
	return s.route == route && s.ids[id]
}

func (s *nativeSelection) count(route string) int {
	if s.route != route {
		return 0
	}
	return len(s.ids)
}

func (s *nativeSelection) clear() { *s = nativeSelection{} }

func (s *nativeSelection) on(route string) {
	if s.route != route || s.ids == nil {
		*s = nativeSelection{route: route, ids: map[string]bool{}}
	}
}

// toggle adds the song at row i, or takes it out; Shift extends from it.
func (s *nativeSelection) toggle(route string, i int, id string) {
	s.on(route)
	if s.ids[id] {
		delete(s.ids, id)
	} else {
		s.ids[id] = true
	}
	s.anchor = i
}

// extend chooses the songs from the anchor to row i, as Shift-click does.
func (s *nativeSelection) extend(route string, t *nativeTable, i int) {
	fresh := s.route != route || len(s.ids) == 0
	s.on(route)
	if fresh {
		s.anchor = 0
	}
	from, to := min(s.anchor, i), max(s.anchor, i)
	t.each(func(j int, it catalog.Item) {
		if j >= from && j <= to && it.ID != "" {
			s.ids[it.ID] = true
		}
	})
}

// only chooses the song at row i alone, as a right-click outside the
// choice does before its menu opens.
func (s *nativeSelection) only(route string, i int, id string) {
	*s = nativeSelection{route: route, ids: map[string]bool{id: true}, anchor: i}
}

// chosen is the chosen songs, in the table's order.
func (s *nativeSelection) chosen(route string, t *nativeTable) []catalog.Item {
	var out []catalog.Item
	if s.route != route {
		return out
	}
	t.each(func(_ int, it catalog.Item) {
		if s.ids[it.ID] {
			out = append(out, it)
		}
	})
	return out
}

// nativeSelectionItems is the menu of a chosen song: what can be done to
// them all, headed by how many there are.
func nativeSelectionItems(items []catalog.Item, m *UIModel, a ViewActions) []nativeMenuItem {
	out := []nativeMenuItem{{label: a.TrArgs("nativeSelectedCount", map[string]any{"count": len(items)}), disabled: true}}
	if a.QueueAppend != nil {
		out = append(out, nativeMenuItem{label: a.Tr("nativeAddToQueue"), divider: true, run: func() {
			for _, it := range items {
				a.QueueAppend(it)
			}
			m.selection.clear()
		}})
	}
	if a.Download == nil {
		return out
	}
	var missing []catalog.Item
	var stored []string
	for _, it := range items {
		switch m.downloadStates[it.ID] {
		case "", downloads.Paused, downloads.Failed:
			missing = append(missing, it)
		}
		if m.downloadStates[it.ID] != "" {
			stored = append(stored, it.ID)
		}
	}
	if len(missing) > 0 {
		out = append(out, nativeMenuItem{label: a.Tr("nativeDownloadSongs"), run: func() {
			if a.DownloadMany != nil {
				a.DownloadMany(missing)
			} else {
				for _, it := range missing {
					a.Download(it)
				}
			}
			m.selection.clear()
		}})
	}
	if len(stored) > 0 && a.DownloadRemove != nil {
		out = append(out, nativeMenuItem{label: a.Tr("nativeDownloadRemoveSongs"), run: func() {
			for _, id := range stored {
				a.DownloadRemove(id)
			}
			m.selection.clear()
		}})
	}
	return out
}

// nativeSelectableTrack wires a table row into the choice: its look while
// chosen, its clicks with modifiers, and its menu. It reports whether the
// click was taken (a plain click is left to the row, which plays).
func nativeSelectableTrack(c *ui.Context, b ui.Element, item catalog.Item, index int, m *UIModel, a ViewActions, p nativePalette, clicked bool) bool {
	t, route := a.table, m.Route
	chosen := m.selection.has(route, item.ID)
	if chosen {
		// A run of chosen rows is one block: the corners between two of
		// them are square.
		r := float32(tokenRadiusControl)
		top, bottom := r, r
		if prev := t.at(index - 1); prev.ID != "" && m.selection.has(route, prev.ID) {
			top = 0
		}
		if next := t.at(index + 1); next.ID != "" && m.selection.has(route, next.ID) {
			bottom = 0
		}
		b.Selected(true).Background(p.selection).TextColor(p.text).Radius(top, top, bottom, bottom)
	}
	nativeContextMenu(c, b, m, p, func() []nativeMenuItem {
		if !m.selection.has(route, item.ID) {
			// Outside the choice, the menu is the song's own, and the
			// choice becomes it, as Finder does.
			m.selection.only(route, index, item.ID)
			return nativeTrackItems(item, m, a)
		}
		if m.selection.count(route) == 1 {
			return nativeTrackItems(item, m, a)
		}
		return nativeSelectionItems(m.selection.chosen(route, t), m, a)
	})
	if !clicked {
		return false
	}
	switch mods := b.ClickModifiers(); {
	case mods&ui.Cmd != 0:
		m.selection.toggle(route, index, item.ID)
	case mods&ui.Shift != 0:
		m.selection.extend(route, t, index)
	default:
		m.selection.clear()
		return false
	}
	return true
}
