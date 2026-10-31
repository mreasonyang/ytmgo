package player

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"io"
	"math"
	"os"
	"strings"
	"sync"
	"time"
)

const videoFrameInterval = time.Second / 24

var errVideoReadiness = errors.New("audio_network_timeout")

type videoCatchUpError struct{ lag time.Duration }

func (*videoCatchUpError) Error() string { return "video_catch_up" }

// A video session must either present a frame or leave loading within a
// bounded time. Slow but progressing decoding may need a fresh audio position;
// retries are limited until several seconds of healthy presentation recover.
type videoPlaybackPolicy struct {
	startupTimeout time.Duration
	stallTimeout   time.Duration
	catchupLag     time.Duration
	catchupGrace   time.Duration
	maximumLead    time.Duration
	healthyPeriod  time.Duration
	maxCatchups    int
}

func (p videoPlaybackPolicy) defaults() videoPlaybackPolicy {
	if p.startupTimeout <= 0 {
		p.startupTimeout = 20 * time.Second
	}
	if p.stallTimeout <= 0 {
		p.stallTimeout = 10 * time.Second
	}
	if p.catchupLag <= 0 {
		p.catchupLag = 750 * time.Millisecond
	}
	if p.catchupGrace <= 0 {
		p.catchupGrace = 1500 * time.Millisecond
	}
	if p.maximumLead <= 0 {
		p.maximumLead = 8 * time.Second
	}
	p.maximumLead = min(p.maximumLead, p.stallTimeout)
	if p.healthyPeriod <= 0 {
		p.healthyPeriod = 5 * time.Second
	}
	if p.maxCatchups <= 0 {
		p.maxCatchups = 2
	}
	return p
}

type videoMedia struct {
	media
	width, height int
}

// Video is an on-demand native video surface synchronized to the audio player's
// clock. It owns no audio output and never opens a WebView. Images returned by
// Frame are immutable; only the current frame and bounded decoding buffers stay
// resident. Stop and Close reap every helper before returning.
type Video struct {
	mu                   sync.RWMutex
	lifecycle            sync.Mutex
	ops                  sync.WaitGroup
	resolve              func(context.Context, string) (videoMedia, error)
	open                 func(context.Context, videoMedia, time.Duration) (*videoDecoder, error)
	cancel               context.CancelFunc
	closed, loading      bool
	generation, sequence uint64
	frame                image.Image
	error                string
	policy               videoPlaybackPolicy
}

func NewVideo(p *Player) (*Video, error) {
	runtime, err := findRuntime()
	if err != nil {
		return nil, err
	}
	if p == nil || p.resolver == nil {
		return nil, errors.New("runtime_missing")
	}
	return &Video{resolve: p.resolveVideo, open: runtime.openVideo}, nil
}

// Start begins asynchronous resolution and decoding. The callbacks must be safe
// to call from a worker and return the current audio position and playing state.
// The supplied context owns this video session until Stop, Close, or replacement.
func (v *Video) Start(ctx context.Context, id string, position func() time.Duration, playing func() bool) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !videoIDPattern.MatchString(id) {
		return errors.New("invalid_video_id")
	}
	if position == nil || playing == nil {
		return errors.New("video_clock_missing")
	}
	v.lifecycle.Lock()
	defer v.lifecycle.Unlock()
	v.mu.Lock()
	if v.closed {
		v.mu.Unlock()
		return ErrClosed
	}
	previous := v.cancel
	v.generation++
	generation := v.generation
	life, cancel := context.WithCancel(ctx)
	v.cancel, v.loading, v.error, v.frame = cancel, true, "", nil
	v.sequence++
	v.mu.Unlock()
	if previous != nil {
		previous()
	}
	v.ops.Wait()
	v.ops.Add(1)
	go func() {
		defer v.ops.Done()
		v.run(life, generation, id, position, playing)
	}()
	return nil
}

func (v *Video) Frame() (image.Image, uint64) {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.frame, v.sequence
}

func (v *Video) Error() string { v.mu.RLock(); defer v.mu.RUnlock(); return v.error }
func (v *Video) Loading() bool { v.mu.RLock(); defer v.mu.RUnlock(); return v.loading }

func (v *Video) Stop()  { v.stop(false) }
func (v *Video) Close() { v.stop(true) }

func (v *Video) stop(close bool) {
	v.lifecycle.Lock()
	defer v.lifecycle.Unlock()
	v.mu.Lock()
	if close {
		v.closed = true
	}
	v.generation++
	cancel := v.cancel
	v.cancel, v.frame, v.loading, v.error = nil, nil, false, ""
	v.sequence++
	v.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	v.ops.Wait()
}

