package accountlogin

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"ytmgo/internal/accountsession"
)

const fixtureMode = "YTMGO_ACCOUNTLOGIN_SYNTHETIC_MODE"
const privateMarker = "synthetic-output-must-never-appear-in-errors"
const fixtureBeacon = "YTMGO_ACCOUNTLOGIN_SYNTHETIC_BEACON"

func TestMain(m *testing.M) {
	if mode := os.Getenv(fixtureMode); mode != "" {
		os.Exit(runFixture(mode))
	}
	os.Exit(m.Run())
}

func syntheticSession() accountsession.Session {
	index, secure := 1, true
	return accountsession.Session{
		AccountIndex: &index,
		Cookies:      []accountsession.Cookie{{Name: "SAPISID", Value: "synthetic-login-fixture", Domain: ".youtube.com", Path: "/", Secure: &secure}},
	}
}

func sessionMessage() []byte {
	data, _ := json.Marshal(struct {
		Version int                    `json:"version"`
		Status  string                 `json:"status"`
		Session accountsession.Session `json:"session"`
	}{1, "session", syntheticSession()})
	return data
}

// The child is this test executable in a fixed synthetic mode. It reads no
// account stores, browser data or general environment and never uses network.
func runFixture(mode string) int {
	if mode == "leaf" {
		path := os.Getenv(fixtureBeacon)
		if path == "" || os.WriteFile(path, []byte(strconv.Itoa(os.Getpid())), 0600) != nil {
			return 30
		}
		// This child does not cooperate with the login protocol; the process
		// group or Job Object must stop it with its parent.
		for {
			time.Sleep(time.Second)
		}
	}
	r := bufio.NewReader(os.Stdin)
	line, err := r.ReadBytes('\n')
	if err != nil {
		return 20
	}
	var config struct {
		Version        int    `json:"version"`
		Title          string `json:"title"`
		TimeoutSeconds int64  `json:"timeoutSeconds"`
	}
	if json.Unmarshal(line, &config) != nil || config.Version != 1 || config.TimeoutSeconds < 1 {
		return 21
	}
	data := sessionMessage()
	switch mode {
	case "family", "family-normal":
		executable, err := os.Executable()
		if err != nil {
			return 31
		}
		child := exec.Command(executable)
		child.Env = append(helperEnvironment(), fixtureMode+"=leaf", fixtureBeacon+"="+os.Getenv(fixtureBeacon))
		if child.Start() != nil {
			return 32
		}
		ready := false
		for range 300 {
			if _, err := os.Stat(os.Getenv(fixtureBeacon)); err == nil {
				ready = true
				break
			}
			time.Sleep(10 * time.Millisecond)
		}
		if !ready {
			_ = child.Process.Kill()
			return 33
		}
		if mode == "family-normal" {
			_, _ = os.Stdout.Write(data)
			return 0
		}
		time.Sleep(20 * time.Second)
	case "valid":
		_, _ = os.Stdout.Write(data)
	case "nonzero":
		_, _ = os.Stdout.Write(data)
		return 22
	case "duplicate":
		_, _ = os.Stdout.Write(append(data, data...))
	case "trailing":
		_, _ = os.Stdout.Write(append(data, []byte(privateMarker)...))
	case "partial":
		_, _ = os.Stdout.Write(data[:len(data)/2])
	case "oversized":
		_, _ = io.WriteString(os.Stdout, strings.Repeat("x", maxWire+1))
		time.Sleep(20 * time.Second)
	case "linger":
		_, _ = os.Stdout.Write(data)
		_ = os.Stdout.Close()
		time.Sleep(20 * time.Second)
	case "delayed":
		_, _ = os.Stdout.Write(data)
		time.Sleep(150 * time.Millisecond)
	case "stubborn":
		time.Sleep(20 * time.Second)
	case "cancel", "focus":
		line, err := r.ReadBytes('\n')
		if mode == "focus" {
			if err != nil || string(line) != "{\"command\":\"focus\"}\n" {
				return 23
			}
			_, _ = os.Stdout.Write(data)
		} else {
			if err == nil && string(line) != "{\"command\":\"cancel\"}\n" {
				return 24
			}
			_, _ = io.WriteString(os.Stdout, `{"version":1,"status":"cancelled"}`)
		}
	case "stderr":
		_, _ = io.WriteString(os.Stderr, privateMarker)
		_, _ = os.Stdout.Write(data)
	default:
		if strings.HasPrefix(mode, "error:") {
			_ = json.NewEncoder(os.Stdout).Encode(map[string]any{"version": 1, "status": "error", "error": strings.TrimPrefix(mode, "error:")})
		} else {
			return 25
		}
	}
	return 0
}

