package main

import (
	"weak"

	"github.com/egoist/mygo/ui"
)

// The last painted content viewport, on the UI thread. Weak references identify
// the exact rendition the user saw without keeping its pixels alive after eviction.
const artworkReturnCoverLimit = 64

type artworkReturnCover struct {
	raw    string
	edge   int
	bitmap weak.Pointer[ui.Bitmap]
}

type artworkReturnView struct {
	generation uint64
	count      int
	covers     [artworkReturnCoverLimit]artworkReturnCover
}

type artworkReturnPaint struct {
	view       *artworkReturnView
	generation uint64
}

func (v *artworkReturnView) items() []artworkReturnCover { return v.covers[:v.count] }

func (v *artworkReturnView) reset() {
	v.generation++
	v.count = 0
	clear(v.covers[:])
}

func (v *artworkReturnView) begin(root ui.Element) {
	v.generation++
	pass := v.paintPass()
	// Keep the last displayed viewport during the build: a click or keyboard
	// shortcut can navigate before the new layout has actually been painted.
	root.Draw(func(*ui.Painter, ui.Rect) {
		if v.generation == pass.generation {
			v.count = 0
		}
	})
}

func (v *artworkReturnView) paintPass() artworkReturnPaint {
	return artworkReturnPaint{view: v, generation: v.generation}
}

func (p artworkReturnPaint) record(raw string, edge int, shown ui.ImageSource) {
	v := p.view
	if v == nil || p.generation != v.generation || v.count == len(v.covers) {
		return
	}
	bitmap, ok := shown.(*ui.Bitmap)
	if !ok || bitmap == nil {
		return
	}
	edge = artworkBucket(edge)
	for _, cover := range v.items() {
		if cover.raw == raw && cover.edge == edge && cover.bitmap.Value() == bitmap {
			return
		}
	}
	// Stable views usually paint the same pictures in the same order.
	slot := &v.covers[v.count]
	if slot.raw != raw || slot.edge != edge || slot.bitmap.Value() != bitmap {
		*slot = artworkReturnCover{raw: raw, edge: edge, bitmap: weak.Make(bitmap)}
	}
	v.count++
}
