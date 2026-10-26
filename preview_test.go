package main

import (
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"ytmgo/internal/library"
)

func TestPreviewRejectsOtherOriginsAndHosts(t *testing.T) {
	s := &Service{library: library.NewStore(filepath.Join(t.TempDir(), "library.json"))}
	h := previewHandler(s, "http://127.0.0.1:12345")
	for _, tc := range []struct {
		host, origin, method, header string
		want                         int
	}{
		{"evil.test", "", "GET", "", 403},
		{"127.0.0.1:12345", "https://evil.test", "GET", "", 403},
		{"127.0.0.1:12345", "", "PUT", "1", 403},
		{"127.0.0.1:12345", "http://127.0.0.1:12345", "PUT", "", 403},
		{"127.0.0.1:12345", "http://127.0.0.1:12345", "PUT", "1", 200},
		{"127.0.0.1:12345", "", "GET", "", 200},
	} {
		r := httptest.NewRequest(tc.method, "http://"+tc.host+"/api/library", strings.NewReader(`{"liked":[],"playlists":[]}`))
		r.Host = tc.host
		r.Header.Set("Origin", tc.origin)
		r.Header.Set("X-Ytmgo-Client", tc.header)
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != tc.want {
			t.Errorf("%+v: got %d", tc, w.Code)
		}
	}
}

func TestFrontendInlinedForDesktop(t *testing.T) {
	html, err := inlineFrontend()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(html, `src="app.js"`) || strings.Contains(html, `href="style.css"`) || strings.Contains(html, `src="theme.js"`) || !strings.Contains(html, "window.ytmgoControls") || !strings.Contains(html, "<style>") {
		t.Fatal("desktop bundle is missing inline assets")
	}
	appearance := strings.Index(html, "const storageKey = 'ytmgo.appearance'")
	if appearance < 0 || appearance > strings.Index(html, "<style>") {
		t.Fatal("appearance must initialize before the first styled frame")
	}
}
