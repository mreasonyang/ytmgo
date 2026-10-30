package main

import (
	"runtime"
	"strconv"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"ytmgo/internal/preference"
)

const defaultGlobalShortcut = "CmdOrCtrl+Shift+Space"
const disabledGlobalShortcut = "disabled"

// shortcutBinding owns the OS registration and persisted choice on the UI thread.
// Capturing temporarily releases the old chord so it cannot play music or steal
// the key from the recorder. Every failed change restores the previous choice.
type shortcutBinding struct {
	value             string
	active, recording bool
	register          func(string) error
	unregister        func(string)
	save              func(string) error
}

func (s *shortcutBinding) restore() string {
	if s.value == disabledGlobalShortcut || s.active {
		return ""
	}
	if s.register(s.value) != nil {
		return "nativeShortcutUnavailable"
	}
	s.active = true
	return ""
}
func (s *shortcutBinding) begin() {
	if s.active {
		s.unregister(s.value)
		s.active = false
	}
	s.recording = true
}
func (s *shortcutBinding) cancel() string {
	s.recording = false
	return s.restore()
}
func (s *shortcutBinding) change(value string) string {
	if !validGlobalShortcut(value) {
		return "nativeShortcutInvalid"
	}
	s.recording = false
	old, wasActive := s.value, s.active
	if value == old {
		s.recording = false
		return s.restore()
	}
	if wasActive {
		s.unregister(old)
		s.active = false
	}
	if value != disabledGlobalShortcut {
		if s.register(value) != nil {
			if s.restore() != "" {
				return "nativeShortcutUnavailable"
			}
			return "nativeShortcutConflict"
		}
		s.active = true
	}
	if s.save(value) != nil {
		if s.active {
			s.unregister(value)
			s.active = false
		}
		if s.restore() != "" {
			return "nativeShortcutUnavailable"
		}
		return "nativeShortcutSaveFailed"
	}
	s.value, s.recording = value, false
	return ""
}

func shortcutKeyName(key ui.Key) string {
	if key >= ui.KeyA && key <= ui.KeyZ {
		return string(rune('A') + rune(key-ui.KeyA))
	}
	if key >= ui.Key0 && key <= ui.Key9 {
		return string(rune('0') + rune(key-ui.Key0))
	}
	if key >= ui.KeyF1 && key <= ui.KeyF12 {
		return "F" + strconv.Itoa(int(key-ui.KeyF1)+1)
	}
	return map[ui.Key]string{ui.KeySpace: "Space", ui.KeyEnter: "Enter", ui.KeyLeft: "Left", ui.KeyRight: "Right", ui.KeyUp: "Up", ui.KeyDown: "Down", ui.KeyHome: "Home", ui.KeyEnd: "End", ui.KeyPageUp: "PageUp", ui.KeyPageDown: "PageDown"}[key]
}
func shortcutChord(mods ui.Modifiers, key ui.Key) (string, string) {
	name := shortcutKeyName(key)
	if name == "" || mods & ^(ui.Ctrl|ui.Alt|ui.Shift|ui.Super) != 0 {
		return "", "nativeShortcutInvalid"
	}
	if mods&(ui.Ctrl|ui.Alt|ui.Super) == 0 && !(key >= ui.KeyF1 && key <= ui.KeyF12) {
		return "", "nativeShortcutInvalid"
	}
	// Keep application navigation, editing and lifecycle shortcuts available.
	if mods&ui.Cmd != 0 && mods&(ui.Ctrl|ui.Alt|ui.Super) == ui.Cmd {
		switch key {
		case ui.KeyA, ui.KeyB, ui.KeyC, ui.KeyF, ui.KeyH, ui.KeyJ, ui.KeyK, ui.KeyL, ui.KeyM, ui.KeyP, ui.KeyQ, ui.KeyV, ui.KeyW, ui.KeyX, ui.KeyZ, ui.Key1, ui.KeyLeft, ui.KeyRight, ui.KeyUp, ui.KeyDown:
			return "", "nativeShortcutReserved"
		}
	}
	parts := []string{}
	for _, mod := range []struct {
		bit  ui.Modifiers
		name string
	}{{ui.Ctrl, "Ctrl"}, {ui.Alt, "Alt"}, {ui.Shift, "Shift"}, {ui.Super, "Super"}} {
		if mods&mod.bit != 0 {
			parts = append(parts, mod.name)
		}
	}
	return strings.Join(append(parts, name), "+"), ""
}
func validGlobalShortcut(value string) bool {
	if value == disabledGlobalShortcut || value == defaultGlobalShortcut {
		return true
	}
	parts := strings.Split(value, "+")
	var mods ui.Modifiers
	for _, part := range parts[:len(parts)-1] {
		var bit ui.Modifiers
		switch part {
		case "Ctrl":
			bit = ui.Ctrl
		case "Alt":
			bit = ui.Alt
		case "Shift":
			bit = ui.Shift
		case "Super":
			bit = ui.Super
		default:
			return false
		}
		if mods&bit != 0 {
			return false
		}
		mods |= bit
	}
	for key := ui.Key(1); key <= ui.KeyZ; key++ {
		if shortcutKeyName(key) == parts[len(parts)-1] {
			chord, err := shortcutChord(mods, key)
			return err == "" && chord == value
		}
	}
	return false
}
func shortcutDisplay(value string) string {
	if value == "" {
		value = defaultGlobalShortcut
	}
	parts := strings.Split(value, "+")
	for i, p := range parts {
		if runtime.GOOS == "darwin" {
			if s := map[string]string{"CmdOrCtrl": "⌘", "Super": "⌘", "Ctrl": "⌃", "Alt": "⌥", "Shift": "⇧", "Left": "←", "Right": "→", "Up": "↑", "Down": "↓", "Enter": "↩", "Space": "Space"}[p]; s != "" {
				parts[i] = s
			}
		} else if p == "CmdOrCtrl" {
			parts[i] = "Ctrl"
		}
	}
	return strings.Join(parts, " ")
}
func (a *desktop) initGlobalShortcut(path string) {
	a.shortcut = &shortcutBinding{
		value:      preference.Load(path, validGlobalShortcut, defaultGlobalShortcut),
		register:   func(value string) error { return mygo.GlobalShortcut.Register(value, a.toggle) },
		unregister: mygo.GlobalShortcut.Unregister,
		save:       func(value string) error { return preference.Save(path, value, validGlobalShortcut) },
	}
	a.model.ShortcutError = a.shortcut.restore()
	a.syncShortcut()
}
func (a *desktop) syncShortcut() {
	if a.shortcut == nil {
		return
	}
	a.model.GlobalShortcut = a.shortcut.value
	a.model.ShortcutRecording = a.shortcut.recording
	a.invalidate()
}
func (a *desktop) beginShortcut() {
	if a.shortcut == nil {
		return
	}
	a.shortcut.begin()
	a.model.ShortcutError = ""
	a.syncShortcut()
}
func (a *desktop) cancelShortcut() {
	if a.shortcut == nil || !a.shortcut.recording {
		return
	}
	a.model.ShortcutError = a.shortcut.cancel()
	a.syncShortcut()
}
func (a *desktop) setShortcut(value string) {
	if a.shortcut == nil {
		return
	}
	a.model.ShortcutError = a.shortcut.change(value)
	a.syncShortcut()
}

