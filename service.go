package main

import (
	"errors"
	"sync"

	"github.com/egoist/mygo"
	"ytmgo/internal/appearance"
	"ytmgo/locales"
)

// Preferences holds the appearance chosen by the native settings view.
type Preferences struct {
	path string
	mu   sync.Mutex
}

func (p *Preferences) Appearance() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return appearance.Load(p.path)
}

func (p *Preferences) SetAppearance(value string) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if err := appearance.Save(p.path, value); err != nil {
		return "", errors.New(locales.AppearanceSaveError())
	}
	mygo.Theme.SetSource(mygo.ThemeSource(value))
	return value, nil
}
