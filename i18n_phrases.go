package main

import "strings"

// wordJoiner forbids a line break between the characters around it.
const wordJoiner = '⁠'

// phrases applies a catalog string's phrase marks (design/README.md §5.3).
// Chinese and Japanese break between any two characters, so a heading can
// split a word ("值 / 得"). A "|" in a message marks where a heading may
// break; phrases keeps each phrase whole by joining its characters with
// invisible word joiners, and drops the marks. Spaces stay break
// opportunities, and a phrase too wide for its line still breaks inside, as
// the website's headings do. Strings without marks are returned as they are.
func phrases(s string) string {
	if !strings.Contains(s, "|") {
		return s
	}
	var b strings.Builder
	for _, phrase := range strings.Split(s, "|") {
		prev := ' '
		for _, r := range phrase {
			if prev != ' ' && r != ' ' {
				b.WriteRune(wordJoiner)
			}
			b.WriteRune(r)
			prev = r
		}
	}
	return b.String()
}
