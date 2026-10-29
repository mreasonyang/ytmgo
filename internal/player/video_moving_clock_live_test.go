package player

import (
	"context"
	"os"
	"testing"
	"time"
)

// Exercise presentation against a moving clock, not merely the decoder's
// first bytes. This opt-in anonymous probe never reads a stored account or
// prints media URLs, headers, image bytes, or dependency diagnostics.
func TestNativeVideoMovingClockLive(t *testing.T) {
	if os.Getenv("YTMGO_LIVE_VIDEO") != "1" {
		t.Skip("set YTMGO_LIVE_VIDEO=1 for moving-clock video network verification")
	}
	p, err := New()
	if err != nil {
		t.Fatal("video_runtime_unavailable")
	}
	defer p.Close()
	initializeAnonymousLive(t, p)
	v, err := NewVideo(p)
	if err != nil {
		t.Fatal("video_runtime_unavailable")
	}
	defer v.Close()
	for _, at := range []time.Duration{73*time.Second + 125*time.Millisecond, 117*time.Second + 625*time.Millisecond} {
		t.Run(at.String(), func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
			defer cancel()
			start := time.Now()
			if err := v.Start(ctx, "52fNscjkST4", func() time.Duration { return at + time.Since(start) }, func() bool { return true }); err != nil {
				t.Fatal("video_start_failed")
			}
			defer v.Stop()
			for v.Loading() && ctx.Err() == nil {
				time.Sleep(20 * time.Millisecond)
			}
			frame, first := v.Frame()
			if ctx.Err() != nil || v.Error() != "" || frame == nil {
				t.Fatalf("moving_clock_first_frame_failed: category=%s", v.Error())
			}
			t.Logf("moving-clock first frame at %.3fs in %.3fs", at.Seconds(), time.Since(start).Seconds())
			deadline := time.Now().Add(2 * time.Second)
			for time.Now().Before(deadline) {
				_, sequence := v.Frame()
				if sequence >= first+12 {
					return
				}
				if v.Error() != "" {
					t.Fatalf("moving_clock_video_stalled: category=%s", v.Error())
				}
				time.Sleep(20 * time.Millisecond)
			}
			t.Fatal("moving_clock_video_did_not_keep_advancing")
		})
	}
}

