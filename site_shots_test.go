package main

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"ytmgo/internal/catalog"
	"ytmgo/locales"
)

// The website shows the real interface. Its catalogue is fictional: invented
// artists, titles and lyrics with generated covers, so the screenshots show
// nobody's music or artwork.

var siteSongs = []struct{ title, artist, album string }{
	{"Slow Orbit", "Lumen Drift", "Low Light Hours"},
	{"Paper Satellites", "Nadia Kerr", "Paper Satellites"},
	{"Neon Tide", "Harbor Lights Collective", "Neon Tide"},
	{"Afterglow Avenue", "Lumen Drift", "Low Light Hours"},
	{"Glass Garden", "Velvet Static", "Glass Garden"},
	{"Midnight Ferry", "Mika Sorensen", "Northern Lines"},
	{"Static Bloom", "Velvet Static", "Glass Garden"},
	{"Northern Lines", "Mika Sorensen", "Northern Lines"},
	{"Quiet Engines", "Tidal Bloom", "Quiet Engines"},
	{"Copper Sky", "Nadia Kerr", "Paper Satellites"},
}

var siteAlbums = []struct{ title, artist string }{
	{"Low Light Hours", "Lumen Drift"},
	{"Glass Garden", "Velvet Static"},
	{"Northern Lines", "Mika Sorensen"},
	{"Paper Satellites", "Nadia Kerr"},
	{"Quiet Engines", "Tidal Bloom"},
	{"Neon Tide", "Harbor Lights Collective"},
}

var siteLyrics = []string{
	"City lights are humming low",
	"Every window keeps a glow",
	"I can hear the river turn",
	"Carry home the things we learn",
	"Hold the night a little longer",
	"Let the quiet make us stronger",
	"Slow orbit, slow orbit",
	"Round the edges of the morning",
	"We were drifting out of time",
	"Counting satellites in line",
	"Every signal sounds like you",
	"Every static fades to blue",
	"Slow orbit, slow orbit",
	"Till the morning finds us home",
}

// Shelf and page labels come from YouTube Music in the viewer's language.
var siteCopy = map[string]map[string]string{
	"en":      {"listen": "Listen again", "albums": "Albums for you", "mixes": "Your mixes", "album": "Album", "playlist": "Playlist", "artist": "Artist", "liked": "Liked music", "mix": "Night Drive Mix", "focus": "Deep Focus"},
	"zh-Hans": {"listen": "再听一遍", "albums": "为你推荐的专辑", "mixes": "你的合辑", "album": "专辑", "playlist": "播放列表", "artist": "艺人", "liked": "喜欢的音乐", "mix": "夜间驾驶合辑", "focus": "专注时刻"},
	"zh-Hant": {"listen": "再聽一次", "albums": "為你推薦的專輯", "mixes": "你的合輯", "album": "專輯", "playlist": "播放清單", "artist": "藝人", "liked": "喜歡的音樂", "mix": "夜間駕駛合輯", "focus": "專注時刻"},
	"ja":      {"listen": "もう一度聴く", "albums": "おすすめのアルバム", "mixes": "あなたのミックス", "album": "アルバム", "playlist": "プレイリスト", "artist": "アーティスト", "liked": "高評価した曲", "mix": "ナイトドライブ ミックス", "focus": "ディープフォーカス"},
}

var sitePalettes = [][3]color.RGBA{
	{{255, 94, 79, 255}, {66, 20, 80, 255}, {255, 196, 120, 255}},
	{{40, 120, 220, 255}, {10, 20, 60, 255}, {120, 230, 255, 255}},
	{{30, 160, 130, 255}, {8, 40, 46, 255}, {220, 255, 180, 255}},
	{{150, 80, 220, 255}, {30, 12, 60, 255}, {255, 140, 220, 255}},
	{{240, 170, 50, 255}, {90, 30, 20, 255}, {255, 236, 170, 255}},
	{{220, 60, 120, 255}, {30, 14, 40, 255}, {90, 200, 255, 255}},
	{{70, 90, 110, 255}, {14, 18, 26, 255}, {240, 120, 90, 255}},
	{{100, 200, 90, 255}, {12, 40, 30, 255}, {250, 240, 120, 255}},
	{{230, 110, 60, 255}, {40, 20, 60, 255}, {120, 200, 220, 255}},
	{{60, 70, 200, 255}, {240, 110, 140, 255}, {255, 230, 200, 255}},
}

