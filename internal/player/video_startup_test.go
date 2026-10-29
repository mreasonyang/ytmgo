package player

import (
	"bytes"
	"context"
	"io"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"
)

// pacedVideoStream models a decoder that stays alive while its first output
// arrives late, and then delivers frames at approximately playback speed.
// Closing it interrupts both the initial delay and any later blocked read.
type pacedVideoStream struct {
	firstDelay time.Duration
	frameDelay time.Duration
	laterDelay time.Duration
	delayAfter int64
	blockAfter int64
	reads      atomic.Int64
	closed     atomic.Bool
	done       chan struct{}
	once       sync.Once
}

func (s *pacedVideoStream) Read(dst []byte) (int, error) {
	index := s.reads.Add(1) - 1
	delay := s.frameDelay
	if index == 0 {
		delay = s.firstDelay
	} else if s.laterDelay > 0 && index >= s.delayAfter {
		delay = s.laterDelay
	}
	if s.blockAfter >= 0 && index >= s.blockAfter {
		<-s.done
		return 0, io.ErrClosedPipe
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-s.done:
		return 0, io.ErrClosedPipe
	case <-timer.C:
	}
	for i := range dst {
		dst[i] = 127
	}
	return len(dst), nil
}

func (s *pacedVideoStream) Close() error {
	s.once.Do(func() { s.closed.Store(true); close(s.done) })
	return nil
}

func pacedVideoDecoder(first, interval time.Duration, blockAfter int64) (*videoDecoder, *pacedVideoStream) {
	s := &pacedVideoStream{firstDelay: first, frameDelay: interval, blockAfter: blockAfter, done: make(chan struct{})}
	wait := make(chan error, 1)
	var once sync.Once
	cancel := func() { once.Do(func() { s.Close(); wait <- nil; close(wait) }) }
	return &videoDecoder{reader: s, cancel: cancel, wait: wait}, s
}

