package catalog

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func directAPIResponse(req *http.Request, status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: req}
}

func TestDirectAccountStatusUsesOneCanonicalPOST(t *testing.T) {
	for _, state := range []struct {
		name              string
		loggedIn, premium bool
	}{
		{"anonymous", false, false},
		{"standard", true, false},
		{"premium", true, true},
	} {
		t.Run(state.name, func(t *testing.T) {
			signedIn := state.loggedIn
			calls := 0
			client := NewWithOptions(Options{HTTPClient: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				calls++
				if req.Method != http.MethodPost || req.URL.Scheme != "https" || req.URL.Host != "music.youtube.com" || req.URL.Path != "/youtubei/v1/browse" || req.URL.RawQuery != "prettyPrint=false" {
					t.Error("account status did not directly target the canonical API POST")
				}
				var body object
				if json.NewDecoder(req.Body).Decode(&body) != nil {
					t.Fatal("synthetic request body could not be decoded")
				}
				if body["browseId"] != "MPREb_4pL8gzRtw1p" || textAt(body, "context", "client", "clientName") != "WEB_REMIX" || textAt(body, "context", "client", "clientVersion") != "1.20250219.01.00" || req.Header.Get("X-Youtube-Client-Version") != "1.20250219.01.00" || req.Header.Get("X-Youtube-Client-Name") != "67" {
					t.Error("direct API client contract was not preserved")
				}
				contextMap, _ := body["context"].(map[string]any)
				clientMap, _ := contextMap["client"].(map[string]any)
				if _, present := clientMap["visitorData"]; present || req.Header.Get("X-Goog-Visitor-Id") != "" {
					t.Error("direct API unexpectedly depended on homepage visitor state")
				}
				if signedIn {
					if req.Header.Get("Cookie") != "SAPISID=synthetic-test-only" || req.Header.Get("Authorization") == "" || req.Header.Get("X-Goog-Authuser") != "2" || req.Header.Get("X-Goog-Pageid") != "synthetic_brand" || textAt(body, "context", "user", "onBehalfOfUser") != "synthetic_brand" {
						t.Error("direct API lost the selected account context")
					}
				} else {
					for _, key := range []string{"Cookie", "Authorization", "X-Goog-Authuser", "X-Goog-Pageid", "X-Origin"} {
						if req.Header.Get(key) != "" {
							t.Error("anonymous direct API carried account authorization")
						}
					}
					if _, present := contextMap["user"]; present {
						t.Error("anonymous direct API carried a selected account payload")
					}
				}
				logged, premium := "0", "False"
				if signedIn {
					logged = "1"
				}
				if state.premium {
					premium = "True"
				}
				response, _ := json.Marshal(validStatus(logged, premium))
				return directAPIResponse(req, 200, string(response)), nil
			})}})
			if signedIn && client.SetSession(syntheticCookies(), 2, "synthetic_brand") != nil {
				t.Fatal("synthetic session rejected")
			}
			for attempt := 1; attempt <= 2; attempt++ {
				status, err := client.AccountStatus(context.Background())
				if err != nil || status.LoggedIn != state.loggedIn || status.MusicPremium != state.premium || calls != attempt {
					t.Fatal("direct status required more than one request per check or returned the wrong state")
				}
			}
		})
	}
}

func TestLateDirectPOSTCannotCrossAccountScope(t *testing.T) {
	for _, change := range []string{"replace", "clear", "expire"} {
		t.Run(change, func(t *testing.T) {
			entered, release := make(chan struct{}), make(chan struct{})
			var calls atomic.Int32
			client := NewWithOptions(Options{HTTPClient: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				calls.Add(1)
				if req.Method != http.MethodPost || req.URL.Path != "/youtubei/v1/browse" {
					t.Error("account status made a preliminary request")
				}
				close(entered)
				<-release
				body, _ := json.Marshal(validStatus("1", "True"))
				return directAPIResponse(req, 200, string(body)), nil
			})}})
			if client.SetSession(syntheticCookies(), 0, "") != nil {
				t.Fatal("synthetic session rejected")
			}
			done := make(chan error, 1)
			go func() {
				status, err := client.AccountStatus(context.Background())
				if status != (AccountStatus{}) {
					t.Error("stale account status was returned")
				}
				done <- err
			}()
			select {
			case <-entered:
			case <-time.After(3 * time.Second):
				close(release)
				t.Fatal("synthetic direct POST did not start")
			}
			want := ErrSessionChanged
			switch change {
			case "replace":
				if client.SetSession(syntheticCookies(), 3, "") != nil {
					close(release)
					t.Fatal("replacement synthetic session rejected")
				}
			case "clear":
				client.ClearSession()
			case "expire":
				// Authorization has completed before the transport signals.
				client.mu.Lock()
				client.session.expires = time.Now().Add(-time.Second)
				client.mu.Unlock()
				want = ErrSessionExpired
			}
			close(release)
			if err := <-done; !errors.Is(err, want) || calls.Load() != 1 {
				t.Fatal("late direct POST escaped scope or issued another request")
			}
		})
	}
}

func TestDirectPOSTRedirectIsNotFollowedOrRetried(t *testing.T) {
	for _, signedIn := range []bool{false, true} {
		name := "anonymous"
		if signedIn {
			name = "selected-account"
		}
		t.Run(name, func(t *testing.T) {
			calls := 0
			client := NewWithOptions(Options{HTTPClient: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				calls++
				if req.Method != http.MethodPost || req.URL.Path != "/youtubei/v1/browse" {
					t.Error("direct status performed an unexpected request")
				}
				response := directAPIResponse(req, http.StatusFound, "")
				response.Header.Set("Location", "https://accounts.google.com/synthetic-redirect")
				return response, nil
			})}})
			if signedIn && client.SetSession(syntheticCookies(), 2, "") != nil {
				t.Fatal("synthetic session rejected")
			}
			_, err := client.AccountStatus(context.Background())
			var httpError *HTTPError
			if !errors.As(err, &httpError) || httpError.Stage != "browse" || httpError.StatusCode != http.StatusFound || calls != 1 {
				t.Fatal("direct API redirect was followed, retried, or lost its browse failure")
			}
		})
	}
}
