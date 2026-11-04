package player

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"ytmgo/internal/catalog"
)

type videoStartupReadTrace struct {
	OpenedMS, OpenDurationMS                   int64
	FirstReadMS, FirstBytesMS, CompleteFrameMS int64
	Bytes                                      int64
	Cause, BridgeCategory                      string
	HTTPStatus                                 int
}

type tracedVideoReader struct {
	io.ReadCloser
	start      time.Time
	frameBytes int64
	trace      *videoStartupReadTrace
}

func (r *tracedVideoReader) Read(dst []byte) (int, error) {
	if r.trace.FirstReadMS == 0 {
		r.trace.FirstReadMS = time.Since(r.start).Milliseconds()
	}
	n, err := r.ReadCloser.Read(dst)
	if n > 0 && r.trace.FirstBytesMS == 0 {
		r.trace.FirstBytesMS = time.Since(r.start).Milliseconds()
	}
	r.trace.Bytes += int64(n)
	if r.trace.CompleteFrameMS == 0 && r.trace.Bytes >= r.frameBytes {
		r.trace.CompleteFrameMS = time.Since(r.start).Milliseconds()
	}
	if err != nil {
		if err == io.EOF {
			r.trace.Cause = "eof"
		} else {
			r.trace.Cause = "reader_closed"
		}
	}
	return n, err
}

type videoStartupTrace struct {
	ResolutionBudgetMS                                      int64
	VideoDetails                                            FailureDetails
	Case                                                    int
	InjectedResolveDelayMS                                  int64
	PolicyMS, TotalMS, ResolveMS, PresentedMS               int64
	ResolveCategory, ResolveStage, ResolveCause, VideoError string
	Presented                                               bool
	Decoders                                                []*videoStartupReadTrace
}

// Public, serial, cold-resolver A/B/B/A runs. Only stage timing, fixed
// categories, byte counts and HTTP status cross the evidence boundary.
func TestNativeVideoStartupTraceLive(t *testing.T) {
	if os.Getenv("YTMGO_LIVE_VIDEO_STARTUP_TRACE") != "1" {
		t.Skip("public startup tracing not selected")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Minute)
	defer cancel()
	client := catalog.New()
	defer client.ClearSession()
	page, err := client.Search(ctx, "P!nk Try Official Video")
	if err != nil {
		t.Fatal("public trace lookup failed")
	}
	id := ""
	for _, item := range page.Items {
		if item.Kind == "video" && strings.Contains(strings.ToLower(item.Title), "try") && strings.Contains(strings.ToLower(item.Title), "official video") && strings.Contains(strings.ToLower(item.Artist), "p!nk") {
			id = item.ID
			break
		}
	}
	if !videoIDPattern.MatchString(id) {
		t.Fatal("trace recording unavailable")
	}
	rows := []videoStartupTrace{}
	for index, budget := range []time.Duration{20 * time.Second, 60 * time.Second, 60 * time.Second, 20 * time.Second} {
		row := traceVideoStartup(t, ctx, id, budget)
		row.Case = index + 1
		rows = append(rows, row)
		if path := os.Getenv("YTMGO_LIVE_EVIDENCE"); path != "" {
			data, _ := json.MarshalIndent(rows, "", "  ")
			if os.WriteFile(path, append(data, '\n'), 0600) != nil {
				t.Fatal("trace evidence write failed")
			}
		}
		t.Logf("case=%d budget_ms=%d presented=%t total_ms=%d resolve_ms=%d", row.Case, row.PolicyMS, row.Presented, row.TotalMS, row.ResolveMS)
		if os.Getenv("YTMGO_VIDEO_TRACE_REQUIRE_FRAMES") == "1" && !row.Presented {
			t.Errorf("startup trace case %d did not present a frame", row.Case)
		}
	}
}