func nativeShortcutValue(m *UIModel, a ViewActions) string {
	if m.GlobalShortcut == disabledGlobalShortcut {
		return a.Tr("nativeShortcutDisabled")
	}
	return shortcutDisplay(m.GlobalShortcut)
}
func nativeShortcutSettings(c *ui.Context, m *UIModel, a ViewActions, p nativePalette) {
	ui.Column(c).Gap(10).Children(func() {
		ui.Text(c, a.Tr("nativeShortcutGlobal")).FontSize(16).FontWeight(700)
		ui.Text(c, a.Tr("nativeShortcutGlobalHint")).FontSize(13).LineHeight(1.4).TextColor(p.secondary)
		if m.ShortcutRecording {
			ui.Button(c, a.Tr("nativeShortcutRecording")).Height(44).WidthPercent(100).Border(2, p.accent).Radius(6).Background(p.input).TextColor(p.text).Focus().InputMethod(false).HandleInput(func(ev ui.InputEvent) bool {
				if ev.Kind != ui.InputKeyDown || ev.Repeat {
					return false
				}
				if ev.Key == ui.KeyEscape {
					if a.CancelShortcut != nil {
						a.CancelShortcut()
					}
					return true
				}
				key := ev.PhysicalKey
				if key == ui.KeyUnknown {
					key = ev.Key
				}
				if key == ui.KeyUnknown {
					return true
				}
				chord, errKey := shortcutChord(ev.Mods, key)
				if errKey != "" {
					m.ShortcutError = errKey
				} else if a.SetShortcut != nil {
					a.SetShortcut(chord)
				}
				return true
			})
			if nativePill(c, a.Tr("buttonCancel"), p, false).Disabled(a.CancelShortcut == nil).Clicked() {
				a.CancelShortcut()
			}
		} else {
			ui.Text(c, nativeShortcutValue(m, a)).FontSize(18).FontWeight(700)
			ui.Row(c).Gap(8).Wrap().Children(func() {
				if nativePill(c, a.Tr("nativeShortcutRecord"), p, false).Disabled(a.BeginShortcut == nil).Clicked() {
					a.BeginShortcut()
				}
				if nativePill(c, a.Tr("nativeShortcutDisable"), p, false).Disabled(m.GlobalShortcut == disabledGlobalShortcut || a.SetShortcut == nil).Clicked() {
					a.SetShortcut(disabledGlobalShortcut)
				}
			})
		}
		if m.ShortcutError != "" {
			ui.Text(c, a.Tr(m.ShortcutError)).FontSize(13).LineHeight(1.4).TextColor(p.accentText)
		}
	})
}