func TestVideoDelayedFirstFrameRejoinsMovingClock(t *testing.T) {
	v := testVideo()
	defer v.Close()
	var opens atomic.Int64
	var first *pacedVideoStream
	v.open = func(_ context.Context, _ videoMedia, _ time.Duration) (*videoDecoder, error) {
		if opens.Add(1) == 1 {
			d, s := pacedVideoDecoder(900*time.Millisecond, videoFrameInterval, -1)
			first = s
			return d, nil
		}
		d, _ := pacedVideoDecoder(0, 0, -1)
		return d, nil
	}
	start := time.Now()
	if err := v.Start(context.Background(), "aaaaaaaaaaa", func() time.Duration { return 20*time.Second + time.Since(start) }, func() bool { return true }); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) {
		if frame, _ := v.Frame(); frame != nil {
			if opens.Load() != 2 || !first.closed.Load() || v.Loading() || v.Error() != "" {
				t.Fatal("recovery did not replace the stale decoder cleanly")
			}
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("late first frame never caught the advancing audio clock")
}

func fastVideoPolicy() videoPlaybackPolicy {
	return videoPlaybackPolicy{
		startupTimeout: 600 * time.Millisecond,
		stallTimeout:   150 * time.Millisecond,
		catchupLag:     100 * time.Millisecond,
		catchupGrace:   60 * time.Millisecond,
		healthyPeriod:  100 * time.Millisecond,
		maxCatchups:    2,
	}
}

func TestVideoStartupDeadlineIncludesResolution(t *testing.T) {
	v := testVideo()
	defer v.Close()
	v.policy = fastVideoPolicy()
	v.policy.startupTimeout = 80 * time.Millisecond
	var canceled atomic.Bool
	v.resolve = func(ctx context.Context, _ string) (videoMedia, error) {
		<-ctx.Done()
		canceled.Store(true)
		return videoMedia{}, ctx.Err()
	}
	if err := v.Start(context.Background(), "aaaaaaaaaaa", func() time.Duration { return 0 }, func() bool { return true }); err != nil {
		t.Fatal(err)
	}
	waitVideo(t, func() bool { return !v.Loading() })
	if v.Error() != "video_network_timeout" || !canceled.Load() {
		t.Fatal("resolution escaped the whole-startup deadline")
	}
}

func TestVideoNoFirstFrameTimesOutAndReapsDecoder(t *testing.T) {
	v := testVideo()
	defer v.Close()
	v.policy = fastVideoPolicy()
	v.policy.startupTimeout = 80 * time.Millisecond
	var opens atomic.Int64
	var stream *pacedVideoStream
	v.open = func(context.Context, videoMedia, time.Duration) (*videoDecoder, error) {
		opens.Add(1)
		d, s := pacedVideoDecoder(0, 0, 0)
		stream = s
		return d, nil
	}
	if err := v.Start(context.Background(), "aaaaaaaaaaa", func() time.Duration { return 0 }, func() bool { return false }); err != nil {
		t.Fatal(err)
	}
	waitVideo(t, func() bool { return !v.Loading() })
	if v.Error() != "video_network_timeout" || opens.Load() != 1 || !stream.closed.Load() {
		t.Fatal("a decoder with no first frame did not exit once and release its pipe")
	}
}

func TestVideoStartupDeadlineIncludesDecoderOpen(t *testing.T) {
	v := testVideo()
	defer v.Close()
	v.policy = fastVideoPolicy()
	v.policy.startupTimeout = 80 * time.Millisecond
	var canceled atomic.Bool
	v.open = func(ctx context.Context, _ videoMedia, _ time.Duration) (*videoDecoder, error) {
		<-ctx.Done()
		canceled.Store(true)
		return nil, ctx.Err()
	}
	if err := v.Start(context.Background(), "aaaaaaaaaaa", func() time.Duration { return 0 }, func() bool { return true }); err != nil {
		t.Fatal(err)
	}
	waitVideo(t, func() bool { return !v.Loading() })
	if v.Error() != "video_network_timeout" || !canceled.Load() {
		t.Fatal("decoder setup escaped the whole-startup deadline")
	}
}

func TestVideoPersistentSlowFramesHaveFiniteCatchups(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		v := testVideo()
		defer v.Close()
		v.policy = fastVideoPolicy()
		v.policy.startupTimeout = 2 * time.Second
		// Exercise the catch-up budget without a competing presentation-stall deadline.
		v.policy.stallTimeout = 2 * time.Second
		v.policy.healthyPeriod = 5 * time.Second
		var opens atomic.Int64
		var mu sync.Mutex
		var streams []*pacedVideoStream
		v.open = func(context.Context, videoMedia, time.Duration) (*videoDecoder, error) {
			opens.Add(1)
			d, s := pacedVideoDecoder(120*time.Millisecond, 200*time.Millisecond, -1)
			mu.Lock()
			streams = append(streams, s)
			mu.Unlock()
			return d, nil
		}
		start := time.Now()
		if err := v.Start(context.Background(), "aaaaaaaaaaa", func() time.Duration { return 20*time.Second + time.Since(start) }, func() bool { return true }); err != nil {
			t.Fatal(err)
		}
		deadline := time.Now().Add(4 * time.Second)
		for v.Error() == "" && time.Now().Before(deadline) {
			time.Sleep(5 * time.Millisecond)
		}
		if v.Error() != "video_network_timeout" || opens.Load() != 3 {
			t.Fatalf("slow stream did not exhaust exactly two catchups: opens=%d", opens.Load())
		}
		mu.Lock()
		defer mu.Unlock()
		for _, stream := range streams {
			if !stream.closed.Load() {
				t.Fatal("catchup left an old decoder active")
			}
		}
	})
}

func TestVideoCatchupsDoNotExtendWholeStartupDeadline(t *testing.T) {
	v := testVideo()
	defer v.Close()
	v.policy = fastVideoPolicy()
	v.policy.startupTimeout = 310 * time.Millisecond
	var opens atomic.Int64
	v.open = func(context.Context, videoMedia, time.Duration) (*videoDecoder, error) {
		opens.Add(1)
		d, _ := pacedVideoDecoder(120*time.Millisecond, 55*time.Millisecond, -1)
		return d, nil
	}
	start := time.Now()
	if err := v.Start(context.Background(), "aaaaaaaaaaa", func() time.Duration { return 20*time.Second + time.Since(start) }, func() bool { return true }); err != nil {
		t.Fatal(err)
	}
	waitVideo(t, func() bool { return !v.Loading() })
	if v.Error() != "video_network_timeout" || opens.Load() != 2 || time.Since(start) > 550*time.Millisecond {
		t.Fatalf("catchup extended startup deadline: opens=%d elapsed=%v", opens.Load(), time.Since(start))
	}
}

