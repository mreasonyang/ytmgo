package main

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"ytmgo/internal/audioquality"
	"ytmgo/internal/catalog"
	"ytmgo/internal/downloads"
	"ytmgo/locales"
)

// The website shows the real interface. Its catalogue is fictional: invented
// artists, titles and lyrics with covers drawn in code (site_covers_test.go),
// so the screenshots show nobody's music or artwork.

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

// The album page shows Low Light Hours' own tracklist.
var siteLowLightHours = []struct {
	title string
	secs  int
}{
	{"Slow Orbit", 168}, {"Afterglow Avenue", 237}, {"Amber Hour", 197}, {"Sodium Lamps", 243},
	{"Long Exposure", 226}, {"Window Seat", 189}, {"Harbour Clock", 251}, {"Last Train West", 302},
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

func siteMix(a, b color.RGBA, t float64) color.RGBA {
	t = max(0, min(1, t))
	l := func(x, y uint8) uint8 { return uint8(float64(x) + (float64(y)-float64(x))*t) }
	return color.RGBA{l(a.R, b.R), l(a.G, b.G), l(a.B, b.B), 255}
}

func siteFixture(locale string, dark bool) (*UIModel, ViewActions) {
	m, a := nativeViewFixture()
	copy := siteCopy[locale]
	art := map[string]ui.ImageSource{}
	tints := map[string]ui.Color{}
	for key, paint := range siteCovers {
		cover := siteRender(paint)
		art[key] = ui.NewBitmap(cover)
		if tint, ok := artworkTint(cover); ok {
			tints[key] = tint
		}
	}
	avatar := image.NewRGBA(image.Rect(0, 0, 96, 96))
	for y := 0; y < 96; y++ {
		for x := 0; x < 96; x++ {
			avatar.SetRGBA(x, y, siteMix(color.RGBA{255, 140, 90, 255}, color.RGBA{180, 60, 160, 255}, float64(x+y)/192))
		}
	}
	art["site-avatar"] = ui.NewBitmap(avatar)
	a.Artwork = func(src string, _ int, _ artworkPriority) ui.ImageSource { return art[src] }
	a.ArtworkTint = func(src string) (ui.Color, bool) { tint, ok := tints[src]; return tint, ok }

	var songs []catalog.Item
	for i, s := range siteSongs {
		songs = append(songs, catalog.Item{ID: fmt.Sprintf("site-song-%d", i), Title: s.title, Artist: s.artist, Album: s.album, Kind: "song", Artwork: siteAlbumCovers[s.album], Duration: time.Duration(168+i*23) * time.Second})
	}
	var albums []catalog.Item
	for i, al := range siteAlbums {
		albums = append(albums, catalog.Item{Title: al.title, Artist: al.artist, Kind: "album", BrowseID: fmt.Sprintf("MPREsite-album-%d", i), Artwork: siteAlbumCovers[al.title]})
	}
	mixes := []catalog.Item{
		{Title: copy["mix"], Kind: "playlist", BrowseID: "site-mix", Artwork: "cover-night-drive"},
		{Title: copy["focus"], Kind: "playlist", BrowseID: "site-focus", Artwork: "cover-deep-focus"},
		{Title: "Lumen Drift", Kind: "artist", BrowseID: "site-artist", Artwork: "artist-lumen-drift"},
		{Title: "Velvet Static", Kind: "artist", BrowseID: "site-artist-2", Artwork: "artist-velvet-static"},
	}
	library := []catalog.Item{{Title: copy["liked"], Kind: "playlist", BrowseID: "site-liked", Artwork: "cover-liked"}}
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
		Page: catalog.Page{Title: phrases(locales.Catalog.Format(locale, "shellGreetingEvening", nil)), Shelves: []catalog.Shelf{
			{Title: copy["listen"], Items: songs[:8]},
			{Title: copy["albums"], Items: albums},
			{Title: copy["mixes"], Items: mixes},
		}},
		// Lyrics open on first playback; the shots choose their own views.
		playbackViewSeen: true,
		PanelTab:         "lyrics", LyricLines: lines, LyricsAvailable: true, Lyrics: siteLyrics[0],
	}
	a.LyricsClock = func() (float64, bool) { return m.Position, true }
	// A few songs are downloaded: the library shows its Downloads entry, and
	// an album not yet downloaded offers to be.
	a.Download, a.DownloadCollection = func(catalog.Item) {}, func() {}
	a.DownloadPause, a.DownloadRemove = func(string, bool) {}, func(string) {}
	m.downloadStates = map[string]downloads.State{}
	for i, n := range []int{1, 4, 5} {
		size := int64(4_200_000 + i*900_000)
		m.Downloads = append(m.Downloads, downloads.Entry{Item: songs[n], State: downloads.Complete, Quality: audioquality.Best, Bytes: size, Total: size})
		m.downloadStates[songs[n].ID] = downloads.Complete
	}
	return m, a
}

func siteAlbumTracks() []catalog.Item {
	var out []catalog.Item
	for i, s := range siteLowLightHours {
		id := fmt.Sprintf("site-llh-%d", i)
		if i == 0 {
			id = "site-song-0" // the playing song, so the page marks it
		}
		out = append(out, catalog.Item{ID: id, Title: s.title, Artist: "Lumen Drift", Album: "Low Light Hours", Kind: "song", Artwork: "cover-low-light-hours", Duration: time.Duration(s.secs) * time.Second})
	}
	return out
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
		// The section is about video too: its track has one, so the
		// Cover/Video switch shows (the app offers it only then).
		{"immersive", 1440, 900, true, func(m *UIModel) { m.Immersive, m.VideoAvailable = true, true }},
		{"album", 1440, 900, true, func(m *UIModel) {
			m.Route = "browse:MPREsite-album-0"
			m.Page = catalog.Page{Title: "Low Light Hours", Creator: "Lumen Drift", Subtitle: siteCopy[m.Language]["album"] + " • Lumen Drift • 2026", Artwork: "cover-low-light-hours", Items: siteAlbumTracks()}
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
