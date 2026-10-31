package player

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"ytmgo/internal/streaming"
)

func TestBootstrapReplyMetadataSurvivesErrorAndPlayerState(t *testing.T) {
	want := FailureDetails{Stage: "bootstrap", BootstrapStep: "session_data", HTTPStatus: 302, Redirect: "google_challenge", MIME: "html"}
	err := fmt.Errorf("wrapped fixture: %w", nativeFailure(&streaming.Failure{Category: "bootstrap_redirect", Stage: "bootstrap", BootstrapStep: "session_data", HTTPStatus: 302, Redirect: "google_challenge", MIME: "html"}))
	if DiagnosticDetails(err) != want || safeError(err) != "bootstrap_redirect" || strings.Contains(err.Error(), "google_challenge") {
		t.Fatal("closed metadata was lost or added to the error string")
	}
	p := &Player{gen: 1}
	p.fail(1, err)
	if state := p.Snapshot(); state.Error != "bootstrap_redirect" || state.ErrorDetails != want {
		t.Fatal("player state lost bootstrap metadata")
	}
	// A new unrelated failure must not retain the previous HTTP response.
	p.fail(1, errors.New("audio_stream_failed"))
	if p.Snapshot().ErrorDetails != (FailureDetails{}) {
		t.Fatal("old bootstrap metadata survived a new failure")
	}
	if !errors.Is(resolverFailureError("request_timeout", FailureDetails{Stage: "bootstrap"}), context.DeadlineExceeded) {
		t.Fatal("structured metadata changed timeout identity")
	}
}

func TestNativeFailureDoesNotExposeUnknownMetadata(t *testing.T) {
	for _, failure := range []*streaming.Failure{
		{Category: "private-value", Stage: "private-value"},
		{Category: "init_failed", Stage: "bootstrap", Redirect: "https://example.invalid/private"},
		{Category: "init_failed", Stage: "bootstrap", HTTPStatus: 600},
		{Category: "init_failed", Stage: "bootstrap", BootstrapStep: "private-value"},
	} {
		err := nativeFailure(failure)
		if strings.Contains(err.Error(), "private") || strings.Contains(fmt.Sprint(DiagnosticDetails(err)), "private") {
			t.Fatal("private diagnostic escaped")
		}
	}
}

func TestBootstrapCategoriesAreNotReducedToGenericFailure(t *testing.T) {
	for _, category := range []string{"bootstrap_redirect", "bootstrap_network_failed", "bootstrap_network_timeout", "bootstrap_tls_failed"} {
		err := resolverFailureError(category, FailureDetails{Stage: "bootstrap", BootstrapStep: "other"})
		if safeError(err) != category {
			t.Fatal("bootstrap category became generic")
		}
	}
}

func TestExplicitLoginRequirementSurvivesIntoPlayerState(t *testing.T) {
	p := &Player{gen: 1}
	p.fail(1, resolverFailureError("login_required", FailureDetails{Stage: "player_request"}))
	if p.Snapshot().Error != "login_required" {
		t.Fatal("explicit service sign-in requirement was reduced to a generic playback failure")
	}
}
