package main

import (
	"fmt"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
	"ytmgo/internal/catalog"
	"ytmgo/locales"
)

func TestNativeLibraryMetadataDoesNotRepeatCategory(t *testing.T) {
	for _, language := range locales.Catalog.Languages() {
		t.Run(language, func(t *testing.T) {
			m, a := nativeViewFixture()
			m.Language, m.SignedIn = language, true
			for _, kind := range []string{"playlist", "album", "artist", "song", "video"} {
				item := catalog.Item{Title: "Example " + kind, Kind: kind, BrowseID: "example-" + kind}
				if kind == "song" || kind == "video" {
					item.ID = "example-id"
				}
				want := nativeKindLabel(item, a)
				for _, sourceLanguage := range locales.Catalog.Languages() {
					item.Artist = " " + locales.Catalog.Format(sourceLanguage, nativeKindKey(item), nil) + " "
					if got := nativeLibrarySubtitle(item, a); got != want {
						t.Errorf("%s from %s: got %q, want %q", kind, sourceLanguage, got, want)
					}
				}
				item.Artist = "Artist Collective"
				if got := nativeLibrarySubtitle(item, a); got != want+" · Artist Collective" {
					t.Errorf("real creator lost: %q", got)
				}
				item.Artist = want
				m.LibraryItems = append(m.LibraryItems, item)
			}
			tt := ui.NewTester(func(c *ui.Context) { nativeView(c, m, a) }, 1440, 900)
			for _, item := range m.LibraryItems {
				kind := nativeKindLabel(item, a)
				if tt.HasText(kind + " · " + kind) {
					t.Errorf("sidebar still repeats %q", kind)
				}
			}
			nativeSnapshot(t, tt, "fixed-library-metadata-"+language)
		})
	}
}

func TestNativeShortImmersiveKeepsArtistAbovePlayer(t *testing.T) {
	for _, width := range []int{761, 960, 1440} {
		for _, language := range locales.Catalog.Languages() {
			for _, appearance := range []string{"dark", "light"} {
				for _, video := range []bool{false, true} {
					t.Run(fmt.Sprintf("%d-%s-%s-video-%v", width, language, appearance, video), func(t *testing.T) {
						m, a := nativeViewFixture()
						m.Immersive, m.Video = true, video
						m.Language, m.Appearance = language, appearance
						m.Current.Title = "Daft Punk : Get Lucky // Grammys Rehearsal (HD)"
						m.Current.Artist = "Daft Punk – Archives"
						if language != "en" {
							m.Current.Title = "坂本龍一：音楽の旅と長い曲名／ライブ・アーカイブ特別版"
							m.Current.Artist = "坂本龍一 – アーカイブ"
						}
						tt := ui.NewTester(func(c *ui.Context) { nativeView(c, m, a) }, width, 609)
						r := nativeAssertVisible(t, tt, m.Current.Artist, width, 609)
						if r.Y+r.H > 609-nativePlayerHeight-24 {
							t.Fatalf("artist lacks the 24 DIP bottom inset: %+v", r)
						}
						nativeAssertVisible(t, tt, a.Tr("shellPause"), width, 609)
						if width == 960 {
							nativeSnapshot(t, tt, fmt.Sprintf("fixed-short-immersive-%s-%s-video-%v", language, appearance, video))
						}
					})
				}
			}
		}
	}
}

func TestNativeImmersiveOverflowCanScroll(t *testing.T) {
	m, a := nativeViewFixture()
	m.Immersive = true
	m.Current.Title = "A long concert title that occupies both title lines"
	m.Current.Artist = strings.Repeat("Guest artist ensemble ", 12)
	m.PlaybackError = "Playback needs a retry"
	a.RetryPlayback = func() { m.PlaybackError = "" }
	tt := ui.NewTester(func(c *ui.Context) { nativeView(c, m, a) }, 960, 609)
	scroll := nativeAssertVisible(t, tt, a.Tr("shellContent"), 960, 609)
	tt.Scroll(scroll.X+scroll.W/2, scroll.Y+scroll.H/2, 0, 1000)
	artist := nativeAssertVisible(t, tt, m.Current.Artist, 960, 609)
	if artist.Y+artist.H > 609-nativePlayerHeight-24 {
		t.Fatalf("overflow artist cannot be reached above the footer: %+v", artist)
	}
	nativeAssertVisible(t, tt, a.Tr("shellPause"), 960, 609)
	if err := tt.Click(a.Tr("buttonRetry")); err != nil {
		t.Fatal(err)
	}
	if m.PlaybackError != "" {
		t.Fatal("scrolled playback notice cannot be activated")
	}
	nativeSnapshot(t, tt, "fixed-short-immersive-scrolled")
}

func TestNativeQueueArtworkVisibleInEveryPresentation(t *testing.T) {
	for _, layout := range []struct {
		name          string
		width, height int
		immersive     bool
	}{{"docked", 1440, 900, false}, {"floating", 1100, 900, false}, {"immersive", 1440, 900, true}, {"compact-immersive", 420, 900, true}} {
		t.Run(layout.name, func(t *testing.T) {
			m, a := nativeViewFixture()
			m.Panel, m.Immersive, m.PanelTab = true, layout.immersive, "queue"
			m.Page.Items, m.Page.Shelves = nil, nil
			for i := len(m.Queue); i < 40; i++ {
				m.Queue = append(m.Queue, catalog.Item{ID: fmt.Sprintf("extra-%d", i), Title: fmt.Sprintf("Queued %d", i), Artist: "Daft Punk", Kind: "song", Artwork: fmt.Sprintf("extra-art-%d", i)})
			}
			requested := map[string]bool{}
			original := a.Artwork
			a.Artwork = func(src string, edge int) ui.ImageSource {
				if edge <= 128 && src != m.Current.Artwork {
					requested[src] = true
				}
				return original(src, edge)
			}
			tt := ui.NewTester(func(c *ui.Context) { nativeView(c, m, a) }, layout.width, layout.height)
			if layout.name == "floating" || layout.name == "compact-immersive" {
				// The queue card starts below the fold in these presentations.
				chip := nativeAssertVisible(t, tt, a.Tr("shellQueue")+" · "+a.Tr("shellNowPlaying"), layout.width, layout.height)
				tt.Scroll(chip.X, chip.Y, 0, 400)
			}
			if !requested[m.Queue[1].Artwork] {
				t.Fatalf("visible queue rows show placeholders instead of artwork: %v", requested)
			}
			if requested[m.Queue[len(m.Queue)-1].Artwork] {
				t.Fatal("offscreen queue artwork was requested")
			}
			nativeSnapshot(t, tt, "queue-artwork-"+layout.name)
		})
	}
}
