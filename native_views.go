package main

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"ytmgo/internal/catalog"
	"ytmgo/locales"
)

// UIModel is owned by the UI thread. Network and player completions are applied
// by the controller before a frame; the view changes only local presentation.
type UIModel struct {
	Page                                                        catalog.Page
	Current                                                     catalog.Item
	LibraryItems                                                []catalog.Item
	LibraryLoaded                                               bool
	LibraryCached, LibraryRefreshing                            bool
	LibraryErrorKey                                             string
	GlobalShortcut, ShortcutError                               string
	ShortcutRecording                                           bool
	Queue                                                       []catalog.Item
	Route, Query, Error, Lyrics, PanelTab, Appearance, Language string
	LyricLines                                                  []catalog.LyricLine
	LyricsSource                                                string
	LyricsAvailable                                             bool
	LyricsRevision                                              uint64
	LyricsExpanded                                              bool
	Loading, Playing, Muted, Liked, Shuffle                     bool
	Panel, Immersive, Settings, Shortcuts, SidebarCollapsed     bool
	SignedIn, CanBack, CanForward                               bool
	MusicPremium                                                bool
	Repeat                                                      int
	Volume, Position, Duration                                  float64
	FocusSearch                                                 bool
	Video, VideoLoading, PlaybackLoading                        bool
	VideoFrame                                                  ui.ImageSource
	PlaybackError                                               string
	VideoError                                                  string
	VideoQuality                                                int
	VideoQualityError                                           string
	ErrorScope                                                  errorScope
	AccountRestore                                              string
	AccountNotice                                               string
	AccountAvatar                                               string
	Version                                                     string
	errorMessage, playbackMessage, videoMessage, lyricsMessage  uiMessage

	// Presentation state for this window.
	playbackViewSeen                bool // first playback opens Now Playing unless already visited
	settingsScroll, shortcutsScroll ui.ScrollState
	shortcutFieldFocused            bool // the recorder has had the focus
	revealShortcut                  bool // bring the global shortcut row into view
}

// Saved-account restoration as the view presents it; account.go owns the
// transitions. "" means the account state is settled.
const (
	accountRestoring       = "restoring" // reading and verifying the saved session
	accountRestoreSlow     = "slow"      // still restoring after a long wait
	accountRestoreDeferred = "deferred"  // saved session kept but not in use
)

func nativeRestoring(m *UIModel) bool {
	return !m.SignedIn && (m.AccountRestore == accountRestoring || m.AccountRestore == accountRestoreSlow)
}

func nativeRestoreDeferred(m *UIModel) bool {
	return !m.SignedIn && m.AccountRestore == accountRestoreDeferred
}

// nativeRestoreActions offers the next step for a slow or deferred restore.
// It is the primary action unless it sits beside the sign-in pill.
func nativeRestoreActions(c *ui.Context, m *UIModel, a ViewActions, p nativePalette, primary bool) {
	switch {
	case m.AccountRestore == accountRestoreSlow && !m.SignedIn:
		if nativePill(c, a.Tr("nativeAccountRestoreSkip"), p, primary).Disabled(a.SkipRestore == nil).Clicked() {
			a.SkipRestore()
		}
	case nativeRestoreDeferred(m):
		if nativePill(c, a.Tr("nativeAccountRestoreRetry"), p, primary).Disabled(a.RestoreAccount == nil).Clicked() {
			a.RestoreAccount()
		}
	}
}

// ViewActions keeps native controls independent from the catalogue and player.
type ViewActions struct {
	Tr                                                  func(string) string
	TrArgs                                              func(string, map[string]any) string
	Navigate                                            func(string)
	Search                                              func(string)
	Open, Play, QueueAppend                             func(catalog.Item)
	PlayQueue                                           func(int)
	Toggle, Next, Previous, Like, Shuffle, Repeat, Mute func()
	Back, Forward, Reload, Account, SignOut             func()
	RestoreAccount, SkipRestore, RetryLibrary           func()
	Seek, Volume                                        func(float64)
	LyricsClock                                         func() (seconds float64, playing bool)
	Appearance, Language                                func(string)
	Artwork                                             func(string, int, artworkPriority) ui.ImageSource
	// ArtworkTint is the colour a loaded cover lends the interface.
	ArtworkTint                   func(string) (ui.Color, bool)
	Video                         func(bool)
	SetVideoQuality               func(int)
	SetLyricsExpanded             func(bool)
	RetryPlayback                 func()
	LoadMore                      func()
	BeginShortcut, CancelShortcut func()
	SetShortcut                   func(string)
	// AppMenu fills the application menu button, where a hidden title bar
	// leaves the window no menu bar (Linux, Windows); nil hides the button.
	AppMenu          func(*ui.Menu)
	artViewport      ui.Rect
	viewportScroll   *ui.ScrollState
	secondaryArtwork bool
}

// Window geometry. Above 1180 DIPs the library is a full panel and Now Playing
// docks on the right; down to 761 the library is a rail and Now Playing floats;
// at 760 and below the window becomes a single column with a mini player and
// a bottom tab bar.
const (
	nativeTopbarHeight  = tokenProductGlobalBar
	nativePlayerHeight  = tokenProductPlayer
	nativeLibraryWidth  = tokenProductLibrary
	nativeRailWidth     = tokenProductRail
	nativePanelWidth    = tokenProductNowPlaying
	nativeCompactTabbar = tokenProductTabBar
	nativeExpandedBar   = 140
	// nativeImmersiveGutter separates immersive mode's media and panel and
	// insets them from the window.
	nativeImmersiveGutter = 48
	// The Now Playing cover: at least nativeCoverMin, and a nativeCoverShare
	// of the height it shares with the lyrics, which is the window less
	// nativeNowPlayingChrome (the global bar, player bar, panel padding,
	// header, media switch, title and the lyrics card's chips, measured).
	// Lyrics first, it is a nativeCoverThumb beside the title.
	nativeCoverMin         = 120
	nativeCoverShare       = 0.45
	nativeNowPlayingChrome = 450
	nativeCoverThumb       = 56
)

func nativeView(c *ui.Context, m *UIModel, a ViewActions) {
	// Apply the default once. Opening either player view before the music starts
	// also counts, so dismissing it while paused or loading is respected.
	firstPlayback := m.Playing && !m.playbackViewSeen && !m.Panel && !m.Immersive
	if firstPlayback {
		m.Panel = true
	}
	m.playbackViewSeen = m.playbackViewSeen || m.Panel || m.Immersive
	before := nativePresentation(m)
	nativeKeyboard(c, m, a)
	defer func() {
		if before != nativePresentation(m) {
			c.Invalidate()
		}
	}()
	width, height := c.Size()
	dark := c.Theme().Dark
	if m.Appearance == "dark" {
		dark = true
	}
	if m.Appearance == "light" {
		dark = false
	}
	c.SetTheme(nativeTheme(dark))
	p := palette(dark)
	if width <= 760 {
		nativeCompactFrame(c, m, a, p, width, height)
	} else {
		nativeDesktopFrame(c, m, a, p, width, height)
	}
	if m.Panel && !m.Immersive && width <= 1180 {
		panelWidth := min(width-16, 380)
		ui.DialogBase(c, &m.Panel, func(backdrop, panel ui.Element) {
			backdrop.Background(p.scrim)
			panel.Width(panelWidth).Height(min(height-100, 760)).Radius(tokenRadiusPanel).Background(p.surface).Shadow(tokenShadowHero.args()).Clip().Children(func() {
				nativeNowPlaying(c, m, a, p, panelWidth, true, firstPlayback).Fill()
			})
		}).Label(a.Tr("shellNowPlaying"))
	}
	nativeSettings(c, m, a, p, width, height)
	nativeShortcuts(c, m, a, p, width, height)
	if m.ShortcutRecording && !m.Settings && a.CancelShortcut != nil {
		a.CancelShortcut()
	}
}

// nativeDesktopFrame is Spotify's desktop arrangement: a global bar on the
// window frame, three rounded panels, and the player bar across the bottom.
func nativeDesktopFrame(c *ui.Context, m *UIModel, a ViewActions, p nativePalette, width, height float32) {
	ui.Column(c).Fill().Background(p.base).TextColor(p.text).Children(func() {
		// Immersive mode drops the global bar. Under a hidden title bar its
		// header moves out of the panel to take the bar's place instead.
		top, header := float32(8), float32(0)
		if !m.Immersive {
			nativeTopbar(c, m, a, p, width)
			top = 0
		} else if c.TitleBar().Height > 0 {
			header = nativeImmersiveHeader(c, m, a, p, width)
			top = 0
		}
		ui.Row(c).Grow(1).MinHeight(0).AlignItems(ui.Stretch).Gap(8).Padding(top, 8, 0).Children(func() {
			if !m.Immersive {
				nativeLibrary(c, m, a, p, width)
			}
			ui.Column(c).Grow(1).MinWidth(0).MinHeight(0).Radius(tokenRadiusPanel).Background(p.surface).Clip().Children(func() {
				nativeBackdrop(c, m, a, p)
				nativePageLayer(c).Children(func() {
					if m.Immersive {
						nativeImmersive(c, m, a, p, width, height-nativePlayerHeight-top-header, header == 0)
					} else {
						nativeContent(c, m, a, p, false)
					}
				})
			})
			if m.Panel && !m.Immersive && width > 1180 {
				nativeNowPlaying(c, m, a, p, nativePanelWidth, false, false).Width(nativePanelWidth).Shrink(0)
			}
		})
		nativePlayerBar(c, m, a, p, width).Height(nativePlayerHeight).Shrink(0)
	})
}

// nativeCompactFrame follows Spotify's phone layout: one full-bleed page, a
// floating mini player and a tab bar.
func nativeCompactFrame(c *ui.Context, m *UIModel, a ViewActions, p nativePalette, width, height float32) {
	ui.Column(c).Fill().Background(p.base).TextColor(p.text).Children(func() {
		ui.Column(c).Grow(1).MinWidth(0).MinHeight(0).Background(p.surface).Clip().Children(func() {
			nativeBackdrop(c, m, a, p)
			nativePageLayer(c).Children(func() {
				if m.Immersive {
					nativeImmersive(c, m, a, p, width, height-nativeExpandedBar-nativeCompactTabbar, true)
				} else {
					nativeCompactTopbar(c, m, a, p, width)
					nativeContent(c, m, a, p, true)
				}
			})
		})
		if m.Immersive {
			nativeExpandedTransport(c, m, a, p).Height(nativeExpandedBar).Shrink(0)
		} else {
			nativeMiniPlayer(c, m, a, p)
		}
		nativeTabbar(c, m, a, p)
	})
}

// nativePageLayer holds a panel's page above its colour wash. MyGo paints
// absolute elements above their in-flow siblings, so the page is absolute too
// and follows the backdrop in paint order.
func nativePageLayer(c *ui.Context) ui.Element {
	return ui.Column(c).Absolute().Left(0).Top(0).WidthPercent(100).HeightPercent(100).MinHeight(0)
}

// nativeIconButton is a round icon button. An active one (an open panel,
// a liked track, settings showing) is drawn in text with a dot beneath:
// coral is for play, so only nativeModeButton uses it.
func nativeIconButton(c *ui.Context, name, label string, p nativePalette, active bool, hit float32) ui.Element {
	return nativeToggleIcon(c, name, label, p, active, hit, p.text)
}

// nativeModeButton is a playback mode toggle (shuffle, repeat): on, it is
// coral with a dot, as what's playing is.
func nativeModeButton(c *ui.Context, name, label string, p nativePalette, on bool, hit float32) ui.Element {
	return nativeToggleIcon(c, name, label, p, on, hit, p.accent)
}

func nativeToggleIcon(c *ui.Context, name, label string, p nativePalette, active bool, hit float32, on ui.Color) ui.Element {
	b := ui.ButtonBase(c).Size(hit, hit).Shrink(0).Radius(hit / 2).Label(label).Tooltip(label).TextColor(p.secondary)
	if active {
		b.Selected(true).Background(ui.Transparent).TextColor(on)
	} else if b.Hovered() {
		b.Background(p.hover).TextColor(p.text)
	}
	nativePress(b)
	b.Children(func() {
		icon := min(hit*0.6, 22)
		ui.Icon(c, nativeIcons[name]).Size(icon, icon)
		if active {
			ui.Box(c).Absolute().Left((hit-4)/2).Bottom(1).Size(4, 4).Radius(tokenRadiusPill).Background(on).PassThrough()
		}
	})
	return b
}

// nativePress gives a button the guidebook's press feedback: every button
// presses to 96 %. Rows and cards answer with the press wash instead.
func nativePress(e ui.Element) {
	if e.Pressed() {
		e.TransformOrigin(0.5, 0.5).Scale(tokenPressScale, tokenPressScale)
	}
}

// nativePlayFeedback is a round play control's: it grows on hover, as in the
// guidebook, keeping its colour, and presses like every button.
func nativePlayFeedback(e ui.Element) {
	switch {
	case e.Pressed():
		nativePress(e)
	case e.Hovered():
		e.TransformOrigin(0.5, 0.5).Scale(tokenGrowScale, tokenGrowScale)
	}
}

// nativeCircleButton is the filled round button of the global bar.
func nativeCircleButton(c *ui.Context, name, label string, p nativePalette, size float32, active bool) ui.Element {
	b := ui.ButtonBase(c).Size(size, size).Shrink(0).Radius(size / 2).Label(label).Tooltip(label).Background(p.raised).TextColor(p.secondary)
	if active {
		b.Selected(true).Background(p.raised).TextColor(p.text)
	}
	if b.Hovered() {
		b.Background(p.highlight).TextColor(p.text)
	}
	nativePress(b)
	b.Children(func() { ui.Icon(c, nativeIcons[name]).Size(size/2, size/2) })
	return b
}

// nativePill is a rounded text button; the primary form is Spotify's white
// (in light appearance, black) call to action.
func nativePill(c *ui.Context, label string, p nativePalette, primary bool) ui.Element {
	b := ui.Button(c, label).MinHeight(tokenProductPill).Padding(8, 16).Radius(tokenRadiusPill).BorderWidth(0).FontWeight(700).Background(p.input).TextColor(p.text)
	if primary {
		b.Selected(true).Background(p.text).TextColor(p.base)
		if b.Hovered() {
			b.Background(p.text.Alpha(0.86))
		}
	} else if b.Hovered() {
		b.Background(p.highlight)
	}
	nativePress(b)
	return b
}

