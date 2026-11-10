package main

import (
	"os"
	"strings"
	"testing"
)

func shippedTokens(t *testing.T) (*tokens, string) {
	t.Helper()
	tok, err := loadTokens("../../design/tokens.json")
	if err != nil {
		t.Fatal(err)
	}
	css, err := tok.css()
	if err != nil {
		t.Fatal(err)
	}
	return tok, css
}

func TestShippedStylesheetFollowsTokens(t *testing.T) {
	_, generated := shippedTokens(t)
	sheet, err := os.ReadFile("../assets/site.css")
	if err != nil {
		t.Fatal(err)
	}
	if err := lintCSS(string(sheet), generated); err != nil {
		t.Fatal(err)
	}
}

func TestLintCatchesDrift(t *testing.T) {
	_, generated := shippedTokens(t)
	for name, line := range map[string]string{
		"hex colour":        ".a { color: #ff0000; }",
		"rgb colour":        ".a { background: rgb(0 0 0 / .5); }",
		"literal radius":    ".a { border-radius: 6px; }",
		"literal font size": ".a { font-size: 14px; }",
		"literal font":      ".a { font: 700 14px/1 var(--sans); }",
		"redefined token":   ".a { --accent: red; }",
		"hand-made shadow":  ".a { box-shadow: 0 16px 40px var(--base); }",
	} {
		if lintCSS(line, generated) == nil {
			t.Errorf("%s: %q passed the lint", name, line)
		}
	}
}

func TestLintAllowsTokensAndMarkedArtwork(t *testing.T) {
	_, generated := shippedTokens(t)
	for _, line := range []string{
		".a { color: var(--text); border-radius: var(--r-control); font-size: var(--t-small); }",
		".a { border-radius: 50%; font-size: 0; }",
		".a { font: 600 var(--t-meta) var(--sans); }",
		".a { background: linear-gradient(#18143a, #f08a4b); } /* artwork: a cover */",
		".a { --p: 0%; }",
		".a { box-shadow: var(--shadow-popover), 0 0 0 1px var(--line); }",
		".a { box-shadow: inset 0 0 0 1px var(--line); }",
	} {
		if err := lintCSS(line, generated); err != nil {
			t.Errorf("%q: %v", line, err)
		}
	}
}

func TestTokenNamesAreUnique(t *testing.T) {
	tok, _ := shippedTokens(t)
	tok.Product["accent"] = 1 // would collide with the colour role --accent
	if _, err := tok.css(); err == nil || !strings.Contains(err.Error(), "--accent") {
		t.Fatalf("a duplicate token name was not reported: %v", err)
	}
}

func TestNotFoundPageUsesTokens(t *testing.T) {
	tok, _ := shippedTokens(t)
	style := notFoundStyle(tok)
	for _, want := range []string{tok.Color.Dark["base"], tok.Color.Dark["text"], tok.Tint["rust"]} {
		if !strings.Contains(style, want) {
			t.Errorf("404 style lacks %s", want)
		}
	}
}
