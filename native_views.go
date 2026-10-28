package main

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"ytmgo/internal/catalog"
)

// UIModel is owned by the UI thread. Network and player completions are applied
// by the controller before a frame; the view changes only local presentation.
type UIModel struct {
	Page                                                        catalog.Page
	Current                                                     catalog.Item
	LibraryItems                                                []catalog.Item
	Queue                                                       []catalog.Item
	Route, Query, Error, Lyrics, PanelTab, Appearance, Language string
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
	compact := width <= 760
	gutter, playerHeight := float32(8), float32(84)
	if compact {
		gutter, playerHeight = 0, 64
		if m.Immersive {
			playerHeight = 140
		}
	}
	ui.Column(c).Fill().Background(p.base).TextColor(p.text).Gap(gutter).Children(func() {
		ui.Row(c).Grow(1).MinHeight(0).AlignItems(ui.Stretch).Gap(gutter).Padding(gutter, gutter, 0).Children(func() {
			if !compact && !m.Immersive {
				nativeSidebar(c, m, a, p, width)
			}
			ui.Column(c).Grow(1).MinWidth(0).MinHeight(0).Radius(12).Background(p.surface).Clip().Children(func() {
				nativeBackdrop(c, m, a)
				if m.Immersive {
					availableHeight := height - playerHeight
					if compact {
						availableHeight -= 64
					}
					nativeImmersive(c, m, a, p, width, availableHeight)
				} else {
					nativeTopbar(c, m, a, p, compact)
					nativeContent(c, m, a, p, compact)
				}
			})
			if m.Panel && !m.Immersive && width > 1180 {
				nativeNowPlaying(c, m, a, p, 360, false).Width(360).Shrink(0)
			}
		})
		nativeTransport(c, m, a, p, compact, width).Height(playerHeight).Shrink(0)
		if compact {
			nativeTabbar(c, m, a, p)
		}
	})
	if m.Panel && !m.Immersive && width <= 1180 {
		ui.DialogBase(c, &m.Panel, func(backdrop, panel ui.Element) {
			backdrop.Background(ui.RGBA(0, 0, 0, 0.35))
			panel.Width(min(width-16, 380)).Height(min(height-100, 760)).Radius(16).Background(p.surface).Clip().Children(func() { nativeNowPlaying(c, m, a, p, min(width-16, 380), true).Fill() })
		}).Label(a.Tr("shellNowPlaying"))
	}
	nativeSettings(c, m, a, p, width, height)
	nativeShortcuts(c, m, a, p, width, height)
}

func nativeIconButton(c *ui.Context, name, label string, p nativePalette, active bool, hit float32) ui.Element {
	b := ui.ButtonBase(c).Size(hit, hit).Shrink(0).Radius(hit / 2).Label(label).Tooltip(label).TextColor(p.secondary)
	if active {
		b.Selected(true).Background(p.highlight).TextColor(p.accent)
	} else if b.Hovered() {
		b.Background(p.raised).TextColor(p.text)
	}
	b.Children(func() {
		ui.Icon(c, nativeIcons[name]).Size(22, 22)
		if active {
			ui.Box(c).Absolute().Left((hit-3)/2).Bottom(2).Size(3, 3).Radius(2).Background(p.accent).PassThrough()
		}
	})
	return b
}

func nativePill(c *ui.Context, label string, p nativePalette, selected bool) ui.Element {
	b := ui.Button(c, label).MinHeight(36).Padding(7, 18).Radius(24).BorderWidth(0).FontWeight(600).Background(p.input).TextColor(p.text)
	if selected {
		b.Selected(true).Background(p.text).TextColor(p.base)
	}
	return b
}

