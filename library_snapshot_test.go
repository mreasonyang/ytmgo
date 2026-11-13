package main

import (
	"context"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"ytmgo/internal/accountsession"
	"ytmgo/internal/catalog"
	"ytmgo/internal/librarysnapshot"
	"ytmgo/locales"
)

type snapshotSecrets struct{ key []byte }

func (s *snapshotSecrets) Get(string, accountsession.Interaction) ([]byte, error) {
	if s.key == nil {
		return nil, accountsession.ErrNotFound
	}
	return append([]byte(nil), s.key...), nil
}
func (s *snapshotSecrets) Set(_ string, key []byte) error {
	s.key = append([]byte(nil), key...)
	return nil
}
func (s *snapshotSecrets) Delete(string, accountsession.Interaction) error {
	s.key = nil
	return nil
}

func TestLibrarySnapshotWritesCannotCrossSelectionOrLogout(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "account")
	store := &accountsession.Store{Secrets: &snapshotSecrets{}, Path: filepath.Join(dir, "session.enc")}
	secure, index := true, 1
	session := accountsession.Session{AccountIndex: &index, Cookies: []accountsession.Cookie{{Name: "SAPISID", Value: "synthetic-only", Domain: ".youtube.com", Path: "/", Secure: &secure}}}
	if store.PrepareLogin(context.Background()) != nil || store.Save(context.Background(), session) != nil {
		t.Fatal("synthetic saved selection could not initialize")
	}
	page := func(title string) catalog.Page {
		return catalog.Page{Items: []catalog.Item{{Title: title, BrowseID: "VLsaved", Kind: "playlist"}}}
	}
	a := &desktop{libraryScope: store.CacheScope(), librarySnapshotPath: filepath.Join(dir, "library.json")}
	a.accountState.store = store
	oldScope := a.libraryScope
	if err := librarysnapshot.Save(a.librarySnapshotPath, oldScope, page("Original")); err != nil {
		t.Fatal(err)
	}
	a.accountState.work.Lock()
	a.saveLibrarySnapshot(page("Late old result"))
	if err := store.Save(context.Background(), session); err != nil {
		a.accountState.work.Unlock()
		t.Fatal(err)
	}
	a.accountState.work.Unlock()
	a.libraryWrites.Wait()
	if saved, ok := librarysnapshot.Load(a.librarySnapshotPath, oldScope); !ok || saved.Items[0].Title != "Original" {
		t.Fatal("late old-account worker replaced the snapshot")
	}
	a.libraryScope = store.CacheScope()
	a.saveLibrarySnapshot(page("New selection"))
	a.libraryWrites.Wait()
	if saved, ok := librarysnapshot.Load(a.librarySnapshotPath, a.libraryScope); !ok || saved.Items[0].Title != "New selection" {
		t.Fatal("new account could not save its own library")
	}
	// A fresh process restores both the encrypted selection and matching list.
	restarted := &accountsession.Store{Secrets: store.Secrets, Path: store.Path}
	if _, err := restarted.Load(accountsession.NoPrompt); err != nil {
		t.Fatal(err)
	}
	if saved, ok := librarysnapshot.Load(a.librarySnapshotPath, restarted.CacheScope()); !ok || saved.Items[0].Title != "New selection" {
		t.Fatal("independent account-store instance could not restore the library")
	}
	a.accountState.work.Lock()
	a.saveLibrarySnapshot(page("Late signed-out result"))
	if err := store.Delete(accountsession.NoPrompt); err != nil {
		a.accountState.work.Unlock()
		t.Fatal(err)
	}
	librarysnapshot.Remove(a.librarySnapshotPath)
	a.accountState.work.Unlock()
	a.libraryWrites.Wait()
	if _, err := os.Lstat(a.librarySnapshotPath); !os.IsNotExist(err) {
		t.Fatal("pending worker recreated a signed-out library")
	}
}

