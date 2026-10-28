package player

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os/exec"
	"sync"
	"time"
)

const resolverReplyLimit = 256 << 10

type workerRequest struct {
	ID      uint64          `json:"id"`
	Op      string          `json:"op"`
	VideoID string          `json:"videoId,omitempty"`
	Kind    string          `json:"kind,omitempty"`
	URL     string          `json:"url,omitempty"`
	Target  uint64          `json:"target,omitempty"`
	Session *AccountSession `json:"session,omitempty"`
}

type workerReply struct {
	ID       uint64          `json:"id"`
	OK       bool            `json:"ok"`
	Category string          `json:"category"`
	Stage    string          `json:"stage"`
	Media    json.RawMessage `json:"media"`
}

// resolver owns the current account's playback session. Its stdout is a private data
// pipe: media capabilities, SDK responses and stderr never become diagnostics.
type resolver struct {
	mu        sync.Mutex
	sequence  uint64
	pending   map[uint64]chan workerReply
	writes    chan workerRequest
	done      chan struct{}
	cancel    context.CancelFunc
	closeOnce sync.Once
}

func (paths runtimePaths) newResolver() (*resolver, error) {
	life, cancel := context.WithCancel(context.Background())
	cmd := childCommand(life, paths.node, "--no-warnings", paths.worker)
	return launchResolver(life, cancel, cmd)
}

func launchResolver(life context.Context, cancel context.CancelFunc, cmd *exec.Cmd) (*resolver, error) {
	cmd.Stderr = io.Discard
	in, err := cmd.StdinPipe()
	if err != nil {
		cancel()
		return nil, errors.New("runtime_missing")
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		in.Close()
		return nil, errors.New("runtime_missing")
	}
	if cmd.Start() != nil {
		cancel()
		in.Close()
		out.Close()
		return nil, errors.New("runtime_missing")
	}
	r := &resolver{pending: make(map[uint64]chan workerReply), writes: make(chan workerRequest, 64), done: make(chan struct{}), cancel: cancel}
	writerDone := make(chan struct{})
	go func() {
		defer close(writerDone)
		encoder := json.NewEncoder(in)
		for {
			select {
			case <-life.Done():
				return
			case request := <-r.writes:
				if encoder.Encode(request) != nil {
					cancel()
					return
				}
			}
		}
	}()
	go func() {
		scanner := bufio.NewScanner(out)
		scanner.Buffer(make([]byte, 4096), resolverReplyLimit)
		for scanner.Scan() {
			var reply workerReply
			if json.Unmarshal(scanner.Bytes(), &reply) != nil || reply.ID == 0 {
				break
			}
			r.mu.Lock()
			pending := r.pending[reply.ID]
			delete(r.pending, reply.ID)
			if pending != nil {
				pending <- reply
			}
			r.mu.Unlock()
			if pending == nil {
				r.releaseReply(reply)
			}
		}
		cancel()
		in.Close()
		out.Close()
		_ = cmd.Wait()
		<-writerDone
		close(r.done)
	}()
	return r, nil
}

func (r *resolver) nextID() uint64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sequence++
	return r.sequence
}

// Control writes are small and bounded. A wedged worker is terminated rather
// than retaining an unbounded number of capabilities or goroutines.
func (r *resolver) control(request workerRequest) {
	request.ID = r.nextID()
	select {
	case <-r.done:
	case r.writes <- request:
	default:
		r.cancel()
	}
}

func (r *resolver) releaseReply(reply workerReply) {
	var result struct {
		URL string `json:"url"`
	}
	if json.Unmarshal(reply.Media, &result) == nil && validMediaURL(result.URL) {
		r.control(workerRequest{Op: "release", URL: result.URL})
	}
}

