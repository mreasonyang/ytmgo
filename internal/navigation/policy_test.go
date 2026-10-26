package navigation

import "testing"

func TestAccountDisplayStateCannotCarryAccountData(t *testing.T) {
	for _, tc := range []struct {
		action   string
		signedIn bool
	}{{"signed-in", true}, {"signed-out", false}} {
		got, ok := AccountStateAction("ytmgo://account/"+tc.action, Home)
		if !ok || got != tc.signedIn {
			t.Fatal("valid display state rejected")
		}
	}
	for _, raw := range []string{"ytmgo://account/signed-in?token=synthetic", "ytmgo://account/signed-in#x", "ytmgo://account/signed-in/", "ytmgo://account/clear", "ytmgo://account/%73igned-in"} {
		if _, ok := AccountStateAction(raw, Home); ok {
			t.Fatal("unexpected payload allowed")
		}
	}
	for _, current := range []string{"https://accounts.google.com/", "https://music.youtube.com.evil.example/", "https://music.youtube.com:443/", "http://music.youtube.com/", "https://user@music.youtube.com/"} {
		if _, ok := AccountStateAction("ytmgo://account/signed-in", current); ok {
			t.Fatal("untrusted page allowed")
		}
	}
}

func TestSignOutRequiresExactActionMusicOriginAndGesture(t *testing.T) {
	if !SignOutAction("ytmgo://session/signout", Home+"library", true) {
		t.Fatal("sidebar action rejected")
	}
	if SignOutAction("ytmgo://session/signout", Home, false) {
		t.Fatal("automatic request allowed")
	}
	for _, origin := range []string{"https://accounts.google.com/", "https://music.youtube.com.evil.example/", "https://music.youtube.com:443/", "http://music.youtube.com/", "https://user@music.youtube.com/", "about:blank"} {
		if SignOutAction("ytmgo://session/signout", origin, true) {
			t.Errorf("untrusted origin allowed: %s", origin)
		}
	}
	for _, action := range []string{"ytmgo://session/signout?all=true", "ytmgo://session/signout#fragment", "ytmgo://session/signout/", "ytmgo://session/%73ignout", "ytmgo://session/retry", "ytmgo://session/continue"} {
		if SignOutAction(action, Home, true) {
			t.Errorf("unexpected native action: %s", action)
		}
	}
}

func TestExternalNavigation(t *testing.T) {
	for _, raw := range []string{"https://support.google.com/youtubemusic", "http://example.org/"} {
		if !External(raw) {
			t.Errorf("rejected web URL %q", raw)
		}
	}
	for _, raw := range []string{"javascript:alert(1)", "file:///tmp/a", "https://user:password@example.org/", "https:///missing-host", "ytmgo-local://app/help", "mailto:a@example.org"} {
		if External(raw) {
			t.Errorf("accepted unsafe external URL %q", raw)
		}
	}
}

func TestEmbeddedPlayerNavigation(t *testing.T) {
	const origin = "https://si.kyun.ytmgo"
	const valid = "https://www.youtube.com/embed/60HN1rIvht4?autoplay=1&controls=1&enablejsapi=1&origin=https%3A%2F%2Fsi.kyun.ytmgo&widgetid=1"
	if !EmbeddedPlayer(valid, origin, false) {
		t.Fatal("automatic official player was blocked")
	}
	if EmbeddedPlayer(valid, origin, true) {
		t.Fatal("user-followed navigation must stay external")
	}
	for _, raw := range []string{
		"http://www.youtube.com/embed/60HN1rIvht4?enablejsapi=1&origin=" + origin,
		"https://www.youtube.com.evil.example/embed/60HN1rIvht4?enablejsapi=1&origin=" + origin,
		"https://www.youtube.com:443/embed/60HN1rIvht4?enablejsapi=1&origin=" + origin,
		"https://user@www.youtube.com/embed/60HN1rIvht4?enablejsapi=1&origin=" + origin,
		"https://www.youtube.com/watch?v=60HN1rIvht4&enablejsapi=1&origin=" + origin,
		"https://www.youtube.com/embed/short?enablejsapi=1&origin=" + origin,
		"https://www.youtube.com/embed/60HN1rIvht4/extra?enablejsapi=1&origin=" + origin,
		"https://www.youtube.com/embed/%36%30HN1rIvht4?enablejsapi=1&origin=" + origin,
		"https://www.youtube.com/embed/60HN1rIvht4?origin=" + origin,
		"https://www.youtube.com/embed/60HN1rIvht4?enablejsapi=1&origin=https://example.org",
		valid + "&origin=https://example.org",
		valid + "&enablejsapi=0",
		valid + "#fragment",
		valid + "&invalid=%zz",
	} {
		if EmbeddedPlayer(raw, origin, false) {
			t.Errorf("unexpected frame navigation allowed: %q", raw)
		}
	}
}

func TestMusicAccountNavigation(t *testing.T) {
	for _, raw := range []string{Home, "https://music.youtube.com/library", "https://accounts.google.com/ServiceLogin?continue=https%3A%2F%2Fmusic.youtube.com", "https://accounts.youtube.com/accounts/SetSID", "https://consent.youtube.com/m", "https://www.youtube.com/signin?action_handle_signin=true", "https://www.youtube.com/logout"} {
		if !MusicPage(raw) {
			t.Errorf("first-party flow blocked: %s", raw)
		}
	}
	for _, raw := range []string{"http://music.youtube.com/", "https://music.youtube.com:443/", "https://user@music.youtube.com/", "https://music.youtube.com.evil.example/", "https://accounts.google.com.evil.example/", "https://www.youtube.com/watch?v=test", "https://example.com/", "file:///tmp/", "javascript:void(0)"} {
		if MusicPage(raw) {
			t.Errorf("untrusted page allowed: %s", raw)
		}
	}
}
func TestAppearanceOriginAndActions(t *testing.T) {
	for _, mode := range []string{"light", "dark", "system"} {
		if got, ok := AppearanceAction("ytmgo://appearance/"+mode, Home); !ok || got != mode {
			t.Errorf("mode %s rejected", mode)
		}
	}
	for _, origin := range []string{"https://accounts.google.com/", "https://music.youtube.com:443/", "https://music.youtube.com.evil.example/", "https://user@music.youtube.com/", "http://music.youtube.com/"} {
		if _, ok := AppearanceAction("ytmgo://appearance/light", origin); ok {
			t.Errorf("action allowed from %s", origin)
		}
	}
	for _, action := range []string{"ytmgo://appearance/light?x=y", "ytmgo://appearance/light#x", "ytmgo://appearance/light/", "ytmgo://appearance/invalid", "ytmgo://library/read", "ytmgo://appearance/%6cight"} {
		if _, ok := AppearanceAction(action, Home); ok {
			t.Errorf("invalid action allowed: %s", action)
		}
	}
}
