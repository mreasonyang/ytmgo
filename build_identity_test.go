package main

import (
	"github.com/egoist/mygo/ui"
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
	label := applicationBuildLabel()
	b := currentBuildIdentity()
	if !strings.Contains(label, "DEV") || (b.Branch != "" && !strings.Contains(label, b.Branch)) {
		t.Fatalf("the build line %q must distinguish development builds", label)
	}
	opened := false
	about := applicationMenuWithUpdates(updateMenuItem, func() { opened = true }).Submenu[0]
	if about.Role != "" || about.Click == nil {
		t.Fatal("the app menu's About must open the app's own About, not the system panel")
	}
	about.Click(about, nil)
	if !opened {
		t.Fatal("the app menu's About opened nothing")
	}
	// The app's About shows that line.
	m, a := nativeViewFixture()
	m.Version, m.About = label, true
	tt := ui.NewTester(func(c *ui.Context) { nativeView(c, m, a) }, 1440, 900)
	tt.Frame()
	if !tt.HasText(label) {
		t.Fatalf("About does not show the build %q", label)
	}
	if err := tt.Click(a.Tr("buttonOK")); err != nil || m.About {
		t.Fatalf("OK did not close About (%v)", err)
	}
}