func (v *Video) run(ctx context.Context, generation uint64, id string, position func() time.Duration, playing func() bool) {
	defer func() {
		v.mu.Lock()
		if generation == v.generation {
			v.loading = false
		}
		v.mu.Unlock()
	}()
	policy := v.policy.defaults()
	life, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)
	// This timer includes resolution and every catch-up attempt until the very
	// first displayed frame. Stopping the timer preserves the session context:
	// a successful video must not be canceled when its startup budget expires.
	startup := time.AfterFunc(policy.startupTimeout, func() { cancel(errVideoReadiness) })
	defer startup.Stop()
	info, err := v.resolve(life, id)
	if err != nil {
		if ctx.Err() == nil {
			if errors.Is(context.Cause(life), errVideoReadiness) {
				v.fail(generation, videoErrorCategory(errVideoReadiness))
			} else {
				v.fail(generation, "video_resolution_failed")
			}
		}
		return
	}
	if info.release != nil {
		defer info.release()
	}
	catchups := 0
	var lead time.Duration
	displayed := false
	var healthySince, lastFrame time.Time
	for life.Err() == nil {
		clock, isPlaying := position(), playing()
		if !isPlaying {
			lead = 0
		}
		at := clampVideoPosition(clock+lead, info.Duration)
		lead = max(0, at-clock)
		target := videoSyncTarget{at: at, clock: clock, lead: lead, began: time.Now(), playing: isPlaying}
		attempt, stopAttempt := context.WithCancelCause(life)
		firstFrameBudget := policy.startupTimeout
		if displayed {
			firstFrameBudget = policy.stallTimeout
		}
		ready := time.AfterFunc(firstFrameBudget, func() { stopAttempt(errVideoReadiness) })
		decoder, err := v.open(attempt, info, at)
		restart := false
		if err == nil {
			restart, err = v.present(attempt, generation, decoder, info, target, position, playing, func() {
				startup.Stop()
				ready.Stop()
				displayed = true
				now := time.Now()
				if !playing() || (!lastFrame.IsZero() && now.Sub(lastFrame) > 500*time.Millisecond) {
					healthySince = time.Time{}
				} else if healthySince.IsZero() {
					healthySince = now
				} else if now.Sub(healthySince) >= policy.healthyPeriod {
					catchups, lead = 0, 0
				}
				lastFrame = now
			})
			decoder.Close()
		}
		ready.Stop()
		cause := context.Cause(attempt)
		stopAttempt(nil)
		if ctx.Err() != nil {
			return
		}
		if errors.Is(cause, errVideoReadiness) {
			v.fail(generation, videoErrorCategory(errVideoReadiness))
			return
		}
		var catchup *videoCatchUpError
		if errors.As(err, &catchup) {
			catchups++
			if catchups > policy.maxCatchups {
				v.fail(generation, videoErrorCategory(errVideoReadiness))
				return
			}
			// Reopening at "now" repeats the startup delay against a moving
			// audio clock. Reserve the observed deficit plus two frames. A
			// second attempt adds its residual lag to the lead already used.
			lead = min(policy.maximumLead, lead+catchup.lag+2*videoFrameInterval)
			healthySince, lastFrame = time.Time{}, time.Time{}
			continue
		}
		if err != nil {
			v.fail(generation, videoErrorCategory(err))
			return
		}
		if !restart {
			return
		}
		// An explicit user seek starts a new recovery episode. Ordinary clock
		// advancement cannot reset the budget for a persistently slow stream.
		catchups, lead = 0, 0
		healthySince, lastFrame = time.Time{}, time.Time{}
	}
	if ctx.Err() == nil && errors.Is(context.Cause(life), errVideoReadiness) {
		v.fail(generation, videoErrorCategory(errVideoReadiness))
	}
}

func clampVideoPosition(position, duration time.Duration) time.Duration {
	return min(max(position, 0), max(duration-videoFrameInterval, 0))
}

// Preserve the pre-open clock so a pause or explicit seek during decoder
// startup can invalidate a future recovery target before its first frame.
type videoSyncTarget struct {
	at, clock, lead time.Duration
	began           time.Time
	playing         bool
}

type decodedFrame struct {
	image *image.RGBA
	at    time.Duration
	err   error
}

