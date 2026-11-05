package streaming

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"
)

func regionTemplate(language, message string) string {
	return fmt.Sprintf(`<!doctype html><html lang="%s"><head><title>%s</title></head><body><div class="content"><img class="logo" src="//music.youtube.com/img/on_platform_logo_dark.svg"><div class="message">%s</div></div></body></html>`, language, message, message)
}

func TestRegionPageTemplateDoesNotDependOnEnglishText(t *testing.T) {
	for _, tc := range []struct{ language, message string }{
		{"ja", "この地域では音楽を利用できません"},
		{"zh-CN", "当前地区无法使用音乐服务"},
		{"zh-TW", "目前地區無法使用音樂服務"},
		{"es", "YouTube Music no está disponible en tu país"},
	} {
		t.Run(tc.language, func(t *testing.T) {
			page := regionTemplate(tc.language, tc.message)
			if !musicRegionUnavailable([]byte(page)) {
				t.Fatal("localized branded region template was not recognized")
			}
		})
	}
	page := regionTemplate("fixture", "synthetic localized rejection")
	for name, body := range map[string]string{
		"unbranded":                strings.ReplaceAll(page, `class="logo"`, `class="other"`),
		"wrong host":               strings.ReplaceAll(page, "music.youtube.com", "example.invalid"),
		"different logo":           strings.ReplaceAll(page, "on_platform_logo_dark.svg", "logo.svg"),
		"generic title":            strings.ReplaceAll(page, "<title>synthetic localized rejection</title>", "<title>YouTube Music</title>"),
		"application page":         strings.ReplaceAll(page, "</head>", `<script>ytcfg.set({})</script></head>`),
		"login form":               strings.ReplaceAll(page, "</body>", `<form></form></body>`),
		"oversized":                page + strings.Repeat(" ", 64<<10),
		"empty":                    "<html></html>",
		"script translation":       `<html><title>YouTube Music</title><script>const text="YouTube Music is not available in your area";</script></html>`,
		"matching generic error":   regionTemplate("en", "Something went wrong"),
		"matching localized error": regionTemplate("ja", "エラーが発生しました"),
		"unknown text":             regionTemplate("fixture", "synthetic localized rejection &amp; notice"),
	} {
		t.Run(name, func(t *testing.T) {
			if musicRegionUnavailable([]byte(body)) {
				t.Fatal("unrelated page was classified as a region refusal")
			}
		})
	}
}

func TestLocalizedRegionMessageSurvivesOfficialLogoRename(t *testing.T) {
	page := regionTemplate("ja", "この地域では YouTube Music を利用できません")
	page = strings.ReplaceAll(page, "on_platform_logo_dark.svg", "renamed_music_logo.svg")
	if !musicRegionUnavailable([]byte(page)) {
		t.Fatal("explicit branded region refusal depended on one logo filename")
	}
}

func TestRegionRefusalContractionsAndSpanishZone(t *testing.T) {
	for _, message := range []string{
		"YouTube Music isn't available in your region",
		"YouTube Music isn’t available in your region",
		"YouTube Music no está disponible en tu zona",
	} {
		t.Run(message, func(t *testing.T) {
			if !musicRegionUnavailable([]byte(regionTemplate("fixture", message))) {
				t.Fatal("observed localized region refusal was not recognized")
			}
		})
	}
	for _, message := range []string{
		"YouTube Music isn't available in this browser",
		"YouTube Music no está disponible en este navegador",
		"YouTube Music: selecciona tu zona",
	} {
		t.Run(message, func(t *testing.T) {
			if musicRegionUnavailable([]byte(regionTemplate("fixture", message))) {
				t.Fatal("non-geographic refusal or zone selection was misclassified")
			}
		})
	}
}

func TestProofConfigLocaleIsPinnedAndLocalizedRefusalClassified(t *testing.T) {
	r := testResolver(t, func(req *http.Request) (*http.Response, error) {
		if req.URL.Host != "music.youtube.com" || req.URL.Path != "/" || req.Header.Get("Accept-Language") != "en-US,en;q=0.7" {
			t.Error("proof config locale no longer matches its text fallback")
		}
		return response(regionTemplate("ja", "地域の制限により利用できません")), nil
	})
	r.session = Session{Revision: 1}
	r.seed = extracted{Code: "public", Timestamp: 12345}
	r.playerID, r.playerChecked = "fixture", time.Now()
	r.sessionClient = map[string]any{"visitorData": "visitor", "hl": "ja"}
	if err := r.Warm(context.Background()); err == nil || err.Error() != "region_unavailable" {
		t.Fatal("localized refusal became a generic proof failure")
	}
}
