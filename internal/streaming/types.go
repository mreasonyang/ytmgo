// Package streaming owns playback protocol, network and media leases in Go.
// It never opens an audio device. Dynamic code runs in a bounded native helper
// which receives no account cookies and has no application network interface.
package streaming

import (
	"context"
	"crypto/sha1" // The service's SAPISIDHASH protocol requires SHA-1.
	"encoding/hex"
	"errors"
	"net/http"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const maxResponse = 16 << 20
const userAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/144.0.0.0 Safari/537.36"

// Proof challenges, their interpreter and its Window must use the same BgUtils
// agent contract. A successfully minted token with the wrong agent can still
// be rejected by the CDN after the initial playable range.
const proofUserAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36(KHTML, like Gecko)"
const musicOrigin = "https://music.youtube.com"
const musicVersion = "1.20250219.01.00"

var videoID = regexp.MustCompile(`^[A-Za-z0-9_-]{11}$`)
var identifier = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)
var cookieNames = map[string]bool{"SAPISID": true, "APISID": true, "SID": true, "HSID": true, "SSID": true, "LOGIN_INFO": true, "SIDCC": true, "__Secure-1PAPISID": true, "__Secure-3PAPISID": true, "__Secure-1PSID": true, "__Secure-3PSID": true, "__Secure-1PSIDTS": true, "__Secure-3PSIDTS": true, "__Secure-1PSIDCC": true, "__Secure-3PSIDCC": true, "PREF": true, "VISITOR_INFO1_LIVE": true, "VISITOR_PRIVACY_METADATA": true, "YSC": true, "CONSENT": true, "SOCS": true}

// Session is private in-memory input. Never format, log or persist it here.
type Session struct {
	LoggedIn       bool   `json:"loggedIn"`
	MusicPremium   bool   `json:"musicPremium"`
	Cookie         string `json:"cookie"`
	AccountIndex   int    `json:"accountIndex"`
	OnBehalfOfUser string `json:"onBehalfOfUser"`
	Revision       uint64 `json:"revision"`
}

func (s Session) Validate() error {
	if s.Revision == 0 || s.Revision > 9007199254740991 || s.AccountIndex < 0 || s.AccountIndex > 99 || len(s.Cookie) > 64<<10 || len(s.OnBehalfOfUser) > 128 || s.OnBehalfOfUser != "" && !identifier.MatchString(s.OnBehalfOfUser) {
		return errors.New("invalid_account_session")
	}
	if !s.LoggedIn {
		if s.MusicPremium || s.Cookie != "" || s.AccountIndex != 0 || s.OnBehalfOfUser != "" {
			return errors.New("unexpected_authentication")
		}
		return nil
	}
	seen := map[string]bool{}
	for _, part := range strings.Split(s.Cookie, ";") {
		name, value, ok := strings.Cut(strings.TrimSpace(part), "=")
		if !ok || !cookieNames[name] || seen[name] || len(value) == 0 || len(value) > 16<<10 {
			return errors.New("unexpected_authentication")
		}
		for _, b := range []byte(value) {
			if !(b == 0x21 || b >= 0x23 && b <= 0x2b || b >= 0x2d && b <= 0x3a || b >= 0x3c && b <= 0x5b || b >= 0x5d && b <= 0x7e) {
				return errors.New("unexpected_authentication")
			}
		}
		seen[name] = true
	}
	if !seen["SAPISID"] {
		return errors.New("login_required")
	}
	return nil
}
func (s Session) authorize(req *http.Request, api bool) {
	if !s.LoggedIn || req.URL.Scheme != "https" || req.URL.Host != "music.youtube.com" || req.URL.User != nil {
		return
	}
	req.Header.Set("Cookie", s.Cookie)
	req.Header.Set("X-Goog-Authuser", strconv.Itoa(s.AccountIndex))
	if s.OnBehalfOfUser != "" {
		req.Header.Set("X-Goog-Pageid", s.OnBehalfOfUser)
	}
	if !api {
		return
	}
	var sid string
	for _, p := range strings.Split(s.Cookie, ";") {
		k, v, _ := strings.Cut(strings.TrimSpace(p), "=")
		if k == "SAPISID" {
			sid = v
		}
	}
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	sum := sha1.Sum([]byte(timestamp + " " + sid + " " + musicOrigin))
	req.Header.Set("Authorization", "SAPISIDHASH "+timestamp+"_"+hex.EncodeToString(sum[:]))
	req.Header.Set("X-Origin", musicOrigin)
}

// Failure carries only bounded categories. Upstream errors are never wrapped.
type Failure struct {
	Category, Stage, BootstrapStep, Redirect, MIME string
	ScriptReason                                   string
	HTTPStatus                                     int
}

func (e *Failure) Error() string           { return e.Category }
func failure(category, stage string) error { return &Failure{Category: category, Stage: stage} }

type Options struct {
	Command func(context.Context, string) *exec.Cmd
	Client  *http.Client
}
type Media struct {
	Lease         *Lease
	Duration      time.Duration
	Width, Height int
}
type extracted struct {
	Code      string `json:"code"`
	Timestamp int    `json:"timestamp"`
}
type proofBinding struct {
	Visitor, Binding string
	VideoBound       bool
}
