package main

import (
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/i18n"
	"ytmgo/internal/language"
	"ytmgo/locales"
)

func (a *desktop) tr(key string) string { return a.trArgs(key, nil) }
func (a *desktop) trArgs(key string, args map[string]any) string {
	return locales.Catalog.Format(mygo.App.Locale(), key, args)
}
func (a *desktop) applyLanguage() {
	choice := language.Load(a.languagePath)
	if choice == language.System {
		mygo.App.SetLocale("")
	} else {
		mygo.App.SetLocale(choice)
	}
}
func (a *desktop) setLanguage(choice string) {
	if language.Save(a.languagePath, choice) != nil {
		return
	}
	a.model.Language = choice
	a.applyLanguage()
	// A preference change can resolve to the same locale; always refresh it.
	a.localeChanged()
}
func (a *desktop) localeChanged() {
	if a.catalog != nil {
		a.catalog.SetLanguage(language.ServiceLanguage(mygo.App.Locale()))
		if a.window != nil {
			a.reload()
		}
	}
	a.relocalize()
}
func (a *desktop) relocalize() {
	mygo.App.SetMenu(a.menu())
	if a.tray != nil {
		a.tray.SetMenu(a.trayMenu())
	}
	if a.window != nil {
		a.window.SetTitle(locales.AppName())
		a.invalidate()
	}
}
func (a *desktop) setAppearance(value string) {
	if _, err := a.prefs.SetAppearance(value); err != nil {
		a.model.Error = a.tr("appearanceSaveError")
		a.model.ErrorScope = ""
	} else {
		a.model.Appearance = value
	}
	a.invalidate()
}
func (a *desktop) languageMenu() []*mygo.MenuItem {
	choice := language.Load(a.languagePath)
	radio := func(label, value string) *mygo.MenuItem {
		return &mygo.MenuItem{Label: label, Type: mygo.MenuItemRadio, Checked: choice == value, Click: func(*mygo.MenuItem, *mygo.Window) { a.setLanguage(value) }}
	}
	items := []*mygo.MenuItem{radio(locales.MenuLanguageSystem(), language.System), mygo.Separator()}
	for _, tag := range language.Supported {
		items = append(items, radio(i18n.Autonym(tag), tag))
	}
	return items
}
