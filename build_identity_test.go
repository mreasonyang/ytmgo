package main

import (
	"strings"
	"testing"
)

func TestBuildIdentityDistinguishesDevelopmentFromRelease(t *testing.T) {
	b := buildIdentity{Version: "0.0.64", Kind: "development", Commit: strings.Repeat("a", 40), Branch: "feature/ui", Dirty: true}
	want := "0.0.64 · DEV · feature/ui · aaaaaaaaaaaa · dirty"
	if got := b.displayVersion(); got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	b.Kind = "release"
	if got := b.displayVersion(); got != "0.0.64" {
		t.Fatalf("release version = %q", got)
	}
}

func TestDevelopmentBuildCannotEnableUpdates(t *testing.T) {
	if buildKind != "development" || updatesEnabled() {
		t.Fatal("development builds must not enable updates")
	}
	item := updateMenuItem()
	if !item.Disabled || item.Click != nil {
		t.Fatal("development update action must not invoke the updater")
	}
}

func TestNativeAboutShowsDevelopmentIdentity(t *testing.T) {
	options := developmentAboutOptions()
	if !strings.Contains(options.ApplicationVersion, "DEV") || options.ApplicationName != "ytmGO Dev" {
		t.Fatal("native About panel must distinguish development builds")
	}
	about := applicationMenuWithUpdates(updateMenuItem).Submenu[0]
	if about.Role != "" || about.Click == nil {
		t.Fatal("native default About action would hide development identity")
	}
}
