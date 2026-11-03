package main

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"ytmgo/internal/catalog"
)

func TestRestartRestoresDuplicateQueueEntryPaused(t *testing.T) {
	path := filepath.Join(t.TempDir(), "playback-session.json")
	track := catalog.Item{ID: "aaaaaaaaaaa", Title: "First", Duration: time.Minute}
	second := catalog.Item{ID: "bbbbbbbbbbb", Title: "Second"}
	queue := []catalog.Item{track, second, track}
	a := &desktop{playbackPath: path, queueIndex: 2, unmutedVolume: .23,
		model: UIModel{Current: track, Queue: queue, Position: 31.25, Duration: 60, Playing: true, Muted: true}}
	a.savePlayback()
	reopened := &desktop{playbackPath: path, queueIndex: -1}
	reopened.restorePlayback()
	m := &reopened.model
	if !reflect.DeepEqual(m.Queue, queue) || reopened.queueIndex != 2 || m.Current != track || m.Playing || !reopened.restoredTrack || m.Position != 31.25 {
		t.Fatal("restart lost the selection, progress, or paused state")
	}
	if !m.Muted || m.Volume != 0 || reopened.unmutedVolume != .23 {
		t.Fatal("restart lost the mute setting or the previous volume")
	}
	// Quit again without playing, then unmute.
	reopened.savePlayback()
	next := &desktop{playbackPath: path}
	next.restorePlayback()
	next.mute()
	if next.model.Position != 31.25 || next.model.Volume != .23 {
		t.Fatal("a second restart lost the bookmark or unmute volume")
	}
}

func TestRestartWithQueuedTracksBeforeFirstPlay(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session")
	a := &desktop{playbackPath: path, queueIndex: -1, unmutedVolume: .37,
		model: UIModel{Volume: .37, Queue: []catalog.Item{{ID: "aaaaaaaaaaa"}}}}
	a.savePlayback()
	next := &desktop{playbackPath: path, queueIndex: -1}
	next.restorePlayback()
	if len(next.model.Queue) != 1 || next.queueIndex != -1 || next.model.Current.ID != "" || next.model.Volume != .37 {
		t.Fatal("unplayed queue or volume lost")
	}
	if err := os.WriteFile(path, []byte("{"), 0600); err != nil {
		t.Fatal(err)
	}
	fresh := &desktop{playbackPath: path, queueIndex: -1, model: UIModel{Volume: .7}}
	fresh.restorePlayback()
	if fresh.model.Volume != .7 || fresh.queueIndex != -1 || fresh.restoredTrack {
		t.Fatal("broken session overwrote startup defaults")
	}
}
