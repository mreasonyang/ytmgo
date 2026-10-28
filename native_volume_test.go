package main

import (
	"testing"

	"github.com/egoist/mygo/ui"
	"ytmgo/internal/catalog"
)

func TestNativeMutedVolumeSurvivesPlaybackRefresh(t *testing.T) {
	m, actions := nativeViewFixture()
	a := &desktop{model: *m, unmutedVolume: .7}
	a.volume(.31)
	m = &a.model
	actions.Mute, actions.Volume = a.mute, a.volume
	tt := ui.NewTester(func(c *ui.Context) { nativeView(c, m, actions) }, 1440, 900)
	if err := tt.Click(actions.Tr("shellMute")); err != nil {
		t.Fatal(err)
	}
	assertMuted := func() {
		t.Helper()
		if m.Volume != 0 || !m.Muted {
			t.Fatalf("mute lost: volume=%v muted=%v", m.Volume, m.Muted)
		}
	}
	assertMuted()
	for i := 0; i < 10; i++ {
		m.Position++
		tt.Frame()
		assertMuted()
	}
	m.Playing, m.PlaybackLoading = false, true
	m.Current = m.Queue[1]
	m.Position = 0
	tt.Frame()
	assertMuted()
	m.Playing, m.PlaybackLoading = true, false
	tt.Frame()
	assertMuted()
	// A seek completion and an account-driven replacement of the visible
	// track must not reapply a stale slider value or repeat the mute click.
	m.Position = 120
	tt.Frame()
	assertMuted()
	m.SignedIn, m.MusicPremium = true, true
	m.Current = catalog.Item{}
	tt.Frame()
	assertMuted()
	m.Current, m.SignedIn, m.MusicPremium = m.Queue[2], false, false
	tt.Frame()
	assertMuted()
	if err := tt.Click(actions.Tr("shellUnmute")); err != nil {
		t.Fatal(err)
	}
	if m.Muted || m.Volume != .31 {
		t.Fatalf("unmute did not restore the user's volume: volume=%v muted=%v", m.Volume, m.Muted)
	}
}
