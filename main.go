package main

import (
	"context"
	"embed"
	"encoding/json"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"ytmgo/internal/catalog"
	"ytmgo/internal/library"
	"ytmgo/internal/navigation"
	"ytmgo/internal/session"
)

//go:embed web/index.html web/style.css web/app.js web/theme.js web/music.js web/music.css resources/icon.png resources/tray.png
var assets embed.FS

const appOrigin = "https://si.kyun.ytmgo"

type desktop struct {
	window           *mygo.Window
	tray             *mygo.Tray
	quitting         bool
	html             string
	legacy           *mygo.Window
	account          *mygo.Window
	service          *Service
	sessionMarker    string
	logoutWindow     *mygo.Window
	logoutBusy       bool
	logoutBlocked    bool
	logoutComplete   bool
	logoutLoading    bool
	logoutNextHTML   string
	signedIn         bool
	accountItem      *mygo.MenuItem
	signOutItem      *mygo.MenuItem
	accountSeparator *mygo.MenuItem
}

func main() {
	preview := flag.Bool("preview", false, "serve a browser preview on loopback (separate local library)")
	flag.Parse()
	if *preview {
		s := &Service{catalog: catalog.NewClient(), library: library.NewStore(filepath.Join(".mygo", "preview", "library.json"))}
		if err := servePreview(s); err != nil {
			log.Fatal("ytmgo preview could not start")
		}
		return
	}
	// Preserve the CLI's separate packaged production/development identities.
	if !mygo.App.IsPackaged() {
		mygo.App.SetName("ytmgo Dev")
	}
	if !mygo.App.RequestSingleInstanceLock() {
		return
	}
	dataDir, err := mygo.App.Path(mygo.PathUserData)
	if err != nil {
		log.Fatal("ytmgo could not locate its application data directory")
	}
	s := &Service{catalog: catalog.NewClient(), library: library.NewStore(filepath.Join(dataDir, "library.json")), appearancePath: filepath.Join(dataDir, "appearance")}
	mygo.Bind(s)
	html, err := inlineFrontend()
	if err != nil {
		log.Fatal("ytmgo frontend assets are incomplete")
	}
	a := &desktop{html: html, service: s, sessionMarker: filepath.Join(dataDir, "signout-pending")}
	mygo.Theme.SetSource(mygo.ThemeSource(s.Appearance()))
	mygo.App.SetMenu(a.menu())
	mygo.App.OnBeforeQuit(func(*mygo.QuitEvent) { a.quitting = true })
	mygo.App.OnActivate(func(bool) { a.show() })
	mygo.App.OnSecondInstance(func([]string, string) { a.show() })
	mygo.App.OnWillQuit(func(*mygo.QuitEvent) { mygo.GlobalShortcut.UnregisterAll() })
	mygo.App.WhenReady(func() {
		pending, pendingErr := session.Pending(a.sessionMarker)
		if pending || pendingErr != nil {
			a.clearAccountData()
		} else {
			a.openMusic()
		}
		icon, _ := assets.ReadFile("resources/tray.png")
		a.tray, err = mygo.NewTray(mygo.TrayOptions{ID: "si.kyun.ytmgo.player", Icon: icon, IconIsTemplate: true, ToolTip: "ytmgo", Menu: a.trayMenu()})
		if err != nil {
			log.Print("ytmgo: tray unavailable; window and Dock controls remain available")
		}
		if err = mygo.GlobalShortcut.Register("CmdOrCtrl+Shift+Space", func() { a.control("toggle") }); err != nil {
			log.Print("ytmgo: global playback shortcut unavailable; use the Playback menu")
		}
	})
	if err = mygo.App.Run(); err != nil {
		log.Fatal("ytmgo could not start its desktop runtime")
	}
}