func TestCachedLibraryRemainsVisibleWhileRefreshingAndAfterFailure(t *testing.T) {
	page := catalog.Page{Items: []catalog.Item{{Title: "Saved playlist", BrowseID: "VLsaved", Kind: "playlist"}}}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	a := &desktop{ctx: ctx, libraryScope: "0123456789abcdef01234567", libraryPage: &page, historyIndex: -1, model: UIModel{Route: "home", AccountRestore: accountRestoring}}
	a.restoreCachedLibrary()
	if a.model.SignedIn || !a.model.LibraryLoaded || !a.model.LibraryCached {
		t.Fatal("local display snapshot changed account verification")
	}
	if len(a.model.LibraryItems) != 1 {
		t.Fatal("restored library is not immediately available to the view")
	}
	a.model.SignedIn = true
	pending := &libraryRequest{cancel: cancel, scope: a.libraryScope}
	a.libraryRequest = pending
	a.model.LibraryRefreshing = true
	if a.loadLibrary() != pending || len(a.model.LibraryItems) != 1 || !a.model.LibraryRefreshing {
		t.Fatal("refresh blanked the restored library")
	}
	a.finishLibrary(pending, catalog.Page{}, catalog.ErrUnavailable)
	if len(a.model.LibraryItems) != 1 || a.model.Error != "" || a.model.LibraryErrorKey == "" || a.model.LibraryRefreshing || !a.model.LibraryCached {
		t.Fatal("network failure discarded the usable snapshot")
	}
	fresh := catalog.Page{Items: []catalog.Item{{Title: "Updated playlist", BrowseID: "VLupdated", Kind: "playlist"}}}
	pending = &libraryRequest{cancel: cancel, pageGeneration: a.pageGeneration}
	a.libraryRequest = pending
	a.model.LibraryErrorKey = ""
	a.finishLibrary(pending, fresh, nil)
	if a.model.LibraryCached || a.model.LibraryItems[0].Title != "Updated playlist" || a.libraryPage.Items[0].Title != "Updated playlist" {
		t.Fatal("fresh library did not replace the snapshot")
	}
	a.forgetLibrarySnapshot()
	a.clearAccountContent()
	if a.libraryPage != nil || a.libraryScope != "" || a.model.LibraryCached || len(a.model.LibraryItems) != 0 {
		t.Fatal("account change retained restored library state")
	}
}

// Sample the main panel before, during and after a delayed Library refresh.
// The sidebar already has the same covers, and Home shows them in larger tiles.
func TestLibraryClickKeepsRowCoversDuringRefresh(t *testing.T) {
	const raw = "https://i.ytimg.com/vi/abc123DEF_-/hqdefault.jpg"
	rows := []catalog.Item{
		{Title: "First album", Kind: "album", BrowseID: "MPREfirst", Artwork: raw},
		{Title: "Second album", Kind: "album", BrowseID: "MPREsecond", Artwork: raw},
		{Title: "Saved playlist", Kind: "playlist", BrowseID: "VLsaved", Artwork: raw},
	}
	live := catalog.Page{Items: rows, Shelves: []catalog.Shelf{{Items: rows}}, Continuation: "next-page"}
	ink := color.RGBA{R: 25, G: 223, B: 93, A: 255}
	im := image.NewRGBA(image.Rect(0, 0, 8, 8))
	draw.Draw(im, im.Bounds(), image.NewUniform(ink), image.Point{}, draw.Src)
	bitmap := ui.NewBitmap(im)

	for _, test := range []struct {
		source string
		width  int
	}{{"memory", 1280}, {"disk", 1280}, {"memory", 420}, {"disk", 420}} {
		t.Run(fmt.Sprintf("%s-%d", test.source, test.width), func(t *testing.T) {
			saved := live
			if test.source == "disk" {
				saved = librarysnapshot.Metadata(live)
			}
			m, a := nativeViewFixture()
			m.SignedIn, m.LibraryLoaded, m.LibraryItems = true, true, rows
			m.instantMotion = false
			pending := &libraryRequest{}
			app := &desktop{model: *m, historyIndex: -1, libraryPage: &saved, libraryRequest: pending}
			app.model.LibraryRefreshing = true
			a.Navigate, a.LoadMore = app.navigate, nil
			a.artworkPeek = func(src string, _ int) ui.ImageSource {
				if src == raw {
					return bitmap
				}
				return nil
			}
			a.Artwork = func(src string, edge int, _ artworkPriority) ui.ImageSource { return a.artworkPeek(src, edge) }
			tt := ui.NewTester(func(c *ui.Context) { nativeView(c, &app.model, a) }, test.width, 800)
			// The Library tab, narrow; wide, the panel is the library and
			// the page is reached by route.
			var err error
			if test.width <= 760 {
				err = tt.Click(a.Tr("shellNavLibrary"))
			} else {
				app.navigate("library")
				tt.Frame()
			}
			if err != nil || app.model.Route != "library" {
				t.Fatalf("Library click failed: %v", err)
			}
			checkRows := func() {
				t.Helper()
				page := rect(t, tt, a.Tr("shellContent"))
				img := tt.Image()
				pixels := 0
				for y := int(page.Y); y < int(page.Y+page.H); y++ {
					for x := int(page.X); x < int(page.X+page.W); x++ {
						if img.RGBAAt(x, y) == ink {
							pixels++
						}
					}
				}
				if pixels < len(rows)*32*32 || pixels > len(rows)*40*40 {
					t.Fatalf("main panel painted %d cover pixels, expected three 40-DIP thumbnails", pixels)
				}
			}
			checkRows()
			for elapsed := time.Duration(0); elapsed < tokenMotionMorph; elapsed += time.Second / 60 {
				tt.Advance(time.Second / 60)
				checkRows()
			}
			if app.model.Page.Continuation != "" {
				t.Fatal("pending refresh exposed the previous continuation")
			}
			app.finishLibrary(pending, live, nil)
			tt.Frame()
			checkRows()
			tt.Advance(tokenMotionMorph)
			checkRows()
			if app.model.Page.Continuation != live.Continuation {
				t.Fatal("refreshed library lost its continuation")
			}
		})
	}
}

