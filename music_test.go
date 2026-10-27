package main

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"ytmgo/internal/language"
	"ytmgo/locales"
)

func TestMusicPreloadIsolation(t *testing.T) {
	script := musicPreload("system", newShellI18N("en", "system"))
	for _, required := range []string{
		"location.origin !== 'https://music.youtube.com'", "window.top !== window",
		`, "system", {`, "ytmgo://appearance/", "ytmgo://account/", "ytmgo://session/signout",
		"window.ytmgoControls", "window.ytmgoSetAppearance", "ytmgo-sidebar", "ytmgo-playerbar", "ytmgo-np",
	} {
		if !strings.Contains(script, required) {
			t.Errorf("missing shell contract %q", required)
		}
	}
	// No bridge, no credential/storage access, no network, and nothing the
	// service's Trusted Types policy would reject.
	for _, forbidden := range []string{"YTMGO_STYLES", "YTMGO_APPEARANCE", "document.cookie", "localStorage", "sessionStorage", "indexedDB", "window.mygo", "fetch(", "XMLHttpRequest", "innerHTML", "__ytmgoTest"} {
		if strings.Contains(script, forbidden) {
			t.Errorf("unexpected bridge, credential access, or template token %q", forbidden)
		}
	}
}

func TestMusicPreloadIncludesEveryManifestEntry(t *testing.T) {
	var manifest struct{ Styles, Scripts []string }
	raw, err := assets.ReadFile("web/shell/bundle.json")
	if err != nil || json.Unmarshal(raw, &manifest) != nil || len(manifest.Styles) == 0 || len(manifest.Scripts) == 0 {
		t.Fatal("shell manifest missing or invalid")
	}
	script := musicPreload("dark", newShellI18N("ja", "ja"))
	for _, name := range manifest.Scripts {
		if !strings.Contains(script, "/* "+name+" */") {
			t.Errorf("script %s not bundled", name)
		}
	}
	for _, name := range manifest.Styles {
		data, err := assets.ReadFile("web/shell/" + name)
		if err != nil || len(data) == 0 {
			t.Errorf("style %s missing", name)
		}
	}
	if !strings.Contains(script, `, "dark", {"locale":"ja"`) {
		t.Error("appearance argument not passed")
	}
}

func TestShellMessagesAreSimpleAndComplete(t *testing.T) {
	keys := shellMessageKeys()
	if len(keys) < 50 {
		t.Fatalf("only %d shell messages found", len(keys))
	}
	for _, tag := range language.Supported {
		payload := newShellI18N(tag, "system")
		if len(payload.Messages) != len(keys) || payload.Script != language.Script(tag) || len(payload.Languages) != len(language.Supported) {
			t.Fatalf("%s: incomplete payload", tag)
		}
		for _, key := range keys {
			m, lang, ok := locales.Catalog.Message(tag, key)
			if !ok || lang != tag {
				t.Errorf("%s: %s is untranslated (falls back to %q)", tag, key, lang)
				continue
			}
			for _, arg := range m.Args() {
				// The shell substitutes plain {name} slots only.
				if arg.Kind != "" {
					t.Errorf("%s: %s uses a %s argument", tag, key, arg.Kind)
				}
				if !strings.Contains(payload.Messages[key], "{"+arg.Name+"}") {
					t.Errorf("%s: %s lost its {%s} slot", tag, key, arg.Name)
				}
			}
		}
	}
	for _, lang := range newShellI18N("en", "system").Languages {
		if lang.Name == "" || lang.Name == lang.Tag {
			t.Errorf("%s has no autonym", lang.Tag)
		}
	}
}

// Every user-visible string lives in locales/*.json. CJK text anywhere else
// in the app's Go or shell sources is a hard-coded string.
func TestNoHardcodedCJKOutsideCatalogs(t *testing.T) {
	cjk := regexp.MustCompile(`[\p{Han}\p{Hiragana}\p{Katakana}\p{Hangul}]`)
	_ = filepath.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && (path == "locales" || path == "build" || path == "resources" || strings.HasPrefix(d.Name(), ".")) && path != "." {
			return filepath.SkipDir
		}
		ext := filepath.Ext(path)
		if d.IsDir() || !(ext == ".go" || ext == ".js" || ext == ".css") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err == nil && cjk.Match(data) {
			t.Errorf("%s contains CJK text; move it into locales/*.json", path)
		}
		return nil
	})
}
