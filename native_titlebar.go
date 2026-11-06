package main

import "github.com/egoist/mygo/ui"

// Linux and Windows main windows hide the system title bar (see
// mainWindowTitleBar), so the view's top bar is the title bar: it drags the
// window, keeps clear of the window controls that c.TitleBar() reports, and
// carries the application menu the hidden title bar leaves no room for.
// macOS keeps its title bar and menu bar; c.TitleBar() is zero there and
// AppMenu nil, which leave its layout as it was.

// nativeCompactSearchMin is the narrowest search field the compact bar keeps
// beside the window controls before it moves to a row of its own.
const nativeCompactSearchMin = 160

// nativeTitleBarInsets returns the horizontal padding of a bar along the top
// of the window: pad, or past the window controls on the sides they take.
func nativeTitleBarInsets(bar ui.TitleBar, pad float32) (left, right float32) {
	left, right = pad, pad
	if bar.Left > 0 {
		left = bar.Left + 8
	}
	if bar.Right > 0 {
		right = bar.Right + 8
	}
	return left, right
}

// nativeTitleBarRow is a bar along the top of the window, which drags the
// window under a hidden title bar. Its buttons and fields keep their clicks.
func nativeTitleBarRow(c *ui.Context, bar ui.TitleBar) ui.Element {
	row := ui.Row(c).Shrink(0)
	if bar.Height > 0 {
		row.DragWindow()
	}
	return row
}

// nativeAppMenuButton opens the application menu (ViewActions.AppMenu), from
// the top-left corner, where Alt and F10 open it on Windows.
func nativeAppMenuButton(c *ui.Context, a ViewActions, p nativePalette, hit float32) {
	if a.AppMenu == nil {
		return
	}
	nativeIconButton(c, "more", a.Tr("shellAppMenu"), p, false, hit).Menu(a.AppMenu)
}
