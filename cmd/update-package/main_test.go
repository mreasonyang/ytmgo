package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func fixture(t *testing.T) (string, string, configuration, string) {
	t.Helper()
	root := t.TempDir()
	app, out := filepath.Join(root, "ytmgo.app"), filepath.Join(root, "release")
	for name, data := range map[string]string{"Contents/MacOS/ytmgo": "executable fixture", "Contents/CodeResources": "stapled ticket fixture"} {
		path := filepath.Join(app, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(data), 0o755); err != nil {
			t.Fatal(err)
		}
		// Set the fixture mode explicitly after creation so a restrictive
		// umask cannot remove its executable bits on Unix.
		if err := os.Chmod(path, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink("MacOS/ytmgo", filepath.Join(app, "Contents/current")); err != nil {
		t.Fatal(err)
	}
	pub, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	cfg := configuration{Version: "0.5.0"}
	cfg.Updates.PublicKey, cfg.Updates.GitHub = base64.StdEncoding.EncodeToString(pub), "mreasonyang/ytmgo"
	return app, out, cfg, base64.StdEncoding.EncodeToString(key)
}

func TestPackagePreservesBundleAndRejectsTampering(t *testing.T) {
	app, out, cfg, key := fixture(t)
	if err := writePackage(app, out, cfg, key, "fixture notes"); err != nil {
		t.Fatal(err)
	}
	if err := verifyPackage(out, cfg); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(out, "ytmgo-0.5.0-darwin-arm64.tar.gz"))
	if err != nil {
		t.Fatal(err)
	}
	gz, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	entries := map[string]*tar.Header{}
	contents := map[string]string{}
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		entries[header.Name] = header
		body, err := io.ReadAll(tr)
		if err != nil {
			t.Fatal(err)
		}
		contents[header.Name] = string(body)
		if header.Uid != 0 || header.Gid != 0 || header.Uname != "" || header.Gname != "" {
			t.Fatal("archive leaks ownership")
		}
	}
	if contents["ytmgo.app/Contents/CodeResources"] != "stapled ticket fixture" || contents["ytmgo.app/Contents/MacOS/ytmgo"] != "executable fixture" {
		t.Fatal("bundle contents lost")
	}
	// Unix fixtures have mode 0755; Windows only exposes its supported mode
	// bits. Packaging must preserve the source mode on either platform.
	source, err := os.Stat(filepath.Join(app, "Contents/MacOS/ytmgo"))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := entries["ytmgo.app/Contents/MacOS/ytmgo"].Mode&0o777, int64(source.Mode().Perm()); got != want {
		t.Fatalf("bundle permissions changed: got %04o, want %04o", got, want)
	}
	if entries["ytmgo.app/Contents/current"].Linkname != "MacOS/ytmgo" {
		t.Fatal("bundle symlink lost")
	}
	data[len(data)/2] ^= 1
	if err := os.WriteFile(filepath.Join(out, "ytmgo-0.5.0-darwin-arm64.tar.gz"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	if verifyPackage(out, cfg) == nil {
		t.Fatal("accepted a tampered archive")
	}
}

func TestSigningRejectsWrongKeyAndUnsafeBundle(t *testing.T) {
	for _, test := range []string{"wrong_key", "invalid_key", "escaping_link", "nested_output", "invalid_version"} {
		t.Run(test, func(t *testing.T) {
			app, out, cfg, key := fixture(t)
			switch test {
			case "wrong_key":
				_, other, err := ed25519.GenerateKey(rand.Reader)
				if err != nil {
					t.Fatal(err)
				}
				key = base64.StdEncoding.EncodeToString(other)
			case "invalid_key":
				key = "synthetic-invalid-fixture"
			case "escaping_link":
				if err := os.Symlink("../../outside", filepath.Join(app, "Contents/escape")); err != nil {
					t.Fatal(err)
				}
			case "nested_output":
				out = filepath.Join(app, "output")
			case "invalid_version":
				cfg.Version = "../../bad"
			}
			if writePackage(app, out, cfg, key, "") == nil {
				t.Fatal("accepted invalid package")
			}
			if _, err := os.Stat(filepath.Join(out, "update-darwin-arm64.json")); !os.IsNotExist(err) {
				t.Fatal("published an invalid feed")
			}
		})
	}
}

func TestVerificationRejectsManifestMismatch(t *testing.T) {
	app, out, cfg, key := fixture(t)
	if err := writePackage(app, out, cfg, key, ""); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(out, "update-darwin-arm64.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var m manifest
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatal(err)
	}
	m.Version = "9.9.9"
	data, err = json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	if verifyPackage(out, cfg) == nil {
		t.Fatal("accepted mismatched version")
	}
}
