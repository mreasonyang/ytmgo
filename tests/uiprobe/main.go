// UI diagnostics use a separate anonymous WebView profile. Interaction stays
// in the native UI; the loopback endpoint returns only layout/color metadata.
package main

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/egoist/mygo"
)

func main() {
	if len(os.Args) != 2 || mygo.App.Name() != "ytmgo UI Audit" {
		return
	}
	root := os.Args[1]
	css, js, err := shellSources(root)
	if err != nil {
		return
	}
	inspection, err := os.ReadFile(filepath.Join(root, "tests/uiprobe/inspect.js"))
	if err != nil {
		return
	}
	encoded, _ := json.Marshal(css)
	text, err := shellText(root, "en")
	if err != nil {
		return
	}
	preload := "(function ytmgoShell(STYLES, APPEARANCE, I18N) {\n'use strict';\n" + js + "\n})(" + string(encoded) + `, "dark", ` + text + ");\n"
	mygo.App.WhenReady(func() {
		w := mygo.NewWindow(mygo.WindowOptions{Title: "ytmgo UI Audit", Width: 1440, Height: 940, MinWidth: 380, MinHeight: 640, Page: mygo.PageOptions{PreloadScript: preload, UserAgentSuffix: "Version/17.0 Safari/605.1.15 ytmgo-ui-audit"}})
		p := w.Page()
		p.OnPageTitleUpdated(func(e *mygo.TitleEvent) { e.PreventDefault() })
		p.OnWillNavigate(func(e *mygo.NavigateEvent) {
			if !strings.HasPrefix(e.URL, "https://music.youtube.com/") {
				e.PreventDefault()
			}
		})
		p.SetWindowOpenHandler(func(mygo.WindowOpenRequest) *mygo.WindowOptions { return nil })
		p.SetPermissionHandler(func(mygo.PermissionRequest) bool { return false })
		p.OnWillDownload(func(e *mygo.DownloadEvent) { e.PreventDefault() })
		eval := func(script string) {
			go func() {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				_, _ = p.EvalContext(ctx, script)
			}()
		}
		entry := func(label string, fn func()) *mygo.MenuItem {
			return &mygo.MenuItem{Label: label, Click: func(*mygo.MenuItem, *mygo.Window) { fn() }}
		}
		mygo.App.SetMenu(mygo.NewMenu([]*mygo.MenuItem{{Role: mygo.RoleAppMenu}, {Role: mygo.RoleEditMenu}, {Label: "Audit", Submenu: []*mygo.MenuItem{
			entry("Dark", func() { eval(`window.ytmgoSetAppearance('dark')`) }),
			entry("Light", func() { eval(`window.ytmgoSetAppearance('light')`) }),
			entry("Reload stylesheet", func() {
				if styles, _, err := shellSources(root); err == nil {
					value, _ := json.Marshal(styles)
					eval(`document.getElementById('ytmgo-shell-style').textContent=` + string(value))
				}
			}),
			entry("Wide window", func() { w.SetSize(1440, 940) }), entry("Medium window", func() { w.SetSize(1100, 800) }), entry("Compact window", func() { w.SetSize(420, 860) }),
		}}}))
		mux := http.NewServeMux()
		mux.HandleFunc("/snapshot", func(rw http.ResponseWriter, r *http.Request) {
			if r.Method != "GET" {
				rw.WriteHeader(405)
				return
			}
			ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
			defer cancel()
			value, err := p.EvalContext(ctx, string(inspection))
			if err != nil {
				http.Error(rw, "inspection unavailable", 503)
				return
			}
			rw.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(rw).Encode(value)
		})
		listener, err := net.Listen("tcp", "127.0.0.1:18747")
		if err != nil {
			mygo.App.Quit()
			return
		}
		server := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
		mygo.App.OnQuit(func() { _ = server.Close() })
		go func() { _ = server.Serve(listener) }()
		_ = p.LoadURL("https://music.youtube.com/")
	})
	_ = mygo.App.Run()
}

// shellSources mirrors musicPreload(): manifest-ordered styles and scripts,
// read from disk so stylesheet edits can be reloaded without rebuilding.
func shellSources(root string) (string, string, error) {
	var manifest struct{ Styles, Scripts []string }
	raw, err := os.ReadFile(filepath.Join(root, "web/shell/bundle.json"))
	if err != nil {
		return "", "", err
	}
	if err = json.Unmarshal(raw, &manifest); err != nil {
		return "", "", err
	}
	var styles, scripts strings.Builder
	for _, name := range manifest.Styles {
		data, err := os.ReadFile(filepath.Join(root, "web/shell", name))
		if err != nil {
			return "", "", err
		}
		styles.Write(data)
		styles.WriteByte('\n')
	}
	for _, name := range manifest.Scripts {
		data, err := os.ReadFile(filepath.Join(root, "web/shell", name))
		if err != nil {
			return "", "", err
		}
		scripts.WriteString("/* " + name + " */\n")
		scripts.Write(data)
		scripts.WriteByte('\n')
	}
	return styles.String(), scripts.String(), nil
}

// shellText builds the shell's I18N argument from a catalog on disk. Shell
// messages are plain strings with {name} slots, so no formatting is needed.
func shellText(root, locale string) (string, error) {
	raw, err := os.ReadFile(filepath.Join(root, "locales", locale+".json"))
	if err != nil {
		return "", err
	}
	var catalog map[string]any
	if err = json.Unmarshal(raw, &catalog); err != nil {
		return "", err
	}
	messages := map[string]string{}
	for key, value := range catalog {
		if text, ok := value.(string); ok && strings.HasPrefix(key, "shell") {
			messages[key] = text
		}
	}
	payload, err := json.Marshal(map[string]any{"locale": locale, "script": "latn", "choice": "system", "languages": []map[string]string{{"tag": locale, "name": locale}}, "messages": messages})
	return string(payload), err
}
