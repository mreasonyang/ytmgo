package main

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestPhrasesJoinWithinMarksOnly(t *testing.T) {
	if got := phrases("音乐库为空"); got != "音乐库为空" {
		t.Fatalf("an unmarked string changed: %q", got)
	}
	got := phrases("你的|音乐库")
	if strings.Contains(got, "|") {
		t.Fatalf("a mark survived: %q", got)
	}
	if want := "你⁠的音⁠乐⁠库"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	// Spaces stay break opportunities.
	if got := phrases("Premium でも、|そうでなくても"); strings.Contains(got, "⁠ ") || strings.Contains(got, " ⁠") {
		t.Fatalf("a space was joined: %q", got)
	}
}

// MyGo's line breaker must honour the joiners: a marked heading breaks at
// its marks, an unmarked one inside words.
func TestPhrasesBreakAtMarksInLayout(t *testing.T) {
	marked := phrases("你的音乐|值得|不止一个|浏览器标签页")
	font := ui.Font{Size: 32, Weight: 800, Lang: "zh-Hans"}
	plain := "你的音乐值得不止一个浏览器标签页"
	// Font metrics vary across systems. Find a width where the unmarked text
	// breaks inside a phrase, and verify that joiners prevent that break.
	boundaries := map[int]bool{len([]rune("你的音乐")): true, len([]rune("你的音乐值得")): true, len([]rune("你的音乐值得不止一个")): true}
	breaksInsidePhrase := func(lines []string) bool {
		offset := 0
		for _, line := range lines[:len(lines)-1] {
			offset += len([]rune(line))
			if !boundaries[offset] {
				return true
			}
		}
		return false
	}
	for width := float32(120); width <= 360; width += 4 {
		fits := true
		for _, phrase := range []string{"你的音乐", "值得", "不止一个", "浏览器标签页"} {
			if len(shapedLines(ui.ShapeText(phrase, font, width), phrase)) != 1 {
				fits = false
				break
			}
		}
		if !fits {
			continue
		}
		plainLines := shapedLines(ui.ShapeText(plain, font, width), plain)
		markedLines := shapedLines(ui.ShapeText(marked, font, width), marked)
		if !breaksInsidePhrase(plainLines) {
			continue
		}
		if breaksInsidePhrase(markedLines) {
			t.Fatalf("marked text broke inside a phrase at width %.0f: %q", width, markedLines)
		}
		return
	}
	t.Fatal("could not find a width where unmarked text breaks inside a phrase")
}

// shapedLines rebuilds the lines of a layout, without the joiners.
func shapedLines(lay *ui.TextLayout, s string) []string {
	var lines []string
	var cur strings.Builder
	last := float32(-1)
	i := -1
	for _, r := range s {
		i++ // UTF-16 units: every rune here is in the BMP
		if r == wordJoiner {
			continue
		}
		y := lay.Caret(i).Y
		if last >= 0 && y > last+1 {
			lines = append(lines, cur.String())
			cur.Reset()
		}
		cur.WriteRune(r)
		last = y
	}
	return append(lines, cur.String())
}
