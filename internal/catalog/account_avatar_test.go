package catalog

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"
)

func avatarResponse(thumbnails ...any) object {
	return object{"actions": []any{object{"openPopupAction": object{"popup": object{"multiPageMenuRenderer": object{
		"header":   object{"activeAccountHeaderRenderer": object{"accountPhoto": object{"thumbnails": thumbnails}}},
		"sections": []any{object{"accountPhoto": object{"thumbnails": []any{object{"url": "https://yt3.ggpht.com/other-account"}}}}},
	}}}}}}
}

func TestAccountAvatarUsesSelectedSession(t *testing.T) {
	client := fixtureClient(t, func(w http.ResponseWriter, req *http.Request) {
		var body object
		_ = json.NewDecoder(req.Body).Decode(&body)
		if req.URL.Path != "/youtubei/v1/account/account_menu" || textAt(body, "context", "client", "clientName") != "WEB_REMIX" ||
			req.Header.Get("X-Goog-Authuser") != "2" || req.Header.Get("X-Goog-Pageid") != "synthetic_brand" || req.Header.Get("Authorization") == "" {
			t.Error("avatar request lost the selected account context")
		}
		_ = json.NewEncoder(w).Encode(avatarResponse(
			object{"url": "https://yt3.ggpht.com/small", "width": 32, "height": 32},
			object{"url": "https://yt3.ggpht.com/large", "width": 96, "height": 96},
		))
	})
	if err := client.SetSession(syntheticCookies(), 2, "synthetic_brand"); err != nil {
		t.Fatal("synthetic session rejected")
	}
	avatar, err := client.AccountAvatar(context.Background())
	if err != nil || avatar != "https://yt3.ggpht.com/large" {
		t.Fatal("active account avatar was not selected")
	}
}

func TestAccountAvatarRequiresSessionAndRejectsLateResponse(t *testing.T) {
	client := fixtureClient(t, func(w http.ResponseWriter, req *http.Request) {
		t.Error("anonymous avatar request reached the network")
	})
	if _, err := client.AccountAvatar(context.Background()); !errors.Is(err, ErrAuthenticationRequired) {
		t.Fatal("anonymous avatar request accepted")
	}
	client = fixtureClient(t, func(w http.ResponseWriter, req *http.Request) {
		client.ClearSession()
		_ = json.NewEncoder(w).Encode(avatarResponse(object{"url": "https://yt3.ggpht.com/old-account"}))
	})
	if client.SetSession(syntheticCookies(), 0, "") != nil {
		t.Fatal("synthetic session rejected")
	}
	if _, err := client.AccountAvatar(context.Background()); !errors.Is(err, ErrSessionChanged) {
		t.Fatal("avatar crossed an account boundary")
	}
}

func TestAccountAvatarOnlyReadsSafeActiveHeaderImage(t *testing.T) {
	for _, raw := range []string{"", "http://yt3.ggpht.com/photo", "https://yt3.ggpht.com.evil.test/photo", "https://user:password@yt3.ggpht.com/photo", "https://yt3.ggpht.com/photo?private=value"} {
		avatar, err := parseAccountAvatar(avatarResponse(object{"url": raw}))
		if err != nil || avatar != "" {
			t.Fatal("unsafe or absent avatar did not fall back")
		}
	}
	if avatar, err := parseAccountAvatar(object{}); avatar != "" || !errors.Is(err, ErrInvalidResponse) {
		t.Fatal("missing active account header accepted")
	}
}
