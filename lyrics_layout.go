package main

import (
	"ytmgo/internal/preference"
	"ytmgo/locales"
)

func validLyricsLayout(value string) bool { return value == "standard" || value == "expanded" }

func loadLyricsExpanded(path string) bool {
	return preference.Load(path, validLyricsLayout, "standard") == "expanded"
}

func (a *desktop) setLyricsExpanded(expanded bool) {
	value := "standard"
	if expanded {
		value = "expanded"
	}
	if err := preference.Save(a.lyricsLayoutPath, value, validLyricsLayout); err != nil {
		a.model.Error = locales.NativeLyricsLayoutSaveError()
		a.model.ErrorScope = ""
	} else {
		a.model.LyricsExpanded = expanded
	}
	a.invalidate()
}