// nativeChip is a filter chip: translucent at rest, inverted when selected.
func nativeChip(c *ui.Context, label string, p nativePalette, selected bool) ui.Element {
	b := ui.Button(c, label).Height(tokenProductChip).MinHeight(tokenProductChip).Padding(0, 12).Radius(tokenRadiusPill).BorderWidth(0).FontSize(tokenSizeMeta).FontWeight(500).Background(p.hover).TextColor(p.text)
	if selected {
		b.Selected(true).Background(p.text).TextColor(p.base)
	} else if b.Hovered() {
		b.Background(p.press)
	}
	nativePress(b)
	return b
}

// nativeTopbar is the global bar. Under a hidden title bar (Linux, Windows)
// it is the window's title bar too: it drags the window, keeps clear of the
// window controls and leads with the application menu.
func nativeTopbar(c *ui.Context, m *UIModel, a ViewActions, p nativePalette, width float32) {
	bar := c.TitleBar()
	left, right := nativeTitleBarInsets(bar, 16)
	nativeTitleBarRow(c, bar).Height(max(nativeTopbarHeight, bar.Height)).Padding(0, right, 0, left).Gap(16).AlignItems(ui.Center).Children(func() {
		ui.Row(c).Grow(1).Basis(0).MinWidth(0).Gap(8).AlignItems(ui.Center).Children(func() {
			nativeAppMenuButton(c, a, p, 32)
			if nativeCircleButton(c, "back", a.Tr("shellBack"), p, 32, false).Disabled(!m.CanBack).Clicked() {
				a.Back()
			}
			if nativeCircleButton(c, "forward", a.Tr("shellForward"), p, 32, false).Disabled(!m.CanForward).Clicked() {
				a.Forward()
			}
		})
		ui.Row(c).Width(min(560, width*0.46)).MinWidth(0).Gap(8).AlignItems(ui.Center).Children(func() {
			home := m.Route == "home"
			icon := "home"
			if home {
				icon = "home-fill"
			}
			if nativeCircleButton(c, icon, a.Tr("shellNavHome"), p, 48, home).Clicked() {
				a.Navigate("home")
			}
			nativeSearchField(c, m, a, p, 48, true).Grow(1)
		})
		ui.Row(c).Grow(1).Basis(0).MinWidth(0).Gap(8).AlignItems(ui.Center).Justify(ui.End).Children(func() {
			if nativeIconButton(c, "settings", a.Tr("shellSettings"), p, m.Settings, 32).Clicked() {
				m.Settings = !m.Settings
			}
			nativeAccount(c, m, a, p, width > 1180)
		})
	})
}

func nativeCompactTopbar(c *ui.Context, m *UIModel, a ViewActions, p nativePalette, width float32) {
	bar := c.TitleBar()
	left, right := nativeTitleBarInsets(bar, 8)
	// The search field shares the bar with the window controls while it
	// keeps room to type; a narrower window gives it a row below them.
	buttons := float32(2)
	if m.SignedIn {
		buttons++
	}
	if a.AppMenu != nil {
		buttons++
	}
	stacked := bar.Height > 0 && width-left-right-buttons*48 < nativeCompactSearchMin
	nativeTitleBarRow(c, bar).Height(max(60, bar.Height)).Padding(8, right, 8, left).Gap(4).AlignItems(ui.Center).Children(func() {
		nativeAppMenuButton(c, a, p, 44)
		if nativeIconButton(c, "back", a.Tr("shellBack"), p, false, 44).Disabled(!m.CanBack).Clicked() {
			a.Back()
		}
		if stacked {
			ui.Spacer(c)
		} else {
			nativeSearchField(c, m, a, p, 44, false).Grow(1)
		}
		if nativeIconButton(c, "settings", a.Tr("shellSettings"), p, m.Settings, 44).Clicked() {
			m.Settings = !m.Settings
		}
		if m.SignedIn {
			nativeAccount(c, m, a, p, false)
		}
	})
	if stacked {
		ui.Row(c).Shrink(0).Padding(0, 8, 8).Children(func() {
			nativeSearchField(c, m, a, p, 44, false).Grow(1)
		})
	}
}

// nativeSearchField is the persistent search pill. On desktop it ends with the
// Explore button, where Spotify places Browse.
func nativeSearchField(c *ui.Context, m *UIModel, a ViewActions, p nativePalette, h float32, explore bool) ui.Element {
	field := ui.Row(c).MinWidth(0).Height(h).PaddingStart(12).PaddingEnd(8).Gap(8).Radius(h/2).Background(p.raised).Border(2, ui.Transparent).AlignItems(ui.Center)
	if field.FocusWithin() {
		field.BorderColor(p.text)
	} else if field.Hovered() {
		field.Background(p.highlight).BorderColor(p.line)
	}
	return field.Children(func() {
		ui.Icon(c, nativeIcons["search"]).Size(22, 22).TextColor(p.secondary)
		input := ui.TextInput(c.Key("search-input"), &m.Query).Grow(1).MinWidth(0).Height(h - 6).Padding(0).BorderWidth(0).Background(ui.Transparent).Placeholder(a.Tr("shellSearchPlaceholder")).Label(a.Tr("shellNavSearch")).FontSize(tokenSizeSmall)
		if m.FocusSearch {
			input.Focus()
			m.FocusSearch = false
		}
		if input.Submitted() && strings.TrimSpace(m.Query) != "" {
			a.Search(m.Query)
		}
		if explore {
			ui.Box(c).Size(1, 24).Shrink(0).Background(p.line)
			active := m.Route == "explore"
			icon := "explore"
			if active {
				icon = "explore-fill"
			}
			b := nativeIconButton(c, icon, a.Tr("shellNavExplore"), p, false, 36)
			if active {
				b.Selected(true).Background(ui.Transparent).TextColor(p.text)
			}
			if b.Clicked() {
				a.Navigate("explore")
			}
		}
	})
}

// nativeAccountStatus names the three account states the controller reports.
func nativeAccountStatus(m *UIModel, a ViewActions) string {
	switch {
	case nativeRestoring(m):
		return a.Tr("nativeAccountRestoring")
	case nativeRestoreDeferred(m):
		return a.Tr("nativeAccountRestoreDeferred")
	case !m.SignedIn:
		return a.Tr("shellAccountSignedOut")
	case m.MusicPremium:
		return a.Tr("shellAccountPremium")
	}
	return a.Tr("shellAccountStandard")
}

func nativeAvatar(c *ui.Context, m *UIModel, a ViewActions, p nativePalette, size float32) ui.Element {
	bg, fg := p.highlight, p.secondary
	if m.SignedIn {
		bg, fg = p.muted, p.surface
	}
	if m.SignedIn && m.MusicPremium {
		bg, fg = p.text, p.base
	}
	var avatar ui.ImageSource
	if m.SignedIn && m.AccountAvatar != "" && a.Artwork != nil {
		avatar = a.Artwork(m.AccountAvatar, int(math.Ceil(float64(size)*2)), artAvatar)
	}
	return ui.Box(c).Size(size, size).Shrink(0).Radius(size / 2).Clip().Background(bg).TextColor(fg).Center().PassThrough().Children(func() {
		if avatar != nil {
			ui.Image(c, avatar).Fill().Fit(ui.Cover).PassThrough()
		} else {
			ui.Icon(c, nativeIcons["user"]).Size(size*0.56, size*0.56)
		}
	})
}

// nativeAccount is the top-right control: a sign-in call to action when
// signed out, otherwise an avatar whose menu states the account's plan.
func nativeAccount(c *ui.Context, m *UIModel, a ViewActions, p nativePalette, badge bool) {
	if nativeRestoring(m) {
		// Sign-in cannot start until restoration settles, so it is not offered.
		status := nativeAccountStatus(m, a)
		ui.Row(c).Height(36).MinWidth(0).Shrink(1).PaddingX(12).Gap(8).Radius(tokenRadiusPill).Background(p.raised).AlignItems(ui.Center).Label(status).Tooltip(status).Children(func() {
			ui.Spinner(c).Size(16, 16)
			if badge {
				ui.Text(c, status).MinWidth(0).FontSize(tokenSizeMeta).FontWeight(600).TextColor(p.secondary).MaxLines(1).Ellipsis("…")
			}
		})
		return
	}
	if !m.SignedIn {
		if nativePill(c, a.Tr("shellSignIn"), p, true).Clicked() {
			a.Account()
		}
		return
	}
	status := nativeAccountStatus(m, a)
	b := ui.ButtonBase(c.Key("account")).Height(48).MinWidth(48).Shrink(0).Padding(8).Gap(8).Radius(tokenRadiusPill).Background(p.raised).Label(a.Tr("menuAccount") + " · " + status).Tooltip(status)
	if b.Hovered() {
		b.Background(p.highlight)
	}
	b.Children(func() {
		if m.MusicPremium && badge {
			ui.Text(c, a.Tr("shellPremiumBadge")).PaddingStart(4).FontSize(tokenSizeMicro).FontWeight(700).TextColor(p.text)
		}
		nativeAvatar(c, m, a, p, 32)
	})
	b.Menu(func(menu *ui.Menu) {
		menu.Item(status).Disabled(true)
		menu.Separator()
		if menu.Item(a.Tr("shellAccountManage")).Chosen() {
			a.Account()
		}
		if menu.Item(a.Tr("shellSettings")).Chosen() {
			m.Settings = true
		}
		menu.Separator()
		if menu.Item(a.Tr("shellSignOut")).Chosen() {
			a.SignOut()
		}
	})
}

// Library filters, in Spotify's order. Songs include music videos.
var nativeLibraryFilters = []struct{ kind, key string }{{"playlist", "shellFilterPlaylists"}, {"album", "shellFilterAlbums"}, {"artist", "shellFilterArtists"}, {"song", "shellFilterSongs"}}

func nativeLibraryKind(item catalog.Item) string {
	switch {
	case item.Kind == "playlist" || item.Kind == "album" || item.Kind == "artist":
		return item.Kind
	case item.ID != "":
		return "song"
	}
	return ""
}

func nativeKindLabel(item catalog.Item, a ViewActions) string {
	if key := nativeKindKey(item); key != "" {
		return a.Tr(key)
	}
	return ""
}

func nativeKindKey(item catalog.Item) string {
	switch item.Kind {
	case "podcast":
		return "nativeKindPodcast"
	case "episode":
		return "nativeKindEpisode"
	}
	switch nativeLibraryKind(item) {
	case "playlist":
		return "shellKindPlaylist"
	case "album":
		return "shellKindAlbum"
	case "artist":
		return "shellKindArtist"
	case "song":
		if item.Kind == "video" {
			return "nativeVideo"
		}
		return "shellKindSong"
	}
	return ""
}

func nativeLibrarySubtitle(item catalog.Item, a ViewActions) string {
	kind, artist := nativeKindLabel(item, a), strings.TrimSpace(item.Artist)
	if artist == "" {
		return kind
	}
	// Cached library entries can keep the service's previous language after
	// changing the interface language. A category is not a second creator.
	if key := nativeKindKey(item); key != "" {
		for _, language := range locales.Catalog.Languages() {
			if strings.EqualFold(artist, locales.Catalog.Format(language, key, nil)) {
				return kind
			}
		}
	}
	if kind == "" {
		return artist
	}
	return kind + " · " + artist
}

// nativeResultSubtitle leads search results with their type, since one list
// can mix songs, artists, albums, playlists and podcasts.
func nativeResultSubtitle(item catalog.Item, m *UIModel, a ViewActions) string {
	if m.Route == "search" {
		return nativeLibrarySubtitle(item, a)
	}
	return item.Artist
}

func nativeItemLabel(item catalog.Item) string {
	if artist := strings.TrimSpace(item.Artist); artist != "" {
		return item.Title + " · " + artist
	}
	return item.Title
}

func nativeResultLabel(item catalog.Item, m *UIModel, a ViewActions) string {
	if m.Route == "search" {
		if subtitle := nativeLibrarySubtitle(item, a); subtitle != "" {
			return item.Title + " · " + subtitle
		}
	}
	return nativeItemLabel(item)
}

