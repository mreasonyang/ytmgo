package main

import (
	"math"

	"github.com/egoist/mygo/ui"
	"ytmgo/internal/catalog"
	"ytmgo/internal/downloads"
)

// Songs in a table (an album's or playlist's, search results, the queue)
// can be chosen with the mouse, as in a file list: Cmd-click (Ctrl-click on
// Linux and Windows) adds a song or takes it out, Shift-click chooses from
// the last one clicked, a drag across rows chooses them (adding to the
// choice with Cmd held), and Cmd+A chooses the page's songs. A plain click
// plays, as before, and lets the choice go, as do Esc and leaving the
// page. The menu of a chosen song acts on them all.

// nativeQueueScope is the queue's table, which is no page's.
const nativeQueueScope = "queue"

// nativeTable is the track table a row belongs to: its songs in order,
// without copying them, and the scope its choice belongs to (a page's
// route, or the queue).
type nativeTable struct {
	scope string
	parts [][]catalog.Item
}

func (t *nativeTable) each(fn func(i int, it catalog.Item)) {
	i := 0
	for _, items := range t.parts {
		for _, it := range items {
			fn(i, it)
			i++
		}
	}
}

func (t *nativeTable) len() int {
	n := 0
	for _, items := range t.parts {
		n += len(items)
	}
	return n
}

// at is the song at row i, or none.
func (t *nativeTable) at(i int) catalog.Item {
	if i < 0 {
		return catalog.Item{}
	}
	for _, items := range t.parts {
		if i < len(items) {
			return items[i]
		}
		i -= len(items)
	}
	return catalog.Item{}
}

func (t *nativeTable) index(id string) int {
	found := -1
	t.each(func(i int, it catalog.Item) {
		if found < 0 && it.ID == id {
			found = i
		}
	})
	return found
}

// nativeFrameTables are the page's song tables the last frame built, for
// Cmd+A (nativeSelectAll).
var nativeFrameTables []*nativeTable

// nativeSelection is the songs chosen in one scope, by ID.
type nativeSelection struct {
	route  string // the scope: a page's route, or the queue
	ids    map[string]bool
	anchor string // the song Shift extends from
}

func (s *nativeSelection) has(scope, id string) bool {
	return s.route == scope && s.ids[id]
}

func (s *nativeSelection) count(scope string) int {
	if s.route != scope {
		return 0
	}
	return len(s.ids)
}

func (s *nativeSelection) clear() { *s = nativeSelection{} }

func (s *nativeSelection) on(scope string) {
	if s.route != scope || s.ids == nil {
		*s = nativeSelection{route: scope, ids: map[string]bool{}}
	}
}

// toggle adds the song, or takes it out; Shift extends from it.
func (s *nativeSelection) toggle(scope, id string) {
	s.on(scope)
	if s.ids[id] {
		delete(s.ids, id)
	} else {
		s.ids[id] = true
	}
	s.anchor = id
}

// extend chooses the songs from the anchor to row i, as Shift-click does;
// with no anchor in the table, from row i alone.
func (s *nativeSelection) extend(t *nativeTable, i int) {
	from := -1
	if s.route == t.scope && s.anchor != "" {
		from = t.index(s.anchor)
	}
	s.on(t.scope)
	if from < 0 {
		from = i
		s.anchor = t.at(i).ID
	}
	s.add(t, from, i)
}

// add chooses rows from to to of t, in either order.
func (s *nativeSelection) add(t *nativeTable, from, to int) {
	lo, hi := min(from, to), max(from, to)
	t.each(func(j int, it catalog.Item) {
		if j >= lo && j <= hi && it.ID != "" {
			s.ids[it.ID] = true
		}
	})
}

// only chooses the one song, as a right-click outside the choice does
// before its menu opens.
func (s *nativeSelection) only(scope, id string) {
	*s = nativeSelection{route: scope, ids: map[string]bool{id: true}, anchor: id}
}

// chosen is the chosen songs, in the table's order, each once.
func (s *nativeSelection) chosen(t *nativeTable) []catalog.Item {
	var out []catalog.Item
	if s.route != t.scope {
		return out
	}
	seen := map[string]bool{}
	t.each(func(_ int, it catalog.Item) {
		if s.ids[it.ID] && !seen[it.ID] {
			seen[it.ID] = true
			out = append(out, it)
		}
	})
	return out
}

// nativeSelectAll chooses every song of the page's tables, for Cmd+A.
func nativeSelectAll(m *UIModel, tables []*nativeTable) {
	m.selection = nativeSelection{route: m.Route, ids: map[string]bool{}}
	for _, t := range tables {
		if t.scope != m.Route {
			continue
		}
		t.each(func(_ int, it catalog.Item) {
			if it.ID != "" {
				m.selection.ids[it.ID] = true
			}
		})
	}
	if len(m.selection.ids) == 0 {
		m.selection.clear()
	}
}

// nativeSelectDrag is a press on a row dragging across others: the rows
// from it to the one under the pointer are chosen, besides those chosen
// before when Cmd was held as it began.
type nativeSelectDrag struct {
	held, on bool
	add      bool // Cmd was held: the rows join the choice
	scope    string
	from     int
	dy       float32
	base     map[string]bool
}