func TestVideoHealthyPlayingSurvivesStartupDeadline(t *testing.T) {
	v := testVideo()
	defer v.Close()
	v.policy = fastVideoPolicy()
	v.policy.startupTimeout = 80 * time.Millisecond
	var opens atomic.Int64
	v.open = func(context.Context, videoMedia, time.Duration) (*videoDecoder, error) {
		opens.Add(1)
		d, _ := pacedVideoDecoder(0, 0, -1)
		return d, nil
	}
	start := time.Now()
	if err := v.Start(context.Background(), "aaaaaaaaaaa", func() time.Duration { return 20*time.Second + time.Since(start) }, func() bool { return true }); err != nil {
		t.Fatal(err)
	}
	waitVideo(t, func() bool { frame, _ := v.Frame(); return frame != nil })
	_, before := v.Frame()
	time.Sleep(300 * time.Millisecond)
	_, after := v.Frame()
	if v.Error() != "" || v.Loading() || opens.Load() != 1 || after <= before {
		t.Fatal("healthy playback was restarted or canceled after startup")
	}
}

func TestVideoPausedFrameDoesNotStallAndResumeIsBounded(t *testing.T) {
	v := testVideo()
	defer v.Close()
	v.policy = fastVideoPolicy()
	v.policy.startupTimeout = 80 * time.Millisecond
	var playing atomic.Bool
	var stream *pacedVideoStream
	v.open = func(context.Context, videoMedia, time.Duration) (*videoDecoder, error) {
		d, s := pacedVideoDecoder(0, 0, 1)
		stream = s
		return d, nil
	}
	start := time.Now()
	if err := v.Start(context.Background(), "aaaaaaaaaaa", func() time.Duration {
		if playing.Load() {
			return 20*time.Second + time.Since(start)
		}
		return 20 * time.Second
	}, playing.Load); err != nil {
		t.Fatal(err)
	}
	waitVideo(t, func() bool { frame, _ := v.Frame(); return frame != nil })
	time.Sleep(300 * time.Millisecond)
	if v.Error() != "" || stream.closed.Load() {
		t.Fatal("intentional pause was mistaken for decoder stall")
	}
	playing.Store(true)
	waitVideo(t, func() bool { return v.Error() != "" })
	if v.Error() != "video_network_timeout" || !stream.closed.Load() {
		t.Fatal("resumed stream without more frames did not time out and release")
	}
}

