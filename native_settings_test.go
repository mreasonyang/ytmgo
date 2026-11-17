package main

import (
	"fmt"
	"runtime"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

func nativeSettingsFixture(width, height int) (*UIModel, ViewActions, *ui.Tester) {
	m, a := nativeViewFixture()
	m.SignedIn, m.MusicPremium, m.Settings = true, true, true
	m.GlobalShortcut = defaultGlobalShortcut
	a.BeginShortcut = func() { m.ShortcutRecording = true; m.ShortcutError = "" }
	a.CancelShortcut = func() { m.ShortcutRecording = false }
	a.SetShortcut = func(value string) { m.GlobalShortcut = value; m.ShortcutRecording = false }
	tt := ui.NewTester(func(c *ui.Context) { nativeView(c, m, a) }, width, height)
	return m, a, tt
}

// At the narrowest window every appearance label fits its segment on one
// line, beside its icon, in every language.
func TestNativeSettingsAppearanceFitsNarrow(t *testing.T) {
	for _, test := range []struct{ name, locale, systemLabel string }{
		{"en", "en", ""}, {"zh-Hans", "zh-Hans", ""}, {"zh-Hant", "zh-Hant", ""}, {"ja", "ja", ""},
		// Exercise unequal label widths on macOS too: its system fonts can
		// otherwise hide the clipping seen with Linux's CJK fallback fonts.
		{"long-system-label", "en", "Follow system"},
	} {
		t.Run(test.name, func(t *testing.T) {
			m, a, tt := nativeSettingsFixture(380, 700)
			m.Language = test.locale
			if test.systemLabel != "" {
				translate := a.Tr
				a.Tr = func(key string) string {
					if key == "shellThemeSystem" {
						return test.systemLabel
					}
					return translate(key)
				}
				tt = ui.NewTester(func(c *ui.Context) { nativeView(c, m, a) }, 380, 700)
			}
			tt.Frame()
			for _, key := range []string{"shellThemeSystem", "shellThemeDark", "shellThemeLight"} {
				label := a.Tr(key)
				r, ok := tt.Find(label)
				if !ok {
					t.Fatalf("%s segment missing", key)
				}
				w, _ := ui.ShapeText(label, ui.Font{Size: 13, Weight: 600}, 0).Size()
				if need := w + 16 + 6 + 20; r.W < need {
					t.Fatalf("%q segment is %.0f wide, needs %.0f", label, r.W, need)
				}
				if r.X < 0 || r.X+r.W > 380 {
					t.Fatalf("%q segment extends beyond the window", label)
				}
			}
		})
	}
}

// Appearance is one radio group: arrows choose within it.
func TestNativeSettingsAppearanceArrows(t *testing.T) {
	m, a, tt := nativeSettingsFixture(1280, 820)
	if err := tt.Click(a.Tr("shellThemeDark")); err != nil {
		t.Fatal(err)
	}
	tt.Key(0, ui.KeyRight)
	if m.Appearance != "light" {
		t.Fatalf("Right arrow chose %q", m.Appearance)
	}
	tt.Key(0, ui.KeyHome)
	if m.Appearance != "system" {
		t.Fatalf("Home chose %q", m.Appearance)
	}
}

// Recording ends when the focus leaves the recorder, and Escape ends only
// the recording.
func TestNativeSettingsRecorderEndsOnBlur(t *testing.T) {
	m, a, tt := nativeSettingsFixture(1280, 820)
	if err := tt.Click(a.Tr("nativeShortcutRecord")); err != nil {
		t.Fatal(err)
	}
	if !m.ShortcutRecording {
		t.Fatal("recording did not start")
	}
	tt.Key(0, ui.KeyEscape)
	if m.ShortcutRecording || !m.Settings {
		t.Fatalf("Escape: recording=%v settings=%v", m.ShortcutRecording, m.Settings)
	}
	if err := tt.Click(a.Tr("nativeShortcutRecord")); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click(a.Tr("shellThemeLight")); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if m.ShortcutRecording || m.Appearance != "light" {
		t.Fatalf("clicking another control: recording=%v appearance=%q", m.ShortcutRecording, m.Appearance)
	}
}

// With the shortcut off the row still offers recording by name.
func TestNativeSettingsShortcutOffOffersRecord(t *testing.T) {
	m, a, tt := nativeSettingsFixture(1280, 820)
	m.GlobalShortcut = disabledGlobalShortcut
	tt.Frame()
	if tt.HasText(a.Tr("nativeShortcutDisable")) {
		t.Fatal("Turn off offered while off")
	}
	count := 0
	for _, text := range tt.Texts() {
		if text == a.Tr("nativeShortcutRecord") {
			count++
		}
	}
	if count < 2 {
		t.Fatal("off state has no visible Record shortcut action")
	}
}

// The sign-out row is inset in its card so its focus ring is not clipped.
func TestNativeSettingsSignOutInset(t *testing.T) {
	_, a, tt := nativeSettingsFixture(1280, 820)
	r, ok := tt.Find(a.Tr("shellSignOut"))
	if !ok {
		t.Fatal("sign out missing")
	}
	cardLeft := float32(1280-nativeSettingsWidth)/2 + 24
	if r.X < cardLeft+4-0.5 {
		t.Fatalf("sign-out row at x=%.1f touches the card edge %.1f", r.X, cardLeft)
	}
}

// Configure in Keyboard shortcuts brings the global shortcut into view, clear
// of the bottom fade, even in a short compact window.
func TestNativeSettingsConfigureRevealsShortcut(t *testing.T) {
	m, a, tt := nativeSettingsFixture(380, 640)
	m.Settings, m.Shortcuts = false, true
	tt.Frame()
	if err := tt.Click(a.Tr("nativeShortcutConfigure")); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	r, ok := tt.Find(a.Tr("nativeShortcutRecord"))
	if !ok || r.H < 36 || r.Y+r.H > 640-40 {
		t.Fatalf("shortcut field not in view: %v %v", r, ok)
	}
}

func TestShortcutDisplayPlatformOrder(t *testing.T) {
	got := nativeLocalChord("CmdOrCtrl+Right / CmdOrCtrl+Left")
	mods := shortcutDisplay("Shift+CmdOrCtrl+Alt+P")
	if runtime.GOOS == "darwin" {
		if got != "⌘ → / ⌘ ←" || mods != "⌥ ⇧ ⌘ P" {
			t.Fatalf("got %q and %q", got, mods)
		}
		return
	}
	if got != "Ctrl → / Ctrl ←" || mods != "Ctrl Alt Shift P" {
		t.Fatalf("got %q and %q", got, mods)
	}
	if strings.Contains(nativeLocalChord("CmdOrCtrl+K"), "⌘") {
		t.Fatal("⌘ shown off macOS")
	}
}

// Keyboard shortcuts opened from Settings leads back there, to the Keyboard
// card; opened from the Help menu it has nothing to go back to.
func TestNativeShortcutsLeadBackToSettings(t *testing.T) {
	m, a, tt := nativeSettingsFixture(1280, 820)
	tt.Frame()
	if err := tt.Click(a.Tr("nativeSettingsViewAll")); err != nil {
		t.Fatal(err)
	}
	if m.Settings || !m.Shortcuts {
		t.Fatal("View all did not open Keyboard shortcuts")
	}
	if err := tt.Click(a.Tr("shellBackToSettings")); err != nil {
		t.Fatalf("Keyboard shortcuts opened from Settings has no way back: %v", err)
	}
	if !m.Settings || m.Shortcuts {
		t.Fatalf("Back did not return to Settings: settings=%v shortcuts=%v", m.Settings, m.Shortcuts)
	}
	tt.Frame()
	if r, ok := tt.Find(a.Tr("nativeSettingsViewAll")); !ok || r.Y+r.H > 820-40 {
		t.Fatalf("Settings did not come back to its Keyboard card: %v %v", r, ok)
	}

	// From the Help menu: the sheet alone, with no Back.
	m.Settings, m.Shortcuts = false, true
	tt.Frame()
	if _, ok := tt.Find(a.Tr("shellBackToSettings")); ok {
		t.Fatal("Keyboard shortcuts from the Help menu offers Back to nothing")
	}
}

// Check for Updates… is in Settings › About, beside the version, in every
// language and at the narrowest window; a build that doesn't update itself
// says so there instead.
func TestNativeSettingsChecksForUpdates(t *testing.T) {
	for _, width := range []int{380, 1440} {
		for _, locale := range []string{"en", "ja", "zh-Hans", "zh-Hant"} {
			m, a, _ := nativeSettingsFixture(width, 2000)
			m.Language, m.Version = locale, "0.0.67"
			checks := 0
			a.CheckUpdates = func() { checks++ }
			tt := ui.NewTester(func(c *ui.Context) { nativeView(c, m, a) }, width, 2000)
			tt.Frame()
			// The wide sheet scrolls; About is at its end.
			if r, ok := tt.Find(a.Tr("nativeSettingsGeneral")); ok {
				tt.Scroll(r.X+r.W/2, r.Y+40, 0, 2000)
				tt.Frame()
			}
			nativeSnapshot(t, tt, "settings-updates-"+locale+"-"+fmt.Sprint(width))
			label := a.Tr("nativeCheckUpdates")
			button, ok := tt.Find(label)
			if !ok {
				t.Fatalf("%s %d: no Check for Updates…", locale, width)
			}
			version, ok := tt.Find(a.TrArgs("nativeSettingsVersion", map[string]any{"version": m.Version}))
			if !ok {
				t.Fatalf("%s %d: no version", locale, width)
			}
			name, _ := tt.Find(a.Tr("appName"))
			if version.H > 24 || name.H > 24 || button.X+button.W > float32(width) {
				t.Fatalf("%s %d: the name %v or version %v wraps, or the button %v is cut", locale, width, name, version, button)
			}
			// Wide, the button is beside the version; narrow, under it.
			if beside := button.X >= version.X+version.W; beside != (width > 600) {
				t.Fatalf("%s %d: the button %v against the version %v", locale, width, button, version)
			}
			if tt.HasText(a.Tr("menuDevelopmentUpdatesDisabled")) {
				t.Fatalf("%s %d: an updating build says updates are off", locale, width)
			}
			if err := tt.Click(label); err != nil {
				t.Fatal(err)
			}
			if checks != 1 {
				t.Fatalf("%s %d: the button did not check", locale, width)
			}

			a.CheckUpdates = nil
			tt = ui.NewTester(func(c *ui.Context) { nativeView(c, m, a) }, width, 2000)
			tt.Frame()
			if r, ok := tt.Find(a.Tr("nativeSettingsGeneral")); ok {
				tt.Scroll(r.X+r.W/2, r.Y+40, 0, 2000)
				tt.Frame()
			}
			if _, ok := tt.Find(label); ok || !tt.HasText(a.Tr("menuDevelopmentUpdatesDisabled")) {
				t.Fatalf("%s %d: a development build offers a check it can't make", locale, width)
			}
			nativeSnapshot(t, tt, "settings-updates-dev-"+locale+"-"+fmt.Sprint(width))
		}
	}
}