func (a *desktop) openLegacy() {
	if a.logoutBlocked {
		a.show()
		return
	}
	if a.legacy != nil && !a.legacy.IsDestroyed() {
		a.legacy.Show()
		a.legacy.Focus()
		return
	}
	a.legacy = mygo.NewWindow(mygo.WindowOptions{
		Title: "ytmgo · 本地收藏（旧版）", Width: 1440, Height: 940, MinWidth: 1000, MinHeight: 720,
		BackgroundColor: "light-dark(#f8f7f5, #141416)", StateKey: "legacy", Page: mygo.PageOptions{
			TrustedOrigins: []string{appOrigin}, DevTools: mygo.DevToolsAuto,
		},
	})
	p := a.legacy.Page()
	p.OnWillNavigate(func(e *mygo.NavigateEvent) {
		if e.URL == appOrigin+"/" || strings.HasPrefix(e.URL, appOrigin+"/#") {
			return
		}
		// WebKitGTK reports iframe src loads with no target frame name;
		// MyGo 0.6.4 consequently forwards them as page navigations. Permit
		// only the automatic, origin-bound official player on this backend.
		if runtime.GOOS == "linux" && navigation.EmbeddedPlayer(e.URL, appOrigin, e.UserInitiated) {
			return
		}
		e.PreventDefault()
		if e.UserInitiated {
			a.external(e.URL)
		}
	})
	p.SetWindowOpenHandler(func(req mygo.WindowOpenRequest) *mygo.WindowOptions { a.external(req.URL); return nil })
	p.SetPermissionHandler(func(mygo.PermissionRequest) bool { return false })
	p.OnWillDownload(func(e *mygo.DownloadEvent) { e.PreventDefault() })
	p.OnRenderProcessGone(func(string) {
		if a.logoutBlocked {
			return
		}
		go func() {
			r, _ := mygo.Dialog.Message(mygo.MessageOptions{Parent: a.legacy, Title: "ytmgo · 本地收藏（旧版）", Message: "音乐界面意外停止", Detail: "本地收藏已保留。重新打开界面后可继续使用。", Buttons: []string{"重新加载", "稍后"}})
			if r.Button == 0 {
				p.LoadHTML(a.html, appOrigin+"/")
			}
		}()
	})
	p.LoadHTML(a.html, appOrigin+"/")

}

func inlineFrontend() (string, error) {
	page, err := assets.ReadFile("web/index.html")
	if err != nil {
		return "", err
	}
	css, err := assets.ReadFile("web/style.css")
	if err != nil {
		return "", err
	}
	js, err := assets.ReadFile("web/app.js")
	if err != nil {
		return "", err
	}
	theme, err := assets.ReadFile("web/theme.js")
	if err != nil {
		return "", err
	}
	html := string(page)
	html = strings.ReplaceAll(html, `<script src="theme.js"></script>`, "<script>"+string(theme)+"</script>")
	// The owned UI is loaded with an HTTPS app identity. Inlining avoids making
	// network requests to that identity and gives YouTube the required Referer.
	html = strings.ReplaceAll(html, `<link rel="stylesheet" href="style.css">`, "<style>"+string(css)+"</style>")
	html = strings.ReplaceAll(html, `<script type="module" src="app.js"></script>`, "<script type=\"module\">"+string(js)+"</script>")
	return html, nil
}

func (a *desktop) show() {
	if a.logoutBlocked && a.logoutWindow != nil && !a.logoutWindow.IsDestroyed() {
		a.logoutWindow.Show()
		a.logoutWindow.Focus()
		return
	}
	if a.window != nil && !a.window.IsDestroyed() {
		a.window.Show()
		a.window.Restore()
		a.window.Focus()
	}
}

func (a *desktop) control(action string) {
	if a.logoutBlocked {
		a.show()
		return
	}
	if a.window == nil || a.window.IsDestroyed() {
		return
	}
	w := a.window
	encoded, _ := json.Marshal(action)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_, _ = w.Page().EvalContext(ctx, `location.origin === 'https://music.youtube.com' && window.ytmgoControls?.(`+string(encoded)+`)`)
	}()
}

func (a *desktop) external(raw string) {
	if !navigation.External(raw) {
		return
	}
	go func() {
		var err error
		if runtime.GOOS == "darwin" {
			// The workspace's browser preference is Brave. Do not silently choose
			// another browser if it is absent. No URL is printed to diagnostics.
			err = exec.Command("open", "-b", "com.brave.Browser", raw).Run()
		} else {
			err = mygo.Shell.OpenExternal(raw)
		}
		if err != nil {
			_, _ = mygo.Dialog.Message(mygo.MessageOptions{Parent: a.window, Title: "打开链接", Message: "无法打开 Brave Browser", Detail: "请安装或打开 Brave 后重试。", Buttons: []string{"好"}})
		}
	}()
}

