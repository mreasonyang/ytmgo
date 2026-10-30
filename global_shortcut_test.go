package main

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
	"ytmgo/internal/preference"
)

func TestGlobalShortcutTransactions(t *testing.T) {
	for _, failure := range []string{"", "register", "save", "restore"} {
		t.Run(failure, func(t *testing.T) {
			registered := map[string]bool{}
			stored := defaultGlobalShortcut
			failRestore := false
			s := &shortcutBinding{value: stored, register: func(v string) error {
				if v == "F8" && (failure == "register" || failure == "restore") {
					failRestore = true
					return errors.New("occupied")
				}
				if failRestore && failure == "restore" {
					return errors.New("occupied")
				}
				if registered[v] {
					t.Fatal("duplicate registration")
				}
				registered[v] = true
				return nil
			}, unregister: func(v string) { delete(registered, v) }, save: func(v string) error {
				if failure == "save" {
					return errors.New("disk")
				}
				stored = v
				return nil
			}}
			if s.restore() != "" {
				t.Fatal("startup registration failed")
			}
			s.begin()
			if len(registered) != 0 || !s.recording {
				t.Fatal("recording did not release current binding")
			}
			got := s.change("F8")
			want := map[string]string{"": "", "register": "nativeShortcutConflict", "save": "nativeShortcutSaveFailed", "restore": "nativeShortcutUnavailable"}[failure]
			if got != want || s.recording {
				t.Fatal("incorrect change result or recorder left active")
			}
			if failure == "" {
				if stored != "F8" || s.value != "F8" || !registered["F8"] {
					t.Fatal("new binding not applied and saved")
				}
				if s.change(disabledGlobalShortcut) != "" || stored != disabledGlobalShortcut || len(registered) != 0 {
					t.Fatal("disable did not persist and unregister")
				}
			} else if stored != defaultGlobalShortcut || s.value != defaultGlobalShortcut || (failure != "restore" && !registered[defaultGlobalShortcut]) {
				t.Fatal("previous setting lost after failure")
			}
		})
	}
}
func TestGlobalShortcutCancelDisableAndPersistence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "global-shortcut")
	registered := map[string]bool{}
	failSave := false
	s := &shortcutBinding{value: defaultGlobalShortcut, register: func(v string) error { registered[v] = true; return nil }, unregister: func(v string) { delete(registered, v) }, save: func(v string) error {
		if failSave {
			return errors.New("disk")
		}
		return preference.Save(path, v, validGlobalShortcut)
	}}
	s.restore()
	s.begin()
	if s.cancel() != "" || !registered[defaultGlobalShortcut] || s.recording {
		t.Fatal("cancel did not restore previous binding")
	}
	s.begin()
	if s.change(defaultGlobalShortcut) != "" || len(registered) != 1 {
		t.Fatal("same chord did not restore registration")
	}
	failSave = true
	if s.change(disabledGlobalShortcut) != "nativeShortcutSaveFailed" || !registered[defaultGlobalShortcut] {
		t.Fatal("failed disable lost old binding")
	}
	failSave = false
	if s.change("Ctrl+Alt+P") != "" || preference.Load(path, validGlobalShortcut, "") != "Ctrl+Alt+P" {
		t.Fatal("binding not persisted")
	}
	if s.change(disabledGlobalShortcut) != "" || preference.Load(path, validGlobalShortcut, "") != disabledGlobalShortcut {
		t.Fatal("disabled choice not persisted")
	}
}
func TestGlobalShortcutValidation(t *testing.T) {
	for _, v := range []string{defaultGlobalShortcut, disabledGlobalShortcut, "F8", "Ctrl+Alt+P", "Ctrl+Shift+Super+Space"} {
		if !validGlobalShortcut(v) {
			t.Errorf("valid shortcut rejected: %s", v)
		}
	}
	for _, v := range []string{"", "P", "Space", "Shift+P", "Ctrl+Ctrl+P", "CmdOrCtrl+Q", "bad", "F13"} {
		if validGlobalShortcut(v) {
			t.Errorf("invalid shortcut accepted: %s", v)
		}
	}
	if chord, key := shortcutChord(ui.Cmd, ui.KeyP); chord != "" || key != "nativeShortcutReserved" {
		t.Fatal("local accelerator can be stolen")
	}
}
func TestNativeShortcutRecorderAndHelp(t *testing.T) {
	for _, locale := range []string{"en", "zh-Hans", "zh-Hant", "ja"} {
		t.Run(locale, func(t *testing.T) {
			m, a := nativeViewFixture()
			m.Language = locale
			m.GlobalShortcut = defaultGlobalShortcut
			a.BeginShortcut = func() { m.ShortcutRecording = true; m.ShortcutError = "" }
			a.CancelShortcut = func() { m.ShortcutRecording = false }
			var saved string
			a.SetShortcut = func(value string) { saved = value; m.GlobalShortcut = value; m.ShortcutRecording = false }
			tt := ui.NewTester(func(c *ui.Context) { nativeShortcutSettings(c, m, a, palette(true), true) }, 420, 600)
			if err := tt.Click(a.Tr("nativeShortcutRecord")); err != nil {
				t.Fatal(err)
			}
			tt.Key(0, ui.KeySpace)
			if m.ShortcutError != "nativeShortcutInvalid" || !m.ShortcutRecording || !m.Playing {
				t.Fatal("plain Space was captured or toggled playback")
			}
			tt.Key(ui.Cmd, ui.KeyP)
			if m.ShortcutError != "nativeShortcutReserved" || !m.ShortcutRecording {
				t.Fatal("reserved key was accepted")
			}
			tt.Key(ui.Ctrl|ui.Alt, ui.KeyP)
			if saved != "Ctrl+Alt+P" || m.ShortcutRecording {
				t.Fatal("recorder did not capture combination")
			}
			if err := tt.Click(a.Tr("nativeShortcutRecord")); err != nil {
				t.Fatal(err)
			}
			tt.Key(0, ui.KeyF8)
			if saved != "F8" {
				t.Fatal("simple function key was not captured")
			}
			if err := tt.Click(a.Tr("nativeShortcutRecord")); err != nil {
				t.Fatal(err)
			}
			tt.Key(0, ui.KeyEscape)
			if m.ShortcutRecording || saved != "F8" {
				t.Fatal("Escape changed chosen binding")
			}
			if err := tt.Click(a.Tr("nativeShortcutDisable")); err != nil {
				t.Fatal(err)
			}
			if saved != disabledGlobalShortcut || !tt.HasText(a.Tr("nativeShortcutDisabled")) {
				t.Fatal("disable control failed")
			}
			m.Shortcuts = true
			tt = ui.NewTester(func(c *ui.Context) { nativeView(c, m, a) }, 757, 609)
			if !tt.HasText(a.Tr("nativeShortcutGlobal")) || !tt.HasText(a.Tr("nativeShortcutDisabled")) || !tt.HasText(a.Tr("nativeShortcutLocalHint")) {
				t.Fatal("help omitted current binding or local scope")
			}
			if err := tt.Click(a.Tr("nativeShortcutConfigure")); err != nil {
				t.Fatal(err)
			}
			if !m.Settings || m.Shortcuts {
				t.Fatal("help cannot open shortcut configuration")
			}
			nativeSnapshot(t, tt, "shortcut-settings-"+locale)
		})
	}
}
func TestNativeShortcutRecordingSuppressesLocalActions(t *testing.T) {
	m, a := nativeViewFixture()
	m.Settings = true
	m.ShortcutRecording = true
	cancelled := 0
	a.CancelShortcut = func() { cancelled++; m.ShortcutRecording = false }
	tt := ui.NewTester(func(c *ui.Context) { nativeView(c, m, a) }, 757, 609)
	tt.Key(ui.Cmd, ui.KeyJ)
	if m.Panel {
		t.Fatal("recording triggered local panel shortcut")
	}
	tt.Key(0, ui.KeyEscape)
	if m.ShortcutRecording || cancelled != 1 {
		t.Fatal("closing settings did not end recording")
	}
}

func TestGlobalShortcutStartupAndMenuCaptureFailure(t *testing.T) {
	s := &shortcutBinding{value: "F8", register: func(string) error { return errors.New("occupied") }}
	if s.restore() != "nativeShortcutUnavailable" || s.active {
		t.Fatal("startup failure was hidden")
	}
	registered := false
	s = &shortcutBinding{value: "F8", register: func(string) error { registered = true; return nil }, unregister: func(string) { registered = false }, save: func(string) error { return nil }}
	s.restore()
	s.begin()
	a := &desktop{shortcut: s, model: UIModel{ShortcutRecording: true}}
	invoked := false
	item := a.menuItem("Play", "CmdOrCtrl+P", func() { invoked = true })
	item.Click(nil, nil)
	if invoked || !registered || a.model.ShortcutRecording || a.model.ShortcutError != "nativeShortcutReserved" {
		t.Fatal("menu accelerator escaped recording or lost registration")
	}
}
