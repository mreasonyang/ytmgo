package main

import (
	"strings"

	"github.com/egoist/mygo/ui"
)

// Settings is one page of titled cards, as macOS and Spotify settings are:
// each row shows an icon, names and explains a setting on the left and holds
// its control on the right. Where the sheet's content is narrower than
// nativeSettingsStack the control moves under its label. Compact windows show
// the sheet full-window.
const (
	nativeSettingsWidth  = 640
	nativeShortcutsWidth = 560
	nativeSettingsStack  = 520
)

var nativeLanguages = []struct{ id, title string }{{"system", ""}, {"en", "English"}, {"zh-Hans", "简体中文"}, {"zh-Hant", "繁體中文"}, {"ja", "日本語"}}

// nativeLanguageTitle names a language in itself, as language pickers do.
func nativeLanguageTitle(a ViewActions, id string) string {
	for _, l := range nativeLanguages {
		if l.id == id && l.title != "" {
			return l.title
		}
	}
	return a.Tr("shellLanguageSystem")
}

// nativeSheet is the frame Settings and Keyboard shortcuts share: a card over
// the dimmed window with a fixed title bar and a scrolling body. A hairline
// under the title shows content scrolled beneath it and a fade at the bottom
// shows there is more; each sheet opens at its top.
func nativeSheet(c *ui.Context, open *bool, scroll *ui.ScrollState, title string, a ViewActions, p nativePalette, width, height, maxWidth float32, body func(narrow bool)) {
	if !*open {
		scroll.Y = 0
		return
	}
	compact := width <= 760
	sheet := min(maxWidth, width-48)
	titleSize := float32(24)
	if compact {
		sheet, titleSize = width, 20
	}
	ui.DialogBase(c, open, func(backdrop, panel ui.Element) {
		backdrop.Background(ui.RGBA(0, 0, 0, 0.6))
		panel.Width(sheet).Background(p.surface).Clip()
		if compact {
			panel.Height(height)
		} else {
			panel.MaxHeight(min(height-48, 780)).Radius(14).Border(1, p.line).Shadow(0, 24, 64, 0, ui.RGBA(0, 0, 0, 0.5))
		}
		panel.Children(func() {
			ui.Row(c).Shrink(0).Padding(20, 16, 12, 24).AlignItems(ui.Center).Children(func() {
				ui.Text(c, title).FontSize(titleSize).FontWeight(800).Grow(1).MinWidth(0).MaxLines(1).Ellipsis("…")
				if nativeIconButton(c, "close", a.Tr("shellClose"), p, false, 36).Clicked() {
					*open = false
				}
			})
			edge := ui.Transparent
			if scroll.Y > 0 {
				edge = p.line
			}
			ui.Box(c).Height(1).Shrink(0).Background(edge)
			ui.Scroll(c).Grow(1).MinHeight(0).Padding(12, 24, 28, 24).Gap(28).TrackScroll(scroll).Children(func() {
				body(sheet-48 < nativeSettingsStack)
			})
			if scroll.Y < scroll.MaxY-1 {
				ui.Box(c).Absolute().Left(0).Bottom(0).WidthPercent(100).Height(40).LinearGradient(ui.LinearGradient{From: p.surface.Alpha(0), To: p.surface, Angle: 180}).PassThrough()
			}
		})
	}).Label(title)
}

// nativeSettingsGroup is a titled card of rows divided by hairlines that start
// inset DIPs in, under the text of rows with icons; nil rows are skipped.
func nativeSettingsGroup(c *ui.Context, p nativePalette, title string, inset float32, rows ...func()) ui.Element {
	return ui.Column(c).Gap(10).Children(func() {
		if title != "" {
			ui.Text(c, title).FontSize(15).FontWeight(700).PaddingX(4)
		}
		ui.Column(c).Radius(12).Background(p.raised).Clip().Children(func() {
			first := true
			for _, row := range rows {
				if row == nil {
					continue
				}
				if !first {
					ui.Box(c).Height(1).Margin(0, 0, 0, inset).Background(p.line)
				}
				first = false
				row()
			}
		})
	})
}

