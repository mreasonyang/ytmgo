package catalog

import (
	"context"
	"crypto/sha1" // Required by the upstream SAPISIDHASH protocol.
	"encoding/hex"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// SessionCredentials is an explicit private, in-memory handoff. It must never
// be formatted, logged or persisted. Availability is not proof of login.
type SessionCredentials struct {
	Cookie         string
	AccountIndex   int
	OnBehalfOfUser string
}

type sessionCookie struct {
	name, value string
	expires     time.Time
}
type accountSession struct {
	cookies        []sessionCookie
	sapisid        string
	expires        time.Time
	accountIndex   int
	onBehalfOfUser string
}
type sessionScopeKey struct{}
type sessionScope struct {
	client     *Client
	generation uint64
	session    *accountSession
}

// Keep this list aligned with runtime/account-session.mjs. The same private
// header is transferred to the catalog and the player's first-party APIs.
var sessionCookieNames = map[string]struct{}{
	"SAPISID": {}, "APISID": {}, "SID": {}, "HSID": {}, "SSID": {}, "LOGIN_INFO": {}, "SIDCC": {},
	"__Secure-1PAPISID": {}, "__Secure-3PAPISID": {}, "__Secure-1PSID": {}, "__Secure-3PSID": {},
	"__Secure-1PSIDTS": {}, "__Secure-3PSIDTS": {}, "__Secure-1PSIDCC": {}, "__Secure-3PSIDCC": {},
	"PREF": {}, "VISITOR_INFO1_LIVE": {}, "VISITOR_PRIVACY_METADATA": {}, "YSC": {}, "CONSENT": {}, "SOCS": {},
}

// SetSession installs an explicitly authorized first-party session in memory.
// It does not inspect browser profiles or credential stores. Invalid input
// leaves the current session intact. AccountStatus verifies service acceptance.
func (c *Client) SetSession(cookies []*http.Cookie, accountIndex int, onBehalfOfUser string) error {
	if len(cookies) > 100 || accountIndex < 0 || accountIndex > 99 || len(onBehalfOfUser) > 128 {
		return ErrInvalidSession
	}
	for _, ch := range onBehalfOfUser {
		if !(ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z' || ch >= '0' && ch <= '9' || ch == '_' || ch == '-') {
			return ErrInvalidSession
		}
	}
	now := time.Now()
	session := &accountSession{accountIndex: accountIndex, onBehalfOfUser: onBehalfOfUser}
	var primary *sessionCookie
	names := make(map[string]struct{})
	for _, cookie := range cookies {
		if cookie == nil {
			continue
		}
		// A flattened header cannot preserve host-only or narrower path scope.
		// Require an explicit shared-domain cookie, rather than broadening it.
		if strings.ToLower(cookie.Domain) != ".youtube.com" {
			continue
		}
		path := cookie.Path
		if path != "" && path != "/" {
			continue
		}
		if _, allowed := sessionCookieNames[cookie.Name]; !allowed {
			continue
		}
		if !validSessionCookieValue(cookie.Value) || cookie.Valid() != nil {
			return ErrInvalidSession
		}
		expires := cookie.Expires
		if cookie.MaxAge > 0 {
			if int64(cookie.MaxAge) > int64((1<<63-1)/int64(time.Second)) {
				return ErrInvalidSession
			}
			expires = now.Add(time.Duration(cookie.MaxAge) * time.Second)
		}
		if cookie.MaxAge < 0 || !expires.IsZero() && !now.Before(expires) {
			continue
		}
		if _, exists := names[cookie.Name]; exists {
			return ErrInvalidSession
		}
		names[cookie.Name] = struct{}{}
		entry := sessionCookie{cookie.Name, cookie.Value, expires}
		session.cookies = append(session.cookies, entry)
		if cookie.Name == "SAPISID" {
			primary = &entry
		}
	}
	if primary == nil {
		return ErrInvalidSession
	}
	// Do not alias another cookie to SAPISID: the player SDK uses this exact
	// cookie, so accepting a different identity here would split the session.
	session.sapisid, session.expires = primary.value, primary.expires
	if len(session.cookieHeader(now)) > 64<<10 {
		return ErrInvalidSession
	}
	c.mu.Lock()
	c.session, c.sessionGeneration = session, c.sessionGeneration+1
	clear(c.continuations)
	clear(c.lyrics)
	c.mu.Unlock()
	return nil
}

func validSessionCookieValue(value string) bool {
	if len(value) == 0 || len(value) > 16<<10 {
		return false
	}
	for i := range len(value) {
		ch := value[i]
		// RFC 6265 cookie-octet; no quoting or sanitizing after validation.
		if !(ch == 0x21 || ch >= 0x23 && ch <= 0x2b || ch >= 0x2d && ch <= 0x3a || ch >= 0x3c && ch <= 0x5b || ch >= 0x5d && ch <= 0x7e) {
			return false
		}
	}
	return true
}

func (s *accountSession) cookieHeader(now time.Time) string {
	parts := make([]string, 0, len(s.cookies))
	for _, cookie := range s.cookies {
		if cookie.expires.IsZero() || now.Before(cookie.expires) {
			// Already validated; avoid Cookie.String logging invalid values.
			parts = append(parts, cookie.name+"="+cookie.value)
		}
	}
	return strings.Join(parts, "; ")
}

// Credentials reports usable local credentials, not a server-verified login.
func (c *Client) Credentials() (SessionCredentials, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	session, now := c.session, time.Now()
	if session == nil || !session.expires.IsZero() && !now.Before(session.expires) {
		return SessionCredentials{}, false
	}
	return SessionCredentials{session.cookieHeader(now), session.accountIndex, session.onBehalfOfUser}, true
}

func (c *Client) ClearSession() {
	c.mu.Lock()
	c.session, c.sessionGeneration = nil, c.sessionGeneration+1
	clear(c.continuations)
	clear(c.lyrics)
	c.mu.Unlock()
}

func (c *Client) operation(ctx context.Context) (context.Context, sessionScope, error) {
	if err := ctx.Err(); err != nil {
		return ctx, sessionScope{}, err
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	scope, exists := ctx.Value(sessionScopeKey{}).(sessionScope)
	if !exists || scope.client != c {
		scope = sessionScope{c, c.sessionGeneration, c.session}
		ctx = context.WithValue(ctx, sessionScopeKey{}, scope)
	}
	return ctx, scope, c.scopeErrorLocked(scope)
}
func (c *Client) scopeErrorLocked(scope sessionScope) error {
	if scope.generation != c.sessionGeneration || scope.session != c.session {
		return ErrSessionChanged
	}
	if scope.session != nil && !scope.session.expires.IsZero() && !time.Now().Before(scope.session.expires) {
		return ErrSessionExpired
	}
	return nil
}
func (c *Client) scopeError(scope sessionScope) error {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.scopeErrorLocked(scope)
}

func authorize(req *http.Request, session *accountSession) {
	if session == nil || req.URL.Scheme != "https" || req.URL.Host != "music.youtube.com" || req.URL.User != nil {
		return
	}
	now := time.Now()
	if !session.expires.IsZero() && !now.Before(session.expires) {
		return
	}
	timestamp := strconv.FormatInt(now.Unix(), 10)
	signature := sha1.Sum([]byte(timestamp + " " + session.sapisid + " " + serviceOrigin))
	req.Header.Set("Authorization", "SAPISIDHASH "+timestamp+"_"+hex.EncodeToString(signature[:]))
	req.Header.Set("Cookie", session.cookieHeader(now))
	req.Header.Set("X-Goog-Authuser", strconv.Itoa(session.accountIndex))
	if session.onBehalfOfUser != "" {
		req.Header.Set("X-Goog-Pageid", session.onBehalfOfUser)
	}
	req.Header.Set("X-Origin", serviceOrigin)
}
