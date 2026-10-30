package main

import (
	"fmt"
	"testing"

	"github.com/egoist/mygo/ui"
	"ytmgo/internal/catalog"
	"ytmgo/locales"
)

func TestNativeSearchAccessibleTypes(t *testing.T) {
	for _, locale := range locales.Catalog.Languages() {
		for _, rows := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s-rows-%v", locale, rows), func(t *testing.T) {
				m, a := nativeViewFixture()
				m.Language = locale
				m.Route = "search"
				m.Current = catalog.Item{}
				m.LibraryItems = nil
				items := []catalog.Item{}
				for _, kind := range []string{"song", "video", "artist", "album", "playlist", "podcast", "episode"} {
					items = append(items, catalog.Item{Title: kind + " title", Artist: "Creator", Kind: kind, ID: kind + "-id", BrowseID: kind + "-browse"})
				}
				var clicks []string
				a.Open = func(v catalog.Item) { clicks = append(clicks, v.Kind) }
				a.Play = a.Open
				tt := ui.NewTester(func(c *ui.Context) {
					ui.Column(c).Children(func() {
						for i, item := range items {
							if rows {
								nativeTrack(c, item, i, m, a, palette(true), nativeTrackRow, false)
							} else {
								nativeCard(c, item, m, a, palette(true), false)
							}
						}
					})
				}, 1440, 2500)
				for _, item := range items {
					label := item.Title + " · " + nativeLibrarySubtitle(item, a)
					if _, ok := tt.Find(label); !ok {
						t.Errorf("accessible %s name lacks type", item.Kind)
					}
					if err := tt.Click(label); err != nil {
						t.Fatal(err)
					}
				}
				if len(clicks) != len(items) {
					t.Fatal("result activation changed")
				}
			})
		}
	}
}
func TestNativeShortCollectionShowsCreatorAndTrack(t *testing.T) {
	for _, locale := range locales.Catalog.Languages() {
		for _, width := range []int{380, 420, 757, 960, 1440} {
			for _, theme := range []string{"dark", "light"} {
				t.Run(fmt.Sprintf("%s-%d-%s", locale, width, theme), func(t *testing.T) {
					m, a := nativeViewFixture()
					m.Language = locale
					m.Appearance = theme
					m.Route = "browse:album"
					m.Panel = false
					m.Page = catalog.Page{Title: "Opus — Live at a Very Long Concert Title", Creator: "Ryuichi Sakamoto", Subtitle: "Album • 2024", Artwork: "art-1", Items: m.Queue}
					tt := ui.NewTester(func(c *ui.Context) { nativeView(c, m, a) }, width, 609)
					nativeAssertVisible(t, tt, m.Page.Creator, width, 609)
					row := nativeAssertVisible(t, tt, nativeItemLabel(m.Queue[0]), width, 609)
					bottom := float32(609 - nativePlayerHeight)
					if width <= 760 {
						bottom = 609 - nativeCompactTabbar - 76
					}
					if row.Y+row.H > bottom {
						t.Fatalf("first track is below the player: %v bottom=%v", row, bottom)
					}
					if width == 757 {
						nativeSnapshot(t, tt, fmt.Sprintf("short-album-%s-%s", locale, theme))
					}
				})
			}
		}
	}
}
func TestNativeLibraryStateText(t *testing.T) {
	for _, locale := range locales.Catalog.Languages() {
		m, a := nativeViewFixture()
		m.Language = locale
		states := []struct {
			signed, loaded, loading   bool
			restore, route, err, want string
		}{
			{false, false, false, "", "home", "", "shellLibraryEmptyBody"},
			{false, false, false, accountRestoring, "home", "", "nativeAccountRestoringBody"},
			{false, false, false, accountRestoreDeferred, "home", "", "nativeAccountRestoreDeferredBody"},
			{true, false, false, "", "home", "", "nativeLibraryOpenBody"},
			{true, false, true, "", "library", "", "nativeLibraryLoadingBody"},
			{true, false, false, "", "library", "failure", "nativeLibraryOpenBody"},
			{true, true, false, "", "home", "", "nativeLibraryEmptyBody"},
		}
		for _, state := range states {
			m.SignedIn = state.signed
			m.LibraryLoaded = state.loaded
			m.Loading = state.loading
			m.AccountRestore = state.restore
			m.Route = state.route
			m.Error = state.err
			_, body := nativeLibraryPromptText(m)
			if body != state.want {
				t.Errorf("%s: incorrect library state", locale)
			}
			if state.signed && a.Tr(body) == a.Tr("shellLibraryEmptyBody") {
				t.Errorf("%s: signed in user prompted to log in", locale)
			}
		}
	}
}
func TestNativeWidePlaybackErrorAppearsOnce(t *testing.T) {
	for _, locale := range locales.Catalog.Languages() {
		for _, width := range []int{420, 1440} {
			for _, panel := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s-%d-panel-%v", locale, width, panel), func(t *testing.T) {
					m, a := nativeViewFixture()
					m.Language = locale
					m.Panel = panel
					m.PlaybackError = a.Tr("nativeVerificationRequired")
					retries := 0
					a.RetryPlayback = func() { retries++ }
					tt := ui.NewTester(func(c *ui.Context) { nativeView(c, m, a) }, width, 900)
					messages, buttons := 0, 0
					for _, text := range tt.Texts() {
						if text == m.PlaybackError {
							messages++
						}
						if text == a.Tr("buttonRetry") {
							buttons++
						}
					}
					want := 0
					if panel {
						want = 1
					}
					if messages != want || buttons != 1 {
						t.Fatalf("error duplication: messages=%d retries=%d", messages, buttons)
					}
					if err := tt.Click(a.Tr("buttonRetry")); err != nil {
						t.Fatal(err)
					}
					if retries != 1 {
						t.Fatal("retry did not reach playback")
					}
					if width == 1440 && panel {
						nativeSnapshot(t, tt, "wide-playback-error-"+locale)
					}
				})
			}
		}
	}
}

func TestNativeCreatorAlreadyInSubtitle(t *testing.T) {
	for _, value := range []string{"Artist", "Artist · Guest"} {
		if !nativeMetadataHasCreator("Album • "+value+" • 2024", value) {
			t.Fatal("creator duplicated in header")
		}
	}
	if nativeMetadataHasCreator("Album • Different Artist • 2024", "Artist") {
		t.Fatal("partial name hid creator")
	}
}

func TestAccountContentResetClearsLibraryLoadState(t *testing.T) {
	a := &desktop{model: UIModel{LibraryLoaded: true, LibraryItems: []catalog.Item{{Title: "Saved album"}}, Page: catalog.Page{Title: "Library"}}}
	a.clearAccountContent()
	if a.model.LibraryLoaded || len(a.model.LibraryItems) != 0 {
		t.Fatal("new account inherited loaded-library state")
	}
}
