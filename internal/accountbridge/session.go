// Package accountbridge accepts an explicitly selected browser session over a
// short-lived, origin-bound loopback channel. It never discovers browser stores.
package accountbridge

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/http"
	"regexp"
	"time"
)

var ErrSession = errors.New("account_session_invalid")
var ErrExpired = errors.New("account_session_expired")

const MaxPayload = 128 * 1024

var cookieNames = map[string]bool{
	"SAPISID": true, "APISID": true, "SID": true, "HSID": true, "SSID": true,
	"LOGIN_INFO": true, "SIDCC": true, "__Secure-1PAPISID": true, "__Secure-3PAPISID": true,
	"__Secure-1PSID": true, "__Secure-3PSID": true, "__Secure-1PSIDTS": true,
	"__Secure-3PSIDTS": true, "__Secure-1PSIDCC": true, "__Secure-3PSIDCC": true,
	"PREF": true, "VISITOR_INFO1_LIVE": true, "VISITOR_PRIVACY_METADATA": true,
	"YSC": true, "CONSENT": true, "SOCS": true,
}
var delegatedPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)

type Cookie struct {
	Name           string   `json:"name"`
	Value          string   `json:"value"`
	Domain         string   `json:"domain"`
	Path           string   `json:"path"`
	Secure         *bool    `json:"secure"`
	ExpirationDate *float64 `json:"expirationDate,omitempty"`
}

type Session struct {
	Cookies        []Cookie `json:"cookies"`
	AccountIndex   *int     `json:"accountIndex"`
	OnBehalfOfUser string   `json:"onBehalfOfUser,omitempty"`
}

func Decode(data []byte, now time.Time) (Session, error) {
	var s Session
	if len(data) > MaxPayload {
		return s, ErrSession
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if d.Decode(&s) != nil {
		return Session{}, ErrSession
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return Session{}, ErrSession
	}
	if _, err := s.HTTPCookies(now); err != nil {
		return Session{}, err
	}
	return s, nil
}

func (s Session) HTTPCookies(now time.Time) ([]*http.Cookie, error) {
	if s.AccountIndex == nil || *s.AccountIndex < 0 || *s.AccountIndex > 99 || len(s.Cookies) == 0 || len(s.Cookies) > 100 ||
		(s.OnBehalfOfUser != "" && !delegatedPattern.MatchString(s.OnBehalfOfUser)) {
		return nil, ErrSession
	}
	seen := map[string]string{}
	size := 0
	requiredExpired := false
	nowSeconds := float64(now.Unix()) + float64(now.Nanosecond())/1e9
	result := make([]*http.Cookie, 0, len(s.Cookies))
	for _, c := range s.Cookies {
		if !cookieNames[c.Name] || c.Domain != ".youtube.com" || c.Path != "/" || c.Secure == nil || !cookieValue(c.Value) {
			return nil, ErrSession
		}
		size += len(c.Name) + len(c.Value)
		if size > 64*1024 {
			return nil, ErrSession
		}
		h := &http.Cookie{Name: c.Name, Value: c.Value, Domain: c.Domain, Path: c.Path, Secure: *c.Secure}
		if c.ExpirationDate != nil {
			e := *c.ExpirationDate
			if math.IsNaN(e) || math.IsInf(e, 0) || e > 253402300799 {
				return nil, ErrSession
			}
			if e <= nowSeconds {
				if c.Name == "SAPISID" {
					requiredExpired = true
				}
				continue
			}
			seconds, fraction := math.Modf(e)
			h.Expires = time.Unix(int64(seconds), int64(fraction*1e9))
		}
		if previous, ok := seen[c.Name]; ok {
			if previous != c.Value {
				return nil, ErrSession
			}
			continue
		}
		seen[c.Name] = c.Value
		result = append(result, h)
	}
	if requiredExpired {
		return nil, ErrExpired
	}
	if _, ok := seen["SAPISID"]; !ok {
		return nil, ErrSession
	}
	return result, nil
}

func cookieValue(v string) bool {
	if len(v) == 0 {
		return false
	}
	for i := 0; i < len(v); i++ {
		c := v[i]
		if c < 0x21 || c > 0x7e || c == '"' || c == ',' || c == ';' || c == '\\' {
			return false
		}
	}
	return true
}
