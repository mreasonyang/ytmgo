// sign-desktop-update signs existing Windows/Linux archives on the release Mac.
// The private key is accepted only on stdin and never enters diagnostics.
package main

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

type configuration struct {
	Version string `json:"version"`
	Updates struct {
		PublicKey string `json:"publicKey"`
		GitHub    string `json:"github"`
	} `json:"updates"`
}

type manifest struct {
	Version, Notes, Date, URL, Signature string
	Size                                 int64
}

func archiveInfo(out, platform string, cfg configuration) ([]byte, int64, string, error) {
	if (platform != "windows-amd64" && platform != "linux-amd64") ||
		!regexp.MustCompile(`^0\.6\.(0|[1-9][0-9]{0,2})$`).MatchString(cfg.Version) || cfg.Updates.GitHub != "mreasonyang/ytmgo" {
		return nil, 0, "", errors.New("invalid release target")
	}
	name := "ytmgo-" + cfg.Version + "-" + platform + ".tar.gz"
	f, err := os.Open(filepath.Join(out, name))
	if err != nil {
		return nil, 0, "", err
	}
	defer f.Close()
	hash := sha256.New()
	size, err := io.Copy(hash, f)
	return hash.Sum(nil), size, "https://github.com/mreasonyang/ytmgo/releases/download/v" + cfg.Version + "/" + name, err
}

func process(out, platform string, cfg configuration, encoded string, verify bool) error {
	hash, size, url, err := archiveInfo(out, platform, cfg)
	pub, pubErr := base64.StdEncoding.DecodeString(cfg.Updates.PublicKey)
	if err != nil || pubErr != nil || len(pub) != ed25519.PublicKeySize || size == 0 {
		return errors.New("invalid public configuration or archive")
	}
	path := filepath.Join(out, "update-"+platform+".json")
	if !verify {
		key, err := base64.StdEncoding.DecodeString(strings.TrimSpace(encoded))
		if err != nil || len(key) != ed25519.PrivateKeySize {
			return errors.New("invalid signing key")
		}
		derived := ed25519.NewKeyFromSeed(key[:ed25519.SeedSize])
		if !ed25519.PrivateKey(derived).Equal(ed25519.PrivateKey(key)) || !ed25519.PublicKey(pub).Equal(derived.Public()) {
			return errors.New("signing key mismatch")
		}
		data, err := json.MarshalIndent(map[string]any{
			"version": cfg.Version, "notes": "Native Windows/Linux release with bundled playback and login helpers.",
			"date": time.Now().UTC().Format(time.RFC3339), "url": url, "size": size,
			"signature": base64.StdEncoding.EncodeToString(ed25519.Sign(derived, hash)),
		}, "", "  ")
		if err != nil {
			return err
		}
		if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
			return err
		}
	}
	data, err := os.ReadFile(path)
	var m manifest
	if err != nil || json.Unmarshal(data, &m) != nil || m.Version != cfg.Version || m.URL != url || m.Size != size {
		return errors.New("manifest mismatch")
	}
	sig, err := base64.StdEncoding.DecodeString(m.Signature)
	if err != nil || !ed25519.Verify(pub, hash, sig) {
		return errors.New("signature mismatch")
	}
	return nil
}

func main() {
	out := flag.String("out", "dist/desktop", "release artifact directory")
	platform := flag.String("platform", "", "windows-amd64 or linux-amd64")
	config := flag.String("config", "mygo.json", "public release configuration")
	verify := flag.Bool("verify", false, "verify without reading a private key")
	flag.Parse()
	data, err := os.ReadFile(*config)
	var cfg configuration
	if err == nil {
		err = json.Unmarshal(data, &cfg)
	}
	var key []byte
	if err == nil && !*verify {
		key, err = io.ReadAll(io.LimitReader(os.Stdin, 1024))
	}
	if err == nil {
		err = process(*out, *platform, cfg, string(key), *verify)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "Desktop update signing or verification failed")
		os.Exit(1)
	}
	fmt.Println("Verified signed desktop update archive")
}
