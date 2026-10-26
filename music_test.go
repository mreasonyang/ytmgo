package main

import (
	"strings"
	"testing"
)

func TestMusicPreloadIsolation(t *testing.T) {
	script := musicPreload("system")
	for _, required := range []string{"location.origin !== 'https://music.youtube.com'", "window.top !== window", "let mode = \"system\"", "ytmgo://appearance/", "ytmgo-sidebar", "ytmgo-welcome"} {
		if !strings.Contains(script, required) {
			t.Errorf("missing shell contract %q", required)
		}
	}
	for _, forbidden := range []string{"YTMGO_STYLES", "YTMGO_APPEARANCE", "document.cookie", "localStorage", "window.mygo", "fetch("} {
		if strings.Contains(script, forbidden) {
			t.Errorf("unexpected bridge, credential access, or template token %q", forbidden)
		}
	}
}
