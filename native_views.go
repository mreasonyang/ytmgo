package main

import (
	"fmt"
	"math"
	"slices"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"ytmgo/internal/audioquality"
	"ytmgo/internal/catalog"
	"ytmgo/internal/downloads"
	"ytmgo/locales"
)

// UIModel is owned by the UI thread. Network and player completions are applied
// by the controller before a frame; the view changes only local presentation.
type UIModel struct {
	Downloads                                                   []downloads.Entry
	downloadStates                                              map[string]downloads.State
	DownloadNotice                                              string
	DownloadWaiting                                             bool
	barMenu, sheetMenu, accountMenu                             bool              // which app menu shows
	collectionDownloads                                         map[string]string // route → its download notice while preparing or failed
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
	// VideoAvailable: the last track that loaded has a video to watch (a
	// music video, not a song's still cover). It holds while the next one
	// loads, so a run of videos keeps its view; Video is only the choice.
	VideoAvailable bool
	// VideoTheater: the video fills the play view (immersive mode) rather
	// than sharing it with the title and the card. VideoFullScreen: the
	// window is full screen with nothing but the video.
	VideoTheater, VideoFullScreen                              bool
	VideoFrame                                                 ui.ImageSource
	PlaybackError                                              string
	VideoError                                                 string
	AudioQuality                                               audioquality.Quality
	AudioQualityNotice                                         string
	VideoQuality                                               int
	VideoQualityError                                          string
	ErrorScope                                                 errorScope
	AccountRestore                                             string
	AccountNotice                                              string
	AccountAvatar                                              string
	Version                                                    string
	errorMessage, playbackMessage, videoMessage, lyricsMessage uiMessage

	// Presentation state for this window.
	playbackViewSeen                bool // first playback opens Now Playing unless already visited
	settingsScroll, shortcutsScroll ui.ScrollState
	shortcutFieldFocused            bool            // the recorder has had the focus
	revealShortcut                  bool            // bring the global shortcut row into view
	shortcutsFromSettings           bool            // Keyboard shortcuts was opened from Settings: it leads back
	pageScroll                      *ui.ScrollState // the page's scrolling, as last built
	instantMotion                   bool            // every frame without animation, for tests of layout

	openedCover nativeCollectionMatch // source of the collection opened in this window

	// Immersive mode's morph: when it last came or went, whether Now Playing
	// was docked as it came (its cover is the one that grows), and whether
	// it is docked now.
	wasImmersive, immersiveFromDock, docked bool
	immersiveAt                             time.Time
	floating                                nativeFloatState // the floating Now Playing sheet's motion
	volumeKeyedAt                           time.Time        // a key last changed the volume: show it
	theaterFromPanel                        bool             // the video filled the view from Now Playing: it goes back there
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
	Download                                            func(catalog.Item)
	DownloadCollection                                  func()
	DownloadPause                                       func(string, bool)
	DownloadRemove                                      func(string)
	DownloadRemoveAll                                   func() // asks first
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
	artworkPeek                                         func(string, int) ui.ImageSource
	// ArtworkTint is the colour a loaded cover lends the interface.
	ArtworkTint                   func(string) (ui.Color, bool)
	Video                         func(bool)
	FullScreen                    func(bool) // the window, for the video
	SetAudioQuality               func(audioquality.Quality)
	SetVideoQuality               func(int)
	SetLyricsExpanded             func(bool)
	RetryPlayback                 func()
	LoadMore                      func()
	BeginShortcut, CancelShortcut func()
	SetShortcut                   func(string)
	// AppMenu fills the application menu button, where a hidden title bar
	// leaves the window no menu bar (Linux, Windows); nil hides the button.
	AppMenu              func(*ui.Menu)
	artViewport          ui.Rect
	viewportScroll       *ui.ScrollState
	secondaryArtwork     bool
	collection           nativeCollectionMatch
	artworkScale         float64
	artworkDX, artworkDY float32
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
	// nativeNowPlayingChrome (the global bar, player bar, frame gap, panel padding,
	// header, media switch, title and the lyrics card's chips, measured).
	// Lyrics first, it is a nativeCoverThumb beside the title.
	nativeCoverMin         = 120
	nativeCoverShare       = 0.45
	nativeNowPlayingChrome = 456
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
	if m.instantMotion {
		c.DisableAnimations()
	}
	before := nativePresentation(m)
	nativeKeyboard(c, m, a)
	// The video fills the play view only in immersive mode: leaving it,
	// however, ends that.
	if !m.Immersive {
		m.VideoTheater, m.theaterFromPanel = false, false
	}
	// After the keys, which may have toggled it: this frame lays it out.
	if m.Immersive != m.wasImmersive {
		m.wasImmersive, m.immersiveAt = m.Immersive, c.Now()
		m.immersiveFromDock = m.Immersive && m.docked
	}
	nativeKeyScrollers = nativeKeyScrollers[:0]
	defer func() {
		// The scrolling keys glide the scroller they are for, now that every
		// scroller has been built; recording a shortcut takes every key.
		if len(nativeKeyScrollers) > 0 && !m.ShortcutRecording {
			for _, k := range nativeScrollKeys {
				if c.Shortcut(0, k.key) {
					nativeScrollByKey(k.key)
				}
			}
		}
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
	c.Root().ObserveInput(nativeReleaseScrollGrabs)
	p := palette(dark)
	if m.VideoFullScreen {
		nativeFullScreenVideo(c, m, a, width, height)
		return
	}
	if width <= 760 {
		nativeCompactFrame(c, m, a, p, width, height)
	} else {
		nativeDesktopFrame(c, m, a, p, width, height)
	}
	if shown, arrived := nativeFloating(c, m, width); shown {
		panelWidth := min(width-16, 380)
		open := true
		ui.DialogBase(c, &open, func(backdrop, panel ui.Element) {
			nativePlaybackShortcut(c, panel, m, a)
			if panel.Shortcut(ui.Cmd, ui.KeyK) {
				m.focusSearch()
			}
			// It rises from the bottom, where the mini player and the player
			// bar that open it are, and sinks back there as it closes. A dialog
			// lies over the window, after MyGo's transitions: it moves itself.
			backdrop.Background(p.scrim.Alpha(float32(p.scrim.A) / 255 * arrived))
			panel.Width(panelWidth).Height(min(height-100, 760)).Radius(tokenRadiusPanel).Background(p.surface).Shadow(tokenShadowHero.args(p)).Clip().
				Translate(0, nativeFloatRise*(1-arrived)).Opacity(arrived).Children(func() {
				nativeNowPlaying(c, m, a, p, panelWidth, true, firstPlayback).Fill()
			})
		}).Label(a.Tr("shellNowPlaying"))
		if !open {
			m.Panel = false
		}
	}
	nativeSheets(c, m, a, p, width, height)
	if m.ShortcutRecording && !m.Settings && a.CancelShortcut != nil {
		a.CancelShortcut()
	}
}

// nativeDesktopFrame is Spotify's desktop arrangement: a global bar on the
// window frame, three rounded panels, and the player bar across the bottom.
func nativeDesktopFrame(c *ui.Context, m *UIModel, a ViewActions, p nativePalette, width, height float32) {
	frame := ui.Column(c).Fill().Background(p.base).TextColor(p.text)
	nativePlaybackShortcut(c, frame, m, a)
	frame.Children(func() {
		// Immersive mode drops the global bar. Under a hidden title bar its
		// header moves out of the panel to take the bar's place instead.
		top, header := float32(tokenProductFrameGap), float32(0)
		if !m.Immersive {
			nativeTopbar(c, m, a, p, width)
			top = 0
		} else if c.TitleBar().Height > 0 {
			header = nativeImmersiveHeader(c, m, a, p, width)
			top = 0
		}
		// Panels sit a frame gap from every edge, the player bar's too: flush
		// against it their bottom corners would sit on the bar's colour.
		row := ui.Row(c).Grow(1).MinHeight(0).AlignItems(ui.Stretch).Gap(tokenProductFrameGap).Padding(top, tokenProductFrameGap, tokenProductFrameGap)
		docked, leaving := nativeDock(c, row, m, width)
		m.docked = docked
		pageWidth := nativePageWidth(m, width)
		if leaving {
			pageWidth -= nativePanelWidth + tokenProductFrameGap
		}
		row.Children(func() {
			if !m.Immersive {
				nativeLibrary(c, m, a, p, width)
			}
			// The page panel moves in step with what changes beside it: it grows
			// over the library's and the global bar's room as immersive mode
			// fades them, and follows the library to the rail and back and Now
			// Playing's room as it goes, rather than jumping.
			pagePanel := ui.Column(c).Grow(1).MinWidth(0).MinHeight(0).Radius(tokenRadiusPanel).Background(p.surface).Clip().Transition(nativeLong(nativeMoves))
			pagePanel.Children(func() {
				nativeBackdrop(c, m, a, p)
				var scrolled *ui.ScrollState
				nativePageWash(nativePageLayer(c), m, a, p, &scrolled).Children(func() {
					if m.Immersive {
						nativeImmersive(c, m, a, p, width, height-nativePlayerHeight-top-header-tokenProductFrameGap, header == 0)
					} else {
						scrolled = nativeContent(c, m, a, p, false, pageWidth)
						m.pageScroll = scrolled
					}
				})
			})
			// Now Playing slides in from the window's right edge, where the
			// controls that open it are, and back out there. As it goes, its
			// room stays until it has gone, so the page fills it once, after.
			// Back from immersive mode, it is where the cover and the card
			// fly to: it fades in in place, as they land, rather than
			// sliding under them.
			if docked {
				dock := nativeDockSlide
				if nativeImmersing(c, m) {
					dock = nativeFadeLong
				}
				nativeNowPlaying(c.Key("now-playing-docked"), m, a, p, nativePanelWidth, false, false).Width(nativePanelWidth).Shrink(0).Transition(dock)
			} else if leaving {
				ui.Box(c.Key("now-playing-room")).Width(nativePanelWidth).Shrink(0)
			}
		})
		nativePlayerBar(c, m, a, p, width).Height(nativePlayerHeight).Shrink(0)
	})
}

// nativeCompactFrame follows Spotify's phone layout: one full-bleed page, a
// floating mini player and a tab bar.
func nativeCompactFrame(c *ui.Context, m *UIModel, a ViewActions, p nativePalette, width, height float32) {
	frame := ui.Column(c).Fill().Background(p.base).TextColor(p.text)
	nativePlaybackShortcut(c, frame, m, a)
	frame.Children(func() {
		ui.Column(c).Grow(1).MinWidth(0).MinHeight(0).Background(p.surface).Clip().Children(func() {
			nativeBackdrop(c, m, a, p)
			var scrolled *ui.ScrollState
			nativePageWash(nativePageLayer(c), m, a, p, &scrolled).Children(func() {
				if m.Immersive {
					nativeImmersive(c, m, a, p, width, height-nativeExpandedBar-nativeCompactTabbar, true)
				} else {
					nativeCompactTopbar(c, m, a, p, width)
					scrolled = nativeContent(c, m, a, p, true, width)
					m.pageScroll = scrolled
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

// nativeMorph moves what persists across a change of layout, as the
// guidebook's panel changes do: over the base motion along the tokens'
// ease. While the desktop asks for less motion, MyGo moves it at once.
var nativeMorph = ui.ElementTransition{Duration: tokenMotionBase, Ease: tokenMotionEase}

// nativeMoves is nativeMorph for where an element is and how large: the
// pieces of a layout that rearranges around it.
var nativeMoves = ui.ElementTransition{Duration: tokenMotionBase, Ease: tokenMotionEase, Position: true, Size: true}

// nativeCrossfade fades content that replaces content in place: the next
// cover over the last, over the base motion.
var nativeCrossfade = ui.ElementTransition{Duration: tokenMotionBase, Ease: tokenMotionEase, Enter: &ui.Motion{}, Exit: &ui.Motion{}}

// nativeSwap cross-fades a small swap in place (a glyph, a card's view)
// over the fast motion.
var nativeSwap = ui.ElementTransition{Duration: tokenMotionFast, Ease: tokenMotionEase, Enter: &ui.Motion{}, Exit: &ui.Motion{}}

// nativeAppear brings in a popover laid out over the window, where MyGo's
// transitions don't reach: from 4 above and transparent to its place over
// the fast motion, as the website's menus open.
func nativeAppear(c *ui.Context, e ui.Element, still bool) ui.Element {
	opened := ui.Local(e, "appeared", func() time.Time { return c.Now() })
	k := float32(c.Now().Sub(*opened)) / float32(tokenMotionFast)
	if k >= 1 || still || c.Preferences().ReduceMotion {
		return e
	}
	c.AnimationFrame()
	arrived := tokenMotionEase(max(0, k))
	return e.Opacity(arrived).Translate(0, -4*(1-arrived))
}

// nativeTrackText is how a track's title and artist arrive as the track
// changes: up 8 from below and in, over the last one fading, as the
// website's player has them.
var nativeTrackText = ui.ElementTransition{Duration: tokenMotionBase, Ease: tokenMotionEase, Enter: &ui.Motion{Y: 8}, Exit: &ui.Motion{}}

// nativeLong is nativeMorph for a long journey across the window, as
// into and out of immersive mode: over the morph motion.
func nativeLong(t ui.ElementTransition) ui.ElementTransition {
	t.Duration = tokenMotionMorph
	return t
}

// nativeFlight is nativeLong for an element that matches across immersive
// mode's way in or out: it flies over the window, out of the clips of the
// column, scroller or panel it lands in, and from the motion of the
// elements around it (the docked panel sliding) rather than adding to it:
// without, the cover showed as a sliver cut by its new column's edge, and
// leaving, it swung past its place with the panel's slide.
func nativeFlight(t ui.ElementTransition) ui.ElementTransition {
	t = nativeLong(t)
	t.Fly = true
	return t
}

// nativeFadeLong fades what immersive mode puts away or brings back (the
// global bar, the library, its backdrop), in step with its morph.
var nativeFadeLong = ui.ElementTransition{Duration: tokenMotionMorph, Ease: tokenMotionEase, Enter: &ui.Motion{}, Exit: &ui.Motion{}}

// nativeImmersing reports whether immersive mode is coming or going: its
// morph's long journeys use the morph motion while it does.
func nativeImmersing(c *ui.Context, m *UIModel) bool {
	return !m.immersiveAt.IsZero() && c.Now().Sub(m.immersiveAt) < tokenMotionMorph && !m.instantMotion
}

// nativeImmersiveCover is the immersive cover's identity: as it comes, the
// cover it grows from (docked Now Playing's, else the player bar's); after,
// the one it would shrink into were it to go now.
func nativeImmersiveCover(c *ui.Context, m *UIModel, width float32) nativeCoverMatch {
	fromDock := m.immersiveFromDock
	if !nativeImmersing(c, m) {
		fromDock = m.Panel && width > 1180
	}
	if fromDock {
		return nativeCoverMatch{"now-playing"}
	}
	return nativeCoverMatch{"player-bar"}
}

// Each entry has its own match, including two copies of one collection on Home.
// The route distinguishes opening a persistent sidebar row from returning to it.
type nativeCollectionMatch struct {
	route  string
	button uint64
	id     string
}

func nativeCollectionSource(m *UIModel, button ui.Element, item catalog.Item) nativeCollectionMatch {
	if item.BrowseID == "" || item.ID != "" {
		return nativeCollectionMatch{}
	}
	return nativeCollectionMatch{route: m.Route, button: button.ID(), id: item.BrowseID}
}

func (a ViewActions) openCollection(m *UIModel, item catalog.Item) {
	m.openedCover = a.collection
	a.Open(item)
}

func (match nativeCollectionMatch) art(art ui.Element) ui.Element {
	if match.id == "" {
		return art
	}
	return art.Transition(nativeLong(nativeMatch(match)))
}

func (a ViewActions) collectionArt(c *ui.Context, item catalog.Item, size, radius float32, m *UIModel, p nativePalette) ui.Element {
	// The sidebar stays on screen beside the header. Give its ordinary
	// thumbnail a separate identity while the detail owns the match. On
	// Back the returning thumbnail can then take the match from the header.
	if a.collection.id != "" && a.collection.button == m.openedCover.button && m.Route == "browse:"+a.collection.id {
		return nativeArtwork(c.Key("open-collection-thumbnail"), item.Artwork, size, radius, a, p).Transition(nativeLong(nativeMoves))
	}
	return a.collection.art(nativeArtwork(c, item.Artwork, size, radius, a, p))
}

// nativeCardMatch is the identity of the card holding the lyrics and the
// queue, in Now Playing and in immersive mode.
type nativeCardMatch struct{}

// nativeCoverMatch is the playing cover's identity in a place that shows
// it large or small (Now Playing's cover and its thumbnail): one appearing
// grows or shrinks from where the other was, instead of cutting.
type nativeCoverMatch struct{ place string }

// nativeMatch is nativeMorph matching the element to the one before with
// the identity id.
func nativeMatch(id any) ui.ElementTransition {
	t := nativeMorph
	t.Match = id
	return t
}

// nativeTip is the app's tooltip: Element.Tooltip's behaviour in the
// guidebook's look, as MyGo's own draws off the grid (a 5 px corner).
func nativeTip(c *ui.Context, e ui.Element, s string, p nativePalette) ui.Element {
	if s == "" {
		return e
	}
	e.Description(s)
	ui.TooltipBase(c, e, func(tip ui.Element) {
		tip.MaxWidth(320).Padding(4, 8).Radius(tokenRadiusArt).Background(p.text).TextColor(p.base).FontSize(tokenSizeMicro).Shadow(tokenShadowTooltip.args(p))
		tip.Children(func() { ui.Text(c, s) })
		tip.AttachTo(e, ui.AnchorTop, ui.AnchorBottom).Margin(0, 0, 8, 0)
	})
	return e
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
	return nativeIconFace(c, ui.ButtonBase(c), name, label, p, active, hit, on)
}

// nativeIconFace gives b an icon button's look: a menu's trigger takes it too.
func nativeIconFace(c *ui.Context, b ui.Element, name, label string, p nativePalette, active bool, hit float32, on ui.Color) ui.Element {
	b.Size(hit, hit).Shrink(0).Radius(hit / 2).Label(label).TextColor(p.secondary)
	nativeTip(c, b, label, p)
	if active {
		b.Selected(true).Background(ui.Transparent).TextColor(on)
	} else if b.Hovered() {
		b.Background(p.hover).TextColor(p.text)
	}
	nativePress(b)
	b.Children(func() {
		icon := min(nativeGrid(hit*0.6), 24)
		ui.Icon(c, nativeIcons[name]).Size(icon, icon)
		if active {
			ui.Box(c).Absolute().Left((hit-4)/2).Bottom(0).Size(4, 4).Radius(tokenRadiusPill).Background(on).PassThrough()
		}
	})
	return b
}

// nativeGrid rounds a computed dimension to the 4 px grid (design/README.md
// §6): every spatial value is a multiple of 4.
func nativeGrid(v float32) float32 {
	return max(4, float32(math.Round(float64(v)/4))*4)
}

// nativeGridFloor rounds a computed size down to the 4 px grid, for sizes
// that must not outgrow the space they were measured to fit.
func nativeGridFloor(v float32) float32 {
	return max(4, float32(math.Floor(float64(v)/4))*4)
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
	b := ui.ButtonBase(c).Size(size, size).Shrink(0).Radius(size / 2).Label(label).Background(p.raised).TextColor(p.secondary)
	nativeTip(c, b, label, p)
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
			b.Background(p.inverseHover)
		}
	} else if b.Hovered() {
		b.Background(p.highlight)
	}
	nativePress(b)
	return b
}

// nativeChip is a filter chip: translucent at rest, inverted when selected.
func nativeChip(c *ui.Context, label string, p nativePalette, selected bool) ui.Element {
	b := ui.Button(c, label).Height(tokenProductChip).MinHeight(tokenProductChip).Padding(0, 12).Radius(tokenRadiusPill).BorderWidth(0).FontSize(tokenSizeMeta).FontWeight(500).Background(p.hover).TextColor(p.text).
		Transition(ui.ElementTransition{Duration: tokenMotionFast, Ease: tokenMotionEase, Colors: true})
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
	nativeTitleBarRow(c, bar).Height(max(nativeTopbarHeight, bar.Height)).Padding(0, right, 0, left).Gap(16).AlignItems(ui.Center).Transition(nativeFadeLong).Children(func() {
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
		ui.Icon(c, nativeIcons["search"]).Size(20, 20).TextColor(p.secondary)
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
		avatar = a.Artwork(m.AccountAvatar, a.artworkPixels(size), artAvatar)
	}
	return ui.Box(c).Size(size, size).Shrink(0).Radius(size / 2).Clip().Background(bg).TextColor(fg).Center().PassThrough().Children(func() {
		if avatar != nil {
			ui.Image(c, avatar).Fill().Fit(ui.Cover).PassThrough()
		} else {
			ui.Icon(c, nativeIcons["user"]).Size(nativeGrid(size*0.56), nativeGrid(size*0.56))
		}
	})
}

// nativeAccount is the top-right control: a sign-in call to action when
// signed out, otherwise an avatar whose menu states the account's plan.
func nativeAccount(c *ui.Context, m *UIModel, a ViewActions, p nativePalette, badge bool) {
	if nativeRestoring(m) {
		// Sign-in cannot start until restoration settles, so it is not offered.
		status := nativeAccountStatus(m, a)
		chip := ui.Row(c).Height(36).MinWidth(0).Shrink(1).PaddingX(12).Gap(8).Radius(tokenRadiusPill).Background(p.raised).AlignItems(ui.Center).Label(status)
		nativeTip(c, chip, status, p)
		chip.Children(func() {
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
	parts := ui.MenuBase(c.Key("account"), &m.accountMenu)
	b := parts.Trigger.Height(48).MinWidth(48).Shrink(0).Padding(8).Gap(8).Radius(tokenRadiusPill).Background(p.raised).Label(a.Tr("menuAccount") + " · " + status)
	nativeTip(c, b, status, p)
	if b.Hovered() {
		b.Background(p.highlight)
	}
	b.Children(func() {
		if m.MusicPremium && badge {
			ui.Text(c, a.Tr("shellPremiumBadge")).PaddingStart(4).FontSize(tokenSizeMicro).FontWeight(700).TextColor(p.text)
		}
		nativeAvatar(c, m, a, p, 32)
	})
	if parts.Open() {
		nativeMenuPanel(c, parts, nativeAccountItems(m, a, status), m, p)
	}
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

// nativeLibraryPanelWidth is the library's width in a window width wide:
// the full panel, or the rail on narrower windows and when collapsed.
func nativeLibraryPanelWidth(m *UIModel, width float32) float32 {
	if width > 1180 && !m.SidebarCollapsed {
		return nativeLibraryWidth
	}
	return nativeRailWidth
}

// nativeDockedNowPlaying reports whether Now Playing docks beside the page
// (wider windows) rather than floating over it.
func nativeDockedNowPlaying(m *UIModel, width float32) bool {
	return m.Panel && !m.Immersive && width > 1180
}

// nativeFloatRise is how far the floating Now Playing sheet rises as it
// opens, and sinks as it closes.
const nativeFloatRise = 64

type nativeFloatState struct {
	open  bool
	since time.Time
	from  float32
}

// arrival is the current fraction of the opening movement. A reversal starts
// from this same position and only takes the time for the remaining distance.
func (s nativeFloatState) arrival(now time.Time) float32 {
	to, distance := float32(0), s.from
	if s.open {
		to, distance = 1, 1-s.from
	}
	d := time.Duration(float64(tokenMotionBase) * float64(distance))
	if s.since.IsZero() || d == 0 || now.Sub(s.since) >= d {
		return to
	}
	return s.from + (to-s.from)*tokenMotionEase(float32(now.Sub(s.since))/float32(d))
}

// nativeFloating reports whether the floating Now Playing sheet is on screen
// and how far it has arrived, including while it is leaving.
// nativeFloatingWanted reports whether Now Playing floats over the window.
func nativeFloatingWanted(m *UIModel, width float32) bool {
	return m.Panel && !m.Immersive && width <= 1180
}

func nativeFloating(c *ui.Context, m *UIModel, width float32) (bool, float32) {
	s := &m.floating
	want := nativeFloatingWanted(m, width)
	now := c.Now()
	if m.instantMotion || c.Preferences().ReduceMotion || m.Immersive || width > 1180 {
		*s = nativeFloatState{open: want}
	}
	arrived := s.arrival(now)
	if want != s.open {
		*s = nativeFloatState{open: want, from: arrived, since: now}
	}
	if (want && arrived < 1) || (!want && arrived > 0) {
		c.AnimationFrame()
		return true, arrived
	}
	return want, arrived
}

// nativeDockSlide is how docked Now Playing comes and goes: across its own
// width from the window's right edge, fading, over the base motion.
var nativeDockSlide = ui.ElementTransition{Duration: tokenMotionBase, Ease: tokenMotionEase, Enter: &ui.Motion{X: nativePanelWidth}, Exit: &ui.Motion{X: nativePanelWidth}}

type nativeDockState struct {
	shown   bool
	leaving time.Time
}

// nativeDock reports whether Now Playing is docked in row, and whether it
// is leaving: closed, still sliding out, its room kept until it has gone.
// It goes at once without motion, or into immersive mode, or as the window
// narrows past docking.
func nativeDock(c *ui.Context, row ui.Element, m *UIModel, width float32) (docked, leaving bool) {
	s := ui.Local(row, "now-playing-dock", func() nativeDockState { return nativeDockState{} })
	docked = nativeDockedNowPlaying(m, width)
	now := c.Now()
	animate := !m.Immersive && width > 1180 && !m.instantMotion && !c.Preferences().ReduceMotion
	switch {
	case docked:
		s.shown, s.leaving = true, time.Time{}
	case s.shown && s.leaving.IsZero() && animate:
		s.leaving = now
	}
	if !docked && animate && !s.leaving.IsZero() {
		if left := tokenMotionBase - now.Sub(s.leaving); left > 0 {
			c.After(left)
			return false, true
		}
	}
	if !docked {
		s.shown, s.leaving = false, time.Time{}
	}
	return docked, false
}

// nativePageWidth is the page panel's width in a desktop window width wide:
// what the frame gaps, the library and a docked Now Playing leave. The page
// lays out from it in the frame that changes it, as a panel opens or the
// window resizes, instead of from the last frame's bounds, which drew one
// frame of the old layout.
func nativePageWidth(m *UIModel, width float32) float32 {
	w := width - 2*tokenProductFrameGap
	if !m.Immersive {
		w -= nativeLibraryPanelWidth(m, width) + tokenProductFrameGap
	}
	if nativeDockedNowPlaying(m, width) {
		w -= nativePanelWidth + tokenProductFrameGap
	}
	return max(0, w)
}

func nativeLibrary(c *ui.Context, m *UIModel, a ViewActions, p nativePalette, width float32) {
	a.secondaryArtwork = true
	full := width > 1180 && !m.SidebarCollapsed
	panelWidth := nativeLibraryPanelWidth(m, width)
	// It fades with immersive mode, and narrows to the rail and back.
	library := nativeFadeLong
	library.Size = true
	panel := ui.Column(c).Width(panelWidth).Shrink(0).MinHeight(0).Background(p.surface).Radius(tokenRadiusPanel).Clip().Transition(library)
	filter := ui.Local(panel, "library-filter", func() string { return "" })
	panel.Children(func() {
		libraryIcon := "library"
		if m.Route == "library" {
			libraryIcon = "library-fill"
		}
		if full {
			ui.Row(c).Height(56).Shrink(0).Padding(8, 12).Gap(8).AlignItems(ui.Center).Children(func() {
				b := ui.ButtonBase(c.Key("library-title")).Grow(1).MinWidth(0).Height(40).PaddingX(8).Gap(12).Justify(ui.Start).Radius(tokenRadiusControl).Label(a.Tr("shellNavLibrary")).TextColor(p.secondary)
				nativeTip(c, b, a.Tr("shellNavLibrary"), p)
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
			// Downloads still leads an empty library: signed out, its songs
			// are all there is.
			if nativeDownloadsShown(m, a) {
				pad := float32(8)
				if !full {
					pad = 4
				}
				ui.Column(c).Shrink(0).Padding(0, pad, 8).Children(func() { nativeDownloadsEntry(c, m, a, p, full) })
			}
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
			// Wrapped: a language with longer names (アーティスト) takes a
			// second line rather than cutting a chip at the panel's edge.
			ui.Row(c).Shrink(0).Padding(0, 16, 8).Gap(8).Wrap().AlignItems(ui.Center).Children(func() {
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
		// Filters and status rows disappear in rail mode. Keep the list's
		// identity so its thumbnails stay in their rows during the resize.
		scroll := ui.Scroll(c.Key("library-scroll")).Grow(1).MinHeight(0).Padding(0, pad, 8).Label(a.Tr("shellLibraryRegion"))
		clipped := nativeArtworkViewport(c, a, scroll)
		nativeScrollEdges(c, scroll, clipped.viewportScroll, p, p.surface)
		scroll.Children(func() {
			// Downloads is the library's own collection: first in the list,
			// as Liked Music leads YouTube's, while no filter narrows it
			// (it is none of their kinds).
			if *filter == "" && nativeDownloadsShown(m, a) {
				nativeDownloadsEntry(c, m, a, p, full)
			}
			occurrences := make(map[string]int, len(items))
			for _, item := range items {
				if *filter != "" && nativeLibraryKind(item) != *filter {
					continue
				}
				key := item.Key()
				rowKey := key
				if n := occurrences[key]; n > 0 {
					// A continuation can overlap a previous page.
					rowKey = fmt.Sprintf("%s#%d", key, n)
				}
				occurrences[key]++
				nativeLibraryRow(c.Key(rowKey), item, m, clipped, p, full)
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
			ui.Spinner(c).Size(16, 16).Label(a.Tr(key))
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
					ui.Box(c).Width(float32(72 + 24*((i+1)%3))).Height(8).Radius(tokenRadiusPill).Background(p.highlight)
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
	b := ui.ButtonBase(c).Height(tokenProductLibraryRow).Padding(8).Gap(12).Radius(tokenRadiusControl).Justify(ui.Start).Label(nativeItemLabel(item))
	a.collection = nativeCollectionSource(m, b, item)
	nativeTip(c, b, item.Title, p)
	if m.LibraryCached && !m.SignedIn {
		b.Disabled(true)
	}
	nativeTrackMenu(c, b, item, m, a, p)
	if b.Clicked() {
		if item.ID != "" {
			a.Play(item)
		} else {
			a.openCollection(m, item)
		}
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
		radius := float32(tokenRadiusArt)
		if item.Kind == "artist" {
			radius = 48 / 2
		}
		a.collectionArt(c, item, 48, radius, m, p)
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

// nativeContent draws the page and returns its scroll position, which the
// page's colour wash follows.
// nativeContent is the page, contentWidth wide: the frame computes it, so
// the page never lays out for the width it had a frame ago.
func nativeContent(c *ui.Context, m *UIModel, a ViewActions, p nativePalette, compact bool, contentWidth float32) *ui.ScrollState {
	inset := nativeContentInset(compact)
	scroll := ui.Scroll(c.Key("content-" + m.Route + "-" + m.Page.Title)).Grow(1).MinHeight(0).Label(a.Tr("shellContent"))
	a = nativeArtworkViewport(c, a, scroll)
	scrolled := a.viewportScroll
	// The page has no fixed header (its own scrolls away), so its top is
	// the panel's edge; its foot fades, as the library's beside it does.
	nativeScrollbar(c, scroll, scrolled, p, nativeFadeBelow(scrolled, p.surface))
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
	return scrolled
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
	if m.Route == "downloads" {
		nativeDownloads(c, m, a, p, compact)
		return
	}
	// A compact window has no library panel: its Library page leads with
	// the entry the panel would show.
	if m.Route == "library" && compact && nativeDownloadsShown(m, a) {
		nativeDownloadsEntry(c, m, a, p, true)
	}
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
	shelves := m.Page.Shelves
	for i := 0; i < len(shelves); i++ {
		shelf := shelves[i]
		if shelf.Title == "" {
			// Continuations append untitled shelves to the same track list:
			// a run of them is one table, numbered on, under one header.
			parts := [][]catalog.Item{shelf.Items}
			for i+1 < len(shelves) && shelves[i+1].Title == "" {
				i++
				parts = append(parts, shelves[i].Items)
			}
			nativeTrackTable(c.Key(fmt.Sprintf("shelf-%d", i+1-len(parts))), parts, m, a, p, compact, browse, 0)
			continue
		}
		ui.Column(c.Key(fmt.Sprintf("shelf-%d", i))).Gap(12).Children(func() {
			size := float32(tokenSizeH3)
			nativeDisplay(ui.Text(c, shelf.Title).FontSize(size).MaxLines(1).Ellipsis("…").Role(ui.RoleHeading), shelf.Title, size)
			switch {
			case m.Route == "search" && nativeSongShelf(shelf.Items):
				nativeTrackList(c, shelf.Items, m, a, p, compact, browse, 0)
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
				// Shelves hang their columns and cards from the top: a row centres
				// by default, which floats a short last column or a card with a
				// one-line subtitle away from its neighbours.
				shelfScroll := ui.ScrollHorizontal(c).Gap(16).PaddingY(4).Height(4*56 + 8).AlignItems(ui.Start)
				clipped := nativeShelfViewport(c, a, shelfScroll)
				nativeScrollbar(c, shelfScroll, clipped.viewportScroll, p, nil)
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
				shelfScroll := ui.ScrollHorizontal(c).PaddingY(4).Height(size + 112).MarginX(-12).AlignItems(ui.Start)
				clipped := nativeShelfViewport(c, a, shelfScroll)
				nativeScrollbar(c, shelfScroll, clipped.viewportScroll, p, nil)
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
	cover := m.openedCover
	if m.Route != "browse:"+cover.id {
		cover = nativeCollectionMatch{}
	}
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
				cover.art(nativeArtwork(c, m.Page.Artwork, min(96, contentWidth*0.25), tokenRadiusControl, a, p).Shrink(0))
			}
			ui.Column(c).Grow(1).MinWidth(0).Gap(8).Children(func() { title(24) })
		})
	} else if compact {
		ui.Column(c).Padding(8, inset, 0).Gap(12).Children(func() {
			if m.Page.Artwork != "" {
				nativeCollectionCover(cover.art(nativeArtwork(c, m.Page.Artwork, min(contentWidth-2*inset-64, 224), tokenRadiusControl, a, p).AlignSelf(ui.Center)), p)
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
				nativeCollectionCover(cover.art(nativeArtwork(c, m.Page.Artwork, art, tokenRadiusControl, a, p)), p)
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
	ui.Row(c).Padding(buttonTop, inset, 0).Gap(16).AlignItems(ui.Center).Children(func() {
		icon, key := "play", "shellPlay"
		if inPage && m.Playing {
			icon, key = "pause", "shellPause"
		}
		b := ui.ButtonBase(c).Size(buttonSize, buttonSize).Shrink(0).Radius(buttonSize / 2).Background(p.accent).TextColor(p.onAccent).Label(a.Tr(key) + " · " + m.Page.Title).Disabled(inPage && m.PlaybackLoading)
		nativeTip(c, b, a.Tr(key), p)
		nativePlayFeedback(b)
		b.Children(func() { ui.Icon(c, nativeIcons[icon]).Size(24, 24) })
		if b.Clicked() {
			if inPage {
				a.Toggle()
			} else {
				a.Play(items[0])
			}
		}
		nativeCollectionDownload(c, m, a, p, items)
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
	b := ui.ButtonBase(c).Height(tokenProductTile).MinWidth(0).PaddingEnd(12).Gap(12).Radius(tokenRadiusControl).Justify(ui.Start).Background(p.hover).Clip().Label(item.Title)
	a.collection = nativeCollectionSource(m, b, item)
	nativeTip(c, b, item.Title, p)
	hovered := b.Hovered()
	if hovered {
		b.Background(p.press)
	}
	b.Children(func() {
		// The art is flush with the tile's left side, so it takes the tile's
		// corners there itself: MyGo's GPU renderer rounds only the innermost
		// clip, and the art's own clip would square the tile's off.
		a.collectionArt(c, item, tokenProductTile, tokenRadiusControl, m, p).Radius(tokenRadiusControl, 0, 0, tokenRadiusControl)
		t := ui.Text(c, item.Title).Grow(1).MinWidth(0).FontSize(tokenSizeSmall).FontWeight(700).MaxLines(2).Ellipsis("…")
		if playing {
			t.TextColor(p.accentText)
			ui.Icon(c, nativeIcons["volume"]).Size(16, 16).TextColor(p.accent)
		} else if hovered && item.ID != "" {
			ui.Box(c).Size(32, 32).Shrink(0).Radius(tokenRadiusPill).Background(p.accent).TextColor(p.onAccent).Center().PassThrough().Children(func() { ui.Icon(c, nativeIcons["play"]).Size(16, 16) })
		}
	})
	nativeTrackMenu(c, b, item, m, a, p)
	if b.Clicked() {
		if item.ID != "" {
			a.Play(item)
		} else {
			a.openCollection(m, item)
		}
	}
}

func (a ViewActions) artworkPixels(size float32) int {
	scale := a.artworkScale
	if scale <= 0 {
		scale = 1
	}
	return int(math.Ceil(float64(size) * scale))
}

// The panel and transport share the playing cover, even before layout.
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
		artwork = a.Artwork(src, a.artworkPixels(size), artPlayer)
	} else if a.Artwork != nil && src != "" {
		bounds := box.Bounds()
		unmeasured := bounds.W == 0 || bounds.H == 0
		if unmeasured {
			// Bounds describe the previous layout; resolve a new cover on the
			// next frame, once we know whether its scroll containers reveal it.
			c.Invalidate()
		} else {
			viewport := a.artViewport
			if a.viewportScroll == nil {
				viewport.W, viewport.H = c.Size()
			}
			bounds.X -= a.artworkDX
			bounds.Y -= a.artworkDY
			visible := nativeIntersection(bounds, viewport)
			if visible.W > 0 && visible.H > 0 {
				artwork = a.Artwork(src, a.artworkPixels(size), priority)
			}
		}
		if artwork == nil && unmeasured && a.artworkPeek != nil {
			artwork = a.artworkPeek(src, a.artworkPixels(size))
		}
	}
	return box.Background(p.highlight).Children(func() {
		if artwork != nil && priority == artPlayer {
			// The playing cover: the next one fades in over it, keyed by
			// its image, as the track changes.
			ui.Image(c.Key(artwork), artwork).Fill().Fit(ui.Cover).Transition(nativeCrossfade)
		} else if artwork != nil {
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
	// Bounds come from the last layout. The parent scroll displacement
	// moves this viewport; its own displacement moves only its contents.
	bounds := scroller.Bounds()
	bounds.X -= a.artworkDX
	bounds.Y -= a.artworkDY
	if state.x != state.scroll.X || state.y != state.scroll.Y {
		dx := state.scroll.X - state.x
		if c.Direction() == ui.RightToLeft {
			dx = -dx
		}
		a.artworkDX += dx
		a.artworkDY += state.scroll.Y - state.y
		state.x, state.y = state.scroll.X, state.scroll.Y
		c.Invalidate()
	}
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
	a.collection = nativeCollectionSource(m, b, item)
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
			art := a.collectionArt(c, item, size, radius, m, p)
			if hovered {
				art.Shadow(tokenShadowArt.args(p))
			}
			if hovered && item.ID != "" {
				ui.Box(c).Absolute().Right(8).Bottom(8).Size(48, 48).Radius(tokenRadiusPill).Background(p.accent).TextColor(p.onAccent).Shadow(tokenShadowControl.args(p)).Center().PassThrough().Children(func() {
					ui.Icon(c, nativeIcons["play"]).Size(24, 24)
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
	nativeTrackMenu(c, b, item, m, a, p)
	if b.Clicked() {
		if item.ID != "" {
			a.Play(item)
		} else {
			a.openCollection(m, item)
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
	nativeTrackTable(c, [][]catalog.Item{items}, m, a, p, compact, numbered, offset)
}

// nativeTrackTable draws parts as one table, under one header: a long
// playlist's pages arrive as shelves of their own, but are one list.
func nativeTrackTable(c *ui.Context, parts [][]catalog.Item, m *UIModel, a ViewActions, p nativePalette, compact, numbered bool, offset int) {
	mode := nativeTrackRow
	if numbered {
		mode = nativeTrackNumbered
	}
	album := !compact && !nativeOneAlbum(parts...)
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
		j := 0
		for _, items := range parts {
			for _, item := range items {
				nativeTrack(c.Key(fmt.Sprintf("list-%d", j)), item, offset+j, m, a, p, mode, compact, album)
				j++
			}
		}
	})
}

// nativeOneAlbum reports whether every row comes from the same album, as on
// an album's own page, where an Album column would repeat its title.
func nativeOneAlbum(parts ...[]catalog.Item) bool {
	var first string
	for _, items := range parts {
		for _, item := range items {
			if first == "" {
				if first = item.Album; first == "" {
					return false
				}
			}
			if item.Album != first {
				return false
			}
		}
	}
	return first != ""
}

func nativeTrack(c *ui.Context, item catalog.Item, index int, m *UIModel, a ViewActions, p nativePalette, mode nativeTrackMode, compact, album bool) {
	playing := item.ID != "" && item.ID == m.Current.ID
	pad := float32(16)
	if compact {
		pad = 8
	}
	radius := float32(tokenRadiusControl)
	if mode == nativeTrackQueue {
		// The queue lies in the Now Playing card, which owns the corners: its
		// rows are flat bands across it, their art inset like the chips.
		pad, radius = 12, 0
	}
	b := ui.ButtonBase(c).Height(tokenProductRow).PaddingX(pad).Gap(16).Radius(radius).Justify(ui.Start).Label(nativeResultLabel(item, m, a))
	a.collection = nativeCollectionSource(m, b, item)
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
					ui.Icon(c, nativeIcons["play"]).Size(16, 16).TextColor(p.text)
				default:
					ui.Text(c, fmt.Sprint(index+1)).FontSize(tokenSizeSmall).TextColor(p.secondary)
				}
			})
		}
		ui.Row(c).Grow(1).Basis(0).MinWidth(0).Gap(12).AlignItems(ui.Center).Children(func() {
			radius := float32(tokenRadiusArt)
			if item.Kind == "artist" {
				radius = 40 / 2
			}
			a.collectionArt(c, item, 40, radius, m, p)
			ui.Column(c).Grow(1).MinWidth(0).Gap(4).Children(func() {
				t := ui.Text(c, item.Title).FontSize(tokenSizeSmall).FontWeight(500).MaxLines(1).Ellipsis("…")
				if playing {
					t.TextColor(p.accentText)
				}
				subtitle := item.Artist
				if mode == nativeTrackRow {
					subtitle = nativeResultSubtitle(item, m, a)
				}
				line := func() {
					artist := ui.Text(c, subtitle).FontSize(tokenSizeMeta).TextColor(p.secondary).MaxLines(1).Ellipsis("…")
					if hovered {
						artist.TextColor(p.text)
					}
				}
				if m.downloadStates[item.ID] != downloads.Complete {
					line()
					return
				}
				// A downloaded song says so before its artist, in the
				// glyph the collection's toggle fills with.
				ui.Row(c).MinWidth(0).Gap(4).AlignItems(ui.Center).Children(func() {
					mark := ui.Icon(c, nativeIcons["download-fill"]).Size(12, 12).Shrink(0).TextColor(p.secondary)
					if hovered {
						mark.TextColor(p.text)
					}
					line()
				})
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
	nativeTrackMenu(c, b, item, m, a, p)
	if b.Clicked() {
		if mode == nativeTrackQueue {
			a.PlayQueue(index)
		} else if item.ID != "" {
			a.Play(item)
		} else {
			a.openCollection(m, item)
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
			// The track toggles Now Playing, as the queue and lyrics buttons
			// do: a second click closes what the first opened.
			showing := m.Panel && !m.Immersive
			label := a.Tr("shellOpenNowPlaying")
			if showing {
				label = a.Tr("shellCloseNowPlaying")
			}
			info := ui.ButtonBase(c).MinWidth(0).Shrink(1).Height(64).Padding(4).Gap(12).Radius(tokenRadiusControl).Justify(ui.Start).Label(label)
			if info.Hovered() {
				info.Background(p.hover)
			}
			info.Children(func() {
				// Re-made as immersive mode comes or goes, so the cover leaving
				// it can shrink back into this one (nativeImmersiveCover).
				// Keyed by the morph, it is a new cover each way. Out of
				// immersive mode it is where the cover flies back to; into
				// it, the cover leaves from the old one, so the new one
				// takes no part (else, flying, it would claim the match).
				art := nativePlayerArtwork(c.Key(m.immersiveAt), m.Current.Artwork, 56, tokenRadiusArt, a, p)
				if !m.Immersive {
					art.Transition(nativeFlight(nativeMatch(nativeCoverMatch{"player-bar"})))
				}
				nativeTrackCaption(c, m, a, p)
			})
			if a.Download != nil && m.Current.ID != "" {
				nativeTrackMenu(c, info, m.Current, m, a, p)
			}
			if info.Clicked() {
				m.Panel = !showing
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
			nativeVolume(c, m, a, p, width > 1000, !nativeFloatingWanted(m, width))
			if width > 1000 {
				nativeMenuButton(c, &m.barMenu, "more", a.Tr("shellMore"), p, 32, m, m.Current.ID == "", func() []nativeMenuItem { return nativeTrackItems(m.Current, m, a) })
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
		ui.Text(c.Key("title-"+m.Current.ID), title).FontSize(tokenSizeSmall).FontWeight(600).MaxLines(1).Ellipsis("…").Transition(nativeTrackText)
		if m.PlaybackError != "" {
			artist = a.Tr("nativePlaybackFailedStatus")
			if !m.Panel && !m.Immersive {
				artist = m.PlaybackError
			}
		} else if nativeAwaitingAccount(m) {
			artist = a.Tr("nativePlaybackAwaitingAccount")
		}
		caption := ui.Text(c.Key("artist-"+m.Current.ID), artist).FontSize(tokenSizeMicro).TextColor(p.secondary).MaxLines(1).Ellipsis("…").Transition(nativeTrackText)
		if m.PlaybackError != "" {
			caption.TextColor(p.attention)
			nativeTip(c, caption, m.PlaybackError, p)
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
			card = p.raised.Mix(tint, nativeCardWash(p))
		}
		// Corners nest: card 16, the info button 12 four in, its art 8. Its
		// colour blends to the next cover's, as the washes do.
		ui.Column(c).Height(tokenProductMiniPlayer).Radius(tokenRadiusPanel).Background(card).Clip().Transition(ui.ElementTransition{Duration: tokenMotionSlow, Ease: tokenMotionEase, Colors: true}).Children(func() {
			ui.Row(c).Grow(1).PaddingX(4).Gap(4).AlignItems(ui.Center).Children(func() {
				info := ui.ButtonBase(c).Grow(1).MinWidth(0).Height(48).Padding(4).Gap(8).Justify(ui.Start).Radius(tokenRadiusControl).Label(a.Tr("shellOpenNowPlaying")).Children(func() {
					nativePlayerArtwork(c, m.Current.Artwork, 40, tokenRadiusArt, a, p)
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

func (m *UIModel) canTogglePlayback() bool {
	return m.Current.ID != "" || len(m.Queue) > 0
}

func nativePlayButton(c *ui.Context, m *UIModel, a ViewActions, p nativePalette, size float32) {
	if m.PlaybackLoading {
		ui.ButtonBase(c).Size(size, size).Shrink(0).Radius(size / 2).Background(p.text).TextColor(p.base).Disabled(true).Label(a.Tr("shellLoading")).Children(func() { ui.Spinner(c.Key("loading")).Size(size*0.5, size*0.5).Transition(nativeSwap) })
		return
	}
	icon, key := "play", "shellPlay"
	if m.Playing {
		icon, key = "pause", "shellPause"
	}
	b := ui.ButtonBase(c).Size(size, size).Shrink(0).Radius(size / 2).Label(a.Tr(key)).Background(p.text).TextColor(p.base).Disabled(!m.canTogglePlayback())
	nativeTip(c, b, a.Tr(key), p)
	nativePlayFeedback(b)
	b.Children(func() {
		ui.Icon(c.Key(icon), nativeIcons[icon]).Size(nativeGrid(size*0.5), nativeGrid(size*0.5)).Transition(nativeSwap)
	})
	if b.Clicked() {
		a.Toggle()
	}
}

type nativeSeekState struct {
	trackID                  string
	value                    float64
	pressed, dirty, canceled bool
	keyedAt                  time.Time // a key last moved it
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
		s := nativeSlider(c.Key("seek"), &state.value, max(1, m.Duration), p, true, nativeReadoutFresh(c, state.keyedAt)).Grow(1).Label(a.Tr("shellProgress")).Disabled(!enabled)
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
		// A key that moves it shows where it went, as dragging does.
		if changed && !pressed {
			state.keyedAt = c.Now()
		}
		if keyed := nativeReadoutFresh(c, state.keyedAt); enabled && !state.canceled && (keyed || s.Hovered() || pressed) {
			preview := state.value
			if !keyed && !pressed {
				x, _, _ := s.PointerPosition()
				preview = float64(nativeSliderFraction(s, x)) * m.Duration
			}
			nativeSliderReadout(c.Key("seek-readout"), s, p, nativeTime(math.Round(preview)), preview/m.Duration)
		}
		ui.Text(c, nativeTime(m.Duration)).Width(40).FontSize(tokenSizeMicro).TextColor(p.secondary)
	})
}

// nativeSliderThumb is the slider thumb's diameter; the track is inset by
// its radius so the thumb stays inside the control at either end.
const (
	nativeSliderThumb  = 12
	nativeSliderHeight = 24
)

// nativeReadoutLinger is how long a slider's value stays shown after a key
// moves it (the volume shortcuts, or the arrows on a focused slider).
const nativeReadoutLinger = time.Second

// nativeReadoutFresh reports whether a key moved a slider at keyed recently
// enough to still show its value, and asks for the frame that hides it.
func nativeReadoutFresh(c *ui.Context, keyed time.Time) bool {
	if keyed.IsZero() {
		return false
	}
	left := nativeReadoutLinger - c.Now().Sub(keyed)
	if left <= 0 {
		return false
	}
	c.After(left)
	return true
}

// nativeSliderFraction is where x, across the slider, falls on its track.
func nativeSliderFraction(s ui.Element, x float32) float32 {
	return max(0, min(1, (x-nativeSliderThumb/2)/max(1, s.LayoutBounds().W-nativeSliderThumb)))
}

// A playback slider (seek) turns coral under the pointer; others (volume)
// stay text, since coral means play. marked shows the thumb without the
// pointer or the focus, as when a shortcut moves it.
func nativeSlider(c *ui.Context, value *float64, maximum float64, p nativePalette, playback, marked bool) ui.Element {
	s := ui.SliderBase(c, value, 0, maximum).Height(nativeSliderHeight).PaddingX(nativeSliderThumb / 2)
	hovered := s.Hovered() || s.Focused() || s.Pressed()
	fraction := float32(max(0, min(1, *value/maximum)))
	s.Draw(func(painter *ui.Painter, r ui.Rect) {
		track := ui.Rect{X: r.X + nativeSliderThumb/2, Y: r.Y + r.H/2 - 2, W: max(0, r.W-nativeSliderThumb), H: 4}
		painter.Fill(track, p.track, 2)
		fill := track
		fill.W *= fraction
		col := p.text
		if hovered && playback {
			col = p.accent
		}
		painter.Fill(fill, col, 2)
		if hovered || marked {
			painter.Fill(ui.Rect{X: track.X + fill.W - nativeSliderThumb/2, Y: r.Y + r.H/2 - nativeSliderThumb/2, W: nativeSliderThumb, H: nativeSliderThumb}, p.text, nativeSliderThumb/2)
		}
	})
	return s
}

// nativeSliderReadout shows a slider's value over the thumb at fraction.
// It stays while pressed, including when a drag leaves the track, so it is
// a non-interactive overlay: ordinary tooltips dismiss on press. It is a
// tooltip by the guidebook (inverted, micro, art corners) and stands 8 off
// its anchor, the thumb, not off the slider's taller hit area.
func nativeSliderReadout(c *ui.Context, slider ui.Element, p nativePalette, text string, fraction float64) {
	x := nativeSliderThumb/2 + max(0, slider.LayoutBounds().W-nativeSliderThumb)*float32(max(0, min(1, fraction)))
	thumbTop := float32(nativeSliderHeight-nativeSliderThumb) / 2
	ui.Overlay(c, func() {
		nativeReadout(c, text, p).AttachTo(slider, ui.AnchorTopLeft, ui.AnchorBottom).Margin(0, 0, 8-thumbTop, x)
	})
}

func nativeReadout(c *ui.Context, text string, p nativePalette) ui.Element {
	return ui.Text(c, text).Padding(4, 8).Radius(tokenRadiusArt).Background(p.text).TextColor(p.base).FontSize(tokenSizeMicro).
		Shadow(tokenShadowTooltip.args(p)).PassThrough().Role(ui.RoleTooltip)
}

func nativeProgressLine(c *ui.Context, m *UIModel, p nativePalette) {
	value := float32(0)
	if m.Duration > 0 {
		value = float32(max(0, min(1, m.Position/m.Duration)))
	}
	// Inset by the card's corner: at the card's foot, nearer an edge, the
	// corner's curve would slice the line's ends.
	ui.Box(c).Height(4).Shrink(0).Margin(0, tokenRadiusPanel, 0).Radius(tokenRadiusPill).Background(p.track).Children(func() {
		ui.Box(c).Height(4).Radius(tokenRadiusPill).WidthPercent(value * 100).Background(p.text)
	})
}

// nativeVolume is the mute button and, with room, the volume slider.
// keyed shows the volume for a moment after the volume shortcuts change it,
// over the slider or, without one, the button; only one of the volume
// controls in view takes it.
func nativeVolume(c *ui.Context, m *UIModel, a ViewActions, p nativePalette, showSlider, keyed bool) {
	icon, key := "volume", "shellMute"
	if m.Muted || m.Volume == 0 {
		icon, key = "volume-off", "shellUnmute"
	} else if m.Volume < 0.5 {
		icon = "volume-low"
	}
	keyed = keyed && nativeReadoutFresh(c, m.volumeKeyedAt)
	value := fmt.Sprintf("%.0f%%", m.Volume*100)
	b := nativeIconButton(c, icon, a.Tr(key), p, false, 32)
	if b.Clicked() {
		a.Mute()
	}
	if !showSlider {
		if keyed {
			ui.Overlay(c.Key("volume-readout"), func() {
				nativeReadout(c, value, p).AttachTo(b, ui.AnchorTop, ui.AnchorBottom).Margin(0, 0, 8, 0)
			})
		}
		return
	}
	volume := m.Volume
	s := nativeSlider(c.Key("volume"), &volume, 1, p, false, keyed).Width(96).Label(a.Tr("shellVolume"))
	if s.Changed() {
		a.Volume(volume)
		if !s.Pressed() {
			m.volumeKeyedAt = c.Now()
		}
	}
	if keyed || s.Hovered() || s.Pressed() {
		nativeSliderReadout(c.Key("volume-readout"), s, p, fmt.Sprintf("%.0f%%", volume*100), volume)
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
		var scrolled *ui.ScrollState
		layer := nativePageLayer(c).Padding(16).Gap(gap)
		if tint, ok := nativePlayingTintOn(layer, m, a); ok {
			nativeScrollingWash(layer, tint, nativeWashStrength(p), nativeNowPlayingWashHeight, &scrolled)
		}
		layer.Children(func() { scrolled = nativeNowPlayingBody(c, m, a, p, width, overlay, revealLyrics, windowHeight, gap) })
	})
}

// nativeNowPlayingBody draws the panel and returns its scroll position when
// it scrolls (as a floating sheet), which the panel's wash follows.
func nativeNowPlayingBody(c *ui.Context, m *UIModel, a ViewActions, p nativePalette, width float32, overlay, revealLyrics bool, windowHeight, gap float32) *ui.ScrollState {
	{
		ui.Row(c).Height(32).Shrink(0).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, a.Tr("shellNowPlaying")).FontSize(tokenSizeBody).FontWeight(700).Grow(1).MinWidth(0).MaxLines(1).Ellipsis("…")
			// Immersive mode's one button is the player bar's. Docked, Now
			// Playing sits right above it, so it offers none of its own;
			// floating, the sheet covers the player bar (or there is none),
			// so it carries the way in. Either way the panel stays open, and
			// leaving immersive mode returns to it.
			if overlay && nativeIconButton(c, "expand", a.Tr("shellImmersive")+" · "+a.Tr("shellNowPlaying"), p, false, 32).Clicked() {
				m.Immersive = true
			}
			if nativeIconButton(c, "close", a.Tr("shellClosePanel"), p, false, 32).Clicked() {
				m.Panel = false
			}
		})
		if m.Current.ID == "" {
			ui.Column(c).Grow(1).Gap(16).AlignItems(ui.Center).Justify(ui.Center).Children(func() {
				nativeArtwork(c, "", 112, tokenRadiusControl, a, p)
				nativeDisplay(ui.Text(c, a.Tr("shellEmptyTitle")).FontSize(tokenSizeH3), a.Tr("shellEmptyTitle"), tokenSizeH3)
				ui.Text(c, a.TrArgs("shellEmptyBody", map[string]any{"shortcut": "⌘ K"})).TextColor(p.secondary).TextAlign(ui.Center)
			})
			return nil
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
			// A video is watched, not glanced at: it takes the panel's full
			// width whatever the window's height, the card the rest.
			if m.showsVideo() {
				artSize = width - 32
			}
			artSize = nativeGridFloor(artSize)
			// Lyrics first: the cover becomes a thumbnail beside the title and
			// the lyrics take the rest. A video keeps its size.
			lyricsFirst := m.LyricsExpanded && m.selectedPanelTab() == "lyrics" && !m.showsVideo()
			lyricsExtra := float32(0)
			// The title, the media switch and the card persist across the two
			// layouts and move between them; the cover shrinks into the
			// thumbnail and grows back out of it (nativeCoverMatch).
			cover := nativeMatch(nativeCoverMatch{"now-playing"})
			card := nativeMoves
			card.Match = nativeCardMatch{}
			if nativeImmersing(c, m) {
				cover, card = nativeFlight(cover), nativeFlight(card)
			}
			identity := func(art bool) {
				ui.Row(c.Key("now-playing-identity")).Shrink(0).Gap(12).AlignItems(ui.Center).Transition(nativeMoves).Children(func() {
					if art {
						nativePlayerArtwork(c, m.Current.Artwork, nativeCoverThumb, tokenRadiusArt, a, p).Transition(cover)
					}
					ui.Column(c.Key("now-playing-title")).Grow(1).MinWidth(0).Gap(4).Transition(nativeMoves).Children(func() {
						nativeDisplay(ui.Text(c.Key("title-"+m.Current.ID), m.Current.Title).FontSize(tokenSizeH3).LineHeight(1.2).MaxLines(2).Ellipsis("…").Transition(nativeTrackText), m.Current.Title, tokenSizeH3)
						ui.Text(c.Key("artist-"+m.Current.ID), m.Current.Artist).FontSize(tokenSizeSmall).TextColor(p.secondary).MaxLines(1).Ellipsis("…").Transition(nativeTrackText)
					})
					if nativeIconButton(c, nativeHeart(m.Liked), a.Tr(nativeLikeKey(m.Liked)), p, m.Liked, 36).Clicked() {
						a.Like()
					}
				})
			}
			// Only a track with a video has the choice.
			mediaSwitch := func() {
				if m.VideoAvailable {
					nativeMediaSwitch(c.Key("now-playing-switch"), m, a, p).Transition(nativeMoves)
				}
			}
			if lyricsFirst {
				lyricsExtra = max(0, artSize-nativeCoverThumb)
				identity(true)
				mediaSwitch()
			} else {
				// Cover or video, the media frame is one element: switching
				// changes its shape (square, 16:9) rather than replacing it.
				nativeMedia(c.Key("now-playing-media"), m, a, p, artSize).AlignSelf(ui.Center).Transition(cover)
				mediaSwitch()
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
				ui.Row(c).Shrink(0).AlignItems(ui.Center).Justify(ui.Center).Children(func() { nativeVolume(c, m, a, p, true, true) })
			}
			// Keyed: the layouts above it differ, and its reading position must
			// survive switching between them.
			panelCard := nativePanelCard(c.Key("now-playing-card"), m, a, p).Transition(card)
			if overlay {
				panelCard.Height(300 + lyricsExtra).Shrink(0)
				if revealLyrics {
					// On short windows the card begins below the fold. Reveal it
					// on entry only, so the reader can still scroll back to the cover.
					panelCard.ScrollIntoView()
				}
			} else {
				panelCard.Grow(1).MinHeight(120)
			}
		}
		if overlay {
			scroll := ui.Scroll(c).Grow(1).MinHeight(0).Gap(gap)
			a = nativeArtworkViewport(c, a, scroll)
			nativeScrollbar(c, scroll, a.viewportScroll, p, nativeFadeBelow(a.viewportScroll, p.surface))
			scroll.Children(body)
			return a.viewportScroll
		}
		body()
		return nil
	}
}

// nativePanelCard holds the queue and lyrics behind a pair of chips.
func nativePanelCard(c *ui.Context, m *UIModel, a ViewActions, p nativePalette) ui.Element {
	card := ui.Column(c).MinHeight(0).Padding(12).Gap(12).Radius(tokenRadiusControl).Background(p.raised).Clip()
	queue := m.selectedPanelTab() == "queue"
	if queue {
		// The queue's rows span the card, which clips them to its corners.
		card.Padding(12, 0)
	}
	return card.Children(func() {
		tabs := nativePanelTabs(c, m, a, p)
		if queue {
			tabs.PaddingX(12)
		}
		nativePanelBody(c, m, a, p).Transition(nativeSwap)
	})
}

func nativePanelTabs(c *ui.Context, m *UIModel, a ViewActions, p nativePalette) ui.Element {
	return ui.Row(c).Shrink(0).Gap(8).AlignItems(ui.Center).Children(func() {
		for _, tab := range []struct{ id, key string }{{"lyrics", "shellLyrics"}, {"queue", "shellQueue"}} {
			active := m.selectedPanelTab() == tab.id
			if nativeChip(c, a.Tr(tab.key), p, active).Label(a.Tr(tab.key) + " · " + a.Tr("shellNowPlaying")).Clicked() {
				m.PanelTab = tab.id
			}
		}
		if !m.Immersive && !m.showsVideo() && m.selectedPanelTab() == "lyrics" {
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
	nativeScrollbar(c, scroll, a.viewportScroll, p, nil)
	// As the lyrics beside it: rows past an edge fade, while more lies beyond.
	st := a.viewportScroll
	scroll.Effect(nativeEdgeFade{top: st.Y > 0.5, bottom: st.Y < st.MaxY-0.5})
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
	if m.VideoTheater {
		// The video fills the stage: as large as it fits under the header,
		// within the gutters, and nothing else.
		padX, padY := float32(nativeImmersiveGutter), float32(24)
		stageW := width - 2*tokenProductFrameGap - 2*padX
		if compact {
			padX, padY = 16, 16
			stageW = width - 2*padX
		}
		stageH := max(1, height-headerHeight-2*padY)
		size := min(stageW, stageH)
		if m.showsVideo() {
			size = min(stageW, stageH*16/9)
		}
		ui.Row(c).Grow(1).MinHeight(0).Padding(padY, padX).AlignItems(ui.Center).Justify(ui.Center).Children(func() {
			nativeMedia(c.Key("immersive-media"), m, a, p, nativeGridFloor(max(4, size))).Shadow(tokenShadowArt.args(p)).Transition(nativeFlight(nativeMatch(nativeImmersiveCover(c, m, width))))
		})
		return
	}
	if compact {
		scroll := ui.Scroll(c).Grow(1).Padding(16).Gap(24)
		a = nativeArtworkViewport(c, a, scroll)
		nativeScrollbar(c, scroll, a.viewportScroll, p, nil)
		scroll.Children(func() {
			nativePlaybackNotice(c, m, a, p)
			mediaHeight := max(float32(180), height-390)
			artSize := min(width-64, mediaHeight)
			if m.showsVideo() {
				// As on desktop, landscape video keeps the cover's height
				// budget and uses the extra width instead of shrinking to it.
				artSize = min(width-64, mediaHeight*16/9)
			}
			nativeMedia(c, m, a, p, artSize).AlignSelf(ui.Center)
			if m.VideoAvailable {
				nativeMediaSwitch(c, m, a, p)
			}
			nativeImmersiveTitle(c, m, a, p, true)
			nativePanelCard(c, m, a, p).Height(280)
		})
		return
	}
	panelWidth := min(float32(380), width*0.32)
	contentHeight := max(float32(0), height-headerHeight-48)
	// The media column and the card are one group, centred in the stage,
	// the card as tall as the column (from the cover's top to the artist's
	// line), so the two read as a pair rather than a cover adrift on the
	// left and a card pinned to the right edge. The column scrolls for
	// extra notices, so it is padded by the cover's shadow, which the
	// scroll would otherwise clip; the row's far side takes the same pad
	// so the group stays centred.
	pad := float32(tokenShadowArt.blur)
	room := max(float32(1), width-2*tokenProductFrameGap-2*nativeImmersiveGutter-nativeImmersivePair-panelWidth-2*pad)
	layout := nativeImmersiveLayout(m, a, room, contentHeight)
	ui.Row(c).Grow(1).MinHeight(0).Padding(24, nativeImmersiveGutter+pad, 24, nativeImmersiveGutter).AlignItems(ui.Center).Justify(ui.Center).Children(func() {
		scroll := ui.Scroll(c).Width(layout.column + 2*pad).Shrink(0).PaddingX(pad).MinHeight(0).HeightPercent(100).Label(a.Tr("shellContent"))
		// The queue card is a sibling of the media column, so it keeps the
		// outer viewport instead of the left column's clip.
		media := nativeArtworkViewport(c, a, scroll)
		nativeScrollbar(c, scroll, media.viewportScroll, p, nil)
		scroll.Children(func() {
			a := media
			ui.Column(c).MinHeight(contentHeight).Shrink(0).Gap(24).AlignItems(ui.Center).Justify(ui.Center).Children(func() {
				// It grows from the cover that was showing, and shrinks back into
				// the one that will; cover or video, it changes shape in place.
				nativeMedia(c.Key("immersive-media"), m, a, p, layout.media).Shadow(tokenShadowArt.args(p)).Transition(nativeFlight(nativeMatch(nativeImmersiveCover(c, m, width))))
				if m.VideoAvailable {
					nativeMediaSwitch(c, m, a, p).Transition(nativeFadeLong)
				}
				nativeImmersiveTitle(c, m, a, p, false)
				nativePlaybackNotice(c, m, a, p)
			})
		})
		card := nativeFlight(nativeMatch(nativeCardMatch{}))
		card.Exit = &ui.Motion{}
		nativePanelCard(c.Key("immersive-card"), m, a, p).Width(panelWidth).Height(layout.height).Shrink(0).Margin(0, 0, 0, nativeImmersivePair-pad).Background(p.pane).Transition(card)
	})
}

// nativeImmersivePair is the space between immersive mode's media column
// and its card.
const nativeImmersivePair = 64

type nativeImmersiveSizes struct {
	media  float32 // the media's width (a cover's side; a video is 16:9)
	column float32 // the column's width: the media's, or the title's minimum
	height float32 // the column's content, cover top to artist line: the card's height
}

// nativeImmersiveLayout fits the media column in room × height. The text
// has priority: the media takes whatever height is left, even below its
// usual size, as a fixed floor would push the artist under the player.
// The title wraps to the column, the wider of the media and the title's
// own width (to nativeImmersiveTitleMax), so they are sized together.
func nativeImmersiveLayout(m *UIModel, a ViewActions, room, height float32) nativeImmersiveSizes {
	var s nativeImmersiveSizes
	text := func(column float32) float32 {
		// The gap under the media, the switch and its gap, the title block
		// as the fonts actually lay it out, and any notice.
		h := 24 + nativeImmersiveTitleHeight(m, a, column)
		if m.VideoAvailable {
			h += 32 + 24
		}
		if m.PlaybackError != "" {
			h += 24 + 56
		}
		return h
	}
	// The column hugs the title's own width up to nativeImmersiveTitleMax:
	// the card keeps its distance from whichever is wider.
	title, artist := nativeImmersiveTitleText(m, a)
	titleW, _ := ui.ShapeText(title, nativeDisplayMetrics(tokenSizeH2, nativeDisplayLeading(title)), 0).Size()
	artistW, _ := ui.ShapeText(artist, ui.Font{Size: tokenSizeBody, Weight: 500}, 0).Size()
	natural := min(room, nativeImmersiveTitleMax, float32(math.Ceil(float64(max(titleW, artistW)))))
	s.media = room
	for range 3 {
		s.column = max(s.media, natural)
		media := max(1, height-text(s.column))
		s.media = min(room, media)
		if m.showsVideo() {
			s.media = min(room, media*16/9)
		}
		s.media = max(1, nativeGridFloor(s.media))
	}
	s.column = max(s.media, natural)
	shown := s.media
	if m.showsVideo() {
		shown = s.media * 9 / 16
	}
	s.height = min(height, max(nativeImmersiveCardMin, shown+text(s.column)))
	return s
}

const (
	nativeImmersiveTitleMax = 560 // the widest the title runs before it wraps: it may outreach the cover
	nativeImmersiveCardMin  = 320 // the card's least height: lyrics need lines
)

func nativeImmersiveTitle(c *ui.Context, m *UIModel, a ViewActions, p nativePalette, compact bool) {
	ui.Column(c).Gap(8).AlignItems(ui.Center).Transition(nativeFadeLong).Children(func() {
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
	flags                              [24]bool
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
		flags:  [24]bool{m.Loading, m.Playing, m.Muted, m.Liked, m.Shuffle, m.Panel, m.Immersive, m.Settings, m.Shortcuts, m.SidebarCollapsed, m.SignedIn, m.CanBack, m.CanForward, m.FocusSearch, m.Video, m.VideoLoading, m.PlaybackLoading, m.MusicPremium, m.LibraryCached, m.LibraryRefreshing, m.LyricsExpanded, m.VideoAvailable, m.VideoTheater, m.VideoFullScreen},
		values: [4]float64{m.Volume, m.Position, m.Duration, 0}, repeat: m.Repeat, queueLength: len(m.Queue), libraryLength: len(m.LibraryItems),
		errorScope: m.ErrorScope, libraryLoaded: m.LibraryLoaded, shortcutRecording: m.ShortcutRecording, shortcut: m.GlobalShortcut, shortcutError: m.ShortcutError,
		videoQuality: m.VideoQuality, videoQualityError: m.VideoQualityError,
	}
}

// nativeBackdrop washes the top of the page in colour: the blurred cover of a
// collection, the current track's cover in immersive mode, or a stable tint
// for pages without artwork.
func nativeBackdrop(c *ui.Context, m *UIModel, a ViewActions, p nativePalette) {
	// Immersive mode is the one place the cover itself fills the view, a
	// fixed ambience behind everything. Pages wash in nativePageWash.
	if !m.Immersive || m.Current.Artwork == "" || a.Artwork == nil {
		return
	}
	if artwork := a.Artwork(m.Current.Artwork, artworkEdge, artPlayer); artwork != nil {
		nativeCoverBlur(c, artwork).HeightPercent(25).Opacity(nativeWashStrength(p) * 0.7)
	}
}

// nativePageWash colours the top of a page: a collection in its cover's
// colour, other pages in their stable tint, a cover without colour not at
// all. The wash belongs to the page's header, so it scrolls away with it
// rather than staying under the list: it is painted beneath the page layer's
// children (the compact top bar included) and moved by the content's scroll
// position, read when it paints, so it never lags a frame behind.
func nativePageWash(layer ui.Element, m *UIModel, a ViewActions, p nativePalette, scrolled **ui.ScrollState) ui.Element {
	if m.Immersive {
		return layer
	}
	tint, ok := nativeHeaderTint(m.Route), true
	if m.Page.Artwork != "" {
		tint, ok = ui.Color{}, false
		if a.ArtworkTint != nil {
			tint, ok = a.ArtworkTint(m.Page.Artwork)
		}
	}
	if !ok {
		return layer
	}
	return nativeScrollingWash(layer, tint, nativeWashStrength(p), nativeWashHeight, scrolled)
}

// nativeScrollEdges marks a scrolling list under a fixed header, as the
// settings sheet does: a hairline along its top once content has scrolled
// beneath the header, and a fade at its bottom while more lies below. Drawn
// over the rows at paint time, from the scroll position then.
func nativeScrollEdges(c *ui.Context, scroll ui.Element, st *ui.ScrollState, p nativePalette, bg ui.Color) {
	below := nativeFadeBelow(st, bg)
	nativeScrollbar(c, scroll, st, p, func(painter *ui.Painter, r ui.Rect) {
		if st.Y > 0 {
			painter.Fill(ui.Rect{X: r.X, Y: r.Y, W: r.W, H: 1}, p.line, 0)
		}
		below(painter, r)
	})
}

// nativeFadeBelow fades a scroller's foot into bg, its panel's colour,
// while more lies below: rows reaching a panel's edge fade there rather
// than being cut. The library, the page and the floating Now Playing
// sheet, side by side or over each other, all end this way.
func nativeFadeBelow(st *ui.ScrollState, bg ui.Color) func(*ui.Painter, ui.Rect) {
	return func(painter *ui.Painter, r ui.Rect) {
		if st.Y < st.MaxY-1 {
			painter.FillGradient(ui.Rect{X: r.X, Y: r.Y + r.H - nativeFadeBelowSize, W: r.W, H: nativeFadeBelowSize}, ui.LinearGradient{From: bg.Alpha(0), To: bg, Angle: 180}, 0)
		}
	}
}

// nativeFadeBelowSize is how far up a panel's foot fades.
const nativeFadeBelowSize = 40

// nativeScrollbarIdle is how long a scroller's bar stays after the last
// scroll, or after the pointer comes over it, before fading.
const nativeScrollbarIdle = time.Second

// nativeScrollGrabs are the scrollbars held down; any release lets them go.
var nativeScrollGrabs []*nativeScrollbarState

// nativeReleaseScrollGrabs is the window root's watch for a release, which
// ends a scrollbar drag wherever the pointer went.
func nativeReleaseScrollGrabs(ev ui.InputEvent) {
	if ev.Kind == ui.InputPointerUp && ev.Button == 0 {
		for _, s := range nativeScrollGrabs {
			s.grabbed = false
		}
		nativeScrollGrabs = nativeScrollGrabs[:0]
	}
}

type nativeScrollbarState struct {
	x, y          float32
	over, gliding bool
	// grabbed holds from a press on the bar's strip until its release:
	// the thumb stays in its grabbed colour wherever the drag takes the
	// pointer, as a held thumb does.
	grabbed        bool
	active, hidden time.Time
	// keyTo is where the keys sent the scroller, while keyGlide holds: a
	// key held down heads on from there, not from where the glide is.
	keyTo    [2]float32
	keyGlide bool
}

// nativeScrollKeys are the keys that scroll pages in browsers, as MyGo's
// own handling has them; vertical says which axis each moves.
var nativeScrollKeys = []struct {
	key      ui.Key
	vertical bool
}{{ui.KeyDown, true}, {ui.KeyUp, true}, {ui.KeyPageDown, true}, {ui.KeyPageUp, true}, {ui.KeyHome, true}, {ui.KeyEnd, true}, {ui.KeyRight, false}, {ui.KeyLeft, false}}

// nativeKeyScroller is a scroller built in this frame that a scrolling key
// may move: whether it holds the focus or is under the pointer, and how it
// moves by a key, reporting whether it had room.
type nativeKeyScroller struct {
	focus, over bool
	scroll      func(key ui.Key) bool
}

// nativeKeyScrollers are this frame's scrollers, outermost first (a
// scroller is built before those inside it). nativeView sends the keys
// once all are built: to the innermost holding the focus, or else under
// the pointer, and outward while one has no room, as MyGo does. The keys
// are the window's, so a focused control that uses one (a radio group's
// Home, a menu button's Down, a text field) still gets it first.
var nativeKeyScrollers []nativeKeyScroller

// nativeScrollByKey sends key to this frame's scrollers, and reports
// whether one moved.
func nativeScrollByKey(key ui.Key) bool {
	for _, focus := range []bool{true, false} {
		for i := len(nativeKeyScrollers) - 1; i >= 0; i-- {
			k := nativeKeyScrollers[i]
			if (focus && k.focus || !focus && k.over) && k.scroll(key) {
				return true
			}
		}
		if focus && slices.ContainsFunc(nativeKeyScrollers, func(k nativeKeyScroller) bool { return k.focus }) {
			return false // the focus is in a scroller: the pointer's don't take its keys
		}
	}
	return false
}

// nativeKeyScroll glides st by key: a line (40), a page (all but a line,
// at least half) or to an end, over the base motion, as browsers smooth the
// keys. MyGo jumps at once; it reports whether st had room to move.
func nativeKeyScroll(st *ui.ScrollState, s *nativeScrollbarState, view ui.Rect, key ui.Key, now time.Time) bool {
	const line = 40
	x, y := st.X, st.Y
	if s.keyGlide && st.Scrolling() {
		x, y = s.keyTo[0], s.keyTo[1]
	}
	page := max(view.H-line, view.H/2)
	switch key {
	case ui.KeyDown:
		y += line
	case ui.KeyUp:
		y -= line
	case ui.KeyPageDown:
		y += page
	case ui.KeyPageUp:
		y -= page
	case ui.KeyHome:
		y = 0
	case ui.KeyEnd:
		y = st.MaxY
	case ui.KeyRight:
		x += line
	case ui.KeyLeft:
		x -= line
	}
	x, y = max(0, min(x, st.MaxX)), max(0, min(y, st.MaxY))
	from := [2]float32{st.X, st.Y}
	if s.keyGlide && st.Scrolling() {
		from = s.keyTo
	}
	// A glide can stop a hair off its end (0.00002 above the top): a key
	// that would only take up that slack hasn't scrolled.
	if math.Abs(float64(x-from[0])) < 0.5 && math.Abs(float64(y-from[1])) < 0.5 {
		return false
	}
	s.keyTo, s.keyGlide, s.active = [2]float32{x, y}, true, now
	st.ScrollTo(x, y, ui.Animation{Duration: tokenMotionBase, Ease: tokenMotionEase})
	return true
}

// nativeScrollbar draws a scroller's bars in place of MyGo's, which the
// theme makes transparent: MyGo shows them for as long as the pointer is
// anywhere over the scroller. These show as it scrolls and as the pointer
// comes over it, then fade once it rests, as macOS's overlay bars do; on
// the bar's own strip, where MyGo lets the pointer grab it, they stay and
// brighten. under paints beneath them in the same DrawOver (an element has
// one), as the scroll edges do.
func nativeScrollbar(c *ui.Context, scroll ui.Element, st *ui.ScrollState, p nativePalette, under func(*ui.Painter, ui.Rect), onKeyScroll ...func()) {
	s := ui.Local(scroll, "scrollbar", func() nativeScrollbarState { return nativeScrollbarState{} })
	now := c.Now()
	// The reader scrolling shows the bar; the app gliding it (ScrollTo,
	// to its last step) doesn't.
	if st.X != s.x || st.Y != s.y {
		if !st.Scrolling() && !s.gliding {
			s.active = now
		}
		s.x, s.y = st.X, st.Y
	}
	s.gliding = st.Scrolling()
	if !s.gliding {
		s.keyGlide = false
	}
	px, py, over := scroll.PointerPosition()
	if over && !s.over {
		s.active = now
	}
	s.over = over
	box := scroll.Bounds()
	// The scrolling keys glide, with the focus in the scroller (the
	// innermost one that moves that way takes them; a focused control that
	// uses a key keeps it) or, from nativeView, with the pointer over it.
	if focus := scroll.FocusWithin(); (focus || over) && (st.MaxX > 0.5 || st.MaxY > 0.5) {
		nativeKeyScrollers = append(nativeKeyScrollers, nativeKeyScroller{focus: focus, over: over, scroll: func(key ui.Key) bool {
			for _, k := range nativeScrollKeys {
				if k.key == key && (k.vertical && st.MaxY > 0.5 || !k.vertical && st.MaxX > 0.5) {
					moved := nativeKeyScroll(st, s, box, key, now)
					if moved && len(onKeyScroll) > 0 {
						onKeyScroll[0]()
					}
					return moved
				}
			}
			return false
		}})
	}
	strip := float32(nativeScrollbarWidth + 6) // MyGo's grab strip
	onStrip := func(x, y float32) bool {
		return x >= 0 && y >= 0 && x <= box.W && y <= box.H &&
			(st.MaxY > 0 && (c.Direction() == ui.RightToLeft && x <= strip || c.Direction() != ui.RightToLeft && x >= box.W-strip) ||
				st.MaxX > 0 && y >= box.H-strip)
	}
	onBar := over && onStrip(px, py)
	// MyGo's bar takes the press, so the scroller isn't Pressed: watch for
	// a press on the strip, and for its release anywhere in the window
	// (nativeView's root lets every grab go).
	scroll.ObserveInput(func(ev ui.InputEvent) {
		if ev.Kind == ui.InputPointerDown && ev.Button == 0 && onStrip(ev.X, ev.Y) {
			s.grabbed = true
			nativeScrollGrabs = append(nativeScrollGrabs, s)
		}
	})
	if s.grabbed {
		s.active = now
	}
	show := onBar || s.grabbed
	if idle := now.Sub(s.active); !onBar && idle < nativeScrollbarIdle {
		show = true
		c.After(nativeScrollbarIdle - idle)
	}
	// It appears at once and fades as it goes, as macOS's do. The fade is
	// timed here, not with Animate, so a frozen clock (as in tests) never
	// holds frames: only a fade under way asks for them.
	alpha := float32(1)
	if show {
		s.hidden = time.Time{}
	} else {
		if s.hidden.IsZero() {
			s.hidden = now
		}
		alpha = 1 - float32(now.Sub(s.hidden))/float32(tokenMotionBase)
		if s.active.IsZero() || alpha <= 0 {
			alpha = 0
		} else {
			c.AnimationFrame()
		}
	}
	thumb := p.track
	if onBar || s.grabbed {
		thumb = p.muted
	}
	rtl := c.Direction() == ui.RightToLeft
	scroll.DrawOver(func(painter *ui.Painter, r ui.Rect) {
		if under != nil {
			under(painter, r)
		}
		if alpha <= 0 {
			return
		}
		col := thumb.Alpha(float32(thumb.A) / 255 * alpha)
		for _, bar := range nativeScrollbarThumbs(r, st, rtl) {
			painter.Fill(bar, col, nativeScrollbarWidth/2)
		}
	})
}

// nativeScrollbarFollowed tells a scroller's bar that the app set it
// moving (lyrics following playback): the bar shows for the reader's
// scrolling, not for that, even where the move is at once (less motion).
func nativeScrollbarFollowed(scroll ui.Element, st *ui.ScrollState) {
	s := ui.Local(scroll, "scrollbar", func() nativeScrollbarState { return nativeScrollbarState{} })
	s.x, s.y, s.gliding, s.keyGlide = st.X, st.Y, true, false
}

// nativeScrollbarThumbs are the thumbs MyGo would draw (its scrollBars and
// scrollThumb), so what shows is where the pointer grabs.
func nativeScrollbarThumbs(r ui.Rect, st *ui.ScrollState, rtl bool) []ui.Rect {
	const w = nativeScrollbarWidth
	vertical, horizontal := st.MaxY > 0.5, st.MaxX > 0.5
	corner := float32(0)
	if vertical && horizontal {
		corner = w + 3
	}
	thumb := func(pos, track, view, content, offset float32) (float32, float32) {
		inner := track - 4
		length := min(max(inner*view/content, 24), inner)
		at := max(0, min((inner-length)*offset/(content-view), inner-length))
		return pos + 2 + at, length
	}
	var bars []ui.Rect
	if vertical {
		y, h := thumb(r.Y, max(r.H-corner, 0), r.H, r.H+st.MaxY, st.Y)
		x := r.X + r.W - w - 3
		if rtl {
			x = r.X + 3
		}
		bars = append(bars, ui.Rect{X: x, Y: y, W: w, H: h})
	}
	if horizontal {
		track := max(r.W-corner, 0)
		x, l := thumb(r.X, track, r.W, r.W+st.MaxX, st.X)
		if rtl {
			x = r.X + track - (x - r.X) - l
			if vertical {
				x += corner
			}
		}
		bars = append(bars, ui.Rect{X: x, Y: r.Y + r.H - w - 3, W: l, H: w})
	}
	return bars
}

// nativeScrollingWash paints a colour wash from the top of layer, beneath its
// children, moved up by the scroll position *scrolled holds once it is set
// (read when it paints, so it never lags a frame): a wash belongs to what
// it heads and scrolls away with it.
func nativeScrollingWash(layer ui.Element, tint ui.Color, strength, height float32, scrolled **ui.ScrollState) ui.Element {
	return layer.Draw(func(painter *ui.Painter, r ui.Rect) {
		y := float32(0)
		if *scrolled != nil {
			y = (*scrolled).Y
		}
		if y >= height {
			return
		}
		painter.FillGradient(ui.Rect{X: r.X, Y: r.Y - y, W: r.W, H: height}, ui.LinearGradient{From: nativeWashOf(tint, strength), To: tint.Alpha(0), Angle: 180}, 0)
	})
}

// nativeWashHeight is how far a page's colour wash runs down from its top.
const nativeWashHeight = 320

// nativeCoverBlur is a cover blurred across its parent, drawn from a
// quarter-size layer: give it a height (a quarter of the band it covers)
// and an opacity. Blur a small layer, then draw it at the final size without
// a transform: a transformed child makes MyGo allocate a full-size RGBA mask
// for the rounded parent clip, which would erase the smaller canvas's savings.
func nativeCoverBlur(c *ui.Context, artwork ui.ImageSource) ui.Element {
	layer := ui.Box(c).Absolute().Left(0).Top(0).WidthPercent(25).Effect(backdropLayer{}).PassThrough().Transition(nativeFadeLong)
	layer.Children(func() { ui.Image(c, artwork).Fill().Fit(ui.Cover).Blur(17.5).PassThrough() })
	return layer
}

// nativePlayingTintOn is nativePlayingTint as e shows it: a new cover's
// colour blends in over the slow motion as the track changes, and a cover
// without colour fades the wash away. It reports whether any shows.
func nativePlayingTintOn(e ui.Element, m *UIModel, a ViewActions) (ui.Color, bool) {
	target := ui.Color{}
	if tint, ok := nativePlayingTint(m, a); ok {
		target = tint
	}
	shown := e.AnimateColor("playing-tint", target, ui.Animation{Duration: tokenMotionSlow, Ease: tokenMotionEase})
	return shown, shown.A > 0
}

// nativeWashOf is tint at strength, the share of it a fading colour keeps.
func nativeWashOf(tint ui.Color, strength float32) ui.Color {
	return tint.Alpha(strength * float32(tint.A) / 255)
}

// nativePlayingTint is the colour the playing track's cover lends the
// interface, once the cover has loaded.
func nativePlayingTint(m *UIModel, a ViewActions) (ui.Color, bool) {
	if m.Current.ID == "" || m.Current.Artwork == "" || a.ArtworkTint == nil {
		return ui.Color{}, false
	}
	return a.ArtworkTint(m.Current.Artwork)
}

// nativeCardWash is how strongly a tint fills a card holding text (the mini
// player): weaker than a wash, so its secondary text keeps 4.5:1 whatever
// the cover's hue (TestNativeCardWashKeepsTextReadable).
func nativeCardWash(p nativePalette) float32 {
	if p.dark {
		return tokenWashCardDark
	}
	return tokenWashCardLight
}

// nativeCollectionCover lifts a collection's cover off its page: a soft
// shadow, and in light appearance a hairline, since a pale cover on a white
// page needs an edge a light shadow can't give.
func nativeCollectionCover(e ui.Element, p nativePalette) ui.Element {
	e.Shadow(tokenShadowArt.args(p))
	if !p.dark {
		e.Border(1, p.line)
	}
	return e
}

// nativeWashStrength is how strongly a tint colours the chrome: the
// guidebook's "the music leads", kept quiet enough that text stays legible.
func nativeWashStrength(p nativePalette) float32 {
	if p.dark {
		return tokenWashDark
	}
	return tokenWashLight
}

// nativeNowPlayingWashHeight is how far the Now Playing panel's wash, in
// the playing cover's colour, runs down from its top.
const nativeNowPlayingWashHeight = 360

// nativePlayerWash glows the playing cover's colour behind the player bar's
// track, fading out toward the transport: what's playing carries colour.
func nativePlayerWash(c *ui.Context, m *UIModel, a ViewActions, p nativePalette) {
	wash := ui.Box(c).Absolute().Left(0).Top(0).WidthPercent(100).HeightPercent(100).PassThrough()
	if tint, ok := nativePlayingTintOn(wash, m, a); ok {
		wash.LinearGradient(ui.LinearGradient{From: nativeWashOf(tint, nativeWashStrength(p)*tokenWashBar), To: tint.Alpha(0), Angle: 90, End: 0.4})
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

// nativeTrackMenu gives target an item's menu: a song's actions, or the way
// into a collection.
func nativeTrackMenu(c *ui.Context, target ui.Element, item catalog.Item, m *UIModel, a ViewActions, p nativePalette) {
	nativeContextMenu(c, target, m, p, func() []nativeMenuItem { return nativeTrackItems(item, m, a) })
}

func nativeTrackItems(item catalog.Item, m *UIModel, a ViewActions) []nativeMenuItem {
	if item.ID == "" {
		return []nativeMenuItem{{label: item.Title, run: func() { a.openCollection(m, item) }}}
	}
	items := []nativeMenuItem{
		{label: a.Tr("shellPlay"), run: func() { a.Play(item) }},
		{label: a.Tr("nativeAddToQueue"), disabled: a.QueueAppend == nil, run: func() { a.QueueAppend(item) }},
	}
	if a.Download != nil {
		switch m.downloadStates[item.ID] {
		case "":
			items = append(items, nativeMenuItem{label: a.Tr("nativeDownloadTrack"), run: func() { a.Download(item) }})
		case downloads.Complete:
			items = append(items, nativeMenuItem{label: a.Tr("nativeDownloadRemove"), run: func() { a.DownloadRemove(item.ID) }})
		default:
			items = append(items, nativeMenuItem{label: a.Tr("nativeShowDownloads"), run: func() { a.Navigate("downloads") }})
		}
	}
	return items
}

// nativeAccountItems is the account button's menu: who is signed in, then
// the account's places, then signing out.
func nativeAccountItems(m *UIModel, a ViewActions, status string) []nativeMenuItem {
	return []nativeMenuItem{
		{label: status, disabled: true},
		{label: a.Tr("shellAccountManage"), divider: true, run: a.Account},
		{label: a.Tr("shellSettings"), run: func() { m.Settings = true }},
		{label: a.Tr("shellSignOut"), divider: true, run: a.SignOut},
	}
}

func nativePlaybackShortcut(c *ui.Context, scope ui.Element, m *UIModel, a ViewActions) {
	if !m.canTogglePlayback() || m.Settings || m.Shortcuts || m.ShortcutRecording {
		return
	}
	// A window shortcut alone lets Space click the last focused song or
	// button. Bind its containing frame as well; text editors keep typing
	// priority and Enter still activates the focused control.
	focused := scope.Shortcut(0, ui.KeySpace)
	window := c.Shortcut(0, ui.KeySpace)
	if (focused || window) && !c.ShortcutRepeated() {
		a.Toggle()
	}
}

func (m *UIModel) focusSearch() {
	m.Immersive, m.Panel, m.FocusSearch = false, false, true
	// Drop the floating dialog immediately so the next keystroke goes to search.
	m.floating = nativeFloatState{}
}

func nativeKeyboard(c *ui.Context, m *UIModel, a ViewActions) {
	if m.ShortcutRecording {
		return
	}
	if c.Shortcut(ui.Cmd, ui.KeyK) {
		m.focusSearch()
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
		m.volumeKeyedAt = c.Now()
	}
	if c.Shortcut(ui.Cmd, ui.KeyDown) {
		a.Volume(max(0, m.Volume-0.05))
		m.volumeKeyedAt = c.Now()
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
		if m.VideoFullScreen {
			nativeSetFullScreen(m, a, false)
		} else if m.Shortcuts {
			m.Shortcuts = false
		} else if m.Settings {
			m.Settings = false
		} else if m.VideoTheater {
			nativeSetTheater(m, false)
		} else if m.Immersive {
			m.Immersive = false
		} else if m.Panel {
			m.Panel = false
		}
	}
}

func nativeMediaSwitch(c *ui.Context, m *UIModel, a ViewActions, p nativePalette) ui.Element {
	return ui.Row(c).Shrink(0).Gap(8).Wrap().AlignItems(ui.Center).AlignSelf(ui.Center).Children(func() {
		for _, option := range []struct {
			video bool
			key   string
		}{{false, "nativeArtwork"}, {true, "nativeVideo"}} {
			b := nativeChip(c, a.Tr(option.key), p, m.Video == option.video).MinWidth(80).Disabled(m.Current.ID == "" || a.Video == nil)
			if b.Clicked() {
				a.Video(option.video)
			}
		}
		if m.showsVideo() {
			nativeVideoQualityControl(c, m, a, p, 108, 32)
		}
	})
}

// showsVideo reports whether the media frame shows the video: chosen, and
// the track has one. Otherwise it is the cover, whatever the choice.
func (m *UIModel) showsVideo() bool { return m.Video && m.VideoAvailable }

func nativeMedia(c *ui.Context, m *UIModel, a ViewActions, p nativePalette, size float32) ui.Element {
	if !m.showsVideo() {
		if !m.VideoTheater {
			return nativePlayerArtwork(c, m.Current.Artwork, size, tokenRadiusControl, a, p)
		}
		// A song after a video, in the video's view: the way back stays.
		frame := ui.Box(c).Shrink(0)
		return frame.Children(func() {
			nativePlayerArtwork(c, m.Current.Artwork, size, tokenRadiusControl, a, p)
			nativeVideoControls(c, frame, m, a)
		})
	}
	// Video plays on a black stage in either appearance, so the stage takes
	// the dark palette.
	stage := palette(true)
	frame := ui.Box(c).Size(size, size*9/16).Shrink(0).Radius(tokenRadiusControl).Background(stage.base).Clip().Center().Label(a.Tr("nativeVideo"))
	return frame.Children(func() {
		nativeVideoPicture(c, m, a, size, size*9/16)
		nativeVideoControls(c, frame, m, a)
	})
}

// nativeVideoPicture is what the video stage shows in w × h: the frame, or
// its loading, failure or absence.
func nativeVideoPicture(c *ui.Context, m *UIModel, a ViewActions, w, h float32) {
	stage := palette(true)
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
		ui.Image(c, m.VideoFrame).Size(w, h).Fit(ui.Contain)
	} else {
		ui.Text(c, a.Tr("nativeVideoUnavailable")).Padding(16).TextAlign(ui.Center).TextColor(stage.secondary).FontSize(tokenSizeMeta)
	}
}

// nativeVideoControls sit in the media frame's corner: fill the play view
// with the video (a toggle, so active with a dot) and full screen. They
// show while the pointer is over the frame or the focus in it, and are
// always there to Tab to. Over a picture they wear the stage's veil to
// stay legible.
func nativeVideoControls(c *ui.Context, frame ui.Element, m *UIModel, a ViewActions) {
	stage := palette(true)
	row := ui.Row(c).Absolute().Right(8).Bottom(8).Gap(4)
	if !frame.Hovered() && !frame.FocusWithin() {
		row.Opacity(0)
	}
	row.Children(func() {
		label := "nativeVideoTheater"
		if m.VideoTheater {
			label = "nativeVideoTheaterExit"
		}
		theater := nativeToggleIcon(c, "theater", a.Tr(label), stage, m.VideoTheater, 32, stage.text)
		if !theater.Hovered() {
			theater.Background(stage.veil)
		}
		if theater.Clicked() {
			nativeSetTheater(m, !m.VideoTheater)
		}
		full := nativeToggleIcon(c, "fullscreen", a.Tr("nativeVideoFullScreen"), stage, false, 32, stage.text)
		if !full.Hovered() {
			full.Background(stage.veil)
		}
		if full.Clicked() {
			nativeSetFullScreen(m, a, true)
		}
	})
}

// nativeSetTheater fills the play view with the video, entering immersive
// mode to do it, or gives the view back: to the panel if that is where
// the video was.
func nativeSetTheater(m *UIModel, on bool) {
	if on {
		m.theaterFromPanel = !m.Immersive
		m.Immersive, m.VideoTheater = true, true
		return
	}
	if m.theaterFromPanel {
		m.Immersive = false
	}
	m.VideoTheater, m.theaterFromPanel = false, false
}

// nativeFadeAt is an opacity fading over the fast motion from from toward
// 1 (in) or 0 (out), elapsed into the fade.
func nativeFadeAt(from float32, in bool, elapsed time.Duration) float32 {
	k := min(1, float32(elapsed)/float32(tokenMotionFast))
	if in {
		return from + (1-from)*k
	}
	return from * (1 - k)
}

func nativeSetFullScreen(m *UIModel, a ViewActions, on bool) {
	if m.VideoFullScreen == on {
		return
	}
	m.VideoFullScreen = on
	if a.FullScreen != nil {
		a.FullScreen(on)
	}
}

// nativeFullScreenIdle is how long the full screen view keeps its controls
// (and the pointer) after the pointer last moved, while playing.
const nativeFullScreenIdle = 2500 * time.Millisecond

type nativeFullScreenState struct {
	x, y    float32
	active  time.Time
	shown   bool
	from    float32   // the controls' opacity when they last turned
	changed time.Time // when they last turned
}

// nativeFullScreenVideo is the window in full screen: the video, as large
// as it fits, on the black stage, and the transport over its foot while
// the pointer moves (or playback is paused).
func nativeFullScreenVideo(c *ui.Context, m *UIModel, a ViewActions, width, height float32) {
	stage := palette(true)
	root := ui.Box(c.Key("full-screen")).Fill().Background(stage.base).TextColor(stage.text).Center().Label(a.Tr("nativeVideo"))
	nativePlaybackShortcut(c, root, m, a)
	s := ui.Local(root, "pointer", func() nativeFullScreenState { return nativeFullScreenState{active: c.Now()} })
	now := c.Now()
	if x, y, over := root.PointerPosition(); over && (x != s.x || y != s.y) {
		s.x, s.y, s.active = x, y, now
	}
	show := !m.Playing || root.FocusWithin()
	if idle := now.Sub(s.active); idle < nativeFullScreenIdle {
		show = true
		c.After(nativeFullScreenIdle - idle)
	}
	if !show {
		root.Cursor(ui.CursorNone)
	}
	// The controls fade in and out (fast) rather than popping, timed here
	// so a frozen clock (tests) never holds frames.
	alpha := float32(0)
	if show {
		alpha = 1
	}
	if show != s.shown {
		from := s.from
		if !s.changed.IsZero() {
			from = nativeFadeAt(s.from, s.shown, now.Sub(s.changed))
		}
		s.shown, s.from, s.changed = show, from, now
	}
	if !s.changed.IsZero() && !c.Preferences().ReduceMotion && !m.instantMotion {
		alpha = nativeFadeAt(s.from, show, now.Sub(s.changed))
		if alpha != 0 && alpha != 1 {
			c.AnimationFrame()
		}
	}
	root.Children(func() {
		if m.showsVideo() {
			w := min(width, height*16/9)
			ui.Box(c).Size(w, w*9/16).Center().Children(func() { nativeVideoPicture(c, m, a, w, w*9/16) })
		} else {
			nativePlayerArtwork(c, m.Current.Artwork, nativeGridFloor(min(width, height)*0.7), tokenRadiusControl, a, stage)
		}
		if alpha == 0 {
			return
		}
		// A scrim fading up from the foot, not a band: the picture keeps its
		// edge, and the controls read over any frame.
		bar := ui.Column(c).Absolute().Left(0).Right(0).Bottom(0).Padding(80, 24, 16, 24).Gap(8).Opacity(alpha).
			LinearGradient(ui.LinearGradient{From: stage.veil.Alpha(0), To: stage.veil, Angle: 180, End: 0.6})
		bar.Children(func() {
			nativeSeek(c, m, a, stage)
			ui.Row(c).Gap(16).AlignItems(ui.Center).Children(func() {
				ui.Column(c).Grow(1).Basis(0).MinWidth(0).Gap(4).Children(func() {
					ui.Text(c, m.Current.Title).FontSize(tokenSizeBody).FontWeight(600).MaxLines(1).Ellipsis("…")
					ui.Text(c, m.Current.Artist).FontSize(tokenSizeSmall).TextColor(stage.secondary).MaxLines(1).Ellipsis("…")
				})
				if nativeIconButton(c, "previous", a.Tr("shellPrevious"), stage, false, 44).Disabled(m.Current.ID == "").Clicked() {
					a.Previous()
				}
				nativePlayButton(c, m, a, stage, 48)
				if nativeIconButton(c, "next", a.Tr("shellNext"), stage, false, 44).Disabled(m.Current.ID == "").Clicked() {
					a.Next()
				}
				ui.Row(c).Grow(1).Basis(0).MinWidth(0).Gap(4).AlignItems(ui.Center).Justify(ui.End).Children(func() {
					nativeVolume(c, m, a, stage, true, true)
					if nativeIconButton(c, "fullscreen-exit", a.Tr("nativeVideoExitFullScreen"), stage, false, 32).Clicked() {
						nativeSetFullScreen(m, a, false)
					}
				})
			})
		})
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
			nativeVolume(c, m, a, p, true, true)
			nativeMenuButton(c, &m.sheetMenu, "more", a.Tr("shellMore"), p, 32, m, m.Current.ID == "", func() []nativeMenuItem { return nativeTrackItems(m.Current, m, a) })
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
	return m.PlaybackLoading && m.PlaybackError == "" && nativeRestoring(m) && m.downloadStates[m.Current.ID] != downloads.Complete
}
