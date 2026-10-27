// Package language maps ytmgo's interface languages to the music service's
// and stores the user's choice ("system" follows the OS languages).
package language

import (
	"slices"

	"ytmgo/internal/preference"
)

// System follows the system's preferred languages.
const System = "system"

// Supported lists the interface languages, source language first. It must
// match the catalogs in locales/ (TestSupportedMatchesCatalogs).
var Supported = []string{"en", "ja", "zh-Hans", "zh-Hant"}

func Valid(value string) bool { return value == System || slices.Contains(Supported, value) }

func Load(path string) string { return preference.Load(path, Valid, System) }

func Save(path, value string) error { return preference.Save(path, value, Valid) }

// ServiceLanguage is the music service's hl parameter for an interface
// language, so the catalogue canvas speaks the same language as the shell.
func ServiceLanguage(locale string) string {
	switch locale {
	case "zh-Hans":
		return "zh-CN"
	case "zh-Hant":
		return "zh-TW"
	case "ja":
		return "ja"
	}
	return "en"
}

// Script names the writing system the shell's typography is tuned for.
func Script(locale string) string {
	switch locale {
	case "zh-Hans":
		return "hans"
	case "zh-Hant":
		return "hant"
	case "ja":
		return "jpan"
	}
	return "latn"
}