// A helper's browser-like descendant must also end on forced cancellation and
// after normal exit. Checks inspect only a PID created by this synthetic child.
func TestHelperOwnsDescendantLifetime(t *testing.T) {
	for _, mode := range []string{"family", "family-normal"} {
		t.Run(mode, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "synthetic-child.pid")
			t.Setenv(fixtureBeacon, path)
			l := startFixture(t, mode, context.Background(), 10*time.Second)
			var pid int
			for range 400 {
				if data, err := os.ReadFile(path); err == nil {
					pid, _ = strconv.Atoi(string(data))
					if pid > 0 {
						break
					}
				}
				time.Sleep(10 * time.Millisecond)
			}
			if pid <= 0 {
				t.Fatal("synthetic descendant did not start")
			}
			needsKill := true
			t.Cleanup(func() {
				if child, err := os.FindProcess(pid); needsKill && err == nil {
					_ = child.Kill()
					_ = child.Release()
				}
			})
			if mode == "family" {
				l.Cancel()
			}
			_, err := l.Wait()
			if mode == "family" && !errors.Is(err, ErrCancelled) || mode == "family-normal" && err != nil {
				t.Fatal("descendant changed helper outcome")
			}
			for range 300 {
				if !syntheticProcessRunning(pid) {
					needsKill = false
					return
				}
				time.Sleep(10 * time.Millisecond)
			}
			t.Fatal("owned descendant survived helper completion")
		})
	}
}

func startFixture(t *testing.T, mode string, ctx context.Context, timeout time.Duration) *Login {
	t.Helper()
	previous := environmentForHelper
	environmentForHelper = func() []string {
		env := append(helperEnvironment(), fixtureMode+"="+mode)
		if path := os.Getenv(fixtureBeacon); path != "" {
			env = append(env, fixtureBeacon+"="+path)
		}
		return env
	}
	t.Cleanup(func() { environmentForHelper = previous })
	path, err := os.Executable()
	if err != nil {
		t.Fatal("test executable unavailable")
	}
	l, err := Start(ctx, Options{Path: path, Title: "Synthetic login", Timeout: timeout})
	if err != nil {
		t.Fatalf("synthetic helper start: %v", err)
	}
	t.Cleanup(func() { l.Cancel(); _, _ = l.Wait() })
	return l
}

// Environment construction propagates named desktop settings while excluding
// unrelated credentials and dynamic-loader/runtime injection options.
func TestHelperEnvironmentIsAllowlisted(t *testing.T) {
	for _, name := range []string{"DYLD_INSERT_LIBRARIES", "LD_PRELOAD", "RUST_LOG", "NODE_OPTIONS", "SYNTHETIC_PRIVATE_CREDENTIAL"} {
		t.Setenv(name, privateMarker)
	}
	t.Setenv("LANG", "en_US.UTF-8")
	foundLocale := false
	for _, entry := range helperEnvironment() {
		if strings.Contains(entry, privateMarker) {
			t.Fatal("unapproved environment propagated")
		}
		if entry == "LANG=en_US.UTF-8" {
			foundLocale = true
		}
	}
	if !foundLocale {
		t.Fatal("desktop locale omitted")
	}
}

