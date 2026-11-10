// Command gen builds the ytmGO website: one static page per language from
// site/template.html and site/locales/*.json, plus the shared assets.
//
//	go run ./site/gen
//
// The output is a static folder for Cloudflare Pages: _headers carries the
// security and cache headers, 404.html the not-found page.
//
// Every locale must carry exactly the English keys, and the template must use
// every key, so a translation can be neither missing nor stale.
//
// A "|" in a string marks a phrase boundary. Chinese and Japanese headings
// keep their phrases whole and break only there (or at spaces and
// punctuation): {{wbr "key"}} turns each mark into <wbr>, while {{t "key"}}
// and the strings passed to the script drop the marks.
package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"html"
	"html/template"
	"io/fs"
	"log"
	"math"
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
	Locale     locale
	Locales    []locale
	Root       string // relative path from the page to the site root
	Base       string // absolute site URL, ending in a slash
	Year       int
	JS         map[string]string
	ScriptHash string // changes the script URL whenever its contents change
	BootHash   string // the same for boot.js
	StyleHash  string // and for the stylesheet
	ThemeColor string // the base colour role, for the browser's chrome
}

func main() {
	src := flag.String("src", "site", "site source directory")
	out := flag.String("out", "site/dist", "output directory")
	base := flag.String("base", "https://ytmgo.kyun.si/", "absolute URL the site is served from")
	design := flag.String("design", "design/tokens.json", "design tokens (see design/README.md)")
	flag.Parse()
	if !strings.HasSuffix(*base, "/") {
		*base += "/"
	}
	if err := build(*src, *out, *base, *design); err != nil {
		log.Fatal(err)
	}
}

