package main

import (
	"context"
	"path/filepath"
	"testing"

	"ytmgo/internal/audioquality"
	"ytmgo/internal/catalog"
	"ytmgo/internal/player"
)

func TestAudioQualityChangeReplacesOnlyQueuedPreparation(t *testing.T) {
	for _, foreground := range []bool{false, true} {
		name := "queued"
		if foreground {
			name = "claimed by foreground load"
		}
		t.Run(name, func(t *testing.T) {
			// A closed player keeps this controller test independent of the network.
			p := &player.Player{}
			p.Close()
			first, next := catalog.Item{ID: "aaaaaaaaaaa"}, catalog.Item{ID: "bbbbbbbbbbb"}
			old, cancelOld := context.WithCancel(t.Context())
			defer cancelOld()
			a := &desktop{ctx: t.Context(), player: p, prepareCancel: cancelOld,
				audioQualityPath: filepath.Join(t.TempDir(), "audio-quality"),
				preparedID:       next.ID, preparedQuality: audioquality.Best,
				model: UIModel{Current: first, Queue: []catalog.Item{first, next}, Playing: true, Position: 24.5}}
			if foreground {
				// startTrackAt has handed off the queued preparation. Canceling
				// its context would also cancel the foreground decoder open.
				a.preparedID = ""
				a.loadPending = true
			}
			a.setAudioQuality(audioquality.Standard)
			if audioquality.Load(a.audioQualityPath) != audioquality.Standard || a.model.AudioQualityNotice != "nativeAudioQualitySaved" {
				t.Fatal("new preference was not saved")
			}
			if foreground && old.Err() != nil {
				t.Fatal("changing preference cancelled the current load")
			}
			if !foreground && (old.Err() == nil || a.preparedID != next.ID || a.preparedQuality != audioquality.Standard) {
				t.Fatal("next track was not rescheduled at the new quality")
			}
			if a.model.Current != first || !a.model.Playing || a.model.Position != 24.5 {
				t.Fatal("saving audio quality changed current playback")
			}
		})
	}
}

func TestAudioQualitySaveFailurePreservesPlaybackPreference(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audio-quality")
	if err := audioquality.Save(path, audioquality.Standard); err != nil {
		t.Fatal(err)
	}
	a := &desktop{audioQualityPath: filepath.Join(path, "child"), model: UIModel{AudioQuality: audioquality.Standard}}
	a.setAudioQuality(audioquality.Saver)
	if a.model.AudioQualityNotice != "nativeAudioQualitySaveError" || a.model.AudioQuality != audioquality.Standard || audioquality.Load(path) != audioquality.Standard {
		t.Fatal("failed save lost the working preference or lacked an error")
	}
	a.audioQualityPath = path
	a.setAudioQuality(audioquality.Saver)
	if a.model.AudioQualityNotice != "nativeAudioQualitySaved" || audioquality.Load(path) != audioquality.Saver {
		t.Fatal("retry did not save the new preference")
	}
}
