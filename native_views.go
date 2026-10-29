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
	Queue                                                       []catalog.Item
	Route, Query, Error, Lyrics, PanelTab, Appearance, Language string
	LyricLines                                                  []catalog.LyricLine
	LyricsSource                                                string
	LyricsAvailable                                             bool
	LyricsRevision                                              uint64
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
	ErrorScope                                                  errorScope
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
	Seek, Volume                                        func(float64)
	Appearance, Language                                func(string)
	Artwork                                             func(string, int) ui.ImageSource
	Video                                               func(bool)
	RetryPlayback                                       func()
	LoadMore                                            func()
	artViewport                                         ui.Rect
	artClipped                                          bool
}

// Window geometry. Above 1180 DIPs the library is a full panel and Now Playing
// docks on the right; down to 761 the library is a rail and Now Playing floats;
// at 760 and below the window becomes a single column with a mini player and
// a bottom tab bar.
const (
	nativeTopbarHeight  = 64
	nativePlayerHeight  = 80
	nativeLibraryWidth  = 300
	nativeRailWidth     = 72
	nativePanelWidth    = 360
	nativeCompactTabbar = 64
	nativeExpandedBar   = 140
)

func nativeView(c *ui.Context, m *UIModel, a ViewActions) {
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
			backdrop.Background(ui.RGBA(0, 0, 0, 0.5))
			panel.Width(panelWidth).Height(min(height-100, 760)).Radius(8).Background(p.surface).Shadow(0, 16, 48, 0, ui.RGBA(0, 0, 0, 0.5)).Clip().Children(func() {
				nativeNowPlaying(c, m, a, p, panelWidth, true).Fill()
			})
		}).Label(a.Tr("shellNowPlaying"))
	}
	nativeSettings(c, m, a, p, width, height)
	nativeShortcuts(c, m, a, p, width, height)
}

