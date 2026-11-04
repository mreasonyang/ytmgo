package accountsession

import (
	"context"
	"testing"
)

func TestSnapshotScopeSurvivesRestartAndChangesWithSelection(t *testing.T) {
	store, secrets := testStore(t)
	if store.CacheScope() != "" {
		t.Fatal("new store supplied an unverified cache scope")
	}
	if err := store.PrepareLogin(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := store.Save(context.Background(), fixture()); err != nil {
		t.Fatal(err)
	}
	first := store.CacheScope()
	if len(first) != 24 {
		t.Fatal("saved selection has no snapshot scope")
	}
	restarted := Store{Secrets: secrets, Path: store.Path}
	if restarted.CacheScope() != "" {
		t.Fatal("scope was available before local session restoration")
	}
	if _, err := restarted.Load(NoPrompt); err != nil || restarted.CacheScope() != first {
		t.Fatal("restart lost the saved selection scope")
	}
	if err := restarted.Save(context.Background(), fixture()); err != nil || restarted.CacheScope() == first {
		t.Fatal("a new saved sign-in reused the old snapshot scope")
	}
	secrets.failGet = true
	if _, err := restarted.Load(NoPrompt); err == nil || restarted.CacheScope() != "" {
		t.Fatal("failed session restoration retained a display scope")
	}
	secrets.failGet = false
	if err := restarted.Delete(NoPrompt); err != nil || restarted.CacheScope() != "" {
		t.Fatal("sign-out retained a display scope")
	}
}