// nativeSelectionItems is the menu of a chosen song: what can be done to
// them all, headed by how many there are. In the queue, they can leave it.
func nativeSelectionItems(items []catalog.Item, scope string, m *UIModel, a ViewActions) []nativeMenuItem {
	out := []nativeMenuItem{{label: a.TrArgs("nativeSelectedCount", map[string]any{"count": len(items)}), disabled: true}}
	first := true
	add := func(it nativeMenuItem) {
		it.divider, first = first, false
		out = append(out, it)
	}
	if scope == nativeQueueScope {
		if a.QueueRemove != nil {
			ids := make([]string, len(items))
			for i, it := range items {
				ids[i] = it.ID
			}
			add(nativeMenuItem{label: a.Tr("nativeQueueRemove"), run: func() {
				a.QueueRemove(ids)
				m.selection.clear()
			}})
		}
	} else if a.QueueAppend != nil {
		add(nativeMenuItem{label: a.Tr("nativeAddToQueue"), run: func() {
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
		add(nativeMenuItem{label: a.Tr("nativeDownloadSongs"), run: func() {
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
		add(nativeMenuItem{label: a.Tr("nativeDownloadRemoveSongs"), run: func() {
			for _, id := range stored {
				a.DownloadRemove(id)
			}
			m.selection.clear()
		}})
	}
	return out
}

// nativeSelectableTrack wires a table row into the choice: its look while
// chosen, a drag from it, its clicks with modifiers, and its menu. It
// reports whether the click was taken (a plain click is left to the row).
func nativeSelectableTrack(c *ui.Context, b ui.Element, item catalog.Item, index int, m *UIModel, a ViewActions, p nativePalette, clicked bool) bool {
	t, scope := a.table, a.table.scope
	nativeSelectDragging(c, b, t, index, m)
	if chosen := m.selection.has(scope, item.ID); chosen {
		// A run of chosen rows is one block: the corners between two of
		// them are square.
		r := float32(tokenRadiusControl)
		if t.scope == nativeQueueScope {
			r = 0 // the queue's rows are flat bands
		}
		top, bottom := r, r
		if prev := t.at(index - 1); prev.ID != "" && m.selection.has(scope, prev.ID) {
			top = 0
		}
		if next := t.at(index + 1); next.ID != "" && m.selection.has(scope, next.ID) {
			bottom = 0
		}
		b.Selected(true).Background(p.selection).TextColor(p.text).Radius(top, top, bottom, bottom)
	}
	// One song's menu is its own; in the queue it can also leave it.
	single := func() []nativeMenuItem {
		items := nativeTrackItems(item, m, a)
		if scope == nativeQueueScope && a.QueueRemove != nil {
			items = append(items, nativeMenuItem{label: a.Tr("nativeQueueRemove"), divider: true, run: func() {
				a.QueueRemove([]string{item.ID})
				m.selection.clear()
			}})
		}
		return items
	}
	nativeContextMenu(c, b, m, p, func() []nativeMenuItem {
		if !m.selection.has(scope, item.ID) {
			// Outside the choice, the menu is the song's own, and the
			// choice becomes it, as Finder does.
			m.selection.only(scope, item.ID)
			return single()
		}
		if items := m.selection.chosen(t); len(items) > 1 {
			return nativeSelectionItems(items, scope, m, a)
		}
		return single()
	})
	if !clicked {
		return false
	}
	if d := &m.selectDrag; d.on && d.scope == scope && d.from == index {
		// The release of a drag that chose rows is no click.
		d.on = false
		return true
	}
	switch mods := b.ClickModifiers(); {
	case mods&ui.Cmd != 0:
		m.selection.toggle(scope, item.ID)
	case mods&ui.Shift != 0:
		m.selection.extend(t, index)
	default:
		m.selection.clear()
		return false
	}
	return true
}

// nativeSelectDragging follows a press on row index across the table: past
// 4 of movement it chooses the rows from it to the one under the pointer.
// The rows are tokenProductRow high, one under another.
func nativeSelectDragging(c *ui.Context, b ui.Element, t *nativeTable, index int, m *UIModel) {
	d := &m.selectDrag
	mine := d.held && d.scope == t.scope && d.from == index
	pressed := b.Pressed()
	if pressed && !mine {
		*d = nativeSelectDrag{held: true, scope: t.scope, from: index}
		if m.selection.route == t.scope {
			// The choice before the drag, which a Cmd-drag adds to.
			d.base = make(map[string]bool, len(m.selection.ids))
			for id := range m.selection.ids {
				d.base[id] = true
			}
		}
		mine = true
	}
	if !mine {
		return
	}
	if pressed {
		// The pointer leaving the row brings it no frames: ask for them
		// while it is held, so the choice follows the pointer.
		c.AnimationFrame()
	} else {
		d.held = false
	}
	// With Cmd held, the drag adds to the choice it began with.
	if c.Modifiers()&ui.Cmd != 0 {
		d.add = true
	}
	// The build after the release still reads the press's last moves.
	_, dy, _ := b.Dragged()
	d.dy += dy
	if !d.on {
		if math.Abs(float64(d.dy)) < 4 {
			return
		}
		d.on = true
	}
	_, py, _ := b.PointerPosition()
	to := index + int(math.Floor(float64(py/tokenProductRow)))
	to = max(0, min(t.len()-1, to))
	m.selection = nativeSelection{route: t.scope, ids: map[string]bool{}, anchor: t.at(index).ID}
	if d.add {
		for id := range d.base {
			m.selection.ids[id] = true
		}
	}
	m.selection.add(t, index, to)
}
