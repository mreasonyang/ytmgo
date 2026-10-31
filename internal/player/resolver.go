package player

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"runtime"
	"ytmgo/internal/streaming"
)

type resolver struct{ native *streaming.Resolver }

func (paths runtimePaths) newResolver() (*resolver, error) {
	native, err := streaming.New(streaming.Options{Command: func(ctx context.Context, mode string) *exec.Cmd {
		cmd := childCommand(ctx, paths.helper, mode)
		if mode == "dom" && runtime.GOOS == "linux" {
			for _, name := range []string{"DISPLAY", "WAYLAND_DISPLAY", "XDG_RUNTIME_DIR", "XAUTHORITY", "DBUS_SESSION_BUS_ADDRESS"} {
				if value := os.Getenv(name); value != "" {
					cmd.Env = append(cmd.Env, name+"="+value)
				}
			}
		}
		return cmd
	}})
	if err != nil {
		return nil, err
	}
	return &resolver{native: native}, nil
}
func nativeFailure(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	var failure *streaming.Failure
	if errors.As(err, &failure) {
		return resolverFailureError(failure.Category, FailureDetails{Stage: failure.Stage, BootstrapStep: failure.BootstrapStep, HTTPStatus: failure.HTTPStatus, Redirect: failure.Redirect, MIME: failure.MIME})
	}
	return resolverError(err.Error())
}
func (r *resolver) Warm(ctx context.Context) error { return nativeFailure(r.native.Warm(ctx)) }
func (r *resolver) Configure(ctx context.Context, s AccountSession) error {
	if !s.valid() {
		return errors.New("invalid_account_session")
	}
	return nativeFailure(r.native.Configure(ctx, streaming.Session(s)))
}
func (r *resolver) Resolve(ctx context.Context, id string) (media, error) {
	result, err := r.native.Resolve(ctx, id, "audio")
	if err != nil {
		return media{}, nativeFailure(err)
	}
	return media{source: result.Lease, Duration: result.Duration, release: result.Lease.Close}, nil
}
func (r *resolver) ResolveVideo(ctx context.Context, id string) (videoMedia, error) {
	result, err := r.native.Resolve(ctx, id, "video")
	if err != nil {
		return videoMedia{}, nativeFailure(err)
	}
	width, height := videoDimensions(result.Width, result.Height)
	return videoMedia{media: media{source: result.Lease, Duration: result.Duration, release: result.Lease.Close}, width: width, height: height}, nil
}
func (r *resolver) Close() { r.native.Close() }

func resolverError(category string) error {
	switch category {
	case "request_cancelled":
		return context.Canceled
	case "request_timeout":
		return context.DeadlineExceeded
	case "resolver_access_denied":
		return errors.New("audio_access_denied")
	case "resolver_rate_limited":
		return errors.New("audio_rate_limited")
	case "login_required", "playability_unavailable", "drm_unsupported", "sabr_unsupported", "no_audio_format", "no_video_format", "media_expired",
		"proof_failed", "proof_expired", "proof_request_rejected", "bootstrap_cache_miss",
		"unexpected_request_host", "unexpected_authentication", "unexpected_client", "resolver_http_error",
		"bootstrap_redirect", "bootstrap_network_failed", "bootstrap_network_timeout", "bootstrap_tls_failed",
		"init_failed", "resolver_response_limit", "cache_limit":
		return errors.New(category)
	default:
		return errors.New("stream_resolution_failed")
	}
}

type resolutionFailure struct {
	err     error
	details FailureDetails
}

func (e *resolutionFailure) Error() string { return e.err.Error() }
func (e *resolutionFailure) Unwrap() error { return e.err }

func resolverFailureError(category string, details FailureDetails) error {
	err := resolverError(category)
	details = details.Sanitized()
	if details.Stage != "" {
		return &resolutionFailure{err: err, details: details}
	}
	return err
}

func safeErrorStage(err error) string {
	return DiagnosticDetails(err).Stage
}
