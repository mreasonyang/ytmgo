package catalog

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func statusResponse(params ...any) object {
	return object{"responseContext": object{"serviceTrackingParams": []any{object{"service": "GFEEDBACK", "params": params}}}}
}
func statusParam(key string, value any) object { return object{"key": key, "value": value} }
func validStatus(logged, premium string) object {
	return statusResponse(statusParam("logged_in", logged), statusParam("has_unlimited_entitlement", premium))
}
func syntheticCookies() []*http.Cookie {
	return []*http.Cookie{{Name: "SAPISID", Value: "synthetic-test-only", Domain: ".youtube.com", Secure: true}}
}

func TestAccountStatusRequiresExplicitConsistentServiceFields(t *testing.T) {
	tests := []struct {
		name     string
		response object
		want     AccountStatus
		valid    bool
	}{
		{"premium", validStatus("1", "True"), AccountStatus{true, true}, true},
		{"free", validStatus("1", "False"), AccountStatus{true, false}, true},
		{"logged-out", validStatus("0", "False"), AccountStatus{}, true},
		{"missing-context", object{}, AccountStatus{}, false},
		{"missing-login", statusResponse(statusParam("has_unlimited_entitlement", "False")), AccountStatus{}, false},
		{"missing-premium", statusResponse(statusParam("logged_in", "1")), AccountStatus{}, false},
		{"invalid-login-type", statusResponse(statusParam("logged_in", true), statusParam("has_unlimited_entitlement", "True")), AccountStatus{}, false},
		{"invalid-login-string", validStatus("true", "True"), AccountStatus{}, false},
		{"invalid-premium-type", statusResponse(statusParam("logged_in", "1"), statusParam("has_unlimited_entitlement", true)), AccountStatus{}, false},
		{"invalid-premium-case", validStatus("1", "true"), AccountStatus{}, false},
		{"premium-without-login", validStatus("0", "True"), AccountStatus{}, false},
		{"conflicting-login", statusResponse(statusParam("logged_in", "1"), statusParam("logged_in", "0"), statusParam("has_unlimited_entitlement", "False")), AccountStatus{}, false},
		{"conflicting-premium", statusResponse(statusParam("logged_in", "1"), statusParam("has_unlimited_entitlement", "True"), statusParam("has_unlimited_entitlement", "False")), AccountStatus{}, false},
		{"identical-duplicates", statusResponse(statusParam("logged_in", "1"), statusParam("logged_in", "1"), statusParam("has_unlimited_entitlement", "True")), AccountStatus{true, true}, true},
		{"wrong-service", object{"responseContext": object{"serviceTrackingParams": []any{object{"service": "OTHER", "params": []any{statusParam("logged_in", "1"), statusParam("has_unlimited_entitlement", "True")}}}}}, AccountStatus{}, false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := parseAccountStatus(test.response)
			if test.valid {
				if err != nil || got != test.want {
					t.Fatal("explicit status was not preserved")
				}
			} else if !errors.Is(err, ErrAccountStatusUnavailable) {
				t.Fatal("unknown or conflicting status was accepted")
			}
		})
	}
	response := validStatus("1", "True")
	response["responseContext"].(object)["serviceTrackingParams"] = append(response["responseContext"].(object)["serviceTrackingParams"].([]any), object{"service": "GFEEDBACK", "params": []any{statusParam("logged_in", "0"), statusParam("has_unlimited_entitlement", "False")}})
	if _, err := parseAccountStatus(response); !errors.Is(err, ErrAccountStatusUnavailable) {
		t.Fatal("conflict across feedback blocks accepted")
	}
}

