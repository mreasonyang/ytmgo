package catalog

import (
	"net/url"
	"strings"
)

// redirectCategory reduces an untrusted Location to the same fixed vocabulary
// used by playback diagnostics. The destination is never retained or opened.
// An arbitrary redirect or HTTP rejection does not establish a challenge.
func redirectCategory(location string, source *url.URL) string {
	if location == "" || len(location) > 8192 || source == nil {
		return "other"
	}
	reference, err := url.Parse(location)
	if err != nil {
		return "other"
	}
	target := source.ResolveReference(reference)
	if target.User != nil || target.Opaque != "" {
		return "other"
	}
	// A lookalike host, insecure scheme, unusual port or ambiguous encoded
	// path must not produce an instruction to perform account verification.
	if target.Scheme == "https" && (target.Port() == "" || target.Port() == "443") && target.EscapedPath() == target.Path {
		host := strings.ToLower(target.Hostname())
		switch {
		case (host == "www.google.com" || host == "google.com") && pathWithin(target.Path, "/sorry"):
			return "google_challenge"
		case host == "accounts.google.com" && (pathWithin(target.Path, "/ServiceLogin") || pathWithin(target.Path, "/signin") || pathWithin(target.Path, "/v3/signin") || pathWithin(target.Path, "/InteractiveLogin")):
			return "signin"
		case (host == "www.youtube.com" || host == "youtube.com" || host == "music.youtube.com" || host == "www.google.com" || host == "google.com") && pathWithin(target.Path, "/signin"):
			return "signin"
		case (host == "consent.google.com" || host == "consent.youtube.com") && (target.Path == "/m" || target.Path == "/ml"):
			return "consent"
		}
	}
	if target.Scheme == source.Scheme && strings.EqualFold(target.Host, source.Host) {
		return "same_origin"
	}
	return "other"
}

func pathWithin(path, root string) bool {
	return path == root || strings.HasPrefix(path, root+"/")
}
