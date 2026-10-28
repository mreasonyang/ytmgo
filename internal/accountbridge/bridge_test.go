package accountbridge

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func fixture() Session {
	i := 2
	secure := true
	return Session{AccountIndex: &i, Cookies: []Cookie{{Name: "SAPISID", Value: "synthetic-only", Domain: ".youtube.com", Path: "/", Secure: &secure}}}
}
func request(b *Bridge, method, path, origin, token string, body []byte) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, "http://"+Address+path, bytes.NewReader(body))
	r.Header.Set("Origin", origin)
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("X-Ytmgo-Handoff", token)
	w := httptest.NewRecorder()
	b.ServeHTTP(w, r)
	return w
}
func TestBridgeOriginTokenOneUseAndExpiry(t *testing.T) {
	b, err := newBridge()
	if err != nil {
		t.Fatal("bridge construction failed")
	}
	defer b.Close()
	data, _ := json.Marshal(fixture())
	for _, origin := range []string{"", "null", "https://music.youtube.com", "chrome-extension://untrusted"} {
		if request(b, http.MethodPost, "/challenge", origin, "", nil).Code != 403 {
			t.Fatal("foreign origin accepted")
		}
	}
	challenge := request(b, http.MethodPost, "/challenge", Origin, "", nil)
	var value struct {
		Token string `json:"token"`
	}
	if json.Unmarshal(challenge.Body.Bytes(), &value) != nil || len(value.Token) != 43 {
		t.Fatal("challenge unavailable")
	}
	if request(b, http.MethodPost, "/session", Origin, "synthetic-wrong", data).Code != 403 {
		t.Fatal("wrong handshake accepted")
	}
	if request(b, http.MethodPost, "/session?secret=forbidden", Origin, value.Token, data).Code != 400 {
		t.Fatal("query accepted")
	}
	if request(b, http.MethodPost, "/session", Origin, value.Token, data).Code != 200 {
		t.Fatal("valid transfer rejected")
	}
	if request(b, http.MethodPost, "/session", Origin, value.Token, data).Code != 410 {
		t.Fatal("transfer reused")
	}
	if request(b, http.MethodPost, "/challenge", Origin, "", nil).Code != 410 {
		t.Fatal("spent handshake exposed")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	s, err := b.Wait(ctx)
	if err != nil || *s.AccountIndex != 2 {
		t.Fatal("session was not delivered")
	}
	expired, _ := newBridge()
	defer expired.Close()
	expired.deadline = time.Now().Add(-time.Second)
	if request(expired, http.MethodPost, "/challenge", Origin, "", nil).Code != 410 {
		t.Fatal("expired challenge accepted")
	}
}
func TestBridgeRejectsHostAndOversizedBody(t *testing.T) {
	b, _ := newBridge()
	defer b.Close()
	r := httptest.NewRequest("GET", "http://evil.test/challenge", nil)
	r.Header.Set("Origin", Origin)
	w := httptest.NewRecorder()
	b.ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatal("foreign Host accepted")
	}
	if request(b, "POST", "/session", Origin, b.token, []byte(strings.Repeat("x", MaxPayload+1))).Code != 413 {
		t.Fatal("oversized body accepted")
	}
}
func TestSessionValidation(t *testing.T) {
	now := time.Now()
	valid := fixture()
	data, _ := json.Marshal(valid)
	if _, err := Decode(data, now); err != nil {
		t.Fatal("valid synthetic session rejected")
	}
	for _, change := range []func(*Session){
		func(s *Session) { s.AccountIndex = nil }, func(s *Session) { i := -1; s.AccountIndex = &i }, func(s *Session) { i := 100; s.AccountIndex = &i },
		func(s *Session) { s.OnBehalfOfUser = strings.Repeat("a", 129) }, func(s *Session) { s.OnBehalfOfUser = "bad value" },
		func(s *Session) { s.Cookies[0].Domain = "youtube.com" }, func(s *Session) { s.Cookies[0].Domain = "music.youtube.com" },
		func(s *Session) { s.Cookies[0].Domain = ".www.youtube.com" }, func(s *Session) { s.Cookies[0].Path = "/account" },
		func(s *Session) { s.Cookies[0].Name = "unapproved" }, func(s *Session) { s.Cookies[0].Secure = nil },
		func(s *Session) { s.Cookies[0].Value = "bad;value" }, func(s *Session) { s.Cookies[0].Value = "bad\nvalue" },
		func(s *Session) {
			expiry := float64(now.Add(-time.Minute).Unix())
			s.Cookies[0].ExpirationDate = &expiry
		},
		func(s *Session) {
			s.Cookies = append(s.Cookies, Cookie{Name: "SAPISID", Value: "different", Domain: ".youtube.com", Path: "/", Secure: s.Cookies[0].Secure})
		},
		func(s *Session) { s.Cookies[0].Value = strings.Repeat("a", 64*1024) },
	} {
		s := fixture()
		change(&s)
		if _, err := s.HTTPCookies(now); err == nil {
			t.Fatal("invalid synthetic session accepted")
		}
	}
	for _, invalid := range []string{string(data) + `{}`, `{"accountIndex":0,"cookies":[],"unknown":true}`, strings.Replace(string(data), `"secure":true`, `"secure":true,"partitionKey":{}`, 1)} {
		if _, err := Decode([]byte(invalid), now); err == nil {
			t.Fatal("unexpected JSON accepted")
		}
	}
}

func TestSessionExpiryDistinguishesOptionalCookiesAndRequiredAuthentication(t *testing.T) {
	now := time.Unix(2000000000, 500000000)
	expired := float64(now.Unix()) + .25
	s := fixture()
	optional := s.Cookies[0]
	optional.Name = "PREF"
	optional.ExpirationDate = &expired
	s.Cookies = append(s.Cookies, optional)
	cookies, err := s.HTTPCookies(now)
	if err != nil || len(cookies) != 1 || cookies[0].Name != "SAPISID" {
		t.Fatal("expired optional cookie invalidated active authentication")
	}
	data, _ := json.Marshal(s)
	if _, err := Decode(data, now); err != nil {
		t.Fatal("optional expiry prevented persisted-session decoding")
	}
	s.Cookies[0].ExpirationDate = &expired
	if _, err := s.HTTPCookies(now); !errors.Is(err, ErrExpired) {
		t.Fatal("required expiry was not distinguished")
	}
	data, _ = json.Marshal(s)
	if _, err := Decode(data, now); !errors.Is(err, ErrExpired) {
		t.Fatal("decoder lost required-expiry classification")
	}
	s.Cookies[0].Domain = "invalid.test"
	if _, err := s.HTTPCookies(now); !errors.Is(err, ErrSession) {
		t.Fatal("malformed session misclassified as expiration")
	}
}
