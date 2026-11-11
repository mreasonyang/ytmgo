package main

import (
	"bytes"
	"os"
	"testing"

	"ytmgo/internal/designtokens"
)

// The app's colours and measures come from the design guidebook's tokens.
// Edit design/tokens.json and run go generate, never the generated file.
func TestDesignTokensAreGenerated(t *testing.T) {
	raw, err := os.ReadFile("design/tokens.json")
	if err != nil {
		t.Fatal(err)
	}
	want, err := designtokens.Generate(raw)
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile("native_tokens_gen.go")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatal("native_tokens_gen.go is stale: run go generate")
	}
}

// Corners take one of the guidebook's radii (§7), in 4 px steps; a capsule
// (pill) is half its height and the app icon's corner is a share of its size,
// so neither is a radius on the scale. The scale rises in nesting order.
func TestRadiusScale(t *testing.T) {
	allowed := map[float32]bool{8: true, 12: true, 16: true, 24: true, 36: true, 44: true}
	for name, r := range map[string]float32{"art": tokenRadiusArt, "control": tokenRadiusControl, "panel": tokenRadiusPanel, "sheet": tokenRadiusSheet} {
		if !allowed[r] {
			t.Errorf("radius %s is %g; the scale is 8, 12, 16, 24, 36, 44", name, r)
		}
	}
	if !(tokenRadiusArt < tokenRadiusControl && tokenRadiusControl < tokenRadiusPanel && tokenRadiusPanel < tokenRadiusSheet) {
		t.Error("each radius must exceed the one it holds: art < control < panel < sheet")
	}
}
