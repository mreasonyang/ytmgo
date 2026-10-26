// This probe only runs inside the separately identified Logout Test app. It
// creates artificial loopback data and returns booleans, never credentials.
package main

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo"
)

const address = "127.0.0.1:18746"

const seed = `
localStorage.setItem('ytmgo-test', 'synthetic');
sessionStorage.setItem('ytmgo-test', 'synthetic');
await new Promise((resolve,reject)=>{const r=indexedDB.open('ytmgo-test',1);r.onupgradeneeded=()=>r.result.createObjectStore('test');r.onsuccess=()=>{r.result.close();resolve()};r.onerror=()=>reject(new Error('test database'))});
await (await caches.open('ytmgo-test')).put('/synthetic-cache',new Response('synthetic'));
await navigator.serviceWorker.register('/sw.js');
await navigator.serviceWorker.ready;
`

const inspect = `
return {
 httpOnly: (await (await fetch('/cookie',{cache:'no-store'})).json()).present,
 scriptCookie: document.cookie.split(';').some(c=>c.trim().startsWith('ytmgo_test_visible=')),
 localStorage: localStorage.getItem('ytmgo-test') !== null,
 sessionStorage: sessionStorage.getItem('ytmgo-test') !== null,
 indexedDB: (await indexedDB.databases()).some(d=>d.name==='ytmgo-test'),
 cache: await caches.has('ytmgo-test'),
 serviceWorker: (await navigator.serviceWorker.getRegistrations()).length > 0
};`

func main() {
	if len(os.Args) != 3 {
		return
	}
	mode, output := os.Args[1], os.Args[2]
	if mode != "seed" && mode != "present" && mode != "absent" {
		return
	}
	write := func(data any) {
		b, err := json.MarshalIndent(data, "", "  ")
		if err == nil {
			_ = os.WriteFile(output, append(b, '\n'), 0600)
		}
	}
	exe, err := os.Executable()
	if err != nil {
		return
	}
	plist, err := os.ReadFile(filepath.Join(filepath.Dir(filepath.Dir(exe)), "Info.plist"))
	if err != nil || !strings.Contains(string(plist), "<string>si.kyun.ytmgo.logout-test</string>") || mygo.App.Name() != "ytmgo Logout Test" || !mygo.App.IsPackaged() {
		write(map[string]any{"passed": false, "reason": "isolated test bundle required"})
		return
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if r.URL.Path == "/seed" {
			http.SetCookie(w, &http.Cookie{Name: "ytmgo_test_http", Value: "synthetic", Path: "/", MaxAge: 86400, HttpOnly: true, SameSite: http.SameSiteLaxMode})
			http.SetCookie(w, &http.Cookie{Name: "ytmgo_test_visible", Value: "synthetic", Path: "/", MaxAge: 86400, SameSite: http.SameSiteLaxMode})
		}
		_, _ = w.Write([]byte("<!doctype html><title>ytmgo synthetic storage check</title><p>Synthetic test data only.</p>"))
	})
	mux.HandleFunc("/cookie", func(w http.ResponseWriter, r *http.Request) {
		// Only the artificial named cookie on this loopback origin is checked.
		_, err := r.Cookie("ytmgo_test_http")
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		_ = json.NewEncoder(w).Encode(map[string]bool{"present": err == nil})
	})
	mux.HandleFunc("/sw.js", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/javascript")
		_, _ = w.Write([]byte("self.addEventListener('install',()=>self.skipWaiting());self.addEventListener('activate',e=>e.waitUntil(self.clients.claim()));"))
	})
	listener, err := net.Listen("tcp", address)
	if err != nil {
		write(map[string]any{"passed": false, "reason": "loopback port unavailable"})
		return
	}
	server := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() { _ = server.Serve(listener) }()
	defer server.Close()
	var once sync.Once
	mygo.App.WhenReady(func() {
		w := mygo.NewWindow(mygo.WindowOptions{Title: "ytmgo Logout Test · Synthetic probe", Width: 640, Height: 400})
		w.Page().OnDidFinishLoad(func() {
			once.Do(func() {
				go func() {
					code := inspect
					if mode == "seed" {
						code = seed + inspect
					}
					ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
					defer cancel()
					value, err := w.Page().EvalContext(ctx, code)
					checks, ok := value.(map[string]any)
					passed := err == nil && ok && len(checks) == 7
					for key, value := range checks {
						expect := mode != "absent"
						if key == "sessionStorage" && mode != "seed" {
							expect = false
						}
						if value != expect {
							passed = false
						}
					}
					if err != nil {
						checks = nil
					}
					write(map[string]any{"mode": mode, "passed": passed, "present": checks, "isolated": true})
					time.Sleep(750 * time.Millisecond)
					mygo.App.Quit()
				}()
			})
		})
		_ = w.Page().LoadURL("http://" + address + "/" + mode)
	})
	if err := mygo.App.Run(); err != nil {
		write(map[string]any{"passed": false, "reason": "test runtime failed"})
	}
}
