package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"ytmgo/internal/player"
)

// Only fixed categories cross this boundary. Never store dependency messages,
// URLs, headers, track metadata, account identifiers, secrets, argv, or environments.
func recordPlaybackDiagnostic(directory, component, category string, details ...player.FailureDetails) {
	if directory == "" || (component != "audio" && component != "video") {
		return
	}
	switch category {
	case "ready", "login_required", "stream_resolution_failed", "invalid_media_response", "audio_stream_failed",
		"audio_decoder_unavailable", "audio_output_unavailable", "runtime_missing", "audio_access_denied",
		"audio_rate_limited", "audio_server_error", "audio_network_timeout", "audio_connection_failed",
		"audio_range_mismatch", "audio_stream_truncated",
		"playback_timeout", "playback_failed", "video_resolution_failed", "video_decoder_unavailable",
		"video_stream_failed", "video_access_denied", "video_rate_limited", "video_server_error",
		"video_network_timeout", "video_connection_failed", "proof_failed", "proof_expired",
		"proof_request_rejected", "bootstrap_cache_miss",
		"unexpected_request_host", "unexpected_authentication", "unexpected_client", "resolver_http_error",
		"init_failed", "resolver_response_limit", "cache_limit",
		"bootstrap_redirect", "bootstrap_network_failed", "bootstrap_network_timeout", "bootstrap_tls_failed":
	default:
		category = "playback_failed"
	}
	metadata := player.FailureDetails{}
	if len(details) == 1 {
		metadata = details[0].Sanitized()
	}
	data, _ := json.Marshal(struct {
		Time, Version, Component, Category string
		Stage                              string `json:"Stage,omitempty"`
		BootstrapStep                      string `json:"BootstrapStep,omitempty"`
		HTTPStatus                         int    `json:"HTTPStatus,omitempty"`
		Redirect                           string `json:"Redirect,omitempty"`
		MIME                               string `json:"MIME,omitempty"`
	}{time.Now().UTC().Format(time.RFC3339), version, component, category,
		metadata.Stage, metadata.BootstrapStep, metadata.HTTPStatus, metadata.Redirect, metadata.MIME})
	if os.MkdirAll(directory, 0700) == nil {
		_ = os.WriteFile(filepath.Join(directory, component+"-status.json"), data, 0600)
	}
}

// Account diagnostics retain only a fixed lifecycle category, never the
// transferred session or an account identifier. This makes status acceptance
// observable without opening a credential store or inspecting process memory.
func recordAccountDiagnostic(directory, category string) {
	if directory == "" {
		return
	}
	switch category {
	case "restoring", "signed_out", "standard", "premium", "refresh_failed", "session_failed":
	default:
		category = "session_failed"
	}
	data, _ := json.Marshal(struct{ Time, Version, Component, Category string }{
		time.Now().UTC().Format(time.RFC3339), version, "account", category,
	})
	if os.MkdirAll(directory, 0700) == nil {
		_ = os.WriteFile(filepath.Join(directory, "account-status.json"), data, 0600)
	}
}
