package catalog

import (
	"context"
	"os"
	"testing"
	"time"
)

// Opt in explicitly. The normal suite is entirely offline. Only public track
// result counts are printed, never full metadata, response, or context data.
func TestLiveAnonymousSearch(t *testing.T) {
	if os.Getenv("YTMGO_LIVE_SEARCH") != "1" {
		t.Skip("set YTMGO_LIVE_SEARCH=1 to check the public endpoint")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	tracks, err := NewClient().Search(ctx, "Daft Punk Get Lucky")
	if err != nil {
		t.Fatal(err)
	}
	if len(tracks) == 0 {
		t.Fatal("live query returned no song/video targets")
	}
	t.Logf("public song/video targets: %d", len(tracks))
}
