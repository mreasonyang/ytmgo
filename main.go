package main

import (
	"context"
	"embed"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"sync/atomic"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"ytmgo/internal/accountsession"
	"ytmgo/internal/catalog"
	"ytmgo/internal/language"
	"ytmgo/internal/player"
	"ytmgo/locales"
)

const version = "0.5.0"

//go:embed locales/en.json resources/icon.png resources/tray.png
var assets embed.FS

type historyEntry struct {
	route, query string
	params       string
	page         catalog.Page
}

// UI state belongs to the main thread. Workers deliver results through post;
// request generations prevent an old search or track from replacing a new one.
type desktop struct {
	window                          *mygo.Window
	tray                            *mygo.Tray
	prefs                           *Preferences
	languagePath                    string
	diagnosticPath                  string
	model                           UIModel
	catalog                         *catalog.Client
	player                          *player.Player
	video                           *player.Video
	videoID                         string
	videoFrame                      *ui.Bitmap
	videoFrameNumber                uint64
	videoActive                     atomic.Bool
	windowVisible                   bool
	art                             *artworkCache
	ctx                             context.Context
	cancel                          context.CancelFunc
	pageCancel, playCancel          context.CancelFunc
	prepareCancel                   context.CancelFunc
	seekCancel                      context.CancelFunc
	resumeCancel                    context.CancelFunc
	preparedID                      string
	likeCancel                      context.CancelFunc
	pageGeneration, playGeneration  uint64
	accountGeneration               uint64
	accountState                    accountState
	queueGeneration, seekGeneration uint64
	likeGeneration                  uint64
	accountBusy                     bool
	videoGeneration                 atomic.Uint64
	loadPending                     bool
	queueIndex                      int
	history                         []historyEntry
	historyIndex                    int
	browseParams                    string
	quitting, endedHandled          bool
	unmutedVolume                   float64
}

func main() {
	if handled, code := runUpdateCommand(); handled {
		os.Exit(code)
	}
	setupUpdater()
	if !mygo.App.IsPackaged() {
		mygo.App.SetName("ytmgo Dev")
	}
	if !mygo.App.RequestSingleInstanceLock() {
		return
	}
	dataDir, err := mygo.App.Path(mygo.PathUserData)
	if err != nil {
		log.Fatal("ytmgo: application data directory unavailable")
	}
	ctx, cancel := context.WithCancel(context.Background())
	a := &desktop{ctx: ctx, cancel: cancel, prefs: &Preferences{path: filepath.Join(dataDir, "appearance")}, languagePath: filepath.Join(dataDir, "language"), queueIndex: -1, historyIndex: -1, unmutedVolume: 0.7}
	a.diagnosticPath = filepath.Join(dataDir, "native-diagnostics")
	a.accountState.store = accountsession.Store{Secrets: nativeAccountSecrets{}, Path: filepath.Join(dataDir, "account", "session.enc")}
	recordPlaybackDiagnostic(a.diagnosticPath, "audio", "ready")
	recordPlaybackDiagnostic(a.diagnosticPath, "video", "ready")
	a.model = UIModel{Route: "home", Appearance: a.prefs.Appearance(), Language: language.Load(a.languagePath), Volume: 0.7, PanelTab: "queue"}
	mygo.Theme.SetSource(mygo.ThemeSource(a.model.Appearance))
	mygo.App.SetLanguages(language.Supported...)
	a.applyLanguage()
	a.catalog = catalog.NewWithOptions(catalog.Options{Language: language.ServiceLanguage(mygo.App.Locale())})
	mygo.App.OnLocaleChanged(func(string) { a.localeChanged() })
	mygo.App.SetMenu(a.menu())
	mygo.App.OnActivate(func(bool) { a.show() })
	mygo.App.OnSecondInstance(func([]string, string) { a.show() })
	mygo.App.OnWindowAllClosed(func() {
		if runtime.GOOS != "darwin" && a.tray == nil {
			mygo.App.Quit()
		}
	})
	mygo.App.OnBeforeQuit(func(*mygo.QuitEvent) { a.quitting = true; a.cancelResume(); a.closeAccount(); a.cancel() })
	mygo.App.OnWillQuit(func(*mygo.QuitEvent) {
		// Keep the parent alive until cancellation has reaped every login child.
		// Its bounded kill timer cannot run after this process has exited.
		a.accountState.loginWait.Wait()
		mygo.GlobalShortcut.UnregisterAll()
		mygo.NowPlaying.Clear()
		if a.video != nil {
			a.video.Close()
		}
		if a.player != nil {
			a.player.Close()
		}
	})
	mygo.App.WhenReady(func() {
		a.player, err = player.New()
		a.video, _ = player.NewVideo(a.player)
		if err != nil {
			a.model.Error = a.tr("nativeRuntimeMissing")
		}
		if a.player != nil {
			a.player.SetVolume(a.model.Volume)
		}
		mygo.NowPlaying.OnCommand(func(cmd mygo.MediaCommand, pos time.Duration) {
			if a.player == nil {
				return
			}
			switch cmd {
			case mygo.MediaPlay:
				a.resume()
			case mygo.MediaPause:
				a.pause()
			case mygo.MediaNext:
				a.next()
			case mygo.MediaPrevious:
				a.previous()
			case mygo.MediaSeek:
				a.seek(pos.Seconds())
			}
			a.invalidate()
		})
		a.art = newArtworkCache(a.ctx, func() {
			a.invalidate()
		})
		a.window = mygo.NewWindow(mygo.WindowOptions{Title: locales.AppName(), Width: 1440, Height: 940, MinWidth: 380, MinHeight: 640, StateKey: "native-main", BackgroundColor: "light-dark(#e7e7ea, #050506)", Content: ui.View(func(c *ui.Context) { nativeView(c, &a.model, a.actions()); a.syncVideo() })})
		a.windowVisible = true
		a.window.OnHide(func() { a.windowVisible = false; a.syncVideo() })
		a.window.OnShow(func() { a.windowVisible = true; a.syncVideo() })
		a.window.OnMinimize(func() { a.windowVisible = false; a.syncVideo() })
		a.window.OnRestore(func() { a.windowVisible = true; a.syncVideo() })
		a.window.OnFocus(func() { a.refreshAccount() })
		a.window.OnClose(func(e *mygo.CloseEvent) {
			if !a.quitting && (runtime.GOOS == "darwin" || a.tray != nil) {
				e.PreventDefault()
				a.window.Hide()
			}
		})
		icon, _ := assets.ReadFile("resources/tray.png")
		a.tray, err = mygo.NewTray(mygo.TrayOptions{ID: "si.kyun.ytmgo.player", Icon: icon, IconIsTemplate: true, ToolTip: locales.AppName(), Menu: a.trayMenu()})
		if err != nil {
			log.Print("ytmgo: tray unavailable")
		}
		_ = mygo.GlobalShortcut.Register("CmdOrCtrl+Shift+Space", a.toggle)
		a.navigate("home")
		a.startAccountPolling()
		go a.pollPlayback()
		go a.pollVideo()
	})
	if err = mygo.App.Run(); err != nil {
		a.cancel()
		a.stopAccountLogin()
		a.accountState.loginWait.Wait()
		log.Fatal("ytmgo: desktop runtime could not start")
	}
}

