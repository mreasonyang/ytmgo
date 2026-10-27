package main

import (
	"encoding/json"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/i18n"
	"ytmgo/internal/language"
	"ytmgo/locales"
)

// shellI18N is the third argument of the injected shell: its messages in the
// language the app shows, formatted with each simple argument left as a
// literal {name} slot for the shell to fill (locales/*.json "shell*" keys
// use no plural/select arguments; TestShellMessagesAreSimple enforces it).
type shellI18N struct {
	Locale    string            `json:"locale"`
	Script    string            `json:"script"`
	Choice    string            `json:"choice"`
	Languages []shellLanguage   `json:"languages"`
	Messages  map[string]string `json:"messages"`
}

type shellLanguage struct {
	Tag  string `json:"tag"`
	Name string `json:"name"`
}

func shellMessageKeys() []string {
	raw, _ := assets.ReadFile("locales/en.json")
	var catalog map[string]json.RawMessage
	_ = json.Unmarshal(raw, &catalog)
	var keys []string
	for key := range catalog {
		if strings.HasPrefix(key, "shell") {
			keys = append(keys, key)
		}
	}
	return keys
}

func newShellI18N(locale, choice string) shellI18N {
	payload := shellI18N{Locale: locale, Script: language.Script(locale), Choice: choice, Messages: map[string]string{}}
	for _, tag := range language.Supported {
		payload.Languages = append(payload.Languages, shellLanguage{Tag: tag, Name: i18n.Autonym(tag)})
	}
	for _, key := range shellMessageKeys() {
		var args map[string]any
		if m, _, ok := locales.Catalog.Message(locale, key); ok {
			for _, arg := range m.Args() {
				if args == nil {
					args = map[string]any{}
				}
				args[arg.Name] = "{" + arg.Name + "}"
			}
		}
		payload.Messages[key] = locales.Catalog.Format(locale, key, args)
	}
	return payload
}

// applyLanguage makes the saved choice the app's language. MyGo matches
// the system languages against the catalogs for "system" (zh-TW → zh-Hant).
func (a *desktop) applyLanguage() {
	choice := language.Load(a.languagePath)
	if choice == language.System {
		mygo.App.SetLocale("")
	} else {
		mygo.App.SetLocale(choice)
	}
}

func (a *desktop) setLanguage(choice string) {
	if err := language.Save(a.languagePath, choice); err != nil {
		return
	}
	a.applyLanguage()
}

// relocalize runs when the shown language changes: native menus and window
// titles are rebuilt, and the music view reloads so both the shell and the
// service canvas (its hl) speak the new language.
func (a *desktop) relocalize() {
	mygo.App.SetMenu(a.menu())
	a.setSignedIn(a.signedIn)
	if a.tray != nil {
		a.tray.SetMenu(a.trayMenu())
	}
	if a.account != nil && !a.account.IsDestroyed() {
		a.account.SetTitle(locales.LoginWindowTitle())
	}
	if a.logoutBlocked {
		if a.logoutWindow != nil && !a.logoutWindow.IsDestroyed() {
			a.logoutWindow.SetTitle(locales.AccountWindowTitle())
		}
		return
	}
	if a.window != nil && !a.window.IsDestroyed() {
		a.window.Destroy()
		a.window = nil
		a.openMusic()
		a.show()
	}
}