// nativeDesktopFrame is Spotify's desktop arrangement: a global bar on the
// window frame, three rounded panels, and the player bar across the bottom.
func nativeDesktopFrame(c *ui.Context, m *UIModel, a ViewActions, p nativePalette, width, height float32) {
	ui.Column(c).Fill().Background(p.base).TextColor(p.text).Children(func() {
		top := float32(8)
		if !m.Immersive {
			nativeTopbar(c, m, a, p, width)
			top = 0
		}
		ui.Row(c).Grow(1).MinHeight(0).AlignItems(ui.Stretch).Gap(8).Padding(top, 8, 0).Children(func() {
			if !m.Immersive {
				nativeLibrary(c, m, a, p, width)
			}
			ui.Column(c).Grow(1).MinWidth(0).MinHeight(0).Radius(8).Background(p.surface).Clip().Children(func() {
				nativeBackdrop(c, m, a, p)
				nativePageLayer(c).Children(func() {
					if m.Immersive {
						nativeImmersive(c, m, a, p, width, height-nativePlayerHeight-top)
					} else {
						nativeContent(c, m, a, p, false)
					}
				})
			})
			if m.Panel && !m.Immersive && width > 1180 {
				nativeNowPlaying(c, m, a, p, nativePanelWidth, false).Width(nativePanelWidth).Shrink(0)
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
					nativeImmersive(c, m, a, p, width, height-nativeExpandedBar-nativeCompactTabbar)
				} else {
					nativeCompactTopbar(c, m, a, p)
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

func nativeIconButton(c *ui.Context, name, label string, p nativePalette, active bool, hit float32) ui.Element {
	b := ui.ButtonBase(c).Size(hit, hit).Shrink(0).Radius(hit / 2).Label(label).Tooltip(label).TextColor(p.secondary)
	if active {
		b.Selected(true).Background(ui.Transparent).TextColor(p.accent)
	} else if b.Hovered() {
		b.Background(p.hover).TextColor(p.text)
	}
	b.Children(func() {
		icon := min(hit*0.6, 22)
		ui.Icon(c, nativeIcons[name]).Size(icon, icon)
		if active {
			ui.Box(c).Absolute().Left((hit-4)/2).Bottom(1).Size(4, 4).Radius(2).Background(p.accent).PassThrough()
		}
	})
	return b
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
	b.Children(func() { ui.Icon(c, nativeIcons[name]).Size(size/2, size/2) })
	return b
}

// nativePill is a rounded text button; the primary form is Spotify's white
// (in light appearance, black) call to action.
func nativePill(c *ui.Context, label string, p nativePalette, primary bool) ui.Element {
	b := ui.Button(c, label).MinHeight(36).Padding(7, 18).Radius(24).BorderWidth(0).FontWeight(700).Background(p.input).TextColor(p.text)
	if primary {
		b.Selected(true).Background(p.text).TextColor(p.base)
		if b.Hovered() {
			b.Background(p.text.Alpha(0.86))
		}
	} else if b.Hovered() {
		b.Background(p.highlight)
	}
	return b
}

// nativeChip is a filter chip: translucent at rest, inverted when selected.
func nativeChip(c *ui.Context, label string, p nativePalette, selected bool) ui.Element {
	b := ui.Button(c, label).Height(32).MinHeight(32).Padding(0, 12).Radius(16).BorderWidth(0).FontSize(13).FontWeight(500).Background(p.hover).TextColor(p.text)
	if selected {
		b.Selected(true).Background(p.text).TextColor(p.base)
	} else if b.Hovered() {
		b.Background(p.press)
	}
	return b
}

func nativeTopbar(c *ui.Context, m *UIModel, a ViewActions, p nativePalette, width float32) {
	ui.Row(c).Height(nativeTopbarHeight).Shrink(0).PaddingX(16).Gap(16).AlignItems(ui.Center).Children(func() {
		ui.Row(c).Grow(1).Basis(0).MinWidth(0).Gap(8).AlignItems(ui.Center).Children(func() {
			brand := ui.ButtonBase(c).Size(32, 32).Shrink(0).Radius(16).Background(p.accent).TextColor(p.onAccent).Label(a.Tr("shellBrandHome")).Tooltip(a.Tr("shellBrandHome")).Children(func() { ui.Icon(c, nativeIcons["play"]).Size(18, 18) })
			if brand.Clicked() {
				a.Navigate("home")
			}
			ui.Box(c).Width(8).Shrink(0)
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

func nativeCompactTopbar(c *ui.Context, m *UIModel, a ViewActions, p nativePalette) {
	ui.Row(c).Height(60).Shrink(0).Padding(8, 8).Gap(4).AlignItems(ui.Center).Children(func() {
		if nativeIconButton(c, "back", a.Tr("shellBack"), p, false, 44).Disabled(!m.CanBack).Clicked() {
			a.Back()
		}
		nativeSearchField(c, m, a, p, 44, false).Grow(1)
		if nativeIconButton(c, "settings", a.Tr("shellSettings"), p, m.Settings, 44).Clicked() {
			m.Settings = !m.Settings
		}
	})
}

// nativeSearchField is the persistent search pill. On desktop it ends with the
// Explore button, where Spotify places Browse.
func nativeSearchField(c *ui.Context, m *UIModel, a ViewActions, p nativePalette, h float32, explore bool) ui.Element {
	field := ui.Row(c).MinWidth(0).Height(h).PaddingStart(12).PaddingEnd(6).Gap(10).Radius(h/2).Background(p.raised).Border(2, ui.Transparent).AlignItems(ui.Center)
	if field.FocusWithin() {
		field.BorderColor(p.text)
	} else if field.Hovered() {
		field.Background(p.highlight).BorderColor(p.line)
	}
	return field.Children(func() {
		ui.Icon(c, nativeIcons["search"]).Size(22, 22).TextColor(p.secondary)
		input := ui.TextInput(c.Key("search-input"), &m.Query).Grow(1).MinWidth(0).Height(h - 6).Padding(0).BorderWidth(0).Background(ui.Transparent).Placeholder(a.Tr("shellSearchPlaceholder")).Label(a.Tr("shellNavSearch")).FontSize(15)
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
	case !m.SignedIn:
		return a.Tr("shellAccountSignedOut")
	case m.MusicPremium:
		return a.Tr("shellAccountPremium")
	}
	return a.Tr("shellAccountStandard")
}

func nativeAvatar(c *ui.Context, m *UIModel, p nativePalette, size float32) ui.Element {
	bg, fg := p.highlight, p.secondary
	if m.SignedIn {
		bg, fg = p.muted, p.surface
	}
	if m.SignedIn && m.MusicPremium {
		bg, fg = p.accent, p.onAccent
	}
	return ui.Box(c).Size(size, size).Shrink(0).Radius(size / 2).Background(bg).TextColor(fg).Center().Children(func() {
		ui.Icon(c, nativeIcons["user"]).Size(size*0.56, size*0.56)
	})
}

// nativeAccount is the top-right control: a sign-in call to action when
// signed out, otherwise an avatar whose menu states the account's plan.
func nativeAccount(c *ui.Context, m *UIModel, a ViewActions, p nativePalette, badge bool) {
	if !m.SignedIn {
		if nativePill(c, a.Tr("shellSignIn"), p, true).Clicked() {
			a.Account()
		}
		return
	}
	status := nativeAccountStatus(m, a)
	b := ui.ButtonBase(c.Key("account")).Height(48).MinWidth(48).Shrink(0).Padding(8).Gap(8).Radius(24).Background(p.raised).Label(a.Tr("menuAccount") + " · " + status).Tooltip(status)
	if b.Hovered() {
		b.Background(p.highlight)
	}
	b.Children(func() {
		if m.MusicPremium && badge {
			ui.Text(c, a.Tr("shellPremiumBadge")).PaddingStart(6).FontSize(12).FontWeight(700).TextColor(p.accent)
		}
		nativeAvatar(c, m, p, 32)
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

func nativeItemLabel(item catalog.Item) string {
	if artist := strings.TrimSpace(item.Artist); artist != "" {
		return item.Title + " · " + artist
	}
	return item.Title
}

func nativeLibrary(c *ui.Context, m *UIModel, a ViewActions, p nativePalette, width float32) {
	full := width > 1180 && !m.SidebarCollapsed
	panelWidth := float32(nativeRailWidth)
	if full {
		panelWidth = nativeLibraryWidth
	}
	panel := ui.Column(c).Width(panelWidth).Shrink(0).MinHeight(0).Background(p.surface).Radius(8).Clip()
	filter := ui.Local(panel, "library-filter", func() string { return "" })
	panel.Children(func() {
		libraryIcon := "library"
		if m.Route == "library" {
			libraryIcon = "library-fill"
		}
		if full {
			ui.Row(c).Height(56).Shrink(0).Padding(8, 12).Gap(8).AlignItems(ui.Center).Children(func() {
				b := ui.ButtonBase(c.Key("library-title")).Grow(1).MinWidth(0).Height(40).PaddingX(8).Gap(12).Justify(ui.Start).Radius(6).Label(a.Tr("shellNavLibrary")).Tooltip(a.Tr("shellNavLibrary")).TextColor(p.secondary)
				if m.Route == "library" || b.Hovered() {
					b.TextColor(p.text)
				}
				b.Children(func() {
					ui.Icon(c, nativeIcons[libraryIcon]).Size(24, 24)
					ui.Text(c, a.Tr("shellLibraryTitle")).FontSize(16).FontWeight(700).MaxLines(1).Ellipsis("…")
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
		if m.SignedIn {
			items = m.LibraryItems
		}
		if len(items) == 0 {
			if full {
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

func nativeLibraryPrompt(c *ui.Context, m *UIModel, a ViewActions, p nativePalette) {
	ui.Column(c).Padding(8, 8).Children(func() {
		ui.Column(c).Padding(20).Gap(12).Radius(8).Background(p.raised).Children(func() {
			ui.Text(c, a.Tr("shellLibraryEmptyTitle")).FontSize(16).FontWeight(700)
			hint := "shellLibraryEmptyBody"
			if m.SignedIn {
				hint = "nativeLibraryHint"
			}
			ui.Text(c, a.Tr(hint)).FontSize(14).LineHeight(1.5).TextColor(p.secondary)
			if !m.SignedIn {
				if nativePill(c, a.Tr("shellSignIn"), p, true).AlignSelf(ui.Start).MarginY(8).Clicked() {
					a.Account()
				}
			} else if nativePill(c, a.Tr("shellNavLibrary"), p, true).AlignSelf(ui.Start).MarginY(8).Clicked() {
				a.Navigate("library")
			}
		})
	})
}

func nativeLibraryRow(c *ui.Context, item catalog.Item, m *UIModel, a ViewActions, p nativePalette, full bool) {
	open := item.BrowseID != "" && m.Route == "browse:"+item.BrowseID
	playing := item.ID != "" && item.ID == m.Current.ID
	b := ui.ButtonBase(c).Height(64).Padding(8).Gap(12).Radius(6).Justify(ui.Start).Label(nativeItemLabel(item)).Tooltip(item.Title)
	if !full {
		b.Padding(4).Justify(ui.Center)
	}
	if open {
		b.Selected(true).Background(p.press)
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
			title := ui.Text(c, item.Title).FontSize(15).FontWeight(500).MaxLines(1).Ellipsis("…")
			if playing || open {
				title.TextColor(p.accent)
			}
			ui.Text(c, nativeLibrarySubtitle(item, a)).FontSize(13).TextColor(p.secondary).MaxLines(1).Ellipsis("…")
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

func nativeContent(c *ui.Context, m *UIModel, a ViewActions, p nativePalette, compact bool) {
	inset := float32(24)
	if compact {
		inset = 16
	}
	scroll := ui.Scroll(c.Key("content-" + m.Route + "-" + m.Page.Title)).Grow(1).MinHeight(0).Label(a.Tr("shellContent"))
	contentWidth := scroll.Bounds().W
	a = nativeArtworkViewport(c, a, scroll)
	scroll.Children(func() {
		if strings.HasPrefix(m.Route, "browse:") && m.Page.Title != "" {
			nativeCollectionHeader(c, m, a, p, compact, inset, contentWidth)
		} else if m.Page.Title != "" {
			top := float32(32)
			if compact {
				top = 8
			}
			ui.Column(c).Padding(top, inset, 0).Gap(8).Children(func() {
				size := float32(32)
				if compact {
					size = 26
				}
				ui.Text(c, m.Page.Title).FontSize(size).FontWeight(800).LineHeight(1.15).MaxLines(2).Ellipsis("…").Role(ui.RoleHeading)
				if m.Page.Subtitle != "" {
					ui.Text(c, m.Page.Subtitle).TextColor(p.secondary).FontSize(14)
				}
			})
		}
		ui.Column(c).Padding(20, inset, 32).Gap(32).Children(func() {
			nativePageBody(c, m, a, p, compact, contentWidth)
		})
	})
}

func nativePageBody(c *ui.Context, m *UIModel, a ViewActions, p nativePalette, compact bool, contentWidth float32) {
	if m.Loading {
		ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
			ui.Spinner(c).Size(20, 20).Label(a.Tr("shellLoading"))
			ui.Text(c, a.Tr("shellLoading")).TextColor(p.secondary)
		})
	}
	if m.Error != "" {
		ui.Column(c).Padding(20).Gap(12).Background(p.raised).Radius(8).Children(func() {
			ui.Text(c, m.Error).TextColor(p.secondary)
			if m.ErrorScope != errorAccount && m.ErrorScope != errorAccountConnect && nativePill(c, a.Tr("buttonRetry"), p, true).AlignSelf(ui.Start).Clicked() {
				a.Reload()
			}
		})
	}
	if m.Route == "library" && !m.SignedIn {
		ui.Column(c).Padding(24).Gap(14).Radius(8).Background(p.raised).Children(func() {
			ui.Text(c, a.Tr("shellLibraryEmptyTitle")).FontSize(24).FontWeight(800)
			ui.Text(c, a.Tr("shellLibraryEmptyBody")).TextColor(p.secondary)
			if nativePill(c, a.Tr("shellSignIn"), p, true).AlignSelf(ui.Start).Clicked() {
				a.Account()
			}
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
	for i, shelf := range m.Page.Shelves {
		ui.Column(c.Key(fmt.Sprintf("shelf-%d", i))).Gap(12).Children(func() {
			if shelf.Title != "" {
				size := float32(24)
				if compact {
					size = 20
				}
				ui.Text(c, shelf.Title).FontSize(size).FontWeight(800).MaxLines(1).Ellipsis("…").Role(ui.RoleHeading)
			}
			switch {
			case shelf.Title == "" || (m.Route == "search" && nativeSongShelf(shelf.Items)):
				nativeTrackList(c, shelf.Items, m, a, p, compact, browse)
			case nativeSongShelf(shelf.Items):
				columnWidth := float32(340)
				if compact {
					columnWidth = 300
				}
				shelfScroll := ui.ScrollHorizontal(c).Gap(16).PaddingY(4).Height(4*56 + 8)
				clipped := nativeArtworkViewport(c, a, shelfScroll)
				shelfScroll.Children(func() {
					for start := 0; start < len(shelf.Items); start += 4 {
						ui.Column(c.Key(fmt.Sprintf("songs-%d", start))).Width(columnWidth).Shrink(0).Children(func() {
							for j := start; j < min(start+4, len(shelf.Items)); j++ {
								nativeTrack(c.Key(fmt.Sprintf("song-%d", j)), shelf.Items[j], j, m, clipped, p, nativeTrackRow, true)
							}
						})
					}
				})
			default:
				size := nativeCardArt(compact)
				shelfScroll := ui.ScrollHorizontal(c).PaddingY(4).Height(size + 112).MarginX(-12)
				clipped := nativeArtworkViewport(c, a, shelfScroll)
				shelfScroll.Children(func() {
					for j, item := range shelf.Items {
						nativeCard(c.Key(fmt.Sprintf("card-%d", j)), item, m, clipped, p, compact)
					}
				})
			}
		})
	}
	if len(m.Page.Shelves) == 0 && len(m.Page.Items) > 0 {
		if nativeSongShelf(m.Page.Items) {
			nativeTrackList(c, m.Page.Items, m, a, p, compact, browse)
		} else {
			ui.Row(c).Wrap().MarginX(-12).Children(func() {
				for i, item := range m.Page.Items {
					nativeCard(c.Key(fmt.Sprintf("item-%d", i)), item, m, a, p, compact)
				}
			})
		}
	}
	if m.Page.Continuation != "" {
		if nativePill(c, a.Tr("nativeLoadMore"), p, false).AlignSelf(ui.Center).Disabled(m.Loading || a.LoadMore == nil).Clicked() {
			a.LoadMore()
		}
	}
	if !m.Loading && m.Error == "" && len(m.Page.Items) == 0 && len(m.Page.Shelves) == 0 && !(m.Route == "library" && !m.SignedIn) {
		ui.Column(c).Padding(48, 16).Gap(12).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, nativeIcons["search"]).Size(48, 48).TextColor(p.muted)
			ui.Text(c, a.Tr("nativeEmptyResults")).FontSize(20).FontWeight(700)
			ui.Text(c, a.Tr("shellSearchPlaceholder")).TextColor(p.secondary)
		})
	}
}

// nativeCollectionHeader is the album/playlist/artist hero: cover, display
// title and a primary play button over the page's colour wash.
func nativeCollectionHeader(c *ui.Context, m *UIModel, a ViewActions, p nativePalette, compact bool, inset, contentWidth float32) {
	if contentWidth == 0 {
		contentWidth = 960
	}
	title := func(size float32) {
		ui.Text(c, m.Page.Title).FontSize(size).FontWeight(800).LineHeight(1.08).MaxLines(2).Ellipsis("…").Role(ui.RoleHeading)
		if m.Page.Subtitle != "" {
			ui.Text(c, m.Page.Subtitle).FontSize(14).FontWeight(500).TextColor(p.secondary).MaxLines(2).Ellipsis("…")
		}
	}
	if compact {
		ui.Column(c).Padding(8, inset, 0).Gap(12).Children(func() {
			if m.Page.Artwork != "" {
				nativeArtwork(c, m.Page.Artwork, min(contentWidth-2*inset-64, 224), 6, a, p).AlignSelf(ui.Center).Shadow(0, 8, 32, 0, ui.RGBA(0, 0, 0, 0.45))
			}
			title(26)
		})
	} else {
		art := float32(232)
		if contentWidth < 900 {
			art = 192
		}
		ui.Row(c).Padding(40, inset, 0).Gap(24).AlignItems(ui.End).Children(func() {
			if m.Page.Artwork != "" {
				nativeArtwork(c, m.Page.Artwork, art, 6, a, p).Shadow(0, 8, 32, 0, ui.RGBA(0, 0, 0, 0.45))
			}
			ui.Column(c).Grow(1).MinWidth(0).Gap(12).Children(func() {
				title(max(32, min(72, contentWidth*0.055)))
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
	ui.Row(c).Padding(24, inset, 0).Gap(24).AlignItems(ui.Center).Children(func() {
		icon, key := "play", "shellPlay"
		if inPage && m.Playing {
			icon, key = "pause", "shellPause"
		}
		b := ui.ButtonBase(c).Size(56, 56).Shrink(0).Radius(28).Background(p.accent).TextColor(p.onAccent).Shadow(0, 8, 16, 0, ui.RGBA(0, 0, 0, 0.3)).Label(a.Tr(key) + " · " + m.Page.Title).Tooltip(a.Tr(key)).Disabled(inPage && m.PlaybackLoading)
		if b.Hovered() {
			b.Background(p.accent.Mix(ui.Hex("#ffffff"), 0.12))
		}
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

// nativeShortcutTile is one of Home's quick-access tiles for library items.
func nativeShortcutTile(c *ui.Context, item catalog.Item, m *UIModel, a ViewActions, p nativePalette) {
	playing := item.ID != "" && item.ID == m.Current.ID
	b := ui.ButtonBase(c).Height(64).MinWidth(0).PaddingEnd(12).Gap(12).Radius(6).Justify(ui.Start).Background(p.hover).Clip().Label(item.Title).Tooltip(item.Title)
	hovered := b.Hovered()
	if hovered {
		b.Background(p.press)
	}
	b.Children(func() {
		nativeArtwork(c, item.Artwork, 64, 0, a, p)
		t := ui.Text(c, item.Title).Grow(1).MinWidth(0).FontSize(14).FontWeight(700).MaxLines(2).Ellipsis("…")
		if playing {
			t.TextColor(p.accent)
			ui.Icon(c, nativeIcons["volume"]).Size(16, 16).TextColor(p.accent)
		} else if hovered && item.ID != "" {
			ui.Box(c).Size(32, 32).Shrink(0).Radius(16).Background(p.accent).TextColor(p.onAccent).Center().PassThrough().Children(func() { ui.Icon(c, nativeIcons["play"]).Size(16, 16) })
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

func nativeArtwork(c *ui.Context, src string, size, radius float32, a ViewActions, p nativePalette) ui.Element {
	box := ui.Box(c).Size(size, size).Shrink(0).Radius(radius).Clip().Center()
	var artwork ui.ImageSource
	if a.Artwork != nil && src != "" {
		bounds := box.Bounds()
		if bounds.W == 0 || bounds.H == 0 {
			// Bounds describe the previous layout; resolve a new cover on the
			// next frame, once we know whether its scroll containers reveal it.
			c.Invalidate()
		} else {
			viewport := a.artViewport
			if !a.artClipped {
				viewport.W, viewport.H = c.Size()
			}
			visible := nativeIntersection(bounds, viewport)
			if visible.W > 0 && visible.H > 0 {
				artwork = a.Artwork(src, int(math.Ceil(float64(size)*2)))
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
	if !a.artClipped {
		a.artViewport.W, a.artViewport.H = c.Size()
	}
	a.artViewport = nativeIntersection(a.artViewport, bounds)
	a.artClipped = true
	return a
}

func nativeIntersection(a, b ui.Rect) ui.Rect {
	x, y := max(a.X, b.X), max(a.Y, b.Y)
	return ui.Rect{X: x, Y: y, W: max(float32(0), min(a.X+a.W, b.X+b.W)-x), H: max(float32(0), min(a.Y+a.H, b.Y+b.H)-y)}
}

func nativeCardArt(compact bool) float32 {
	if compact {
		return 140
	}
	return 164
}

// nativeCard is Spotify's shelf card: cover, title and a two-line subtitle,
// with a play button rising over playable covers on hover.
func nativeCard(c *ui.Context, item catalog.Item, m *UIModel, a ViewActions, p nativePalette, compact bool) {
	size := nativeCardArt(compact)
	b := ui.ButtonBase(c).Column().Width(size + 24).Shrink(0).Padding(12).Gap(8).Radius(8).AlignItems(ui.Stretch).Justify(ui.Start).Label(nativeItemLabel(item))
	hovered := b.Hovered()
	if hovered {
		b.Background(p.raised)
	}
	b.Children(func() {
		radius := float32(6)
		if item.Kind == "artist" {
			radius = size / 2
		}
		ui.Box(c).Size(size, size).Shrink(0).Children(func() {
			nativeArtwork(c, item.Artwork, size, radius, a, p).Shadow(0, 8, 24, 0, ui.RGBA(0, 0, 0, 0.3))
			if hovered && item.ID != "" {
				ui.Box(c).Absolute().Right(8).Bottom(8).Size(48, 48).Radius(24).Background(p.accent).TextColor(p.onAccent).Shadow(0, 8, 8, 0, ui.RGBA(0, 0, 0, 0.3)).Center().PassThrough().Children(func() {
					ui.Icon(c, nativeIcons["play"]).Size(22, 22)
				})
			}
		})
		title := ui.Text(c, item.Title).FontSize(15).FontWeight(700).MaxLines(1).Ellipsis("…")
		if item.ID != "" && item.ID == m.Current.ID {
			title.TextColor(p.accent)
		}
		subtitle := item.Artist
		if subtitle == "" {
			subtitle = nativeKindLabel(item, a)
		}
		ui.Text(c, subtitle).FontSize(13).LineHeight(1.4).TextColor(p.secondary).MaxLines(2).Ellipsis("…")
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

func nativeTrackList(c *ui.Context, items []catalog.Item, m *UIModel, a ViewActions, p nativePalette, compact, numbered bool) {
	mode := nativeTrackRow
	if numbered {
		mode = nativeTrackNumbered
	}
	ui.Column(c).Children(func() {
		if numbered && !compact {
			ui.Row(c).Height(36).PaddingX(16).Gap(16).AlignItems(ui.Center).TextColor(p.secondary).Children(func() {
				ui.Text(c, "#").Width(24).Shrink(0).FontSize(14).TextAlign(ui.Center)
				ui.Text(c, a.Tr("shellColumnTitle")).Grow(1).Basis(0).MinWidth(0).FontSize(13).MaxLines(1).Ellipsis("…")
				ui.Text(c, a.Tr("shellColumnAlbum")).Grow(0.75).Basis(0).MinWidth(0).FontSize(13).MaxLines(1).Ellipsis("…")
				ui.Row(c).Width(48).Shrink(0).Justify(ui.End).Label(a.Tr("shellColumnDuration")).Children(func() {
					ui.Icon(c, nativeIcons["clock"]).Size(16, 16)
				})
			})
			ui.Box(c).Height(1).Margin(0, 0, 8).Background(p.line)
		}
		for j, item := range items {
			nativeTrack(c.Key(fmt.Sprintf("list-%d", j)), item, j, m, a, p, mode, compact)
		}
	})
}

func nativeTrack(c *ui.Context, item catalog.Item, index int, m *UIModel, a ViewActions, p nativePalette, mode nativeTrackMode, compact bool) {
	playing := item.ID != "" && item.ID == m.Current.ID
	pad := float32(16)
	if compact {
		pad = 8
	}
	b := ui.ButtonBase(c).Height(56).PaddingX(pad).Gap(16).Radius(6).Justify(ui.Start).Label(nativeItemLabel(item))
	hovered := b.Hovered()
	if playing {
		b.Selected(true).Background(p.hover)
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
					ui.Text(c, fmt.Sprint(index+1)).FontSize(15).TextColor(p.secondary)
				}
			})
		}
		ui.Row(c).Grow(1).Basis(0).MinWidth(0).Gap(12).AlignItems(ui.Center).Children(func() {
			nativeArtwork(c, item.Artwork, 40, 4, a, p)
			ui.Column(c).Grow(1).MinWidth(0).Gap(4).Children(func() {
				t := ui.Text(c, item.Title).FontSize(15).FontWeight(500).MaxLines(1).Ellipsis("…")
				if playing {
					t.TextColor(p.accent)
				}
				artist := ui.Text(c, item.Artist).FontSize(13).TextColor(p.secondary).MaxLines(1).Ellipsis("…")
				if hovered {
					artist.TextColor(p.text)
				}
			})
		})
		if !compact && mode != nativeTrackQueue {
			ui.Text(c, item.Album).Grow(0.75).Basis(0).MinWidth(0).FontSize(13).TextColor(p.secondary).MaxLines(1).Ellipsis("…")
		}
		if playing && mode != nativeTrackNumbered {
			ui.Icon(c, nativeIcons["volume"]).Size(16, 16).TextColor(p.accent)
		}
		if item.Duration > 0 || mode == nativeTrackNumbered {
			duration := ""
			if item.Duration > 0 {
				duration = nativeTime(item.Duration.Seconds())
			}
			ui.Text(c, duration).Width(48).Shrink(0).FontSize(13).TextColor(p.secondary).TextAlign(ui.End)
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
	return ui.Row(c).Background(p.base).Padding(0, 16).Gap(16).AlignItems(ui.Center).Label(a.Tr("shellPlayerControls")).Children(func() {
		ui.Row(c).Grow(1).Basis(0).MinWidth(0).Gap(8).AlignItems(ui.Center).Children(func() {
			info := ui.ButtonBase(c).MinWidth(0).Shrink(1).Height(64).Padding(4).Gap(12).Radius(6).Justify(ui.Start).Label(a.Tr("shellOpenNowPlaying"))
			if info.Hovered() {
				info.Background(p.hover)
			}
			info.Children(func() {
				nativeArtwork(c, m.Current.Artwork, 56, 4, a, p)
				nativeTrackCaption(c, m, a, p)
			})
			if info.Clicked() {
				m.Panel = true
			}
			if m.PlaybackError != "" {
				if nativePill(c, a.Tr("buttonRetry"), p, false).Width(72).PaddingX(6).Shrink(0).Disabled(a.RetryPlayback == nil).Clicked() {
					a.RetryPlayback()
				}
			} else if nativeIconButton(c, nativeHeart(m.Liked), a.Tr(nativeLikeKey(m.Liked)), p, m.Liked, 32).Disabled(m.Current.ID == "").Clicked() {
				a.Like()
			}
		})
		ui.Column(c).Width(min(722, width*0.38)).Shrink(0).Gap(2).AlignItems(ui.Stretch).Children(func() {
			ui.Row(c).Height(40).Gap(16).AlignItems(ui.Center).Justify(ui.Center).Children(func() {
				if nativeIconButton(c, "shuffle", a.Tr("shellShuffle"), p, m.Shuffle, 32).Disabled(len(m.Queue) < 2).Clicked() {
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
				if nativeIconButton(c, icon, a.Tr(key), p, m.Repeat > 0, 32).Clicked() {
					a.Repeat()
				}
			})
			nativeSeek(c, m, a, p)
		})
		ui.Row(c).Grow(1).Basis(0).MinWidth(0).Gap(4).AlignItems(ui.Center).Justify(ui.End).Children(func() {
			if width > 1000 {
				if nativeIconButton(c, "lyrics", a.Tr("shellLyrics"), p, m.Panel && m.PanelTab == "lyrics", 32).Clicked() {
					nativeTogglePanel(m, "lyrics")
				}
			}
			if nativeIconButton(c, "queue", a.Tr("shellQueue"), p, m.Panel && m.PanelTab != "lyrics", 32).Clicked() {
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

// nativeTogglePanel opens Now Playing on a tab, or closes it when that tab is
// already showing, as Spotify's queue and lyrics buttons do.
func nativeTogglePanel(m *UIModel, tab string) {
	current := m.PanelTab
	if current != "lyrics" {
		current = "queue"
	}
	if m.Panel && current == tab {
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
		ui.Text(c, title).FontSize(14).FontWeight(600).MaxLines(1).Ellipsis("…")
		if m.PlaybackError != "" {
			artist = m.PlaybackError
		}
		caption := ui.Text(c, artist).FontSize(12).TextColor(p.secondary).MaxLines(1).Ellipsis("…")
		if m.PlaybackError != "" {
			caption.TextColor(p.accent)
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
	ui.Column(c).Shrink(0).Background(p.base).Padding(6, 8, 0).Children(func() {
		ui.Column(c).Height(60).Radius(8).Background(p.raised).Clip().Children(func() {
			ui.Row(c).Grow(1).Padding(0, 4, 0, 8).Gap(4).AlignItems(ui.Center).Children(func() {
				info := ui.ButtonBase(c).Grow(1).MinWidth(0).Height(52).Gap(10).Justify(ui.Start).Radius(6).Label(a.Tr("shellOpenNowPlaying")).Children(func() {
					nativeArtwork(c, m.Current.Artwork, 40, 4, a, p)
					nativeTrackCaption(c, m, a, p)
				})
				if info.Clicked() {
					m.Panel = true
				}
				if m.PlaybackError != "" {
					if nativePill(c, a.Tr("buttonRetry"), p, false).Width(64).PaddingX(6).Shrink(0).Disabled(a.RetryPlayback == nil).Clicked() {
						a.RetryPlayback()
					}
				} else if nativeIconButton(c, nativeHeart(m.Liked), a.Tr(nativeLikeKey(m.Liked)), p, m.Liked, 40).Disabled(m.Current.ID == "").Clicked() {
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
	if b.Hovered() {
		b.Background(p.text.Alpha(0.86))
	}
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
		ui.Text(c, nativeTime(state.value)).Width(40).TextAlign(ui.End).FontSize(12).TextColor(p.secondary)
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
		ui.Text(c, nativeTime(m.Duration)).Width(40).FontSize(12).TextColor(p.secondary)
	})
}

func nativeSlider(c *ui.Context, value *float64, maximum float64, p nativePalette) ui.Element {
	s := ui.SliderBase(c, value, 0, maximum).Height(24).PaddingX(5)
	hovered := s.Hovered() || s.Focused() || s.Pressed()
	fraction := float32(max(0, min(1, *value/maximum)))
	s.Draw(func(painter *ui.Painter, r ui.Rect) {
		track := ui.Rect{X: r.X + 5, Y: r.Y + r.H/2 - 2, W: max(0, r.W-10), H: 4}
		painter.Fill(track, p.text.Alpha(0.3), 2)
		fill := track
		fill.W *= fraction
		col := p.text
		if hovered {
			col = p.accent
		}
		painter.Fill(fill, col, 2)
		if hovered {
			painter.Fill(ui.Rect{X: track.X + fill.W - 6, Y: r.Y + r.H/2 - 6, W: 12, H: 12}, p.text, 6)
		}
	})
	return s
}

func nativeProgressLine(c *ui.Context, m *UIModel, p nativePalette) {
	value := float32(0)
	if m.Duration > 0 {
		value = float32(max(0, min(1, m.Position/m.Duration)))
	}
	ui.Box(c).Height(2).Shrink(0).Margin(0, 8, 0).Radius(1).Background(p.text.Alpha(0.2)).Children(func() {
		ui.Box(c).Height(2).Radius(1).WidthPercent(value * 100).Background(p.text)
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
	}
}

func nativeTabbar(c *ui.Context, m *UIModel, a ViewActions, p nativePalette) {
	ui.Row(c).Height(nativeCompactTabbar).Shrink(0).Padding(4, 8).Background(p.base).Label(a.Tr("shellNavMain")).Children(func() {
		for _, nav := range []struct{ route, key, icon string }{{"home", "shellNavHome", "home"}, {"search", "shellNavSearch", "search"}, {"explore", "shellNavExplore", "explore"}, {"library", "shellNavLibrary", "library"}} {
			active := m.Route == nav.route
			b := ui.ButtonBase(c.Key("tab-" + nav.route)).Column().Grow(1).Gap(4).Radius(8).Label(a.Tr(nav.key)).Selected(active).Background(ui.Transparent).TextColor(p.secondary)
			if active || b.Hovered() {
				b.TextColor(p.text)
			}
			b.Children(func() {
				icon := nav.icon
				if active && icon != "search" {
					icon += "-fill"
				}
				ui.Icon(c, nativeIcons[icon]).Size(24, 24)
				ui.Text(c, a.Tr(nav.key)).FontSize(11).FontWeight(600).MaxLines(1).Ellipsis("…")
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
func nativeNowPlaying(c *ui.Context, m *UIModel, a ViewActions, p nativePalette, width float32, overlay bool) ui.Element {
	_, windowHeight := c.Size()
	gap := float32(16)
	if windowHeight < 760 {
		gap = 12
	}
	return ui.Column(c).Background(p.surface).Radius(8).Padding(16).Gap(gap).MinHeight(0).Children(func() {
		ui.Row(c).Height(32).Shrink(0).Gap(4).AlignItems(ui.Center).Children(func() {
			ui.Text(c, a.Tr("shellNowPlaying")).FontSize(16).FontWeight(700).Grow(1).MinWidth(0).MaxLines(1).Ellipsis("…")
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
				ui.Text(c, a.Tr("shellEmptyTitle")).FontSize(20).FontWeight(800)
				ui.Text(c, a.TrArgs("shellEmptyBody", map[string]any{"shortcut": "⌘ K"})).TextColor(p.secondary).TextAlign(ui.Center)
			})
			return
		}
		body := func() {
			nativePlaybackNotice(c, m, a, p, false)
			artSize := min(width-32, max(float32(120), windowHeight-560))
			if overlay {
				artSize = min(artSize, max(float32(120), windowHeight-600))
			}
			nativeMedia(c, m, a, p, artSize).AlignSelf(ui.Center)
			nativeMediaSwitch(c, m, a, p)
			ui.Row(c).Shrink(0).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Column(c).Grow(1).MinWidth(0).Gap(4).Children(func() {
					ui.Text(c, m.Current.Title).FontSize(22).FontWeight(800).LineHeight(1.2).MaxLines(2).Ellipsis("…")
					ui.Text(c, m.Current.Artist).FontSize(15).TextColor(p.secondary).MaxLines(1).Ellipsis("…")
				})
				if nativeIconButton(c, nativeHeart(m.Liked), a.Tr(nativeLikeKey(m.Liked)), p, m.Liked, 36).Clicked() {
					a.Like()
				}
			})
			if overlay {
				ui.Row(c).Shrink(0).Gap(18).AlignItems(ui.Center).Justify(ui.Center).Children(func() {
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
			card := nativePanelCard(c, m, a, p)
			if overlay {
				card.Height(300).Shrink(0)
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
	})
}

// nativePanelCard holds the queue and lyrics behind a pair of chips.
func nativePanelCard(c *ui.Context, m *UIModel, a ViewActions, p nativePalette) ui.Element {
	return ui.Column(c).MinHeight(0).Padding(12).Gap(12).Radius(8).Background(p.raised).Children(func() {
		nativePanelTabs(c, m, a, p)
		nativePanelBody(c, m, a, p)
	})
}

func nativePanelTabs(c *ui.Context, m *UIModel, a ViewActions, p nativePalette) {
	ui.Row(c).Shrink(0).Gap(8).Children(func() {
		for _, tab := range []struct{ id, key string }{{"queue", "shellQueue"}, {"lyrics", "shellLyrics"}} {
			active := m.PanelTab == tab.id || (m.PanelTab == "" && tab.id == "queue")
			if nativeChip(c, a.Tr(tab.key), p, active).Label(a.Tr(tab.key) + " · " + a.Tr("shellNowPlaying")).Clicked() {
				m.PanelTab = tab.id
			}
		}
	})
}

func nativePanelBody(c *ui.Context, m *UIModel, a ViewActions, p nativePalette) ui.Element {
	if m.PanelTab == "lyrics" {
		return nativeLyricsBody(c, m, a, p)
	}
	scroll := ui.Scroll(c.Key("panel-" + m.PanelTab)).Grow(1).MinHeight(68)
	a = nativeArtworkViewport(c, a, scroll)
	return scroll.Children(func() {
		for i, item := range m.Queue {
			nativeTrack(c.Key(fmt.Sprintf("queue-%d", i)), item, i, m, a, p, nativeTrackQueue, true)
		}
	})
}

func nativeImmersive(c *ui.Context, m *UIModel, a ViewActions, p nativePalette, width, height float32) {
	compact := width <= 760
	ui.Row(c).Height(64).Shrink(0).Padding(12, 20).AlignItems(ui.Center).Children(func() {
		if nativeIconButton(c, "chevron-down", a.Tr("shellExitImmersive"), p, false, 40).Clicked() {
			m.Immersive = false
		}
		ui.Spacer(c)
		ui.Text(c, a.Tr("shellNowPlaying")).FontSize(12).FontWeight(700).LetterSpacing(1).TextColor(p.secondary)
		ui.Spacer(c)
		if nativeIconButton(c, "settings", a.Tr("shellSettings"), p, false, 40).Clicked() {
			m.Settings = true
		}
	})
	if compact {
		scroll := ui.Scroll(c).Grow(1).Padding(20).Gap(24)
		a = nativeArtworkViewport(c, a, scroll)
		scroll.Children(func() {
			nativePlaybackNotice(c, m, a, p, false)
			artSize := min(width-64, max(float32(180), height-390))
			nativeMedia(c, m, a, p, artSize).AlignSelf(ui.Center)
			nativeMediaSwitch(c, m, a, p)
			nativeImmersiveTitle(c, m, a, p, true)
			nativePanelCard(c, m, a, p).Height(280)
		})
		return
	}
	panelWidth := min(float32(380), width*0.32)
	contentHeight := max(float32(0), height-64-48)
	mediaWidth := max(float32(1), width-16-80-40-panelWidth)
	ui.Row(c).Grow(1).MinHeight(0).Padding(24, 40).Gap(40).AlignItems(ui.Center).Children(func() {
		scroll := ui.Scroll(c).Grow(1).MinWidth(0).MinHeight(0).HeightPercent(100).Label(a.Tr("shellContent"))
		a = nativeArtworkViewport(c, a, scroll)
		scroll.Children(func() {
			ui.Column(c).MinHeight(contentHeight).Shrink(0).Gap(24).AlignItems(ui.Center).Justify(ui.Center).Children(func() {
				// Reserve the switch, both gaps, two title lines and an artist
				// line. Extra artist lines/notices remain reachable by scrolling.
				textHeight := float32(32 + 48 + 88 + 8 + 22)
				if m.PlaybackError != "" {
					textHeight += 24 + 56
				}
				mediaHeight := max(float32(180), contentHeight-textHeight)
				artSize := min(mediaWidth, mediaHeight)
				if m.Video {
					// A landscape video can use the same vertical budget as the
					// square cover while filling more of the available column.
					artSize = min(mediaWidth, mediaHeight*16/9)
				}
				nativeMedia(c, m, a, p, artSize).Shadow(0, 24, 64, 0, ui.RGBA(0, 0, 0, 0.5))
				nativeMediaSwitch(c, m, a, p)
				nativeImmersiveTitle(c, m, a, p, false)
				nativePlaybackNotice(c, m, a, p, false)
			})
		})
		nativePanelCard(c, m, a, p).Width(panelWidth).HeightPercent(90).Background(p.base.Alpha(0.35))
	})
}

func nativeImmersiveTitle(c *ui.Context, m *UIModel, a ViewActions, p nativePalette, compact bool) {
	ui.Column(c).Gap(8).AlignItems(ui.Center).Children(func() {
		title, artist := m.Current.Title, m.Current.Artist
		if title == "" {
			title, artist = a.Tr("shellEmptyTitle"), a.Tr("shellIdleSubtitle")
		}
		size := float32(40)
		if compact {
			size = 28
		}
		ui.Text(c, title).FontSize(size).FontWeight(800).LineHeight(1.1).TextAlign(ui.Center).MaxLines(2).Ellipsis("…")
		ui.Text(c, artist).FontSize(17).FontWeight(500).TextColor(p.secondary).TextAlign(ui.Center)
	})
}

func nativeSettings(c *ui.Context, m *UIModel, a ViewActions, p nativePalette, width, height float32) {
	ui.DialogBase(c, &m.Settings, func(backdrop, panel ui.Element) {
		backdrop.Background(ui.RGBA(0, 0, 0, 0.5))
		panel.Width(min(420, width-24)).MaxHeight(height-32).Padding(24).Gap(20).Radius(12).Background(p.raised).Border(1, p.line).Shadow(0, 16, 48, 0, ui.RGBA(0, 0, 0, 0.5)).Children(func() {
			ui.Row(c).AlignItems(ui.Center).Children(func() {
				ui.Text(c, a.Tr("shellSettings")).FontSize(24).FontWeight(800).Grow(1)
				if nativeIconButton(c, "close", a.Tr("shellClose"), p, false, 32).Clicked() {
					m.Settings = false
				}
			})
			ui.Scroll(c).MinHeight(0).Gap(24).Children(func() {
				section := func(key string) {
					ui.Text(c, a.Tr(key)).FontSize(16).FontWeight(700)
				}
				ui.Column(c).Gap(12).Children(func() {
					section("menuAccount")
					ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
						nativeAvatar(c, m, p, 40)
						ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
							ui.Text(c, nativeAccountStatus(m, a)).FontWeight(600).MaxLines(1).Ellipsis("…")
							if m.SignedIn && m.MusicPremium {
								ui.Text(c, a.Tr("shellPremiumBadge")).FontSize(12).FontWeight(700).TextColor(p.accent)
							}
						})
					})
					ui.Row(c).Gap(8).Wrap().Children(func() {
						key := "shellAccountSignIn"
						if m.SignedIn {
							key = "shellAccountManage"
						}
						if nativePill(c, a.Tr(key), p, !m.SignedIn).Clicked() {
							m.Settings = false
							a.Account()
						}
						if m.SignedIn {
							if nativePill(c, a.Tr("shellSignOut"), p, false).TextColor(p.accent).Clicked() {
								m.Settings = false
								a.SignOut()
							}
						}
					})
				})
				ui.Column(c).Gap(12).Children(func() {
					section("shellAppearance")
					ui.Row(c).Gap(8).Children(func() {
						for _, option := range []struct{ id, key string }{{"system", "shellThemeSystem"}, {"dark", "shellThemeDark"}, {"light", "shellThemeLight"}} {
							if nativeChip(c, a.Tr(option.key), p, m.Appearance == option.id).Grow(1).PaddingX(8).Clicked() {
								a.Appearance(option.id)
							}
						}
					})
				})
				ui.Column(c).Gap(4).Children(func() {
					ui.Box(c).Height(8)
					section("shellLanguage")
					for _, option := range []struct{ id, title string }{{"system", a.Tr("shellLanguageSystem")}, {"en", "English"}, {"zh-Hans", "简体中文"}, {"zh-Hant", "繁體中文"}, {"ja", "日本語"}} {
						active := m.Language == option.id
						b := ui.Button(c, option.title).Height(40).Justify(ui.Start).PaddingX(12).Radius(6).BorderWidth(0).Background(ui.Transparent).TextColor(p.secondary).Selected(active)
						if active {
							b.Background(p.hover).TextColor(p.accent).FontWeight(700)
						} else if b.Hovered() {
							b.Background(p.hover).TextColor(p.text)
						}
						if b.Clicked() {
							a.Language(option.id)
						}
					}
				})
				ui.Box(c).Height(1).Background(p.line)
				if nativePill(c, a.Tr("shellShortcuts"), p, false).AlignSelf(ui.Start).Clicked() {
					m.Settings = false
					m.Shortcuts = true
				}
				ui.Text(c, a.Tr("shellFooter")).FontSize(11).LineHeight(1.5).TextColor(p.muted)
			})
		})
	}).Label(a.Tr("shellSettings"))
}

func nativeShortcuts(c *ui.Context, m *UIModel, a ViewActions, p nativePalette, width, height float32) {
	ui.DialogBase(c, &m.Shortcuts, func(backdrop, panel ui.Element) {
		backdrop.Background(ui.RGBA(0, 0, 0, 0.5))
		panel.Width(min(480, width-24)).MaxHeight(height-32).Padding(24).Gap(20).Radius(12).Background(p.raised).Border(1, p.line).Shadow(0, 16, 48, 0, ui.RGBA(0, 0, 0, 0.5)).Children(func() {
			ui.Row(c).AlignItems(ui.Center).Children(func() {
				ui.Text(c, a.Tr("shellShortcuts")).FontSize(24).FontWeight(800).Grow(1)
				if nativeIconButton(c, "close", a.Tr("shellClose"), p, false, 32).Clicked() {
					m.Shortcuts = false
				}
			})
			ui.Scroll(c).Gap(14).Children(func() {
				for _, shortcut := range []struct{ key, chord string }{{"shellShortcutPlayPause", "Space"}, {"shellShortcutNextPrevious", "⌘ → / ←"}, {"shellShortcutVolume", "⌘ ↑ / ↓"}, {"shellShortcutSearch", "⌘ K"}, {"shellShortcutPanel", "⌘ J"}, {"shellShortcutImmersive", "⌘ ⇧ F"}, {"shellShortcutHome", "⌘ ⇧ H"}, {"shellShortcutLibrary", "⌘ L"}, {"shellShortcutMute", "⌘ ⇧ M"}, {"shellShortcutSidebar", "⌘ B"}, {"shellShortcutDismiss", "Esc"}} {
					ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
						ui.Text(c, a.Tr(shortcut.key)).Grow(1)
						ui.Text(c, shortcut.chord).FontSize(12).FontWeight(600).Padding(5, 8).Radius(4).Background(p.highlight).TextColor(p.secondary)
					})
				}
			})
		})
	}).Label(a.Tr("shellShortcuts"))
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
	texts                              [13]string
	flags                              [18]bool
	values                             [4]float64
	repeat, queueLength, libraryLength int
	errorScope                         errorScope
}

func nativePresentation(m *UIModel) nativePresentationKey {
	return nativePresentationKey{
		texts:  [13]string{m.Route, m.Query, m.PanelTab, m.Appearance, m.Language, m.Current.ID, m.Current.Title, m.Page.Title, m.Error, m.Lyrics, m.Current.Artwork, m.VideoError, m.PlaybackError},
		flags:  [18]bool{m.Loading, m.Playing, m.Muted, m.Liked, m.Shuffle, m.Panel, m.Immersive, m.Settings, m.Shortcuts, m.SidebarCollapsed, m.SignedIn, m.CanBack, m.CanForward, m.FocusSearch, m.Video, m.VideoLoading, m.PlaybackLoading, m.MusicPremium},
		values: [4]float64{m.Volume, m.Position, m.Duration, 0}, repeat: m.Repeat, queueLength: len(m.Queue), libraryLength: len(m.LibraryItems),
		errorScope: m.ErrorScope,
	}
}

// nativeBackdrop washes the top of the page in colour: the blurred cover of a
// collection, the current track's cover in immersive mode, or a stable tint
// for pages without artwork.
func nativeBackdrop(c *ui.Context, m *UIModel, a ViewActions, p nativePalette) {
	src := m.Page.Artwork
	if m.Immersive {
		src = m.Current.Artwork
	}
	var artwork ui.ImageSource
	if src != "" && a.Artwork != nil {
		artwork = a.Artwork(src, artworkEdge)
	}
	strength := float32(0.5)
	if !p.dark {
		strength = 0.32
	}
	if artwork == nil {
		if m.Immersive {
			return
		}
		ui.Box(c).Absolute().Left(0).Top(0).WidthPercent(100).Height(320).LinearGradient(ui.LinearGradient{From: nativeHeaderTint(m.Route).Alpha(strength), To: nativeHeaderTint(m.Route).Alpha(0), Angle: 180}).PassThrough()
		return
	}
	// Blur a small layer, then draw it at the final size without a transform.
	// A transformed child makes MyGo allocate a full-size RGBA mask for the
	// rounded parent clip, which would erase the smaller canvas's savings.
	layer := ui.Box(c).Absolute().Left(0).Top(0).WidthPercent(25).Effect(backdropLayer{}).PassThrough()
	if m.Immersive {
		layer.HeightPercent(25).Opacity(strength * 0.7)
	} else {
		layer.Height(80).Opacity(strength)
	}
	layer.Children(func() { ui.Image(c, artwork).Fill().Fit(ui.Cover).Blur(17.5).PassThrough() })
	if !m.Immersive {
		// The blur spills below its 320-DIP band; fade it into the panel.
		ui.Box(c).Absolute().Left(0).Top(0).WidthPercent(100).Height(560).LinearGradient(ui.LinearGradient{From: p.surface.Alpha(0), To: p.surface, Angle: 180, Start: 0.2, End: 0.7}).PassThrough()
	}
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
	ui.Row(c).Shrink(0).Gap(8).AlignSelf(ui.Center).Children(func() {
		for _, option := range []struct {
			video bool
			key   string
		}{{false, "nativeArtwork"}, {true, "nativeVideo"}} {
			b := nativeChip(c, a.Tr(option.key), p, m.Video == option.video).MinWidth(80).Disabled(m.Current.ID == "" || a.Video == nil)
			if b.Clicked() {
				a.Video(option.video)
			}
		}
	})
}

func nativeMedia(c *ui.Context, m *UIModel, a ViewActions, p nativePalette, size float32) ui.Element {
	if !m.Video {
		return nativeArtwork(c, m.Current.Artwork, size, 8, a, p)
	}
	return ui.Box(c).Size(size, size*9/16).Shrink(0).Radius(8).Background(ui.Hex("#000000")).Clip().Center().Label(a.Tr("nativeVideo")).Children(func() {
		if m.VideoError != "" {
			ui.Column(c).WidthPercent(100).Padding(8).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Text(c, m.VideoError).TextAlign(ui.Center).TextColor(ui.Hex("#b3b3b3")).FontSize(13).MaxLines(2).Ellipsis("…")
				if nativePill(c, a.Tr("buttonRetry"), p, false).Disabled(a.Video == nil || m.Current.ID == "").Clicked() {
					a.Video(true)
				}
			})
		} else if m.VideoLoading {
			ui.Spinner(c).Size(28, 28).Label(a.Tr("shellLoading"))
		} else if m.VideoFrame != nil {
			ui.Image(c, m.VideoFrame).Size(size, size*9/16).Fit(ui.Contain)
		} else {
			ui.Text(c, a.Tr("nativeVideoUnavailable")).Padding(16).TextAlign(ui.Center).TextColor(ui.Hex("#b3b3b3")).FontSize(13)
		}
	})
}

func nativeExpandedTransport(c *ui.Context, m *UIModel, a ViewActions, p nativePalette) ui.Element {
	return ui.Column(c).Padding(6, 16).Gap(4).Background(p.base).Children(func() {
		if m.PlaybackError != "" {
			nativePlaybackNotice(c, m, a, p, true)
		} else {
			nativeSeek(c, m, a, p)
		}
		ui.Row(c).Height(56).Gap(16).AlignItems(ui.Center).Justify(ui.Center).Children(func() {
			if nativeIconButton(c, "shuffle", a.Tr("shellShuffle"), p, m.Shuffle, 44).Disabled(len(m.Queue) < 2).Clicked() {
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
			if nativeIconButton(c, icon, a.Tr(key), p, m.Repeat > 0, 44).Clicked() {
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

func nativePlaybackNotice(c *ui.Context, m *UIModel, a ViewActions, p nativePalette, footer bool) {
	if m.PlaybackError == "" {
		return
	}
	row := ui.Row(c).Shrink(0).Gap(8).Padding(10).Radius(8).Background(p.raised).AlignItems(ui.Center)
	if footer {
		row.Height(24).Padding(0).Background(ui.Transparent)
	}
	row.Children(func() {
		text := ui.Text(c, m.PlaybackError).Grow(1).MinWidth(0).FontSize(12).TextColor(p.accent)
		if footer {
			text.MaxLines(1).Ellipsis("…").FontSize(11)
		}
		button := nativePill(c, a.Tr("buttonRetry"), p, false).Width(72).PaddingX(6).Disabled(a.RetryPlayback == nil)
		if footer {
			button.Height(24).MinHeight(24).PaddingY(0)
		}
		if button.Clicked() {
			a.RetryPlayback()
		}
	})
}