func nativeSidebar(c *ui.Context, m *UIModel, a ViewActions, p nativePalette, width float32) {
	full := width > 1180 && !m.SidebarCollapsed
	sidebarWidth := float32(72)
	if full {
		sidebarWidth = 280
	}
	ui.Column(c).Width(sidebarWidth).Shrink(0).Gap(8).Children(func() {
		ui.Column(c).Background(p.surface).Radius(12).Padding(16, 12).Gap(6).Children(func() {
			ui.Row(c).Height(44).Gap(12).PaddingX(8).AlignItems(ui.Center).Children(func() {
				brand := ui.ButtonBase(c).Size(32, 32).Radius(10).Background(p.accent).TextColor(p.onAccent).Label(a.Tr("shellBrandHome")).Children(func() { ui.Icon(c, nativeIcons["play"]).Size(23, 23) })
				if brand.Clicked() {
					a.Navigate("home")
				}
				if full {
					ui.Text(c, "ytmgo").FontSize(22).FontWeight(800).Grow(1)
					if nativeIconButton(c, "sidebar", a.Tr("shellSidebarCollapse"), p, false, 32).Clicked() {
						m.SidebarCollapsed = true
					}
				}
			})
			for _, nav := range []struct{ route, key, icon string }{{"home", "shellNavHome", "home"}, {"explore", "shellNavExplore", "explore"}, {"library", "shellNavLibrary", "library"}} {
				active := m.Route == nav.route
				b := ui.ButtonBase(c.Key("nav-" + nav.route)).Height(44).PaddingX(10).Gap(16).Justify(ui.Start).Radius(8).Label(a.Tr(nav.key)).Tooltip(a.Tr(nav.key)).TextColor(p.secondary)
				if active {
					b.Selected(true).Background(ui.Transparent).TextColor(p.text)
				}
				if b.Hovered() {
					b.Background(p.raised)
				}
				b.Children(func() {
					icon := nav.icon
					if active {
						icon += "-fill"
					}
					ui.Icon(c, nativeIcons[icon]).Size(24, 24)
					if full {
						ui.Text(c, a.Tr(nav.key)).FontWeight(600)
					}
				})
				if b.Clicked() {
					a.Navigate(nav.route)
				}
			}
		})
		ui.Column(c).Grow(1).MinHeight(0).Background(p.surface).Radius(12).Padding(16, 12).Gap(20).Children(func() {
			ui.Row(c).Height(32).PaddingX(8).Gap(16).AlignItems(ui.Center).TextColor(p.secondary).Children(func() {
				if !full {
					if width > 1180 {
						if nativeIconButton(c, "sidebar", a.Tr("shellSidebarExpand"), p, false, 32).Clicked() {
							m.SidebarCollapsed = false
						}
					} else {
						ui.Icon(c, nativeIcons["library"]).Size(24, 24)
					}
				} else {
					ui.Icon(c, nativeIcons["library"]).Size(24, 24)
					ui.Text(c, a.Tr("shellLibraryTitle")).FontWeight(600)
				}
			})
			if m.SignedIn && len(m.LibraryItems) > 0 {
				scroll := ui.Scroll(c).Grow(1).MinHeight(0).Gap(4).Label(a.Tr("shellLibraryRegion"))
				clipped := nativeArtworkViewport(c, a, scroll)
				scroll.Children(func() {
					for i, item := range m.LibraryItems {
						b := ui.ButtonBase(c.Key(fmt.Sprintf("library-%d", i))).Height(56).Padding(4).Gap(10).Radius(8).Justify(ui.Start).Label(item.Title + " · " + item.Artist).Tooltip(item.Title)
						if b.Hovered() {
							b.Background(p.raised)
						}
						b.Children(func() {
							nativeArtwork(c, item.Artwork, 40, 4, clipped, p)
							if full {
								ui.Column(c).Grow(1).MinWidth(0).Gap(4).Children(func() {
									ui.Text(c, item.Title).FontWeight(600).MaxLines(1).Ellipsis("…")
									ui.Text(c, item.Artist).FontSize(12).TextColor(p.secondary).MaxLines(1).Ellipsis("…")
								})
							}
						})
						if b.Clicked() {
							if item.ID != "" {
								a.Play(item)
							} else {
								a.Open(item)
							}
						}
					}
				})
			} else if full {
				ui.Column(c).Padding(16).Gap(12).Radius(8).Background(p.raised).Children(func() {
					ui.Text(c, a.Tr("shellLibraryEmptyTitle")).FontWeight(650)
					hint := "shellLibraryEmptyBody"
					if m.SignedIn {
						hint = "nativeLibraryHint"
					}
					ui.Text(c, a.Tr(hint)).FontSize(13).TextColor(p.secondary).LineHeight(1.6)
					if !m.SignedIn {
						if nativePill(c, a.Tr("shellSignIn"), p, true).AlignSelf(ui.Start).Clicked() {
							a.Account()
						}
					} else {
						if nativePill(c, a.Tr("shellNavLibrary"), p, false).AlignSelf(ui.Start).Clicked() {
							a.Navigate("library")
						}
					}
				})
			}
		})
	})
}

func nativeTopbar(c *ui.Context, m *UIModel, a ViewActions, p nativePalette, compact bool) {
	h, hit, pad := float32(64), float32(32), float32(16)
	if compact {
		h, hit, pad = 56, 44, 8
	}
	ui.Row(c).Height(h).Shrink(0).Padding(8, pad).Gap(8).AlignItems(ui.Center).Children(func() {
		if nativeIconButton(c, "back", a.Tr("shellBack"), p, false, hit).Disabled(!m.CanBack).Clicked() {
			a.Back()
		}
		if !compact {
			if nativeIconButton(c, "forward", a.Tr("shellForward"), p, false, hit).Disabled(!m.CanForward).Clicked() {
				a.Forward()
			}
		}
		ui.Row(c).Grow(1).MinWidth(0).MaxWidth(560).Height(40).PaddingX(12).Gap(10).Radius(22).Background(p.input).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, nativeIcons["search"]).Size(20, 20).TextColor(p.secondary)
			input := ui.TextInput(c.Key("search-input"), &m.Query).Grow(1).MinWidth(0).Height(38).Padding(0).BorderWidth(0).Background(ui.Transparent).Placeholder(a.Tr("shellSearchPlaceholder")).Label(a.Tr("shellNavSearch")).FontSize(15)
			if m.FocusSearch {
				input.Focus()
				m.FocusSearch = false
			}
			if input.Submitted() && strings.TrimSpace(m.Query) != "" {
				a.Search(m.Query)
			}
		})
		if !compact {
			ui.Spacer(c)
			if m.SignedIn {
				if nativeIconButton(c, "user", a.Tr("shellAccountManage"), p, false, 36).Clicked() {
					a.Account()
				}
			} else {
				if nativePill(c, a.Tr("shellSignIn"), p, true).Clicked() {
					a.Account()
				}
			}
		}
		if nativeIconButton(c, "settings", a.Tr("shellSettings"), p, m.Settings, hit).Clicked() {
			m.Settings = !m.Settings
		}
	})
}