func (v *Video) present(ctx context.Context, generation uint64, decoder *videoDecoder, info videoMedia, target videoSyncTarget, position func() time.Duration, playing func() bool, presented func()) (bool, error) {
	policy := v.policy.defaults()
	life, cancel := context.WithCancel(ctx)
	defer cancel()
	frames := make(chan decodedFrame, 1)
	readerDone := make(chan struct{})
	go func() {
		defer close(readerDone)
		defer close(frames)
		for index := int64(0); ; index++ {
			frame := image.NewRGBA(image.Rect(0, 0, info.width, info.height))
			_, err := io.ReadFull(decoder.reader, frame.Pix)
			result := decodedFrame{image: frame, at: target.at + time.Duration(index)*videoFrameInterval, err: err}
			select {
			case frames <- result:
			case <-life.Done():
				return
			}
			if err != nil {
				return
			}
		}
	}()
	defer func() { cancel(); decoder.Close(); <-readerDone }()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	lastPosition, lastCheck := target.clock, target.began
	var pending *decodedFrame
	hasFrame := false
	lastPresented := time.Now()
	var lagSince time.Time
	wasPlaying := target.playing
	for {
		if err := ctx.Err(); err != nil {
			return false, err
		}
		now := time.Now()
		isPlaying := playing()
		if !isPlaying || !wasPlaying {
			lastPresented = now
			lagSince = time.Time{}
		}
		wasPlaying = isPlaying
		if hasFrame && isPlaying && now.Sub(lastPresented) >= policy.stallTimeout {
			return false, errVideoReadiness
		}
		clock := position()
		if !isPlaying && target.lead > 0 && target.at > clock+videoFrameInterval/2 {
			return true, nil
		}
		advance := clock - lastPosition
		if advance < -200*time.Millisecond || advance > now.Sub(lastCheck)+600*time.Millisecond {
			return true, nil
		}
		lastPosition, lastCheck = clock, now
		if pending != nil {
			if pending.err != nil {
				if pending.err == io.EOF && hasFrame {
					exit := time.NewTimer(policy.stallTimeout)
					defer exit.Stop()
					select {
					case err := <-decoder.wait:
						if err != nil {
							return false, decoder.streamError()
						}
						return false, nil
					case <-ctx.Done():
						return false, ctx.Err()
					case <-exit.C:
						return false, errVideoReadiness
					}
				}
				return false, decoder.streamError()
			}
			if pending.at < clock-2*videoFrameInterval {
				lagThreshold := policy.catchupLag
				if target.lead > 0 && !hasFrame {
					// A recovery startup can be slightly slower than the previous
					// one; adapt its residual lag instead of waiting for 750 ms.
					lagThreshold = min(lagThreshold, 2*videoFrameInterval)
				}
				if isPlaying && clock-pending.at >= lagThreshold {
					if lagSince.IsZero() {
						lagSince = now
					} else if now.Sub(lagSince) >= policy.catchupGrace {
						return true, &videoCatchUpError{lag: clock - pending.at}
					}
				} else {
					lagSince = time.Time{}
				}
				pending = nil
				continue
			}
			// A paused seek can still show its first accurate frame. Later frames
			// wait for the audio clock; pipe backpressure bounds decode-ahead.
			if pending.at <= clock+videoFrameInterval/2 && (isPlaying || !hasFrame) {
				v.mu.Lock()
				if generation == v.generation && !v.closed {
					v.frame, v.loading, v.error = pending.image, false, ""
					v.sequence++
				}
				v.mu.Unlock()
				hasFrame, pending = true, nil
				lastPresented, lagSince = now, time.Time{}
				presented()
				continue
			}
		}
		if pending == nil {
			select {
			case <-ctx.Done():
				return false, ctx.Err()
			case frame, ok := <-frames:
				if !ok {
					return false, nil
				}
				pending = &frame
			case <-ticker.C:
			}
		} else {
			select {
			case <-ctx.Done():
				return false, ctx.Err()
			case <-ticker.C:
			}
		}
	}
}

func (v *Video) fail(generation uint64, category string) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if generation == v.generation && !v.closed {
		v.error, v.loading = category, false
	}
}

