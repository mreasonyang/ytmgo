package streaming

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func cachedFixture() cachedPlayer {
	return cachedPlayer{PlayerID: "version_a", CheckedAt: time.Now(), Seed: extracted{Code: "public-transform", Timestamp: 12345}}
}

func cacheResolver(t *testing.T, dir string, fn roundTrip) *Resolver {
	t.Helper()
	r := testResolver(t, fn)
	r.playerCache.dir = dir
	r.newHost = func(mode string) (scriptHost, error) {
		if mode != "extract" {
			t.Fatal("unexpected host mode")
		}
		return &stubHost{run: func(_ string, out any) error {
			*out.(*extracted) = cachedFixture().Seed
			return nil
		}}, nil
	}
	return r
}

func TestPlayerCacheRestartAndAccountChanges(t *testing.T) {
	dir := t.TempDir()
	identifiers, downloads, extractions := 0, 0, 0
	r := cacheResolver(t, dir, func(req *http.Request) (*http.Response, error) {
		if req.Header.Get("Cookie") != "" || req.Header.Get("Authorization") != "" {
			t.Fatal("credentials sent to public player")
		}
		if req.URL.Path == "/iframe_api" {
			identifiers++
			return response(`/s/player/version_a/`), nil
		}
		downloads++
		return response("public script fixture"), nil
	})
	newHost := r.newHost
	r.newHost = func(mode string) (scriptHost, error) { extractions++; return newHost(mode) }
	if err := r.ensurePlayer(context.Background()); err != nil {
		t.Fatal(err)
	}
	for _, session := range []Session{{Revision: 1}, {Revision: 2, LoggedIn: true, MusicPremium: true, Cookie: "SAPISID=synthetic"}, {Revision: 3}} {
		if err := r.Configure(context.Background(), session); err != nil {
			t.Fatal(err)
		}
		if err := r.ensurePlayer(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	if identifiers != 1 || downloads != 1 || extractions != 1 {
		t.Fatal("public player repeated across account generations")
	}
	r.Close()
	restarted := cacheResolver(t, dir, func(*http.Request) (*http.Response, error) {
		t.Fatal("fresh cache restart accessed network")
		return nil, errors.New("unreachable")
	})
	restarted.newHost = func(string) (scriptHost, error) {
		t.Fatal("fresh cache restart extracted again")
		return nil, errors.New("unreachable")
	}
	if err := restarted.ensurePlayer(context.Background()); err != nil || restarted.seed != cachedFixture().Seed {
		t.Fatal("restart did not reuse persisted transform")
	}
	info, err := os.Stat(filepath.Join(dir, "current.json"))
	if err != nil || runtime.GOOS != "windows" && info.Mode().Perm()&0077 != 0 {
		t.Fatal("cache permissions are not private")
	}
	data, _ := os.ReadFile(filepath.Join(dir, "current.json"))
	if strings.Contains(string(data), "synthetic") || strings.Contains(string(data), "SAPISID") {
		t.Fatal("account data entered public cache")
	}
}

func TestPlayerCacheVersionRevalidation(t *testing.T) {
	for _, id := range []string{"version_a", "version_b"} {
		t.Run(id, func(t *testing.T) {
			dir := t.TempDir()
			p := cachedFixture()
			p.CheckedAt = time.Now().Add(-8 * time.Hour)
			playerCache{dir}.save(p)
			downloads := 0
			r := cacheResolver(t, dir, func(req *http.Request) (*http.Response, error) {
				if req.URL.Path == "/iframe_api" {
					return response(`/s/player/` + id + `/`), nil
				}
				downloads++
				if req.URL.Path != "/s/player/"+id+"/"+playerProfile+"/base.js" {
					t.Fatal("wrong version/profile downloaded")
				}
				return response("public script fixture"), nil
			})
			if err := r.ensurePlayer(context.Background()); err != nil {
				t.Fatal(err)
			}
			want := 0
			if id != p.PlayerID {
				want = 1
			}
			stored, ok := r.playerCache.load()
			if downloads != want || !ok || stored.PlayerID != id || time.Since(stored.CheckedAt) > time.Minute {
				t.Fatal("version revalidation did not reuse/replace the right record")
			}
			files, _ := os.ReadDir(dir)
			if len(files) != 1 {
				t.Fatal("cache grew beyond one record")
			}
		})
	}
}

func TestPlayerCacheRejectsInvalidRecords(t *testing.T) {
	for _, mutation := range []string{"checksum", "schema", "profile", "id", "timestamp", "future", "unknown", "truncated", "oversize", "symlink"} {
		t.Run(mutation, func(t *testing.T) {
			dir := t.TempDir()
			c := playerCache{dir}
			c.save(cachedFixture())
			p, _ := c.load()
			switch mutation {
			case "schema":
				p.Schema++
			case "profile":
				p.Profile = "other"
			case "id":
				p.PlayerID = "../../other"
			case "timestamp":
				p.Seed.Timestamp = 0
			case "future":
				p.CheckedAt = time.Now().Add(time.Hour)
			}
			p.Checksum = p.checksum()
			if mutation == "checksum" {
				p.Seed.Code += "changed"
			}
			data, _ := json.Marshal(p)
			if mutation == "unknown" {
				data = append(data[:len(data)-1], []byte(`,"session":"synthetic"}`)...)
			}
			if mutation == "truncated" {
				data = data[:len(data)/2]
			}
			path := filepath.Join(dir, "current.json")
			if err := os.WriteFile(path, data, 0600); err != nil {
				t.Fatal(err)
			}
			if mutation == "oversize" {
				if err := os.Truncate(path, maxPlayerCache+1); err != nil {
					t.Fatal(err)
				}
			}
			if mutation == "symlink" {
				other := filepath.Join(dir, "other.json")
				if err := os.Rename(path, other); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(other, path); err != nil {
					t.Skip("symlink unavailable")
				}
			}
			if _, ok := c.load(); ok {
				t.Fatal("invalid player record accepted")
			}
		})
	}
}

func TestPlayerCacheFailureDoesNotPoisonPreviousVersion(t *testing.T) {
	for _, stage := range []string{"download", "extraction", "invalid_extraction", "cancel"} {
		t.Run(stage, func(t *testing.T) {
			dir := t.TempDir()
			p := cachedFixture()
			p.CheckedAt = time.Now().Add(-8 * time.Hour)
			playerCache{dir}.save(p)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			r := cacheResolver(t, dir, func(req *http.Request) (*http.Response, error) {
				if req.URL.Path == "/iframe_api" {
					return response(`/s/player/version_b/`), nil
				}
				if stage == "download" {
					return nil, errors.New("synthetic network failure")
				}
				if stage == "cancel" {
					cancel()
					return nil, context.Canceled
				}
				return response("public script fixture"), nil
			})
			r.newHost = func(string) (scriptHost, error) {
				return &stubHost{run: func(_ string, out any) error {
					if stage == "extraction" {
						return errors.New("script_syntax_error")
					}
					*out.(*extracted) = extracted{}
					return nil
				}}, nil
			}
			if err := r.ensurePlayer(ctx); err == nil {
				t.Fatal("failed refresh was hidden")
			}
			stored, ok := r.playerCache.load()
			if !ok || stored.PlayerID != p.PlayerID || !stored.CheckedAt.Equal(p.CheckedAt) {
				t.Fatal("failed refresh overwrote good cache")
			}
		})
	}
}

func TestPlayerCacheVersionCheckOutageAndStaleLimit(t *testing.T) {
	for _, age := range []time.Duration{8 * time.Hour, 8 * 24 * time.Hour} {
		t.Run(age.String(), func(t *testing.T) {
			dir := t.TempDir()
			p := cachedFixture()
			p.CheckedAt = time.Now().Add(-age)
			playerCache{dir}.save(p)
			calls := 0
			r := cacheResolver(t, dir, func(*http.Request) (*http.Response, error) {
				calls++
				return nil, errors.New("synthetic network outage")
			})
			err := r.ensurePlayer(context.Background())
			if age < playerStaleFor {
				if err != nil {
					t.Fatal("temporary version-check outage blocked cached player")
				}
				if err = r.ensurePlayer(context.Background()); err != nil || calls != 1 {
					t.Fatal("outage retried every track")
				}
			} else if err == nil {
				t.Fatal("unbounded stale reuse")
			}
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			if !errors.Is(r.ensurePlayer(ctx), context.Canceled) {
				t.Fatal("cache hid cancellation")
			}
		})
	}
}

func TestPlayerCacheInvalidationAndUnavailableStorage(t *testing.T) {
	dir := t.TempDir()
	c := playerCache{dir}
	c.save(cachedFixture())
	r := cacheResolver(t, dir, nil)
	if err := r.ensurePlayer(context.Background()); err != nil {
		t.Fatal(err)
	}
	r.invalidatePlayer()
	if _, ok := c.load(); ok || r.seed.Code != "" {
		t.Fatal("invalid player survived")
	}
	file := filepath.Join(dir, "not-a-directory")
	if err := os.WriteFile(file, []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	r.playerCache.dir = file
	r.client.Transport = roundTrip(func(req *http.Request) (*http.Response, error) {
		if req.URL.Path == "/iframe_api" {
			return response(`/s/player/version_a/`), nil
		}
		return response("public fixture"), nil
	})
	if err := r.ensurePlayer(context.Background()); err != nil || r.seed.Code == "" {
		t.Fatal("unavailable disk cache blocked playback")
	}
}

func TestResolveInvalidatesFailedPlayerTransform(t *testing.T) {
	dir := t.TempDir()
	playerCache{dir}.save(cachedFixture())
	r := cacheResolver(t, dir, func(req *http.Request) (*http.Response, error) {
		if req.URL.Path != "/youtubei/v1/player" {
			t.Fatal("unexpected request")
		}
		return response(`{"playabilityStatus":{"status":"OK"},"streamingData":{"adaptiveFormats":[{"mimeType":"audio/mp4; codecs=\"mp4a.40.2\"","url":"https://r.googlevideo.com/videoplayback?n=fixture","contentLength":"128","approxDurationMs":"120000"}]}}`), nil
	})
	if err := r.Configure(context.Background(), Session{Revision: 1}); err != nil {
		t.Fatal(err)
	}
	r.ready, r.expires = true, time.Now().Add(time.Hour)
	r.newHost = func(string) (scriptHost, error) {
		return &stubHost{run: func(string, any) error { return errors.New("script_syntax_error") }}, nil
	}
	if _, err := r.Resolve(context.Background(), "AAAAAAAAAAA", "audio"); err == nil || err.Error() != "decipher_failed" {
		t.Fatal("expected failed transform")
	}
	if _, ok := r.playerCache.load(); ok || r.seed.Code != "" {
		t.Fatal("failed transform was retained for the next retry")
	}
}

func TestAccountSwitchCancelsPlayerDownloadWithoutCachingPartialData(t *testing.T) {
	started := make(chan struct{})
	r := cacheResolver(t, t.TempDir(), func(req *http.Request) (*http.Response, error) {
		if req.URL.Path == "/iframe_api" {
			return response(`/s/player/version_a/`), nil
		}
		close(started)
		<-req.Context().Done()
		return nil, req.Context().Err()
	})
	if err := r.Configure(context.Background(), Session{Revision: 1}); err != nil {
		t.Fatal(err)
	}
	r.sessionClient = map[string]any{}
	done := make(chan error, 1)
	go func() { done <- r.Warm(context.Background()) }()
	<-started
	if err := r.Configure(context.Background(), Session{Revision: 2}); err != nil {
		t.Fatal(err)
	}
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatal("account switch did not cancel player download")
	}
	if _, ok := r.playerCache.load(); ok || r.seed.Code != "" {
		t.Fatal("partial download entered cache")
	}
}

func TestPlayerRequestBudgetsAndCancellation(t *testing.T) {
	for _, step := range []string{"player_identifier", "player_script", "session"} {
		t.Run(step, func(t *testing.T) {
			r := testResolver(t, func(req *http.Request) (*http.Response, error) {
				deadline, ok := req.Context().Deadline()
				budget := 20 * time.Second
				if step == "player_identifier" {
					budget = 5 * time.Second
				}
				if step == "player_script" {
					budget = 60 * time.Second
				}
				if !ok || time.Until(deadline) > budget || time.Until(deadline) < budget-time.Second {
					t.Fatal("incorrect request budget")
				}
				return response("fixture"), nil
			})
			if _, err := r.request(context.Background(), "GET", "https://www.youtube.com/iframe_api", nil, nil, "bootstrap", step); err != nil {
				t.Fatal(err)
			}
		})
	}
	r := testResolver(t, func(req *http.Request) (*http.Response, error) {
		res := response("")
		res.Body = &cancelledBody{ctx: req.Context()}
		return res, nil
	})
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if data, err := r.request(ctx, "GET", "https://www.youtube.com/s/player/fixture/base.js", nil, nil, "bootstrap", "player_script"); !errors.Is(err, context.DeadlineExceeded) || len(data) != 0 {
		t.Fatal("download cancellation returned partial script")
	}
}

type cancelledBody struct{ ctx context.Context }

func (b *cancelledBody) Read([]byte) (int, error) { <-b.ctx.Done(); return 0, b.ctx.Err() }
func (*cancelledBody) Close() error               { return nil }

// Explicit opt-in, public requests only. The second resolver cannot access the
// network or launch an extractor, so this proves restart reuse of a real script.
func TestPublicPlayerCacheRestart(t *testing.T) {
	if os.Getenv("YTMGO_TEST_PUBLIC_PLAYER_CACHE") != "1" {
		t.Skip("public player cache validation not selected")
	}
	dir := t.TempDir()
	r, err := New(Options{Command: nativeCommand(t), PlayerCacheDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	ctx, cancel := context.WithTimeout(context.Background(), resolverOperationTimeout)
	defer cancel()
	began := time.Now()
	if err := r.ensurePlayer(ctx); err != nil {
		t.Fatalf("public player cache: %v", err)
	}
	cold := time.Since(began)
	var transformed struct{ N string }
	h, err := r.newHost("eval")
	if err != nil {
		t.Fatal("eval host failed")
	}
	err = h.call(ctx, "(()=>{"+r.seed.Code+"\n"+transform+"\n"+`return process("synthetic-input-n","","");})()`, &transformed)
	h.Close()
	if err != nil || transformed.N == "" || strings.HasPrefix(transformed.N, "enhanced_except_") {
		t.Fatal("cached public transform failed")
	}
	expected := r.seed
	r.Close()
	restarted := cacheResolver(t, dir, func(*http.Request) (*http.Response, error) { t.Fatal("restart reached network"); return nil, io.EOF })
	restarted.newHost = func(string) (scriptHost, error) { t.Fatal("restart launched extraction"); return nil, io.EOF }
	began = time.Now()
	if err := restarted.ensurePlayer(ctx); err != nil {
		t.Fatal(err)
	}
	if restarted.seed != expected {
		t.Fatal("restart transform changed")
	}
	t.Logf("public player cache: cold=%s restart=%s code_bytes=%d restart_network_requests=0 restart_extractions=0", cold.Round(time.Millisecond), time.Since(began).Round(time.Millisecond), len(restarted.seed.Code))
}
