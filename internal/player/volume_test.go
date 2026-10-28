package player

import (
	"bytes"
	"context"
	"io"
	"testing"
	"time"
)

func assertPlayerVolume(t *testing.T, p *Player, want float64, output bool) {
	t.Helper()
	if got := p.Snapshot().Volume; got != want {
		t.Fatalf("player volume = %v, want %v", got, want)
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if output && p.session == nil {
		t.Fatal("expected an installed audio output")
	}
	if p.session != nil && p.session.output.Volume() != want {
		t.Fatalf("audio output volume = %v, want %v", p.session.output.Volume(), want)
	}
}

func TestVolumeSurvivesTrackSeekAndAccountTransitions(t *testing.T) {
	for _, tc := range []struct {
		name   string
		volume float64
	}{{"muted", 0}, {"user-volume", .31}} {
		t.Run(tc.name, func(t *testing.T) {
			p := testPlayer()
			defer p.Close()
			p.configure = func(context.Context, AccountSession) error { return nil }
			ctx := context.Background()
			if err := p.SetAccount(ctx, AccountSession{Revision: 1}); err != nil {
				t.Fatal(err)
			}
			if err := p.Load(ctx, "aaaaaaaaaaa"); err != nil {
				t.Fatal(err)
			}
			p.SetVolume(tc.volume)
			assertPlayerVolume(t, p, tc.volume, true)
			if err := p.Load(ctx, "bbbbbbbbbbb"); err != nil {
				t.Fatal(err)
			}
			assertPlayerVolume(t, p, tc.volume, true)
			if err := p.Prepare(ctx, "ccccccccccc"); err != nil {
				t.Fatal(err)
			}
			if err := p.Load(ctx, "ccccccccccc"); err != nil {
				t.Fatal(err)
			}
			assertPlayerVolume(t, p, tc.volume, true)
			if err := p.SeekContext(ctx, 20*time.Second); err != nil {
				t.Fatal(err)
			}
			assertPlayerVolume(t, p, tc.volume, true)
			for _, account := range []AccountSession{
				accountFixture(false, 2), // Sign in to a different identity.
				accountFixture(true, 2),  // Start membership, same identity.
				accountFixture(false, 2), // Expire membership, same identity.
				{Revision: 3},            // Sign out.
			} {
				if err := p.SetAccount(ctx, account); err != nil {
					t.Fatal(err)
				}
				assertPlayerVolume(t, p, tc.volume, false)
				if err := p.Load(ctx, "ddddddddddd"); err != nil {
					t.Fatal(err)
				}
				assertPlayerVolume(t, p, tc.volume, true)
			}
		})
	}
}

func TestMutedVolumeSurvivesNaturalEndAndPreparedNext(t *testing.T) {
	p := testPlayer()
	defer p.Close()
	// A finite, silent PCM source reaches the real output's natural EOF;
	// no Ended flag or decoder EOF flag is injected by the test.
	p.resolve = func(context.Context, string) (media, error) {
		return media{Duration: 100 * time.Millisecond}, nil
	}
	p.open = func(context.Context, media, time.Duration) (*pcmDecoder, error) {
		frames := sampleRate / 10
		stream := io.NopCloser(bytes.NewReader(make([]byte, frames*channels*4)))
		done := make(chan error)
		close(done)
		return &pcmDecoder{reader: stream, input: stream, cancel: func() {}, wait: done, length: int64(frames), checkLength: true}, nil
	}
	waitForNaturalEnd := func() {
		// Let MyGo consume the PCM, including its timed null output on a
		// machine with no audio device. Polling Snapshot while it drains
		// would exercise the separate output-unavailable path, which
		// intentionally pauses playback before EOF on headless CI.
		waitPlayerCondition(t, func() bool {
			p.mu.Lock()
			defer p.mu.Unlock()
			return p.session != nil && !p.session.output.Playing() && p.session.decoder.eof.Load()
		})
		if !p.Snapshot().Ended {
			t.Fatal("consumed PCM did not reach natural end")
		}
	}
	ctx := context.Background()
	p.SetVolume(.31)
	if err := p.Load(ctx, "aaaaaaaaaaa"); err != nil {
		t.Fatal(err)
	}
	p.SetVolume(0)
	if err := p.Prepare(ctx, "bbbbbbbbbbb"); err != nil {
		t.Fatal(err)
	}
	waitForNaturalEnd()
	assertPlayerVolume(t, p, 0, true)
	if err := p.Load(ctx, "bbbbbbbbbbb"); err != nil {
		t.Fatal(err)
	}
	assertPlayerVolume(t, p, 0, true)
	waitForNaturalEnd()
	assertPlayerVolume(t, p, 0, true)
}
