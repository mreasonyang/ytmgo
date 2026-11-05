package main

import (
	"testing"

	"ytmgo/internal/player"
	"ytmgo/locales"
)

func TestDisplayedFailuresAndLyricsNoticeFollowLanguage(t *testing.T) {
	for _, locale := range []string{"zh-Hans", "zh-Hant", "ja", "en"} {
		t.Run(locale, func(t *testing.T) {
			a := &desktop{}
			en := func(key string, args map[string]any) string { return locales.Catalog.Format("en", key, args) }
			a.model.setPlaybackFailure(player.State{Error: "audio_access_denied", ErrorDetails: player.FailureDetails{Stage: "media_stream", HTTPStatus: 403}}, en)
			a.model.errorMessage.set(&a.model.Error, en, "nativeAccountLoginFailed", nil)
			a.model.videoMessage.set(&a.model.VideoError, en, "nativeVideoUnavailable", nil)
			a.model.lyricsMessage.set(&a.model.Lyrics, en, "nativeNoLyrics", nil)
			render := func(key string, args map[string]any) string { return locales.Catalog.Format(locale, key, args) }
			a.model.relocalizeMessages(render)
			if a.model.PlaybackError != render("nativePlaybackAccessDeniedHTTP", map[string]any{"status": 403}) ||
				a.model.Error != render("nativeAccountLoginFailed", nil) || a.model.VideoError != render("nativeVideoUnavailable", nil) || a.model.Lyrics != render("nativeNoLyrics", nil) {
				t.Fatal("existing notices did not follow the selected language or lost their HTTP status")
			}
		})
	}
}

func TestRelocalizationCannotResurrectClearedErrorOrTranslateRealLyrics(t *testing.T) {
	a := &desktop{}
	en := func(key string, args map[string]any) string { return locales.Catalog.Format("en", key, args) }
	a.model.setPlaybackFailure(player.State{Error: "region_unavailable"}, en)
	a.model.lyricsMessage.set(&a.model.Lyrics, en, "nativeNoLyrics", nil)
	old := a.model.Lyrics
	a.model.PlaybackError = ""
	// Provider text can even equal an old notice: clearLyrics removes its key.
	a.model.clearLyrics(old)
	a.model.LyricsAvailable = true
	render := func(key string, args map[string]any) string { return locales.Catalog.Format("zh-Hans", key, args) }
	a.model.relocalizeMessages(render)
	if a.model.PlaybackError != "" || a.model.Lyrics != old || !a.model.LyricsAvailable {
		t.Fatal("locale change resurrected a cleared failure or translated provider lyrics")
	}
}