func item(label, shortcut string, fn func()) *mygo.MenuItem {
	return &mygo.MenuItem{Label: label, Accelerator: shortcut, Click: func(*mygo.MenuItem, *mygo.Window) { fn() }}
}
func (a *desktop) menu() *mygo.Menu {
	a.accountItem = item("登录 Google 账号", "", func() { a.show(); a.control("account") })
	a.signOutItem = item("退出登录并清除本机账号数据…", "", a.requestSignOut)
	a.signOutItem.Hidden = true
	a.accountSeparator = mygo.Separator()
	a.accountSeparator.Hidden = true
	return mygo.NewMenu([]*mygo.MenuItem{
		{Role: mygo.RoleAppMenu},
		{Label: "文件", Submenu: []*mygo.MenuItem{item("显示 ytmgo", "CmdOrCtrl+1", a.show), item("本地收藏（旧版）", "", a.openLegacy), {Role: mygo.RoleClose}, mygo.Separator(), {Role: mygo.RoleQuit}}},
		{Role: mygo.RoleEditMenu},
		{Label: "播放", Submenu: []*mygo.MenuItem{
			item("播放 / 暂停", "CmdOrCtrl+P", func() { a.control("toggle") }),
			item("下一首", "CmdOrCtrl+Right", func() { a.control("next") }),
			item("上一首", "CmdOrCtrl+Left", func() { a.control("previous") }),
			mygo.Separator(), item("增大音量", "CmdOrCtrl+Up", func() { a.control("volume-up") }), item("减小音量", "CmdOrCtrl+Down", func() { a.control("volume-down") }), item("静音", "CmdOrCtrl+Shift+M", func() { a.control("mute") }),
		}},
		{Label: "视图", Submenu: []*mygo.MenuItem{item("搜索音乐", "CmdOrCtrl+K", func() { a.show(); a.control("search") }), item("首页", "CmdOrCtrl+Shift+H", func() { a.control("home") }), item("发现", "", func() { a.control("explore") }), item("音乐库", "CmdOrCtrl+L", func() { a.control("library") }), item("登录 / 账号", "", func() { a.control("account") }), mygo.Separator(), {Role: mygo.RoleReload}, {Role: mygo.RoleResetZoom}, {Role: mygo.RoleZoomIn}, {Role: mygo.RoleZoomOut}, {Role: mygo.RoleToggleFullScreen}}},
		{Role: mygo.RoleWindowMenu},
		{Label: "账号", Submenu: []*mygo.MenuItem{a.accountItem, a.accountSeparator, a.signOutItem}},
		{Label: "帮助", Submenu: []*mygo.MenuItem{item("关于 ytmgo", "", func() {
			go func() {
				_, _ = mygo.Dialog.Message(mygo.MessageOptions{Parent: a.window, Title: "关于 ytmgo", Message: "ytmgo · 0.2.1", Detail: "基于 MyGo 的独立音乐客户端。\n音乐与账号由 YouTube Music 提供。\n登录后使用账号推荐、音乐库与歌单。\n旧版本地收藏可从“文件”菜单打开。\n\n全局播放 / 暂停：⌘ / Ctrl + Shift + Space\n关闭窗口后保留播放；彻底退出请按 ⌘ / Ctrl + Q。", Buttons: []string{"好"}})
			}()
		}), item("YouTube Music", "", func() { a.external(navigation.Home) })}},
	})
}
func (a *desktop) trayMenu() *mygo.Menu {
	return mygo.NewMenu([]*mygo.MenuItem{item("打开 ytmgo", "", a.show), mygo.Separator(), item("播放 / 暂停", "", func() { a.control("toggle") }), item("下一首", "", func() { a.control("next") }), item("上一首", "", func() { a.control("previous") }), mygo.Separator(), item("退出 ytmgo", "", func() { mygo.App.Quit() })})
}

func frontendFS() fs.FS {
	f, err := fs.Sub(assets, "web")
	if err != nil {
		panic(fmt.Errorf("frontend unavailable"))
	}
	return f
}