func nativeLibrary(c *ui.Context, m *UIModel, a ViewActions, p nativePalette, width float32) {
	a.secondaryArtwork = true
	full := width > 1180 && !m.SidebarCollapsed
	panelWidth := float32(nativeRailWidth)
	if full {
		panelWidth = nativeLibraryWidth
	}
	panel := ui.Column(c).Width(panelWidth).Shrink(0).MinHeight(0).Background(p.surface).Radius(tokenRadiusPanel).Clip()
	filter := ui.Local(panel, "library-filter", func() string { return "" })
	panel.Children(func() {
		libraryIcon := "library"
		if m.Route == "library" {
			libraryIcon = "library-fill"
		}
		if full {
			ui.Row(c).Height(56).Shrink(0).Padding(8, 12).Gap(8).AlignItems(ui.Center).Children(func() {
				b := ui.ButtonBase(c.Key("library-title")).Grow(1).MinWidth(0).Height(40).PaddingX(8).Gap(12).Justify(ui.Start).Radius(tokenRadiusControl).Label(a.Tr("shellNavLibrary")).Tooltip(a.Tr("shellNavLibrary")).TextColor(p.secondary)
				if m.Route == "library" || b.Hovered() {
					b.TextColor(p.text)
				}
				b.Children(func() {
					ui.Icon(c, nativeIcons[libraryIcon]).Size(24, 24)
					ui.Text(c, a.Tr("shellLibraryTitle")).FontSize(tokenSizeBody).FontWeight(700).MaxLines(1).Ellipsis("…")
				})
				if b.Clicked() {
					a.Navigate("library")
				}
				if nativeIconButton(c, "sidebar", a.Tr("shellSidebarCollapse"), p, false, 32).Clicked() {
					m.SidebarCollapsed = true
				}
			})
		} else {
			ui.Column(c).Shrink(0).Padding(12, 0, 4).Gap(4).AlignItems(ui.Center).Children(func() {
				if width > 1180 {
					if nativeIconButton(c, "sidebar", a.Tr("shellSidebarExpand"), p, false, 40).Clicked() {
						m.SidebarCollapsed = false
					}
				}
				b := nativeIconButton(c, libraryIcon, a.Tr("shellNavLibrary"), p, false, 40)
				if m.Route == "library" {
					b.Selected(true).Background(ui.Transparent).TextColor(p.text)
				}
				if b.Clicked() {
					a.Navigate("library")
				}
			})
		}
		var items []catalog.Item
		if m.SignedIn || m.LibraryCached {
			items = m.LibraryItems
		}
		if full && m.Route != "library" {
			nativeLibraryCacheStatus(c, m, a, p)
		}
		if len(items) == 0 {
			switch {
			case m.SignedIn && !m.LibraryLoaded && m.LibraryErrorKey == "":
				nativeLibrarySkeleton(c, a, p, full)
			case full:
				nativeLibraryPrompt(c, m, a, p)
			}
			return
		}
		present := map[string]bool{}
		for _, item := range items {
			present[nativeLibraryKind(item)] = true
		}
		if !present[*filter] {
			*filter = ""
		}
		kinds := 0
		for _, f := range nativeLibraryFilters {
			if present[f.kind] {
				kinds++
			}
		}
		// Chips appear only when they separate at least two kinds of item.
		if full && kinds > 1 {
			ui.Row(c).Shrink(0).Padding(0, 16, 8).Gap(8).AlignItems(ui.Center).Children(func() {
				if *filter != "" {
					clear := nativeIconButton(c, "close", a.Tr("shellFilterClear"), p, false, 32).Background(p.hover)
					if clear.Clicked() {
						*filter = ""
					}
				}
				for _, f := range nativeLibraryFilters {
					if !present[f.kind] || (*filter != "" && *filter != f.kind) {
						continue
					}
					if nativeChip(c, a.Tr(f.key), p, *filter == f.kind).Clicked() {
						if *filter == f.kind {
							*filter = ""
						} else {
							*filter = f.kind
						}
					}
				}
			})
		}
		pad := float32(8)
		if !full {
			pad = 4
		}
		scroll := ui.Scroll(c).Grow(1).MinHeight(0).Padding(0, pad, 8).Label(a.Tr("shellLibraryRegion"))
		clipped := nativeArtworkViewport(c, a, scroll)
		scroll.Children(func() {
			for i, item := range items {
				if *filter != "" && nativeLibraryKind(item) != *filter {
					continue
				}
				nativeLibraryRow(c.Key(fmt.Sprintf("library-%d", i)), item, m, clipped, p, full)
			}
		})
	})
}

func nativeLibraryCacheStatus(c *ui.Context, m *UIModel, a ViewActions, p nativePalette) {
	if !m.LibraryLoaded || (!m.LibraryCached && !m.LibraryRefreshing) {
		return
	}
	key := "nativeLibraryUpdating"
	if !m.SignedIn {
		key = "nativeLibrarySaved"
	}
	if m.LibraryErrorKey != "" {
		key = "nativeLibrarySavedError"
	}
	ui.Row(c).Shrink(0).Padding(8, 16).Gap(8).AlignItems(ui.Center).Children(func() {
		if m.LibraryRefreshing {
			ui.Spinner(c).Size(14, 14).Label(a.Tr(key))
		}
		ui.Text(c, a.Tr(key)).Grow(1).MinWidth(0).FontSize(tokenSizeMicro).TextColor(p.secondary).MaxLines(2)
		if m.LibraryErrorKey != "" && m.SignedIn && a.RetryLibrary != nil {
			if nativeIconButton(c, "refresh", a.Tr("buttonRetry"), p, false, 28).Clicked() {
				a.RetryLibrary()
			}
		}
	})
}

// nativeLibrarySkeleton holds the shape of library rows while the saved
// library loads, as Spotify does, instead of describing the wait.
func nativeLibrarySkeleton(c *ui.Context, a ViewActions, p nativePalette, full bool) {
	pad := float32(8)
	if !full {
		pad = 4
	}
	ui.Column(c).Padding(0, pad, 8).Label(a.Tr("nativeLibraryLoadingTitle")).Children(func() {
		for i := range 6 {
			row := ui.Row(c.Key(fmt.Sprintf("skeleton-%d", i))).Height(64).Padding(8).Gap(12).AlignItems(ui.Center).PassThrough()
			if !full {
				row.Padding(4).Justify(ui.Center)
			}
			row.Children(func() {
				ui.Box(c).Size(48, 48).Shrink(0).Radius(tokenRadiusArt).Background(p.highlight)
				if !full {
					return
				}
				// Bars of varied length read as titles rather than a table.
				ui.Column(c).Grow(1).MinWidth(0).Gap(8).Children(func() {
					ui.Box(c).Width(float32(120 + 40*(i%3))).Height(12).Radius(tokenRadiusPill).Background(p.highlight)
					ui.Box(c).Width(float32(72 + 24*((i+1)%3))).Height(10).Radius(tokenRadiusPill).Background(p.highlight)
				})
			})
		}
	})
}

func nativeLibraryPrompt(c *ui.Context, m *UIModel, a ViewActions, p nativePalette) {
	ui.Column(c).Padding(8, 8).Children(func() {
		ui.Column(c).Padding(16).Gap(12).Radius(tokenRadiusControl).Background(p.raised).Children(func() {
			title, hint := nativeLibraryPromptText(m)
			ui.Text(c, a.Tr(title)).FontSize(tokenSizeBody).FontWeight(700)
			ui.Text(c, a.Tr(hint)).FontSize(tokenSizeSmall).LineHeight(1.5).TextColor(p.secondary)
			switch {
			case nativeRestoring(m):
				// The Library page and Settings carry the slow-restore detail.
				ui.Row(c).Gap(8).Wrap().Children(func() { nativeRestoreActions(c, m, a, p, true) })
			case !m.SignedIn:
				ui.Row(c).Gap(8).Wrap().MarginY(8).Children(func() {
					if nativePill(c, a.Tr("shellSignIn"), p, true).Clicked() {
						a.Account()
					}
					nativeRestoreActions(c, m, a, p, false)
				})
			case m.LibraryErrorKey != "" && (m.Route != "library" || m.Error == ""):
				if nativePill(c, a.Tr("buttonRetry"), p, true).AlignSelf(ui.Start).MarginY(8).Disabled(a.RetryLibrary == nil).Clicked() {
					a.RetryLibrary()
				}
			}
		})
	})
}

// nativeLibraryPromptText names the library explanation for the account state.
func nativeLibraryPromptText(m *UIModel) (title, body string) {
	switch {
	case m.SignedIn:
		switch {
		case m.LibraryErrorKey != "":
			return "nativeLibraryFailedTitle", m.LibraryErrorKey
		case !m.LibraryLoaded:
			return "nativeLibraryLoadingTitle", "nativeLibraryLoadingBody"
		default:
			return "nativeLibraryEmptyTitle", "nativeLibraryEmptyBody"
		}
	case nativeRestoring(m):
		return "nativeAccountRestoring", "nativeAccountRestoringBody"
	case nativeRestoreDeferred(m):
		return "nativeAccountRestoreDeferred", "nativeAccountRestoreDeferredBody"
	}
	return "shellLibraryEmptyTitle", "shellLibraryEmptyBody"
}

func nativeLibraryRow(c *ui.Context, item catalog.Item, m *UIModel, a ViewActions, p nativePalette, full bool) {
	open := item.BrowseID != "" && m.Route == "browse:"+item.BrowseID
	playing := item.ID != "" && item.ID == m.Current.ID
	b := ui.ButtonBase(c).Height(tokenProductLibraryRow).Padding(8).Gap(12).Radius(tokenRadiusControl).Justify(ui.Start).Label(nativeItemLabel(item)).Tooltip(item.Title)
	if m.LibraryCached && !m.SignedIn {
		b.Disabled(true)
	}
	if !full {
		b.Padding(4).Justify(ui.Center)
	}
	if open {
		// Selected keeps the row's text colour; MyGo's selected text is the
		// inverted theme colour, for fills, not washes.
		b.Selected(true).Background(p.press).TextColor(p.text)
	} else if b.Hovered() {
		b.Background(p.hover)
	}
	b.Children(func() {
		radius := float32(4)
		if item.Kind == "artist" {
			radius = 24
		}
		nativeArtwork(c, item.Artwork, 48, radius, a, p)
		if !full {
			return
		}
		ui.Column(c).Grow(1).MinWidth(0).Gap(4).Children(func() {
			title := ui.Text(c, item.Title).FontSize(tokenSizeSmall).FontWeight(500).MaxLines(1).Ellipsis("…")
			if playing {
				title.TextColor(p.accentText)
			}
			ui.Text(c, nativeLibrarySubtitle(item, a)).FontSize(tokenSizeMeta).TextColor(p.secondary).MaxLines(1).Ellipsis("…")
		})
		if playing {
			ui.Icon(c, nativeIcons["volume"]).Size(16, 16).TextColor(p.accent)
		}
	})
	b.ContextMenu(nativeTrackMenu(item, a))
	if b.Clicked() {
		if item.ID != "" {
			a.Play(item)
		} else {
			a.Open(item)
		}
	}
}

func nativeContentInset(compact bool) float32 {
	if compact {
		return 16
	}
	return 24
}

// nativeColumns fits as many columns of at least minWidth as width holds and
// returns how many, with the width that makes them fill it exactly.
func nativeColumns(width, minWidth, gap float32) (int, float32) {
	n := max(1, int((width+gap)/(minWidth+gap)))
	return n, float32(math.Floor(float64((width - gap*float32(n-1)) / float32(n))))
}

func nativeContent(c *ui.Context, m *UIModel, a ViewActions, p nativePalette, compact bool) {
	inset := nativeContentInset(compact)
	scroll := ui.Scroll(c.Key("content-" + m.Route + "-" + m.Page.Title)).Grow(1).MinHeight(0).Label(a.Tr("shellContent"))
	contentWidth := scroll.Bounds().W
	a = nativeArtworkViewport(c, a, scroll)
	nativeLoadNearEnd(c, scroll, m, a)
	scroll.Children(func() {
		if strings.HasPrefix(m.Route, "browse:") && m.Page.Title != "" {
			nativeCollectionHeader(c, m, a, p, compact, inset, contentWidth)
		} else if m.Page.Title != "" {
			top := float32(32)
			if compact {
				top = 8
			}
			ui.Column(c).Padding(top, inset, 0).Gap(8).Children(func() {
				size := float32(tokenSizeH2)
				if compact {
					size = tokenSizeH3
				}
				nativeDisplay(ui.Text(c, m.Page.Title).FontSize(size).LineHeight(nativeDisplayLeading(m.Page.Title)).MaxLines(2).Ellipsis("…").Role(ui.RoleHeading), m.Page.Title, size)
				if m.Page.Subtitle != "" {
					ui.Text(c, m.Page.Subtitle).TextColor(p.secondary).FontSize(tokenSizeMeta)
				}
			})
		}
		ui.Column(c).Padding(24, inset, 32).Gap(32).Children(func() {
			nativePageBody(c, m, a, p, compact, contentWidth)
		})
	})
}

type nativeContinuation struct{ seen, requested string }

// nativeLoadNearEnd asks for the next page once the reader is within a
// screen of the end, as Spotify does, instead of offering a button; a page
// shorter than its viewport is at its end. A token is acted on from the
// frame after it arrives, when the layout includes the page it continues,
// and is asked for once: a failed or repeated one waits until the reader
// scrolls away from the end and back, or presses Retry.
func nativeLoadNearEnd(c *ui.Context, scroller ui.Element, m *UIModel, a ViewActions) {
	token := m.Page.Continuation
	if token == "" || a.LoadMore == nil || nativeContinuationExpired(m) {
		return
	}
	state := ui.Local(scroller, "continuation", func() nativeContinuation { return nativeContinuation{} })
	scroll := ui.Local(scroller, "artwork-scroll", func() nativeArtworkScroll { return nativeArtworkScroll{} })
	view := scroller.Bounds()
	if state.seen != token || view.H == 0 {
		state.seen = token
		c.Invalidate()
		return
	}
	if scroll.scroll.MaxY-scroll.scroll.Y > view.H {
		state.requested = ""
		return
	}
	if !m.Loading && state.requested != token {
		state.requested = token
		a.LoadMore()
	}
}

// nativeContinuationFailed reports a failed next page: a page load clears
// the page, so a catalog error beside a continuation came from loading more.
func nativeContinuationFailed(m *UIModel) bool {
	return m.Error != "" && m.ErrorScope == errorCatalog && m.Page.Continuation != ""
}

func nativeContinuationExpired(m *UIModel) bool {
	return nativeContinuationFailed(m) && m.errorMessage.key == "nativeCatalogPageExpired" && m.Error == m.errorMessage.rendered
}

func nativePageBody(c *ui.Context, m *UIModel, a ViewActions, p nativePalette, compact bool, contentWidth float32) {
	if m.AccountNotice != "" {
		ui.Text(c, a.Tr(m.AccountNotice)).TextColor(p.secondary)
	}
	if m.Route == "library" {
		nativeLibraryCacheStatus(c, m, a, p)
	}
	if m.Loading && m.Page.Continuation == "" {
		ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
			ui.Spinner(c).Size(20, 20).Label(a.Tr("shellLoading"))
			ui.Text(c, a.Tr("shellLoading")).TextColor(p.secondary)
		})
	}
	if m.Error != "" && !nativeContinuationFailed(m) {
		nativePageError(c, m, a, p)
	}
	nativePageContent(c, m, a, p, compact, contentWidth)
	if m.Page.Continuation != "" {
		// The next page loads out of sight; the footer keeps the room for its
		// spinner, so nothing shifts when it shows, and says when it failed.
		if nativeContinuationFailed(m) {
			// The reader is at the end when it fails, so show the whole card.
			card := nativePageError(c.Key("continuation-error"), m, a, p)
			if revealed := ui.Local(card, "revealed", func() bool { return false }); !*revealed {
				*revealed = true
				card.ScrollIntoView()
			}
		} else {
			ui.Row(c).Height(48).Justify(ui.Center).AlignItems(ui.Center).Children(func() {
				if m.Loading {
					ui.Spinner(c).Size(24, 24).Label(a.Tr("shellLoading"))
				}
			})
		}
	}
	if !m.Loading && m.Error == "" && len(m.Page.Items) == 0 && len(m.Page.Shelves) == 0 && !(m.Route == "library" && !m.SignedIn) {
		ui.Column(c).Padding(48, 16).Gap(12).AlignItems(ui.Center).Children(func() {
			if m.Route == "library" && m.SignedIn {
				ui.Icon(c, nativeIcons["library"]).Size(48, 48).TextColor(p.muted)
				nativeDisplay(ui.Text(c, a.Tr("nativeLibraryEmptyTitle")).FontSize(tokenSizeH3), a.Tr("nativeLibraryEmptyTitle"), tokenSizeH3)
				ui.Text(c, a.Tr("nativeLibraryEmptyBody")).TextColor(p.secondary)
				return
			}
			ui.Icon(c, nativeIcons["search"]).Size(48, 48).TextColor(p.muted)
			nativeDisplay(ui.Text(c, a.Tr("nativeEmptyResults")).FontSize(tokenSizeH3), a.Tr("nativeEmptyResults"), tokenSizeH3)
			ui.Text(c, a.Tr("shellSearchPlaceholder")).TextColor(p.secondary)
		})
	}
}