func TestAccountStatusUsesFixedAlbumAndDoesNotInferLoginFromCookies(t *testing.T) {
	client := fixtureClient(t, func(w http.ResponseWriter, req *http.Request) {
		var body object
		_ = json.NewDecoder(req.Body).Decode(&body)
		if req.URL.Path != "/youtubei/v1/browse" || body["browseId"] != "MPREb_4pL8gzRtw1p" || textAt(body, "context", "client", "clientName") != "WEB_REMIX" {
			t.Error("account status used the wrong service operation")
		}
		response := validStatus("0", "False")
		response["responseContext"].(object)["mainAppWebResponseContext"] = object{"loggedOut": true}
		_ = json.NewEncoder(w).Encode(response)
	})
	if err := client.SetSession(syntheticCookies(), 0, ""); err != nil {
		t.Fatal("synthetic session rejected")
	}
	status, err := client.AccountStatus(context.Background())
	if err != nil || status.LoggedIn || status.MusicPremium {
		t.Fatal("cookie presence was treated as verified login")
	}
	if _, ok := client.Credentials(); !ok {
		t.Fatal("status lookup unexpectedly destroyed credentials")
	}
}

func TestSelectedAccountHeadersAndPrivateCredentials(t *testing.T) {
	for _, index := range []int{0, 2, 99} {
		t.Run(strconv.Itoa(index), func(t *testing.T) {
			var requests atomic.Int32
			client := fixtureClient(t, func(w http.ResponseWriter, req *http.Request) {
				var body object
				_ = json.NewDecoder(req.Body).Decode(&body)
				if req.Header.Get("X-Goog-Authuser") != strconv.Itoa(index) || req.Header.Get("X-Goog-Pageid") != "synthetic_brand" || textAt(body, "context", "user", "onBehalfOfUser") != "synthetic_brand" || req.Header.Get("Authorization") == "" {
					t.Error("selected account context missing")
				}
				if strings.Contains(req.Header.Get("Cookie"), "external_cookie") {
					t.Error("nonservice cookie was transferred")
				}
				requests.Add(1)
				_ = json.NewEncoder(w).Encode(validStatus("1", "True"))
			})
			cookies := append(syntheticCookies(), &http.Cookie{Name: "external_cookie", Value: "fixture", Domain: "example.com"})
			if err := client.SetSession(cookies, index, "synthetic_brand"); err != nil {
				t.Fatal("valid account selection rejected")
			}
			cookies[0].Value = "mutated-after-transfer"
			credentials, ok := client.Credentials()
			if !ok || credentials.AccountIndex != index || credentials.OnBehalfOfUser != "synthetic_brand" || strings.Contains(credentials.Cookie, "mutated-after-transfer") {
				t.Fatal("session did not retain an independent selected snapshot")
			}
			status, err := client.AccountStatus(context.Background())
			if err != nil || !status.LoggedIn || !status.MusicPremium || requests.Load() != 1 {
				t.Fatal("selected account verification failed")
			}
		})
	}
}

func TestInvalidAccountSelectionPreservesExistingSession(t *testing.T) {
	client := New()
	if err := client.SetSession(syntheticCookies(), 2, "brand"); err != nil {
		t.Fatal("fixture session rejected")
	}
	before, _ := client.Credentials()
	for _, selection := range []struct {
		index int
		brand string
	}{{-1, ""}, {100, ""}, {0, "contains space"}, {0, "line\nbreak"}, {0, strings.Repeat("a", 129)}} {
		if err := client.SetSession(syntheticCookies(), selection.index, selection.brand); !errors.Is(err, ErrInvalidSession) {
			t.Fatal("unsafe account selection accepted")
		}
		after, ok := client.Credentials()
		if !ok || after != before {
			t.Fatal("invalid selection replaced existing session")
		}
	}
	for _, domain := range []string{"", "example.com", "youtube.com.example.com", "www.youtube.com", "youtube.com", "music.youtube.com", ".music.youtube.com"} {
		cookies := syntheticCookies()
		cookies[0].Domain = domain
		if err := client.SetSession(cookies, 0, ""); !errors.Is(err, ErrInvalidSession) {
			t.Fatal("cookie without applicable target scope accepted")
		}
	}
}

