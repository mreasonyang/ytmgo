package main

import "github.com/egoist/mygo/ui"

// Display titles use the system font, as all of ytmGO's text does
// (design/README.md §5.1): the platform's own UI face at the display weight,
// with its own spacing and optical sizes. ytmGO ships no typefaces.

// nativeDisplay styles a display title.
func nativeDisplay(t ui.Element, text string, size float32) ui.Element {
	return t.FontWeight(tokenDisplayWeight)
}

// nativeDisplayMetrics is the same, for measuring a display title.
func nativeDisplayMetrics(size, lineHeight float32) ui.Font {
	return ui.Font{Size: size, Weight: tokenDisplayWeight, LineHeight: lineHeight}
}

// nativeDisplayLeading is the display leading for Latin, and the roomier
// leading CJK headings need.
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
