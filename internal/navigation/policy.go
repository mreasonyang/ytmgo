// Package navigation defines the small set of pages allowed inside ytmgo.
package navigation

import (
	"net/url"

	"ytmgo/internal/language"
)

const Home = "https://music.youtube.com/"

// SignOutAction opens a native confirmation, never an unguarded data deletion.
func SignOutAction(raw, current string, userInitiated bool) bool {
	return userInitiated && MusicContent(current) && raw == "ytmgo://session/signout"
}

// AccountStateAction accepts a display-state boolean, never account details.
// The page cannot trigger deletion this way; sign-out still needs confirmation.
func AccountStateAction(raw, current string) (bool, bool) {
	if !MusicContent(current) {
		return false, false
	}
	switch raw {
	case "ytmgo://account/signed-in":
		return true, true
	case "ytmgo://account/signed-out":
		return false, true
	}
	return false, false
}

func MusicContent(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && u.User == nil && u.Scheme == "https" && u.Host == "music.youtube.com" && u.Opaque == ""
}

// MusicPage keeps first-party music and sign-in flows in the app's persistent
// WebView. These origins are never granted native IPC access.
func MusicPage(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.User != nil || u.Scheme != "https" || u.Port() != "" || u.Opaque != "" {
		return false
	}
	switch u.Host {
	case "music.youtube.com", "accounts.google.com", "accounts.youtube.com", "consent.google.com", "consent.youtube.com":
		return true
	case "www.youtube.com":
		return u.Path == "/signin" || u.Path == "/logout" || u.Path == "/signin_prompt"
	}
	return false
}

// AppearanceAction accepts only the three presentation preferences. No remote
// page receives a general-purpose native bridge or access to local libraries.
func AppearanceAction(raw, current string) (string, bool) {
	page, err := url.Parse(current)
	if err != nil || page.Scheme != "https" || page.Host != "music.youtube.com" || page.User != nil {
		return "", false
	}
	for _, mode := range []string{"light", "dark", "system"} {
		if raw == "ytmgo://appearance/"+mode {
			return mode, true
		}
	}
	return "", false
}

// LanguageAction accepts only "system" or a supported interface language,
// requested by the music page itself (the shell's settings popover).
func LanguageAction(raw, current string) (string, bool) {
	if !MusicContent(current) {
		return "", false
	}
	for _, choice := range append([]string{language.System}, language.Supported...) {
		if raw == "ytmgo://language/"+choice {
			return choice, true
		}
	}
	return "", false
}

// External permits only ordinary web links; never send arbitrary URL schemes
// (or URLs with embedded credentials) to the operating system.
func External(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && u.User == nil && u.Opaque == "" && u.Hostname() != "" && (u.Scheme == "https" || u.Scheme == "http")
}
