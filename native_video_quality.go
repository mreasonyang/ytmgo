package main

import (
	"github.com/egoist/mygo/ui"
	"ytmgo/internal/videoquality"
)

// The player and Settings edit the same saved preference.
func nativeVideoQualityControl(c *ui.Context, m *UIModel, a ViewActions, p nativePalette, width, height float32) {
	current := videoquality.Label(m.VideoQuality)
	chosen := current
	sel := ui.SelectBase(c, &chosen)
	sel.Trigger.Height(height).Width(width).PaddingX(12).Gap(8).Radius(8).AlignItems(ui.Center).Background(p.input).TextColor(p.text).Label(a.Tr("nativeVideoQuality")).Disabled(a.SetVideoQuality == nil)
	if m.VideoQualityError != "" {
		sel.Trigger.Tooltip(a.Tr(m.VideoQualityError))
	}
	if sel.Trigger.Hovered() || sel.Open() {
		sel.Trigger.Background(p.highlight)
	}
	sel.Trigger.Children(func() {
		ui.Text(c, current).Grow(1).MinWidth(0).SingleLine()
		ui.Icon(c, nativeIcons["chevron-down"]).Size(16, 16).TextColor(p.secondary)
	})
	sel.Popup(func(panel ui.Element) {
		panel.Margin(6, 0, 0, 0).Padding(4).Radius(10).Background(p.raised).Border(1, p.line).Shadow(0, 12, 32, 0, ui.RGBA(0, 0, 0, 0.35))
		for _, quality := range videoquality.Options() {
			label := videoquality.Label(quality)
			item := sel.Item(label).Height(36).PaddingX(10).Gap(10).Radius(6).AlignItems(ui.Center).TextColor(p.text)
			if item.Highlighted() {
				item.Background(p.hover)
			}
			item.Children(func() {
				ui.Text(c, label).Grow(1)
				check := ui.Icon(c, nativeIcons["check"]).Size(16, 16).TextColor(p.text)
				if label != current {
					check.Opacity(0)
				}
			})
		}
	})
	if chosen != current && a.SetVideoQuality != nil {
		for _, quality := range videoquality.Options() {
			if videoquality.Label(quality) == chosen {
				a.SetVideoQuality(quality)
				break
			}
		}
	}
}