func nativeSettingTile(c *ui.Context, icon string, bg, fg ui.Color) {
	ui.Box(c).Size(32, 32).Shrink(0).Radius(8).Background(bg).TextColor(fg).Center().Children(func() {
		ui.Icon(c, nativeIcons[icon]).Size(18, 18)
	})
}

// nativeSettingRow lays out one setting. A narrow row puts the control under
// the text, across the row.
func nativeSettingRow(c *ui.Context, p nativePalette, narrow bool, icon, title, detail string, control func()) {
	text := func() ui.Element {
		return ui.Row(c).MinWidth(0).Gap(16).AlignItems(ui.Center).Children(func() {
			nativeSettingTile(c, icon, p.highlight, p.text)
			ui.Column(c).Grow(1).MinWidth(0).Gap(3).Children(func() {
				ui.Text(c, title).FontWeight(600)
				if detail != "" {
					ui.Text(c, detail).FontSize(13).LineHeight(1.4).TextColor(p.secondary)
				}
			})
		})
	}
	if narrow {
		ui.Column(c).Padding(14, 16).Gap(12).Children(func() {
			text()
			ui.Row(c).Gap(8).Wrap().AlignItems(ui.Center).Children(control)
		})
		return
	}
	ui.Row(c).Padding(14, 16).Gap(16).AlignItems(ui.Center).Children(func() {
		text().Grow(1).Basis(0)
		ui.Row(c).Shrink(0).Gap(8).AlignItems(ui.Center).Children(control)
	})
}

// nativeKeycaps draws a chord as key caps; "/" separates alternatives.
func nativeKeycaps(c *ui.Context, p nativePalette, chord string) ui.Element {
	return ui.Row(c).Shrink(0).Gap(4).AlignItems(ui.Center).Label(chord).Children(func() {
		for _, key := range strings.Fields(chord) {
			if key == "/" {
				ui.Text(c, key).FontSize(12).PaddingX(2).TextColor(p.muted)
				continue
			}
			ui.Box(c).MinWidth(26).Height(26).PaddingX(7).Radius(6).Background(p.surface).Border(1, p.line).Center().Children(func() {
				ui.Text(c, key).FontSize(12).FontWeight(600).TextColor(p.text)
			})
		}
	})
}

// nativeLinkButton is a quiet text action for rows: secondary at rest,
// highlighted under the pointer.
func nativeLinkButton(c *ui.Context, label string, p nativePalette, icon string) ui.Element {
	b := ui.ButtonBase(c).Height(32).PaddingX(12).Gap(4).Radius(16).AlignItems(ui.Center).Label(label).TextColor(p.text).Background(p.input)
	if b.Hovered() {
		b.Background(p.highlight)
	}
	b.Children(func() {
		ui.Text(c, label).FontSize(13).FontWeight(600)
		if icon != "" {
			ui.Icon(c, nativeIcons[icon]).Size(14, 14).TextColor(p.secondary)
		}
	})
	return b
}

