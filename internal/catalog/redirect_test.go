package catalog

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestRedirectCategoriesRequireTrustedHTTPSDestinations(t *testing.T) {
	source, _ := url.Parse(serviceOrigin + "/youtubei/v1/browse")
	for _, tc := range []struct{ location, want string }{
		{"https://www.google.com/sorry/index?continue=synthetic-private", "google_challenge"},
		{"https://google.com/sorry", "google_challenge"},
		{"https://www.google.com:443/sorry/index", "google_challenge"},
		{"https://accounts.google.com/ServiceLogin?continue=synthetic-private", "signin"},
		{"https://accounts.google.com/v3/signin/identifier", "signin"},
		{"https://accounts.google.com/signin/v2/identifier", "signin"},
		{"/signin?continue=synthetic-private", "signin"},
		{"https://consent.youtube.com/m?continue=synthetic-private", "consent"},
		{"https://consent.google.com/ml", "consent"},
		{"/other?synthetic-private=value", "same_origin"},
		{"", "other"},
		{"https://example.invalid/sorry/index", "other"},
		{"https://www.google.com.example.invalid/sorry/index", "other"},
		{"https://www.google.com@other.invalid/sorry/index", "other"},
		{"https://synthetic-private@www.google.com/sorry/index", "other"},
		{"http://www.google.com/sorry/index", "other"},
		{"https://www.google.com:8443/sorry/index", "other"},
		{"https://www.google.com/sorryish", "other"},
		{"https://www.google.com/%73orry/index", "other"},
		{"https://accounts.google.com/unknown", "other"},
		{"https://accounts.google.com/ServiceLoginOther", "other"},
		{"https://consent.youtube.com/unknown", "other"},
		{"https://consent.youtube.com.example.invalid/m", "other"},
		{"javascript:synthetic-private", "other"},
		{"%", "other"},
	} {
		if got := redirectCategory(tc.location, source); got != tc.want {
			t.Errorf("redirect classification: got %q, want %q", got, tc.want)
		}
	}
	if redirectCategory("/sorry", nil) != "other" || redirectCategory(strings.Repeat("x", 8193), source) != "other" {
		t.Fatal("invalid or oversized redirect was accepted")
	}
}

func TestCatalogChallengePreservesOnlyClosedMetadataAndSession(t *testing.T) {
	for _, signedIn := range []bool{false, true} {
		for _, tc := range []struct {
			status   int
			location string
			category string
		}{
			{302, "https://www.google.com/sorry/index?continue=synthetic-private", "google_challenge"},
			{303, "https://accounts.google.com/ServiceLogin?synthetic-private=value", "signin"},
			{307, "https://consent.youtube.com/m?synthetic-private=value", "consent"},
			{302, "https://example.invalid/sorry/index?synthetic-private=value", "other"},
			{302, "", "other"},
			{403, "https://www.google.com/sorry/index", ""},
			{429, "https://www.google.com/sorry/index", ""},
		} {
			calls := 0
			client := NewWithOptions(Options{HTTPClient: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				calls++
				response := directAPIResponse(req, tc.status, "synthetic-private-response-body")
				response.Header.Set("Location", tc.location)
				return response, nil
			})}})
			if signedIn && client.SetSession(syntheticCookies(), 2, "") != nil {
				t.Fatal("synthetic account setup failed")
			}
			_, err := client.AccountStatus(context.Background())
			var failure *HTTPError
			if !errors.As(err, &failure) || failure.Stage != "browse" || failure.StatusCode != tc.status || failure.Redirect != tc.category || calls != 1 {
				t.Fatal("redirect was followed, retried, or lost its fixed metadata")
			}
			data, marshalErr := json.Marshal(failure)
			if marshalErr != nil || strings.Contains(string(data), "synthetic-private") || strings.Contains(err.Error(), "synthetic-private") || strings.Contains(string(data), "https:") {
				t.Fatal("upstream response or destination escaped the error boundary")
			}
			_, hasSession := client.Credentials()
			if hasSession != signedIn {
				t.Fatal("service rejection changed the saved account")
			}
			want := ErrUnavailable
			if tc.status == 403 {
				want = ErrAuthenticationRequired
			}
			if !errors.Is(err, want) {
				t.Fatal("fixed redirect metadata changed existing error identity")
			}
		}
	}
}
