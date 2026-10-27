package main

import (
	"html"

	"github.com/egoist/mygo"
	"ytmgo/internal/language"
	"ytmgo/internal/session"
	"ytmgo/locales"
)

func (a *desktop) setSignedIn(signedIn bool) {
	a.signedIn = signedIn
	if a.signOutItem != nil {
		a.signOutItem.SetVisible(signedIn)
	}
	if a.accountSeparator != nil {
		a.accountSeparator.SetVisible(signedIn)
	}
	if a.accountItem != nil {
		label := locales.MenuSignIn()
		if signedIn {
			label = locales.MenuManageAccount()
		}
		a.accountItem.SetLabel(label)
	}
}

// Window and logout state is accessed on the UI thread. The native store clear
// runs in a goroutine; its errors are never printed because runtime diagnostics
// can contain account URLs. No cookie enumeration or token extraction is used.
func (a *desktop) requestSignOut() {
	if a.logoutBlocked {
		a.show()
		return
	}
	if !a.signedIn {
		return
	}
	if a.logoutBusy {
		return
	}
	a.logoutBusy = true
	parent := a.window
	go func() {
		result, err := mygo.Dialog.Message(mygo.MessageOptions{
			Parent: parent, Type: mygo.MessageQuestion, Title: locales.SignOutTitle(),
			Message: locales.SignOutMessage(),
			Detail:  locales.SignOutDetail(),
			Buttons: []string{locales.ButtonCancel(), locales.SignOutConfirm()}, DefaultButton: 0, CancelButton: 0,
		})
		mygo.RunOnMain(func() {
			a.logoutBusy = false
			if err == nil && result.Button == 1 && !a.quitting {
				a.clearAccountData()
			}
		})
	}()
}

func (a *desktop) clearAccountData() {
	if a.logoutBusy || a.quitting {
		return
	}
	a.logoutBusy, a.logoutBlocked, a.logoutComplete = true, true, false
	a.setSignedIn(false)
	if a.logoutWindow == nil || a.logoutWindow.IsDestroyed() {
		w := mygo.NewWindow(mygo.WindowOptions{
			Title: locales.AccountWindowTitle(), Width: 660, Height: 460, MinWidth: 580, MinHeight: 420,
			BackgroundColor: "light-dark(#e7e7ea, #050506)",
		})
		a.logoutWindow = w
		a.logoutLoading, a.logoutNextHTML = false, ""
		w.Page().OnDidFinishLoad(func() {
			a.logoutLoading = false
			if next := a.logoutNextHTML; next != "" {
				a.logoutNextHTML = ""
				a.loadLogoutHTML(next)
			}
		})
		w.Page().OnWillNavigate(func(e *mygo.NavigateEvent) {
			e.PreventDefault()
			if !e.UserInitiated || a.logoutBusy {
				return
			}
			switch e.URL {
			case "ytmgo://session/retry":
				if !a.logoutComplete {
					a.clearAccountData()
				}
			case "ytmgo://session/continue":
				if a.logoutComplete {
					a.logoutBlocked = false
					a.openMusic()
					w.Destroy()
					a.logoutWindow = nil
				}
			}
		})
		w.Page().SetWindowOpenHandler(func(mygo.WindowOpenRequest) *mygo.WindowOptions { return nil })
		w.Page().SetPermissionHandler(func(mygo.PermissionRequest) bool { return false })
		w.Page().OnWillDownload(func(e *mygo.DownloadEvent) { e.PreventDefault() })
		w.OnClose(func(e *mygo.CloseEvent) {
			if a.logoutBusy && !a.quitting {
				e.PreventDefault()
				return
			}
			if !a.quitting {
				e.PreventDefault()
				mygo.App.Quit()
			}
		})
	}
	a.logoutPage(locales.LogoutBusyTitle(), locales.LogoutBusyDetail(), "", "")
	a.show()
	go func() {
		err := session.Clear(a.sessionMarker, func() {
			mygo.RunOnMain(a.closeSessionPages)
		}, mygo.App.ClearBrowsingData)
		mygo.RunOnMain(func() {
			a.logoutBusy = false
			if a.quitting {
				return
			}
			if err != nil {
				a.closeSessionPages()
				a.logoutPage(locales.LogoutFailedTitle(), locales.LogoutFailedDetail(), locales.LogoutRetry(), "retry")
				return
			}
			a.logoutComplete = true
			a.logoutPage(locales.LogoutDoneTitle(), locales.LogoutDoneDetail(), locales.LogoutContinue(), "continue")
		})
	}()
}

func (a *desktop) closeSessionPages() {
	// Destroy every page sharing the store before clearing it. Merely
	// reloading afterwards leaves live sessions able to write back.
	for _, w := range []*mygo.Window{a.account, a.window} {
		if w != nil && !w.IsDestroyed() {
			w.Destroy()
		}
	}
	a.account, a.window = nil, nil
}