func (r *resolver) call(parent context.Context, request workerRequest) (workerReply, error) {
	ctx, cancel := context.WithTimeout(parent, 45*time.Second)
	defer cancel()
	if ctx.Err() != nil {
		return workerReply{}, ctx.Err()
	}
	request.ID = r.nextID()
	response := make(chan workerReply, 1)
	r.mu.Lock()
	if len(r.pending) >= 16 {
		r.mu.Unlock()
		return workerReply{}, errors.New("resolver_busy")
	}
	r.pending[request.ID] = response
	r.mu.Unlock()
	defer func() { r.mu.Lock(); delete(r.pending, request.ID); r.mu.Unlock() }()
	select {
	case r.writes <- request:
	case <-ctx.Done():
		return workerReply{}, ctx.Err()
	case <-r.done:
		return workerReply{}, errors.New("stream_resolution_failed")
	}
	select {
	case reply := <-response:
		if !reply.OK {
			return workerReply{}, resolverStageError(reply.Category, reply.Stage)
		}
		return reply, nil
	case <-ctx.Done():
		r.mu.Lock()
		delete(r.pending, request.ID)
		select {
		case reply := <-response:
			r.mu.Unlock()
			r.releaseReply(reply)
		default:
			r.mu.Unlock()
		}
		r.control(workerRequest{Op: "cancel", Target: request.ID})
		return workerReply{}, ctx.Err()
	case <-r.done:
		return workerReply{}, errors.New("stream_resolution_failed")
	}
}

func (r *resolver) Warm(ctx context.Context) error {
	_, err := r.call(ctx, workerRequest{Op: "warm"})
	return err
}

func (r *resolver) Configure(ctx context.Context, session AccountSession) error {
	if !session.valid() {
		return errors.New("invalid_account_session")
	}
	_, err := r.call(ctx, workerRequest{Op: "configure", Session: &session})
	return err
}

func (r *resolver) Resolve(ctx context.Context, id string) (media, error) {
	if !videoIDPattern.MatchString(id) {
		return media{}, errors.New("invalid_video_id")
	}
	reply, err := r.call(ctx, workerRequest{Op: "resolve", VideoID: id, Kind: "audio"})
	if err != nil {
		return media{}, err
	}
	info, err := parseMedia(reply.Media)
	if err != nil {
		r.releaseReply(reply)
		return media{}, err
	}
	r.lease(&info)
	return info, nil
}

func (r *resolver) ResolveVideo(ctx context.Context, id string) (videoMedia, error) {
	if !videoIDPattern.MatchString(id) {
		return videoMedia{}, errors.New("invalid_video_id")
	}
	reply, err := r.call(ctx, workerRequest{Op: "resolve", VideoID: id, Kind: "video"})
	if err != nil {
		return videoMedia{}, err
	}
	info, err := parseVideoMedia(reply.Media)
	if err != nil {
		r.releaseReply(reply)
		return videoMedia{}, err
	}
	r.lease(&info.media)
	return info, nil
}

func (r *resolver) lease(info *media) {
	var once sync.Once
	url := info.URL
	info.release = func() { once.Do(func() { r.control(workerRequest{Op: "release", URL: url}) }) }
}

func (r *resolver) Close() {
	r.closeOnce.Do(func() { r.cancel(); <-r.done })
}

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
		"init_failed", "resolver_response_limit", "cache_limit":
		return errors.New(category)
	default:
		return errors.New("stream_resolution_failed")
	}
}

type resolutionFailure struct {
	err   error
	stage string
}

func (e *resolutionFailure) Error() string { return e.err.Error() }
func (e *resolutionFailure) Unwrap() error { return e.err }

func resolverStageError(category, stage string) error {
	err := resolverError(category)
	switch stage {
	case "bootstrap", "sdk_create", "account_validation", "proof_config_request", "proof_config_parse", "proof_init", "player_request", "decipher", "proof_mint":
		return &resolutionFailure{err: err, stage: stage}
	default:
		return err
	}
}

func safeErrorStage(err error) string {
	var failure *resolutionFailure
	if errors.As(err, &failure) {
		return failure.stage
	}
	return ""
}
