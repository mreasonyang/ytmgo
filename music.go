package main

import (
	"context"
	"encoding/json"
	"runtime"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"ytmgo/internal/navigation"
)

func musicPreload(mode string) string {
	css, _ := assets.ReadFile("web/music.css")
	js, _ := assets.ReadFile("web/music.js")
	encodedCSS, _ := json.Marshal(string(css))
	encodedMode, _ := json.Marshal(mode)
	return strings.NewReplacer("YTMGO_STYLES", string(encodedCSS), "YTMGO_APPEARANCE", string(encodedMode)).Replace(string(js))
}

func musicUserAgentSuffix() string {
	// Declare WebKit compatibility while retaining the engine's default UA.
	// Bare WKWebView otherwise receives the deprecated-browser page.
	suffix := "ytmgo/0.2.1"
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
		Title: "ytmgo", Width: 1440, Height: 940, MinWidth: 1000, MinHeight: 720,
		BackgroundColor: "light-dark(#ecebe7, #101614)", StateKey: "main",
		Page: mygo.PageOptions{PreloadScript: musicPreload(a.service.Appearance()), UserAgentSuffix: musicUserAgentSuffix(), DevTools: mygo.DevToolsAuto},
	})
	w := a.window
	p := w.Page()
	p.OnPageTitleUpdated(func(e *mygo.TitleEvent) {
		e.PreventDefault()
		w.SetTitle(strings.ReplaceAll(e.Title, "YouTube Music", "ytmgo"))
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
		if mode, ok := navigation.AppearanceAction(e.URL, p.URL()); ok {
			e.PreventDefault()
			go func() {
				saved, err := a.service.SetAppearance(mode)
				if err != nil {
					saved = a.service.Appearance()
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
	p.OnDOMReady(func() { a.syncMusicAppearance(a.service.Appearance()) })
	p.OnDidFailLoad(func(e *mygo.LoadError) {
		// Cancellation is normal during redirects. Never log account URLs/errors.
		if e.Code == -999 || e.Code == -3 || w.IsDestroyed() || a.logoutBlocked {
			return
		}
		go func() {
			result, _ := mygo.Dialog.Message(mygo.MessageOptions{Parent: w, Title: "ytmgo", Message: "暂时无法加载音乐服务", Detail: "请检查网络连接后重试。账号会话和音乐库不会被清除。", Buttons: []string{"重试", "回到首页", "稍后"}})
			if result.Button == 0 {
				p.Reload()
			} else if result.Button == 1 {
				p.LoadURL(navigation.Home)
			}
		}()
	})
	p.OnRenderProcessGone(func(string) {
		if w.IsDestroyed() || a.logoutBlocked {
			return
		}
		go func() {
			result, _ := mygo.Dialog.Message(mygo.MessageOptions{Parent: w, Title: "ytmgo", Message: "音乐界面意外停止", Detail: "重新加载后可继续使用。", Buttons: []string{"重新加载", "稍后"}})
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
	p.LoadURL(navigation.Home + "?hl=zh-CN")
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
		Title: "Google 登录 · ytmgo", Width: 980, Height: 760, MinWidth: 760, MinHeight: 640,
		Page: mygo.PageOptions{UserAgentSuffix: musicUserAgentSuffix()},
	})
	a.account = w
	p := w.Page()
	p.OnPageTitleUpdated(func(e *mygo.TitleEvent) {
		e.PreventDefault()
		w.SetTitle("Google 登录 · ytmgo")
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
			result, _ := mygo.Dialog.Message(mygo.MessageOptions{Parent: w, Title: "Google 登录", Message: "登录页面暂时无法加载", Detail: "请检查网络后重试，也可以关闭此窗口，稍后从主界面重新登录。", Buttons: []string{"重试", "稍后"}})
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