// nativePageError is the page's error card. Retry loads the next page again
// when that failed, keeping what is shown, and reloads the page otherwise.
func nativePageError(c *ui.Context, m *UIModel, a ViewActions, p nativePalette) ui.Element {
	retry, label := a.Reload, a.Tr("buttonRetry")
	if nativeContinuationExpired(m) {
		label = a.Tr("buttonReload")
	} else if nativeContinuationFailed(m) {
		retry = a.LoadMore
	}
	return ui.Column(c).Padding(16).Gap(12).Background(p.raised).Radius(tokenRadiusControl).Children(func() {
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, nativeIcons["alert"]).Size(16, 16).Shrink(0).TextColor(p.attention)
			ui.Text(c, m.Error).TextColor(p.text)
		})
		if m.ErrorScope != errorAccount && m.ErrorScope != errorAccountConnect && retry != nil && nativePill(c, label, p, true).AlignSelf(ui.Start).Clicked() {
			retry()
		}
		// A failed restore names its retry here, except where the library
		// card below already offers it.
		if m.ErrorScope == errorAccount && m.Route != "library" {
			ui.Row(c).Gap(8).Children(func() { nativeRestoreActions(c, m, a, p, true) })
		}
	})
}

// nativePageContent is what the page loaded: the library prompt, home
// shortcuts, shelves and items.
func nativePageContent(c *ui.Context, m *UIModel, a ViewActions, p nativePalette, compact bool, contentWidth float32) {
	if m.Route == "library" && !m.SignedIn && (!m.LibraryCached || m.AccountRestore != accountRestoring) {
		ui.Column(c).Padding(24).Gap(12).Radius(tokenRadiusControl).Background(p.raised).Children(func() {
			title, body := nativeLibraryPromptText(m)
			if m.LibraryCached {
				body = "nativeLibraryReconnectBody"
			}
			nativeDisplay(ui.Text(c, a.Tr(title)).FontSize(tokenSizeH3), a.Tr(title), tokenSizeH3)
			ui.Text(c, a.Tr(body)).TextColor(p.secondary)
			if m.AccountRestore == accountRestoreSlow {
				ui.Text(c, a.Tr("nativeAccountRestoreSlow")).TextColor(p.secondary)
			}
			ui.Row(c).Gap(8).Wrap().Children(func() {
				if !nativeRestoring(m) && nativePill(c, a.Tr("shellSignIn"), p, true).Clicked() {
					a.Account()
				}
				nativeRestoreActions(c, m, a, p, nativeRestoring(m))
			})
		})
	}
	if m.Route == "home" && m.SignedIn && len(m.LibraryItems) > 0 {
		columns := 4
		if compact || (contentWidth > 0 && contentWidth < 900) {
			columns = 2
		}
		ui.Grid(c).Columns(columns).Gap(8).Children(func() {
			for i, item := range m.LibraryItems[:min(len(m.LibraryItems), 8)] {
				nativeShortcutTile(c.Key(fmt.Sprintf("shortcut-%d", i)), item, m, a, p)
			}
		})
	}
	browse := strings.HasPrefix(m.Route, "browse:")
	// Shelves and grids size to the content column; its width is known from
	// the second frame, so the first lays out at a typical desktop width.
	inner := contentWidth - 2*nativeContentInset(compact)
	if contentWidth == 0 {
		inner = 960
	}
	trackOffset := 0
	for i, shelf := range m.Page.Shelves {
		offset := trackOffset
		if shelf.Title == "" {
			// Continuations append untitled shelves to the same track list.
			trackOffset += len(shelf.Items)
		} else {
			offset, trackOffset = 0, 0
		}
		ui.Column(c.Key(fmt.Sprintf("shelf-%d", i))).Gap(12).Children(func() {
			if shelf.Title != "" {
				size := float32(tokenSizeH3)
				nativeDisplay(ui.Text(c, shelf.Title).FontSize(size).MaxLines(1).Ellipsis("…").Role(ui.RoleHeading), shelf.Title, size)
			}
			switch {
			case shelf.Title == "" || (m.Route == "search" && nativeSongShelf(shelf.Items)):
				nativeTrackList(c, shelf.Items, m, a, p, compact, browse, offset)
			case nativeSongShelf(shelf.Items):
				columnWidth := float32(340)
				if compact {
					columnWidth = 300
				}
				// Columns that all fit stretch to fill the row; more than fit
				// keep their width so the next one peeks in to scroll to.
				if groups := (len(shelf.Items) + 3) / 4; groups > 0 {
					if fit, _ := nativeColumns(inner, columnWidth, 16); groups <= fit {
						columnWidth = min(float32(math.Floor(float64((inner-16*float32(groups-1))/float32(groups)))), 560)
					}
				}
				shelfScroll := ui.ScrollHorizontal(c).Gap(16).PaddingY(4).Height(4*56 + 8)
				clipped := nativeShelfViewport(c, a, shelfScroll)
				shelfScroll.Children(func() {
					for start := 0; start < len(shelf.Items); start += 4 {
						ui.Column(c.Key(fmt.Sprintf("songs-%d", start))).Width(columnWidth).Shrink(0).Children(func() {
							for j := start; j < min(start+4, len(shelf.Items)); j++ {
								nativeTrack(c.Key(fmt.Sprintf("song-%d", j)), shelf.Items[j], j, m, clipped, p, nativeTrackRow, true, false)
							}
						})
					}
				})
			default:
				size := nativeCardArt(compact)
				shelfScroll := ui.ScrollHorizontal(c).PaddingY(4).Height(size + 112).MarginX(-12)
				clipped := nativeShelfViewport(c, a, shelfScroll)
				shelfScroll.Children(func() {
					for j, item := range shelf.Items {
						nativeCard(c.Key(fmt.Sprintf("card-%d", j)), item, m, clipped, p, size)
					}
				})
			}
		})
	}
	if len(m.Page.Shelves) == 0 && len(m.Page.Items) > 0 {
		if nativeSongShelf(m.Page.Items) {
			nativeTrackList(c, m.Page.Items, m, a, p, compact, browse, 0)
		} else {
			// Cards take the column count that fits and grow to fill the
			// row, so a grid never ends in a ragged gap.
			columns, cell := nativeColumns(inner+24, nativeCardArt(compact)+24, 0)
			ui.Grid(c).Columns(columns).MarginX(-12).Children(func() {
				for i, item := range m.Page.Items {
					nativeCard(c.Key(fmt.Sprintf("item-%d", i)), item, m, a, p, cell-24)
				}
			})
		}
	}
}

// nativeCollectionHeader is the album/playlist/artist hero: cover, display
// title and a primary play button over the page's colour wash.
func nativeCollectionHeader(c *ui.Context, m *UIModel, a ViewActions, p nativePalette, compact bool, inset, contentWidth float32) {
	if contentWidth == 0 {
		contentWidth = 960
	}
	_, height := c.Size()
	short := height < 760
	title := func(size float32) {
		nativeDisplay(ui.Text(c, m.Page.Title).FontSize(size).LineHeight(nativeDisplayLeading(m.Page.Title)).MaxLines(2).Ellipsis("…").Role(ui.RoleHeading), m.Page.Title, size)
		if creator := strings.TrimSpace(m.Page.Creator); creator != "" && !nativeMetadataHasCreator(m.Page.Subtitle, creator) {
			ui.Text(c, creator).FontSize(tokenSizeSmall).FontWeight(600).MaxLines(1).Ellipsis("…")
		}
		if m.Page.Subtitle != "" {
			ui.Text(c, m.Page.Subtitle).FontSize(tokenSizeMeta).FontWeight(500).TextColor(p.secondary).MaxLines(2).Ellipsis("…")
		}
	}
	if compact && short {
		ui.Row(c).Padding(8, inset, 0).Gap(16).AlignItems(ui.Center).Children(func() {
			if m.Page.Artwork != "" {
				nativeArtwork(c, m.Page.Artwork, min(96, contentWidth*0.25), tokenRadiusControl, a, p).Shrink(0)
			}
			ui.Column(c).Grow(1).MinWidth(0).Gap(8).Children(func() { title(24) })
		})
	} else if compact {
		ui.Column(c).Padding(8, inset, 0).Gap(12).Children(func() {
			if m.Page.Artwork != "" {
				nativeArtwork(c, m.Page.Artwork, min(contentWidth-2*inset-64, 224), tokenRadiusControl, a, p).AlignSelf(ui.Center).Shadow(tokenShadowHero.args())
			}
			title(26)
		})
	} else {
		art := float32(tokenProductCollectionCover)
		if contentWidth < 900 {
			art = 192
		}
		top, font := float32(40), max(float32(32), min(72, contentWidth*0.055))
		if short {
			art, top, font = 112, 16, 32
		}
		ui.Row(c).Padding(top, inset, 0).Gap(24).AlignItems(ui.End).Children(func() {
			if m.Page.Artwork != "" {
				nativeArtwork(c, m.Page.Artwork, art, tokenRadiusControl, a, p).Shadow(tokenShadowHero.args())
			}
			ui.Column(c).Grow(1).MinWidth(0).Gap(12).Children(func() {
				title(font)
			})
		})
	}
	items := playableItems(m.Page.Items)
	if len(items) == 0 {
		return
	}
	inPage := false
	for _, item := range items {
		inPage = inPage || item.ID == m.Current.ID
	}
	buttonSize, buttonTop := float32(tokenProductPlayButton), float32(24)
	if short {
		buttonSize, buttonTop = tokenProductTargetTouch, 12
	}
	ui.Row(c).Padding(buttonTop, inset, 0).Gap(24).AlignItems(ui.Center).Children(func() {
		icon, key := "play", "shellPlay"
		if inPage && m.Playing {
			icon, key = "pause", "shellPause"
		}
		b := ui.ButtonBase(c).Size(buttonSize, buttonSize).Shrink(0).Radius(buttonSize / 2).Background(p.accent).TextColor(p.onAccent).Shadow(tokenShadowControl.args()).Label(a.Tr(key) + " · " + m.Page.Title).Tooltip(a.Tr(key)).Disabled(inPage && m.PlaybackLoading)
		nativePlayFeedback(b)
		b.Children(func() { ui.Icon(c, nativeIcons[icon]).Size(26, 26) })
		if b.Clicked() {
			if inPage {
				a.Toggle()
			} else {
				a.Play(items[0])
			}
		}
	})
}

func nativeMetadataHasCreator(subtitle, creator string) bool {
	split := func(value string) []string {
		return strings.FieldsFunc(value, func(r rune) bool { return r == '·' || r == '•' })
	}
	present := map[string]bool{}
	for _, part := range split(subtitle) {
		present[strings.ToLower(strings.TrimSpace(part))] = true
	}
	creators := split(creator)
	for _, part := range creators {
		if !present[strings.ToLower(strings.TrimSpace(part))] {
			return false
		}
	}
	return len(creators) > 0
}

// nativeShortcutTile is one of Home's quick-access tiles for library items.
func nativeShortcutTile(c *ui.Context, item catalog.Item, m *UIModel, a ViewActions, p nativePalette) {
	playing := item.ID != "" && item.ID == m.Current.ID
	b := ui.ButtonBase(c).Height(tokenProductTile).MinWidth(0).PaddingEnd(12).Gap(12).Radius(tokenRadiusControl).Justify(ui.Start).Background(p.hover).Clip().Label(item.Title).Tooltip(item.Title)
	hovered := b.Hovered()
	if hovered {
		b.Background(p.press)
	}
	b.Children(func() {
		nativeArtwork(c, item.Artwork, tokenProductTile, 0, a, p)
		t := ui.Text(c, item.Title).Grow(1).MinWidth(0).FontSize(tokenSizeSmall).FontWeight(700).MaxLines(2).Ellipsis("…")
		if playing {
			t.TextColor(p.accentText)
			ui.Icon(c, nativeIcons["volume"]).Size(16, 16).TextColor(p.accent)
		} else if hovered && item.ID != "" {
			ui.Box(c).Size(32, 32).Shrink(0).Radius(tokenRadiusPill).Background(p.accent).TextColor(p.onAccent).Center().PassThrough().Children(func() { ui.Icon(c, nativeIcons["play"]).Size(16, 16) })
		}
	})
	b.ContextMenu(nativeTrackMenu(item, a))
	if b.Clicked() {
		if item.ID != "" {
			a.Play(item)
		} else {
			a.Open(item)
		}
	}
}

// The current cover is shared by the panel and transport. Resolve it on the
// first frame, even before layout; only lists need viewport-based scheduling.
func nativePlayerArtwork(c *ui.Context, src string, size, radius float32, a ViewActions, p nativePalette) ui.Element {
	return nativeCover(c, src, size, radius, a, p, artPlayer)
}

func nativeArtwork(c *ui.Context, src string, size, radius float32, a ViewActions, p nativePalette) ui.Element {
	priority := artPrimary
	if a.secondaryArtwork {
		priority = artSecondary
	}
	return nativeCover(c, src, size, radius, a, p, priority)
}

