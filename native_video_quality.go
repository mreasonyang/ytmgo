package main

import (
	"github.com/egoist/mygo/ui"
	"ytmgo/internal/audioquality"
	"ytmgo/internal/videoquality"
)

func nativeAudioQualitySetting(c *ui.Context, m *UIModel, a ViewActions, p nativePalette, narrow bool) {
	detail := a.Tr("nativeAudioQualityHint")
	if m.AudioQualityNotice != "" {
		detail = a.Tr(m.AudioQualityNotice)
	}
	label := func(q audioquality.Quality) string {
		switch q {
		case audioquality.Saver:
			return a.Tr("nativeAudioQualitySaver")
		case audioquality.Standard:
			return a.Tr("nativeAudioQualityStandard")
		default:
			return a.Tr("nativeAudioQualityBest")
		}
	}
	title := a.Tr("nativeAudioQuality")
	hint := nativeSettingRow(c, p, narrow, "headphones", title, detail, func() {
		trigger := nativeQualityControl(c, m, p, title, m.AudioQuality.Normalize(), []audioquality.Quality{audioquality.Saver, audioquality.Standard, audioquality.Best}, label, a.SetAudioQuality, 200, 36)
		trigger.Description(detail)
	})
	switch m.AudioQualityNotice {
	case "nativeAudioQualitySaveError":
		hint.Role(ui.RoleAlert).TextColor(p.attention)
	case "nativeAudioQualitySaved":
		hint.Role(ui.RoleStatus)
	}
}

// The player and Settings edit the same video quality preference.
func nativeVideoQualityControl(c *ui.Context, m *UIModel, a ViewActions, p nativePalette, width, height float32) {
	trigger := nativeQualityControl(c, m, p, a.Tr("nativeVideoQuality"), videoquality.Normalize(m.VideoQuality), videoquality.Options(), videoquality.Label, a.SetVideoQuality, width, height)
	if m.VideoQualityError != "" {
		nativeTip(c, trigger, a.Tr(m.VideoQualityError), p)
	}
}

func nativeQualityControl[T comparable](c *ui.Context, m *UIModel, p nativePalette, title string, current T, options []T, label func(T) string, change func(T), width, height float32) ui.Element {
	chosen := current
	currentLabel := label(current)
	sel := ui.SelectBase(c, &chosen)
	sel.Trigger.Height(height).Width(width).PaddingX(12).Gap(8).Radius(tokenRadiusControl).AlignItems(ui.Center).Background(p.input).TextColor(p.text).Label(title).Value(currentLabel).Disabled(change == nil)
	if sel.Trigger.Hovered() || sel.Open() {
		sel.Trigger.Background(p.highlight)
	}
	sel.Trigger.Children(func() {
		ui.Text(c, currentLabel).Grow(1).MinWidth(0).SingleLine()
		ui.Icon(c, nativeIcons["chevron-down"]).Size(16, 16).TextColor(p.secondary)
	})
	sel.Popup(func(panel ui.Element) {
		nativeAppear(c, panel.Margin(8, 0, 0, 0).Padding(4).Radius(tokenRadiusControl).Background(p.raised).Border(1, p.line).Shadow(tokenShadowPopover.args(p)), m.instantMotion)
		for _, quality := range options {
			name := label(quality)
			item := sel.Item(quality).Label(name).Height(36).PaddingX(12).Gap(8).Radius(tokenRadiusArt).AlignItems(ui.Center).TextColor(p.text)
			if item.Highlighted() {
				item.Background(p.hover)
			}
			item.Children(func() {
				ui.Text(c, name).Grow(1)
				check := ui.Icon(c, nativeIcons["check"]).Size(16, 16).TextColor(p.text)
				if quality != current {
					check.Opacity(0)
				}
			})
		}
	})
	if chosen != current && change != nil {
		change(chosen)
	}
	return sel.Trigger
}
