package main

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
)

// tokens is the part of design/tokens.json the website uses. The guidebook
// (design/README.md) explains every value.
type tokens struct {
	Color struct {
		Dark map[string]string `json:"dark"`
	} `json:"color"`
	Tint map[string]string `json:"tint"`
	Font struct {
		Display struct {
			Weight   int     `json:"weight"`
			Width    int     `json:"width"`
			Tracking float64 `json:"tracking"`
			Leading  float64 `json:"leading"`
		} `json:"display"`
	} `json:"font"`
	Size    map[string]json.RawMessage `json:"size"`
	SizeCJK map[string]fluid           `json:"sizeCJK"`
	Radius  map[string]json.RawMessage `json:"radius"`
	Motion  struct {
		Fast  int     `json:"fast"`
		Base  int     `json:"base"`
		Slow  int     `json:"slow"`
		Ease  string  `json:"ease"`
		Press float64 `json:"press"`
		Grow  float64 `json:"grow"`
	} `json:"motion"`
	Product map[string]int `json:"product"`
	Brand   struct {
		Wrap     int            `json:"wrap"`
		WrapWide int            `json:"wrapWide"`
		Nav      int            `json:"nav"`
		Section  int            `json:"section"`
		Block    int            `json:"block"`
		Gap      int            `json:"gap"`
		Button   int            `json:"button"`
		Compact  map[string]int `json:"compact"`
	} `json:"brand"`
}

// fluid is a size that scales with the viewport between two bounds.
type fluid struct {
	Min float64 `json:"min"`
	VW  float64 `json:"vw"`
	Max float64 `json:"max"`
}

func (f fluid) css() string { return fmt.Sprintf("clamp(%gpx, %gvw, %gpx)", f.Min, f.VW, f.Max) }

func loadTokens(path string) (*tokens, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var t tokens
	if err := json.Unmarshal(raw, &t); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return &t, nil
}