func (a *desktop) post(fn func()) {
	if a.ctx.Err() != nil {
		return
	}
	mygo.RunOnMain(func() {
		if a.ctx.Err() == nil {
			fn()
			a.invalidate()
		}
	})
}

func (a *desktop) show() {
	if a.window != nil && !a.window.IsDestroyed() {
		a.window.Show()
		a.window.Restore()
		a.window.Focus()
	}
}

func (a *desktop) actions() ViewActions {
	return ViewActions{Tr: a.tr, TrArgs: a.trArgs, Navigate: a.navigate, Search: a.search, Open: a.openItem, Play: a.playItem, QueueAppend: a.appendQueue, PlayQueue: a.playQueue, LoadMore: a.loadMore, Video: a.setVideo, RetryPlayback: func() { a.startTrack(a.model.Current) }, Toggle: a.toggle, Next: a.next, Previous: a.previous, Like: a.like, Shuffle: a.shuffle, Repeat: a.repeat, Mute: a.mute, Back: func() { a.travel(-1) }, Forward: func() { a.travel(1) }, Reload: a.reload, Account: a.signIn, SignOut: a.signOut, RestoreAccount: a.retryAccountRestore, SkipRestore: a.skipAccountRestore, Seek: a.seek, Volume: a.volume, Appearance: a.setAppearance, Language: a.setLanguage, Artwork: a.art.get}
}

func (a *desktop) pollPlayback() {
	t := time.NewTicker(350 * time.Millisecond)
	defer t.Stop()
	var previous player.State
	for {
		select {
		case <-a.ctx.Done():
			return
		case <-t.C:
			if a.player == nil {
				continue
			}
			current := a.player.Snapshot()
			if current == previous {
				continue
			}
			previous = current
			a.post(func() {
				if a.loadPending {
					return
				}
				state := a.player.Snapshot()
				a.model.Playing = state.Playing
				a.model.PlaybackLoading = state.Loading
				a.model.Position = state.Position.Seconds()
				if state.Duration > 0 {
					a.model.Duration = state.Duration.Seconds()
				}
				if state.Error != "" {
					recordPlaybackDiagnostic(a.diagnosticPath, "audio", state.Error, state.ErrorDetails)
					a.playbackFailure(state)
				}
				if state.Ended && !a.endedHandled {
					a.endedHandled = true
					if a.model.Repeat == 2 {
						a.resume()
					} else {
						a.next()
					}
				}
				playState := mygo.PlaybackPaused
				if state.Playing {
					playState = mygo.PlaybackPlaying
				}
				mygo.NowPlaying.SetPlayback(playState, state.Position)
			})
		}
	}
}