func nativeSettings(c *ui.Context, m *UIModel, a ViewActions, p nativePalette, width, height float32) {
	nativeSheet(c, &m.Settings, &m.settingsScroll, a.Tr("shellSettings"), a, p, width, height, nativeSettingsWidth, func(narrow bool) {
		nativeSettingsGroup(c, p, a.Tr("menuAccount"), 64, func() { nativeAccountSettings(c, m, a, p, narrow) }, nativeSignOutRow(c, m, a, p))
		nativeSettingsGroup(c, p, a.Tr("nativeSettingsGeneral"), 64,
			func() {
				nativeSettingRow(c, p, narrow, "sun", a.Tr("shellAppearance"), a.Tr("nativeSettingsAppearanceHint"), func() { nativeAppearanceControl(c, m, a, p, narrow) })
			},
			func() {
				nativeSettingRow(c, p, narrow, "globe", a.Tr("shellLanguage"), a.Tr("nativeSettingsLanguageHint"), func() { nativeLanguageControl(c, m, a, p, narrow) })
			})
		keyboard := nativeSettingsGroup(c, p, a.Tr("nativeSettingsKeyboard"), 64,
			func() { nativeShortcutSettings(c, m, a, p, narrow) },
			func() {
				nativeSettingRow(c, p, narrow, "keyboard", a.Tr("shellShortcuts"), a.Tr("nativeSettingsShortcutsHint"), func() {
					if nativeLinkButton(c, a.Tr("nativeSettingsViewAll"), p, "forward").Clicked() {
						m.Settings = false
						m.Shortcuts = true
					}
				})
			})
		// Arriving from Keyboard shortcuts, the whole card comes into view, clear
		// of the bottom fade.
		if m.revealShortcut {
			keyboard.ScrollIntoView()
			m.revealShortcut = false
		}
		ui.Column(c).Gap(10).Children(func() {
			nativeSettingsGroup(c, p, a.Tr("nativeSettingsAbout"), 64, func() {
				ui.Row(c).Padding(14, 16).Gap(16).AlignItems(ui.Center).Children(func() {
					nativeSettingTile(c, "play", p.accent, p.onAccent)
					ui.Column(c).Grow(1).MinWidth(0).Gap(3).Children(func() {
						ui.Text(c, "ytmgo").FontWeight(700)
						if m.Version != "" {
							ui.Text(c, a.TrArgs("nativeSettingsVersion", map[string]any{"version": m.Version})).FontSize(13).TextColor(p.secondary)
						}
					})
				})
			})
			ui.Text(c, a.Tr("shellFooter")).FontSize(12).LineHeight(1.5).PaddingX(4).TextColor(p.muted)
		})
	})
}

// nativeAccountSettings states the account and its plan (the status names
// Premium; the accent avatar marks it) and offers the next
// step: sign in, manage the account, or settle a slow or deferred restore.
func nativeAccountSettings(c *ui.Context, m *UIModel, a ViewActions, p nativePalette, narrow bool) {
	// A restore notice is long, so its actions go under it.
	detail, stack := "", narrow
	switch {
	case m.AccountRestore == accountRestoreSlow && !m.SignedIn:
		detail, stack = a.Tr("nativeAccountRestoreSlow"), true
	case nativeRestoreDeferred(m):
		detail, stack = a.Tr("nativeAccountRestoreDeferredBody"), true
	case !m.SignedIn && !nativeRestoring(m):
		detail = a.Tr("nativeAccountSignedOutHint")
	}
	actions := func() {
		if m.AccountRestore == accountRestoring && !m.SignedIn {
			ui.Spinner(c).Size(20, 20)
		}
		key := "shellAccountSignIn"
		if m.SignedIn {
			key = "shellAccountManage"
		}
		if !nativeRestoring(m) && nativePill(c, a.Tr(key), p, !m.SignedIn).Clicked() {
			m.Settings = false
			a.Account()
		}
		nativeRestoreActions(c, m, a, p, nativeRestoring(m))
	}
	ui.Column(c).Padding(16).Gap(14).Children(func() {
		ui.Row(c).Gap(14).AlignItems(ui.Center).Children(func() {
			nativeAvatar(c, m, p, 48)
			ui.Column(c).Grow(1).Basis(0).MinWidth(0).Gap(4).Children(func() {
				ui.Text(c, nativeAccountStatus(m, a)).FontSize(16).FontWeight(700).MaxLines(2).Ellipsis("…")
				if detail != "" {
					ui.Text(c, detail).FontSize(13).LineHeight(1.4).TextColor(p.secondary)
				}
			})
			if !stack {
				ui.Row(c).Shrink(0).Gap(8).AlignItems(ui.Center).Children(actions)
			}
		})
		if stack {
			ui.Row(c).Padding(0, 0, 0, 62).Gap(8).Wrap().AlignItems(ui.Center).Children(actions)
		}
	})
}

