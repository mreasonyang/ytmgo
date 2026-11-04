package main

import (
	"log"

	"ytmgo/internal/catalog"
	"ytmgo/internal/librarysnapshot"
)

// The local account file was decrypted before this scope was obtained. Display
// data can be restored before network verification without claiming sign-in.
func (a *desktop) restoreCachedLibrary() {
	if a.libraryPage == nil || a.libraryScope == "" {
		return
	}
	a.model.LibraryItems = append([]catalog.Item(nil), a.libraryPage.Items...)
	a.model.LibraryLoaded, a.model.LibraryCached = true, true
}

func (a *desktop) cachedLibraryPage() catalog.Page {
	if a.libraryPage == nil {
		return catalog.Page{Title: a.tr("shellLibraryTitle")}
	}
	page := librarysnapshot.Metadata(*a.libraryPage)
	page.Title = a.tr("shellLibraryTitle")
	return page
}

func (a *desktop) forgetLibrarySnapshot() {
	a.libraryScope, a.libraryPage = "", nil
	a.model.LibraryCached = false
}

func (a *desktop) saveLibrarySnapshot(page catalog.Page) {
	if a.librarySnapshotPath == "" || a.libraryScope == "" || a.accountState.store == nil {
		return
	}
	scope, path := a.libraryScope, a.librarySnapshotPath
	page = librarysnapshot.Metadata(page)
	revision := a.libraryWriteRevision.Add(1)
	a.libraryWrites.Add(1)
	go func() {
		defer a.libraryWrites.Done()
		s := &a.accountState
		s.work.Lock()
		defer s.work.Unlock()
		// Save/Delete of the selected account uses this same serial worker.
		// A result queued by an old account cannot rewrite its snapshot later.
		if s.store.CacheScope() != scope || a.libraryWriteRevision.Load() != revision {
			return
		}
		if librarysnapshot.Save(path, scope, page) != nil {
			log.Print("ytmgo: could not save library snapshot")
		}
	}()
}
