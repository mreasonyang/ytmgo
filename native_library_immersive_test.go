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
