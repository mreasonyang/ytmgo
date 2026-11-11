package main

import (
	"math"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

// requireReadoutOverThumb checks a slider's value readout stands 8 above
// its thumb (the guidebook's tooltip offset), centred on it at fraction.
func requireReadoutOverThumb(t *testing.T, tt *ui.Tester, text, slider string, fraction float32) {
	t.Helper()
	tip, s := rect(t, tt, text), rect(t, tt, slider)
	thumbTop := s.Y + (nativeSliderHeight-nativeSliderThumb)/2
	if gap := thumbTop - (tip.Y + tip.H); math.Abs(float64(gap-8)) > 0.5 {
		t.Fatalf("%s readout %v stands %.1f off the thumb (top %.1f), want 8", text, tip, gap, thumbTop)
	}
	thumbX := s.X + nativeSliderThumb/2 + (s.W-nativeSliderThumb)*fraction
	if mid := tip.X + tip.W/2; math.Abs(float64(mid-thumbX)) > 0.5 {
		t.Fatalf("%s readout centred at %.1f, thumb at %.1f", text, mid, thumbX)
	}
}

func TestVolumeShortcutShowsThePercent(t *testing.T) {
	m, a := nativeViewFixture()
	tt := ui.NewTester(func(c *ui.Context) { nativeView(c, m, a) }, 1280, 800)
	tt.Frame()
	tt.Key(ui.Cmd, ui.KeyUp)
	if math.Abs(m.Volume-0.75) > 1e-9 {
		t.Fatalf("volume %v after the shortcut, want 0.75", m.Volume)
	}
	requireReadoutOverThumb(t, tt, "75%", "Volume", 0.75)
	tt.Advance(nativeReadoutLinger + time.Millisecond)
	tt.Frame()
	if tt.HasText("75%") {
		t.Fatal("the volume readout outstays its moment")
	}
}

func TestVolumeShortcutShowsThePercentOverMuteWithoutSlider(t *testing.T) {
	m, a := nativeViewFixture()
	tt := ui.NewTester(func(c *ui.Context) { nativeView(c, m, a) }, 900, 700)
	tt.Frame()
	tt.Key(ui.Cmd, ui.KeyDown)
	tip, button := rect(t, tt, "65%"), rect(t, tt, "Mute")
	if gap := button.Y - (tip.Y + tip.H); math.Abs(float64(gap-8)) > 0.5 {
		t.Fatalf("readout %v stands %.1f off the mute button %v, want 8", tip, gap, button)
	}
}

func TestSeekKeysShowTheTime(t *testing.T) {
	m, a := nativeViewFixture()
	tt := ui.NewTester(func(c *ui.Context) { nativeView(c, m, a) }, 1280, 800)
	tt.Frame()
	s := rect(t, tt, "Playback position")
	// Focus it with the pointer, then take the pointer away: the keys alone
	// show where they moved it.
	at := s.X + nativeSliderThumb/2 + (s.W-nativeSliderThumb)*float32(m.Position/m.Duration)
	tt.ClickAt(at, s.Y+s.H/2)
	tt.Move(10, 10)
	tt.Advance(nativeReadoutLinger + time.Millisecond)
	tt.Frame()
	before := m.Position
	tt.Key(0, ui.KeyRight)
	if m.Position <= before {
		t.Fatalf("Right left the position at %v (was %v)", m.Position, before)
	}
	// The time beside the bar reads the same, so count: the readout is a
	// second copy (its place is the volume's, by the same code).
	text := nativeTime(math.Round(m.Position))
	if n := countText(tt, text); n != 2 {
		t.Fatalf("%q shown %d times after Right, want the readout too", text, n)
	}
	tt.Advance(nativeReadoutLinger + time.Millisecond)
	tt.Frame()
	if n := countText(tt, text); n != 1 {
		t.Fatalf("%q shown %d times after the readout's moment, want 1", text, n)
	}
}

func countText(tt *ui.Tester, s string) int {
	n := 0
	for _, text := range tt.Texts() {
		if text == s {
			n++
		}
	}
	return n
}