// kebab turns a token name into a CSS custom property name: accentText → accent-text.
func kebab(s string) string {
	var b strings.Builder
	for i, r := range s {
		if r >= 'A' && r <= 'Z' {
			if i > 0 {
				b.WriteByte('-')
			}
			r += 'a' - 'A'
		}
		b.WriteRune(r)
	}
	return b.String()
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// css renders the tokens as custom properties, with the CJK and compact
// overrides and one class per header tint. It is prepended to site.css.
func (t *tokens) css() (string, error) {
	var b strings.Builder
	// Names must be unique within a block: a colour role and a motion token
	// once both came out as --press, and the later one silently won.
	seen := map[string]bool{}
	var dup []string
	prop := func(name, value string) {
		if seen[name] {
			dup = append(dup, "--"+name)
		}
		seen[name] = true
		fmt.Fprintf(&b, "  --%s: %s;\n", name, value)
	}
	block := func() { seen = map[string]bool{} }
	b.WriteString("/* Generated from design/tokens.json by site/gen. Edit the tokens, not this block. */\n:root {\n")
	for _, k := range sortedKeys(t.Color.Dark) {
		prop(kebab(k), t.Color.Dark[k])
	}
	rust, ok := t.Tint["rust"]
	if !ok {
		return "", fmt.Errorf("tokens: the default tint rust is missing")
	}
	prop("tint", rust)
	d := t.Font.Display
	prop("display-weight", fmt.Sprint(d.Weight))
	prop("display-width", fmt.Sprintf("%d%%", d.Width))
	prop("display-tracking", fmt.Sprintf("%gem", d.Tracking))
	prop("display-leading", fmt.Sprint(d.Leading))
	for _, k := range sortedKeys(t.Size) {
		var px float64
		if json.Unmarshal(t.Size[k], &px) == nil {
			prop("t-"+kebab(k), fmt.Sprintf("%gpx", px))
			continue
		}
		var f fluid
		if err := json.Unmarshal(t.Size[k], &f); err != nil {
			return "", fmt.Errorf("tokens: size %s: %w", k, err)
		}
		prop("t-"+kebab(k), f.css())
	}
	for _, k := range sortedKeys(t.Radius) {
		var px float64
		if json.Unmarshal(t.Radius[k], &px) == nil {
			prop("r-"+kebab(k), fmt.Sprintf("%gpx", px))
			continue
		}
		var s string
		if err := json.Unmarshal(t.Radius[k], &s); err != nil {
			return "", fmt.Errorf("tokens: radius %s: %w", k, err)
		}
		prop("r-"+kebab(k), s)
	}
	m := t.Motion
	prop("d-fast", fmt.Sprintf("%dms", m.Fast))
	prop("d-base", fmt.Sprintf("%dms", m.Base))
	prop("d-slow", fmt.Sprintf("%dms", m.Slow))
	prop("ease", m.Ease)
	prop("press-scale", fmt.Sprint(m.Press))
	prop("grow-scale", fmt.Sprint(m.Grow))
	// Replicas of the app use the product measures (the replica rule).
	for _, k := range sortedKeys(t.Product) {
		prop(kebab(k), fmt.Sprintf("%dpx", t.Product[k]))
	}
	br := t.Brand
	prop("wrap", fmt.Sprintf("%dpx", br.Wrap))
	prop("wrap-wide", fmt.Sprintf("%dpx", br.WrapWide))
	prop("nav", fmt.Sprintf("%dpx", br.Nav))
	prop("s-section", fmt.Sprintf("%dpx", br.Section))
	prop("s-block", fmt.Sprintf("%dpx", br.Block))
	prop("s-gap", fmt.Sprintf("%dpx", br.Gap))
	prop("button", fmt.Sprintf("%dpx", br.Button))
	b.WriteString("}\n")

	block()
	b.WriteString(":is(:lang(zh-Hans), :lang(zh-Hant), :lang(ja)) {\n")
	for _, k := range sortedKeys(t.SizeCJK) {
		prop("t-"+kebab(k), t.SizeCJK[k].css())
	}
	b.WriteString("}\n")

	// Compact windows: the brand spacing tightens and the player becomes
	// the app's mini player.
	block()
	b.WriteString("@media (max-width: 720px) {\n:root {\n")
	for _, k := range sortedKeys(br.Compact) {
		name := "s-" + kebab(k)
		if k == "body" || k == "lead" {
			name = "t-" + k
		}
		prop(name, fmt.Sprintf("%dpx", br.Compact[k]))
	}
	prop("player", fmt.Sprintf("%dpx", t.Product["miniPlayer"]))
	b.WriteString("}\n}\n")

	for _, k := range sortedKeys(t.Tint) {
		fmt.Fprintf(&b, ".tint-%s { --tint: %s; }\n", k, t.Tint[k])
	}
	if len(dup) > 0 {
		return "", fmt.Errorf("tokens: two tokens share a CSS name: %s", strings.Join(dup, ", "))
	}
	return b.String(), nil
}

var (
	hexColor     = regexp.MustCompile(`#[0-9a-fA-F]{3,8}\b`)
	rgbColor     = regexp.MustCompile(`\brgba?\(`)
	radiusDecl   = regexp.MustCompile(`border-radius:\s*([^;}]+)`)
	fontSizeDecl = regexp.MustCompile(`font-size:\s*([^;}]+)`)
	fontDecl     = regexp.MustCompile(`(?:^|[\s;{])font:\s*([^;}]+)`)
	customProp   = regexp.MustCompile(`(?:^|[\s;{])(--[a-z0-9-]+)\s*:`)
)

// lintCSS keeps site.css on the tokens: colours, radii and font sizes come
// from custom properties, and the stylesheet doesn't redefine a token. A
// line marked "artwork" may use literal colours, since it draws a cover.
func lintCSS(css string, generated string) error {
	defined := map[string]bool{}
	for _, m := range customProp.FindAllStringSubmatch(generated, -1) {
		defined[m[1]] = true
	}
	var problems []string
	for i, line := range strings.Split(css, "\n") {
		n := i + 1
		if !strings.Contains(line, "artwork") && (hexColor.MatchString(line) || rgbColor.MatchString(line)) {
			problems = append(problems, fmt.Sprintf("line %d: literal colour; use a colour role", n))
		}
		for _, m := range radiusDecl.FindAllStringSubmatch(line, -1) {
			if v := strings.TrimSpace(m[1]); !strings.HasPrefix(v, "var(--r-") && v != "50%" && v != "0" && v != "inherit" {
				problems = append(problems, fmt.Sprintf("line %d: border-radius %s; use a radius token", n, v))
			}
		}
		for _, m := range fontSizeDecl.FindAllStringSubmatch(line, -1) {
			if v := strings.TrimSpace(m[1]); !strings.HasPrefix(v, "var(--t-") && v != "0" && v != "inherit" {
				problems = append(problems, fmt.Sprintf("line %d: font-size %s; use a type token", n, v))
			}
		}
		for _, m := range fontDecl.FindAllStringSubmatch(line, -1) {
			if !strings.Contains(m[1], "var(--t-") {
				problems = append(problems, fmt.Sprintf("line %d: font shorthand without a type token", n))
			}
		}
		for _, m := range customProp.FindAllStringSubmatch(line, -1) {
			if defined[m[1]] {
				problems = append(problems, fmt.Sprintf("line %d: %s is a token; change design/tokens.json", n, m[1]))
			}
		}
	}
	if len(problems) > 0 {
		return fmt.Errorf("site.css strays from design/tokens.json:\n  %s", strings.Join(problems, "\n  "))
	}
	return nil
}
