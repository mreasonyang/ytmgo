// Package videoquality shares the video resolution preference with selection,
// decoding and the native UI.
package videoquality

import (
	"strconv"

	"ytmgo/internal/preference"
)

const Default = 720

func Options() []int { return []int{144, 240, 360, 480, 720, 1080} }

func Valid(height int) bool {
	for _, option := range Options() {
		if option == height {
			return true
		}
	}
	return false
}

func Normalize(height int) int {
	if !Valid(height) {
		return Default
	}
	return height
}

func Label(height int) string { return strconv.Itoa(Normalize(height)) + "p" }

func validStored(value string) bool {
	height, err := strconv.Atoi(value)
	return err == nil && Valid(height)
}

func Load(path string) int {
	value := preference.Load(path, validStored, strconv.Itoa(Default))
	height, _ := strconv.Atoi(value)
	return height
}

func Save(path string, height int) error {
	return preference.Save(path, strconv.Itoa(height), validStored)
}
