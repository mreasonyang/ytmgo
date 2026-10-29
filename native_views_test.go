package main

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"ytmgo/internal/catalog"
	"ytmgo/locales"
)

func nativeViewFixture() (*UIModel, ViewActions) {
	items := []catalog.Item{}
	for i, title := range []string{"Something About Us", "Instant Crush", "Get Lucky", "Digital Love", "One More Time", "Around the World", "Veridis Quo", "Within"} {
		items = append(items, catalog.Item{ID: fmt.Sprintf("song-%d", i), Title: title, Artist: "Daft Punk", Album: "Random Access Memories", Kind: "song", Artwork: fmt.Sprintf("art-%d", i), Duration: time.Duration(180+i*20) * time.Second})
	}
	albums := []catalog.Item{}
	for i, title := range []string{"Random Access Memories", "Discovery", "Homework", "Alive 2007", "Human After All"} {
		albums = append(albums, catalog.Item{Title: title, Artist: "Daft Punk", Kind: "album", BrowseID: fmt.Sprintf("album-%d", i), Artwork: fmt.Sprintf("art-%d", i)})
	}
	m := &UIModel{Route: "home", Appearance: "dark", Language: "en", Current: items[0], Queue: items, Playing: true, Duration: 240, Position: 48, Volume: 0.7, CanBack: true, Page: catalog.Page{Title: "Good evening", Shelves: []catalog.Shelf{{Title: "Listen again", Items: items}, {Title: "Albums for you", Items: albums}}, Items: items}, Lyrics: "It might not be the right time\nI might not be the right one"}
	colors := []color.RGBA{{28, 65, 84, 255}, {90, 50, 94, 255}, {164, 113, 49, 255}, {64, 85, 52, 255}, {117, 47, 63, 255}, {42, 84, 106, 255}, {76, 64, 101, 255}, {85, 94, 82, 255}}
	art := map[string]ui.ImageSource{}
	for i, col := range colors {
		pic := image.NewRGBA(image.Rect(0, 0, 80, 80))
		for y := 0; y < 80; y++ {
			for x := 0; x < 80; x++ {
				shade := float64(x+y+80) / 240
				pic.SetRGBA(x, y, color.RGBA{uint8(float64(col.R) * shade), uint8(float64(col.G) * shade), uint8(float64(col.B) * shade), 255})
			}
		}
		art[fmt.Sprintf("art-%d", i)] = ui.NewBitmap(pic)
	}
	noop := func() {}
	a := ViewActions{Tr: func(key string) string { return locales.Catalog.Format(m.Language, key, nil) }, TrArgs: func(key string, args map[string]any) string { return locales.Catalog.Format(m.Language, key, args) }, Navigate: func(route string) { m.Route = route }, Search: func(query string) { m.Route = "search"; m.Query = query }, Open: func(item catalog.Item) { m.Page = catalog.Page{Title: item.Title} }, Play: func(item catalog.Item) { m.Current = item; m.Playing = true }, QueueAppend: func(item catalog.Item) { m.Queue = append(m.Queue, item) }, PlayQueue: func(i int) { m.Current = m.Queue[i] }, Toggle: func() { m.Playing = !m.Playing }, Next: noop, Previous: noop, Like: func() { m.Liked = !m.Liked }, Shuffle: func() { m.Shuffle = !m.Shuffle }, Repeat: func() { m.Repeat = (m.Repeat + 1) % 3 }, Mute: func() { m.Muted = !m.Muted }, Back: noop, Forward: noop, Reload: noop, Account: noop, SignOut: noop, Seek: func(value float64) { m.Position = value }, Volume: func(value float64) { m.Volume = value }, Appearance: func(value string) { m.Appearance = value }, Language: func(value string) { m.Language = value }, Artwork: func(src string, _ int) ui.ImageSource { return art[src] }}
	a.Video = func(video bool) { m.Video = video }
	a.LoadMore = func() { m.Page.Continuation = "" }
	return m, a
}

