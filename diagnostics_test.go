package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ytmgo/internal/player"
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
		recordPlaybackDiagnostic(dir, "audio", "init_failed", player.FailureDetails{Stage: stage})
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

func TestBootstrapDiagnosticsContainOnlyClosedMetadata(t *testing.T) {
	dir := t.TempDir()
	details := player.FailureDetails{Stage: "bootstrap", BootstrapStep: "session_data", HTTPStatus: 302, Redirect: "google_challenge", MIME: "html"}
	for _, category := range []string{"bootstrap_redirect", "bootstrap_network_failed", "bootstrap_network_timeout", "bootstrap_tls_failed"} {
		recordPlaybackDiagnostic(dir, "audio", category, details)
		data, err := os.ReadFile(filepath.Join(dir, "audio-status.json"))
		var record map[string]any
		if err != nil || json.Unmarshal(data, &record) != nil || len(record) != 9 || record["Category"] != category || record["Stage"] != "bootstrap" || record["BootstrapStep"] != "session_data" || record["HTTPStatus"] != float64(302) || record["Redirect"] != "google_challenge" || record["MIME"] != "html" {
			t.Fatal("fixed bootstrap metadata did not reach native diagnostics")
		}
	}
	for _, invalid := range []player.FailureDetails{
		{Stage: "bootstrap", BootstrapStep: "private-detail", HTTPStatus: 302},
		{Stage: "bootstrap", HTTPStatus: 600},
		{Stage: "bootstrap", Redirect: "https://example.invalid/private"},
		{Stage: "bootstrap", MIME: "text/html; private-detail"},
		{Stage: "player_request", BootstrapStep: "session_data", HTTPStatus: 403, Redirect: "other", MIME: "html"},
	} {
		recordPlaybackDiagnostic(dir, "audio", "init_failed", invalid)
		data, err := os.ReadFile(filepath.Join(dir, "audio-status.json"))
		var record map[string]any
		if err != nil || json.Unmarshal(data, &record) != nil || len(record) != 5 || strings.Contains(string(data), "private") || strings.Contains(string(data), "https") {
			t.Fatal("untrusted or non-bootstrap details persisted")
		}
	}
}
