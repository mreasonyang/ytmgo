package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPlaybackDiagnosticsExcludeUntrustedDetails(t *testing.T) {
	dir := t.TempDir()
	recordPlaybackDiagnostic(dir, "audio", "synthetic-private-detail https://example.invalid/media")
	data, err := os.ReadFile(filepath.Join(dir, "audio-status.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "synthetic") || strings.Contains(string(data), "https") {
		t.Fatal("untrusted error persisted")
	}
	var status struct{ Category string }
	if err := json.Unmarshal(data, &status); err != nil || status.Category != "playback_failed" {
		t.Fatalf("invalid safe status: %s", data)
	}
	recordPlaybackDiagnostic(dir, "../outside", "ready")
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 1 {
		t.Fatal("unexpected diagnostic output")
	}
}

func TestAccountDiagnosticsContainOnlyFixedState(t *testing.T) {
	dir := t.TempDir()
	for _, category := range []string{"premium", "standard", "signed_out", "synthetic-private-session"} {
		recordAccountDiagnostic(dir, category)
		data, err := os.ReadFile(filepath.Join(dir, "account-status.json"))
		if err != nil || strings.Contains(string(data), "synthetic") {
			t.Fatal("account diagnostic exposed input")
		}
		var value map[string]string
		if json.Unmarshal(data, &value) != nil || len(value) != 4 || value["Component"] != "account" {
			t.Fatal("account diagnostic shape changed")
		}
	}
}

func TestResolverDiagnosticCategoriesPersistWithoutDetails(t *testing.T) {
	dir := t.TempDir()
	for _, category := range []string{"unexpected_request_host", "unexpected_authentication", "unexpected_client", "resolver_http_error", "init_failed", "resolver_response_limit", "cache_limit"} {
		recordPlaybackDiagnostic(dir, "audio", category)
		data, err := os.ReadFile(filepath.Join(dir, "audio-status.json"))
		var status struct{ Category string }
		if err != nil || json.Unmarshal(data, &status) != nil || status.Category != category {
			t.Fatal("fixed diagnostic category lost")
		}
		recordPlaybackDiagnostic(dir, "audio", category+": synthetic private details")
		data, err = os.ReadFile(filepath.Join(dir, "audio-status.json"))
		if err != nil || json.Unmarshal(data, &status) != nil || status.Category != "playback_failed" {
			t.Fatal("untrusted diagnostic details persisted")
		}
	}
}

func TestResolverDiagnosticStageNeverIncludesUntrustedData(t *testing.T) {
	dir := t.TempDir()
	for _, stage := range []string{"proof_config_request", "synthetic-private-stage"} {
		recordPlaybackDiagnostic(dir, "audio", "init_failed", stage)
		data, err := os.ReadFile(filepath.Join(dir, "audio-status.json"))
		if err != nil || strings.Contains(string(data), "synthetic") {
			t.Fatal("untrusted stage persisted")
		}
		var status struct{ Stage string }
		if json.Unmarshal(data, &status) != nil || (stage == "proof_config_request" && status.Stage != stage) {
			t.Fatal("fixed stage lost")
		}
	}
}
