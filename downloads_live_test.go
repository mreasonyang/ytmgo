package main

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"runtime"
	"syscall"
	"testing"
	"time"

	"ytmgo/internal/accountsession"
	"ytmgo/internal/audioquality"
	"ytmgo/internal/catalog"
	"ytmgo/internal/downloads"
	"ytmgo/internal/player"
)

// Use an isolated native key-store entry shared only by the two test
// processes. Neither process reads an existing account or offline library.
type acceptanceDownloadKeys struct {
	nativeAccountSecrets
	name string
}

func (k acceptanceDownloadKeys) Get(_ string, i accountsession.Interaction) ([]byte, error) {
	return k.nativeAccountSecrets.Get(k.name, i)
}
func (k acceptanceDownloadKeys) Set(_ string, b []byte) error {
	return k.nativeAccountSecrets.Set(k.name, b)
}
func (k acceptanceDownloadKeys) Delete(_ string, i accountsession.Interaction) error {
	return k.nativeAccountSecrets.Delete(k.name, i)
}

func TestNativeOfflineDownloadLive(t *testing.T) {
	if os.Getenv("YTMGO_LIVE_OFFLINE_DOWNLOAD") != "1" {
		t.Skip("opt-in public download and native audio acceptance")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 4*time.Minute)
	defer cancel()
	root := os.Getenv("YTMGO_OFFLINE_RESTART_DIR")
	child := root != ""
	if !child {
		root = t.TempDir()
	}
	hash := sha256.Sum256([]byte(root))
	keys := acceptanceDownloadKeys{name: fmt.Sprintf("offline-acceptance-%x", hash[:10])}
	if !child {
		defer func() {
			err := keys.Delete("", accountsession.AllowPrompt)
			if err != nil && err != accountsession.ErrNotFound {
				t.Error("test key cleanup failed")
			}
		}()
	}
	p, err := player.New()
	if err != nil {
		t.Fatal("playback runtime unavailable")
	}
	defer p.Close()
	p.SetVolume(0)
	resolve := p.DownloadSource
	if child {
		resolve = func(context.Context, string, audioquality.Quality) (downloads.Remote, error) {
			t.Error("unexpected network download on restart")
			return downloads.Remote{}, context.Canceled
		}
	}
	m, err := downloads.New(ctx, root, keys, resolve, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	p.AttachDownloads(m)
	const id = "52fNscjkST4"
	if child {
		if runtime.GOOS == "darwin" {
			conn, dialErr := net.DialTimeout("tcp4", "192.0.2.1:443", time.Second)
			if conn != nil {
				conn.Close()
			}
			if !errors.Is(dialErr, syscall.EPERM) && !errors.Is(dialErr, syscall.EACCES) {
				t.Fatal("macOS did not enforce the offline network restriction")
			}
		}
		// This fresh player has no account configuration. Online resolution
		// would wait on its initial account gate until this context expires.
		cold, stop := context.WithTimeout(ctx, 18*time.Second)
		defer stop()
		if !m.Has(id) {
			t.Fatal("completed track missing after process restart")
		}
		if err = p.LoadAt(cold, id, 2*time.Second); err != nil {
			t.Fatalf("offline load: %s", p.Snapshot().Error)
		}
		if err = p.SeekContext(cold, 10*time.Second); err != nil {
			t.Fatal("offline seek failed")
		}
		start := p.Snapshot().Position
		for p.Snapshot().Position < start+250*time.Millisecond {
			select {
			case <-cold.Done():
				t.Fatal("native audio clock failed to advance")
			case <-time.After(50 * time.Millisecond):
			}
		}
		t.Log("fresh process played and sought encrypted audio before account initialization")
		return
	}
	if p.SetAccount(ctx, player.AccountSession{Revision: 1}) != nil {
		t.Fatal("anonymous resolver configuration failed")
	}
	m.Online(true)
	if err = m.Enqueue(downloads.Guest, []catalog.Item{{ID: id, Title: "Offline acceptance", Kind: "song"}}, audioquality.Saver); err != nil {
		t.Fatal(err)
	}
	for !m.Has(id) {
		rows := m.Snapshot()
		if len(rows) > 0 && rows[0].State == downloads.Failed {
			t.Fatalf("download failed: %s", rows[0].Failure)
		}
		select {
		case <-ctx.Done():
			t.Fatal("download timed out")
		case <-m.Changes():
		}
	}
	t.Logf("encrypted download complete: %d audio bytes", m.Snapshot()[0].Bytes)
	m.Close()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("YTMGO_OFFLINE_RESTART_DIR", root)
	args := []string{"-test.run=^TestNativeOfflineDownloadLive$", "-test.v"}
	if runtime.GOOS == "darwin" {
		// FFmpeg needs the app's local bridge. Deny outbound IP traffic for
		// both processes while leaving the machine's connectivity unchanged.
		const offline = `(version 1)(allow default)(deny network-outbound (remote ip "*:*"))(allow network-outbound (remote ip "localhost:*"))`
		args = append([]string{"-p", offline, exe}, args...)
		exe = "/usr/bin/sandbox-exec"
	}
	childCmd := exec.CommandContext(ctx, exe, args...)
	childCmd.Stdout = os.Stdout
	if err = childCmd.Run(); err != nil {
		t.Fatal("offline process acceptance failed")
	}
}
