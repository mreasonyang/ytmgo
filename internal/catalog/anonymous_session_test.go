package catalog

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"time"
)

func TestAnonymousHomeSurvivesRedundantClearSession(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	client := fixtureClient(t, func(w http.ResponseWriter, req *http.Request) {
		close(entered)
		<-release
		_ = json.NewEncoder(w).Encode(continuationResponse("synthetic-next", "synthetic-visitor", false))
	})
	type result struct {
		page Page
		err  error
	}
	done := make(chan result, 1)
	go func() {
		page, err := client.Home(context.Background())
		done <- result{page, err}
	}()
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		close(release)
		t.Fatal("anonymous Home request did not start")
	}
	client.ClearSession()
	client.ClearSession()
	close(release)
	select {
	case got := <-done:
		if got.err != nil || len(got.page.Items) != 1 || got.page.Continuation == "" {
			t.Fatal("anonymous restore invalidated the in-flight Home response")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("anonymous Home request did not complete")
	}
}

func TestAnonymousContinuationSurvivesRedundantClearSession(t *testing.T) {
	client := fixtureClient(t, func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Query().Get("continuation") == "" {
			_ = json.NewEncoder(w).Encode(continuationResponse("synthetic-next", "synthetic-visitor", false))
			return
		}
		if req.Header.Get("X-Goog-Visitor-Id") != "synthetic-visitor" {
			t.Error("anonymous continuation lost its visitor")
		}
		_ = json.NewEncoder(w).Encode(catalogResponse(responsiveSong("def456ABC_-", "Next")))
	})
	page, err := client.Home(context.Background())
	if err != nil || page.Continuation == "" {
		t.Fatal("initial anonymous page failed")
	}
	client.ClearSession()
	next, err := client.Continue(context.Background(), page.Continuation)
	if err != nil || len(next.Items) != 1 || next.Items[0].ID != "def456ABC_-" {
		t.Fatal("anonymous restore discarded an existing continuation")
	}
}

func TestAnonymousClearSessionDoesNotHideRequestFailure(t *testing.T) {
	client := fixtureClient(t, func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	})
	client.ClearSession()
	_, err := client.Home(context.Background())
	var failure *HTTPError
	if !errors.As(err, &failure) || failure.StatusCode != http.StatusServiceUnavailable {
		t.Fatal("anonymous session clearing hid a real service failure")
	}
}
