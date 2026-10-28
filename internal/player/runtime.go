package player

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"
)

type runtimePaths struct{ worker, node, ffmpeg string }
type media struct {
	URL       string
	Duration  time.Duration
	UserAgent string
	Referer   string
	// A lease belongs to a track, not each of its seek decoders.
	release func()
}

func findRuntime() (runtimePaths, error) {
	var r runtimePaths
	dir := os.Getenv("YTMGO_RUNTIME_DIR")
	if dir == "" {
		exe, err := os.Executable()
		if err != nil {
			return r, errors.New("runtime_missing")
		}
		if filepath.Base(filepath.Dir(exe)) == "MacOS" {
			dir = filepath.Join(filepath.Dir(filepath.Dir(exe)), "Resources", "runtime")
		} else {
			dir = filepath.Join(filepath.Dir(exe), "runtime")
		}
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return r, errors.New("runtime_missing")
	}
	suffix := ""
	if runtime.GOOS == "windows" {
		suffix = ".exe"
	}
	r = runtimePaths{filepath.Join(abs, "worker.mjs"), filepath.Join(abs, "node"+suffix), filepath.Join(abs, "ffmpeg"+suffix)}
	for _, name := range []string{r.node, r.ffmpeg} {
		st, err := os.Stat(name)
		if err != nil || !st.Mode().IsRegular() || runtime.GOOS != "windows" && st.Mode().Perm()&0111 == 0 {
			return runtimePaths{}, errors.New("runtime_missing")
		}
	}
	if st, err := os.Stat(r.worker); err != nil || !st.Mode().IsRegular() {
		return runtimePaths{}, errors.New("runtime_missing")
	}
	return r, nil
}

func parseMedia(data []byte) (media, error) {
	var result struct {
		URL      string            `json:"url"`
		Duration float64           `json:"duration"`
		Headers  map[string]string `json:"http_headers"`
	}
	if json.Unmarshal(data, &result) != nil || !validMediaURL(result.URL) || math.IsNaN(result.Duration) || math.IsInf(result.Duration, 0) || result.Duration <= 0 || result.Duration > 86400 {
		return media{}, errors.New("invalid_media_response")
	}
	agent, referer := result.Headers["User-Agent"], result.Headers["Referer"]
	if !validHeaderValue(agent) || !validHeaderValue(referer) {
		return media{}, errors.New("invalid_media_response")
	}
	return media{URL: result.URL, Duration: time.Duration(result.Duration * float64(time.Second)), UserAgent: agent, Referer: referer}, nil
}

func validHeaderValue(value string) bool {
	if len(value) > 1024 || strings.ContainsAny(value, "'\\") {
		return false
	}
	for _, r := range value {
		if r < 32 || r > 126 {
			return false
		}
	}
	return true
}

var capabilityPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)

// Remote signed URLs stay inside the worker. Go accepts only its private
// loopback capability, with no credentials, query, or redirect target.
func validMediaURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "http" || u.User != nil || u.Hostname() != "127.0.0.1" || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.RawPath != "" {
		return false
	}
	port, err := strconv.Atoi(u.Port())
	return err == nil && port > 0 && port <= 65535 && len(u.Path) == 65 && u.Path[0] == '/' && capabilityPattern.MatchString(u.Path[1:])
}

// No inherited Node hooks or authentication configuration reach helpers.
func childCommand(ctx context.Context, path string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, path, args...)
	cmd.Env = childEnvironment()
	configureChild(cmd)
	return cmd
}

func childEnvironment() []string {
	if runtime.GOOS != "windows" {
		return []string{"PATH=/usr/bin:/bin", "LANG=en_US.UTF-8", "LC_ALL=en_US.UTF-8"}
	}
	// Windows loads system DLLs and resolves its temporary directory through
	// these variables. Do not inherit PATH, Node hooks or authentication data.
	env := []string{"LANG=en_US.UTF-8", "LC_ALL=en_US.UTF-8"}
	for _, name := range []string{"SystemRoot", "WINDIR", "TEMP", "TMP"} {
		if value := os.Getenv(name); value != "" {
			env = append(env, name+"="+value)
		}
	}
	if root := os.Getenv("SystemRoot"); root != "" {
		env = append(env, "PATH="+filepath.Join(root, "System32"))
	}
	return env
}
