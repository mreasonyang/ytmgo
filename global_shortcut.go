package main

import (
	"runtime"
	"sort"
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

// shortcutDisplay spells an accelerator for this platform, modifiers in the
// platform's order (⌃ ⌥ ⇧ ⌘ on macOS, Ctrl Alt Shift elsewhere), keys space
// separated so they can be drawn as key caps.
func shortcutDisplay(value string) string {
	if value == "" {
		value = defaultGlobalShortcut
	}
	mac := runtime.GOOS == "darwin"
	rank := map[string]int{"Ctrl": 0, "Alt": 1, "Shift": 2, "CmdOrCtrl": 3, "Super": 3}
	if !mac {
		rank["CmdOrCtrl"] = 0
	}
	parts := strings.Split(value, "+")
	sort.SliceStable(parts, func(i, j int) bool {
		ri, ok := rank[parts[i]]
		if !ok {
			ri = 4
		}
		rj, ok := rank[parts[j]]
		if !ok {
			rj = 4
		}
		return ri < rj
	})
	names := map[string]string{"CmdOrCtrl": "Ctrl", "Left": "←", "Right": "→", "Up": "↑", "Down": "↓", "Escape": "Esc"}
	if mac {
		names = map[string]string{"CmdOrCtrl": "⌘", "Super": "⌘", "Ctrl": "⌃", "Alt": "⌥", "Shift": "⇧", "Left": "←", "Right": "→", "Up": "↑", "Down": "↓", "Enter": "↩", "Escape": "Esc"}
	}
	for i, p := range parts {
		if s := names[p]; s != "" {
			parts[i] = s
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

// nativeShortcutSettings is the global shortcut's settings row. Its field
// shows the chord as key caps; clicking it records a new one in place, until
// a chord is pressed, Escape or Cancel, or the focus leaves the field.
func nativeShortcutSettings(c *ui.Context, m *UIModel, a ViewActions, p nativePalette, narrow bool) {
	ui.Column(c).Children(func() {
		nativeSettingRow(c, p, narrow, "play", a.Tr("nativeShortcutGlobal"), a.Tr("nativeShortcutGlobalShortHint"), func() {
			if m.ShortcutRecording {
				field := ui.Button(c, a.Tr("nativeShortcutRecording")).Height(36).MinWidth(170).PaddingX(14).Radius(8).Border(2, p.accent).Background(p.input).TextColor(p.text).AutoFocus().InputMethod(false)
				if narrow {
					field.Grow(1)
				}
				if field.Focused() {
					m.shortcutFieldFocused = true
				} else if m.shortcutFieldFocused && a.CancelShortcut != nil {
					a.CancelShortcut()
				}
				field.HandleInput(func(ev ui.InputEvent) bool {
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
				if nativeLinkButton(c, a.Tr("buttonCancel"), p, "").Disabled(a.CancelShortcut == nil).Clicked() && m.ShortcutRecording {
					a.CancelShortcut()
				}
				return
			}
			m.shortcutFieldFocused = false
			off := m.GlobalShortcut == disabledGlobalShortcut
			label := a.Tr("nativeShortcutRecord")
			field := ui.ButtonBase(c).Height(36).MinWidth(170).PaddingX(5).Radius(8).AlignItems(ui.Center).Justify(ui.Center).Background(p.input).Border(1, p.line).Label(label).Tooltip(label).Disabled(a.BeginShortcut == nil)
			if narrow {
				field.Grow(1)
			}
			if field.Hovered() {
				field.Background(p.highlight).Border(1, p.secondary)
			}
			field.Children(func() {
				if off {
					ui.Text(c, a.Tr("nativeShortcutDisabled")).FontWeight(600).TextColor(p.secondary)
				} else {
					nativeKeycaps(c, p, nativeShortcutValue(m, a))
				}
			})
			if field.Clicked() {
				a.BeginShortcut()
			}
			// Off, the field has no chord to show, so recording is offered by name.
			if off {
				if nativeLinkButton(c, label, p, "").Disabled(a.BeginShortcut == nil).Clicked() {
					a.BeginShortcut()
				}
			} else if nativeLinkButton(c, a.Tr("nativeShortcutDisable"), p, "").Disabled(a.SetShortcut == nil).Clicked() {
				a.SetShortcut(disabledGlobalShortcut)
			}
		})
		if m.ShortcutError != "" {
			ui.Text(c, a.Tr(m.ShortcutError)).Padding(0, 16, 14, 64).FontSize(13).LineHeight(1.4).TextColor(p.accentText)
		}
	})
}