// logoutStyle mirrors web/shell/tokens.css: base gutter, panel surface,
// type scale, pill action and the shared focus ring.
const logoutStyle = `:root{color-scheme:light dark;--base:light-dark(#e7e7ea,#050506);--surface:light-dark(#fff,#121214);--text:light-dark(#121214,#f5f5f7);--text2:light-dark(#55555c,#a8a8af);--accent:light-dark(#e8382d,#ff5a4f);--onaccent:light-dark(#fff,#0b0b0c)}
*{box-sizing:border-box}html,body{height:100%}body{margin:0;padding:8px;background:var(--base);color:var(--text);font:500 15px/1.5 -apple-system,BlinkMacSystemFont,"SF Pro Text","PingFang SC","Noto Sans CJK SC",system-ui,sans-serif;-webkit-font-smoothing:antialiased}
main{height:100%;display:grid;align-content:center;gap:16px;padding:40px;border-radius:12px;background:var(--surface)}
.mark{width:40px;height:40px;border-radius:11px;background:var(--accent);display:grid;place-items:center}.mark i{width:0;height:0;margin-left:3px;border-left:13px solid var(--onaccent);border-top:8px solid transparent;border-bottom:8px solid transparent}
small{font:700 11px/1.3 inherit;letter-spacing:.08em;text-transform:uppercase;color:var(--text2)}h1{margin:0;font:760 32px/1.12 -apple-system,BlinkMacSystemFont,"SF Pro Display",system-ui,sans-serif;letter-spacing:-.025em}p{margin:0 0 8px;color:var(--text2);max-width:46ch}
a{justify-self:start;display:inline-flex;align-items:center;height:40px;padding:0 22px;border-radius:999px;background:var(--text);color:var(--surface);text-decoration:none;font-weight:700;transition:transform .24s cubic-bezier(.34,1.4,.64,1)}a:hover{transform:scale(1.04)}a:active{transform:scale(.97)}a:focus-visible{outline:2px solid var(--text);outline-offset:3px}
.busy{width:22px;height:22px;border-radius:50%;border:2px solid color-mix(in srgb,var(--text) 20%,transparent);border-top-color:var(--text);animation:spin .9s linear infinite}@keyframes spin{to{transform:rotate(360deg)}}
html[data-script=hans] body{font-family:-apple-system,BlinkMacSystemFont,"PingFang SC","Noto Sans CJK SC","Microsoft YaHei UI",system-ui,sans-serif}
html[data-script=hant] body{font-family:-apple-system,BlinkMacSystemFont,"PingFang TC","Noto Sans CJK TC","Microsoft JhengHei UI",system-ui,sans-serif}
html[data-script=jpan] body{font-family:-apple-system,BlinkMacSystemFont,"Hiragino Sans","Noto Sans CJK JP","Yu Gothic UI",system-ui,sans-serif}
html:not([data-script=latn]) body{line-height:1.7;line-break:strict;text-autospace:normal}html:not([data-script=latn]) h1{letter-spacing:0;line-height:1.3;font-feature-settings:"palt"}html:not([data-script=latn]) small{letter-spacing:.04em}
@media (prefers-reduced-motion:reduce){*{animation:none!important;transition:none!important}}`

func (a *desktop) logoutPage(title, detail, label, action string) {
	colorScheme := "light dark"
	if mode := a.prefs.Appearance(); mode == "light" || mode == "dark" {
		colorScheme = mode
	}
	control := `<div class="busy" role="progressbar" aria-label="` + html.EscapeString(locales.LogoutBusyLabel()) + `"></div>`
	if action != "" {
		control = `<a href="ytmgo://session/` + html.EscapeString(action) + `">` + html.EscapeString(label) + `</a>`
	}
	a.loadLogoutHTML(`<!doctype html><html lang="` + html.EscapeString(mygo.App.Locale()) + `" data-script="` + language.Script(mygo.App.Locale()) + `" style="color-scheme:` + colorScheme + `"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1"><meta http-equiv="Content-Security-Policy" content="default-src 'none'; style-src 'unsafe-inline'"><style>` + logoutStyle + `</style></head><body><main><div class="mark" aria-hidden="true"><i></i></div><small>` + html.EscapeString(locales.LogoutEyebrow()) + `</small><h1>` + html.EscapeString(title) + `</h1><p>` + html.EscapeString(detail) + `</p>` + control + `</main></body></html>`)
}

func (a *desktop) loadLogoutHTML(document string) {
	// A fast storage failure may arrive before the first document commits.
	// Serialize loads so the runtime's navigation guard does not cancel the
	// second document and leave the recovery window blank.
	if a.logoutLoading {
		a.logoutNextHTML = document
		return
	}
	a.logoutLoading = true
	a.logoutWindow.Page().LoadHTML(document, "https://session.ytmgo.invalid/")
}
