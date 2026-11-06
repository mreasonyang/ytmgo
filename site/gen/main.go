// Command gen builds the ytmGO website: one static page per language from
// site/template.html and site/locales/*.json, plus the shared assets.
//
//	go run ./site/gen -base https://example.org/
//
// Every locale must carry exactly the English keys, and the template must use
// every key, so a translation can be neither missing nor stale.
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"html/template"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

type locale struct {
	Code string // the app's locale and the file name under locales/
	Slug string // the page's directory, relative to the site root
	Lang string // BCP 47 tag for lang and hreflang
	Name string // the language's own name, for the switcher
}

var locales = []locale{
	{"en", "", "en", "English"},
	{"zh-Hans", "zh-hans/", "zh-Hans", "简体中文"},
	{"zh-Hant", "zh-hant/", "zh-Hant", "繁體中文"},
	{"ja", "ja/", "ja", "日本語"},
}

type page struct {
	Locale  locale
	Locales []locale
	Root    string // relative path from the page to the site root
	Base    string // absolute site URL, ending in a slash
	Year    int
	JS      map[string]string
}

func main() {
	src := flag.String("src", "site", "site source directory")
	out := flag.String("out", "site/dist", "output directory")
	base := flag.String("base", "https://mreasonyang.github.io/ytmgo/", "absolute URL the site is served from")
	flag.Parse()
	if !strings.HasSuffix(*base, "/") {
		*base += "/"
	}
	if err := build(*src, *out, *base); err != nil {
		log.Fatal(err)
	}
}

func build(src, out, base string) error {
	strs := map[string]map[string]string{}
	for _, l := range locales {
		raw, err := os.ReadFile(filepath.Join(src, "locales", l.Code+".json"))
		if err != nil {
			return err
		}
		m := map[string]string{}
		if err := json.Unmarshal(raw, &m); err != nil {
			return fmt.Errorf("%s: %w", l.Code, err)
		}
		strs[l.Code] = m
	}
	if err := checkKeys(strs); err != nil {
		return err
	}
	raw, err := os.ReadFile(filepath.Join(src, "template.html"))
	if err != nil {
		return err
	}
	if err := os.RemoveAll(out); err != nil {
		return err
	}
	used := map[string]bool{}
	for _, l := range locales {
		s := strs[l.Code]
		js := map[string]string{}
		for k, v := range s {
			if strings.HasPrefix(k, "js.") {
				js[strings.TrimPrefix(k, "js.")] = v
				used[k] = true
			}
		}
		t, err := template.New("page").Funcs(template.FuncMap{
			"t": func(key string) (string, error) {
				v, ok := s[key]
				if !ok {
					return "", fmt.Errorf("unknown key %q", key)
				}
				used[key] = true
				return v, nil
			},
		}).Parse(string(raw))
		if err != nil {
			return err
		}
		root := strings.Repeat("../", strings.Count(l.Slug, "/"))
		var buf bytes.Buffer
		if err := t.Execute(&buf, page{Locale: l, Locales: locales, Root: root, Base: base, Year: time.Now().Year(), JS: js}); err != nil {
			return fmt.Errorf("%s: %w", l.Code, err)
		}
		if err := write(filepath.Join(out, l.Slug, "index.html"), buf.Bytes()); err != nil {
			return err
		}
	}
	var unused []string
	for k := range strs["en"] {
		if !used[k] {
			unused = append(unused, k)
		}
	}
	if len(unused) > 0 {
		slices.Sort(unused)
		return fmt.Errorf("keys not used by the template: %s", strings.Join(unused, ", "))
	}
	if err := copyDir(filepath.Join(src, "assets"), filepath.Join(out, "assets")); err != nil {
		return err
	}
	return extras(out, base)
}

// checkKeys requires every locale to carry exactly the English keys.
func checkKeys(strs map[string]map[string]string) error {
	var errs []error
	for _, l := range locales[1:] {
		for k := range strs["en"] {
			if strings.TrimSpace(strs[l.Code][k]) == "" {
				errs = append(errs, fmt.Errorf("%s: missing %q", l.Code, k))
			}
		}
		for k := range strs[l.Code] {
			if _, ok := strs["en"][k]; !ok {
				errs = append(errs, fmt.Errorf("%s: unknown %q", l.Code, k))
			}
		}
	}
	return errors.Join(errs...)
}

// extras writes what a static host expects beside the pages.
func extras(out, base string) error {
	var sm strings.Builder
	sm.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	sm.WriteString(`<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9" xmlns:xhtml="http://www.w3.org/1999/xhtml">` + "\n")
	for _, l := range locales {
		fmt.Fprintf(&sm, "  <url>\n    <loc>%s%s</loc>\n", base, l.Slug)
		for _, alt := range locales {
			fmt.Fprintf(&sm, "    <xhtml:link rel=\"alternate\" hreflang=\"%s\" href=\"%s%s\"/>\n", alt.Lang, base, alt.Slug)
		}
		fmt.Fprintf(&sm, "    <xhtml:link rel=\"alternate\" hreflang=\"x-default\" href=\"%s\"/>\n  </url>\n", base)
	}
	sm.WriteString("</urlset>\n")
	files := map[string]string{
		"sitemap.xml": sm.String(),
		"robots.txt":  "User-agent: *\nAllow: /\nSitemap: " + base + "sitemap.xml\n",
		".nojekyll":   "",
	}
	var links []string
	for _, l := range locales {
		links = append(links, fmt.Sprintf(`<a href="%s%s" lang="%s">%s</a>`, base, l.Slug, l.Lang, l.Name))
	}
	files["404.html"] = `<!doctype html><html lang="en"><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>ytmGO — page not found</title>` +
		`<style>body{margin:0;min-height:100vh;display:grid;place-content:center;gap:16px;text-align:center;background:#0a0a0b;color:#fff;font:16px/1.5 system-ui,sans-serif}a{color:#ff7a6e}nav{display:flex;gap:16px;justify-content:center}</style>` +
		`<h1>404</h1><nav>` + strings.Join(links, "") + `</nav></html>` + "\n"
	for name, body := range files {
		if err := write(filepath.Join(out, name), []byte(body)); err != nil {
			return err
		}
	}
	return nil
}

func write(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func copyDir(from, to string) error {
	return filepath.WalkDir(from, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, err := filepath.Rel(from, path)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return write(filepath.Join(to, rel), data)
	})
}
