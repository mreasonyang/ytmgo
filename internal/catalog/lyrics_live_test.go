package catalog

import (
	"context"
	"os"
	"testing"
	"time"
)

// Explicit anonymous metadata verification. The client never reads a stored
// account; only counts, durations and a fixed result leave the test. No lyric
// text, response bodies, media URLs or authentication data are logged.
func TestAnonymousTimedLyricsLive(t *testing.T) {
	if os.Getenv("YTMGO_LIVE_LYRICS") != "1" {
		t.Skip("set YTMGO_LIVE_LYRICS=1 for anonymous timed lyrics verification")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	client := New()
	synced := 0
	for index, query := range []string{"Daft Punk Get Lucky", "Adele Hello"} {
		started := time.Now()
		page, err := client.Search(ctx, query)
		if err != nil {
			t.Fatal("anonymous_lyrics_search_failed")
		}
		id := ""
		for _, item := range page.Items {
			if item.ID != "" && item.Kind == "song" {
				id = item.ID
				break
			}
		}
		if id == "" {
			t.Fatal("anonymous_lyrics_song_missing")
		}
		result, err := client.LyricsWithTiming(ctx, id)
		if err != nil {
			t.Logf("anonymous_lyrics_track=%d available=0 elapsed_seconds=%.3f", index+1, time.Since(started).Seconds())
			continue
		}
		if len(result.Lines) == 0 {
			t.Logf("anonymous_lyrics_track=%d available=1 timed_lines=0 text_bytes=%d elapsed_seconds=%.3f", index+1, len(result.Text), time.Since(started).Seconds())
			continue
		}
		synced++
		t.Logf("anonymous_lyrics_track=%d available=1 timed_lines=%d first_start_seconds=%.3f last_end_seconds=%.3f elapsed_seconds=%.3f", index+1, len(result.Lines), result.Lines[0].Start.Seconds(), result.Lines[len(result.Lines)-1].End.Seconds(), time.Since(started).Seconds())
	}
	if synced == 0 {
		t.Fatal("anonymous_timed_lyrics_not_observed")
	}
	t.Logf("anonymous_timed_lyrics_tracks=%d", synced)
}
