package streaming

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestProofScriptCacheRestartResetVersionExpiryAndCorruption(t *testing.T) {
	dir := t.TempDir()
	calls := 0
	transport := func(req *http.Request) (*http.Response, error) {
		calls++
		if req.Header.Get("Cookie") != "" || req.Header.Get("Authorization") != "" {
			t.Fatal("credentials entered public script request")
		}
		res := response("public interpreter fixture")
		res.Header.Set("Cache-Control", "public, max-age=3600")
		res.Header.Set("Content-Type", "text/javascript")
		return res, nil
	}
	const raw = "https://www.google.com/js/th/version_a.js"
	r := testResolver(t, transport)
	r.proofScripts.dir = dir
	if _, e := r.proofScript(context.Background(), raw); e != nil {
		t.Fatal(e)
	}
	r.resetHosts()
	if _, e := r.proofScript(context.Background(), raw); e != nil || calls != 1 {
		t.Fatal("proof reset downloaded the interpreter again")
	}
	restarted := testResolver(t, transport)
	restarted.proofScripts.dir = dir
	if _, e := restarted.proofScript(context.Background(), raw); e != nil || calls != 1 {
		t.Fatal("restart did not reuse public interpreter")
	}
	if _, e := restarted.proofScript(context.Background(), "https://www.google.com/js/th/version_b.js"); e != nil || calls != 2 {
		t.Fatal("changed script URL used old code")
	}
	p := restarted.proofScripts.entry
	p.Expires = time.Now().Add(-time.Minute)
	p.Checksum = p.checksum()
	b, _ := json.Marshal(p)
	path := filepath.Join(dir, "proof-interpreter.json")
	if err := os.WriteFile(path, b, 0600); err != nil {
		t.Fatal(err)
	}
	restarted.proofScripts.entry = publicProofScript{}
	if _, e := restarted.proofScript(context.Background(), p.URL); e != nil || calls != 3 {
		t.Fatal("expired interpreter reused")
	}
	if err := os.WriteFile(path, []byte("corrupt"), 0600); err != nil {
		t.Fatal(err)
	}
	restarted.proofScripts.entry = publicProofScript{}
	if _, e := restarted.proofScript(context.Background(), p.URL); e != nil || calls != 4 {
		t.Fatal("corrupt interpreter not recovered")
	}
	t.Logf("same-URL proof reset and restart: downloads=1; URL change, expiry and corruption each refresh once")
	restarted.proofScripts.invalidate()
	if _, e := restarted.proofScript(context.Background(), p.URL); e != nil || calls != 5 {
		t.Fatal("invalid interpreter was retained")
	}
}

func TestProofScriptCacheRespectsHTTPAndCancellation(t *testing.T) {
	for _, mode := range []string{"private", "no-store", "no-cache", "missing", "expired", "vary", "cookie", "query", "html"} {
		t.Run(mode, func(t *testing.T) {
			calls := 0
			r := testResolver(t, func(*http.Request) (*http.Response, error) {
				calls++
				res := response("public fixture")
				res.Header.Set("Cache-Control", "public, max-age=3600")
				res.Header.Set("Content-Type", "text/javascript")
				switch mode {
				case "private", "no-store", "no-cache":
					res.Header.Set("Cache-Control", mode+", max-age=3600")
				case "missing":
					res.Header.Del("Cache-Control")
				case "expired":
					res.Header.Set("Age", "3600")
				case "vary":
					res.Header.Set("Vary", "Cookie")
				case "cookie":
					res.Header.Set("Set-Cookie", "synthetic=fixture")
				case "html":
					res.Header.Set("Content-Type", "text/html")
				}
				return res, nil
			})
			r.proofScripts.dir = t.TempDir()
			raw := "https://www.google.com/js/th/version.js"
			if mode == "query" {
				raw += "?fixture=1"
			}
			for range 2 {
				if _, e := r.proofScript(context.Background(), raw); e != nil {
					t.Fatal(e)
				}
			}
			if calls != 2 || len(r.proofScripts.entry.Data) != 0 {
				t.Fatal("uncacheable response reused")
			}
			files, _ := os.ReadDir(r.proofScripts.dir)
			if len(files) != 0 {
				t.Fatal("uncacheable response persisted")
			}
		})
	}
	r := testResolver(t, nil)
	r.proofScripts.save(publicProofScript{URL: "https://www.google.com/js/th/version.js", Expires: time.Now().Add(time.Hour), Data: []byte("fixture")})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, e := r.proofScript(ctx, r.proofScripts.entry.URL); e != context.Canceled {
		t.Fatal("cached script hid cancellation")
	}
}
