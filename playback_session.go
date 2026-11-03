package main

import (
	"encoding/json"
	"log"
	"os"
	"time"

	"github.com/egoist/mygo"
	"ytmgo/internal/catalog"
	"ytmgo/internal/preference"
)

type playbackSession struct {
	Version       int            `json:"version"`
	Queue         []catalog.Item `json:"queue"`
	Index         int            `json:"index"`
	Position      time.Duration  `json:"positionNs"`
	Volume        float64        `json:"volume"`
	UnmutedVolume float64        `json:"unmutedVolume"`
}

func (a *desktop) savePlayback() {
	if a.playbackPath == "" {
		return
	}
	s := playbackSession{Version: 1, Index: a.queueIndex, Volume: a.model.Volume, UnmutedVolume: a.unmutedVolume}
	s.Queue = make([]catalog.Item, len(a.model.Queue))
	for i, it := range a.model.Queue {
		s.Queue[i] = it.PlaybackMetadata()
	}
	if s.Index >= 0 && s.Index < len(s.Queue) && s.Queue[s.Index].ID == a.model.Current.ID {
		it := a.model.Current.PlaybackMetadata()
		it.Duration = time.Duration(a.model.Duration * float64(time.Second))
		s.Position = time.Duration(a.model.Position * float64(time.Second))
		// Take the final decoder position unless the UI still owns the bookmark.
		if a.player != nil && !a.restoredTrack && !a.loadPending {
			state := a.player.Snapshot()
			s.Position = state.Position
			if state.Duration > 0 {
				it.Duration = state.Duration
			}
		}
		s.Queue[s.Index] = it
	} else {
		s.Index = -1
	}
	data, err := json.Marshal(s)
	if err != nil || preference.WriteFile(a.playbackPath, data) != nil {
		log.Print("ytmgo: could not save playback session")
	}
}

func (a *desktop) restorePlayback() {
	data, err := os.ReadFile(a.playbackPath)
	var s playbackSession
	if err != nil || json.Unmarshal(data, &s) != nil || s.Version != 1 ||
		s.Index < -1 || s.Index >= len(s.Queue) || s.Position < 0 ||
		s.Volume < 0 || s.Volume > 1 || s.UnmutedVolume <= 0 || s.UnmutedVolume > 1 {
		return
	}
	for i, it := range s.Queue {
		it = it.PlaybackMetadata()
		if it.ID == "" {
			return
		}
		s.Queue[i] = it
	}
	a.model.Volume, a.model.Muted, a.unmutedVolume = s.Volume, s.Volume == 0, s.UnmutedVolume
	a.model.Queue, a.queueIndex = s.Queue, s.Index
	if s.Index >= 0 {
		it := s.Queue[s.Index]
		a.model.Current = it
		if it.Duration > 0 {
			s.Position = min(s.Position, it.Duration)
		}
		a.model.Position, a.model.Duration = s.Position.Seconds(), it.Duration.Seconds()
		a.restoredTrack = true
	}
}

func (a *desktop) publishPausedTrack() {
	it := a.model.Current
	mygo.NowPlaying.Set(mygo.NowPlayingInfo{
		Title: it.Title, Artist: it.Artist, Album: it.Album, Duration: it.Duration,
		Position: time.Duration(a.model.Position * float64(time.Second)), State: mygo.PlaybackPaused,
		Commands: mygo.MediaPlay | mygo.MediaPause | mygo.MediaNext | mygo.MediaPrevious | mygo.MediaSeek,
	})
}