func nativeCover(c *ui.Context, src string, size, radius float32, a ViewActions, p nativePalette, priority artworkPriority) ui.Element {
	box := ui.Box(c).Size(size, size).Shrink(0).Radius(radius).Clip().Center()
	var artwork ui.ImageSource
	if a.Artwork != nil && src != "" && priority == artPlayer {
		artwork = a.Artwork(src, int(math.Ceil(float64(size)*2)), artPlayer)
	} else if a.Artwork != nil && src != "" {
		bounds := box.Bounds()
		if bounds.W == 0 || bounds.H == 0 {
			// Bounds describe the previous layout; resolve a new cover on the
			// next frame, once we know whether its scroll containers reveal it.
			c.Invalidate()
		} else {
			viewport := a.artViewport
			if a.viewportScroll == nil {
				viewport.W, viewport.H = c.Size()
			}
			visible := nativeIntersection(bounds, viewport)
			if visible.W > 0 && visible.H > 0 {
				artwork = a.Artwork(src, int(math.Ceil(float64(size)*2)), priority)
			}
		}
	}
	return box.Background(p.highlight).Children(func() {
		if artwork != nil {
			ui.Image(c, artwork).Fill().Fit(ui.Cover)
		} else {
			ui.Icon(c, nativeIcons["note"]).Size(min(size*0.4, 48), min(size*0.4, 48)).TextColor(p.muted)
		}
	})
}

type nativeArtworkScroll struct {
	scroll ui.ScrollState
	x, y   float32
}

func nativeArtworkViewport(c *ui.Context, a ViewActions, scroller ui.Element) ViewActions {
	state := ui.Local(scroller, "artwork-scroll", func() nativeArtworkScroll { return nativeArtworkScroll{} })
	scroller.TrackScroll(&state.scroll)
	if state.x != state.scroll.X || state.y != state.scroll.Y {
		state.x, state.y = state.scroll.X, state.scroll.Y
		c.Invalidate()
	}
	bounds := scroller.Bounds()
	if bounds.W == 0 || bounds.H == 0 {
		c.Invalidate()
	}
	if a.viewportScroll == nil {
		a.artViewport.W, a.artViewport.H = c.Size()
	}
	a.artViewport = nativeIntersection(a.artViewport, bounds)
	a.viewportScroll = &state.scroll
	return a
}

func nativeShelfViewport(c *ui.Context, a ViewActions, scroller ui.Element) ViewActions {
	page := a.viewportScroll
	shelf := nativeArtworkViewport(c, a, scroller)
	row := shelf.viewportScroll
	rtl := c.Direction() == ui.RightToLeft
	scroller.HandleInput(func(ev ui.InputEvent) bool {
		// Leave single-axis scrolling (including Shift+wheel) to MyGo.
		// For diagonal input, consume the minor axis even at an edge.
		if ev.Kind != ui.InputScroll || ev.DX == 0 || ev.DY == 0 {
			return false
		}
		if math.Abs(float64(ev.DY)) >= math.Abs(float64(ev.DX)) {
			page.Y = max(0, min(page.MaxY, page.Y+ev.DY))
		} else {
			dx := ev.DX
			if rtl {
				dx = -dx
			}
			row.X = max(0, min(row.MaxX, row.X+dx))
		}
		return true
	})
	return shelf
}

func nativeIntersection(a, b ui.Rect) ui.Rect {
	x, y := max(a.X, b.X), max(a.Y, b.Y)
	return ui.Rect{X: x, Y: y, W: max(float32(0), min(a.X+a.W, b.X+b.W)-x), H: max(float32(0), min(a.Y+a.H, b.Y+b.H)-y)}
}

func nativeCardArt(compact bool) float32 {
	if compact {
		return 140
	}
	return tokenProductCardCover
}

// nativeCard is Spotify's shelf card: cover, title and a two-line subtitle,
// with a play button rising over playable covers on hover.
func nativeCard(c *ui.Context, item catalog.Item, m *UIModel, a ViewActions, p nativePalette, size float32) {
	b := ui.ButtonBase(c).Column().Width(size + 24).Shrink(0).Padding(12).Gap(8).Radius(tokenRadiusControl).AlignItems(ui.Stretch).Justify(ui.Start).Label(nativeResultLabel(item, m, a))
	if m.Route == "library" && m.LibraryCached && !m.SignedIn {
		b.Disabled(true)
	}
	hovered := b.Hovered()
	if hovered {
		b.Background(p.raised)
	}
	b.Children(func() {
		radius := float32(tokenRadiusArt) // inside the card's padding: nested
		if item.Kind == "artist" {
			radius = size / 2
		}
		ui.Box(c).Size(size, size).Shrink(0).Children(func() {
			nativeArtwork(c, item.Artwork, size, radius, a, p).Shadow(tokenShadowArt.args())
			if hovered && item.ID != "" {
				ui.Box(c).Absolute().Right(8).Bottom(8).Size(48, 48).Radius(tokenRadiusPill).Background(p.accent).TextColor(p.onAccent).Shadow(tokenShadowControl.args()).Center().PassThrough().Children(func() {
					ui.Icon(c, nativeIcons["play"]).Size(22, 22)
				})
			}
		})
		title := ui.Text(c, item.Title).FontSize(tokenSizeSmall).FontWeight(700).MaxLines(1).Ellipsis("…")
		if item.ID != "" && item.ID == m.Current.ID {
			title.TextColor(p.accentText)
		}
		subtitle := nativeResultSubtitle(item, m, a)
		if subtitle == "" {
			subtitle = nativeKindLabel(item, a)
		}
		ui.Text(c, subtitle).FontSize(tokenSizeMeta).LineHeight(1.4).TextColor(p.secondary).MaxLines(2).Ellipsis("…")
	})
	b.ContextMenu(nativeTrackMenu(item, a))
	if b.Clicked() {
		if item.ID != "" {
			a.Play(item)
		} else {
			a.Open(item)
		}
	}
}

type nativeTrackMode uint8

const (
	nativeTrackRow      nativeTrackMode = iota // search results and shelf columns
	nativeTrackNumbered                        // album and playlist tables
	nativeTrackQueue                           // the Now Playing queue
)

func nativeTrackList(c *ui.Context, items []catalog.Item, m *UIModel, a ViewActions, p nativePalette, compact, numbered bool, offset int) {
	mode := nativeTrackRow
	if numbered {
		mode = nativeTrackNumbered
	}
	album := !compact && !nativeOneAlbum(items)
	ui.Column(c).Children(func() {
		if numbered && !compact {
			ui.Row(c).Height(36).PaddingX(16).Gap(16).AlignItems(ui.Center).TextColor(p.secondary).Children(func() {
				ui.Text(c, "#").Width(24).Shrink(0).FontSize(tokenSizeMeta).TextAlign(ui.Center)
				ui.Text(c, a.Tr("shellColumnTitle")).Grow(1).Basis(0).MinWidth(0).FontSize(tokenSizeMeta).MaxLines(1).Ellipsis("…")
				if album {
					ui.Text(c, a.Tr("shellColumnAlbum")).Grow(0.75).Basis(0).MinWidth(0).FontSize(tokenSizeMeta).MaxLines(1).Ellipsis("…")
				}
				ui.Row(c).Width(48).Shrink(0).Justify(ui.End).Label(a.Tr("shellColumnDuration")).Children(func() {
					ui.Icon(c, nativeIcons["clock"]).Size(16, 16)
				})
			})
			ui.Box(c).Height(1).Margin(0, 0, 8).Background(p.line)
		}
		for j, item := range items {
			nativeTrack(c.Key(fmt.Sprintf("list-%d", j)), item, offset+j, m, a, p, mode, compact, album)
		}
	})
}

// nativeOneAlbum reports whether every row comes from the same album, as on
// an album's own page, where an Album column would repeat its title.
func nativeOneAlbum(items []catalog.Item) bool {
	if len(items) == 0 || items[0].Album == "" {
		return false
	}
	for _, item := range items[1:] {
		if item.Album != items[0].Album {
			return false
		}
	}
	return true
}

func nativeTrack(c *ui.Context, item catalog.Item, index int, m *UIModel, a ViewActions, p nativePalette, mode nativeTrackMode, compact, album bool) {
	playing := item.ID != "" && item.ID == m.Current.ID
	pad := float32(16)
	if compact {
		pad = 8
	}
	b := ui.ButtonBase(c).Height(tokenProductRow).PaddingX(pad).Gap(16).Radius(tokenRadiusControl).Justify(ui.Start).Label(nativeResultLabel(item, m, a))
	if m.Route == "library" && m.LibraryCached && !m.SignedIn && mode != nativeTrackQueue {
		b.Disabled(true)
	}
	hovered := b.Hovered()
	if playing {
		b.Selected(true).Background(p.hover).TextColor(p.text)
	}
	if hovered {
		b.Background(p.press)
	}
	b.Children(func() {
		if mode == nativeTrackNumbered {
			ui.Box(c).Width(24).Shrink(0).Center().Children(func() {
				switch {
				case playing:
					ui.Icon(c, nativeIcons["volume"]).Size(16, 16).TextColor(p.accent)
				case hovered:
					ui.Icon(c, nativeIcons["play"]).Size(14, 14).TextColor(p.text)
				default:
					ui.Text(c, fmt.Sprint(index+1)).FontSize(tokenSizeSmall).TextColor(p.secondary)
				}
			})
		}
		ui.Row(c).Grow(1).Basis(0).MinWidth(0).Gap(12).AlignItems(ui.Center).Children(func() {
			radius := float32(4)
			if item.Kind == "artist" {
				radius = 20
			}
			nativeArtwork(c, item.Artwork, 40, radius, a, p)
			ui.Column(c).Grow(1).MinWidth(0).Gap(4).Children(func() {
				t := ui.Text(c, item.Title).FontSize(tokenSizeSmall).FontWeight(500).MaxLines(1).Ellipsis("…")
				if playing {
					t.TextColor(p.accentText)
				}
				subtitle := item.Artist
				if mode == nativeTrackRow {
					subtitle = nativeResultSubtitle(item, m, a)
				}
				artist := ui.Text(c, subtitle).FontSize(tokenSizeMeta).TextColor(p.secondary).MaxLines(1).Ellipsis("…")
				if hovered {
					artist.TextColor(p.text)
				}
			})
		})
		if album {
			ui.Text(c, item.Album).Grow(0.75).Basis(0).MinWidth(0).FontSize(tokenSizeMeta).TextColor(p.secondary).MaxLines(1).Ellipsis("…")
		}
		if mode != nativeTrackNumbered {
			// A fixed slot, filled while playing, so the playing row's columns
			// line up with the rest.
			ui.Box(c).Size(16, 16).Shrink(0).Children(func() {
				if playing {
					ui.Icon(c, nativeIcons["volume"]).Size(16, 16).TextColor(p.accent)
				}
			})
		}
		if item.Duration > 0 || mode == nativeTrackNumbered {
			duration := ""
			if item.Duration > 0 {
				duration = nativeTime(item.Duration.Seconds())
			}
			ui.Text(c, duration).Width(48).Shrink(0).FontSize(tokenSizeMeta).TextColor(p.secondary).TextAlign(ui.End)
		}
	})
	b.ContextMenu(nativeTrackMenu(item, a))
	if b.Clicked() {
		if mode == nativeTrackQueue {
			a.PlayQueue(index)
		} else if item.ID != "" {
			a.Play(item)
		} else {
			a.Open(item)
		}
	}
}

// nativePlayerBar is the desktop player: track and like on the left, the
// transport centred, and view and volume controls on the right.
func nativePlayerBar(c *ui.Context, m *UIModel, a ViewActions, p nativePalette, width float32) ui.Element {
	return ui.Box(c).Background(p.base).Clip().Children(func() {
		nativePlayerWash(c, m, a, p)
		nativePlayerBarContent(c, m, a, p, width)
	})
}

func nativePlayerBarContent(c *ui.Context, m *UIModel, a ViewActions, p nativePalette, width float32) ui.Element {
	return ui.Row(c).Absolute().Left(0).Top(0).WidthPercent(100).HeightPercent(100).Padding(0, 16).Gap(16).AlignItems(ui.Center).Label(a.Tr("shellPlayerControls")).Children(func() {
		ui.Row(c).Grow(1).Basis(0).MinWidth(0).Gap(8).AlignItems(ui.Center).Children(func() {
			info := ui.ButtonBase(c).MinWidth(0).Shrink(1).Height(64).Padding(4).Gap(12).Radius(tokenRadiusControl).Justify(ui.Start).Label(a.Tr("shellOpenNowPlaying"))
			if info.Hovered() {
				info.Background(p.hover)
			}
			info.Children(func() {
				nativePlayerArtwork(c, m.Current.Artwork, 56, 4, a, p)
				nativeTrackCaption(c, m, a, p)
			})
			if info.Clicked() {
				m.Panel = true
			}
			if m.PlaybackError != "" && !m.Panel && !m.Immersive {
				if nativePill(c, a.Tr("buttonRetry"), p, false).Width(72).PaddingX(8).Shrink(0).Disabled(a.RetryPlayback == nil).Clicked() {
					a.RetryPlayback()
				}
			} else if m.PlaybackError == "" && nativeIconButton(c, nativeHeart(m.Liked), a.Tr(nativeLikeKey(m.Liked)), p, m.Liked, 32).Disabled(m.Current.ID == "").Clicked() {
				a.Like()
			}
		})
		ui.Column(c).Width(min(722, width*0.38)).Shrink(0).Gap(4).AlignItems(ui.Stretch).Children(func() {
			ui.Row(c).Height(40).Gap(16).AlignItems(ui.Center).Justify(ui.Center).Children(func() {
				if nativeModeButton(c, "shuffle", a.Tr("shellShuffle"), p, m.Shuffle, 32).Disabled(len(m.Queue) < 2).Clicked() {
					a.Shuffle()
				}
				if nativeIconButton(c, "previous", a.Tr("shellPrevious"), p, false, 32).Disabled(m.Current.ID == "").Clicked() {
					a.Previous()
				}
				nativePlayButton(c, m, a, p, 36)
				if nativeIconButton(c, "next", a.Tr("shellNext"), p, false, 32).Disabled(m.Current.ID == "").Clicked() {
					a.Next()
				}
				icon, key := nativeRepeat(m)
				if nativeModeButton(c, icon, a.Tr(key), p, m.Repeat > 0, 32).Clicked() {
					a.Repeat()
				}
			})
			nativeSeek(c, m, a, p)
		})
		ui.Row(c).Grow(1).Basis(0).MinWidth(0).Gap(4).AlignItems(ui.Center).Justify(ui.End).Children(func() {
			if width > 1000 {
				if nativeIconButton(c, "lyrics", a.Tr("shellLyrics"), p, m.Panel && m.selectedPanelTab() == "lyrics", 32).Clicked() {
					nativeTogglePanel(m, "lyrics")
				}
			}
			if nativeIconButton(c, "queue", a.Tr("shellQueue"), p, m.Panel && m.selectedPanelTab() == "queue", 32).Clicked() {
				nativeTogglePanel(m, "queue")
			}
			nativeVolume(c, m, a, p, width > 1000)
			if width > 1000 {
				nativeIconButton(c, "more", a.Tr("shellMore"), p, false, 32).Disabled(m.Current.ID == "").Menu(nativeTrackMenu(m.Current, a))
			}
			if nativeIconButton(c, "expand", a.Tr("shellImmersive"), p, m.Immersive, 32).Clicked() {
				m.Immersive = !m.Immersive
			}
		})
	})
}