func nativeSnapshot(t *testing.T, tt *ui.Tester, name string) {
	t.Helper()
	dir := os.Getenv("YTMGO_UI_SNAPSHOT_DIR")
	if dir == "" {
		return
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(filepath.Join(dir, name+".png"))
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(f, tt.Image()); err != nil {
		f.Close()
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
}

func nativeAssertVisible(t *testing.T, tt *ui.Tester, label string, width, height int) ui.Rect {
	t.Helper()
	r, ok := tt.Find(label)
	if !ok {
		t.Fatalf("control %q missing", label)
	}
	if r.W <= 0 || r.H <= 0 || r.X < 0 || r.Y < 0 || r.X+r.W > float32(width)+1 || r.Y+r.H > float32(height)+1 {
		t.Fatalf("control %q outside %dx%d: %+v", label, width, height, r)
	}
	return r
}

func TestNativeViewResponsive(t *testing.T) {
	for _, width := range []int{420, 760, 1100, 1440} {
		for _, appearance := range []string{"dark", "light"} {
			t.Run(fmt.Sprintf("%d-%s", width, appearance), func(t *testing.T) {
				m, a := nativeViewFixture()
				m.Appearance = appearance
				tt := ui.NewTester(func(c *ui.Context) { nativeView(c, m, a) }, width, 900)
				nativeAssertVisible(t, tt, a.Tr("shellSettings"), width, 900)
				nativeAssertVisible(t, tt, a.Tr("shellPause"), width, 900)
				nativeAssertVisible(t, tt, a.Tr("shellNavHome"), width, 900)
				nativeAssertVisible(t, tt, a.Tr("shellOpenNowPlaying"), width, 900)
				if !tt.HasText("Listen again") || !tt.HasText("Albums for you") {
					t.Fatalf("catalogue shelves missing: %v", tt.Texts())
				}
				// Flattened Page.Items must not be drawn again below its shelves.
				count := 0
				for _, text := range tt.Texts() {
					if text == "Digital Love" {
						count++
					}
				}
				if count != 1 {
					t.Fatalf("shelf song drawn %d times; want once", count)
				}
				nativeSnapshot(t, tt, fmt.Sprintf("%d-%s-home", width, appearance))
			})
		}
	}
}

func TestNativePlaybackAndSearchControls(t *testing.T) {
	m, a := nativeViewFixture()
	tt := ui.NewTester(func(c *ui.Context) { nativeView(c, m, a) }, 1440, 900)
	if err := tt.Click(a.Tr("shellPause")); err != nil {
		t.Fatal(err)
	}
	if m.Playing {
		t.Fatal("pause did not reach player action")
	}
	if err := tt.Click(a.Tr("shellPlay")); err != nil {
		t.Fatal(err)
	}
	if !m.Playing {
		t.Fatal("play did not reach player action")
	}
	if err := tt.Click(a.Tr("shellNavSearch")); err != nil {
		t.Fatal(err)
	}
	tt.Type("坂本龍一")
	tt.Key(0, ui.KeyEnter)
	if m.Query != "坂本龍一" || m.Route != "search" {
		t.Fatalf("native search submission: query=%q route=%q", m.Query, m.Route)
	}
	if err := tt.Click(a.Tr("shellProgress")); err != nil {
		t.Fatal(err)
	}
	tt.Key(0, ui.KeyEnd)
	if m.Position != m.Duration {
		t.Fatalf("seek keyboard value=%v duration=%v", m.Position, m.Duration)
	}
	if err := tt.Click(a.Tr("shellVolume")); err != nil {
		t.Fatal(err)
	}
	tt.Key(0, ui.KeyHome)
	if m.Volume != 0 {
		t.Fatalf("volume keyboard value=%v", m.Volume)
	}
}

func TestNativeSeekCommitsPointerOnceAndKeepsKeyboardTarget(t *testing.T) {
	m, a := nativeViewFixture()
	var seeks []float64
	// Match the asynchronous controller: a callback does not update the
	// playback snapshot immediately.
	a.Seek = func(value float64) { seeks = append(seeks, value) }
	tt := ui.NewTester(func(c *ui.Context) { nativeView(c, m, a) }, 1440, 900)
	r := nativeAssertVisible(t, tt, a.Tr("shellProgress"), 1440, 900)
	y := r.Y + r.H/2
	x := func(fraction float32) float32 { return r.X + 5 + (r.W-10)*fraction }
	tt.Press(x(0.25), y)
	for i := 3; i <= 7; i++ {
		tt.Move(x(float32(i)/10), y)
		tt.Frame()
	}
	if len(seeks) != 0 {
		t.Fatalf("drag preview rebuilt the decoder %d times", len(seeks))
	}
	// A normal playback poll must not move the thumb away from the drag.
	m.Position = 49
	tt.Frame()
	tt.Release(x(0.7), y)
	if len(seeks) != 1 || math.Abs(seeks[0]-m.Duration*0.7) > 0.1 {
		t.Fatalf("pointer release did not commit exactly its final target: %v", seeks)
	}
	if math.Abs(m.Position-seeks[0]) > 0.01 {
		t.Fatal("committed seek target is not displayed while the player catches up")
	}
	tt.Key(0, ui.KeyRight)
	if len(seeks) != 2 || math.Abs(seeks[1]-(seeks[0]+m.Duration/100)) > 0.1 {
		t.Fatalf("key after release used an old playback position: %v", seeks)
	}
	tt.Key(0, ui.KeyRight)
	if len(seeks) != 3 || seeks[2] <= seeks[1] {
		t.Fatalf("successive keyboard changes did not commit individually: %v", seeks)
	}
	// A click commits once too, followed by a key relative to that click.
	tt.Press(x(0.4), y)
	tt.Release(x(0.4), y)
	tt.Key(0, ui.KeyRight)
	if len(seeks) != 5 || math.Abs(seeks[3]-m.Duration*0.4) > 0.1 || seeks[4] <= seeks[3] {
		t.Fatalf("click then keyboard seek sequence is wrong: %v", seeks)
	}
}

func TestNativeSeekAbandonsPreviewWhenTrackChanges(t *testing.T) {
	m, a := nativeViewFixture()
	seeks := 0
	a.Seek = func(float64) { seeks++ }
	tt := ui.NewTester(func(c *ui.Context) { nativeView(c, m, a) }, 1100, 900)
	r := nativeAssertVisible(t, tt, a.Tr("shellProgress"), 1100, 900)
	x, y := r.X+r.W*0.7, r.Y+r.H/2
	tt.Press(x, y)
	m.Current = m.Queue[1]
	m.Position = 0
	tt.Frame()
	tt.Release(x, y)
	if seeks != 0 || m.Position != 0 {
		t.Fatalf("previous track's drag was committed to the next track: calls=%d position=%v", seeks, m.Position)
	}
	if err := tt.Click(a.Tr("shellProgress")); err != nil {
		t.Fatal(err)
	}
	if seeks != 1 {
		t.Fatal("new track could not seek after the old drag was canceled")
	}
}

func TestNativeSeekAbandonsPreviewWhenPlaybackDisables(t *testing.T) {
	m, a := nativeViewFixture()
	seeks := 0
	a.Seek = func(float64) { seeks++ }
	tt := ui.NewTester(func(c *ui.Context) { nativeView(c, m, a) }, 1100, 900)
	r := nativeAssertVisible(t, tt, a.Tr("shellProgress"), 1100, 900)
	x, y := r.X+r.W*0.7, r.Y+r.H/2
	tt.Press(x, y)
	m.PlaybackLoading = true
	tt.Frame()
	tt.Release(x, y)
	if seeks != 0 || m.Position != 48 {
		t.Fatalf("disabled playback committed an unfinished drag: calls=%d position=%v", seeks, m.Position)
	}
	m.PlaybackLoading = false
	tt.Frame()
	if err := tt.Click(a.Tr("shellProgress")); err != nil {
		t.Fatal(err)
	}
	if seeks != 1 {
		t.Fatal("seek stayed blocked after playback finished loading")
	}
}

func TestNativePanelAndImmersive(t *testing.T) {
	for _, width := range []int{420, 1100, 1440} {
		t.Run(fmt.Sprint(width), func(t *testing.T) {
			m, a := nativeViewFixture()
			tt := ui.NewTester(func(c *ui.Context) { nativeView(c, m, a) }, width, 900)
			if err := tt.Click(a.Tr("shellOpenNowPlaying")); err != nil {
				t.Fatal(err)
			}
			if !m.Panel {
				t.Fatal("artwork did not open now-playing")
			}
			nativeAssertVisible(t, tt, a.Tr("shellClosePanel"), width, 900)
			if err := tt.Click(a.Tr("shellLyrics") + " · " + a.Tr("shellNowPlaying")); err != nil {
				t.Fatal(err)
			}
			if m.PanelTab != "lyrics" {
				t.Fatal("lyrics tab did not switch")
			}
			if !tt.HasText(m.Lyrics) {
				t.Fatalf("lyrics missing: %v", tt.Texts())
			}
			nativeSnapshot(t, tt, fmt.Sprintf("%d-panel", width))
			if err := tt.Click(a.Tr("shellImmersive") + " · " + a.Tr("shellNowPlaying")); err != nil {
				t.Fatal(err)
			}
			if !m.Immersive {
				t.Fatal("immersive did not open")
			}
			nativeAssertVisible(t, tt, a.Tr("shellExitImmersive"), width, 900)
			nativeSnapshot(t, tt, fmt.Sprintf("%d-immersive", width))
			if err := tt.Click(a.Tr("shellExitImmersive")); err != nil {
				t.Fatal(err)
			}
			if m.Immersive {
				t.Fatal("immersive did not close")
			}
		})
	}
}

func TestNativeSettingsLocalizations(t *testing.T) {
	for _, locale := range []string{"en", "zh-Hans", "zh-Hant", "ja"} {
		t.Run(locale, func(t *testing.T) {
			m, a := nativeViewFixture()
			m.Language = locale
			tt := ui.NewTester(func(c *ui.Context) { nativeView(c, m, a) }, 420, 900)
			if err := tt.Click(a.Tr("shellSettings")); err != nil {
				t.Fatal(err)
			}
			nativeAssertVisible(t, tt, a.Tr("shellClose"), 420, 900)
			nativeAssertVisible(t, tt, a.Tr("shellThemeLight"), 420, 900)
			if err := tt.Click(a.Tr("shellThemeLight")); err != nil {
				t.Fatal(err)
			}
			if m.Appearance != "light" {
				t.Fatal("appearance action missing")
			}
			nativeSnapshot(t, tt, "420-settings-"+locale)
			if err := tt.Click("日本語"); err != nil {
				t.Fatal(err)
			}
			if m.Language != "ja" {
				t.Fatal("language action missing")
			}
			if !tt.HasText(locales.Catalog.Format("ja", "shellSettings", nil)) {
				t.Fatalf("locale changed without refreshing text: %v", tt.Texts())
			}
			tt.Key(0, ui.KeyEscape)
			if m.Settings {
				t.Fatal("Escape did not dismiss settings")
			}
		})
	}
}

func TestNativeIconsAndPalette(t *testing.T) {
	for name, icon := range nativeIcons {
		if icon == nil {
			t.Fatalf("icon %q failed to parse", name)
		}
	}
	if len(nativeIcons) < 30 {
		t.Fatalf("icon registry incomplete: %d", len(nativeIcons))
	}
	for _, dark := range []bool{true, false} {
		theme := nativeTheme(dark)
		if theme.Dark != dark {
			t.Fatal("native theme changed appearance")
		}
		if theme.Accent != palette(dark).accent {
			t.Fatal("original accent palette changed")
		}
	}
	if nativeSongShelf([]catalog.Item{{Kind: "album", BrowseID: "album"}}) {
		t.Fatal("album shelf identified as songs")
	}
	if !strings.Contains(nativeIconSources["settings"], "stroke-width=\"1.8\"") {
		t.Fatal("original icon stroke changed")
	}
}

func TestNativeKeyboardAndContextMenu(t *testing.T) {
	m, a := nativeViewFixture()
	tt := ui.NewTester(func(c *ui.Context) { nativeView(c, m, a) }, 1440, 900)
	tt.Key(ui.Cmd, ui.KeyK)
	tt.Type("Miles")
	tt.TypeKey(0, ui.KeySpace, " ")
	tt.Type("Davis")
	if m.Query != "Miles Davis" || !m.Playing {
		t.Fatalf("typing Space interfered with playback: query=%q playing=%v", m.Query, m.Playing)
	}
	tt.Key(0, ui.KeyEnter)
	if m.Route != "search" {
		t.Fatal("keyboard search did not submit")
	}
	tt.ClickAt(100, 700)
	tt.Key(0, ui.KeySpace)
	if m.Playing {
		t.Fatal("Space did not pause from canvas")
	}
	tt.Key(ui.Cmd|ui.Shift, ui.KeyF)
	if !m.Immersive {
		t.Fatal("Cmd-Shift-F did not enter immersive")
	}
	tt.Key(0, ui.KeyEscape)
	if m.Immersive {
		t.Fatal("Escape did not exit immersive")
	}
	tt.Key(ui.Cmd, ui.KeyJ)
	if !m.Panel {
		t.Fatal("Cmd-J did not open panel")
	}
	tt.Key(0, ui.KeyEscape)
	if m.Panel {
		t.Fatal("Escape did not close panel")
	}
	tt.Key(ui.Cmd, ui.KeyL)
	if m.Route != "library" {
		t.Fatal("Cmd-L did not navigate to library")
	}
	tt.Key(ui.Cmd|ui.Shift, ui.KeyH)
	if m.Route != "home" {
		t.Fatal("Cmd-Shift-H did not navigate home")
	}
	before := len(m.Queue)
	if err := tt.RightClick("Instant Crush · Daft Punk"); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem(a.Tr("nativeAddToQueue")); err != nil {
		t.Fatal(err)
	}
	if len(m.Queue) != before+1 || m.Queue[len(m.Queue)-1].Title != "Instant Crush" {
		t.Fatal("native context menu did not append selected track")
	}
}

func TestNativeMinimumWindow(t *testing.T) {
	for _, width := range []int{380, 420, 760, 1100, 1440} {
		t.Run(fmt.Sprint(width), func(t *testing.T) {
			m, a := nativeViewFixture()
			tt := ui.NewTester(func(c *ui.Context) { nativeView(c, m, a) }, width, 640)
			nativeAssertVisible(t, tt, a.Tr("shellPause"), width, 640)
			if err := tt.Click(a.Tr("shellOpenNowPlaying")); err != nil {
				t.Fatal(err)
			}
			nativeAssertVisible(t, tt, a.Tr("shellClosePanel"), width, 640)
			nativeSnapshot(t, tt, fmt.Sprintf("%d-short-panel", width))
			if err := tt.Click(a.Tr("shellImmersive") + " · " + a.Tr("shellNowPlaying")); err != nil {
				t.Fatal(err)
			}
			nativeAssertVisible(t, tt, a.Tr("shellExitImmersive"), width, 640)
			if width <= 760 {
				for _, key := range []string{"shellPrevious", "shellNext", "shellProgress", "shellVolume", "shellNavHome"} {
					nativeAssertVisible(t, tt, a.Tr(key), width, 640)
				}
			}
			nativeSnapshot(t, tt, fmt.Sprintf("%d-short-immersive", width))
		})
	}
}

func TestNativeVideoAndContinuation(t *testing.T) {
	m, a := nativeViewFixture()
	m.Immersive = true
	frame := image.NewRGBA(image.Rect(0, 0, 160, 90))
	for y := 0; y < 90; y++ {
		for x := 0; x < 160; x++ {
			frame.SetRGBA(x, y, color.RGBA{uint8(40 + x), uint8(40 + y), 120, 255})
		}
	}
	m.VideoFrame = ui.NewBitmap(frame)
	tt := ui.NewTester(func(c *ui.Context) { nativeView(c, m, a) }, 1440, 900)
	if err := tt.Click(a.Tr("nativeVideo")); err != nil {
		t.Fatal(err)
	}
	if !m.Video {
		t.Fatal("video switch did not reach action")
	}
	nativeSnapshot(t, tt, "1440-native-video")
	if err := tt.Click(a.Tr("nativeArtwork")); err != nil {
		t.Fatal(err)
	}
	if m.Video {
		t.Fatal("artwork switch did not reach action")
	}
	m.Immersive = false
	m.Page = catalog.Page{Title: "Results", Items: m.Queue[:1], Continuation: "local-page-handle"}
	tt.Frame()
	if err := tt.Click(a.Tr("nativeLoadMore")); err != nil {
		t.Fatal(err)
	}
	if m.Page.Continuation != "" {
		t.Fatal("continuation button did not request next page")
	}
}

func TestNativePendingPlayback(t *testing.T) {
	m, a := nativeViewFixture()
	m.PlaybackLoading = true
	tt := ui.NewTester(func(c *ui.Context) { nativeView(c, m, a) }, 420, 900)
	nativeAssertVisible(t, tt, a.Tr("shellLoading"), 420, 900)
	if err := tt.Click(a.Tr("shellNavSearch")); err != nil {
		t.Fatal(err)
	}
	tt.Type("search while loading")
	if m.Query != "search while loading" {
		t.Fatal("playback loading blocked browsing")
	}
	m.PlaybackLoading = false
	tt.Frame()
	nativeAssertVisible(t, tt, a.Tr("shellPause"), 420, 900)
	if tt.FramePending() {
		t.Fatal("native view keeps requesting frames when no state changes")
	}
	m.PlaybackLoading = true
	seekCalls := 0
	a.Seek = func(float64) { seekCalls++ }
	desktop := ui.NewTester(func(c *ui.Context) { nativeView(c, m, a) }, 1100, 900)
	_ = desktop.Click(a.Tr("shellProgress"))
	desktop.Key(0, ui.KeyEnd)
	if seekCalls != 0 {
		t.Fatal("pending playback accepted a seek before the track loaded")
	}
}

func TestNativeLibrarySidebar(t *testing.T) {
	for _, width := range []int{1100, 1440} {
		t.Run(fmt.Sprint(width), func(t *testing.T) {
			m, a := nativeViewFixture()
			m.SignedIn = true
			m.LibraryItems = []catalog.Item{{Title: "Weekend mixes", Artist: "Your playlist", BrowseID: "library-weekend", Kind: "playlist", Artwork: "art-1"}}
			tt := ui.NewTester(func(c *ui.Context) { nativeView(c, m, a) }, width, 900)
			nativeAssertVisible(t, tt, "Weekend mixes · Your playlist", width, 900)
			if err := tt.Click("Weekend mixes · Your playlist"); err != nil {
				t.Fatal(err)
			}
			if m.Page.Title != "Weekend mixes" {
				t.Fatal("library sidebar did not open selected playlist")
			}
		})
	}
}

func TestNativeTrackListLayout(t *testing.T) {
	for _, route := range []string{"search", "browse:album"} {
		t.Run(route, func(t *testing.T) {
			m, a := nativeViewFixture()
			m.Route = route
			m.Page = catalog.Page{Title: "Discovery", Items: m.Queue, Shelves: []catalog.Shelf{{Items: m.Queue}}}
			tt := ui.NewTester(func(c *ui.Context) { nativeView(c, m, a) }, 1100, 900)
			first := nativeAssertVisible(t, tt, "Something About Us · Daft Punk", 1100, 900)
			fifth := nativeAssertVisible(t, tt, "One More Time · Daft Punk", 1100, 900)
			if fifth.Y <= first.Y || fifth.X != first.X {
				t.Fatalf("playlist/search rows did not retain vertical order: first=%+v fifth=%+v", first, fifth)
			}
		})
	}
}

func TestNativePlaybackFailureRetry(t *testing.T) {
	for _, width := range []int{420, 1440} {
		for _, immersive := range []bool{false, true} {
			t.Run(fmt.Sprintf("%d-%v", width, immersive), func(t *testing.T) {
				m, a := nativeViewFixture()
				m.Playing = false
				m.PlaybackError = "Playback could not start"
				m.Immersive = immersive
				a.RetryPlayback = func() { m.PlaybackError = ""; m.PlaybackLoading = true }
				tt := ui.NewTester(func(c *ui.Context) { nativeView(c, m, a) }, width, 900)
				if !tt.HasText(m.PlaybackError) {
					t.Fatal("playback failure has no visible explanation")
				}
				if err := tt.Click(a.Tr("buttonRetry")); err != nil {
					t.Fatal(err)
				}
				if m.PlaybackError != "" || !m.PlaybackLoading {
					t.Fatal("retry did not restart failed playback")
				}
			})
		}
	}
}

func TestNativeArtworkOnlyRequestsVisibleCovers(t *testing.T) {
	m, a := nativeViewFixture()
	m.Current = catalog.Item{}
	m.Queue = nil
	m.Page = catalog.Page{Title: "Long playlist"}
	for i := range 300 {
		m.Page.Items = append(m.Page.Items, catalog.Item{ID: fmt.Sprint(i), Title: fmt.Sprintf("Track %d", i), Artwork: fmt.Sprintf("cover-%d", i)})
	}
	requested := map[string]int{}
	a.Artwork = func(src string, edge int) ui.ImageSource { requested[src] = edge; return nil }
	tt := ui.NewTester(func(c *ui.Context) { nativeView(c, m, a) }, 1100, 900)
	if len(requested) == 0 || len(requested) > 16 {
		t.Fatalf("initial view requested offscreen artwork: %d covers", len(requested))
	}
	for _, edge := range requested {
		if edge > 128 {
			t.Fatalf("small track artwork requested oversized pixels: %d", edge)
		}
	}
	if _, exists := requested["cover-100"]; exists {
		t.Fatal("offscreen track requested artwork before scrolling")
	}
	clear(requested)
	content, ok := tt.Find(a.Tr("shellContent"))
	if !ok {
		t.Fatal("content scroller missing")
	}
	first, ok := tt.Find("Track 0")
	second, ok2 := tt.Find("Track 1")
	if !ok || !ok2 {
		t.Fatal("track rows missing")
	}
	// Bring row 100 to the middle of the page, whatever the row pitch.
	tt.Scroll(content.X+content.W/2, content.Y+content.H/2, 0, first.Y+100*(second.Y-first.Y)-content.Y-content.H/2)
	if _, exists := requested["cover-100"]; !exists {
		t.Fatalf("newly visible cover not requested after scrolling: %v", requested)
	}
	if len(requested) > 32 {
		t.Fatalf("scroll requested too many offscreen covers: %d", len(requested))
	}
}

func TestNativeArtworkClipsHorizontalShelves(t *testing.T) {
	m, a := nativeViewFixture()
	m.Current = catalog.Item{}
	m.Queue = nil
	items := make([]catalog.Item, 100)
	for i := range items {
		items[i] = catalog.Item{BrowseID: fmt.Sprint(i), Title: fmt.Sprintf("Album %d", i), Artwork: fmt.Sprintf("album-cover-%d", i)}
	}
	m.Page = catalog.Page{Title: "Explore", Shelves: []catalog.Shelf{{Title: "Albums", Items: items}}}
	requested := map[string]int{}
	a.Artwork = func(src string, edge int) ui.ImageSource { requested[src] = edge; return nil }
	tt := ui.NewTester(func(c *ui.Context) { nativeView(c, m, a) }, 1100, 900)
	if len(requested) == 0 || len(requested) > 7 {
		t.Fatalf("horizontal shelf requested hidden albums: %d covers", len(requested))
	}
	for _, edge := range requested {
		if edge > 384 || edge <= 128 {
			t.Fatalf("album cover is not sized for its Retina display area: %d", edge)
		}
	}
	clear(requested)
	album, ok := tt.Find("Album 0")
	if !ok {
		t.Fatal("album card missing")
	}
	tt.Scroll(album.X+album.W/2, album.Y+album.H/2, 1960, 0)
	if _, exists := requested["album-cover-10"]; !exists {
		t.Fatalf("horizontal scroll did not request newly visible artwork: %v", requested)
	}
	if len(requested) > 14 {
		t.Fatalf("horizontal scroll requested hidden covers: %d", len(requested))
	}
}

func TestNativeAccountStates(t *testing.T) {
	for _, state := range []struct {
		name              string
		signedIn, premium bool
		status            string
	}{{"signed-out", false, false, "shellAccountSignedOut"}, {"standard", true, false, "shellAccountStandard"}, {"premium", true, true, "shellAccountPremium"}} {
		t.Run(state.name, func(t *testing.T) {
			m, a := nativeViewFixture()
			m.SignedIn, m.MusicPremium = state.signedIn, state.premium
			if state.signedIn {
				m.LibraryItems = []catalog.Item{
					{Title: "Weekend mixes", Artist: "Your playlist", BrowseID: "library-weekend", Kind: "playlist", Artwork: "art-1"},
					{Title: "Discovery", Artist: "Daft Punk", BrowseID: "album-1", Kind: "album", Artwork: "art-2"},
					{Title: "Daft Punk", BrowseID: "artist-1", Kind: "artist", Artwork: "art-3"},
				}
			}
			accounts, signOuts := 0, 0
			a.Account = func() { accounts++ }
			a.SignOut = func() { signOuts++ }
			tt := ui.NewTester(func(c *ui.Context) { nativeView(c, m, a) }, 1440, 900)
			label := a.Tr("menuAccount") + " · " + a.Tr(state.status)
			if !state.signedIn {
				if _, exists := tt.Find(label); exists {
					t.Fatal("signed-out window shows an account avatar")
				}
				if !tt.HasText(a.Tr("shellLibraryEmptyTitle")) {
					t.Fatal("signed-out library has no sign-in prompt")
				}
				if err := tt.Click(a.Tr("shellSignIn")); err != nil {
					t.Fatal(err)
				}
				if accounts != 1 {
					t.Fatal("sign in did not reach the account action")
				}
				a.Navigate("library")
				tt.Frame()
				if !tt.HasText(a.Tr("shellLibraryEmptyBody")) {
					t.Fatal("library route did not require sign-in")
				}
				nativeSnapshot(t, tt, "1440-account-"+state.name)
				return
			}
			if _, exists := tt.Find(a.Tr("shellSignIn")); exists {
				t.Fatal("signed-in window still offers sign-in")
			}
			nativeAssertVisible(t, tt, label, 1440, 900)
			if tt.HasText(a.Tr("shellPremiumBadge")) != state.premium {
				t.Fatalf("Premium badge shown=%v for premium=%v", !state.premium, state.premium)
			}
			// Signed-in Home leads with the library's quick-access tiles.
			nativeAssertVisible(t, tt, "Weekend mixes", 1440, 900)
			nativeAssertVisible(t, tt, a.Tr("shellFilterAlbums"), 1440, 900)
			if err := tt.Click(a.Tr("shellFilterAlbums")); err != nil {
				t.Fatal(err)
			}
			if _, exists := tt.Find("Weekend mixes · Your playlist"); exists {
				t.Fatal("album filter kept a playlist")
			}
			nativeAssertVisible(t, tt, "Discovery · Daft Punk", 1440, 900)
			nativeSnapshot(t, tt, "1440-account-"+state.name)
			if err := tt.Click(a.Tr("shellFilterClear")); err != nil {
				t.Fatal(err)
			}
			nativeAssertVisible(t, tt, "Weekend mixes · Your playlist", 1440, 900)
			if err := tt.Click(label); err != nil {
				t.Fatal(err)
			}
			if err := tt.ChooseMenuItem(a.Tr("shellAccountManage")); err != nil {
				t.Fatalf("%v: %v", err, tt.Menu())
			}
			if err := tt.Click(label); err != nil {
				t.Fatal(err)
			}
			if err := tt.ChooseMenuItem(a.Tr("shellSignOut")); err != nil {
				t.Fatal(err)
			}
			if accounts != 1 || signOuts != 1 {
				t.Fatalf("account menu actions: manage=%d sign out=%d", accounts, signOuts)
			}
			if err := tt.Click(a.Tr("shellSettings")); err != nil {
				t.Fatal(err)
			}
			if !tt.HasText(a.Tr(state.status)) {
				t.Fatal("settings do not state the account plan")
			}
		})
	}
}

func TestNativeCollectionPage(t *testing.T) {
	for _, width := range []int{420, 1100, 1440} {
		t.Run(fmt.Sprint(width), func(t *testing.T) {
			m, a := nativeViewFixture()
			m.Route = "browse:album-0"
			m.Current = catalog.Item{}
			m.Playing = false
			m.Page = catalog.Page{Title: "Random Access Memories", Subtitle: "Album · Daft Punk · 2013", Artwork: "art-0", Items: m.Queue, Shelves: []catalog.Shelf{{Items: m.Queue}}}
			tt := ui.NewTester(func(c *ui.Context) { nativeView(c, m, a) }, width, 900)
			play := a.Tr("shellPlay") + " · " + m.Page.Title
			nativeAssertVisible(t, tt, play, width, 900)
			if width > 760 && !tt.HasText(a.Tr("shellColumnAlbum")) {
				t.Fatal("track table has no column headings")
			}
			nativeSnapshot(t, tt, fmt.Sprintf("%d-collection", width))
			if err := tt.Click(play); err != nil {
				t.Fatal(err)
			}
			if m.Current.ID != m.Queue[0].ID {
				t.Fatal("collection play did not start the first track")
			}
			tt.Frame()
			if err := tt.Click(a.Tr("shellPause") + " · " + m.Page.Title); err != nil {
				t.Fatal(err)
			}
			if m.Playing {
				t.Fatal("collection button did not pause its own playback")
			}
		})
	}
}
