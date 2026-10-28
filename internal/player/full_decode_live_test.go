package player

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// This evidence type deliberately has no track ID, URL, headers, credentials,
// dependency message, or arbitrary data field.
type fullDecodeEvidence struct {
	Track           int     `json:"track"`
	Category        string  `json:"category"`
	Stage           string  `json:"stage,omitempty"`
	MetadataSeconds float64 `json:"metadata_seconds"`
	PCMSeconds      float64 `json:"pcm_seconds"`
	PCMFrames       int64   `json:"pcm_frames"`
	NonzeroSamples  int64   `json:"nonzero_samples"`
	EOF             int     `json:"eof"`
	DecoderFailed   int     `json:"decoder_failed"`
	BridgeFailed    int     `json:"bridge_failed"`
	ResolveSeconds  float64 `json:"resolve_seconds"`
	FirstPCMSeconds float64 `json:"first_pcm_seconds"`
	DecodeSeconds   float64 `json:"decode_seconds"`
	TotalSeconds    float64 `json:"total_seconds"`
}

// Explicit anonymous network acceptance. This drains bounded PCM buffers as
// fast as the transport permits, without creating a native audio output.
// FFmpeg exit 0 alone is insufficient: both entire tracks must reach EOF with
// nonzero, finite PCM and agree with their metadata duration within 0.5 seconds.
func TestNativeFullDecodeLive(t *testing.T) {
	if os.Getenv("YTMGO_LIVE_FULL_DECODE") != "1" {
		t.Skip("set YTMGO_LIVE_FULL_DECODE=1 for anonymous full-track decoding")
	}
	var evidence io.Writer = io.Discard
	if dir := os.Getenv("YTMGO_LIVE_EVIDENCE"); dir != "" {
		if os.MkdirAll(filepath.Dir(dir), 0700) != nil || os.Mkdir(dir, 0700) != nil {
			t.Fatal("full_decode_evidence_directory_failed")
		}
		file, err := os.OpenFile(filepath.Join(dir, "results.jsonl"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			t.Fatal("full_decode_evidence_file_failed")
		}
		defer file.Close()
		evidence = file
	}
	emit := func(row fullDecodeEvidence) {
		t.Helper()
		data, err := json.Marshal(row)
		if err != nil {
			t.Fatal("full_decode_evidence_encoding_failed")
		}
		if _, err = evidence.Write(append(data, '\n')); err != nil {
			t.Fatal("full_decode_evidence_write_failed")
		}
		t.Log(string(data))
	}
	paths, err := findRuntime()
	if err != nil {
		emit(fullDecodeEvidence{Category: "runtime_missing"})
		t.Fatal("full_decode_setup_failed")
	}
	resolver, err := paths.newResolver()
	if err != nil {
		emit(fullDecodeEvidence{Category: "runtime_missing"})
		t.Fatal("full_decode_setup_failed")
	}
	defer resolver.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 7*time.Minute)
	defer cancel()
	configureCtx, configured := context.WithTimeout(ctx, 15*time.Second)
	err = resolver.Configure(configureCtx, AccountSession{Revision: 1})
	configured()
	if err != nil {
		emit(fullDecodeEvidence{Category: "account_configuration_failed"})
		t.Fatal("full_decode_setup_failed")
	}
	for index, id := range []string{"52fNscjkST4", "4D7u5KF7SP8"} {
		row := decodeFullTrack(ctx, paths, resolver, index+1, id)
		emit(row)
		if row.Category != "pass" {
			t.Fatal("full_decode_track_failed")
		}
	}
}

func decodeFullTrack(parent context.Context, paths runtimePaths, resolver *resolver, ordinal int, id string) (row fullDecodeEvidence) {
	row.Track = ordinal
	started := time.Now()
	defer func() { row.TotalSeconds = time.Since(started).Seconds() }()
	// A complete recording must not be required to download faster than its
	// real duration. The live fixtures include a 6m10s track.
	ctx, cancel := context.WithTimeout(parent, 7*time.Minute)
	defer cancel()
	info, err := resolver.Resolve(ctx, id)
	row.ResolveSeconds = time.Since(started).Seconds()
	if err != nil {
		row.Category = safeError(err)
		row.Stage = safeErrorStage(err)
		return
	}
	defer releaseMedia(&info)
	row.MetadataSeconds = info.Duration.Seconds()
	opened := time.Now()
	decoder, err := paths.open(ctx, info, 0)
	row.FirstPCMSeconds = time.Since(opened).Seconds()
	if err != nil {
		row.Category = safeError(err)
		row.Stage = safeErrorStage(err)
		return
	}
	defer decoder.Close()
	// Decoder startup separates its lifetime from the loading context. Bind
	// this explicit acceptance workload's deadline to the entire PCM drain.
	stop := context.AfterFunc(ctx, func() { _ = decoder.Close() })
	defer stop()
	decoded := time.Now()
	samples := make([]float32, 4096)
	finite := true
	var readErr error
	for {
		n, err := decoder.Read(samples)
		row.PCMFrames += int64(n / channels)
		for _, sample := range samples[:n] {
			if math.IsNaN(float64(sample)) || math.IsInf(float64(sample), 0) {
				finite = false
			} else if sample != 0 {
				row.NonzeroSamples++
			}
		}
		if err != nil {
			readErr = err
			break
		}
	}
	row.DecodeSeconds = time.Since(decoded).Seconds()
	row.PCMSeconds = float64(row.PCMFrames) / sampleRate
	if decoder.eof.Load() {
		row.EOF = 1
	}
	if decoder.failed.Load() {
		row.DecoderFailed = 1
	}
	bridgeErr := decoder.bridge.Err()
	if bridgeErr != nil {
		row.BridgeFailed = 1
	}
	switch {
	case ctx.Err() != nil:
		row.Category = safeError(ctx.Err())
	case bridgeErr != nil:
		row.Category = safeError(bridgeErr)
	case !errors.Is(readErr, io.EOF) || row.EOF != 1:
		row.Category = "pcm_eof_missing"
	case row.DecoderFailed != 0:
		row.Category = "audio_stream_failed"
	case !finite:
		row.Category = "pcm_non_finite"
	case row.NonzeroSamples == 0:
		row.Category = "pcm_silent"
	case math.Abs(row.PCMSeconds-row.MetadataSeconds) > .5:
		row.Category = "pcm_duration_mismatch"
	default:
		row.Category = "pass"
	}
	return
}