// Invalid configuration is rejected before starting a helper, including titles
// that cannot safely be represented by the native window's protocol.
func TestStartRejectsInvalidConfiguration(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal("test executable unavailable")
	}
	for _, title := range []string{"", "login\nwindow", "login\x00window", strings.Repeat("界", 43), string([]byte{0xff})} {
		if _, err := Start(context.Background(), Options{Path: executable, Title: title}); !errors.Is(err, ErrFailed) {
			t.Fatal("invalid title accepted")
		}
	}
	for _, timeout := range []time.Duration{-time.Second, 1800*time.Second + time.Nanosecond} {
		if _, err := Start(context.Background(), Options{Path: executable, Title: "Login", Timeout: timeout}); !errors.Is(err, ErrFailed) {
			t.Fatal("invalid timeout accepted")
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Start(ctx, Options{Path: executable, Title: "Login"}); !errors.Is(err, ErrCancelled) {
		t.Fatal("cancelled start not rejected")
	}
}

// A full stdin pipe must not block cancellation or leave UI focus callers
// stranded after a helper stops reading control messages.
func TestCancellationReleasesBlockedControlWriter(t *testing.T) {
	l := startFixture(t, "stubborn", context.Background(), 8*time.Second)
	writerDone := make(chan struct{})
	go func() {
		defer close(writerDone)
		for range 100000 {
			if l.Focus() != nil {
				return
			}
		}
	}()
	time.Sleep(40 * time.Millisecond)
	l.Cancel()
	if _, err := l.Wait(); !errors.Is(err, ErrCancelled) {
		t.Fatal("blocked writer changed cancel result")
	}
	select {
	case <-writerDone:
	case <-time.After(time.Second):
		t.Fatal("control writer remained blocked")
	}
}

// A session is usable only after valid output and a successful helper exit;
// repeated Wait calls and focus requests cannot launch or reap another helper.
func TestHelperSessionFocusAndConcurrentWait(t *testing.T) {
	l := startFixture(t, "focus", context.Background(), 5*time.Second)
	if err := l.Focus(); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for range 3 {
		wg.Go(func() {
			session, err := l.Wait()
			if err != nil || session.AccountIndex == nil || *session.AccountIndex != 1 {
				t.Error("synthetic session not delivered")
			}
		})
	}
	wg.Wait()
	if !l.cmd.ProcessState.Exited() || !l.cmd.ProcessState.Success() {
		t.Fatal("helper was not reaped")
	}
}

// Invalid or incomplete output and a failed exit never install session data or
// include any child output in their public errors.
func TestHelperRejectsInvalidOutputAndExit(t *testing.T) {
	for _, mode := range []string{"nonzero", "duplicate", "trailing", "partial", "oversized", "linger", "error:" + privateMarker} {
		t.Run(mode, func(t *testing.T) {
			l := startFixture(t, mode, context.Background(), 8*time.Second)
			started := time.Now()
			session, err := l.Wait()
			if !errors.Is(err, ErrFailed) || len(session.Cookies) != 0 {
				t.Fatal("invalid helper output accepted")
			}
			if strings.Contains(err.Error(), privateMarker) {
				t.Fatal("child output entered error")
			}
			if time.Since(started) > exitGrace+3*time.Second {
				t.Fatal("failed helper was not stopped promptly")
			}
			if l.cmd.ProcessState == nil {
				t.Fatal("failed helper was not reaped")
			}
		})
	}
}

// Receiving JSON is not completion: the login keeps waiting until the child
// closes its output and exits, while stderr remains outside the result channel.
func TestHelperWaitsForExitAndDiscardsStderr(t *testing.T) {
	for _, mode := range []string{"delayed", "stderr"} {
		t.Run(mode, func(t *testing.T) {
			started := time.Now()
			l := startFixture(t, mode, context.Background(), 5*time.Second)
			if _, err := l.Wait(); err != nil {
				t.Fatal(err)
			}
			if mode == "delayed" && time.Since(started) < 130*time.Millisecond {
				t.Fatal("session returned before helper exit")
			}
		})
	}
}

// Cancel is idempotent and nonblocking even when a helper refuses to read its
// control pipe; a deadline is reported separately from user cancellation.
func TestHelperCancellationAndDeadline(t *testing.T) {
	for _, mode := range []string{"cancel", "stubborn", "deadline"} {
		t.Run(mode, func(t *testing.T) {
			timeout := 8 * time.Second
			fixture := mode
			want := ErrCancelled
			if mode == "deadline" {
				fixture, timeout, want = "cancel", 50*time.Millisecond, ErrTimeout
			}
			l := startFixture(t, fixture, context.Background(), timeout)
			started := time.Now()
			if mode != "deadline" {
				l.Cancel()
				l.Cancel()
				if time.Since(started) > 100*time.Millisecond {
					t.Fatal("Cancel blocked UI")
				}
			}
			session, err := l.Wait()
			if !errors.Is(err, want) || len(session.Cookies) != 0 {
				t.Fatal("cancellation classification or session rejection failed")
			}
			if time.Since(started) > exitGrace+3*time.Second || l.cmd.ProcessState == nil {
				t.Fatal("cancelled helper not reaped")
			}
		})
	}
}

// Only the protocol's fixed error categories leave the child boundary.
func TestHelperErrorCategories(t *testing.T) {
	for _, row := range []struct {
		code string
		want error
	}{{"runtime_missing", ErrRuntimeMissing}, {"runtime_outdated", ErrRuntimeOutdated}, {"login_timeout", ErrTimeout}, {"login_failed", ErrFailed}} {
		t.Run(row.code, func(t *testing.T) {
			l := startFixture(t, "error:"+row.code, context.Background(), 5*time.Second)
			if _, err := l.Wait(); !errors.Is(err, row.want) {
				t.Fatal("helper error category lost")
			}
		})
	}
}

// The session schema, envelope and size bounds are enforced before installing
// any credentials, independent of what a native backend reports.
func TestResultValidation(t *testing.T) {
	valid := string(sessionMessage())
	invalid := []string{
		strings.Replace(valid, `"version":1`, `"version":2`, 1),
		strings.Replace(valid, `"status":"session"`, `"status":"unknown"`, 1),
		strings.Replace(valid, `"accountIndex":1`, `"accountIndex":null`, 1),
		strings.Replace(valid, `"domain":".youtube.com"`, `"domain":".google.com"`, 1),
		`{"version":1,"status":"cancelled","session":null}`,
		`{"version":1,"status":"session","error":"login_failed","session":{}}`,
		`{"version":1,"status":"error","error":"login_failed","extra":true}`,
	}
	for _, data := range invalid {
		if _, err := decodeResult([]byte(data), time.Now()); !errors.Is(err, ErrFailed) {
			t.Fatal("invalid protocol result accepted")
		}
	}
}

// Explicit development paths never search PATH; packaged metadata must match
// the running OS and architecture, while its pre-signing digest is informational.
func TestRuntimePathAndManifest(t *testing.T) {
	t.Setenv("YTMGO_LOGIN_HELPER", filepath.Join(t.TempDir(), "missing"))
	if _, err := helperPath(""); !errors.Is(err, ErrRuntimeMissing) {
		t.Fatal("missing helper accepted")
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal("test executable unavailable")
	}
	if got, err := helperPath(executable); err != nil || got != executable {
		t.Fatal("explicit helper path rejected")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "manifest.json")
	for _, row := range []struct {
		platform, arch, name string
		want                 error
	}{
		{runtime.GOOS, runtime.GOARCH, executableName(), nil},
		{"wrong-platform", runtime.GOARCH, executableName(), ErrRuntimeOutdated},
		{runtime.GOOS, "wrong-architecture", executableName(), ErrRuntimeOutdated},
		{runtime.GOOS, runtime.GOARCH, "../different", ErrRuntimeOutdated},
	} {
		data := fmt.Sprintf(`{"schema":1,"platform":%q,"architecture":%q,"executable":%q,"sha256":"informational-before-signing"}`, row.platform, row.arch, row.name)
		if os.WriteFile(path, []byte(data), 0600) != nil {
			t.Fatal("fixture write failed")
		}
		if err := validateManifest(path); !errors.Is(err, row.want) {
			t.Fatal("platform metadata validation failed")
		}
	}
}