// Exercise the actual mid-song path: anonymous audio has already initialized
// the shared resolver and its native output supplies the video clock. Unlike
// the video-only cold-start probe above, resolution is not artificially warmed
// and the clock cannot advance independently of AudioQueue playback.
func TestNativeVideoDuringAnonymousAudioLive(t *testing.T) {
	if os.Getenv("YTMGO_LIVE_VIDEO") != "1" {
		t.Skip("set YTMGO_LIVE_VIDEO=1 for video during anonymous native audio")
	}
	p, err := New()
	if err != nil {
		t.Fatal("audio_runtime_unavailable")
	}
	defer p.Close()
	initializeAnonymousLive(t, p)
	p.SetVolume(0)
	loadContext, cancelLoad := context.WithTimeout(context.Background(), 45*time.Second)
	loadStarted := time.Now()
	err = p.Load(loadContext, "52fNscjkST4")
	cancelLoad()
	if err != nil {
		t.Fatal("anonymous_audio_load_failed")
	}
	t.Logf("anonymous_audio_load_seconds=%.3f", time.Since(loadStarted).Seconds())

	awaitAudio := func(t *testing.T, from time.Duration) {
		t.Helper()
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			state := p.Snapshot()
			if state.Error != "" || state.Ended {
				t.Fatal("anonymous_audio_playback_failed")
			}
			if state.Playing && state.Position >= from+250*time.Millisecond {
				return
			}
			time.Sleep(20 * time.Millisecond)
		}
		t.Fatal("anonymous_audio_clock_not_advancing")
	}
	awaitAudio(t, p.Snapshot().Position)
	v, err := NewVideo(p)
	if err != nil {
		t.Fatal("video_runtime_unavailable")
	}
	defer v.Close()

	for _, at := range []time.Duration{73*time.Second + 125*time.Millisecond, 117*time.Second + 625*time.Millisecond} {
		t.Run(at.String(), func(t *testing.T) {
			if err := p.Pause(); err != nil {
				t.Fatal("anonymous_audio_pause_failed")
			}
			seekContext, cancelSeek := context.WithTimeout(context.Background(), 30*time.Second)
			seekStarted := time.Now()
			err := p.SeekContext(seekContext, at)
			cancelSeek()
			if err != nil {
				t.Fatal("anonymous_audio_seek_failed")
			}
			state := p.Snapshot()
			if state.Playing || state.Error != "" || state.Position < at || state.Position > at+50*time.Millisecond {
				t.Fatal("anonymous_audio_paused_seek_inaccurate")
			}
			t.Logf("anonymous_audio_seek_target_seconds=%.3f seek_seconds=%.3f", at.Seconds(), time.Since(seekStarted).Seconds())
			if err := p.Play(); err != nil {
				t.Fatal("anonymous_audio_resume_failed")
			}
			awaitAudio(t, at)
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			started := time.Now()
			startPosition := p.Snapshot().Position
			if err := v.Start(ctx, "52fNscjkST4", func() time.Duration { return p.Snapshot().Position }, func() bool { return p.Snapshot().Playing }); err != nil {
				t.Fatal("video_during_audio_start_failed")
			}
			defer v.Stop()
			for v.Loading() && ctx.Err() == nil {
				if state := p.Snapshot(); !state.Playing || state.Error != "" {
					t.Fatal("anonymous_audio_failed_during_video_start")
				}
				time.Sleep(20 * time.Millisecond)
			}
			frame, _ := v.Frame()
			if ctx.Err() != nil || v.Error() != "" || frame == nil {
				t.Logf("video_during_audio_first_frame_elapsed_seconds=%.3f audio_advance_seconds=%.3f", time.Since(started).Seconds(), (p.Snapshot().Position - startPosition).Seconds())
				t.Fatal("video_during_audio_first_frame_failed")
			}
			t.Logf("video_during_audio_first_frame_seconds=%.3f audio_position_seconds=%.3f width=%d height=%d", time.Since(started).Seconds(), p.Snapshot().Position.Seconds(), frame.Bounds().Dx(), frame.Bounds().Dy())

			observePlayback := func() {
				t.Helper()
				begin := time.Now()
				position := p.Snapshot().Position
				_, first := v.Frame()
				last, advancedAt := first, begin
				for time.Since(begin) < 2*time.Second {
					state := p.Snapshot()
					if ctx.Err() != nil || !state.Playing || state.Error != "" || v.Error() != "" || v.Loading() {
						t.Fatal("video_during_audio_playback_failed")
					}
					_, sequence := v.Frame()
					if sequence > last {
						last, advancedAt = sequence, time.Now()
					}
					if time.Since(advancedAt) > 750*time.Millisecond {
						t.Fatal("video_during_audio_frames_stalled")
					}
					time.Sleep(20 * time.Millisecond)
				}
				advance := p.Snapshot().Position - position
				if last-first < 24 || advance < 1500*time.Millisecond {
					t.Fatal("video_during_audio_insufficient_progress")
				}
				t.Logf("video_during_audio_observation_seconds=%.3f frames=%d audio_advance_seconds=%.3f", time.Since(begin).Seconds(), last-first, advance.Seconds())
			}
			observePlayback()
			if err := p.Pause(); err != nil {
				t.Fatal("anonymous_audio_pause_failed")
			}
			// Allow an in-flight presentation to settle before comparing frames.
			time.Sleep(100 * time.Millisecond)
			paused := p.Snapshot().Position
			_, pausedSequence := v.Frame()
			time.Sleep(300 * time.Millisecond)
			state = p.Snapshot()
			_, sequence := v.Frame()
			if state.Playing || state.Error != "" || state.Position > paused+50*time.Millisecond || sequence != pausedSequence || v.Error() != "" || v.Loading() {
				t.Fatal("video_during_audio_pause_failed")
			}
			if err := p.Play(); err != nil {
				t.Fatal("anonymous_audio_resume_failed")
			}
			awaitAudio(t, paused)
			observePlayback()
			t.Log("video_during_audio_pause_resume_passed")
		})
	}
}
