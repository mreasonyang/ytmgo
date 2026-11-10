package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

// ytmGO uses the system font for all text: a display title is the system UI
// face at the display weight, drawn exactly as plain text of that weight.
func TestDisplayTitlesUseTheSystemFont(t *testing.T) {
	width := func(display bool) float32 {
		tt := ui.NewTester(func(c *ui.Context) {
			ui.Row(c).Children(func() {
				e := ui.Text(c, "Listen again").FontSize(tokenSizeH3)
				if display {
					nativeDisplay(e, "Listen again", tokenSizeH3)
				} else {
					e.FontWeight(tokenDisplayWeight)
				}
			})
		}, 800, 200)
		tt.Frame()
		r, _ := tt.Find("Listen again")
		return r.W
	}
	if a, b := width(true), width(false); a != b {
		t.Fatalf("a display title (%v) differs from system text of its weight (%v)", a, b)
	}
}

// No typeface is bundled or registered.
func TestNoBundledFonts(t *testing.T) {
	for _, pattern := range []string{"resources/**/*.ttf", "resources/**/*.otf", "resources/*/*.ttf", "resources/*/*.otf", "site/assets/**/*.woff2", "site/assets/*/*.woff2"} {
		if found, _ := filepath.Glob(pattern); len(found) > 0 {
			t.Errorf("bundled font files: %v", found)
		}
	}
	files, _ := filepath.Glob("*.go")
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		src, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(src), "RegisterFont(") {
			t.Errorf("%s registers a font", name)
		}
	}
}
