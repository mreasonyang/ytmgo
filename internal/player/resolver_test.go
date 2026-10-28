package player

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
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

func TestResolverProcess(t *testing.T) {
	// This is a synthetic child fixture, never an application process lookup.
	args := os.Args
	if len(args) < 3 || args[len(args)-2] != "--resolver-fixture" {
		return
	}
	mode := args[len(args)-1]
	if mode == "oversized" {
		fmt.Print(strings.Repeat("x", resolverReplyLimit+1))
		os.Exit(0)
	}
	if mode == "malformed" {
		fmt.Println("not a protocol response")
		os.Exit(0)
	}
	output := json.NewEncoder(os.Stdout)
	scanner := bufio.NewScanner(os.Stdin)
	var held uint64
	released := 0
	mediaResponse := func(id uint64) {
		_ = output.Encode(map[string]any{"id": id, "ok": true, "media": map[string]any{
			"url": "http://127.0.0.1:23456/" + strings.Repeat("a", 64), "duration": 120,
			"http_headers": map[string]string{"User-Agent": "fixture"},
		}})
	}
	for scanner.Scan() {
		var request workerRequest
		if json.Unmarshal(scanner.Bytes(), &request) != nil {
			os.Exit(2)
		}
		switch request.Op {
		case "configure":
			_ = output.Encode(workerReply{ID: request.ID, OK: request.Session != nil && request.Session.valid()})
		case "warm":
			_ = output.Encode(workerReply{ID: request.ID, OK: true, Category: strconv.Itoa(released)})
		case "resolve":
			switch request.VideoID {
			case "BBBBBBBBBBB":
				held = request.ID
			case "CCCCCCCCCCC":
				_ = output.Encode(workerReply{ID: request.ID, Category: "synthetic dependency error with private details"})
			default:
				mediaResponse(request.ID)
			}
		case "cancel":
			if held == request.Target {
				mediaResponse(held)
				held = 0
			}
			_ = output.Encode(workerReply{ID: request.ID, OK: true})
		case "release":
			released++
			_ = output.Encode(workerReply{ID: request.ID, OK: true})
		}
	}
	os.Exit(0)
}

func fixtureResolver(t *testing.T, mode string) *resolver {
	t.Helper()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal("fixture executable unavailable")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cmd := childCommand(ctx, executable, "-test.run=^TestResolverProcess$", "--", "--resolver-fixture", mode)
	r, err := launchResolver(ctx, cancel, cmd)
	if err != nil {
		t.Fatal("fixture launch failed")
	}
	t.Cleanup(r.Close)
	return r
}

func TestResolverReusesSessionAndReleasesLeaseOnce(t *testing.T) {
	r := fixtureResolver(t, "normal")
	if err := r.Warm(context.Background()); err != nil {
		t.Fatal("warm failed")
	}
	info, err := r.Resolve(context.Background(), "AAAAAAAAAAA")
	if err != nil || info.Duration != 2*time.Minute || info.release == nil {
		t.Fatal("resolve failed")
	}
	info.release()
	info.release()
	reply, err := r.call(context.Background(), workerRequest{Op: "warm"})
	if err != nil || reply.Category != "1" {
		t.Fatal("lease was not released exactly once")
	}
}

func TestResolverCancellationReleasesLateResponse(t *testing.T) {
	r := fixtureResolver(t, "normal")
	if err := r.Warm(context.Background()); err != nil {
		t.Fatal("warm failed")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	_, err := r.Resolve(ctx, "BBBBBBBBBBB")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("request did not cancel")
	}
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		reply, err := r.call(context.Background(), workerRequest{Op: "warm"})
		if err != nil {
			t.Fatal("cancellation killed usable session")
		}
		if reply.Category == "1" {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("late response retained its media capability")
}

func TestResolverFailureDoesNotExposeWorkerText(t *testing.T) {
	r := fixtureResolver(t, "normal")
	_, err := r.Resolve(context.Background(), "CCCCCCCCCCC")
	if err == nil || err.Error() != "stream_resolution_failed" {
		t.Fatal("raw worker text escaped")
	}
}

func TestResolverRejectsMalformedOrOversizedReplies(t *testing.T) {
	for _, mode := range []string{"oversized", "malformed"} {
		t.Run(mode, func(t *testing.T) {
			r := fixtureResolver(t, mode)
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			if err := r.Warm(ctx); err == nil || err.Error() != "stream_resolution_failed" {
				t.Fatal("invalid worker output accepted")
			}
		})
	}
}
