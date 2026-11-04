package streaming

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"ytmgo/internal/httpfreshness"
)

const maxProofScript = 4 << 20

// One public interpreter survives proof/account resets and process restarts.
// Challenge programs, bindings and minted tokens never enter this record.
type proofScriptCache struct {
	dir   string
	entry publicProofScript
}
type publicProofScript struct {
	URL      string    `json:"url"`
	Expires  time.Time `json:"expires"`
	Data     []byte    `json:"data"`
	Checksum string    `json:"checksum"`
}

func (p publicProofScript) checksum() string {
	p.Checksum = ""
	b, _ := json.Marshal(p)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
func publicInterpreterURL(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && u.Host == "www.google.com" && allowedRequest(u, "GET") && u.RawQuery == "" && !u.ForceQuery && strings.HasPrefix(u.Path, "/js/th/")
}
func (p publicProofScript) valid(raw string) bool {
	return p.URL == raw && publicInterpreterURL(raw) && len(p.Data) > 0 && len(p.Data) <= maxProofScript && p.Expires.After(time.Now()) && p.Expires.Before(time.Now().Add(24*time.Hour+time.Minute)) && p.Checksum == p.checksum()
}
func (c *proofScriptCache) load(raw string) []byte {
	if c.entry.valid(raw) {
		return c.entry.Data
	}
	c.entry = publicProofScript{}
	if c.dir == "" || !publicInterpreterURL(raw) {
		return nil
	}
	path := filepath.Join(c.dir, "proof-interpreter.json")
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() > 2*maxProofScript {
		return nil
	}
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	if opened, err := f.Stat(); err != nil || !os.SameFile(info, opened) {
		return nil
	}
	b, err := io.ReadAll(io.LimitReader(f, 2*maxProofScript+1))
	if err != nil || len(b) > 2*maxProofScript {
		return nil
	}
	var p publicProofScript
	if json.Unmarshal(b, &p) != nil || !p.valid(raw) {
		return nil
	}
	c.entry = p
	return p.Data
}
func (c *proofScriptCache) save(p publicProofScript) {
	p.Checksum = p.checksum()
	if !p.valid(p.URL) {
		return
	}
	c.entry = p
	if c.dir == "" || os.MkdirAll(c.dir, 0700) != nil {
		return
	}
	b, err := json.Marshal(p)
	if err != nil {
		return
	}
	f, err := os.CreateTemp(c.dir, ".proof-*")
	if err != nil {
		return
	}
	defer os.Remove(f.Name())
	_, err = f.Write(b)
	if err == nil {
		err = f.Sync()
	}
	closed := f.Close()
	if err == nil && closed == nil {
		_ = os.Rename(f.Name(), filepath.Join(c.dir, "proof-interpreter.json"))
	}
}

func (c *proofScriptCache) invalidate() {
	c.entry = publicProofScript{}
	if c.dir != "" {
		_ = os.Remove(filepath.Join(c.dir, "proof-interpreter.json"))
	}
}

func (r *Resolver) proofScript(ctx context.Context, raw string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if data := r.proofScripts.load(raw); len(data) > 0 {
		return data, nil
	}
	requested := time.Now()
	var headers http.Header
	data, err := r.requestWithHeaders(ctx, "GET", raw, nil, nil, "proof_init", "", &headers)
	if err != nil {
		return nil, err
	}
	if expires := httpfreshness.FreshUntil(headers, requested, 24*time.Hour); expires.After(time.Now()) && publicInterpreterURL(raw) && len(data) <= maxProofScript && mimeClass(headers.Get("Content-Type")) == "javascript" {
		r.proofScripts.save(publicProofScript{URL: raw, Expires: expires, Data: data})
	}
	return data, nil
}
