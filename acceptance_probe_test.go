package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"ytmgo/internal/player"
)

func TestAcceptanceProbeRequiresBoundedExplicitOptIn(t *testing.T) {
	now := time.Now()
	for _, test := range []struct {
		name, body string
		valid      bool
	}{
		{"absent", "", false},
		{"malformed", "{", false},
		{"oversized", strings.Repeat(" ", 1025), false},
		{"wrong_version", `{"version":"0.0.0","until":"` + now.Add(time.Hour).Format(time.RFC3339Nano) + `"}`, false},
		{"expired", `{"version":"` + version + `","until":"` + now.Add(-time.Second).Format(time.RFC3339Nano) + `"}`, false},
		{"unbounded", `{"version":"` + version + `","until":"` + now.Add(5*time.Hour).Format(time.RFC3339Nano) + `"}`, false},
		{"unknown_field", `{"version":"` + version + `","until":"` + now.Add(time.Hour).Format(time.RFC3339Nano) + `","extra":true}`, false},
		{"trailing_data", `{"version":"` + version + `","until":"` + now.Add(time.Hour).Format(time.RFC3339Nano) + `"} {}`, false},
		{"valid", `{"version":"` + version + `","until":"` + now.Add(time.Hour).Format(time.RFC3339Nano) + `"}`, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			if test.body != "" {
				os.WriteFile(filepath.Join(dir, "acceptance-request.json"), []byte(test.body), 0600)
			}
			probe := loadAcceptanceProbe(dir, now)
			if (probe != nil) != test.valid {
				t.Fatal("opt-in bound was not enforced")
			}
		})
	}
}

func TestAcceptanceProbeDoesNotFollowRequestSymlinks(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "synthetic-request")
	now := time.Now()
	data, _ := json.Marshal(acceptanceRequest{Version: version, Until: now.Add(time.Hour)})
	os.WriteFile(target, data, 0600)
	if err := os.Symlink(target, filepath.Join(dir, "acceptance-request.json")); err != nil {
		t.Skip("symlink fixture is unavailable")
	}
	if loadAcceptanceProbe(dir, now) != nil {
		t.Fatal("probe followed a request symlink")
	}
}

func TestAcceptanceProbeWritesOnlyClosedStateAndExpires(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()
	p := &acceptanceProbe{directory: dir, started: now, until: now.Add(time.Minute)}
	state := player.State{Position: 19*time.Second + 150*time.Millisecond, Duration: time.Minute, Playing: true, Error: "synthetic-private-dependency-value"}
	p.observe(now, state)
	path := filepath.Join(dir, "acceptance-playback.json")
	first, err := os.ReadFile(path)
	if err != nil || strings.Contains(string(first), "synthetic") {
		t.Fatal("closed playback state was not safely recorded")
	}
	var fields map[string]any
	if json.Unmarshal(first, &fields) != nil || len(fields) != 10 || fields["PositionMS"] != float64(19150) || fields["Failed"] != true || fields["Playing"] != true {
		t.Fatal("acceptance schema changed")
	}
	p.observe(now.Add(time.Second), player.State{})
	unchanged, _ := os.ReadFile(path)
	if string(first) != string(unchanged) {
		t.Fatal("probe exceeded its five-second cadence")
	}
	p.observe(now.Add(5*time.Second), player.State{})
	updated, _ := os.ReadFile(path)
	if string(first) == string(updated) {
		t.Fatal("stopped playback did not produce a fresh heartbeat")
	}
	p.observe(now.Add(time.Minute), state)
	expired, _ := os.ReadFile(path)
	if string(updated) != string(expired) {
		t.Fatal("probe continued after expiry")
	}
	info, _ := os.Stat(path)
	if info.Mode().Perm() != 0600 {
		t.Fatal("acceptance state has broad permissions")
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Fatal("probe retained temporary files")
	}
	var disabled *acceptanceProbe
	disabled.observe(now, state)
}