func traceVideoStartup(t *testing.T, parent context.Context, id string, budget time.Duration) videoStartupTrace {
	t.Helper()
	row := videoStartupTrace{PolicyMS: budget.Milliseconds()}
	if raw := os.Getenv("YTMGO_VIDEO_TRACE_RESOLVE_DELAY_MS"); raw != "" {
		delay, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || delay < 0 || delay > 30000 {
			t.Fatal("trace delay outside allowed range")
		}
		row.InjectedResolveDelayMS = delay
	}
	p, err := New()
	if err != nil {
		t.Fatal("trace runtime unavailable")
	}
	defer p.Close()
	initializeAnonymousLive(t, p)
	v, err := NewVideo(p)
	if err != nil {
		t.Fatal("trace runtime unavailable")
	}
	defer v.Close()
	v.policy.startupTimeout = budget
	row.ResolutionBudgetMS = v.policy.defaults().resolutionTimeout.Milliseconds()
	start := time.Now()
	resolve := v.resolve
	v.resolve = func(ctx context.Context, id string, quality int) (videoMedia, error) {
		began := time.Now()
		info, err := resolve(ctx, id, quality)
		if err == nil && row.InjectedResolveDelayMS > 0 {
			timer := time.NewTimer(time.Duration(row.InjectedResolveDelayMS) * time.Millisecond)
			select {
			case <-timer.C:
			case <-ctx.Done():
				timer.Stop()
				if info.release != nil {
					info.release()
				}
				info, err = videoMedia{}, ctx.Err()
			}
		}
		row.ResolveMS = time.Since(began).Milliseconds()
		if err != nil {
			row.ResolveCategory, row.ResolveStage = safeError(err), safeErrorStage(err)
			switch {
			case errors.Is(context.Cause(ctx), errVideoResolutionTimeout):
				row.ResolveCause = "resolution_deadline"
			case errors.Is(context.Cause(ctx), errVideoReadiness):
				row.ResolveCause = "startup_deadline"
			case errors.Is(err, context.Canceled):
				row.ResolveCause = "canceled"
			case errors.Is(err, context.DeadlineExceeded):
				row.ResolveCause = "request_deadline"
			default:
				row.ResolveCause = "upstream_failure"
			}
		}
		return info, err
	}
	open := v.open
	var decoders []*videoDecoder
	v.open = func(ctx context.Context, info videoMedia, at time.Duration) (*videoDecoder, error) {
		trace := &videoStartupReadTrace{OpenedMS: time.Since(start).Milliseconds()}
		row.Decoders = append(row.Decoders, trace)
		began := time.Now()
		d, err := open(ctx, info, at)
		trace.OpenDurationMS = time.Since(began).Milliseconds()
		if err == nil {
			d.reader = &tracedVideoReader{ReadCloser: d.reader, start: start, frameBytes: int64(info.width * info.height * 4), trace: trace}
			decoders = append(decoders, d)
		}
		return d, err
	}
	policy := v.policy.defaults()
	ctx, cancel := context.WithTimeout(parent, policy.resolutionTimeout+policy.startupTimeout+5*time.Second)
	defer cancel()
	if v.Start(ctx, id, func() time.Duration { return 73*time.Second + time.Since(start) }, func() bool { return true }) != nil {
		t.Fatal("trace start failed")
	}
	for v.Loading() && ctx.Err() == nil {
		time.Sleep(20 * time.Millisecond)
	}
	frame, _ := v.Frame()
	row.Presented = frame != nil && v.Error() == "" && ctx.Err() == nil
	row.VideoError = v.Error()
	row.VideoDetails = v.ErrorDetails()
	if row.Presented {
		row.PresentedMS = time.Since(start).Milliseconds()
		time.Sleep(200 * time.Millisecond)
	}
	v.Stop()
	for index, d := range decoders {
		if d.bridge != nil {
			if err := d.bridge.Err(); err != nil {
				row.Decoders[index].BridgeCategory = safeError(err)
				row.Decoders[index].HTTPStatus = DiagnosticDetails(err).HTTPStatus
			}
		}
	}
	row.TotalMS = time.Since(start).Milliseconds()
	return row
}
