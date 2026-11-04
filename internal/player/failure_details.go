package player

import (
	"errors"
	"ytmgo/internal/streaming"
)

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
	case "media_stream":
		if d.HTTPStatus >= 100 && d.HTTPStatus <= 599 {
			return FailureDetails{Stage: d.Stage, HTTPStatus: d.HTTPStatus}
		}
		return FailureDetails{Stage: d.Stage}
	case "sdk_create", "account_validation", "proof_config_request", "proof_config_parse", "proof_init", "player_request", "decipher", "proof_mint", "video_resolution", "video_first_frame", "video_playback":
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
	var media *streaming.Failure
	if errors.As(err, &media) && media.Stage == "media_stream" {
		return (FailureDetails{Stage: media.Stage, HTTPStatus: media.HTTPStatus}).Sanitized()
	}
	return FailureDetails{}
}
