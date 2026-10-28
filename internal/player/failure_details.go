package player

import "errors"

// FailureDetails contains only fixed categories and bounded HTTP status codes.
// It never contains upstream messages, URLs, headers, bodies or account data.
type FailureDetails struct {
	Stage         string
	BootstrapStep string
	HTTPStatus    int
	Redirect      string
	MIME          string
}

func (d FailureDetails) validBootstrap() bool {
	switch d.BootstrapStep {
	case "", "session_data", "player_identifier", "player_script", "other":
	default:
		return false
	}
	if d.HTTPStatus != 0 && (d.HTTPStatus < 100 || d.HTTPStatus > 599) {
		return false
	}
	switch d.Redirect {
	case "", "google_challenge", "signin", "consent", "same_origin", "other":
	default:
		return false
	}
	switch d.MIME {
	case "", "html", "json", "javascript", "text", "other", "missing":
	default:
		return false
	}
	return true
}

// Sanitized is also applied at the native diagnostic boundary, independently
// of IPC validation, so callers cannot persist arbitrary diagnostic strings.
func (d FailureDetails) Sanitized() FailureDetails {
	switch d.Stage {
	case "bootstrap":
		if d.validBootstrap() {
			return d
		}
		return FailureDetails{Stage: d.Stage}
	case "sdk_create", "account_validation", "proof_config_request", "proof_config_parse", "proof_init", "player_request", "decipher", "proof_mint":
		return FailureDetails{Stage: d.Stage}
	default:
		return FailureDetails{}
	}
}

// DiagnosticDetails extracts the closed metadata from a wrapped playback
// failure. Raw dependency errors have no diagnostic metadata.
func DiagnosticDetails(err error) FailureDetails {
	var failure *resolutionFailure
	if errors.As(err, &failure) {
		return failure.details.Sanitized()
	}
	return FailureDetails{}
}