func (m *UIModel) selectedPanelTab() string {
	if m.PanelTab == "queue" {
		return "queue"
	}
	return "lyrics"
}

// nativeTogglePanel opens Now Playing on a tab, or closes it when that tab is
// already showing, as Spotify's queue and lyrics buttons do.
func nativeTogglePanel(m *UIModel, tab string) {
	if m.Panel && m.selectedPanelTab() == tab {
		m.Panel = false
		return
	}
	m.Panel, m.PanelTab = true, tab
}

func nativeTrackCaption(c *ui.Context, m *UIModel, a ViewActions, p nativePalette) {
	ui.Column(c).Grow(1).MinWidth(0).Gap(4).Children(func() {
		title, artist := m.Current.Title, m.Current.Artist
		if title == "" {
			title, artist = a.Tr("shellIdleTitle"), a.Tr("shellIdleSubtitle")
		}
		ui.Text(c, title).FontSize(tokenSizeSmall).FontWeight(600).MaxLines(1).Ellipsis("…")
		if m.PlaybackError != "" {
			artist = a.Tr("nativePlaybackFailedStatus")
			if !m.Panel && !m.Immersive {
				artist = m.PlaybackError
			}
		} else if nativeAwaitingAccount(m) {
			artist = a.Tr("nativePlaybackAwaitingAccount")
		}
		caption := ui.Text(c, artist).FontSize(tokenSizeMicro).TextColor(p.secondary).MaxLines(1).Ellipsis("…")
		if m.PlaybackError != "" {
			caption.TextColor(p.attention)
			caption.Tooltip(m.PlaybackError)
		}
	})
}

func nativeRepeat(m *UIModel) (icon, key string) {
	switch m.Repeat {
	case 1:
		return "repeat", "shellRepeatAll"
	case 2:
		return "repeat-one", "shellRepeatOne"
	}
	return "repeat", "shellRepeatOff"
}

// nativeMiniPlayer is the compact player card above the tab bar.
func nativeMiniPlayer(c *ui.Context, m *UIModel, a ViewActions, p nativePalette) {
	ui.Column(c).Shrink(0).Background(p.base).Padding(8, 8, 0).Children(func() {
		// The card takes the playing cover's colour, as the player bar's wash does.
		card := p.raised
		if tint, ok := nativePlayingTint(m, a); ok {
			card = p.raised.Mix(tint, nativeWashStrength(p))
		}
		ui.Column(c).Height(tokenProductMiniPlayer).Radius(tokenRadiusControl).Background(card).Clip().Children(func() {
			ui.Row(c).Grow(1).Padding(0, 4, 0, 8).Gap(4).AlignItems(ui.Center).Children(func() {
				info := ui.ButtonBase(c).Grow(1).MinWidth(0).Height(52).Gap(8).Justify(ui.Start).Radius(tokenRadiusArt).Label(a.Tr("shellOpenNowPlaying")).Children(func() {
					nativePlayerArtwork(c, m.Current.Artwork, 40, 4, a, p)
					nativeTrackCaption(c, m, a, p)
				})
				if info.Clicked() {
					m.Panel = true
				}
				if m.PlaybackError != "" && !m.Panel {
					if nativePill(c, a.Tr("buttonRetry"), p, false).Width(64).PaddingX(8).Shrink(0).Disabled(a.RetryPlayback == nil).Clicked() {
						a.RetryPlayback()
					}
				} else if m.PlaybackError == "" && nativeIconButton(c, nativeHeart(m.Liked), a.Tr(nativeLikeKey(m.Liked)), p, m.Liked, 40).Disabled(m.Current.ID == "").Clicked() {
					a.Like()
				}
				nativePlayButton(c, m, a, p, 40)
			})
			nativeProgressLine(c, m, p)
		})
	})
}

func nativePlayButton(c *ui.Context, m *UIModel, a ViewActions, p nativePalette, size float32) {
	if m.PlaybackLoading {
		ui.ButtonBase(c).Size(size, size).Shrink(0).Radius(size / 2).Background(p.text).TextColor(p.base).Disabled(true).Label(a.Tr("shellLoading")).Children(func() { ui.Spinner(c).Size(size*0.5, size*0.5) })
		return
	}
	icon, key := "play", "shellPlay"
	if m.Playing {
		icon, key = "pause", "shellPause"
	}
	b := ui.ButtonBase(c).Size(size, size).Shrink(0).Radius(size / 2).Label(a.Tr(key)).Tooltip(a.Tr(key)).Background(p.text).TextColor(p.base).Disabled(m.Current.ID == "")
	nativePlayFeedback(b)
	b.Children(func() { ui.Icon(c, nativeIcons[icon]).Size(size*0.5, size*0.5) })
	if b.Clicked() {
		a.Toggle()
	}
}

type nativeSeekState struct {
	trackID                  string
	value                    float64
	pressed, dirty, canceled bool
}

func nativeSeek(c *ui.Context, m *UIModel, a ViewActions, p nativePalette) {
	row := ui.Row(c).Height(24).Gap(8).AlignItems(ui.Center)
	state := ui.Local(row, "seek-preview", func() nativeSeekState { return nativeSeekState{} })
	enabled := m.Duration > 0 && !m.PlaybackLoading
	if state.trackID != m.Current.ID || !enabled {
		state.trackID, state.value, state.dirty = m.Current.ID, m.Position, false
		state.canceled = state.canceled || state.pressed
	} else if !state.pressed {
		state.value = m.Position
	}
	row.Children(func() {
		ui.Text(c, nativeTime(state.value)).Width(40).TextAlign(ui.End).FontSize(tokenSizeMicro).TextColor(p.secondary)
		s := nativeSlider(c.Key("seek"), &state.value, max(1, m.Duration), p).Grow(1).Label(a.Tr("shellProgress")).Disabled(!enabled)
		changed, pressed := s.Changed(), s.Pressed()
		if changed {
			state.dirty = true
		}
		// Dragging previews locally and commits when the pointer is released.
		// Keyboard and accessibility changes have no pointer press, so each
		// commits immediately. Keep that target visible for the next key.
		if state.canceled {
			state.value, state.dirty = m.Position, false
			if enabled && !pressed {
				state.canceled = false
			}
		} else if enabled && !pressed && state.dirty && (changed || state.pressed) {
			m.Position = state.value
			state.dirty = false
			a.Seek(state.value)
		}
		state.pressed = pressed
		if enabled && !state.canceled && (s.Hovered() || pressed) {
			preview := state.value
			if !pressed {
				x, _, _ := s.PointerPosition()
				preview = float64(max(0, min(1, (x-5)/max(1, s.LayoutBounds().W-10)))) * m.Duration
			}
			nativeSliderReadout(c.Key("seek-readout"), s, p, nativeTime(math.Round(preview)), preview/m.Duration)
		}
		ui.Text(c, nativeTime(m.Duration)).Width(40).FontSize(tokenSizeMicro).TextColor(p.secondary)
	})
}

// nativeSliderThumb is the slider thumb's diameter; the track is inset by
// its radius so the thumb stays inside the control at either end.
const nativeSliderThumb = 12

func nativeSlider(c *ui.Context, value *float64, maximum float64, p nativePalette) ui.Element {
	s := ui.SliderBase(c, value, 0, maximum).Height(24).PaddingX(nativeSliderThumb / 2)
	hovered := s.Hovered() || s.Focused() || s.Pressed()
	fraction := float32(max(0, min(1, *value/maximum)))
	s.Draw(func(painter *ui.Painter, r ui.Rect) {
		track := ui.Rect{X: r.X + nativeSliderThumb/2, Y: r.Y + r.H/2 - 2, W: max(0, r.W-nativeSliderThumb), H: 4}
		painter.Fill(track, p.text.Alpha(0.3), 2)
		fill := track
		fill.W *= fraction
		col := p.text
		if hovered {
			col = p.accent
		}
		painter.Fill(fill, col, 2)
		if hovered {
			painter.Fill(ui.Rect{X: track.X + fill.W - nativeSliderThumb/2, Y: r.Y + r.H/2 - nativeSliderThumb/2, W: nativeSliderThumb, H: nativeSliderThumb}, p.text, nativeSliderThumb/2)
		}
	})
	return s
}

// Slider values stay visible while pressed, including when a drag leaves the
// track. Ordinary tooltips dismiss on press, so use a non-interactive overlay.
func nativeSliderReadout(c *ui.Context, slider ui.Element, p nativePalette, text string, fraction float64) {
	x := 5 + max(0, slider.LayoutBounds().W-10)*float32(max(0, min(1, fraction)))
	ui.Overlay(c, func() {
		ui.Text(c, text).AttachTo(slider, ui.AnchorTopLeft, ui.AnchorBottom).Margin(0, 0, 8, x).
			Padding(4, 8).Radius(tokenRadiusPill).Background(p.raised).TextColor(p.text).FontSize(tokenSizeMicro).FontWeight(600).
			Shadow(tokenShadowTooltip.args()).PassThrough().Role(ui.RoleTooltip)
	})
}

func nativeProgressLine(c *ui.Context, m *UIModel, p nativePalette) {
	value := float32(0)
	if m.Duration > 0 {
		value = float32(max(0, min(1, m.Position/m.Duration)))
	}
	ui.Box(c).Height(2).Shrink(0).Margin(0, 8, 0).Radius(tokenRadiusPill).Background(p.text.Alpha(0.2)).Children(func() {
		ui.Box(c).Height(2).Radius(tokenRadiusPill).WidthPercent(value * 100).Background(p.text)
	})
}

func nativeVolume(c *ui.Context, m *UIModel, a ViewActions, p nativePalette, showSlider bool) {
	icon, key := "volume", "shellMute"
	if m.Muted || m.Volume == 0 {
		icon, key = "volume-off", "shellUnmute"
	} else if m.Volume < 0.5 {
		icon = "volume-low"
	}
	if nativeIconButton(c, icon, a.Tr(key), p, false, 32).Clicked() {
		a.Mute()
	}
	if showSlider {
		volume := m.Volume
		s := nativeSlider(c.Key("volume"), &volume, 1, p).Width(93).Label(a.Tr("shellVolume"))
		if s.Changed() {
			a.Volume(volume)
		}
		if s.Hovered() || s.Pressed() {
			nativeSliderReadout(c.Key("volume-readout"), s, p, fmt.Sprintf("%.0f%%", volume*100), volume)
		}
	}
}

func nativeTabbar(c *ui.Context, m *UIModel, a ViewActions, p nativePalette) {
	ui.Row(c).Height(nativeCompactTabbar).Shrink(0).Padding(4, 8).Background(p.base).Label(a.Tr("shellNavMain")).Children(func() {
		for _, nav := range []struct{ route, key, icon string }{{"home", "shellNavHome", "home"}, {"search", "shellNavSearch", "search"}, {"explore", "shellNavExplore", "explore"}, {"library", "shellNavLibrary", "library"}} {
			active := m.Route == nav.route
			b := ui.ButtonBase(c.Key("tab-" + nav.route)).Column().Grow(1).Gap(4).Radius(tokenRadiusControl).Label(a.Tr(nav.key)).Selected(active).Background(ui.Transparent).TextColor(p.secondary)
			if active || b.Hovered() {
				b.TextColor(p.text)
			}
			b.Children(func() {
				icon := nav.icon
				if active && icon != "search" {
					icon += "-fill"
				}
				ui.Icon(c, nativeIcons[icon]).Size(24, 24)
				ui.Text(c, a.Tr(nav.key)).FontSize(tokenSizeMicro).FontWeight(600).MaxLines(1).Ellipsis("…")
			})
			if b.Clicked() {
				m.Immersive = false
				m.Panel = false
				if nav.route == "search" {
					m.FocusSearch = true
				} else {
					a.Navigate(nav.route)
				}
			}
		}
	})
}

// nativeNowPlaying is the right-hand Now Playing view: large cover, title,
// and a card holding the queue or lyrics.
func nativeNowPlaying(c *ui.Context, m *UIModel, a ViewActions, p nativePalette, width float32, overlay, revealLyrics bool) ui.Element {
	_, windowHeight := c.Size()
	gap := float32(16)
	if windowHeight < 760 {
		gap = 12
	}
	return ui.Column(c).Background(p.surface).Radius(tokenRadiusPanel).MinHeight(0).Clip().Children(func() {
		nativeNowPlayingWash(c, m, a, p)
		nativePageLayer(c).Padding(16).Gap(gap).Children(func() { nativeNowPlayingBody(c, m, a, p, width, overlay, revealLyrics, windowHeight, gap) })
	})
}

