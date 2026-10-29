package player

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const sampleRate = 48000
const channels = 2
const firstPCMTimeout = 30 * time.Second
const pcmDurationTolerance = time.Second / 2

type pcmDecoder struct {
	reader      io.ReadCloser
	input       io.Reader
	cancel      context.CancelFunc
	wait        <-chan error
	bridge      *streamBridge
	closeOnce   sync.Once
	length      int64
	readFrames  int64
	checkLength bool
	buf         [16 << 10]byte
	eof, failed atomic.Bool
}

// decoderDiagnostics retains only a short parsing tail while the process runs.
// It emits fixed categories and erases the tail when read; never expose stderr.
type decoderDiagnostics struct {
	mu       sync.Mutex
	tail     string
	category string
}

func (d *decoderDiagnostics) Write(data []byte) (int, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	text := strings.ToLower(d.tail + string(data))
	for _, rule := range []struct{ token, category string }{
		{"403 forbidden", "audio_access_denied"},
		{"server returned 403", "audio_access_denied"},
		{"http error 403", "audio_access_denied"},
		{"429 too many requests", "audio_rate_limited"},
		{"server returned 5", "audio_server_error"},
		{"timed out", "audio_network_timeout"},
		{"connection reset", "audio_connection_failed"},
		{"certificate verify failed", "audio_connection_failed"},
	} {
		if strings.Contains(text, rule.token) {
			d.category = rule.category
			break
		}
	}
	d.tail = text[max(0, len(text)-512):]
	return len(data), nil
}

func (d *decoderDiagnostics) result() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.tail = ""
	if d.category == "" {
		return errors.New("audio_stream_failed")
	}
	return errors.New(d.category)
}

func (r runtimePaths) open(ctx context.Context, info media, at time.Duration) (*pcmDecoder, error) {
	if !validMediaURL(info.URL) {
		return nil, errors.New("invalid_media_response")
	}
	return startDecoder(ctx, r.ffmpeg, info, at)
}

func startDecoder(ctx context.Context, executable string, info media, at time.Duration) (*pcmDecoder, error) {
	ready, readyCancel := context.WithTimeout(ctx, firstPCMTimeout)
	defer readyCancel()
	life, cancel := context.WithCancel(context.Background())
	bridge, err := newStreamBridge(life, info)
	if err != nil {
		cancel()
		return nil, err
	}
	input := audioDecoderManifest(bridge.URL, info.Duration, at)
	cmd := childCommand(life, executable, "-hide_banner", "-loglevel", "error", "-nostdin",
		"-ss", "0", "-f", "concat", "-safe", "0",
		"-protocol_whitelist", "pipe,file,http,tcp", "-i", "pipe:0",
		"-vn", "-sn", "-dn", "-threads", "1", "-ac", "2", "-ar", "48000", "-f", "f32le", "pipe:1")
	diagnostic := &decoderDiagnostics{}
	cmd.Stdin, cmd.Stderr = strings.NewReader(input), diagnostic
	rd, wr, err := os.Pipe()
	if err != nil {
		cancel()
		bridge.Close()
		return nil, errors.New("audio_decoder_unavailable")
	}
	cmd.Stdout = wr
	if err = cmd.Start(); err != nil {
		cancel()
		bridge.Close()
		rd.Close()
		wr.Close()
		return nil, errors.New("audio_decoder_unavailable")
	}
	wr.Close()
	done := make(chan error, 1)
	go func() { done <- cmd.Wait(); close(done) }()
	d := &pcmDecoder{reader: rd, input: rd, cancel: cancel, wait: done, bridge: bridge, length: int64((info.Duration - at).Seconds() * sampleRate), checkLength: true}
	stop := context.AfterFunc(ready, func() { d.Close() })
	defer stop()
	// Require actual PCM except when a successful decoder has already reached
	// the advertised tail: codec padding can leave no samples at the last ms.
	first := make([]byte, 4096)
	n, err := io.ReadFull(rd, first)
	if ready.Err() != nil {
		d.Close()
		return nil, ready.Err()
	}
	remaining := info.Duration - at
	if n == 0 && err == io.EOF && at > 0 && remaining >= 0 && remaining <= pcmDurationTolerance {
		// EOF on stdout is insufficient: the process could close it and hang,
		// or exit with an error. Keep readiness cancellation active until Wait.
		select {
		case result := <-done:
			if ready.Err() != nil {
				d.Close()
				return nil, ready.Err()
			}
			if result == nil && bridge.Err() == nil {
				return d, nil
			}
		case <-ready.Done():
			d.Close()
			return nil, ready.Err()
		}
	}
	if err != nil && err != io.ErrUnexpectedEOF || n < 8 {
		d.Close()
		if err := bridge.Err(); err != nil {
			return nil, err
		}
		return nil, diagnostic.result()
	}
	d.input = io.MultiReader(bytes.NewReader(first[:n]), rd)
	return d, nil
}

// Seek the media inside the concat demuxer before it probes packets. Applying
// only -ss to the outer concat input can scan an entire AAC file before the
// first PCM sample, making late seeks hit the startup deadline. The inpoint
// resets the timeline; input -ss 0 still discards audio codec preroll.
// Both the upstream URL and the local capability stay out of process argv:
// FFmpeg receives only this single-file manifest over its private stdin.
func audioDecoderManifest(url string, duration, at time.Duration) string {
	return fmt.Sprintf("ffconcat version 1.0\nfile '%s'\ninpoint %.6f\nduration %.6f\noption rw_timeout 10000000\n", url, at.Seconds(), max(duration-at, 0).Seconds())
}

func (d *pcmDecoder) SampleRate() int { return sampleRate }
func (d *pcmDecoder) Channels() int   { return channels }
func (d *pcmDecoder) Len() int64      { return max(d.length, 0) }
func (d *pcmDecoder) SetPosition(frame int64) error {
	// The owning Player implements seek by opening a range-capable decoder;
	// MyGo must never try to replay an exhausted one-shot pipe.
	return errors.New("decoder_requires_reopen")
}
func (d *pcmDecoder) Read(dst []float32) (int, error) {
	count := min(len(dst)-len(dst)%channels, len(d.buf)/4)
	if count == 0 {
		return 0, nil
	}
	n, err := io.ReadFull(d.input, d.buf[:count*4])
	n -= n % 8
	d.readFrames += int64(n / 8)
	for i := 0; i < n/4; i++ {
		dst[i] = math.Float32frombits(binary.LittleEndian.Uint32(d.buf[i*4:]))
	}
	if err != nil {
		d.eof.Store(true)
		if err != io.EOF && err != io.ErrUnexpectedEOF {
			d.failed.Store(true)
		}
		if result := <-d.wait; result != nil {
			d.failed.Store(true)
		}
		// FFmpeg can exit successfully when a CDN truncates the stream. Require
		// the advertised PCM duration (allowing codec padding), not only exit 0.
		if d.checkLength && d.readFrames+int64(pcmDurationTolerance.Seconds()*sampleRate) < d.length {
			d.failed.Store(true)
		}
		if d.bridge != nil && d.bridge.Err() != nil {
			d.failed.Store(true)
		}
		return n / 4, io.EOF
	}
	return n / 4, nil
}
func (d *pcmDecoder) Close() error {
	d.closeOnce.Do(func() {
		d.cancel()
		d.reader.Close()
		if d.bridge != nil {
			d.bridge.Close()
		}
		// Every helper is reaped before Close returns, including cancellation
		// during read/seek. The channel closes after its single status value.
		<-d.wait
	})
	return nil
}
