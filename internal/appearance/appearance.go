// Package appearance stores the app's appearance: system, light or dark.
package appearance

import "ytmgo/internal/preference"

func Valid(value string) bool { return value == "system" || value == "light" || value == "dark" }

func Load(path string) string { return preference.Load(path, Valid, "system") }

func Save(path, value string) error { return preference.Save(path, value, Valid) }