func menuItem(label, shortcut string, fn func()) *mygo.MenuItem {
	return &mygo.MenuItem{Label: label, Accelerator: shortcut, Click: func(*mygo.MenuItem, *mygo.Window) { fn() }}
}

func (a *desktop) menu() *mygo.Menu {
	return mygo.NewMenu([]*mygo.MenuItem{
		applicationMenuWithUpdates(),
		{Label: locales.MenuFile(), Submenu: []*mygo.MenuItem{menuItem(locales.MenuShowApp(), "CmdOrCtrl+1", a.show), {Role: mygo.RoleClose}, mygo.Separator(), {Role: mygo.RoleQuit}}},
		{Role: mygo.RoleEditMenu},
		{Label: locales.MenuPlayback(), Submenu: []*mygo.MenuItem{
			menuItem(locales.MenuPlayPause(), "CmdOrCtrl+P", a.toggle), menuItem(locales.MenuNext(), "CmdOrCtrl+Right", a.next), menuItem(locales.MenuPrevious(), "CmdOrCtrl+Left", a.previous), mygo.Separator(), menuItem(locales.MenuLike(), "", a.like), menuItem(locales.MenuVolumeUp(), "CmdOrCtrl+Up", func() { a.volume(a.model.Volume + 0.05) }), menuItem(locales.MenuVolumeDown(), "CmdOrCtrl+Down", func() { a.volume(a.model.Volume - 0.05) }), menuItem(locales.MenuMute(), "CmdOrCtrl+Shift+M", a.mute)}},
		{Label: locales.MenuView(), Submenu: []*mygo.MenuItem{
			menuItem(locales.MenuHome(), "CmdOrCtrl+Shift+H", func() { a.navigate("home") }), menuItem(locales.MenuExplore(), "", func() { a.navigate("explore") }), menuItem(locales.MenuLibrary(), "CmdOrCtrl+L", func() { a.navigate("library") }), mygo.Separator(), menuItem(locales.MenuPanel(), "CmdOrCtrl+J", func() { a.model.Panel = !a.model.Panel; a.invalidate() }), menuItem(locales.MenuLyrics(), "", func() { a.model.Panel = true; a.model.PanelTab = "lyrics"; a.invalidate() }), menuItem(locales.MenuImmersive(), "CmdOrCtrl+Shift+F", func() { a.model.Immersive = !a.model.Immersive; a.invalidate() }), menuItem(locales.MenuSidebar(), "CmdOrCtrl+B", func() { a.model.SidebarCollapsed = !a.model.SidebarCollapsed; a.invalidate() }), {Label: locales.MenuLanguage(), Submenu: a.languageMenu()}, {Role: mygo.RoleToggleFullScreen}}},
		{Role: mygo.RoleWindowMenu},
		{Label: locales.MenuAccount(), Submenu: a.accountMenuItems()},
		{Label: locales.MenuHelp(), Submenu: []*mygo.MenuItem{menuItem(locales.MenuShortcuts(), "CmdOrCtrl+/", func() { a.model.Shortcuts = true; a.invalidate() }), menuItem(locales.MenuAbout(), "", func() {
			go func() {
				_, _ = mygo.Dialog.Message(mygo.MessageOptions{Parent: a.window, Title: locales.MenuAbout(), Message: locales.AboutMessage(applicationVersion()), Detail: a.tr("nativeAbout"), Buttons: []string{locales.ButtonOK()}})
			}()
		})}},
	})
}

func (a *desktop) accountMenuItems() []*mygo.MenuItem {
	items := []*mygo.MenuItem{menuItem(locales.MenuSignIn(), "", a.signIn)}
	if a.model.SignedIn {
		items = append(items, menuItem(locales.MenuSignOut(), "", a.signOut))
	}
	return items
}

func (a *desktop) invalidate() {
	if a.window != nil {
		a.window.Update(func() {})
	}
}
func (a *desktop) trayMenu() *mygo.Menu {
	return mygo.NewMenu([]*mygo.MenuItem{menuItem(locales.TrayOpen(), "", a.show), mygo.Separator(), menuItem(locales.MenuPlayPause(), "", a.toggle), menuItem(locales.MenuNext(), "", a.next), menuItem(locales.MenuPrevious(), "", a.previous), mygo.Separator(), menuItem(locales.TrayQuit(), "", func() { mygo.App.Quit() })})
}
