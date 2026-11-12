package main

import (
	"context"
	"testing"

	"ytmgo/internal/audioquality"
	"ytmgo/internal/downloads"
)

// Downloading keeps the listener where they asked, and asking again for a
// paused or failed song restarts it.
func TestDownloadStaysOnThePageAndResumes(t *testing.T) {
	fixture, _ := nativeViewFixture()
	app := &desktop{model: *fixture, historyIndex: -1, ctx: t.Context()}
	offline := func(context.Context, string, audioquality.Quality) (downloads.Remote, error) {
		return downloads.Remote{}, context.Canceled
	}
	library, err := downloads.New(t.Context(), t.TempDir(), nil, offline, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer library.Close()
	app.downloads = library
	it := app.model.Queue[1]
	it.ID = "AAAAAAAAAAA" // the library keeps only real video IDs
	app.downloadItem(it)
	if app.model.Route != "home" || app.model.downloadStates[it.ID] != downloads.Queued {
		t.Fatalf("route %q, state %q after Download song", app.model.Route, app.model.downloadStates[it.ID])
	}
	if err := library.SetPaused(it.ID, true); err != nil {
		t.Fatal(err)
	}
	app.refreshDownloads()
	app.downloadItem(it)
	if app.model.downloadStates[it.ID] != downloads.Queued {
		t.Fatalf("downloading a paused song again left it %q", app.model.downloadStates[it.ID])
	}
}
