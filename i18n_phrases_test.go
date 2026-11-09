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
	lines := shapedLines(ui.ShapeText(marked, font, 260), marked)
	want := []string{"你的音乐值得", "不止一个", "浏览器标签页"}
	if strings.Join(lines, "/") != strings.Join(want, "/") {
		t.Fatalf("lines %q, want %q", lines, want)
	}
	plain := "你的音乐值得不止一个浏览器标签页"
	if got := shapedLines(ui.ShapeText(plain, font, 260), plain); strings.Join(got, "/") == strings.Join(want, "/") {
		t.Fatal("the same text breaks at the phrases without joiners, so this test proves nothing")
	}
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
