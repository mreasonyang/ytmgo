package main

// Keep the message identity alongside the rendered text. Locale changes can
// reformat notices without translating provider lyrics or resurrecting a
// message that another operation has cleared/replaced.
type uiMessage struct {
	key, rendered string
	args          map[string]any
}

func (m *uiMessage) set(target *string, tr func(string, map[string]any) string, key string, args map[string]any) {
	*m = uiMessage{key: key, args: args}
	m.rendered = tr(key, args)
	*target = m.rendered
}

func (m *uiMessage) relocalize(target *string, tr func(string, map[string]any) string) bool {
	if m.key == "" || *target == "" || *target != m.rendered {
		*m = uiMessage{}
		return false
	}
	old := *target
	m.rendered = tr(m.key, m.args)
	*target = m.rendered
	return old != *target
}

func (m *UIModel) relocalizeMessages(tr func(string, map[string]any) string) {
	m.errorMessage.relocalize(&m.Error, tr)
	m.playbackMessage.relocalize(&m.PlaybackError, tr)
	m.videoMessage.relocalize(&m.VideoError, tr)
	if m.lyricsMessage.relocalize(&m.Lyrics, tr) {
		m.LyricsRevision++
	}
}

func (a *desktop) setError(key string) {
	a.model.errorMessage.set(&a.model.Error, a.trArgs, key, nil)
}

func (a *desktop) setVideoError(key string) {
	a.model.videoMessage.set(&a.model.VideoError, a.trArgs, key, nil)
}

func (a *desktop) setLyricsNotice(key string) {
	a.model.clearLyrics("")
	a.model.lyricsMessage.set(&a.model.Lyrics, a.trArgs, key, nil)
}
