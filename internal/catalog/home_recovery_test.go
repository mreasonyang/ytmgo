package catalog

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"sync/atomic"
	"testing"
	"time"
)

func TestColdJapaneseHomeRetriesEmptyResponse(t *testing.T) {
	var calls atomic.Int32
	c := fixtureClient(t, func(w http.ResponseWriter, req *http.Request) {
		if calls.Add(1) == 1 {
			_ = json.NewEncoder(w).Encode(catalogResponse())
			return
		}
		_ = json.NewEncoder(w).Encode(catalogResponse(responsiveSong("def456ABC_-", "Home song")))
	})
	c.SetLanguage("ja")
	page, err := c.Home(context.Background())
	if err != nil || len(page.Items) != 1 || calls.Load() != 2 {
		t.Fatal("cold Home retained a transient empty response")
	}
	if _, err := c.Home(context.Background()); err != nil || calls.Load() != 2 {
		t.Fatal("recovered Home was not cached")
	}
}

func TestHomeStartupLive(t *testing.T) {
	if os.Getenv("YTMGO_LIVE_HOME_STARTUP") != "1" {
		t.Skip("explicit anonymous Home acceptance not selected")
	}
	for _, language := range []string{"ja", "en", "zh-CN", "zh-TW"} {
		t.Run(language, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
			defer cancel()
			c := NewWithOptions(Options{Language: language})
			page, err := c.Home(ctx)
			if err != nil {
				t.Fatal("anonymous Home request failed")
			}
			if len(page.Items) == 0 {
				t.Fatal("anonymous Home contains no displayable items")
			}
			t.Logf("items=%d shelves=%d continuation=%t", len(page.Items), len(page.Shelves), page.Continuation != "")
		})
	}
}

func TestEmptyHomeHasBoundedRetryAndIsNotCached(t *testing.T) {
	var calls atomic.Int32
	c := fixtureClient(t, func(w http.ResponseWriter, req *http.Request) {
		calls.Add(1)
		_ = json.NewEncoder(w).Encode(catalogResponse())
	})
	for attempt := 1; attempt <= 2; attempt++ {
		if _, err := c.Home(context.Background()); !errors.Is(err, ErrInvalidResponse) || calls.Load() != int32(attempt*2) {
			t.Fatal("empty Home became a successful page or exceeded its retry budget")
		}
	}
}
