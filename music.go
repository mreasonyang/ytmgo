package main

import (
	"context"
	"encoding/json"
	"runtime"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"ytmgo/internal/language"
	"ytmgo/internal/navigation"
	"ytmgo/locales"
)

// musicPreload assembles the shell from web/shell/bundle.json: every script
// shares one closure (in manifest order) and receives the stylesheet and the
// saved appearance and its localized messages as arguments. web/shell/bundle.mjs mirrors this for tests.
func musicPreload(mode string, text shellI18N) string {
	var manifest struct{ Styles, Scripts []string }
	raw, _ := assets.ReadFile("web/shell/bundle.json")
	_ = json.Unmarshal(raw, &manifest)
	var styles, scripts strings.Builder
	for _, name := range manifest.Styles {
		data, _ := assets.ReadFile("web/shell/" + name)
		styles.Write(data)
		styles.WriteByte('\n')
	}
	for _, name := range manifest.Scripts {
		data, _ := assets.ReadFile("web/shell/" + name)
		scripts.WriteString("/* " + name + " */\n")
		scripts.Write(data)
		scripts.WriteByte('\n')
	}
	encodedStyles, _ := json.Marshal(styles.String())
	encodedMode, _ := json.Marshal(mode)
	encodedI18N, _ := json.Marshal(text)
	return "(function ytmgoShell(STYLES, APPEARANCE, I18N) {\n'use strict';\n" + scripts.String() + "\n})(" + string(encodedStyles) + ", " + string(encodedMode) + ", " + string(encodedI18N) + ");\n"
}

func musicUserAgentSuffix() string {
	// Declare WebKit compatibility while retaining the engine's default UA.
	// Bare WKWebView otherwise receives the deprecated-browser page.
	suffix := "ytmgo/" + version
	if runtime.GOOS == "darwin" {
		suffix = "Version/17.0 Safari/605.1.15 " + suffix
	}
	return suffix
}

func (a *desktop) openMusic() {
	if a.logoutBlocked {
		a.show()
		return
	}
	a.setSignedIn(false)
	a.window = mygo.NewWindow(mygo.WindowOptions{
		// The shell is responsive down to a phone-width compact layout.
		Title: locales.AppName(), Width: 1440, Height: 940, MinWidth: 380, MinHeight: 640,
		BackgroundColor: "light-dark(#e7e7ea, #050506)", StateKey: "main",
		Page: mygo.PageOptions{PreloadScript: musicPreload(a.prefs.Appearance(), newShellI18N(mygo.App.Locale(), language.Load(a.languagePath))), UserAgentSuffix: musicUserAgentSuffix(), DevTools: mygo.DevToolsAuto},
	})
	w := a.window
	p := w.Page()
	p.OnPageTitleUpdated(func(e *mygo.TitleEvent) {
		e.PreventDefault()
		w.SetTitle(strings.ReplaceAll(e.Title, locales.ServiceName(), locales.AppName()))
	})
	p.OnWillNavigate(func(e *mygo.NavigateEvent) {
		if a.logoutBlocked {
			e.PreventDefault()
			return
		}
		if signedIn, ok := navigation.AccountStateAction(e.URL, p.URL()); ok {
			e.PreventDefault()
			a.setSignedIn(signedIn)
			return
		}
		if navigation.SignOutAction(e.URL, p.URL(), e.UserInitiated) {
			e.PreventDefault()
			a.requestSignOut()
			return
		}
		if choice, ok := navigation.LanguageAction(e.URL, p.URL()); ok {
			e.PreventDefault()
			a.setLanguage(choice)
			return
		}
		if mode, ok := navigation.AppearanceAction(e.URL, p.URL()); ok {
			e.PreventDefault()
			go func() {
				saved, err := a.prefs.SetAppearance(mode)
				if err != nil {
					saved = a.prefs.Appearance()
				}
				a.syncMusicAppearance(saved)
			}()
			return
		}
		if navigation.MusicPage(e.URL) {
			if !navigation.MusicContent(e.URL) {
				e.PreventDefault()
				a.openAccount(e.URL)
			}
			return
		}
		e.PreventDefault()
		if e.UserInitiated {
			a.external(e.URL)
		}
	})
	p.SetWindowOpenHandler(func(req mygo.WindowOpenRequest) *mygo.WindowOptions {
		if a.logoutBlocked {
			return nil
		}
		if navigation.MusicPage(req.URL) {
			if navigation.MusicContent(req.URL) {
				p.LoadURL(req.URL)
			} else {
				a.openAccount(req.URL)
			}
		} else {
			a.external(req.URL)
		}
		return nil
	})
	p.SetPermissionHandler(func(mygo.PermissionRequest) bool { return false })
	p.OnWillDownload(func(e *mygo.DownloadEvent) { e.PreventDefault() })
	p.OnDOMReady(func() { a.syncMusicAppearance(a.prefs.Appearance()) })
	p.OnDidFailLoad(func(e *mygo.LoadError) {
		// Cancellation is normal during redirects. Never log account URLs/errors.
		if e.Code == -999 || e.Code == -3 || w.IsDestroyed() || a.logoutBlocked {
			return
		}
		go func() {
			result, _ := mygo.Dialog.Message(mygo.MessageOptions{Parent: w, Title: locales.AppName(), Message: locales.LoadErrorMessage(), Detail: locales.LoadErrorDetail(), Buttons: []string{locales.ButtonRetry(), locales.ButtonGoHome(), locales.ButtonLater()}})
			if result.Button == 0 {
				p.Reload()
			} else if result.Button == 1 {
				p.LoadURL(navigation.Home + "?hl=" + language.ServiceLanguage(mygo.App.Locale()))
			}
		}()
	})
	p.OnRenderProcessGone(func(string) {
		if w.IsDestroyed() || a.logoutBlocked {
			return
		}
		go func() {
			result, _ := mygo.Dialog.Message(mygo.MessageOptions{Parent: w, Title: locales.AppName(), Message: locales.CrashMessage(), Detail: locales.CrashDetail(), Buttons: []string{locales.ButtonReload(), locales.ButtonLater()}})
			if result.Button == 0 {
				p.LoadURL(navigation.Home)
			}
		}()
	})
	w.OnClose(func(e *mygo.CloseEvent) {
		if !a.quitting && (a.tray != nil || runtime.GOOS == "darwin") {
			e.PreventDefault()
			w.Hide()
		}
	})
	p.LoadURL(navigation.Home + "?hl=" + language.ServiceLanguage(mygo.App.Locale()))
}

