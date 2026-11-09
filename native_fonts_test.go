package main

import (
	"math"
	"testing"

	"github.com/egoist/mygo/ui"
	"golang.org/x/image/font/sfnt"
)

// The display face must be drawn from the embedded file. Its names are its
// own, so an installed Bricolage Grotesque can't stand in for it (one did:
// the Linux renders then showed a wider face than the app ships).
func TestDisplayFontHasItsOwnNames(t *testing.T) {
	f, err := sfnt.Parse(nativeDisplayFont)
	if err != nil {
		t.Fatal(err)
	}
	for id, want := range map[sfnt.NameID]string{sfnt.NameIDFamily: nativeDisplayFamily, sfnt.NameIDPostScript: "ytmGODisplay"} {
		if got, err := f.Name(nil, id); err != nil || got != want {
			t.Errorf("name %d is %q (%v), want %q", id, got, err, want)
		}
	}
}

func displayWidth(t *testing.T, text string, size float32, weight int) float32 {
	t.Helper()
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Row(c).Children(func() { ui.Text(c, text).FontSize(size).Font(nativeDisplayFamily).FontWeight(weight) })
	}, 1600, 240)
	tt.Frame()
	r, ok := tt.Find(text)
	if !ok {
		t.Fatalf("%q not drawn", text)
	}
	return r.W
}

// The embedded face has one weight, so the weight asked for can't change
// its width; a substituted variable face, or a synthesised bold, would.
func TestDisplayFontIsTheEmbeddedFace(t *testing.T) {
	if a, b := displayWidth(t, "Listen again", 24, 400), displayWidth(t, "Listen again", 24, 800); a != b {
		t.Fatalf("width depends on weight (%v vs %v): not the embedded face", a, b)
	}
}

// Optical size follows the heading's size (design/README.md §5.1): small
// headings get the open spacing of the small optical sizes. A face fixed at
// one optical size scales in proportion; this one must not.
func TestDisplayFontFollowsOpticalSize(t *testing.T) {
	small, large := displayWidth(t, "Listen again", 24, 800)/24, displayWidth(t, "Listen again", 72, 800)/72
	if math.Abs(float64(small-large))/float64(large) < 0.03 {
		t.Fatalf("width per size is %.3f at 24 and %.3f at 72: optical size isn't varying", small, large)
	}
}
