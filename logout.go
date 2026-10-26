package main

import (
	"html"

	"github.com/egoist/mygo"
	"ytmgo/internal/session"
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
		label := "登录 Google 账号"
		if signedIn {
			label = "管理账号"
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
			Parent: parent, Type: mygo.MessageQuestion, Title: "退出登录",
			Message: "退出登录并清除本机账号数据？",
			Detail:  "将停止播放，关闭登录窗口，并清除 ytmgo 内所有账号的登录状态、Cookie、网页存储与缓存。\n外观设置、本地收藏和云端音乐库会保留，其他浏览器的登录不受影响。",
			Buttons: []string{"取消", "退出并清除"}, DefaultButton: 0, CancelButton: 0,
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
			Title: "账号 · ytmgo", Width: 660, Height: 460, MinWidth: 580, MinHeight: 420,
			BackgroundColor: "light-dark(#ecebe7, #101614)",
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
	a.logoutPage("正在退出登录", "正在清除本机账号数据，请稍候…", "", "")
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
				a.logoutPage("清理尚未完成", "请重试。清理完成前不会重新打开音乐页面；外观设置和本地收藏仍然保留。", "重试清理", "retry")
				return
			}
			a.logoutComplete = true
			a.logoutPage("已退出登录", "此应用内所有账号的登录状态、Cookie、网页存储和缓存已清除。外观设置、本地收藏及云端音乐库已保留。", "返回首页", "continue")
		})
	}()
}

func (a *desktop) closeSessionPages() {
	// Destroy every page sharing the store before clearing it. Merely
	// reloading afterwards leaves live sessions able to write back.
	for _, w := range []*mygo.Window{a.account, a.legacy, a.window} {
		if w != nil && !w.IsDestroyed() {
			w.Destroy()
		}
	}
	a.account, a.legacy, a.window = nil, nil, nil
}

func (a *desktop) logoutPage(title, detail, label, action string) {
	colorScheme := "light dark"
	if mode := a.service.Appearance(); mode == "light" || mode == "dark" {
		colorScheme = mode
	}
	button := ""
	if action != "" {
		button = `<a href="ytmgo://session/` + html.EscapeString(action) + `">` + html.EscapeString(label) + `</a>`
	}
	a.loadLogoutHTML(`<!doctype html><html lang="zh-CN" style="color-scheme:` + colorScheme + `"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1"><meta http-equiv="Content-Security-Policy" content="default-src 'none'; style-src 'unsafe-inline'"><style>
:root{color-scheme:light dark}*{box-sizing:border-box}body{margin:0;padding:54px 46px;background:light-dark(#ecebe7,#101614);color:light-dark(#192c24,#ecf1eb);font:15px/1.8 -apple-system,BlinkMacSystemFont,sans-serif}small{letter-spacing:2px;color:light-dark(#3b6952,#a0cdb5)}h1{font-size:28px;margin:18px 0 12px}p{opacity:.8;margin-bottom:28px}a{display:inline-block;padding:9px 20px;border-radius:9px;background:light-dark(#2c6247,#b2dbb9);color:light-dark(#fff,#101614);text-decoration:none;font-weight:600}a:focus-visible{outline:3px solid currentColor;outline-offset:4px}</style></head><body><small>ytmgo · 账号</small><h1>` + html.EscapeString(title) + `</h1><p>` + html.EscapeString(detail) + `</p>` + button + `</body></html>`)
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
