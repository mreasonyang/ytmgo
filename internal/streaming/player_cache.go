package streaming

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"time"
)

const (
	// Bump when the extractor/exportedVars contract changes incompatibly.
	playerCacheSchema = 1
	playerProfile     = "player_es6.vflset/en_US"
	playerCheckEvery  = 6 * time.Hour
	playerStaleFor    = 7 * 24 * time.Hour
	maxPlayerCode     = 8 << 20
	// JSON may escape each code byte as six characters.
	maxPlayerCache = 6*maxPlayerCode + 4096
)

var playerIDPattern = regexp.MustCompile(`player\\?/([A-Za-z0-9_-]{1,128})\\?/`)

// A single bounded record replaces the previous version atomically. Only the
// public transform is persisted: no full player, account, visitor, proof or URL.
type playerCache struct{ dir string }
type cachedPlayer struct {
	Schema    int       `json:"schema"`
	Profile   string    `json:"profile"`
	PlayerID  string    `json:"playerId"`
	CheckedAt time.Time `json:"checkedAt"`
	Seed      extracted `json:"seed"`
	Checksum  string    `json:"checksum"`
}

func validExtraction(seed extracted) bool {
	return seed.Timestamp > 0 && seed.Timestamp <= 1000000 && len(seed.Code) > 0 && len(seed.Code) <= maxPlayerCode
}

func (p cachedPlayer) checksum() string {
	p.Checksum = ""
	data, _ := json.Marshal(p)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func (c playerCache) load() (cachedPlayer, bool) {
	if c.dir == "" {
		return cachedPlayer{}, false
	}
	path := filepath.Join(c.dir, "current.json")
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() > maxPlayerCache {
		return cachedPlayer{}, false
	}
	f, err := os.Open(path)
	if err != nil {
		return cachedPlayer{}, false
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, maxPlayerCache+1))
	if err != nil || len(data) > maxPlayerCache {
		return cachedPlayer{}, false
	}
	var p cachedPlayer
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if d.Decode(&p) != nil || d.Decode(new(any)) != io.EOF || p.Schema != playerCacheSchema || p.Profile != playerProfile || !identifier.MatchString(p.PlayerID) || !validExtraction(p.Seed) || p.CheckedAt.IsZero() || p.CheckedAt.After(time.Now().Add(time.Minute)) || p.Checksum != p.checksum() {
		return cachedPlayer{}, false
	}
	return p, true
}

func (c playerCache) save(p cachedPlayer) {
	if c.dir == "" || os.MkdirAll(c.dir, 0700) != nil {
		return
	}
	p.Schema, p.Profile = playerCacheSchema, playerProfile
	p.Checksum = p.checksum()
	data, err := json.Marshal(p)
	if err != nil || len(data) > maxPlayerCache {
		return
	}
	f, err := os.CreateTemp(c.dir, ".player-*")
	if err != nil {
		return
	}
	defer os.Remove(f.Name())
	_, err = f.Write(data)
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil && closeErr == nil {
		// A failed write/rename never replaces a previously usable record.
		_ = os.Rename(f.Name(), filepath.Join(c.dir, "current.json"))
	}
}

// All access is serialized by Resolver.gate. This cache is deliberately outside
// resetHosts/Configure: public transforms do not belong to an account session.
func (r *Resolver) ensurePlayer(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if r.seed.Code == "" {
		if p, ok := r.playerCache.load(); ok {
			r.seed, r.playerID, r.playerChecked = p.Seed, p.PlayerID, p.CheckedAt
		}
	}
	now := time.Now()
	if r.seed.Code != "" && (now.Sub(r.playerChecked) < playerCheckEvery || now.Before(r.playerRetry)) {
		return nil
	}
	data, err := r.request(ctx, "GET", "https://www.youtube.com/iframe_api", nil, nil, "bootstrap", "player_identifier")
	if err != nil {
		// A brief version-check outage must not discard a known public transform.
		// Never mask cancellation, TLS/redirect/auth failures, or a known new ID.
		var f *Failure
		if ctx.Err() == nil && r.seed.Code != "" && now.Sub(r.playerChecked) < playerStaleFor && errors.As(err, &f) && (f.Category == "bootstrap_network_failed" || f.Category == "bootstrap_network_timeout" || f.HTTPStatus >= 500 && f.HTTPStatus <= 599) {
			r.playerRetry = now.Add(5 * time.Minute)
			return nil
		}
		return err
	}
	match := playerIDPattern.FindSubmatch(data)
	if len(match) != 2 {
		return &Failure{Category: "init_failed", Stage: "bootstrap", BootstrapStep: "player_identifier"}
	}
	id := string(match[1])
	if r.seed.Code == "" || r.playerID != id {
		script, err := r.request(ctx, "GET", "https://www.youtube.com/s/player/"+id+"/"+playerProfile+"/base.js", nil, nil, "bootstrap", "player_script")
		if err != nil {
			return err
		}
		seed, err := r.extractPlayer(ctx, script)
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		r.seed, r.playerID = seed, id
	}
	r.playerChecked, r.playerRetry = time.Now(), time.Time{}
	r.playerCache.save(cachedPlayer{PlayerID: r.playerID, CheckedAt: r.playerChecked, Seed: r.seed})
	return nil
}

func (r *Resolver) invalidatePlayer() {
	if p, ok := r.playerCache.load(); ok && p.PlayerID == r.playerID {
		_ = os.Remove(filepath.Join(r.playerCache.dir, "current.json"))
	}
	r.seed, r.playerID, r.playerChecked, r.playerRetry = extracted{}, "", time.Time{}, time.Time{}
}

func (r *Resolver) extractPlayer(ctx context.Context, script []byte) (extracted, error) {
	h, err := r.newHost("extract")
	if err != nil {
		return extracted{}, &Failure{Category: "init_failed", Stage: "bootstrap", BootstrapStep: "player_script", ScriptReason: "host_start"}
	}
	defer h.Close()
	var seed extracted
	err = h.call(ctx, "extractPlayer("+jsJSON(string(script))+")", &seed)
	if err != nil {
		if ctx.Err() != nil {
			return extracted{}, ctx.Err()
		}
		if err.Error() == "script_memory_limit" || err.Error() == "script_output_limit" {
			return extracted{}, &Failure{Category: "cache_limit", Stage: "bootstrap", BootstrapStep: "player_script"}
		}
		if err.Error() == "script_timeout" {
			return extracted{}, &Failure{Category: "request_timeout", Stage: "bootstrap", BootstrapStep: "player_script"}
		}
		reason := "script_failed"
		switch err.Error() {
		case "script_stack_limit", "player_timestamp_missing", "player_function_missing", "script_reference_error", "script_type_error", "script_syntax_error", "script_internal_error", "script_range_error":
			reason = err.Error()
		}
		return extracted{}, &Failure{Category: "init_failed", Stage: "bootstrap", BootstrapStep: "player_script", ScriptReason: reason}
	}
	if !validExtraction(seed) {
		return extracted{}, &Failure{Category: "init_failed", Stage: "bootstrap", BootstrapStep: "player_script", ScriptReason: "invalid_extraction"}
	}
	return seed, nil
}
