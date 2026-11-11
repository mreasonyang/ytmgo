package streaming

import "testing"

func TestMovingPicturesTellsSongsFromVideos(t *testing.T) {
	for kind, want := range map[string]bool{
		"MUSIC_VIDEO_TYPE_ATV":                   false, // a song: its cover, still
		"MUSIC_VIDEO_TYPE_PRIVATELY_OWNED_TRACK": false, // an upload: its cover, still
		"MUSIC_VIDEO_TYPE_OMV":                   true,
		"MUSIC_VIDEO_TYPE_UGC":                   true,
		"MUSIC_VIDEO_TYPE_OFFICIAL_SOURCE_MUSIC": true,
		"":                                       false, // left out: unknown
	} {
		response := map[string]any{"videoDetails": map[string]any{}}
		if kind != "" {
			response["videoDetails"].(map[string]any)["musicVideoType"] = kind
		}
		if got := movingPictures(response); got != want {
			t.Errorf("%q: moving pictures %v, want %v", kind, got, want)
		}
	}
}
