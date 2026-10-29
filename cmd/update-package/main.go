// update-package signs the final notarized bundle for MyGo's updater protocol.
// The private key is accepted only on stdin; diagnostics never include values.
package main

import (
	"archive/tar"
	"compress/gzip"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
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
	Version   string `json:"version"`
	Notes     string `json:"notes,omitempty"`
	Date      string `json:"date"`
	URL       string `json:"url"`
	Size      int64  `json:"size"`
	Signature string `json:"signature"`
}

var validVersion = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+(?:-[A-Za-z0-9][A-Za-z0-9.-]*)?$`)
var validRepository = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)

func main() {
	app := flag.String("app", "", "final signed and notarized app bundle")
	out := flag.String("out", "dist/macos", "output directory")
	config := flag.String("config", "mygo.json", "public build configuration")
	verify := flag.Bool("verify", false, "verify the published archive with only the public key")
	notes := flag.String("notes", "Native update checks, verified downloads, and restart installation.", "release notes")
	flag.Parse()
	data, err := os.ReadFile(*config)
	var cfg configuration
	if err == nil {
		err = json.Unmarshal(data, &cfg)
	}
	if err == nil && *verify {
		err = verifyPackage(*out, cfg)
	} else if err == nil {
		var private []byte
		private, err = io.ReadAll(io.LimitReader(os.Stdin, 1024))
		if err == nil {
			err = writePackage(*app, *out, cfg, strings.TrimSpace(string(private)), *notes)
		}
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "Signed update packaging failed")
		os.Exit(1)
	}
	fmt.Println("Verified signed darwin-arm64 update package")
}

func writePackage(app, out string, cfg configuration, encodedKey, notes string) error {
	key, err := base64.StdEncoding.DecodeString(encodedKey)
	pub, pubErr := base64.StdEncoding.DecodeString(cfg.Updates.PublicKey)
	if err != nil || pubErr != nil || len(key) != ed25519.PrivateKeySize || len(pub) != ed25519.PublicKeySize {
		return errors.New("invalid signing key")
	}
	derived := ed25519.NewKeyFromSeed(key[:ed25519.SeedSize])
	if !ed25519.PrivateKey(derived).Equal(ed25519.PrivateKey(key)) || !ed25519.PublicKey(pub).Equal(derived.Public()) {
		return errors.New("signing key mismatch")
	}
	if !validVersion.MatchString(cfg.Version) || !validRepository.MatchString(cfg.Updates.GitHub) {
		return errors.New("invalid release configuration")
	}
	info, err := os.Lstat(app)
	if err != nil || !info.IsDir() || !strings.HasSuffix(filepath.Base(app), ".app") {
		return errors.New("invalid app bundle")
	}
	app, err = filepath.Abs(app)
	if err != nil {
		return err
	}
	out, err = filepath.Abs(out)
	if err != nil {
		return err
	}
	if relative, err := filepath.Rel(app, out); err != nil || relative == "." || (relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))) {
		return errors.New("output must be outside the app")
	}
	if err := os.MkdirAll(out, 0o755); err != nil {
		return err
	}
	name := "ytmgo-" + cfg.Version + "-darwin-arm64.tar.gz"
	archive := filepath.Join(out, name)
	temporary := archive + ".partial"
	defer os.Remove(temporary)
	file, err := os.Create(temporary)
	if err != nil {
		return err
	}
	sum := sha256.New()
	err = archiveBundle(io.MultiWriter(file, sum), app)
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	stat, err := os.Stat(temporary)
	if err != nil {
		return err
	}
	signature := ed25519.Sign(derived, sum.Sum(nil))
	if !ed25519.Verify(pub, sum.Sum(nil), signature) {
		return errors.New("signature verification failed")
	}
	m := manifest{Version: cfg.Version, Notes: notes, Date: time.Now().UTC().Format(time.RFC3339),
		URL:  "https://github.com/" + cfg.Updates.GitHub + "/releases/download/v" + cfg.Version + "/" + name,
		Size: stat.Size(), Signature: base64.StdEncoding.EncodeToString(signature)}
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	if err := os.Rename(temporary, archive); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(out, "update-darwin-arm64.json"), append(data, '\n'), 0o644)
}

func verifyPackage(out string, cfg configuration) error {
	if !validVersion.MatchString(cfg.Version) || !validRepository.MatchString(cfg.Updates.GitHub) {
		return errors.New("invalid release configuration")
	}
	data, err := os.ReadFile(filepath.Join(out, "update-darwin-arm64.json"))
	if err != nil {
		return err
	}
	var m manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return err
	}
	name := "ytmgo-" + cfg.Version + "-darwin-arm64.tar.gz"
	if m.Version != cfg.Version || m.URL != "https://github.com/"+cfg.Updates.GitHub+"/releases/download/v"+cfg.Version+"/"+name {
		return errors.New("invalid update manifest")
	}
	pub, err := base64.StdEncoding.DecodeString(cfg.Updates.PublicKey)
	signature, sigErr := base64.StdEncoding.DecodeString(m.Signature)
	if err != nil || sigErr != nil || len(pub) != ed25519.PublicKeySize {
		return errors.New("invalid public key or signature")
	}
	archive, err := os.Open(filepath.Join(out, name))
	if err != nil {
		return err
	}
	defer archive.Close()
	sum := sha256.New()
	size, err := io.Copy(sum, archive)
	if err != nil || size != m.Size || !ed25519.Verify(pub, sum.Sum(nil), signature) {
		return errors.New("archive signature verification failed")
	}
	return nil
}

func archiveBundle(out io.Writer, app string) error {
	gz, err := gzip.NewWriterLevel(out, gzip.BestCompression)
	if err != nil {
		return err
	}
	tw := tar.NewWriter(gz)
	parent := filepath.Dir(app)
	err = filepath.WalkDir(app, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() && !info.IsDir() && info.Mode()&fs.ModeSymlink == 0 {
			return errors.New("unsupported bundle entry")
		}
		link := ""
		if info.Mode()&fs.ModeSymlink != 0 {
			link, err = os.Readlink(path)
			if err != nil || filepath.IsAbs(link) {
				return errors.New("invalid bundle link")
			}
			resolved := filepath.Clean(filepath.Join(filepath.Dir(path), link))
			relative, err := filepath.Rel(app, resolved)
			if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
				return errors.New("bundle link escapes app")
			}
		}
		// TAR link targets use slash-separated paths just like entry names.
		// Readlink returns native separators on Windows.
		header, err := tar.FileInfoHeader(info, filepath.ToSlash(link))
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(parent, path)
		if err != nil {
			return err
		}
		header.Name = filepath.ToSlash(relative)
		if info.IsDir() {
			header.Name += "/"
		}
		header.Uid, header.Gid, header.Uname, header.Gname = 0, 0, "", ""
		if err := tw.WriteHeader(header); err != nil {
			return err
		}
		if info.Mode().IsRegular() {
			file, err := os.Open(path)
			if err != nil {
				return err
			}
			_, err = io.Copy(tw, file)
			file.Close()
			return err
		}
		return nil
	})
	if err != nil {
		tw.Close()
		gz.Close()
		return err
	}
	if err := tw.Close(); err != nil {
		gz.Close()
		return err
	}
	return gz.Close()
}