func TestSessionCookiesMatchPlayerContract(t *testing.T) {
	client := New()
	cookies := append(syntheticCookies(),
		&http.Cookie{Name: "PREF", Value: "fixture", Domain: ".youtube.com", Path: "/"},
		&http.Cookie{Name: "unknown_cookie", Value: "fixture", Domain: ".youtube.com"},
		&http.Cookie{Name: "SID", Value: "fixture", Domain: "music.youtube.com"},
		&http.Cookie{Name: "HSID", Value: "fixture", Domain: ".youtube.com", Path: "/youtubei"},
		&http.Cookie{Name: "SSID", Value: "fixture", Domain: ".youtube.com", Expires: time.Now().Add(-time.Second)})
	if err := client.SetSession(cookies, 0, ""); err != nil {
		t.Fatal("scoped synthetic session rejected")
	}
	credentials, ok := client.Credentials()
	if !ok || credentials.Cookie != "SAPISID=synthetic-test-only; PREF=fixture" {
		t.Fatal("cookie names, expiry or scope were not preserved")
	}
	for _, mutation := range []string{"alternate-auth", "duplicate", "space", "comma", "semicolon", "quote", "slash", "unicode", "empty", "value-limit", "header-limit", "expired-primary", "deleted-primary", "narrow-path"} {
		t.Run(mutation, func(t *testing.T) {
			cookies := syntheticCookies()
			switch mutation {
			case "alternate-auth":
				cookies[0].Name = "__Secure-3PAPISID"
			case "duplicate":
				cookies = append(cookies, syntheticCookies()[0])
			case "space":
				cookies[0].Value = "fixture value"
			case "comma":
				cookies[0].Value = "fixture,value"
			case "semicolon":
				cookies[0].Value = "fixture;value"
			case "quote":
				cookies[0].Value = "fixture\"value"
			case "slash":
				cookies[0].Value = "fixture\\value"
			case "unicode":
				cookies[0].Value = "fixture值"
			case "empty":
				cookies[0].Value = ""
			case "value-limit":
				cookies[0].Value = strings.Repeat("a", (16<<10)+1)
			case "header-limit":
				for _, name := range []string{"SID", "HSID", "SSID", "APISID"} {
					cookies = append(cookies, &http.Cookie{Name: name, Value: strings.Repeat("a", 16<<10), Domain: ".youtube.com"})
				}
			case "expired-primary":
				cookies[0].Expires = time.Now().Add(-time.Second)
			case "deleted-primary":
				cookies[0].MaxAge = -1
			case "narrow-path":
				cookies[0].Path = "/youtubei/v1"
			}
			if err := client.SetSession(cookies, 0, ""); !errors.Is(err, ErrInvalidSession) {
				t.Fatal("invalid flattened session accepted")
			}
			after, ok := client.Credentials()
			if !ok || after != credentials {
				t.Fatal("invalid cookie transfer replaced active account")
			}
		})
	}
	// MaxAge overrides Expires according to net/http's cookie contract.
	cookies = syntheticCookies()
	cookies[0].Expires, cookies[0].MaxAge = time.Now().Add(-time.Hour), 60
	if err := client.SetSession(cookies, 0, ""); err != nil {
		t.Fatal("positive max-age failed to override expires")
	}
}

func TestLateCatalogResultsCannotRepopulateSessionCaches(t *testing.T) {
	for _, operation := range []string{"page", "queue"} {
		t.Run(operation, func(t *testing.T) {
			entered, release := make(chan struct{}), make(chan struct{})
			client := fixtureClient(t, func(w http.ResponseWriter, req *http.Request) {
				close(entered)
				<-release
				response := queueResponse(true)
				if operation == "page" {
					response = catalogResponse(responsiveSong("abc123DEF_-", "First"))
					section := at(response, "contents", "singleColumnBrowseResultsRenderer").(object)["tabs"].([]any)[0].(object)["tabRenderer"].(object)["content"].(object)["sectionListRenderer"].(object)
					section["continuations"] = []any{object{"nextContinuationData": object{"continuation": "synthetic-upstream-token"}}}
				}
				_ = json.NewEncoder(w).Encode(response)
			})
			if err := client.SetSession(syntheticCookies(), 0, ""); err != nil {
				t.Fatal("fixture session rejected")
			}
			done := make(chan error, 1)
			go func() {
				if operation == "page" {
					page, err := client.Home(context.Background())
					if len(page.Items) != 0 || page.Continuation != "" {
						t.Error("stale page escaped session boundary")
					}
					done <- err
				} else {
					items, err := client.Queue(context.Background(), "abc123DEF_-")
					if len(items) != 0 {
						t.Error("stale queue escaped session boundary")
					}
					done <- err
				}
			}()
			select {
			case <-entered:
			case <-time.After(3 * time.Second):
				close(release)
				t.Fatal("fixture request did not start")
			}
			if err := client.SetSession(syntheticCookies(), 1, ""); err != nil {
				close(release)
				t.Fatal("replacement account rejected")
			}
			close(release)
			if err := <-done; !errors.Is(err, ErrSessionChanged) {
				t.Fatal("stale catalog request was accepted")
			}
			client.mu.RLock()
			cacheEntries := len(client.continuations) + len(client.lyrics)
			client.mu.RUnlock()
			if cacheEntries != 0 {
				t.Fatal("old account repopulated current account caches")
			}
		})
	}
}