func TestLibraryPaginationPersistsEveryLoadedItemAndRejectsStalePage(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "account")
	store := &accountsession.Store{Secrets: &snapshotSecrets{}, Path: filepath.Join(dir, "session.enc")}
	if err := store.PrepareLogin(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := store.Save(context.Background(), syntheticAccountSession()); err != nil {
		t.Fatal(err)
	}
	first := catalog.Page{Items: []catalog.Item{{Title: "First", BrowseID: "VLfirst", Kind: "playlist"}}, Continuation: "old-process-page-2"}
	a := &desktop{libraryScope: store.CacheScope(), librarySnapshotPath: filepath.Join(dir, "library.json"),
		pageGeneration: 1, historyIndex: -1, libraryPage: &first,
		model: UIModel{Route: "library", SignedIn: true, LibraryLoaded: true, LibraryItems: first.Items, Page: first}}
	a.accountState.store = store
	// Both saves queue behind the worker. The initial page cannot overwrite a
	// newer full list even if the scheduler acquires the lock out of order.
	storeLock := &a.accountState.work
	storeLock.Lock()
	a.saveLibrarySnapshot(first)
	a.finishContinuation(1, catalog.Page{Items: []catalog.Item{{Title: "Second", BrowseID: "VLsecond", Kind: "playlist"}}}, nil)
	storeLock.Unlock()
	a.libraryWrites.Wait()
	saved, ok := librarysnapshot.Load(a.librarySnapshotPath, a.libraryScope)
	if !ok || len(saved.Items) != 2 || len(a.model.LibraryItems) != 2 || len(a.libraryPage.Items) != 2 || saved.Items[1].Title != "Second" || saved.Continuation != "" {
		t.Fatal("pagination did not persist the complete metadata list independently of continuation handles")
	}
	a.clearAccountContent()
	a.finishContinuation(1, catalog.Page{Items: []catalog.Item{{Title: "Late", BrowseID: "VLlate", Kind: "playlist"}}}, nil)
	a.libraryWrites.Wait()
	saved, ok = librarysnapshot.Load(a.librarySnapshotPath, a.libraryScope)
	if !ok || len(saved.Items) != 2 || len(a.model.LibraryItems) != 0 {
		t.Fatal("stale continuation modified the account's saved library")
	}
}

func TestNativeSavedLibraryStatesAndInteraction(t *testing.T) {
	for _, locale := range locales.Catalog.Languages() {
		for _, width := range []int{1440, 420} {
			for _, state := range []string{"restoring", "updating", "failed"} {
				t.Run(fmt.Sprintf("%s-%d-%s", locale, width, state), func(t *testing.T) {
					m, actions := nativeViewFixture()
					m.Language, m.Route, m.AccountRestore = locale, "library", accountRestoring
					m.LibraryLoaded, m.LibraryCached = true, true
					m.LibraryItems = []catalog.Item{{Title: "Saved playlist", BrowseID: "VLsaved", Kind: "playlist"}}
					m.Page = catalog.Page{Items: m.LibraryItems}
					key := "nativeLibrarySaved"
					opened := 0
					actions.Open = func(catalog.Item) { opened++ }
					if state != "restoring" {
						m.SignedIn, m.AccountRestore = true, ""
						m.LibraryRefreshing = state == "updating"
						key = "nativeLibraryUpdating"
					}
					retried := 0
					actions.RetryLibrary = func() { retried++ }
					if state == "failed" {
						m.LibraryErrorKey = "errorCatalogUnavailable"
						key = "nativeLibrarySavedError"
					}
					tt := ui.NewTester(func(c *ui.Context) { nativeView(c, m, actions) }, width, 900)
					if !tt.HasText("Saved playlist") || !tt.HasText(actions.Tr(key)) {
						t.Fatalf("saved library state is missing: %v", tt.Texts())
					}
					if state == "restoring" {
						_ = tt.Click(nativeResultLabel(m.LibraryItems[0], m, actions))
						if opened != 0 {
							t.Fatal("unverified snapshot opened a private collection")
						}
					}
					if state == "failed" {
						if err := tt.Click(actions.Tr("buttonRetry")); err != nil || retried != 1 {
							t.Fatal("cached library failure has no working retry")
						}
					}
					if locale == "en" || locale == "zh-Hans" {
						nativeSnapshot(t, tt, fmt.Sprintf("library-snapshot-%s-%d-%s", locale, width, state))
					}
				})
			}
		}
	}
}
