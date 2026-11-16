// Package audioquality persists the preferred streaming audio quality.
package audioquality

import "ytmgo/internal/preference"

type Quality string

const (
	Saver    Quality = "saver"
	Standard Quality = "standard"
	Best     Quality = "best"
)

func (q Quality) Valid() bool {
	return q == Saver || q == Standard || q == Best
}

func (q Quality) Normalize() Quality {
	if !q.Valid() {
		return Best
	}
	return q
}

func validStored(value string) bool { return Quality(value).Valid() }

func Load(path string) Quality {
	return Quality(preference.Load(path, validStored, string(Best)))
}

func Save(path string, q Quality) error {
	return preference.Save(path, string(q), validStored)
}
