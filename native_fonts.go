package main

import (
	_ "embed"
	"log"

	"github.com/egoist/mygo/ui"
)

// nativeDisplayFont is the design guidebook's display face: Bricolage
// Grotesque, cut as a static instance at weight 720, width 88 % and the
// display optical size, Latin only (resources/fonts, SIL Open Font License
// 1.1, listed in THIRD_PARTY_NOTICES.md). Other scripts fall back to the
// system's own heavy weight, as the guidebook asks for CJK display text.
//
//go:embed resources/fonts/ytmgo-display.ttf
var nativeDisplayFont []byte

const nativeDisplayFamily = "ytmGO Display"

func init() {
	if err := ui.RegisterFont(nativeDisplayFont, nativeDisplayFamily); err != nil {
		// Display titles fall back to the system font; nothing else depends on it.
		log.Printf("display font unavailable: %v", err)
	}
}

// nativeDisplay styles a display title. Latin takes the display face with
// its tight tracking; CJK, which the face doesn't cover, takes the system's
// heavy weight without tracking, as the guidebook asks.
func nativeDisplay(t ui.Element, text string, size float32) ui.Element {
	t.Font(nativeDisplayFamily + ", system-ui").FontWeight(800)
	if !nativeHasCJK(text) {
		t.LetterSpacing(size * tokenDisplayTracking)
	}
	return t
}

// nativeDisplayMetrics is the same, for measuring a display title.
func nativeDisplayMetrics(size, lineHeight float32) ui.Font {
	return ui.Font{Family: nativeDisplayFamily + ", system-ui", Size: size, Weight: 800, LineHeight: lineHeight}
}

// nativeDisplayLeading is the guidebook's display leading for Latin, and
// the roomier leading CJK headings need.
func nativeDisplayLeading(text string) float32 {
	if nativeHasCJK(text) {
		return tokenDisplayLeadingCJK
	}
	return tokenDisplayLeading
}

func nativeHasCJK(s string) bool {
	for _, r := range s {
		switch {
		case r >= 0x2E80 && r <= 0x9FFF, // CJK radicals to unified ideographs, kana
			r >= 0xAC00 && r <= 0xD7AF, // Hangul syllables
			r >= 0xF900 && r <= 0xFAFF, // compatibility ideographs
			r >= 0xFF00 && r <= 0xFFEF: // full-width forms
			return true
		}
	}
	return false
}