func TestVideoStartupCancelAndReplacementReleaseBlockedDecoder(t *testing.T) {
	for _, action := range []string{"cancel", "replace", "stop"} {
		t.Run(action, func(t *testing.T) {
			v := testVideo()
			defer v.Close()
			v.policy = fastVideoPolicy()
			var opens atomic.Int64
			var first *pacedVideoStream
			started := make(chan struct{})
			v.open = func(context.Context, videoMedia, time.Duration) (*videoDecoder, error) {
				if opens.Add(1) == 1 {
					d, s := pacedVideoDecoder(0, 0, 0)
					first = s
					close(started)
					return d, nil
				}
				d, _ := pacedVideoDecoder(0, 0, -1)
				return d, nil
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if err := v.Start(ctx, "aaaaaaaaaaa", func() time.Duration { return 0 }, func() bool { return false }); err != nil {
				t.Fatal(err)
			}
			<-started
			switch action {
			case "cancel":
				cancel()
			case "stop":
				v.Stop()
			case "replace":
				if err := v.Start(context.Background(), "bbbbbbbbbbb", func() time.Duration { return 0 }, func() bool { return false }); err != nil {
					t.Fatal(err)
				}
				waitVideo(t, func() bool { frame, _ := v.Frame(); return frame != nil })
			}
			waitVideo(t, func() bool { return first.closed.Load() && !v.Loading() })
			if v.Error() != "" {
				t.Fatal("user cancellation or replacement was reported as a timeout")
			}
		})
	}
}

func TestVideoBriefBufferingCatchesUpWithoutReopen(t *testing.T) {
	v := testVideo()
	defer v.Close()
	v.policy = fastVideoPolicy()
	var opens atomic.Int64
	v.open = func(context.Context, videoMedia, time.Duration) (*videoDecoder, error) {
		opens.Add(1)
		d, _ := pacedVideoDecoder(120*time.Millisecond, 0, -1)
		return d, nil
	}
	start := time.Now()
	if err := v.Start(context.Background(), "aaaaaaaaaaa", func() time.Duration { return 20*time.Second + time.Since(start) }, func() bool { return true }); err != nil {
		t.Fatal(err)
	}
	waitVideo(t, func() bool { frame, _ := v.Frame(); return frame != nil })
	if opens.Load() != 1 || v.Error() != "" || v.Loading() {
		t.Fatal("a transient buffer delay unnecessarily reopened the decoder")
	}
}

func TestVideoHealthyPeriodRenewsCatchupBudget(t *testing.T) {
	v := testVideo()
	defer v.Close()
	v.policy = fastVideoPolicy()
	v.policy.stallTimeout = 500 * time.Millisecond
	v.policy.maxCatchups = 1
	var opens atomic.Int64
	v.open = func(context.Context, videoMedia, time.Duration) (*videoDecoder, error) {
		switch opens.Add(1) {
		case 1:
			d, _ := pacedVideoDecoder(120*time.Millisecond, 55*time.Millisecond, -1)
			return d, nil
		case 2:
			d, s := pacedVideoDecoder(0, 0, -1)
			// Deliver healthy frames long enough to end the first recovery
			// episode, then emulate a later independent slowdown.
			s.delayAfter, s.laterDelay = 8, 120*time.Millisecond
			return d, nil
		default:
			d, _ := pacedVideoDecoder(0, 0, -1)
			return d, nil
		}
	}
	start := time.Now()
	if err := v.Start(context.Background(), "aaaaaaaaaaa", func() time.Duration { return 20*time.Second + time.Since(start) }, func() bool { return true }); err != nil {
		t.Fatal(err)
	}
	waitVideo(t, func() bool { return opens.Load() >= 3 || v.Error() != "" })
	if v.Error() != "" || opens.Load() != 3 {
		t.Fatalf("a later slowdown inherited an exhausted lifetime budget: opens=%d", opens.Load())
	}
	_, before := v.Frame()
	waitVideo(t, func() bool { _, after := v.Frame(); return after > before })
}

func TestVideoEOFDoesNotWaitForeverForDecoderExit(t *testing.T) {
	v := testVideo()
	defer v.Close()
	v.policy = fastVideoPolicy()
	var canceled atomic.Bool
	v.open = func(context.Context, videoMedia, time.Duration) (*videoDecoder, error) {
		done := make(chan error, 1)
		var once sync.Once
		return &videoDecoder{
			reader: io.NopCloser(bytes.NewReader(make([]byte, 4*2*4))),
			wait:   done,
			cancel: func() { once.Do(func() { canceled.Store(true); done <- nil; close(done) }) },
		}, nil
	}
	if err := v.Start(context.Background(), "aaaaaaaaaaa", func() time.Duration { return 0 }, func() bool { return false }); err != nil {
		t.Fatal(err)
	}
	waitVideo(t, func() bool { return v.Error() != "" })
	if v.Error() != "video_network_timeout" || !canceled.Load() {
		t.Fatal("closed decoder output left an unbounded process wait")
	}
}

func TestVideoRepeatedStartupDelayUsesLeadWithoutEarlyPresentation(t *testing.T) {
	v := testVideo()
	defer v.Close()
	v.policy = fastVideoPolicy()
	v.policy.startupTimeout = time.Second
	v.policy.stallTimeout = 500 * time.Millisecond
	var opens atomic.Int64
	var recoveryTarget atomic.Int64
	v.open = func(_ context.Context, _ videoMedia, at time.Duration) (*videoDecoder, error) {
		if opens.Add(1) == 2 {
			recoveryTarget.Store(int64(at))
		}
		d, _ := pacedVideoDecoder(120*time.Millisecond, videoFrameInterval, -1)
		return d, nil
	}
	start := time.Now()
	position := func() time.Duration { return 20*time.Second + time.Since(start) }
	if err := v.Start(context.Background(), "aaaaaaaaaaa", position, func() bool { return true }); err != nil {
		t.Fatal(err)
	}
	waitVideo(t, func() bool { frame, _ := v.Frame(); return frame != nil || v.Error() != "" })
	if frame, _ := v.Frame(); frame == nil || v.Error() != "" || opens.Load() != 2 {
		t.Fatalf("reopening at the current clock never compensated decoder startup: opens=%d", opens.Load())
	}
	if position()+videoFrameInterval/2 < time.Duration(recoveryTarget.Load()) {
		t.Fatal("recovery displayed a future frame before the audio clock reached it")
	}
}

func TestVideoPauseDuringLeadRecoveryReopensAtPausedClock(t *testing.T) {
	for _, mode := range []string{"during_open", "waiting_for_future_frame"} {
		t.Run(mode, func(t *testing.T) {
			v := testVideo()
			defer v.Close()
			v.policy = fastVideoPolicy()
			v.policy.startupTimeout = 2 * time.Second
			v.policy.stallTimeout = 500 * time.Millisecond
			var opens atomic.Int64
			var paused atomic.Bool
			var frozen, pausedTarget atomic.Int64
			secondOpened, releaseOpen := make(chan struct{}), make(chan struct{})
			v.open = func(ctx context.Context, _ videoMedia, at time.Duration) (*videoDecoder, error) {
				index := opens.Add(1)
				if index == 2 {
					close(secondOpened)
					if mode == "during_open" {
						select {
						case <-releaseOpen:
						case <-ctx.Done():
							return nil, ctx.Err()
						}
					}
					d, _ := pacedVideoDecoder(0, videoFrameInterval, -1)
					return d, nil
				}
				if index == 3 {
					pausedTarget.Store(int64(at))
				}
				d, _ := pacedVideoDecoder(120*time.Millisecond, videoFrameInterval, -1)
				return d, nil
			}
			start := time.Now()
			position := func() time.Duration {
				if paused.Load() {
					return time.Duration(frozen.Load())
				}
				return 20*time.Second + time.Since(start)
			}
			if err := v.Start(context.Background(), "aaaaaaaaaaa", position, func() bool { return !paused.Load() }); err != nil {
				t.Fatal(err)
			}
			select {
			case <-secondOpened:
			case <-time.After(time.Second):
				t.Fatal("lead recovery was not opened")
			}
			if mode == "waiting_for_future_frame" {
				time.Sleep(20 * time.Millisecond)
				if frame, _ := v.Frame(); frame != nil {
					t.Fatal("future recovery frame appeared before its audio position")
				}
			}
			frozen.Store(int64(position()))
			paused.Store(true)
			close(releaseOpen)
			waitVideo(t, func() bool { frame, _ := v.Frame(); return frame != nil || v.Error() != "" })
			if frame, _ := v.Frame(); frame == nil || v.Error() != "" || opens.Load() != 3 || pausedTarget.Load() != frozen.Load() {
				t.Fatalf("pause retained a future target: opens=%d", opens.Load())
			}
		})
	}
}

func TestVideoSeekDuringLeadOpenDropsRecoveryLead(t *testing.T) {
	v := testVideo()
	defer v.Close()
	v.policy = fastVideoPolicy()
	v.policy.startupTimeout = 2 * time.Second
	v.policy.stallTimeout = 500 * time.Millisecond
	var opens, jump, finalLead atomic.Int64
	secondOpened, releaseOpen := make(chan struct{}), make(chan struct{})
	start := time.Now()
	position := func() time.Duration { return 20*time.Second + time.Since(start) + time.Duration(jump.Load()) }
	v.open = func(ctx context.Context, _ videoMedia, at time.Duration) (*videoDecoder, error) {
		switch opens.Add(1) {
		case 1:
			d, _ := pacedVideoDecoder(120*time.Millisecond, videoFrameInterval, -1)
			return d, nil
		case 2:
			close(secondOpened)
			select {
			case <-releaseOpen:
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		default:
			finalLead.Store(int64(at - position()))
		}
		d, _ := pacedVideoDecoder(0, 0, -1)
		return d, nil
	}
	if err := v.Start(context.Background(), "aaaaaaaaaaa", position, func() bool { return true }); err != nil {
		t.Fatal(err)
	}
	select {
	case <-secondOpened:
	case <-time.After(time.Second):
		t.Fatal("lead recovery was not opened")
	}
	jump.Store(int64(10 * time.Second))
	close(releaseOpen)
	waitVideo(t, func() bool { frame, _ := v.Frame(); return frame != nil || v.Error() != "" })
	if frame, _ := v.Frame(); frame == nil || v.Error() != "" || opens.Load() != 3 || time.Duration(finalLead.Load()) > videoFrameInterval/2 {
		t.Fatalf("explicit seek kept the stale lead/target: opens=%d lead=%v", opens.Load(), time.Duration(finalLead.Load()))
	}
}

func TestVideoLeadRecoveryClampsToFinalFrame(t *testing.T) {
	v := testVideo()
	defer v.Close()
	v.policy = fastVideoPolicy()
	v.policy.startupTimeout = time.Second
	v.policy.stallTimeout = 500 * time.Millisecond
	const duration = 2 * time.Second
	v.resolve = func(context.Context, string) (videoMedia, error) {
		return videoMedia{media: media{Duration: duration}, width: 4, height: 2}, nil
	}
	var opens, target atomic.Int64
	v.open = func(_ context.Context, _ videoMedia, at time.Duration) (*videoDecoder, error) {
		if opens.Add(1) == 2 {
			target.Store(int64(at))
		}
		d, _ := pacedVideoDecoder(120*time.Millisecond, videoFrameInterval, -1)
		return d, nil
	}
	start := time.Now()
	position := func() time.Duration { return min(1650*time.Millisecond+time.Since(start), duration-videoFrameInterval) }
	if err := v.Start(context.Background(), "aaaaaaaaaaa", position, func() bool { return true }); err != nil {
		t.Fatal(err)
	}
	waitVideo(t, func() bool { frame, _ := v.Frame(); return frame != nil || v.Error() != "" })
	if frame, _ := v.Frame(); frame == nil || v.Error() != "" || opens.Load() != 2 || time.Duration(target.Load()) != duration-videoFrameInterval {
		t.Fatalf("tail recovery was not clamped to a decodable timestamp: opens=%d target=%v", opens.Load(), time.Duration(target.Load()))
	}
}

func TestVideoRecoveryLeadIsBounded(t *testing.T) {
	v := testVideo()
	defer v.Close()
	v.policy = fastVideoPolicy()
	v.policy.startupTimeout = 1500 * time.Millisecond
	v.policy.stallTimeout = 500 * time.Millisecond
	v.policy.maximumLead = 120 * time.Millisecond
	var opens, maximumLead atomic.Int64
	start := time.Now()
	position := func() time.Duration { return 20*time.Second + time.Since(start) }
	v.open = func(_ context.Context, _ videoMedia, at time.Duration) (*videoDecoder, error) {
		opens.Add(1)
		lead := int64(at - position())
		if lead > maximumLead.Load() {
			maximumLead.Store(lead)
		}
		d, _ := pacedVideoDecoder(250*time.Millisecond, videoFrameInterval, -1)
		return d, nil
	}
	if err := v.Start(context.Background(), "aaaaaaaaaaa", position, func() bool { return true }); err != nil {
		t.Fatal(err)
	}
	waitVideo(t, func() bool { return v.Error() != "" })
	if v.Error() != "video_network_timeout" || opens.Load() != 3 || maximumLead.Load() > int64(v.policy.maximumLead) || maximumLead.Load() < int64(100*time.Millisecond) {
		t.Fatalf("lead escaped its cap or retry budget: opens=%d lead=%v", opens.Load(), time.Duration(maximumLead.Load()))
	}
}

func TestVideoWaitingForFutureFrameKeepsWholeStartupDeadline(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		v := testVideo()
		defer v.Close()
		v.policy = fastVideoPolicy()
		v.policy.startupTimeout = 350 * time.Millisecond
		v.policy.stallTimeout = 500 * time.Millisecond
		var opens atomic.Int64
		v.open = func(context.Context, videoMedia, time.Duration) (*videoDecoder, error) {
			if opens.Add(1) == 1 {
				d, _ := pacedVideoDecoder(120*time.Millisecond, videoFrameInterval, -1)
				return d, nil
			}
			d, _ := pacedVideoDecoder(0, 0, -1)
			return d, nil
		}
		start := time.Now()
		if err := v.Start(context.Background(), "aaaaaaaaaaa", func() time.Duration { return 20*time.Second + time.Since(start) }, func() bool { return true }); err != nil {
			t.Fatal(err)
		}
		waitVideo(t, func() bool { return v.Error() != "" })
		if frame, _ := v.Frame(); frame != nil || v.Error() != "video_network_timeout" || opens.Load() != 2 || time.Since(start) > 600*time.Millisecond {
			t.Fatalf("future-frame waiting escaped the first-frame deadline: opens=%d", opens.Load())
		}
	})
}