func siteMix(a, b color.RGBA, t float64) color.RGBA {
	t = max(0, min(1, t))
	l := func(x, y uint8) uint8 { return uint8(float64(x) + (float64(y)-float64(x))*t) }
	return color.RGBA{l(a.R, b.R), l(a.G, b.G), l(a.B, b.B), 255}
}

// siteCover draws an abstract cover: a diagonal wash, a glowing orb and,
// on alternate seeds, rings or horizon lines.
func siteCover(seed int) *image.RGBA {
	const n = 384
	pal := sitePalettes[seed%len(sitePalettes)]
	img := image.NewRGBA(image.Rect(0, 0, n, n))
	cx := 0.3 + 0.4*math.Mod(float64(seed)*0.37, 1)
	cy := 0.3 + 0.4*math.Mod(float64(seed)*0.61, 1)
	r := 0.22 + 0.12*math.Mod(float64(seed)*0.29, 1)
	for y := 0; y < n; y++ {
		for x := 0; x < n; x++ {
			u, v := float64(x)/n, float64(y)/n
			c := siteMix(pal[1], pal[0], 0.15+0.85*(u*0.4+(1-v)*0.6))
			d := math.Hypot(u-cx, v-cy)
			if d < r {
				c = siteMix(c, pal[2], 0.85*(1-d/r)+0.15)
			} else {
				c = siteMix(c, pal[2], 0.35*math.Exp(-(d-r)*9))
			}
			switch seed % 3 {
			case 0:
				if math.Abs(math.Mod(d*28, 2)-1) < 0.06 && d > r {
					c = siteMix(c, pal[2], 0.3)
				}
			case 1:
				if v > 0.62 && math.Mod(v*40, 2) < 0.5 {
					c = siteMix(c, pal[1], 0.45)
				}
			}
			img.SetRGBA(x, y, c)
		}
	}
	return img
}

