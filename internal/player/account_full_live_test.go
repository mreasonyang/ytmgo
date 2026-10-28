package player

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"ytmgo/internal/accountbridge"
	"ytmgo/internal/catalog"
)

// This opt-in test accepts one explicit extension handoff into memory. It never
// opens a keychain or browser store and never saves a session. Evidence is a
// closed schema: fixed categories/stages, server flags, PID and numeric PCM
// metrics only. This is packaged-runtime acceptance, not native app acceptance.
func TestAccountFullDecodeLive(t *testing.T) {
	mode := os.Getenv("YTMGO_LIVE_ACCOUNT")
	cycleFlag := os.Getenv("YTMGO_LIVE_CYCLE")
	if cycleFlag != "" && cycleFlag != "1" {
		t.Fatal("account_live_cycle_flag_invalid")
	}
	cycle := cycleFlag == "1"
	if cycle && mode != "premium" {
		t.Fatal("account_live_cycle_requires_premium")
	}
	if mode == "" {
		t.Skip("set YTMGO_LIVE_ACCOUNT=standard or premium for explicit account handoff")
	}
	if mode != "standard" && mode != "premium" {
		t.Fatal("account_live_mode_invalid")
	}
	if runtime.GOOS != "darwin" || runtime.GOARCH != "arm64" {
		t.Fatal("account_live_package_target_unavailable")
	}
	type event struct {
		Event        string              `json:"event"`
		Category     string              `json:"category"`
		Stage        string              `json:"stage,omitempty"`
		PID          int                 `json:"pid,omitempty"`
		LoggedIn     *bool               `json:"logged_in,omitempty"`
		MusicPremium *bool               `json:"music_premium,omitempty"`
		Decode       *fullDecodeEvidence `json:"decode,omitempty"`
	}
	var evidence io.Writer = io.Discard
	evidenceDir := os.Getenv("YTMGO_LIVE_EVIDENCE")
	if dir := evidenceDir; dir != "" {
		if os.MkdirAll(filepath.Dir(dir), 0700) != nil || os.Mkdir(dir, 0700) != nil {
			t.Fatal("account_live_evidence_directory_failed")
		}
		file, err := os.OpenFile(filepath.Join(dir, "results.jsonl"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			t.Fatal("account_live_evidence_file_failed")
		}
		defer file.Close()
		evidence = file
	}
	emit := func(row event) {
		t.Helper()
		data, err := json.Marshal(row)
		if err != nil {
			t.Fatal("account_live_evidence_encoding_failed")
		}
		if _, err = evidence.Write(append(data, '\n')); err != nil {
			t.Fatal("account_live_evidence_write_failed")
		}
		t.Log(string(data))
	}
	phase := func(value string) {
		t.Helper()
		switch value {
		case "account_decode", "anonymous_warming", "anonymous_warmed", "premium_reconfiguring", "premium_reconfigured", "premium_restored_decode", "cycle_complete":
		default:
			t.Fatal("account_live_phase_invalid")
		}
		if evidenceDir != "" && os.WriteFile(filepath.Join(evidenceDir, "phase.txt"), []byte(value+"\n"), 0600) != nil {
			t.Fatal("account_live_phase_write_failed")
		}
	}
	// Select the actual current bundle explicitly; never use a source runtime or
	// fall back to an unrelated executable on the developer's PATH.
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("account_live_source_location_failed")
	}
	root := filepath.Dir(filepath.Dir(filepath.Dir(source)))
	t.Setenv("YTMGO_RUNTIME_DIR", filepath.Join(root, "build", "darwin-arm64", "ytmgo.app", "Contents", "Resources", "runtime"))
	paths, err := findRuntime()
	if err != nil {
		t.Fatal("account_live_package_runtime_missing")
	}
	deadline := 11 * time.Minute
	if cycle {
		deadline = 15 * time.Minute
	}
	ctx, cancel := context.WithTimeout(context.Background(), deadline)
	defer cancel()
	waitCtx, waitCancel := context.WithTimeout(ctx, 3*time.Minute)
	bridge, err := accountbridge.Listen(waitCtx)
	if err != nil {
		waitCancel()
		t.Fatal("account_live_bridge_unavailable")
	}
	defer bridge.Close()
	emit(event{Event: "handoff_ready", Category: "waiting", PID: os.Getpid()})
	session, err := bridge.Wait(waitCtx)
	bridge.Close()
	waitCancel()
	if err != nil {
		t.Fatal("account_live_handoff_failed")
	}
	client := catalog.NewWithOptions(catalog.Options{Language: "en", Region: "US"})
	defer client.ClearSession()
	cookies, err := session.HTTPCookies(time.Now())
	if err != nil || session.AccountIndex == nil {
		t.Fatal("account_live_session_invalid")
	}
	if client.SetSession(cookies, *session.AccountIndex, session.OnBehalfOfUser) != nil {
		t.Fatal("account_live_session_rejected")
	}
	clear(cookies)
	session = accountbridge.Session{}
	checkCtx, checkCancel := context.WithTimeout(ctx, 30*time.Second)
	status, err := client.AccountStatus(checkCtx)
	checkCancel()
	if err != nil {
		emit(event{Event: "account_verified", Category: "account_verification_failed"})
		t.Fatal("account_live_verification_failed")
	}
	emit(event{Event: "account_verified", Category: "pass", LoggedIn: &status.LoggedIn, MusicPremium: &status.MusicPremium})
	if !status.LoggedIn || status.MusicPremium != (mode == "premium") {
		t.Fatal("account_live_expected_state_mismatch")
	}
	credentials, ok := client.Credentials()
	if !ok {
		t.Fatal("account_live_credentials_unavailable")
	}
	p, err := New()
	if err != nil {
		t.Fatal("account_live_player_unavailable")
	}
	defer p.Close()
	configureCtx, configureCancel := context.WithTimeout(ctx, 15*time.Second)
	err = p.SetAccount(configureCtx, AccountSession{
		LoggedIn: status.LoggedIn, MusicPremium: status.MusicPremium,
		Cookie: credentials.Cookie, AccountIndex: credentials.AccountIndex,
		OnBehalfOfUser: credentials.OnBehalfOfUser, Revision: 1,
	})
	configureCancel()
	credentials = catalog.SessionCredentials{}
	client.ClearSession()
	if err != nil {
		emit(event{Event: "account_configured", Category: safeError(err), Stage: safeErrorStage(err)})
		t.Fatal("account_live_configuration_failed")
	}
	emit(event{Event: "account_configured", Category: "pass"})
	// decodeFullTrack uses the configured production resolver and packaged
	// FFmpeg without constructing a native audio output or changing volume.
	phase("account_decode")
	tracks := []string{"52fNscjkST4", "4D7u5KF7SP8"}
	if cycle {
		// One complete recording on each side of the transition is sufficient
		// for this lifecycle test; the two-track test remains the default.
		tracks = tracks[:1]
	}
	for index, id := range tracks {
		row := decodeFullTrack(ctx, paths, p.resolver, index+1, id)
		emit(event{Event: "full_decode", Category: row.Category, Stage: row.Stage, Decode: &row})
		if row.Category != "pass" {
			t.Fatal("account_live_full_decode_failed")
		}
	}
	if cycle {
		// Keep only the server-verified Premium session, in memory. Revisions
		// deliberately change identity: this exercises an actual anonymous
		// configuration followed by restoration, not a simulated subscription
		// downgrade or a fabricated Premium entitlement for a free account.
		p.mu.Lock()
		premium := p.account
		p.mu.Unlock()
		defer func() { premium = AccountSession{} }()
		if !premium.LoggedIn || !premium.MusicPremium {
			t.Fatal("account_live_cycle_verified_state_missing")
		}
		configure := func(next AccountSession, eventName string) {
			t.Helper()
			configureCtx, configureCancel := context.WithTimeout(ctx, 15*time.Second)
			err := p.SetAccount(configureCtx, next)
			configureCancel()
			if err != nil {
				emit(event{Event: eventName, Category: safeError(err), Stage: safeErrorStage(err)})
				t.Fatal("account_live_cycle_configuration_failed")
			}
		}
		hold := func() {
			t.Helper()
			timer := time.NewTimer(10 * time.Second)
			defer timer.Stop()
			select {
			case <-timer.C:
			case <-ctx.Done():
				t.Fatal("account_live_cycle_observation_timeout")
			}
		}
		phase("anonymous_warming")
		configure(AccountSession{Revision: premium.Revision + 1}, "anonymous_configured")
		warmCtx, warmCancel := context.WithTimeout(ctx, 45*time.Second)
		err := p.resolver.Warm(warmCtx)
		warmCancel()
		if err != nil {
			emit(event{Event: "anonymous_warmed", Category: safeError(err), Stage: safeErrorStage(err)})
			t.Fatal("account_live_cycle_anonymous_warm_failed")
		}
		phase("anonymous_warmed")
		emit(event{Event: "anonymous_warmed", Category: "pass"})
		hold()

		phase("premium_reconfiguring")
		premium.Revision += 2
		configure(premium, "premium_reconfigured")
		phase("premium_reconfigured")
		emit(event{Event: "premium_reconfigured", Category: "pass"})
		hold()

		phase("premium_restored_decode")
		row := decodeFullTrack(ctx, paths, p.resolver, 1, "52fNscjkST4")
		emit(event{Event: "premium_restored_full_decode", Category: row.Category, Stage: row.Stage, Decode: &row})
		if row.Category != "pass" {
			t.Fatal("account_live_cycle_restored_decode_failed")
		}
		phase("cycle_complete")
		emit(event{Event: "cycle_complete", Category: "pass"})
	}
	emit(event{Event: "complete", Category: "pass"})
}
