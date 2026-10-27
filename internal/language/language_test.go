package language

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestSupportedMatchesCatalogs(t *testing.T) {
	entries, err := os.ReadDir("../../locales")
	if err != nil {
		t.Fatal(err)
	}
	var tags []string
	for _, e := range entries {
		if tag, ok := strings.CutSuffix(e.Name(), ".json"); ok {
			tags = append(tags, tag)
		}
	}
	slices.Sort(tags)
	if !slices.Equal(tags, Supported) {
		t.Fatalf("catalogs %v, supported %v", tags, Supported)
	}
}

func TestServiceLanguageAndScript(t *testing.T) {
	for locale, want := range map[string][2]string{"en": {"en", "latn"}, "zh-Hans": {"zh-CN", "hans"}, "zh-Hant": {"zh-TW", "hant"}, "ja": {"ja", "jpan"}, "en-XA": {"en", "latn"}} {
		if got := [2]string{ServiceLanguage(locale), Script(locale)}; got != want {
			t.Errorf("%s: got %v want %v", locale, got, want)
		}
	}
}

func TestChoiceRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "language")
	if Load(path) != System {
		t.Fatal("missing choice must follow the system")
	}
	if err := Save(path, "zh-Hant"); err != nil || Load(path) != "zh-Hant" {
		t.Fatal("choice not kept", err)
	}
	if Save(path, "fr") == nil || Load(path) != "zh-Hant" {
		t.Fatal("unsupported language accepted")
	}
	os.WriteFile(path, []byte("../etc\n"), 0600)
	if Load(path) != System {
		t.Fatal("corrupt choice must fall back to the system")
	}
}
