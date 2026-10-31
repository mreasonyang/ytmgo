package player

import (
	"context"
	"errors"
	"testing"
)

func TestResolverDiagnosticCategoriesRemainBounded(t *testing.T) {
	if safeErrorStage(resolverFailureError("init_failed", FailureDetails{Stage: "proof_config_request"})) != "proof_config_request" ||
		safeErrorStage(resolverFailureError("init_failed", FailureDetails{Stage: "synthetic-private-stage"})) != "" {
		t.Fatal("resolver stage boundary failed")
	}
	if !errors.Is(resolverFailureError("request_timeout", FailureDetails{Stage: "bootstrap"}), context.DeadlineExceeded) {
		t.Fatal("resolver stage hid timeout")
	}
	for _, category := range []string{"unexpected_request_host", "unexpected_authentication", "unexpected_client", "resolver_http_error", "init_failed", "resolver_response_limit", "cache_limit"} {
		if got := safeError(resolverError(category)); got != category {
			t.Fatalf("fixed category lost: %s", category)
		}
		if got := safeError(resolverError(category + ": synthetic private details")); got != "stream_resolution_failed" {
			t.Fatal("untrusted diagnostic details escaped")
		}
	}
}