func nativeNowPlayingBody(c *ui.Context, m *UIModel, a ViewActions, p nativePalette, width float32, overlay, revealLyrics bool, windowHeight, gap float32) {
	{
		ui.Row(c).Height(32).Shrink(0).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, a.Tr("shellNowPlaying")).FontSize(tokenSizeBody).FontWeight(700).Grow(1).MinWidth(0).MaxLines(1).Ellipsis("…")
			if nativeIconButton(c, "expand", a.Tr("shellImmersive")+" · "+a.Tr("shellNowPlaying"), p, false, 32).Clicked() {
				m.Immersive = true
				m.Panel = false
			}
			if nativeIconButton(c, "close", a.Tr("shellClosePanel"), p, false, 32).Clicked() {
				m.Panel = false
			}
		})
		if m.Current.ID == "" {
			ui.Column(c).Grow(1).Gap(16).AlignItems(ui.Center).Justify(ui.Center).Children(func() {
				nativeArtwork(c, "", 112, 8, a, p)
				nativeDisplay(ui.Text(c, a.Tr("shellEmptyTitle")).FontSize(tokenSizeH3), a.Tr("shellEmptyTitle"), tokenSizeH3)
				ui.Text(c, a.TrArgs("shellEmptyBody", map[string]any{"shortcut": "⌘ K"})).TextColor(p.secondary).TextAlign(ui.Center)
			})
			return
		}
		body := func() {
			nativePlaybackNotice(c, m, a, p)
			// Lyrics are a first-class view: the cover takes a share of the
			// height it shares with them, never first claim on it, and grows
			// to the panel's width only when the window has room for both.
			artSize := min(width-32, max(nativeCoverMin, (windowHeight-nativeNowPlayingChrome)*nativeCoverShare))
			if overlay {
				artSize = min(artSize, max(nativeCoverMin, windowHeight-600))
			}
			// Lyrics first: the cover becomes a thumbnail beside the title and
			// the lyrics take the rest. A video keeps its size.
			lyricsFirst := m.LyricsExpanded && m.selectedPanelTab() == "lyrics" && !m.Video
			lyricsExtra := float32(0)
			identity := func(art bool) {
				ui.Row(c).Shrink(0).Gap(12).AlignItems(ui.Center).Children(func() {
					if art {
						nativePlayerArtwork(c, m.Current.Artwork, nativeCoverThumb, tokenRadiusArt, a, p)
					}
					ui.Column(c).Grow(1).MinWidth(0).Gap(4).Children(func() {
						nativeDisplay(ui.Text(c, m.Current.Title).FontSize(tokenSizeH3).LineHeight(1.2).MaxLines(2).Ellipsis("…"), m.Current.Title, tokenSizeH3)
						ui.Text(c, m.Current.Artist).FontSize(tokenSizeSmall).TextColor(p.secondary).MaxLines(1).Ellipsis("…")
					})
					if nativeIconButton(c, nativeHeart(m.Liked), a.Tr(nativeLikeKey(m.Liked)), p, m.Liked, 36).Clicked() {
						a.Like()
					}
				})
			}
			if lyricsFirst {
				lyricsExtra = max(0, artSize-nativeCoverThumb)
				identity(true)
				nativeMediaSwitch(c, m, a, p)
			} else {
				nativeMedia(c, m, a, p, artSize).AlignSelf(ui.Center)
				nativeMediaSwitch(c, m, a, p)
				identity(false)
			}
			if overlay {
				ui.Row(c).Shrink(0).Gap(16).AlignItems(ui.Center).Justify(ui.Center).Children(func() {
					if nativeIconButton(c, "previous", a.Tr("shellPrevious"), p, false, 44).Clicked() {
						a.Previous()
					}
					nativePlayButton(c, m, a, p, 48)
					if nativeIconButton(c, "next", a.Tr("shellNext"), p, false, 44).Clicked() {
						a.Next()
					}
				})
				nativeSeek(c, m, a, p)
				ui.Row(c).Shrink(0).AlignItems(ui.Center).Justify(ui.Center).Children(func() { nativeVolume(c, m, a, p, true) })
			}
			// Keyed: the layouts above it differ, and its reading position must
			// survive switching between them.
			card := nativePanelCard(c.Key("now-playing-card"), m, a, p)
			if overlay {
				card.Height(300 + lyricsExtra).Shrink(0)
				if revealLyrics {
					// On short windows the card begins below the fold. Reveal it
					// on entry only, so the reader can still scroll back to the cover.
					card.ScrollIntoView()
				}
			} else {
				card.Grow(1).MinHeight(120)
			}
		}
		if overlay {
			scroll := ui.Scroll(c).Grow(1).MinHeight(0).Gap(gap)
			a = nativeArtworkViewport(c, a, scroll)
			scroll.Children(body)
		} else {
			body()
		}
	}
}

// nativePanelCard holds the queue and lyrics behind a pair of chips.
func nativePanelCard(c *ui.Context, m *UIModel, a ViewActions, p nativePalette) ui.Element {
	return ui.Column(c).MinHeight(0).Padding(12).Gap(12).Radius(tokenRadiusControl).Background(p.raised).Children(func() {
		nativePanelTabs(c, m, a, p)
		nativePanelBody(c, m, a, p)
	})
}

func nativePanelTabs(c *ui.Context, m *UIModel, a ViewActions, p nativePalette) {
	ui.Row(c).Shrink(0).Gap(8).AlignItems(ui.Center).Children(func() {
		for _, tab := range []struct{ id, key string }{{"lyrics", "shellLyrics"}, {"queue", "shellQueue"}} {
			active := m.selectedPanelTab() == tab.id
			if nativeChip(c, a.Tr(tab.key), p, active).Label(a.Tr(tab.key) + " · " + a.Tr("shellNowPlaying")).Clicked() {
				m.PanelTab = tab.id
			}
		}
		if !m.Immersive && !m.Video && m.selectedPanelTab() == "lyrics" {
			ui.Spacer(c)
			// The lyrics card grows over the cover, or gives the space back.
			// Its glyph says which, so it needs no active dot; it sits in the
			// chips' row in the chips' style, part of that control row.
			icon, label := "grow-vertical", "nativeLyricsExpand"
			if m.LyricsExpanded {
				icon, label = "shrink-vertical", "nativeLyricsRestore"
			}
			b := nativeIconButton(c.Key("lyrics-layout"), icon, a.Tr(label), p, false, tokenProductChip).TextColor(p.text).Background(p.hover)
			if b.Hovered() {
				b.Background(p.press)
			}
			if b.Disabled(a.SetLyricsExpanded == nil).Clicked() {
				a.SetLyricsExpanded(!m.LyricsExpanded)
			}
		}
	})
}

func nativePanelBody(c *ui.Context, m *UIModel, a ViewActions, p nativePalette) ui.Element {
	if m.selectedPanelTab() == "lyrics" {
		return nativeLyricsBody(c, m, a, p)
	}
	scroll := ui.Scroll(c.Key("panel-queue")).Grow(1).MinHeight(68)
	a = nativeArtworkViewport(c, a, scroll)
	return scroll.Children(func() {
		for i, item := range m.Queue {
			nativeTrack(c.Key(fmt.Sprintf("queue-%d", i)), item, i, m, a, p, nativeTrackQueue, true, false)
		}
	})
}

// nativeImmersiveHeader is the immersive view's bar, and its title bar under
// a hidden one. It returns its height.
func nativeImmersiveHeader(c *ui.Context, m *UIModel, a ViewActions, p nativePalette, width float32) float32 {
	bar := c.TitleBar()
	left, right := nativeTitleBarInsets(bar, 20)
	height := max(64, bar.Height)
	buttons := float32(2)
	if a.AppMenu != nil {
		buttons++
	}
	nativeTitleBarRow(c, bar).Height(height).Padding(12, right, 12, left).Gap(4).AlignItems(ui.Center).Children(func() {
		nativeAppMenuButton(c, a, p, 40)
		if nativeIconButton(c, "chevron-down", a.Tr("shellExitImmersive"), p, false, 40).Clicked() {
			m.Immersive = false
		}
		ui.Spacer(c)
		// Window controls can leave the title too little room: it goes first.
		if width-left-right-buttons*44 >= 120 {
			ui.Text(c, a.Tr("shellNowPlaying")).FontSize(tokenSizeMeta).FontWeight(600).TextColor(p.secondary)
			ui.Spacer(c)
		}
		if nativeIconButton(c, "settings", a.Tr("shellSettings"), p, false, 40).Clicked() {
			m.Settings = true
		}
	})
	return height
}

// nativeImmersive lays out the immersive view in height, under its own header
// unless the frame drew it in the title bar.
func nativeImmersive(c *ui.Context, m *UIModel, a ViewActions, p nativePalette, width, height float32, header bool) {
	compact := width <= 760
	headerHeight := float32(0)
	if header {
		headerHeight = nativeImmersiveHeader(c, m, a, p, width)
	}
	if compact {
		scroll := ui.Scroll(c).Grow(1).Padding(16).Gap(24)
		a = nativeArtworkViewport(c, a, scroll)
		scroll.Children(func() {
			nativePlaybackNotice(c, m, a, p)
			mediaHeight := max(float32(180), height-390)
			artSize := min(width-64, mediaHeight)
			if m.Video {
				// As on desktop, landscape video keeps the cover's height
				// budget and uses the extra width instead of shrinking to it.
				artSize = min(width-64, mediaHeight*16/9)
			}
			nativeMedia(c, m, a, p, artSize).AlignSelf(ui.Center)
			nativeMediaSwitch(c, m, a, p)
			nativeImmersiveTitle(c, m, a, p, true)
			nativePanelCard(c, m, a, p).Height(280)
		})
		return
	}
	panelWidth := min(float32(380), width*0.32)
	contentHeight := max(float32(0), height-headerHeight-48)
	// The frame gap on both sides, a gutter at each edge and one between the
	// media and the panel.
	mediaWidth := max(float32(1), width-2*tokenProductFrameGap-3*nativeImmersiveGutter-panelWidth)
	ui.Row(c).Grow(1).MinHeight(0).Padding(24, nativeImmersiveGutter).Gap(nativeImmersiveGutter).AlignItems(ui.Center).Children(func() {
		scroll := ui.Scroll(c).Grow(1).MinWidth(0).MinHeight(0).HeightPercent(100).Label(a.Tr("shellContent"))
		// The queue card is a sibling of the media column, so it keeps the
		// outer viewport instead of the left column's clip.
		media := nativeArtworkViewport(c, a, scroll)
		scroll.Children(func() {
			a := media
			ui.Column(c).MinHeight(contentHeight).Shrink(0).Gap(24).AlignItems(ui.Center).Justify(ui.Center).Children(func() {
				// Reserve the switch, both gaps and the title block as the
				// fonts actually lay it out. Extra artist lines/notices remain
				// reachable by scrolling.
				textHeight := 32 + 48 + nativeImmersiveTitleHeight(m, a, mediaWidth)
				if m.PlaybackError != "" {
					textHeight += 24 + 56
				}
				// The text has priority: the media takes whatever is left, even
				// below its usual size, as a fixed floor would push the artist
				// under the player. It keeps a pixel so its box keeps bounds.
				mediaHeight := max(1, contentHeight-textHeight)
				artSize := min(mediaWidth, mediaHeight)
				if m.Video {
					// A landscape video can use the same vertical budget as the
					// square cover while filling more of the available column.
					artSize = min(mediaWidth, mediaHeight*16/9)
				}
				nativeMedia(c, m, a, p, artSize).Shadow(tokenShadowDialog.args())
				nativeMediaSwitch(c, m, a, p)
				nativeImmersiveTitle(c, m, a, p, false)
				nativePlaybackNotice(c, m, a, p)
			})
		})
		nativePanelCard(c, m, a, p).Width(panelWidth).HeightPercent(90).Background(p.base.Alpha(0.35))
	})
}

func nativeImmersiveTitle(c *ui.Context, m *UIModel, a ViewActions, p nativePalette, compact bool) {
	ui.Column(c).Gap(8).AlignItems(ui.Center).Children(func() {
		title, artist := nativeImmersiveTitleText(m, a)
		size := float32(tokenSizeH2)
		if compact {
			size = tokenSizeH3
		}
		nativeDisplay(ui.Text(c, title).FontSize(size).LineHeight(nativeDisplayLeading(title)).TextAlign(ui.Center).MaxLines(2).Ellipsis("…"), title, size)
		ui.Text(c, artist).FontSize(tokenSizeBody).FontWeight(500).TextColor(p.secondary).TextAlign(ui.Center)
	})
}

func nativeImmersiveTitleText(m *UIModel, a ViewActions) (title, artist string) {
	if m.Current.Title == "" {
		return a.Tr("shellEmptyTitle"), a.Tr("shellIdleSubtitle")
	}
	return m.Current.Title, m.Current.Artist
}

// nativeImmersiveTitleHeight measures the desktop nativeImmersiveTitle at
// width: up to two title lines, the gap and the first artist line. Fallback
// fonts (CJK, or Linux without the system UI font) can need far more height
// than the design font, so a fixed estimate would let the media squeeze the
// artist out of a short window.
func nativeImmersiveTitleHeight(m *UIModel, a ViewActions, width float32) float32 {
	title, artist := nativeImmersiveTitleText(m, a)
	titleFont := nativeDisplayMetrics(tokenSizeH2, nativeDisplayLeading(title))
	_, titleHeight := ui.ShapeText(title, titleFont, width).Size()
	// A single unwrapped line has the tallest font any wrapped line uses.
	_, titleLine := ui.ShapeText(title, titleFont, 0).Size()
	_, artistLine := ui.ShapeText(artist, ui.Font{Size: tokenSizeBody, Weight: 500}, 0).Size()
	return min(titleHeight, 2*titleLine) + 8 + artistLine
}

func nativeTime(seconds float64) string {
	if seconds < 0 {
		seconds = 0
	}
	d := time.Duration(seconds) * time.Second
	total := int(d.Seconds())
	return fmt.Sprintf("%d:%02d", total/60, total%60)
}
func nativeHeart(liked bool) string {
	if liked {
		return "heart-fill"
	}
	return "heart"
}
func nativeLikeKey(liked bool) string {
	if liked {
		return "shellUnlike"
	}
	return "shellLike"
}

func nativeSongShelf(items []catalog.Item) bool {
	if len(items) == 0 {
		return false
	}
	songs := 0
	for _, item := range items {
		if item.ID != "" && (item.Kind == "song" || item.Kind == "video" || item.Kind == "") {
			songs++
		}
	}
	return songs*2 >= len(items)
}

// A presentation action can occur after its destination was built in this pass.
// Request the next frame explicitly so it becomes visible without another input.
type nativePresentationKey struct {
	texts                              [16]string
	flags                              [21]bool
	values                             [4]float64
	repeat, queueLength, libraryLength int
	errorScope                         errorScope
	libraryLoaded, shortcutRecording   bool
	shortcut, shortcutError            string
	videoQuality                       int
	videoQualityError                  string
}