// A separate first-party login window shares the runtime's persistent session.
// The app never reads or transfers cookies, passwords or authorization data.
func (a *desktop) openAccount(raw string) {
	if a.logoutBlocked {
		a.show()
		return
	}
	if !navigation.MusicPage(raw) {
		return
	}
	if a.account != nil && !a.account.IsDestroyed() {
		a.account.Show()
		a.account.Focus()
		return
	}
	w := mygo.NewWindow(mygo.WindowOptions{
		Title: locales.LoginWindowTitle(), Width: 980, Height: 760, MinWidth: 760, MinHeight: 640,
		Page: mygo.PageOptions{UserAgentSuffix: musicUserAgentSuffix()},
	})
	a.account = w
	p := w.Page()
	p.OnPageTitleUpdated(func(e *mygo.TitleEvent) {
		e.PreventDefault()
		w.SetTitle(locales.LoginWindowTitle())
	})
	p.OnWillNavigate(func(e *mygo.NavigateEvent) {
		if a.logoutBlocked {
			e.PreventDefault()
			return
		}
		if navigation.MusicPage(e.URL) {
			return
		}
		e.PreventDefault()
		if e.UserInitiated {
			a.external(e.URL)
		}
	})
	p.SetWindowOpenHandler(func(req mygo.WindowOpenRequest) *mygo.WindowOptions {
		if a.logoutBlocked {
			return nil
		}
		if navigation.MusicPage(req.URL) {
			p.LoadURL(req.URL)
		} else {
			a.external(req.URL)
		}
		return nil
	})
	p.SetPermissionHandler(func(mygo.PermissionRequest) bool { return false })
	p.OnWillDownload(func(e *mygo.DownloadEvent) { e.PreventDefault() })
	p.OnDidFinishLoad(func() {
		if a.logoutBlocked || w.IsDestroyed() {
			return
		}
		if navigation.MusicContent(p.URL()) {
			w.Close()
			a.window.Page().Reload()
			a.show()
		}
	})
	p.OnDidFailLoad(func(e *mygo.LoadError) {
		if e.Code == -999 || e.Code == -3 || w.IsDestroyed() || a.logoutBlocked {
			return
		}
		go func() {
			result, _ := mygo.Dialog.Message(mygo.MessageOptions{Parent: w, Title: locales.LoginErrorTitle(), Message: locales.LoginErrorMessage(), Detail: locales.LoginErrorDetail(), Buttons: []string{locales.ButtonRetry(), locales.ButtonLater()}})
			if result.Button == 0 && !w.IsDestroyed() {
				p.Reload()
			}
		}()
	})
	w.OnClose(func(*mygo.CloseEvent) { a.account = nil })
	p.LoadURL(raw)
}

func (a *desktop) syncMusicAppearance(mode string) {
	if a.logoutBlocked || a.window == nil || a.window.IsDestroyed() {
		return
	}
	encoded, _ := json.Marshal(mode)
	w := a.window
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_, _ = w.Page().EvalContext(ctx, `location.origin === 'https://music.youtube.com' && window.ytmgoSetAppearance?.(`+string(encoded)+`)`)
	}()
}