func nativeContent(c *ui.Context, m *UIModel, a ViewActions, p nativePalette, compact bool) {
	inset := float32(32)
	if compact {
		inset = 16
	}
	scroll := ui.Scroll(c.Key("content-" + m.Route + "-" + m.Page.Title)).Grow(1).MinHeight(0).Padding(inset).Gap(28).Label(a.Tr("shellContent"))
	a = nativeArtworkViewport(c, a, scroll)
	scroll.Children(func() {
		if m.Loading {
			ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
				ui.Spinner(c).Size(20, 20).Label(a.Tr("shellLoading"))
				ui.Text(c, a.Tr("shellLoading")).TextColor(p.secondary)
			})
		}
		if m.Error != "" {
			ui.Column(c).Padding(20).Gap(12).Background(p.raised).Radius(12).Children(func() {
				ui.Text(c, m.Error).TextColor(p.secondary)
				if nativePill(c, a.Tr("buttonRetry"), p, false).AlignSelf(ui.Start).Clicked() {
					a.Reload()
				}
			})
		}
		if m.Page.Title != "" {
			ui.Column(c).Gap(8).Children(func() {
				ui.Text(c, m.Page.Title).FontSize(32).FontWeight(760).LineHeight(1.2).Role(ui.RoleHeading)
				if m.Page.Subtitle != "" {
					ui.Text(c, m.Page.Subtitle).TextColor(p.secondary).FontSize(14)
				}
			})
		}
		if m.Route == "library" && !m.SignedIn {
			ui.Column(c).Padding(24).Gap(14).Radius(12).Background(p.raised).Children(func() {
				ui.Text(c, a.Tr("shellLibraryEmptyTitle")).FontSize(24).FontWeight(700)
				ui.Text(c, a.Tr("shellLibraryEmptyBody")).TextColor(p.secondary)
				if nativePill(c, a.Tr("shellSignIn"), p, true).AlignSelf(ui.Start).Clicked() {
					a.Account()
				}
			})
		}
		for i, shelf := range m.Page.Shelves {
			ui.Column(c.Key(fmt.Sprintf("shelf-%d", i))).Gap(16).Children(func() {
				if shelf.Title != "" {
					ui.Text(c, shelf.Title).FontSize(24).FontWeight(720).Role(ui.RoleHeading)
				}
				if shelf.Title == "" || (m.Route == "search" && nativeSongShelf(shelf.Items)) {
					ui.Column(c).Gap(4).Children(func() {
						for j, item := range shelf.Items {
							nativeTrack(c.Key(fmt.Sprintf("list-%d", j)), item, j, m, a, p, compact, false)
						}
					})
				} else if nativeSongShelf(shelf.Items) {
					shelfScroll := ui.ScrollHorizontal(c).Gap(16).PaddingY(4).Height(272)
					clipped := nativeArtworkViewport(c, a, shelfScroll)
					shelfScroll.Children(func() {
						for start := 0; start < len(shelf.Items); start += 4 {
							ui.Column(c.Key(fmt.Sprintf("songs-%d", start))).Width(312).Shrink(0).Gap(4).Children(func() {
								for j := start; j < min(start+4, len(shelf.Items)); j++ {
									nativeTrack(c.Key(fmt.Sprintf("song-%d", j)), shelf.Items[j], j, m, clipped, p, true, false)
								}
							})
						}
					})
				} else {
					shelfScroll := ui.ScrollHorizontal(c).Gap(16).PaddingY(4).Height(240)
					clipped := nativeArtworkViewport(c, a, shelfScroll)
					shelfScroll.Children(func() {
						for j, item := range shelf.Items {
							nativeCard(c.Key(fmt.Sprintf("card-%d", j)), item, clipped, p, compact)
						}
					})
				}
			})
		}
		if len(m.Page.Shelves) == 0 && len(m.Page.Items) > 0 {
			ui.Column(c).Gap(4).Children(func() {
				for i, item := range m.Page.Items {
					nativeTrack(c.Key(fmt.Sprintf("track-%d", i)), item, i, m, a, p, compact, false)
				}
			})
		}
		if m.Page.Continuation != "" {
			if nativePill(c, a.Tr("nativeLoadMore"), p, false).AlignSelf(ui.Center).Disabled(m.Loading || a.LoadMore == nil).Clicked() {
				a.LoadMore()
			}
		}
		if !m.Loading && m.Error == "" && len(m.Page.Items) == 0 && len(m.Page.Shelves) == 0 && !(m.Route == "library" && !m.SignedIn) {
			ui.Column(c).Padding(32, 16).Gap(12).AlignItems(ui.Center).Children(func() {
				ui.Icon(c, nativeIcons["search"]).Size(40, 40).TextColor(p.muted)
				ui.Text(c, a.Tr("nativeEmptyResults")).FontSize(18).FontWeight(650)
				ui.Text(c, a.Tr("shellSearchPlaceholder")).TextColor(p.secondary)
			})
		}
	})
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
	return box.Background(p.input).Children(func() {
		if artwork != nil {
			ui.Image(c, artwork).Fill().Fit(ui.Cover)
		} else {
			ui.Icon(c, nativeIcons["note"]).Size(min(size*0.45, 48), min(size*0.45, 48)).TextColor(p.muted)
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

func nativeCard(c *ui.Context, item catalog.Item, a ViewActions, p nativePalette, compact bool) {
	size := float32(164)
	if compact {
		size = 148
	}
	b := ui.ButtonBase(c).Column().Width(size + 16).Shrink(0).Padding(8).Gap(10).Radius(8).AlignItems(ui.Stretch).Justify(ui.Start).Label(item.Title + " · " + item.Artist)
	if b.Hovered() {
		b.Background(p.raised)
	}
	b.Children(func() {
		nativeArtwork(c, item.Artwork, size, 6, a, p)
		ui.Text(c, item.Title).FontWeight(600).MaxLines(1).Ellipsis("…")
		ui.Text(c, item.Artist).FontSize(13).TextColor(p.secondary).MaxLines(1).Ellipsis("…")
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

func nativeTrack(c *ui.Context, item catalog.Item, index int, m *UIModel, a ViewActions, p nativePalette, compact, queue bool) {
	playing := item.ID != "" && item.ID == m.Current.ID
	b := ui.ButtonBase(c).Height(64).Padding(6, 8).Gap(12).Radius(8).Justify(ui.Start).Label(item.Title + " · " + item.Artist)
	if playing {
		b.Selected(true).Background(p.accent.Alpha(0.10)).TextColor(p.text)
	} else if b.Hovered() {
		b.Background(p.raised)
	}
	b.Children(func() {
		nativeArtwork(c, item.Artwork, 46, 4, a, p)
		ui.Column(c).Grow(1).MinWidth(0).Gap(5).Children(func() {
			t := ui.Text(c, item.Title).FontWeight(600).MaxLines(1).Ellipsis("…")
			if playing {
				t.TextColor(p.accent)
			}
			ui.Text(c, item.Artist).FontSize(13).TextColor(p.secondary).MaxLines(1).Ellipsis("…")
		})
		if !compact && !queue && item.Album != "" {
			ui.Text(c, item.Album).Width(180).TextColor(p.secondary).FontSize(13).MaxLines(1).Ellipsis("…")
		}
		if item.Duration > 0 {
			ui.Text(c, nativeTime(item.Duration.Seconds())).TextColor(p.muted).FontSize(12).Width(40).TextAlign(ui.End)
		}
		if playing {
			ui.Icon(c, nativeIcons["volume"]).Size(18, 18).TextColor(p.accent)
		}
	})
	b.ContextMenu(nativeTrackMenu(item, a))
	if b.Clicked() {
		if queue {
			a.PlayQueue(index)
		} else if item.ID != "" {
			a.Play(item)
		} else {
			a.Open(item)
		}
	}
}

func nativeTransport(c *ui.Context, m *UIModel, a ViewActions, p nativePalette, compact bool, width float32) ui.Element {
	if compact && m.Immersive {
		return nativeExpandedTransport(c, m, a, p)
	}
	return ui.Column(c).Background(p.base).Padding(0, 16).Children(func() {
		ui.Row(c).Grow(1).Gap(16).AlignItems(ui.Center).Children(func() {
			infoWidth := max(float32(170), min(width*0.25, 320))
			if compact {
				infoWidth = width - 144
				if m.PlaybackError != "" {
					infoWidth = width - 168
				}
			}
			info := ui.ButtonBase(c).Width(infoWidth).MinWidth(0).Gap(12).Justify(ui.Start).Label(a.Tr("shellOpenNowPlaying")).Children(func() {
				size := float32(56)
				if compact {
					size = 44
				}
				nativeArtwork(c, m.Current.Artwork, size, 6, a, p)
				ui.Column(c).Grow(1).MinWidth(0).Gap(5).Children(func() {
					title, artist := m.Current.Title, m.Current.Artist
					if title == "" {
						title, artist = a.Tr("shellIdleTitle"), a.Tr("shellIdleSubtitle")
					}
					ui.Text(c, title).FontWeight(600).MaxLines(1).Ellipsis("…")
					if m.PlaybackError != "" {
						artist = m.PlaybackError
					}
					caption := ui.Text(c, artist).FontSize(13).TextColor(p.secondary).MaxLines(1).Ellipsis("…")
					if m.PlaybackError != "" {
						caption.TextColor(p.accent)
					}
				})
			})
			if info.Clicked() {
				m.Panel = true
			}
			if !compact && m.PlaybackError == "" {
				if nativeIconButton(c, nativeHeart(m.Liked), a.Tr(nativeLikeKey(m.Liked)), p, m.Liked, 32).Disabled(m.Current.ID == "").Clicked() {
					a.Like()
				}
			}
			if m.PlaybackError != "" {
				if nativePill(c, a.Tr("buttonRetry"), p, false).Width(64).PaddingX(6).Disabled(a.RetryPlayback == nil).Clicked() {
					a.RetryPlayback()
				}
			}
			if compact {
				if m.PlaybackError == "" && nativeIconButton(c, nativeHeart(m.Liked), a.Tr(nativeLikeKey(m.Liked)), p, m.Liked, 40).Disabled(m.Current.ID == "").Clicked() {
					a.Like()
				}
				nativePlayButton(c, m, a, p, 40)
				return
			}
			ui.Column(c).Grow(1).MinWidth(150).MaxWidth(640).Gap(1).AlignItems(ui.Stretch).Children(func() {
				ui.Row(c).Height(44).Gap(12).AlignItems(ui.Center).Justify(ui.Center).Children(func() {
					if nativeIconButton(c, "shuffle", a.Tr("shellShuffle"), p, m.Shuffle, 32).Disabled(len(m.Queue) < 2).Clicked() {
						a.Shuffle()
					}
					if nativeIconButton(c, "previous", a.Tr("shellPrevious"), p, false, 32).Disabled(m.Current.ID == "").Clicked() {
						a.Previous()
					}
					nativePlayButton(c, m, a, p, 40)
					if nativeIconButton(c, "next", a.Tr("shellNext"), p, false, 32).Disabled(m.Current.ID == "").Clicked() {
						a.Next()
					}
					icon, key := "repeat", "shellRepeatOff"
					if m.Repeat == 1 {
						key = "shellRepeatAll"
					}
					if m.Repeat == 2 {
						icon, key = "repeat-one", "shellRepeatOne"
					}
					if nativeIconButton(c, icon, a.Tr(key), p, m.Repeat > 0, 32).Clicked() {
						a.Repeat()
					}
				})
				nativeSeek(c, m, a, p)
			})
			ui.Row(c).Gap(4).AlignItems(ui.Center).Justify(ui.End).Children(func() {
				if width > 1000 {
					if nativeIconButton(c, "lyrics", a.Tr("shellLyrics"), p, m.Panel && m.PanelTab == "lyrics", 32).Clicked() {
						m.Panel, m.PanelTab = true, "lyrics"
					}
				}
				if nativeIconButton(c, "queue", a.Tr("shellQueue"), p, m.Panel && m.PanelTab != "lyrics", 32).Clicked() {
					m.Panel, m.PanelTab = true, "queue"
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
		if compact {
			nativeProgressLine(c, m, p)
		}
	})
}

func nativePlayButton(c *ui.Context, m *UIModel, a ViewActions, p nativePalette, size float32) {
	if m.PlaybackLoading {
		ui.ButtonBase(c).Size(size, size).Shrink(0).Radius(size / 2).Background(p.text).TextColor(p.base).Disabled(true).Label(a.Tr("shellLoading")).Children(func() { ui.Spinner(c).Size(22, 22) })
		return
	}
	icon, key := "play", "shellPlay"
	if m.Playing {
		icon, key = "pause", "shellPause"
	}
	b := nativeIconButton(c, icon, a.Tr(key), p, false, size).Background(p.text).TextColor(p.base).Disabled(m.Current.ID == "")
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
		ui.Text(c, nativeTime(state.value)).Width(36).TextAlign(ui.End).FontSize(11).TextColor(p.muted)
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
		ui.Text(c, nativeTime(m.Duration)).Width(36).FontSize(11).TextColor(p.muted)
	})
}

func nativeSlider(c *ui.Context, value *float64, maximum float64, p nativePalette) ui.Element {
	s := ui.SliderBase(c, value, 0, maximum).Height(24).PaddingX(5)
	hovered := s.Hovered() || s.Focused()
	fraction := float32(max(0, min(1, *value/maximum)))
	s.Draw(func(painter *ui.Painter, r ui.Rect) {
		track := ui.Rect{X: r.X + 5, Y: r.Y + r.H/2 - 2, W: max(0, r.W-10), H: 4}
		painter.Fill(track, p.highlight, 2)
		fill := track
		fill.W *= fraction
		col := p.text
		if hovered {
			col = p.accent
		}
		painter.Fill(fill, col, 2)
		if hovered {
			painter.Fill(ui.Rect{X: track.X + fill.W - 5, Y: r.Y + r.H/2 - 5, W: 10, H: 10}, p.text, 5)
		}
	})
	return s
}

func nativeProgressLine(c *ui.Context, m *UIModel, p nativePalette) {
	value := float32(0)
	if m.Duration > 0 {
		value = float32(max(0, min(1, m.Position/m.Duration)))
	}
	ui.Box(c).Height(2).Background(p.input).Children(func() { ui.Box(c).Height(2).WidthPercent(value * 100).Background(p.accent) })
}

func nativeVolume(c *ui.Context, m *UIModel, a ViewActions, p nativePalette, showSlider bool) {
	icon, key := "volume", "shellMute"
	if m.Muted || m.Volume == 0 {
		icon, key = "volume-off", "shellUnmute"
	}
	if nativeIconButton(c, icon, a.Tr(key), p, false, 32).Clicked() {
		a.Mute()
	}
	if showSlider {
		volume := m.Volume
		s := nativeSlider(c.Key("volume"), &volume, 1, p).Width(92).Label(a.Tr("shellVolume"))
		if s.Changed() {
			a.Volume(volume)
		}
	}
}

func nativeTabbar(c *ui.Context, m *UIModel, a ViewActions, p nativePalette) {
	ui.Row(c).Height(64).Shrink(0).Padding(4, 8).Label(a.Tr("shellNavMain")).Children(func() {
		for _, nav := range []struct{ route, key, icon string }{{"home", "shellNavHome", "home"}, {"search", "shellNavSearch", "search"}, {"explore", "shellNavExplore", "explore"}, {"library", "shellNavLibrary", "library"}} {
			active := m.Route == nav.route
			b := ui.ButtonBase(c.Key("tab-" + nav.route)).Column().Grow(1).Gap(4).Radius(8).Label(a.Tr(nav.key)).Selected(active).Background(ui.Transparent).TextColor(p.secondary)
			if active {
				b.TextColor(p.text)
			}
			b.Children(func() {
				icon := nav.icon
				if active && icon != "search" {
					icon += "-fill"
				}
				ui.Icon(c, nativeIcons[icon]).Size(24, 24)
				ui.Text(c, a.Tr(nav.key)).FontSize(11).FontWeight(600)
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

func nativeNowPlaying(c *ui.Context, m *UIModel, a ViewActions, p nativePalette, width float32, overlay bool) ui.Element {
	_, windowHeight := c.Size()
	gap := float32(16)
	if windowHeight < 760 {
		gap = 12
	}
	return ui.Column(c).Background(p.surface).Radius(12).Padding(20).Gap(gap).MinHeight(0).Children(func() {
		ui.Row(c).Height(32).Shrink(0).AlignItems(ui.Center).Children(func() {
			ui.Text(c, a.Tr("shellNowPlaying")).FontWeight(700).Grow(1)
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
				nativeArtwork(c, "", 100, 12, a, p)
				ui.Text(c, a.Tr("shellEmptyTitle")).FontSize(20).FontWeight(700)
				ui.Text(c, a.TrArgs("shellEmptyBody", map[string]any{"shortcut": "⌘ K"})).TextColor(p.secondary).TextAlign(ui.Center)
			})
			return
		}
		body := func() {
			nativePlaybackNotice(c, m, a, p, false)
			artSize := min(width-40, float32(280), max(float32(120), windowHeight-520))
			if overlay {
				artSize = min(artSize, max(float32(120), windowHeight-580))
			}
			nativeMediaSwitch(c, m, a, p)
			nativeMedia(c, m, a, p, artSize).AlignSelf(ui.Center)
			ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
				ui.Column(c).Grow(1).MinWidth(0).Gap(6).Children(func() {
					ui.Text(c, m.Current.Title).FontSize(24).FontWeight(720).MaxLines(2).Ellipsis("…")
					ui.Text(c, m.Current.Artist).TextColor(p.secondary).MaxLines(1).Ellipsis("…")
				})
				if nativeIconButton(c, nativeHeart(m.Liked), a.Tr(nativeLikeKey(m.Liked)), p, m.Liked, 36).Clicked() {
					a.Like()
				}
			})
			if overlay {
				ui.Row(c).Gap(18).AlignItems(ui.Center).Justify(ui.Center).Children(func() {
					if nativeIconButton(c, "previous", a.Tr("shellPrevious"), p, false, 44).Clicked() {
						a.Previous()
					}
					nativePlayButton(c, m, a, p, 48)
					if nativeIconButton(c, "next", a.Tr("shellNext"), p, false, 44).Clicked() {
						a.Next()
					}
				})
				nativeSeek(c, m, a, p)
				ui.Row(c).AlignItems(ui.Center).Justify(ui.Center).Children(func() { nativeVolume(c, m, a, p, true) })
			}
			nativePanelTabs(c, m, a, p)
			queue := nativePanelBody(c, m, a, p)
			if overlay {
				queue.Height(220).Grow(0).Shrink(0)
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

func nativePanelTabs(c *ui.Context, m *UIModel, a ViewActions, p nativePalette) {
	ui.Row(c).Height(36).Padding(3).Gap(3).Radius(20).Background(p.raised).Children(func() {
		for _, tab := range []struct{ id, key string }{{"queue", "shellQueue"}, {"lyrics", "shellLyrics"}} {
			active := m.PanelTab == tab.id || (m.PanelTab == "" && tab.id == "queue")
			b := ui.Button(c, a.Tr(tab.key)).Label(a.Tr(tab.key) + " · " + a.Tr("shellNowPlaying")).Grow(1).Height(30).Padding(0).Radius(16).BorderWidth(0).Background(ui.Transparent).Selected(active).Background(ui.Transparent).TextColor(p.secondary)
			if active {
				b.Background(p.highlight).TextColor(p.text)
			}
			if b.Clicked() {
				m.PanelTab = tab.id
			}
		}
	})
}

func nativePanelBody(c *ui.Context, m *UIModel, a ViewActions, p nativePalette) ui.Element {
	scroll := ui.Scroll(c.Key("panel-" + m.PanelTab)).Grow(1).MinHeight(68).Gap(4)
	a = nativeArtworkViewport(c, a, scroll)
	return scroll.Children(func() {
		if m.PanelTab == "lyrics" {
			lyrics := m.Lyrics
			if lyrics == "" {
				lyrics = a.Tr("nativeLyricsUnavailable")
			}
			ui.Text(c, lyrics).FontSize(18).FontWeight(600).LineHeight(1.7).TextColor(p.secondary)
		} else {
			for i, item := range m.Queue {
				nativeTrack(c.Key(fmt.Sprintf("queue-%d", i)), item, i, m, a, p, true, true)
			}
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
		ui.Text(c, a.Tr("shellNowPlaying")).FontSize(12).FontWeight(700).TextColor(p.secondary)
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
			nativeMediaSwitch(c, m, a, p)
			nativeMedia(c, m, a, p, artSize).AlignSelf(ui.Center)
			nativeImmersiveTitle(c, m, a, p)
			nativePanelTabs(c, m, a, p)
			ui.Box(c).Height(220).Children(func() { nativePanelBody(c, m, a, p) })
		})
	} else {
		ui.Row(c).Grow(1).MinHeight(0).Padding(24, 40).Gap(40).AlignItems(ui.Center).Children(func() {
			ui.Column(c).Grow(1).MinWidth(0).Gap(24).AlignItems(ui.Center).Children(func() {
				artSize := min(max(float32(220), width-540), max(float32(180), height-240))
				nativeMediaSwitch(c, m, a, p)
				nativeMedia(c, m, a, p, artSize)
				nativeImmersiveTitle(c, m, a, p)
				nativePlaybackNotice(c, m, a, p, false)
			})
			ui.Column(c).Width(min(360, width*0.32)).HeightPercent(90).Padding(24).Gap(20).Radius(16).Background(p.raised).Children(func() { nativePanelTabs(c, m, a, p); nativePanelBody(c, m, a, p) })
		})
	}
}

func nativeImmersiveTitle(c *ui.Context, m *UIModel, a ViewActions, p nativePalette) {
	ui.Column(c).Gap(10).AlignItems(ui.Center).Children(func() {
		title, artist := m.Current.Title, m.Current.Artist
		if title == "" {
			title, artist = a.Tr("shellEmptyTitle"), a.Tr("shellIdleSubtitle")
		}
		ui.Text(c, title).FontSize(32).FontWeight(760).TextAlign(ui.Center).MaxLines(2).Ellipsis("…")
		ui.Text(c, artist).FontSize(16).TextColor(p.secondary).TextAlign(ui.Center)
	})
}

func nativeSettings(c *ui.Context, m *UIModel, a ViewActions, p nativePalette, width, height float32) {
	ui.DialogBase(c, &m.Settings, func(backdrop, panel ui.Element) {
		backdrop.Background(ui.RGBA(0, 0, 0, 0.40))
		panel.Width(min(390, width-24)).MaxHeight(height-32).Padding(24).Gap(20).Radius(16).Background(p.surface).Border(1, p.line).Shadow(0, 20, 48, 0, ui.RGBA(0, 0, 0, 0.4)).Children(func() {
			ui.Row(c).AlignItems(ui.Center).Children(func() {
				ui.Text(c, a.Tr("shellSettings")).FontSize(24).FontWeight(720).Grow(1)
				if nativeIconButton(c, "close", a.Tr("shellClose"), p, false, 32).Clicked() {
					m.Settings = false
				}
			})
			ui.Scroll(c).MinHeight(0).Gap(20).Children(func() {
				ui.Column(c).Gap(12).Children(func() {
					ui.Text(c, a.Tr("shellAppearance")).FontSize(13).FontWeight(650).TextColor(p.secondary)
					ui.Row(c).Gap(8).Children(func() {
						for _, option := range []struct{ id, key string }{{"system", "shellThemeSystem"}, {"dark", "shellThemeDark"}, {"light", "shellThemeLight"}} {
							if nativePill(c, a.Tr(option.key), p, m.Appearance == option.id).Grow(1).PaddingX(8).Clicked() {
								a.Appearance(option.id)
							}
						}
					})
				})
				ui.Column(c).Gap(8).Children(func() {
					ui.Text(c, a.Tr("shellLanguage")).FontSize(13).FontWeight(650).TextColor(p.secondary)
					for _, option := range []struct{ id, title string }{{"system", a.Tr("shellLanguageSystem")}, {"en", "English"}, {"zh-Hans", "简体中文"}, {"zh-Hant", "繁體中文"}, {"ja", "日本語"}} {
						active := m.Language == option.id
						b := ui.Button(c, option.title).Height(36).Justify(ui.Start).PaddingX(12).Radius(8).BorderWidth(0).Background(ui.Transparent).Selected(active)
						if active {
							b.Background(p.highlight).TextColor(p.accent)
						}
						if b.Clicked() {
							a.Language(option.id)
						}
					}
				})
				ui.Box(c).Height(1).Background(p.line)
				key := "shellAccountSignIn"
				if m.SignedIn {
					key = "shellAccountManage"
				}
				if ui.Button(c, a.Tr(key)).Height(40).BorderWidth(0).Background(p.raised).Clicked() {
					m.Settings = false
					a.Account()
				}
				if m.SignedIn {
					if ui.Button(c, a.Tr("shellSignOut")).Height(40).BorderWidth(0).Background(p.raised).TextColor(p.accent).Clicked() {
						m.Settings = false
						a.SignOut()
					}
				}
				if ui.Button(c, a.Tr("shellShortcuts")).Height(40).BorderWidth(0).Background(p.raised).Clicked() {
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
		backdrop.Background(ui.RGBA(0, 0, 0, 0.4))
		panel.Width(min(480, width-24)).MaxHeight(height - 32).Padding(24).Gap(20).Radius(16).Background(p.surface).Children(func() {
			ui.Row(c).AlignItems(ui.Center).Children(func() {
				ui.Text(c, a.Tr("shellShortcuts")).FontSize(24).FontWeight(720).Grow(1)
				if nativeIconButton(c, "close", a.Tr("shellClose"), p, false, 32).Clicked() {
					m.Shortcuts = false
				}
			})
			ui.Scroll(c).Gap(14).Children(func() {
				for _, shortcut := range []struct{ key, chord string }{{"shellShortcutPlayPause", "Space"}, {"shellShortcutNextPrevious", "⌘ → / ←"}, {"shellShortcutVolume", "⌘ ↑ / ↓"}, {"shellShortcutSearch", "⌘ K"}, {"shellShortcutPanel", "⌘ J"}, {"shellShortcutImmersive", "⌘ ⇧ F"}, {"shellShortcutHome", "⌘ ⇧ H"}, {"shellShortcutLibrary", "⌘ L"}, {"shellShortcutMute", "⌘ ⇧ M"}, {"shellShortcutSidebar", "⌘ B"}, {"shellShortcutDismiss", "Esc"}} {
					ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
						ui.Text(c, a.Tr(shortcut.key)).Grow(1)
						ui.Text(c, shortcut.chord).FontSize(12).Padding(5, 8).Radius(6).Background(p.input).TextColor(p.secondary)
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
}

func nativePresentation(m *UIModel) nativePresentationKey {
	return nativePresentationKey{
		texts:  [13]string{m.Route, m.Query, m.PanelTab, m.Appearance, m.Language, m.Current.ID, m.Current.Title, m.Page.Title, m.Error, m.Lyrics, m.Current.Artwork, m.VideoError, m.PlaybackError},
		flags:  [18]bool{m.Loading, m.Playing, m.Muted, m.Liked, m.Shuffle, m.Panel, m.Immersive, m.Settings, m.Shortcuts, m.SidebarCollapsed, m.SignedIn, m.CanBack, m.CanForward, m.FocusSearch, m.Video, m.VideoLoading, m.PlaybackLoading},
		values: [4]float64{m.Volume, m.Position, m.Duration, 0}, repeat: m.Repeat, queueLength: len(m.Queue), libraryLength: len(m.LibraryItems),
	}
}

func nativeBackdrop(c *ui.Context, m *UIModel, a ViewActions) {
	src := m.Page.Artwork
	if m.Immersive || src == "" {
		src = m.Current.Artwork
	}
	if src == "" || a.Artwork == nil {
		return
	}
	artwork := a.Artwork(src, artworkEdge)
	if artwork == nil {
		return
	}
	// Blur a small layer, then draw it at the final size without a transform.
	// A transformed child makes MyGo allocate a full-size RGBA mask for the
	// rounded parent clip, which would erase the smaller canvas's savings.
	layer := ui.Box(c).Absolute().Left(0).Top(0).WidthPercent(25).Effect(backdropLayer{}).PassThrough()
	if m.Immersive {
		layer.HeightPercent(25).Opacity(0.14)
	} else {
		layer.Height(80).Opacity(0.07)
	}
	layer.Children(func() { ui.Image(c, artwork).Fill().Fit(ui.Cover).Blur(17.5).PassThrough() })
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
	ui.Row(c).Height(32).Shrink(0).Gap(4).Padding(2).Radius(20).Background(p.raised).AlignSelf(ui.Center).Children(func() {
		for _, option := range []struct {
			video bool
			key   string
		}{{false, "nativeArtwork"}, {true, "nativeVideo"}} {
			b := ui.Button(c, a.Tr(option.key)).MinWidth(88).Height(28).Padding(3, 12).Radius(16).BorderWidth(0).Selected(m.Video == option.video).Background(ui.Transparent).TextColor(p.secondary).Disabled(m.Current.ID == "" || a.Video == nil)
			if m.Video == option.video {
				b.Background(p.highlight).TextColor(p.text)
			}
			if b.Clicked() {
				a.Video(option.video)
			}
		}
	})
}

func nativeMedia(c *ui.Context, m *UIModel, a ViewActions, p nativePalette, size float32) ui.Element {
	if !m.Video {
		return nativeArtwork(c, m.Current.Artwork, size, 12, a, p)
	}
	return ui.Box(c).Size(size, size*9/16).Shrink(0).Radius(12).Background(ui.Hex("#000000")).Clip().Center().Label(a.Tr("nativeVideo")).Children(func() {
		if m.VideoFrame != nil {
			ui.Image(c, m.VideoFrame).Size(size, size*9/16).Fit(ui.Contain)
		} else if m.VideoLoading {
			ui.Spinner(c).Size(28, 28).Label(a.Tr("shellLoading"))
		} else {
			message := m.VideoError
			if message == "" {
				message = a.Tr("nativeVideoUnavailable")
			}
			ui.Text(c, message).Padding(16).TextAlign(ui.Center).TextColor(ui.Hex("#a8a8af")).FontSize(13)
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
			key, icon := "shellRepeatOff", "repeat"
			if m.Repeat == 1 {
				key = "shellRepeatAll"
			}
			if m.Repeat == 2 {
				key, icon = "shellRepeatOne", "repeat-one"
			}
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
	row := ui.Row(c).Gap(8).Padding(10).Radius(8).Background(p.raised).AlignItems(ui.Center)
	if footer {
		row.Height(24).Padding(0).Background(ui.Transparent)
	}
	row.Children(func() {
		text := ui.Text(c, m.PlaybackError).Grow(1).MinWidth(0).FontSize(12).TextColor(p.accent).MaxLines(2).Ellipsis("…")
		if footer {
			text.MaxLines(1).FontSize(11)
		}
		button := nativePill(c, a.Tr("buttonRetry"), p, false).Width(64).PaddingX(6).Disabled(a.RetryPlayback == nil)
		if footer {
			button.Height(24).MinHeight(24).PaddingY(0)
		}
		if button.Clicked() {
			a.RetryPlayback()
		}
	})
}
