package player

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type livePCM struct {
	source  io.Reader
	bytes   atomic.Uint64
	nonzero atomic.Bool
}

func (r *livePCM) Read(dst []byte) (int, error) {
	n, err := r.source.Read(dst)
	r.bytes.Add(uint64(n))
	for _, value := range dst[:n] {
		if value != 0 {
			r.nonzero.Store(true)
			break
		}
	}
	return n, err
}

// Explicit anonymous network/device test. Output is restricted to timings,
// counters and state categories; URLs, headers and account stores are excluded.
func TestNativePreparedPlaybackLive(t *testing.T) {
	if os.Getenv("YTMGO_LIVE_PREPARED") != "1" {
		t.Skip("set YTMGO_LIVE_PREPARED=1 for anonymous playback timings")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	var evidence io.Writer = io.Discard
	if dir := os.Getenv("YTMGO_LIVE_EVIDENCE"); dir != "" {
		if os.MkdirAll(dir, 0700) != nil {
			t.Fatal("evidence directory failed")
		}
		if os.WriteFile(filepath.Join(dir, "probe.pid"), []byte(strconv.Itoa(os.Getpid())), 0600) != nil {
			t.Fatal("pid evidence failed")
		}
		file, err := os.OpenFile(filepath.Join(dir, "results.jsonl"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			t.Fatal("evidence file failed")
		}
		defer file.Close()
		evidence = file
	}
	emit := func(row map[string]any) {
		row["time"] = time.Now().UTC().Format(time.RFC3339Nano)
		_ = json.NewEncoder(evidence).Encode(row)
		line, _ := json.Marshal(row)
		t.Log(string(line))
	}
	start := time.Now()
	p, err := New()
	if err != nil {
		t.Fatal("native runtime init failed")
	}
	defer p.Close()
	initializeAnonymousLive(t, p)
	p.SetVolume(.05)
	var mu sync.Mutex
	readers := map[string]*livePCM{}
	var resolveCount, openCount atomic.Int64
	resolve, open := p.resolve, p.open
	p.resolve = func(ctx context.Context, id string) (media, error) { resolveCount.Add(1); return resolve(ctx, id) }
	p.open = func(ctx context.Context, info media, at time.Duration) (*pcmDecoder, error) {
		openCount.Add(1)
		d, err := open(ctx, info, at)
		if err != nil {
			return nil, err
		}
		reader := &livePCM{source: d.input}
		d.input = reader
		mu.Lock()
		readers[info.URL] = reader
		mu.Unlock()
		return d, nil
	}
	const first, second = "52fNscjkST4", "4D7u5KF7SP8"
	awaitOutput := func() *livePCM {
		t.Helper()
		deadline := time.Now().Add(10 * time.Second)
		for time.Now().Before(deadline) && ctx.Err() == nil {
			p.mu.Lock()
			info := p.media
			p.mu.Unlock()
			if info != nil {
				mu.Lock()
				reader := readers[info.URL]
				mu.Unlock()
				state := p.Snapshot()
				if state.Error != "" {
					t.Fatal("native playback error")
				}
				if reader != nil && reader.nonzero.Load() && state.Playing && state.Position >= 50*time.Millisecond {
					return reader
				}
			}
			time.Sleep(10 * time.Millisecond)
		}
		t.Fatal("nonzero PCM and advancing native clock not observed")
		return nil
	}
	load := func(id, label string, prepared bool) {
		t.Helper()
		resolved, opened := resolveCount.Load(), openCount.Load()
		click := time.Now()
		if err := p.Load(ctx, id); err != nil {
			t.Fatalf("%s: %s", label, safeError(err))
		}
		readyMS := time.Since(click).Seconds() * 1000
		reader := awaitOutput()
		if prepared && (resolveCount.Load() != resolved || openCount.Load() != opened) {
			t.Fatal("prepared handoff repeated resolution or decoder startup")
		}
		emit(map[string]any{"event": label, "load_ms": readyMS, "output_observed_ms": time.Since(click).Seconds() * 1000, "pcm_nonzero": reader.nonzero.Load(), "new_resolves": resolveCount.Load() - resolved, "new_decoders": openCount.Load() - opened, "position_seconds": p.Snapshot().Position.Seconds()})
	}
	load(first, "cold_first", false)
	emit(map[string]any{"event": "cold_start_total", "ms": time.Since(start).Seconds() * 1000})
	time.Sleep(3 * time.Second)
	for index, id := range []string{second, first, second} {
		before := p.Snapshot().Position
		prepareStart := time.Now()
		if err := p.Prepare(ctx, id); err != nil {
			t.Fatalf("prepare: %s", safeError(err))
		}
		emit(map[string]any{"event": "prepare", "cycle": index + 1, "ms": time.Since(prepareStart).Seconds() * 1000, "current_position_before": before.Seconds(), "current_position_after": p.Snapshot().Position.Seconds()})
		if index == 0 {
			// A prepared pipe must survive being held, rather than timing out
			// while the current track keeps playing. This also includes video
			// as a simultaneous third media consumer.
			v, err := NewVideo(p)
			if err != nil {
				t.Fatal("video init failed")
			}
			if err := v.Start(ctx, first, func() time.Duration { return p.Snapshot().Position }, func() bool { return p.Snapshot().Playing }); err != nil {
				t.Fatal("video start failed")
			}
			deadline := time.Now().Add(25 * time.Second)
			for v.Loading() && time.Now().Before(deadline) {
				time.Sleep(50 * time.Millisecond)
			}
			frame, _ := v.Frame()
			if frame == nil || v.Error() != "" {
				v.Close()
				t.Fatal("audio/prepared/video coexistence failed")
			}
			emit(map[string]any{"event": "concurrent_video", "width": frame.Bounds().Dx(), "height": frame.Bounds().Dy()})
			v.Close()
			time.Sleep(35 * time.Second)
		} else {
			time.Sleep(2 * time.Second)
		}
		load(id, "prepared_next", true)
		time.Sleep(3 * time.Second)
	}
	p.ClearPrepared()
	load(first, "unprepared_random", false)
	if err := p.Pause(); err != nil {
		t.Fatal("pause failed")
	}
	paused := p.Snapshot().Position
	time.Sleep(250 * time.Millisecond)
	if p.Snapshot().Position > paused+50*time.Millisecond {
		t.Fatal("pause advanced")
	}
	if err := p.Seek(45 * time.Second); err != nil {
		t.Fatal("seek failed")
	}
	if err := p.Play(); err != nil {
		t.Fatal("resume failed")
	}
	time.Sleep(3 * time.Second)
	if state := p.Snapshot(); !state.Playing || state.Error != "" || state.Position < 47*time.Second {
		t.Fatal("seek did not advance")
	}
	emit(map[string]any{"event": "controls", "pause_held": true, "seek_position_seconds": p.Snapshot().Position.Seconds()})
	p.Close()
	emit(map[string]any{"event": "closed"})

	// A fresh player that finishes bootstrap before the first selection.
	p2, err := New()
	if err != nil {
		emit(map[string]any{"event": "failed", "phase": "fresh_create", "category": safeError(err)})
		t.Fatal("warm runtime init failed")
	}
	defer p2.Close()
	initializeAnonymousLive(t, p2)
	p2.SetVolume(.05)
	warm := time.Now()
	if err := p2.resolver.Warm(ctx); err != nil {
		emit(map[string]any{"event": "failed", "phase": "fresh_warm", "category": safeError(err)})
		t.Fatal("warm session failed")
	}
	emit(map[string]any{"event": "startup_warm", "ms": time.Since(warm).Seconds() * 1000})
	click := time.Now()
	if err := p2.Load(ctx, first); err != nil {
		emit(map[string]any{"event": "failed", "phase": "fresh_load", "category": safeError(err)})
		t.Fatal("warm first load failed")
	}
	emit(map[string]any{"event": "warm_first", "load_ms": time.Since(click).Seconds() * 1000})
	time.Sleep(3 * time.Second)
	if state := p2.Snapshot(); !state.Playing || state.Error != "" || state.Position < 2*time.Second {
		t.Fatal("warm playback did not advance")
	}
}
