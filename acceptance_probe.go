package main

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"time"

	"ytmgo/internal/player"
)

// An expiring, local opt-in supports unattended playback acceptance while the
// screen is locked. Normal launches do no additional sampling or file writes.
// Neither the request nor the samples contain track/account metadata, URLs,
// dependency errors, credentials, process arguments or environment values.
type acceptanceRequest struct {
	Version string    `json:"version"`
	Until   time.Time `json:"until"`
}

type acceptanceProbe struct {
	directory      string
	started, until time.Time
	next           time.Time
	sequence       uint64
}

func loadAcceptanceProbe(directory string, now time.Time) *acceptanceProbe {
	path := filepath.Join(directory, "acceptance-request.json")
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() > 1024 {
		return nil
	}
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	var request acceptanceRequest
	decoder := json.NewDecoder(io.LimitReader(f, 1024))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&request) != nil || decoder.Decode(new(any)) != io.EOF || request.Version != version || !request.Until.After(now) || request.Until.Sub(now) > 4*time.Hour {
		return nil
	}
	return &acceptanceProbe{directory: directory, started: now, until: request.Until}
}

func (p *acceptanceProbe) observe(now time.Time, state player.State) {
	if p == nil || !now.Before(p.until) || now.Before(p.next) {
		return
	}
	p.next = now.Add(5 * time.Second)
	p.sequence++
	data, err := json.Marshal(struct {
		Version                string
		Time                   time.Time
		Sequence               uint64
		ElapsedMS              int64
		PositionMS, DurationMS int64
		Playing, Loading       bool
		Ended, Failed          bool
	}{version, now.UTC(), p.sequence, now.Sub(p.started).Milliseconds(),
		max(state.Position.Milliseconds(), 0), max(state.Duration.Milliseconds(), 0),
		state.Playing, state.Loading, state.Ended, state.Error != ""})
	if err != nil {
		return
	}
	f, err := os.CreateTemp(p.directory, "acceptance-*.tmp")
	if err != nil {
		return
	}
	defer os.Remove(f.Name())
	_, err = f.Write(data)
	closeErr := f.Close()
	if err == nil && closeErr == nil {
		_ = os.Rename(f.Name(), filepath.Join(p.directory, "acceptance-playback.json"))
	}
}
