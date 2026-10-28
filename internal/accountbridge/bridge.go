package accountbridge

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"sync"
	"time"
)

const Address = "127.0.0.1:52963"
const ExtensionID = "llinflbckniejeiomanmglbdganiccfk"
const Origin = "chrome-extension://" + ExtensionID
const Lifetime = 5 * time.Minute

var ErrBridge = errors.New("account_bridge_unavailable")

type Bridge struct {
	mu       sync.Mutex
	token    string
	deadline time.Time
	used     bool
	result   chan Session
	server   *http.Server
	cancel   context.CancelFunc
}

func Listen(parent context.Context) (*Bridge, error) {
	listener, err := net.Listen("tcp4", Address)
	if err != nil {
		return nil, ErrBridge
	}
	b, err := newBridge()
	if err != nil {
		_ = listener.Close()
		return nil, err
	}
	ctx, cancel := context.WithTimeout(parent, Lifetime)
	b.cancel = cancel
	b.server = &http.Server{Handler: b, ReadHeaderTimeout: 3 * time.Second, ReadTimeout: 5 * time.Second, WriteTimeout: 5 * time.Second, IdleTimeout: 3 * time.Second, MaxHeaderBytes: 4096, ErrorLog: log.New(io.Discard, "", 0)}
	go func() { _ = b.server.Serve(listener) }()
	go func() { <-ctx.Done(); b.Close() }()
	return b, nil
}

func newBridge() (*Bridge, error) {
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return nil, ErrBridge
	}
	b := &Bridge{token: base64.RawURLEncoding.EncodeToString(secret), deadline: time.Now().Add(Lifetime), result: make(chan Session, 1)}
	clear(secret)
	return b, nil
}

func (b *Bridge) Wait(ctx context.Context) (Session, error) {
	timer := time.NewTimer(time.Until(b.deadline))
	defer timer.Stop()
	select {
	case s := <-b.result:
		return s, nil
	case <-ctx.Done():
		return Session{}, ErrBridge
	case <-timer.C:
		return Session{}, ErrBridge
	}
}

func (b *Bridge) Close() {
	b.mu.Lock()
	b.token = ""
	b.used = true
	cancel := b.cancel
	b.cancel = nil
	server := b.server
	b.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if server != nil {
		_ = server.Close()
	}
}

func (b *Bridge) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	// Host plus exact extension origin excludes DNS rebinding and ordinary sites.
	if r.Host != Address || r.Header.Get("Origin") != Origin {
		reject(w, http.StatusForbidden)
		return
	}
	w.Header().Set("Access-Control-Allow-Origin", Origin)
	w.Header().Set("Vary", "Origin")
	if r.Method == http.MethodOptions {
		if r.URL.Path != "/session" && r.URL.Path != "/challenge" {
			reject(w, 404)
			return
		}
		w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-Ytmgo-Handoff")
		w.Header().Set("Access-Control-Allow-Private-Network", "true")
		w.WriteHeader(http.StatusNoContent)
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.used || b.token == "" || !time.Now().Before(b.deadline) {
		reject(w, http.StatusGone)
		return
	}
	if r.URL.RawQuery != "" {
		reject(w, 400)
		return
	}
	if r.URL.Path == "/challenge" && r.Method == http.MethodPost && r.Header.Get("Content-Type") == "application/json" && r.ContentLength == 0 {
		_ = json.NewEncoder(w).Encode(struct {
			Token string `json:"token"`
		}{b.token})
		return
	}
	if r.URL.Path != "/session" || r.Method != http.MethodPost {
		reject(w, 405)
		return
	}
	if r.Header.Get("Content-Type") != "application/json" || subtle.ConstantTimeCompare([]byte(r.Header.Get("X-Ytmgo-Handoff")), []byte(b.token)) != 1 {
		reject(w, 403)
		return
	}
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, MaxPayload))
	defer clear(data)
	if err != nil {
		reject(w, 413)
		return
	}
	session, err := Decode(data, time.Now())
	if err != nil {
		reject(w, 400)
		return
	}
	b.used = true
	b.token = ""
	_, _ = io.WriteString(w, `{"ok":true,"session_received":true}`)
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}
	b.result <- session
}

func reject(w http.ResponseWriter, code int) {
	w.WriteHeader(code)
	_, _ = io.WriteString(w, `{"ok":false}`)
}
