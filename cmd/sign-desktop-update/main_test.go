package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"
)

func TestSignAndVerifyArchivesAndRejectTampering(t *testing.T) {
	pub, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal("fixture_generation_failed")
	}
	cfg := configuration{Version: "0.0.61"}
	cfg.Updates.PublicKey, cfg.Updates.GitHub = base64.StdEncoding.EncodeToString(pub), "mreasonyang/ytmgo"
	for _, platform := range []string{"windows-amd64", "linux-amd64"} {
		t.Run(platform, func(t *testing.T) {
			out := t.TempDir()
			archive := filepath.Join(out, "ytmgo-"+cfg.Version+"-"+platform+".tar.gz")
			if os.WriteFile(archive, []byte("synthetic archive"), 0o600) != nil {
				t.Fatal("fixture_write_failed")
			}
			if process(out, platform, cfg, base64.StdEncoding.EncodeToString(key), false) != nil || process(out, platform, cfg, "", true) != nil {
				t.Fatal("signature_roundtrip_failed")
			}
			if os.WriteFile(archive, []byte("tampered archive"), 0o600) != nil {
				t.Fatal("fixture_write_failed")
			}
			if process(out, platform, cfg, "", true) == nil {
				t.Fatal("changed_archive_accepted")
			}
		})
	}
	if process(t.TempDir(), "../../foreign", cfg, "", false) == nil {
		t.Fatal("invalid_platform_accepted")
	}
	for _, version := range []string{"0.0.061", "0.0.1000", "0.0.61-rc.1", "../0.0.61"} {
		cfg.Version = version
		if _, _, _, err := archiveInfo(t.TempDir(), "windows-amd64", cfg); err == nil || err.Error() != "invalid release target" {
			t.Fatal("invalid_version_accepted")
		}
	}
}
