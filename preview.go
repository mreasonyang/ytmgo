package main

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"ytmgo/internal/library"
)

func servePreview(service *Service) error {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}
	origin := "http://" + listener.Addr().String()
	fmt.Println("ytmgo preview: " + origin)
	server := &http.Server{Handler: previewHandler(service, origin), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 60 * time.Second, IdleTimeout: 60 * time.Second}
	return server.Serve(listener)
}

func previewHandler(service *Service, origin string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Host != strings.TrimPrefix(origin, "http://") {
			http.Error(w, "invalid host", http.StatusForbidden)
			return
		}
		if o := r.Header.Get("Origin"); o != "" && o != origin {
			http.Error(w, "invalid origin", http.StatusForbidden)
			return
		}
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Cache-Control", "no-store")
		if !strings.HasPrefix(r.URL.Path, "/api/") {
			if r.Method != "GET" && r.Method != "HEAD" {
				http.Error(w, "method not allowed", 405)
				return
			}
			http.FileServer(http.FS(frontendFS())).ServeHTTP(w, r)
			return
		}
		var result any
		var err error
		switch {
		case r.URL.Path == "/api/search" && r.Method == "GET":
			result, err = service.Search(r.Context(), r.URL.Query().Get("q"))
		case r.URL.Path == "/api/home" && r.Method == "GET":
			result, err = service.Home(r.Context())
		case r.URL.Path == "/api/library" && r.Method == "GET":
			result, err = service.Library()
		case r.URL.Path == "/api/library" && r.Method == "PUT":
			if r.Header.Get("Origin") != origin || r.Header.Get("X-Ytmgo-Client") != "1" || !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
				http.Error(w, "invalid request", 403)
				return
			}
			var data library.Library
			decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<20))
			if err = decoder.Decode(&data); err == nil {
				result, err = service.SaveLibrary(data)
			}
		default:
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		if err != nil {
			w.WriteHeader(502)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "请求未完成，请检查网络或本地音乐库后重试"})
			return
		}
		_ = json.NewEncoder(w).Encode(result)
	})
}