func build(src, out, base, design string) error {
	tok, err := loadTokens(design)
	if err != nil {
		return err
	}
	tokenCSS, err := tok.css()
	if err != nil {
		return err
	}
	sheet, err := os.ReadFile(filepath.Join(src, "assets", "site.css"))
	if err != nil {
		return err
	}
	if err := lintCSS(string(sheet), tokenCSS); err != nil {
		return err
	}
	style := []byte(tokenCSS + "\n" + string(sheet))
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
	hashes := map[string]string{}
	for _, name := range []string{"site.js", "boot.js"} {
		data, err := os.ReadFile(filepath.Join(src, "assets", name))
		if err != nil {
			return err
		}
		hashes[name] = fmt.Sprintf("%x", sha256.Sum256(data))[:12]
	}
	hashes["site.css"] = fmt.Sprintf("%x", sha256.Sum256(style))[:12]
	if err := os.RemoveAll(out); err != nil {
		return err
	}
	used := map[string]bool{}
	for _, l := range locales {
		s := strs[l.Code]
		js := map[string]string{}
		for k, v := range s {
			if strings.HasPrefix(k, "js.") {
				js[strings.TrimPrefix(k, "js.")] = plain(v)
				used[k] = true
			}
		}
		lookup := func(key string) (string, error) {
			v, ok := s[key]
			if !ok {
				return "", fmt.Errorf("unknown key %q", key)
			}
			used[key] = true
			return v, nil
		}
		t, err := template.New("page").Funcs(template.FuncMap{
			"t": func(key string) (string, error) {
				v, err := lookup(key)
				return plain(v), err
			},
			"wbr": func(key string) (template.HTML, error) {
				v, err := lookup(key)
				parts := strings.Split(v, "|")
				for i, p := range parts {
					parts[i] = html.EscapeString(p)
				}
				return template.HTML(strings.Join(parts, "<wbr>")), err
			},
		}).Parse(string(raw))
		if err != nil {
			return err
		}
		root := strings.Repeat("../", strings.Count(l.Slug, "/"))
		var buf bytes.Buffer
		if err := t.Execute(&buf, page{Locale: l, Locales: locales, Root: root, Base: base, Year: time.Now().Year(), JS: js, ScriptHash: hashes["site.js"], BootHash: hashes["boot.js"], StyleHash: hashes["site.css"], ThemeColor: tok.Color.Dark["base"]}); err != nil {
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
	if err := write(filepath.Join(out, "assets", "site.css"), style); err != nil {
		return err
	}
	return extras(out, base, notFoundStyle(tok))
}

// plain drops the phrase marks from a string.
func plain(s string) string { return strings.ReplaceAll(s, "|", "") }

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

// notFoundStyle is the 404 page's only style; _headers allows it by hash.
// It uses the same tokens as the site: the rust wash, the text roles and
// the pill buttons.
func notFoundStyle(t *tokens) string {
	c := t.Color.Dark
	return fmt.Sprintf(`body{margin:0;min-height:100vh;display:grid;place-content:center;justify-items:center;gap:20px;padding:24px;text-align:center;`+
		`background:linear-gradient(color-mix(in srgb,%[1]s %[6]g%%,%[2]s),%[2]s 60%%) %[2]s;color:%[3]s;font:16px/1.5 system-ui,sans-serif}`+
		`img{border-radius:%[4]s}h1{margin:0;font-size:56px;line-height:1}p{margin:0;color:%[5]s}`+
		`nav{display:flex;flex-wrap:wrap;gap:8px;justify-content:center}`+
		`a{padding:8px 16px;border-radius:999px;background:%[7]s;color:%[3]s;text-decoration:none;font-weight:600}a:hover{background:%[8]s}`,
		t.Tint["rust"], c["base"], c["text"], radiusString(t, "icon"), c["secondary"], math.Round(t.Wash.BrandTop*100), c["hover"], c["press"])
}

func radiusString(t *tokens, name string) string {
	var s string
	if json.Unmarshal(t.Radius[name], &s) == nil {
		return s
	}
	return string(t.Radius[name]) + "px"
}

// extras writes what Cloudflare Pages expects beside the pages.
func extras(out, base, notFound string) error {
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
		"_headers":    headers(notFound),
	}
	var links []string
	for _, l := range locales {
		// Root-relative, so preview deployments link within themselves.
		links = append(links, fmt.Sprintf(`<a href="/%s" lang="%s">%s</a>`, l.Slug, l.Lang, l.Name))
	}
	files["404.html"] = `<!doctype html><html lang="en"><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>ytmGO — page not found</title><link rel="icon" href="/assets/icon-32.png" sizes="32x32">` +
		`<style>` + notFound + `</style>` +
		`<img src="/assets/icon-180.png" alt="" width="88" height="88"><h1>404</h1>` +
		`<p>This page isn't here.<br><span lang="zh-Hans">找不到此页面。</span> <span lang="zh-Hant">找不到此頁面。</span> <span lang="ja">このページは見つかりません。</span></p>` +
		`<nav>` + strings.Join(links, "") + `</nav></html>` + "\n"
	for name, body := range files {
		if err := write(filepath.Join(out, name), []byte(body)); err != nil {
			return err
		}
	}
	return nil
}

// headers is Cloudflare's _headers file. The pages load only their own
// files; the one outside request is the GitHub API for release links. The
// inline JSON blocks are data, which script-src does not govern.
func headers(notFound string) string {
	sum := sha256.Sum256([]byte(notFound))
	csp := strings.Join([]string{
		"default-src 'self'",
		"img-src 'self' data:",
		"style-src 'self' 'sha256-" + base64.StdEncoding.EncodeToString(sum[:]) + "'",
		"connect-src 'self' https://api.github.com",
		"object-src 'none'",
		"base-uri 'self'",
		"form-action 'none'",
		"frame-ancestors 'none'",
	}, "; ")
	return "/*\n" +
		"  Content-Security-Policy: " + csp + "\n" +
		"  X-Content-Type-Options: nosniff\n" +
		"  Referrer-Policy: strict-origin-when-cross-origin\n" +
		"  Permissions-Policy: camera=(), microphone=(), geolocation=()\n" +
		"  X-Frame-Options: DENY\n" +
		"\n" +
		"/assets/shots/*\n" +
		"  Cache-Control: public, max-age=604800\n"
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