// nativeSignOutRow is the destructive account action, set apart in its own
// row. It is nil when there is no session to clear.
func nativeSignOutRow(c *ui.Context, m *UIModel, a ViewActions, p nativePalette) func() {
	if !m.SignedIn && !nativeRestoreDeferred(m) {
		return nil
	}
	return func() {
		label := a.Tr("shellSignOut")
		// Inset by 4 so the focus ring is not clipped by the card.
		b := ui.ButtonBase(c).Margin(4).Padding(8, 12).Gap(16).Radius(8).AlignItems(ui.Center).Justify(ui.Start).Label(label).TextColor(p.accentText)
		if b.Hovered() {
			b.Background(p.hover)
		}
		b.Children(func() {
			nativeSettingTile(c, "logout", p.accent.Alpha(0.14), p.accent)
			ui.Text(c, label).FontWeight(600)
		})
		if b.Clicked() {
			m.Settings = false
			a.SignOut()
		}
	}
}

// nativeAppearanceControl is a segmented control: one Tab stop whose arrows
// choose, announced as a radio group. The chosen segment is inverted, as
// selected chips are. Labels stay on one line; narrow, the segments share
// the row.
func nativeAppearanceControl(c *ui.Context, m *UIModel, a ViewActions, p nativePalette, narrow bool) {
	options := []struct{ id, key, icon string }{{"system", "shellThemeSystem", "monitor"}, {"dark", "shellThemeDark", "moon"}, {"light", "shellThemeLight", "sun"}}
	chosen := -1
	for i, option := range options {
		if m.Appearance == option.id {
			chosen = i
		}
	}
	index := max(chosen, 0)
	seg := ui.SegmentedBase(c, &index, len(options))
	seg.Track.Padding(3).Gap(2).Radius(10).Background(p.input).Label(a.Tr("shellAppearance"))
	if narrow {
		seg.Track.Grow(1)
	}
	seg.Track.Children(func() {
		for i, option := range options {
			label := a.Tr(option.key)
			b := seg.Segment(i).Height(32).PaddingX(10).Gap(6).Radius(8).Label(label).TextColor(p.secondary).Background(ui.Transparent)
			if narrow {
				b.Grow(1).Basis(0).MinWidth(0)
			}
			if i == chosen {
				b.Background(p.text).TextColor(p.base)
			} else if b.Hovered() {
				b.Background(p.hover).TextColor(p.text)
			}
			b.Children(func() {
				ui.Icon(c, nativeIcons[option.icon]).Size(16, 16).Shrink(0)
				ui.Text(c, label).FontSize(13).FontWeight(600).SingleLine().MinWidth(0).Ellipsis("…")
			})
		}
	})
	if index != chosen {
		a.Appearance(options[index].id)
	}
}

// nativeLanguageControl is a drop-down listing each language in itself. Its
// value is the name shown, so assistive technology reads the name.
func nativeLanguageControl(c *ui.Context, m *UIModel, a ViewActions, p nativePalette, narrow bool) {
	current := nativeLanguageTitle(a, m.Language)
	chosen := current
	sel := ui.SelectBase(c, &chosen)
	sel.Trigger.Height(36).MinWidth(200).PaddingX(14).Gap(10).Radius(8).AlignItems(ui.Center).Background(p.input).TextColor(p.text).Label(a.Tr("shellLanguage"))
	if narrow {
		sel.Trigger.Grow(1)
	}
	if sel.Trigger.Hovered() || sel.Open() {
		sel.Trigger.Background(p.highlight)
	}
	sel.Trigger.Children(func() {
		ui.Text(c, current).Grow(1).MinWidth(0).MaxLines(1).Ellipsis("…")
		ui.Icon(c, nativeIcons["chevron-down"]).Size(16, 16).TextColor(p.secondary)
	})
	sel.Popup(func(panel ui.Element) {
		panel.Margin(6, 0, 0, 0).Padding(4).Radius(10).Background(p.raised).Border(1, p.line).Shadow(0, 12, 32, 0, ui.RGBA(0, 0, 0, 0.35))
		for _, option := range nativeLanguages {
			title := nativeLanguageTitle(a, option.id)
			item := sel.Item(title).Height(36).PaddingX(10).Gap(10).Radius(6).AlignItems(ui.Center).TextColor(p.text)
			if item.Highlighted() {
				item.Background(p.hover)
			}
			item.Children(func() {
				ui.Text(c, title).Grow(1)
				check := ui.Icon(c, nativeIcons["check"]).Size(16, 16).TextColor(p.accent)
				if option.id != m.Language {
					check.Opacity(0)
				}
			})
		}
	})
	if chosen != current {
		for _, option := range nativeLanguages {
			if nativeLanguageTitle(a, option.id) == chosen {
				a.Language(option.id)
			}
		}
	}
}

