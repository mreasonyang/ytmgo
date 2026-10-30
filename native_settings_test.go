package main

import (
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
	for _, locale := range []string{"en", "zh-Hans", "zh-Hant", "ja"} {
		t.Run(locale, func(t *testing.T) {
			m, a, tt := nativeSettingsFixture(380, 700)
			m.Language = locale
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