func siteFixture(locale string, dark bool) (*UIModel, ViewActions) {
	m, a := nativeViewFixture()
	copy := siteCopy[locale]
	art := map[string]ui.ImageSource{}
	for i := range sitePalettes {
		art[fmt.Sprintf("site-%d", i)] = ui.NewBitmap(siteCover(i))
	}
	avatar := image.NewRGBA(image.Rect(0, 0, 96, 96))
	for y := 0; y < 96; y++ {
		for x := 0; x < 96; x++ {
			avatar.SetRGBA(x, y, siteMix(color.RGBA{255, 140, 90, 255}, color.RGBA{180, 60, 160, 255}, float64(x+y)/192))
		}
	}
	art["site-avatar"] = ui.NewBitmap(avatar)
	a.Artwork = func(src string, _ int, _ artworkPriority) ui.ImageSource { return art[src] }

	var songs []catalog.Item
	for i, s := range siteSongs {
		songs = append(songs, catalog.Item{ID: fmt.Sprintf("site-song-%d", i), Title: s.title, Artist: s.artist, Album: s.album, Kind: "song", Artwork: fmt.Sprintf("site-%d", i), Duration: time.Duration(168+i*23) * time.Second})
	}
	var albums []catalog.Item
	for i, al := range siteAlbums {
		albums = append(albums, catalog.Item{Title: al.title, Artist: al.artist, Kind: "album", BrowseID: fmt.Sprintf("site-album-%d", i), Artwork: fmt.Sprintf("site-%d", (i*3+1)%len(sitePalettes))})
	}
	mixes := []catalog.Item{
		{Title: copy["mix"], Kind: "playlist", BrowseID: "site-mix", Artwork: "site-5"},
		{Title: copy["focus"], Kind: "playlist", BrowseID: "site-focus", Artwork: "site-2"},
		{Title: "Lumen Drift", Kind: "artist", BrowseID: "site-artist", Artwork: "site-0"},
		{Title: "Velvet Static", Kind: "artist", BrowseID: "site-artist-2", Artwork: "site-4"},
	}
	library := []catalog.Item{{Title: copy["liked"], Kind: "playlist", BrowseID: "site-liked", Artwork: "site-9"}}
	library = append(library, mixes...)
	library = append(library, albums[:4]...)

	var lines []catalog.LyricLine
	for i, text := range siteLyrics {
		lines = append(lines, catalog.LyricLine{Text: text, Start: time.Duration(i*5+4) * time.Second, End: time.Duration(i*5+8) * time.Second})
	}
	appearance := "light"
	if dark {
		appearance = "dark"
	}
	*m = UIModel{
		Route: "home", Appearance: appearance, Language: locale, SignedIn: true, AccountAvatar: "site-avatar",
		Current: songs[0], Queue: songs, Playing: true, Duration: songs[0].Duration.Seconds(), Position: 36.5, Volume: 0.72,
		CanBack: true, LibraryItems: library, LibraryLoaded: true,
		Page: catalog.Page{Title: locales.Catalog.Format(locale, "shellGreetingEvening", nil), Shelves: []catalog.Shelf{
			{Title: copy["listen"], Items: songs[:8]},
			{Title: copy["albums"], Items: albums},
			{Title: copy["mixes"], Items: mixes},
		}},
		// Lyrics open on first playback; the shots choose their own views.
		playbackViewSeen: true,
		PanelTab:         "lyrics", LyricLines: lines, LyricsAvailable: true, Lyrics: siteLyrics[0],
	}
	a.LyricsClock = func() (float64, bool) { return m.Position, true }
	return m, a
}

func TestSiteScreenshots(t *testing.T) {
	dir := os.Getenv("YTMGO_SITE_SHOTS_DIR")
	if dir == "" {
		t.Skip("set YTMGO_SITE_SHOTS_DIR to render the website screenshots")
	}
	shots := []struct {
		name          string
		width, height int
		dark          bool
		setup         func(m *UIModel)
	}{
		{"home", 1440, 900, true, func(m *UIModel) {}},
		{"immersive", 1440, 900, true, func(m *UIModel) { m.Immersive = true }},
		{"album", 1440, 900, true, func(m *UIModel) {
			m.Route = "browse:site-album-0"
			m.Page = catalog.Page{Title: "Low Light Hours", Creator: "Lumen Drift", Subtitle: siteCopy[m.Language]["album"] + " • Lumen Drift • 2026", Artwork: "site-1", Items: m.Queue}
		}},
		{"light", 1440, 900, false, func(m *UIModel) {}},
		{"compact", 420, 860, true, func(m *UIModel) {}},
	}
	for _, locale := range []string{"en", "zh-Hans", "zh-Hant", "ja"} {
		for _, shot := range shots {
			m, a := siteFixture(locale, shot.dark)
			shot.setup(m)
			tt := ui.NewTester(func(c *ui.Context) { nativeView(c, m, a) }, shot.width, shot.height)
			tt.SetScale(2)
			// Shaping follows the locale, so CJK text takes that language's fonts.
			tt.SetLocale(locale)
			// Let the lyrics scroll settle on the current line.
			for range 20 {
				tt.Advance(100 * time.Millisecond)
				tt.Frame()
			}
			path := filepath.Join(dir, locale, shot.name+".png")
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				t.Fatal(err)
			}
			f, err := os.Create(path)
			if err != nil {
				t.Fatal(err)
			}
			if err := png.Encode(f, tt.Image()); err != nil {
				t.Fatal(err)
			}
			if err := f.Close(); err != nil {
				t.Fatal(err)
			}
		}
	}
}