// nativeShortcuts lists the in-app shortcuts beside the global one, which
// Settings configures.
func nativeShortcuts(c *ui.Context, m *UIModel, a ViewActions, p nativePalette, width, height float32) {
	nativeSheet(c, &m.Shortcuts, &m.shortcutsScroll, a.Tr("shellShortcuts"), a, p, width, height, nativeShortcutsWidth, func(narrow bool) {
		ui.Text(c, a.Tr("nativeShortcutLocalHint")).FontSize(13).LineHeight(1.5).PaddingX(4).TextColor(p.secondary)
		nativeSettingsGroup(c, p, "", 64, func() {
			nativeSettingRow(c, p, narrow, "play", a.Tr("nativeShortcutGlobal"), "", func() {
				if m.GlobalShortcut == disabledGlobalShortcut {
					ui.Text(c, a.Tr("nativeShortcutDisabled")).FontWeight(600).TextColor(p.secondary)
				} else {
					nativeKeycaps(c, p, nativeShortcutValue(m, a))
				}
				if nativeLinkButton(c, a.Tr("nativeShortcutConfigure"), p, "forward").Clicked() {
					m.Shortcuts = false
					m.Settings = true
					m.revealShortcut = true
				}
			})
		})
		rows := []func(){}
		for _, shortcut := range []struct{ key, chord string }{{"shellShortcutPlayPause", "Space"}, {"shellShortcutNextPrevious", "CmdOrCtrl+Right / CmdOrCtrl+Left"}, {"shellShortcutVolume", "CmdOrCtrl+Up / CmdOrCtrl+Down"}, {"shellShortcutSearch", "CmdOrCtrl+K"}, {"shellShortcutPanel", "CmdOrCtrl+J"}, {"shellShortcutImmersive", "CmdOrCtrl+Shift+F"}, {"shellShortcutHome", "CmdOrCtrl+Shift+H"}, {"shellShortcutLibrary", "CmdOrCtrl+L"}, {"shellShortcutMute", "CmdOrCtrl+Shift+M"}, {"shellShortcutSidebar", "CmdOrCtrl+B"}, {"shellShortcutDismiss", "Escape"}} {
			rows = append(rows, func() {
				ui.Row(c).MinHeight(48).Padding(8, 16).Gap(12).AlignItems(ui.Center).Children(func() {
					ui.Text(c, a.Tr(shortcut.key)).Grow(1).MinWidth(0)
					nativeKeycaps(c, p, nativeLocalChord(shortcut.chord))
				})
			})
		}
		nativeSettingsGroup(c, p, "", 16, rows...)
	})
}

// nativeLocalChord spells in-app accelerators, " / " between alternatives.
func nativeLocalChord(chords string) string {
	alternatives := strings.Split(chords, " / ")
	for i, chord := range alternatives {
		alternatives[i] = shortcutDisplay(chord)
	}
	return strings.Join(alternatives, " / ")
}