func parseVideoMedia(data []byte) (videoMedia, error) {
	var result struct {
		URL      string            `json:"url"`
		Duration float64           `json:"duration"`
		Width    int               `json:"width"`
		Height   int               `json:"height"`
		Headers  map[string]string `json:"http_headers"`
	}
	if json.Unmarshal(data, &result) != nil || !validMediaURL(result.URL) || result.Duration <= 0 || result.Duration > 86400 || result.Width < 2 || result.Height < 2 || result.Width > 8192 || result.Height > 8192 {
		return videoMedia{}, errors.New("invalid_media_response")
	}
	agent, referer := result.Headers["User-Agent"], result.Headers["Referer"]
	if !validHeaderValue(agent) || !validHeaderValue(referer) {
		return videoMedia{}, errors.New("invalid_media_response")
	}
	width, height := videoDimensions(result.Width, result.Height)
	return videoMedia{media: media{URL: result.URL, Duration: time.Duration(result.Duration * float64(time.Second)), UserAgent: agent, Referer: referer}, width: width, height: height}, nil
}

func videoDimensions(width, height int) (int, int) {
	scale := math.Min(1, math.Min(854/float64(width), 480/float64(height)))
	return max(2, int(float64(width)*scale)/2*2), max(2, int(float64(height)*scale)/2*2)
}

type videoDecoder struct {
	reader    io.ReadCloser
	bridge    *streamBridge
	cancel    context.CancelFunc
	wait      <-chan error
	closeOnce sync.Once
}

func (r runtimePaths) openVideo(ctx context.Context, info videoMedia, at time.Duration) (*videoDecoder, error) {
	if info.source == nil && !validMediaURL(info.URL) {
		return nil, errors.New("invalid_media_response")
	}
	return startVideoDecoder(ctx, r.ffmpeg, info, at)
}

func startVideoDecoder(ctx context.Context, executable string, info videoMedia, at time.Duration) (*videoDecoder, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	life, cancel := context.WithCancel(ctx)
	bridge, err := newStreamBridge(life, info.media)
	if err != nil {
		cancel()
		return nil, err
	}
	// The worker owns remote transport. FFmpeg receives only the local relay
	// capability through stdin, never remote URLs or headers.
	// Seek the underlying media before probing rather than scanning the
	// concat input from its beginning. Preserve negative keyframe preroll
	// timestamps through decoding, then trim at output time: input-side
	// -ss 0 discards reordered H.264 frames and can lose the tail entirely.
	manifest := fmt.Sprintf("ffconcat version 1.0\nfile '%s'\ninpoint %.6f\nduration %.6f\noption rw_timeout 10000000\n", bridge.URL, at.Seconds(), max(info.Duration-at, 0).Seconds())
	// Trim before fps quantization: otherwise a slightly negative preroll
	// timestamp can round to zero and leak the preceding frame into output.
	filter := fmt.Sprintf("trim=start=0,scale=%d:%d:flags=fast_bilinear,fps=24,format=rgba", info.width, info.height)
	cmd := childCommand(life, executable, "-hide_banner", "-loglevel", "quiet", "-nostdin",
		"-threads", "1", "-copyts", "-f", "concat", "-safe", "0",
		"-protocol_whitelist", "pipe,file,http,tcp", "-i", "pipe:0",
		"-ss", "0", "-an", "-sn", "-dn", "-threads", "1", "-filter_threads", "1", "-vf", filter,
		"-c:v", "rawvideo", "-pix_fmt", "rgba", "-f", "rawvideo", "pipe:1")
	cmd.Stdin, cmd.Stderr = strings.NewReader(manifest), io.Discard
	reader, writer, err := os.Pipe()
	if err != nil {
		cancel()
		bridge.Close()
		return nil, errors.New("video_decoder_unavailable")
	}
	cmd.Stdout = writer
	if err := cmd.Start(); err != nil {
		cancel()
		bridge.Close()
		reader.Close()
		writer.Close()
		return nil, errors.New("video_decoder_unavailable")
	}
	writer.Close()
	done := make(chan error, 1)
	go func() { done <- cmd.Wait(); close(done) }()
	return &videoDecoder{reader: reader, bridge: bridge, cancel: cancel, wait: done}, nil
}

func (d *videoDecoder) Close() {
	d.closeOnce.Do(func() {
		d.cancel()
		d.reader.Close()
		if d.bridge != nil {
			d.bridge.Close()
		}
		<-d.wait
	})
}

func (d *videoDecoder) streamError() error {
	if d.bridge != nil {
		if err := d.bridge.Err(); err != nil {
			return err
		}
	}
	return errors.New("video_stream_failed")
}

func videoErrorCategory(err error) string {
	switch err.Error() {
	case "audio_access_denied", "audio_rate_limited", "audio_server_error", "audio_network_timeout", "audio_connection_failed":
		return "video_" + strings.TrimPrefix(err.Error(), "audio_")
	default:
		return "video_stream_failed"
	}
}
