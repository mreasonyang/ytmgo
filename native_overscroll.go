package main

import (
	"math"
	"time"

	"github.com/egoist/mygo/ui"
)

// Scrolling with a touchpad past either end stretches the content, as
// Safari and every macOS scroll view do: it follows the fingers less and
// less far (ui.Rubberband), a fling reaching the end overshoots a little,
// and either springs back once the touchpad lets go. A wheel stops at the
// end, as Safari's does; MyGo glides its notches, and the system's
// momentum carries a fling. Reduced motion stops at the end too.

// nativeOverscroll is a scroller's stretch past its ends.
type nativeOverscroll struct {
	raw    float32   // how far the touchpad pulled past the end: < 0 above the top, > 0 below the bottom
	last   time.Time // the last scroll event that stretched it
	from   float32   // the stretch shown as the spring back began
	settle time.Time // when it began; zero while held
}

const (
	nativeOverscrollIdle   = 64 * time.Millisecond  // no events this long: the touchpad let go
	nativeOverscrollSpring = 90 * time.Millisecond  // the spring back's time constant
	nativeMomentumStretch  = 0.35                   // a fling stretches this much of its travel
	nativeOverscrollRest   = 400 * time.Millisecond // the spring back is done by then
)

// nativeBand is the stretch shown for a pull of raw past the end of a
// view dimension tall, and nativeUnband its inverse.
func nativeBand(raw, dimension float32) float32 { return ui.Rubberband(raw, 0, 0, dimension) }

func nativeUnband(shown, dimension float32) float32 {
	const c = 0.55
	if dimension <= 0 || shown == 0 {
		return 0
	}
	sign := float32(1)
	if shown < 0 {
		sign, shown = -1, -shown
	}
	shown = min(shown, dimension*0.999)
	return sign * dimension / c * (1/(1-shown/dimension) - 1)
}

// nativeRubberband gives scroll (tracked by st) its stretch past the ends.
func nativeRubberband(c *ui.Context, scroll ui.Element, st *ui.ScrollState) {
	o := ui.Local(scroll, "overscroll", func() nativeOverscroll { return nativeOverscroll{} })
	h := scroll.Bounds().H
	now := c.Now()
	// The stretch shown now, springing back once the touchpad let go.
	shown := nativeBand(o.raw, h)
	if o.raw != 0 {
		if !o.settle.IsZero() {
			t := float64(now.Sub(o.settle)) / float64(nativeOverscrollSpring)
			shown = o.from * float32((1+t)*math.Exp(-t)) // critically damped
			if now.Sub(o.settle) >= nativeOverscrollRest || math.Abs(float64(shown)) < 0.25 {
				*o, shown = nativeOverscroll{}, 0
			} else {
				c.AnimationFrame()
			}
		} else if idle := now.Sub(o.last); idle >= nativeOverscrollIdle {
			o.from, o.settle = shown, now
			c.AnimationFrame()
		} else {
			c.After(nativeOverscrollIdle - idle)
		}
	}
	if shown != 0 {
		scroll.Translate(0, -shown)
	}
	if c.Preferences().ReduceMotion {
		*o = nativeOverscroll{}
		return
	}
	scroll.HandleInput(func(ev ui.InputEvent) bool {
		if ev.Kind != ui.InputScroll || !ev.Precise || ev.DY == 0 || math.Abs(float64(ev.DX)) > math.Abs(float64(ev.DY)) {
			return false
		}
		dy := ev.DY
		if o.raw > 0 && st.Y < st.MaxY-0.5 || o.raw < 0 && st.Y > 0.5 {
			// The content grew under a stretch (the next page came in
			// below): there is room again, and the scroll goes on into it.
			*o = nativeOverscroll{}
		}
		if o.raw == 0 && !(dy < 0 && st.Y <= 0 || dy > 0 && st.Y >= st.MaxY) {
			return false // within the content: the container scrolls
		}
		if !o.settle.IsZero() {
			// Caught as it springs back: it goes on from where it shows.
			t := float64(ev.Time.Sub(o.settle)) / float64(nativeOverscrollSpring)
			o.raw = nativeUnband(o.from*float32((1+t)*math.Exp(-t)), h)
			o.settle = time.Time{}
		}
		if ev.Momentum && o.raw*dy > 0 {
			dy *= nativeMomentumStretch
		}
		next := o.raw + dy
		if o.raw != 0 && next*o.raw <= 0 {
			// Back past the end: the rest scrolls the content.
			*o = nativeOverscroll{}
			st.Y = max(0, min(st.MaxY, st.Y+next))
			return true
		}
		o.raw, o.last = next, ev.Time
		return true
	})
}