func TestAccountNetworkErrorsDoNotDestroyCredentials(t *testing.T) {
	client := fixtureClient(t, func(w http.ResponseWriter, req *http.Request) { w.WriteHeader(http.StatusServiceUnavailable) })
	if err := client.SetSession(syntheticCookies(), 1, ""); err != nil {
		t.Fatal("fixture session rejected")
	}
	before, _ := client.Credentials()
	if _, err := client.AccountStatus(context.Background()); err == nil {
		t.Fatal("network failure became a logged-out state")
	}
	after, ok := client.Credentials()
	if !ok || after != before {
		t.Fatal("network failure cleared the account")
	}
}

func TestLateAccountResponseCannotCrossSessionChangeOrExpiry(t *testing.T) {
	for _, change := range []string{"clear", "replace", "expire"} {
		t.Run(change, func(t *testing.T) {
			entered, release := make(chan struct{}), make(chan struct{})
			client := fixtureClient(t, func(w http.ResponseWriter, req *http.Request) {
				close(entered)
				<-release
				_ = json.NewEncoder(w).Encode(validStatus("1", "True"))
			})
			if err := client.SetSession(syntheticCookies(), 0, ""); err != nil {
				t.Fatal("fixture session rejected")
			}
			done := make(chan error, 1)
			go func() { _, err := client.AccountStatus(context.Background()); done <- err }()
			select {
			case <-entered:
			case <-time.After(3 * time.Second):
				close(release)
				t.Fatal("fixture request did not start")
			}
			want := ErrSessionChanged
			switch change {
			case "clear":
				client.ClearSession()
			case "replace":
				if err := client.SetSession(syntheticCookies(), 3, ""); err != nil {
					t.Fatal("replacement session rejected")
				}
			case "expire":
				// Request authorization is complete before the fixture handler signals.
				client.mu.Lock()
				client.session.expires = time.Now().Add(-time.Second)
				client.mu.Unlock()
				want = ErrSessionExpired
			}
			close(release)
			if err := <-done; !errors.Is(err, want) {
				t.Fatal("late account response escaped session boundary")
			}
			if change == "expire" {
				if _, ok := client.Credentials(); ok {
					t.Fatal("expired session exported credentials")
				}
				if _, err := client.AccountStatus(context.Background()); !errors.Is(err, ErrSessionExpired) {
					t.Fatal("expired session silently became anonymous")
				}
			}
		})
	}
}

func TestAuthorizationNeverTargetsAnotherOrigin(t *testing.T) {
	client := New()
	if err := client.SetSession(syntheticCookies(), 4, ""); err != nil {
		t.Fatal("fixture session rejected")
	}
	for _, address := range []string{"http://music.youtube.com/youtubei/v1/browse", "https://www.youtube.com/youtubei/v1/browse", "https://music.youtube.com.example.com/", "https://music.youtube.com:444/"} {
		req, _ := http.NewRequest(http.MethodPost, address, nil)
		authorize(req, client.session)
		if req.Header.Get("Cookie") != "" || req.Header.Get("Authorization") != "" {
			t.Fatal("account credentials escaped the intended origin")
		}
	}
}