func nativePresentation(m *UIModel) nativePresentationKey {
	return nativePresentationKey{
		texts:  [16]string{m.Route, m.Query, m.PanelTab, m.Appearance, m.Language, m.Current.ID, m.Current.Title, m.Page.Title, m.Error, m.Lyrics, m.Current.Artwork, m.VideoError, m.PlaybackError, m.AccountRestore, m.LibraryErrorKey, m.AccountNotice},
		flags:  [21]bool{m.Loading, m.Playing, m.Muted, m.Liked, m.Shuffle, m.Panel, m.Immersive, m.Settings, m.Shortcuts, m.SidebarCollapsed, m.SignedIn, m.CanBack, m.CanForward, m.FocusSearch, m.Video, m.VideoLoading, m.PlaybackLoading, m.MusicPremium, m.LibraryCached, m.LibraryRefreshing, m.LyricsExpanded},
		values: [4]float64{m.Volume, m.Position, m.Duration, 0}, repeat: m.Repeat, queueLength: len(m.Queue), libraryLength: len(m.LibraryItems),
		errorScope: m.ErrorScope, libraryLoaded: m.LibraryLoaded, shortcutRecording: m.ShortcutRecording, shortcut: m.GlobalShortcut, shortcutError: m.ShortcutError,
		videoQuality: m.VideoQuality, videoQualityError: m.VideoQualityError,
	}
}

// nativeBackdrop washes the top of the page in colour: the blurred cover of a
// collection, the current track's cover in immersive mode, or a stable tint
// for pages without artwork.
func nativeBackdrop(c *ui.Context, m *UIModel, a ViewActions, p nativePalette) {
	src, priority := m.Page.Artwork, artSecondary
	if m.Immersive {
		src, priority = m.Current.Artwork, artPlayer
	}
	var artwork ui.ImageSource
	if src != "" && a.Artwork != nil {
		artwork = a.Artwork(src, artworkEdge, priority)
	}
	strength := nativeWashStrength(p)
	if artwork == nil {
		if m.Immersive {
			return
		}
		ui.Box(c).Absolute().Left(0).Top(0).WidthPercent(100).Height(320).LinearGradient(ui.LinearGradient{From: nativeHeaderTint(m.Route).Alpha(strength), To: nativeHeaderTint(m.Route).Alpha(0), Angle: 180}).PassThrough()
		return
	}
	layer := nativeCoverBlur(c, artwork)
	if m.Immersive {
		layer.HeightPercent(25).Opacity(strength * 0.7)
	} else {
		layer.Height(80).Opacity(strength)
	}
	if !m.Immersive {
		// The blur spills below its 320-DIP band; fade it into the panel.
		ui.Box(c).Absolute().Left(0).Top(0).WidthPercent(100).Height(560).LinearGradient(ui.LinearGradient{From: p.surface.Alpha(0), To: p.surface, Angle: 180, Start: 0.2, End: 0.7}).PassThrough()
	}
}

// nativeCoverBlur is a cover blurred across its parent, drawn from a
// quarter-size layer: give it a height (a quarter of the band it covers)
// and an opacity. Blur a small layer, then draw it at the final size without
// a transform: a transformed child makes MyGo allocate a full-size RGBA mask
// for the rounded parent clip, which would erase the smaller canvas's savings.
func nativeCoverBlur(c *ui.Context, artwork ui.ImageSource) ui.Element {
	layer := ui.Box(c).Absolute().Left(0).Top(0).WidthPercent(25).Effect(backdropLayer{}).PassThrough()
	layer.Children(func() { ui.Image(c, artwork).Fill().Fit(ui.Cover).Blur(17.5).PassThrough() })
	return layer
}

// nativePlayingTint is the colour the playing track's cover lends the
// interface, once the cover has loaded.
func nativePlayingTint(m *UIModel, a ViewActions) (ui.Color, bool) {
	if m.Current.ID == "" || m.Current.Artwork == "" || a.ArtworkTint == nil {
		return ui.Color{}, false
	}
	return a.ArtworkTint(m.Current.Artwork)
}

// nativeWashStrength is how strongly a tint colours the chrome: the
// guidebook's "the music leads", kept quiet enough that text stays legible.
func nativeWashStrength(p nativePalette) float32 {
	if p.dark {
		return tokenWashDark
	}
	return tokenWashLight
}

// nativeNowPlayingWash colours the top of the Now Playing panel with the
// playing cover's colour, fading into the panel, as a page without
// artwork is washed by its tint.
func nativeNowPlayingWash(c *ui.Context, m *UIModel, a ViewActions, p nativePalette) {
	tint, ok := nativePlayingTint(m, a)
	if !ok {
		return
	}
	ui.Box(c).Absolute().Left(0).Top(0).WidthPercent(100).Height(360).LinearGradient(ui.LinearGradient{From: tint.Alpha(nativeWashStrength(p)), To: tint.Alpha(0), Angle: 180}).PassThrough()
}

// nativePlayerWash glows the playing cover's colour behind the player bar's
// track, fading out toward the transport: what's playing carries colour.
func nativePlayerWash(c *ui.Context, m *UIModel, a ViewActions, p nativePalette) {
	tint, ok := nativePlayingTint(m, a)
	if !ok {
		return
	}
	ui.Box(c).Absolute().Left(0).Top(0).WidthPercent(100).HeightPercent(100).LinearGradient(ui.LinearGradient{From: tint.Alpha(nativeWashStrength(p) * tokenWashBar), To: tint.Alpha(0), Angle: 90, End: 0.4}).PassThrough()
}

// The inner blur extends three sigma beyond its quarter-size image. Drawing
// that complete layer four times larger preserves the crop and 70-DIP blur.
type backdropLayer struct{}

func (backdropLayer) EffectMargin() ui.Point { return ui.Point{X: 52.5, Y: 52.5} }

func (backdropLayer) PaintEffect(p *ui.Painter, box ui.Rect, content *ui.Bitmap, inset ui.Point) {
	w, h := content.Size()
	scale := 4 / p.Scale()
	p.ImageRegion(content, ui.Rect{}, ui.Rect{
		X: box.X - inset.X*scale, Y: box.Y - inset.Y*scale,
		W: float32(w) * scale, H: float32(h) * scale,
	}, ui.ImageOptions{})
}

func nativeTrackMenu(item catalog.Item, a ViewActions) func(*ui.Menu) {
	return func(menu *ui.Menu) {
		if item.ID != "" {
			if menu.Item(a.Tr("shellPlay")).Chosen() {
				a.Play(item)
			}
			if menu.Item(a.Tr("nativeAddToQueue")).Disabled(a.QueueAppend == nil).Chosen() {
				a.QueueAppend(item)
			}
		} else if menu.Item(item.Title).Chosen() {
			a.Open(item)
		}
	}
}

func nativeKeyboard(c *ui.Context, m *UIModel, a ViewActions) {
	if m.ShortcutRecording {
		return
	}
	// MyGo gives focused text editors and controls first refusal, including Space.
	if c.Shortcut(0, ui.KeySpace) && m.Current.ID != "" {
		a.Toggle()
	}
	if c.Shortcut(ui.Cmd, ui.KeyK) {
		m.Immersive = false
		m.Panel = false
		m.FocusSearch = true
	}
	if c.Shortcut(ui.Cmd, ui.KeyJ) {
		m.Panel = !m.Panel
	}
	if c.Shortcut(ui.Cmd|ui.Shift, ui.KeyF) {
		m.Immersive = !m.Immersive
	}
	if c.Shortcut(ui.Cmd, ui.KeyB) {
		m.SidebarCollapsed = !m.SidebarCollapsed
	}
	if c.Shortcut(ui.Cmd|ui.Shift, ui.KeyM) {
		a.Mute()
	}
	if c.Shortcut(ui.Cmd, ui.KeyRight) {
		a.Next()
	}
	if c.Shortcut(ui.Cmd, ui.KeyLeft) {
		a.Previous()
	}
	if c.Shortcut(ui.Cmd, ui.KeyUp) {
		a.Volume(min(1, m.Volume+0.05))
	}
	if c.Shortcut(ui.Cmd, ui.KeyDown) {
		a.Volume(max(0, m.Volume-0.05))
	}
	if c.Shortcut(ui.Cmd|ui.Shift, ui.KeyH) {
		m.Immersive = false
		a.Navigate("home")
	}
	if c.Shortcut(ui.Cmd, ui.KeyL) {
		m.Immersive = false
		a.Navigate("library")
	}
	if c.Shortcut(0, ui.KeyEscape) {
		if m.Shortcuts {
			m.Shortcuts = false
		} else if m.Settings {
			m.Settings = false
		} else if m.Panel {
			m.Panel = false
		} else if m.Immersive {
			m.Immersive = false
		}
	}
}

func nativeMediaSwitch(c *ui.Context, m *UIModel, a ViewActions, p nativePalette) {
	ui.Row(c).Shrink(0).Gap(8).Wrap().AlignItems(ui.Center).AlignSelf(ui.Center).Children(func() {
		for _, option := range []struct {
			video bool
			key   string
		}{{false, "nativeArtwork"}, {true, "nativeVideo"}} {
			b := nativeChip(c, a.Tr(option.key), p, m.Video == option.video).MinWidth(80).Disabled(m.Current.ID == "" || a.Video == nil)
			if b.Clicked() {
				a.Video(option.video)
			}
		}
		if m.Video {
			nativeVideoQualityControl(c, m, a, p, 108, 32)
		}
	})
}

func nativeMedia(c *ui.Context, m *UIModel, a ViewActions, p nativePalette, size float32) ui.Element {
	if !m.Video {
		return nativePlayerArtwork(c, m.Current.Artwork, size, tokenRadiusControl, a, p)
	}
	// Video plays on a black stage in either appearance, so the stage takes
	// the dark palette.
	stage := palette(true)
	return ui.Box(c).Size(size, size*9/16).Shrink(0).Radius(tokenRadiusControl).Background(stage.base).Clip().Center().Label(a.Tr("nativeVideo")).Children(func() {
		if m.VideoError != "" {
			ui.Column(c).WidthPercent(100).Padding(8).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
					ui.Icon(c, nativeIcons["alert"]).Size(16, 16).Shrink(0).TextColor(stage.attention)
					ui.Text(c, m.VideoError).TextAlign(ui.Center).TextColor(stage.text).FontSize(tokenSizeMeta).MaxLines(2).Ellipsis("…")
				})
				if nativePill(c, a.Tr("buttonRetry"), stage, false).Disabled(a.Video == nil || m.Current.ID == "").Clicked() {
					a.Video(true)
				}
			})
		} else if m.VideoLoading {
			ui.Spinner(c).Size(28, 28).Label(a.Tr("shellLoading"))
		} else if m.VideoFrame != nil {
			ui.Image(c, m.VideoFrame).Size(size, size*9/16).Fit(ui.Contain)
		} else {
			ui.Text(c, a.Tr("nativeVideoUnavailable")).Padding(16).TextAlign(ui.Center).TextColor(stage.secondary).FontSize(tokenSizeMeta)
		}
	})
}

func nativeExpandedTransport(c *ui.Context, m *UIModel, a ViewActions, p nativePalette) ui.Element {
	return ui.Column(c).Padding(8, 16).Gap(4).Background(p.base).Children(func() {
		// Compact immersive content leads with the full playback notice, so
		// the transport keeps its seek bar instead of a truncated copy.
		nativeSeek(c, m, a, p)
		ui.Row(c).Height(56).Gap(16).AlignItems(ui.Center).Justify(ui.Center).Children(func() {
			if nativeModeButton(c, "shuffle", a.Tr("shellShuffle"), p, m.Shuffle, 44).Disabled(len(m.Queue) < 2).Clicked() {
				a.Shuffle()
			}
			if nativeIconButton(c, "previous", a.Tr("shellPrevious"), p, false, 44).Disabled(m.Current.ID == "").Clicked() {
				a.Previous()
			}
			nativePlayButton(c, m, a, p, 56)
			if nativeIconButton(c, "next", a.Tr("shellNext"), p, false, 44).Disabled(m.Current.ID == "").Clicked() {
				a.Next()
			}
			icon, key := nativeRepeat(m)
			if nativeModeButton(c, icon, a.Tr(key), p, m.Repeat > 0, 44).Clicked() {
				a.Repeat()
			}
		})
		ui.Row(c).Height(32).Gap(12).AlignItems(ui.Center).Justify(ui.Center).Children(func() {
			if nativeIconButton(c, nativeHeart(m.Liked), a.Tr(nativeLikeKey(m.Liked)), p, m.Liked, 32).Disabled(m.Current.ID == "").Clicked() {
				a.Like()
			}
			nativeVolume(c, m, a, p, true)
			nativeIconButton(c, "more", a.Tr("shellMore"), p, false, 32).Disabled(m.Current.ID == "").Menu(nativeTrackMenu(m.Current, a))
		})
	})
}

// nativePlaybackNotice explains a failed track, or one waiting for the saved
// account, beside the action that resolves it.
func nativePlaybackNotice(c *ui.Context, m *UIModel, a ViewActions, p nativePalette) {
	if nativeAwaitingAccount(m) {
		ui.Column(c).Shrink(0).Gap(8).Padding(12).Radius(tokenRadiusControl).Background(p.raised).Children(func() {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Spinner(c).Size(16, 16)
				ui.Text(c, a.Tr("nativePlaybackAwaitingAccount")).Grow(1).MinWidth(0).FontSize(tokenSizeMicro).TextColor(p.secondary)
			})
			ui.Row(c).Gap(8).Wrap().Children(func() { nativeRestoreActions(c, m, a, p, true) })
		})
		return
	}
	if m.PlaybackError == "" {
		return
	}
	ui.Row(c).Shrink(0).Gap(8).Padding(12).Radius(tokenRadiusControl).Background(p.raised).AlignItems(ui.Center).Children(func() {
		ui.Icon(c, nativeIcons["alert"]).Size(16, 16).Shrink(0).TextColor(p.attention)
		ui.Text(c, m.PlaybackError).Grow(1).MinWidth(0).FontSize(tokenSizeMicro).TextColor(p.text)
		if nativePill(c, a.Tr("buttonRetry"), p, false).Width(72).PaddingX(8).Disabled(a.RetryPlayback == nil).Clicked() {
			a.RetryPlayback()
		}
	})
}

// nativeAwaitingAccount reports a track load held until restoration settles.
func nativeAwaitingAccount(m *UIModel) bool {
	return m.PlaybackLoading && m.PlaybackError == "" && nativeRestoring(m)
}
