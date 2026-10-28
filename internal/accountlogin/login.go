// Package accountlogin runs the temporary account window through private pipes.
// Neither process output nor execution errors are exposed to callers: both can
// contain authentication data. The helper never receives a stored session.
package accountlogin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"ytmgo/internal/accountsession"
)

var (
	ErrCancelled       = errors.New("account_login_cancelled")
	ErrRuntimeMissing  = errors.New("account_login_runtime_missing")
	ErrRuntimeOutdated = errors.New("account_login_runtime_outdated")
	ErrTimeout         = errors.New("account_login_timeout")
	ErrFailed          = errors.New("account_login_failed")
)

const (
	protocolVersion = 1
	defaultTimeout  = 10 * time.Minute
	exitGrace       = 5 * time.Second
	maxWire         = accountsession.MaxPayload + 1024
)

type Options struct {
	Title   string
	Timeout time.Duration
	// Path explicitly selects a development or acceptance-test helper. Normal
	// packaged launches leave it empty and require matching platform metadata.
	Path string
}

type Login struct {
	ctx        context.Context
	cmd        *exec.Cmd
	tree       *processTree
	stdin      io.WriteCloser
	stdout     io.ReadCloser
	writeMu    sync.Mutex
	mu         sync.Mutex
	stopReason error
	finished   bool
	result     accountsession.Session
	err        error
	done       chan struct{}
	cancel     context.CancelFunc
}

func Start(parent context.Context, options Options) (*Login, error) {
	if parent == nil {
		return nil, ErrFailed
	}
	if err := parent.Err(); err != nil {
		return nil, contextError(err)
	}
	if options.Title == "" || len(options.Title) > 128 || !utf8.ValidString(options.Title) || options.Timeout < 0 {
		return nil, ErrFailed
	}
	for _, r := range options.Title {
		if unicode.IsControl(r) {
			return nil, ErrFailed
		}
	}
	path, err := helperPath(options.Path)
	if err != nil {
		return nil, err
	}
	timeout := options.Timeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	seconds := int64((timeout-1)/time.Second) + 1
	if seconds < 1 || seconds > 1800 {
		return nil, ErrFailed
	}
	initial, err := json.Marshal(struct {
		Version        int    `json:"version"`
		Title          string `json:"title"`
		TimeoutSeconds int64  `json:"timeoutSeconds"`
	}{protocolVersion, options.Title, seconds})
	if err != nil {
		return nil, ErrFailed
	}
	ctx, cancel := context.WithTimeout(parent, timeout)
	cmd := exec.Command(path)
	cmd.Env = environmentForHelper()
	// nil sends stderr directly to the null device. Descendants cannot keep a
	// diagnostic-copy pipe alive after the helper or make Wait block on logs.
	cmd.Stderr = nil
	tree, err := newProcessTree(cmd)
	if err != nil {
		cancel()
		return nil, ErrFailed
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		tree.close()
		cancel()
		return nil, ErrFailed
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		tree.close()
		_ = stdin.Close()
		cancel()
		return nil, ErrFailed
	}
	if err := cmd.Start(); err != nil {
		tree.close()
		_ = stdin.Close()
		_ = stdout.Close()
		cancel()
		return nil, ErrFailed
	}
	if err := tree.attach(cmd.Process); err != nil {
		_ = stdin.Close()
		_ = stdout.Close()
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		tree.close()
		cancel()
		return nil, ErrFailed
	}
	l := &Login{ctx: ctx, cmd: cmd, tree: tree, stdin: stdin, stdout: stdout, done: make(chan struct{}), cancel: cancel}
	go l.run()
	go func() {
		select {
		case <-ctx.Done():
			l.stop(contextError(ctx.Err()))
		case <-l.done:
		}
	}()
	if err := l.write(initial); err != nil {
		l.stop(ErrFailed)
		_, resultErr := l.Wait()
		if resultErr == nil {
			resultErr = ErrFailed
		}
		return nil, resultErr
	}
	return l, nil
}

// The helper needs a desktop session, not the parent application's credentials
// or runtime injection settings. Read only named variables and never report them.
func helperEnvironment() []string {
	names := []string{"LANG", "LANGUAGE", "LC_ALL", "LC_CTYPE", "LC_MESSAGES", "LC_NUMERIC", "LC_TIME", "LC_COLLATE", "LC_MONETARY"}
	switch runtime.GOOS {
	case "windows":
		names = append(names, "SystemRoot", "WINDIR", "USERPROFILE", "LOCALAPPDATA", "APPDATA", "TEMP", "TMP")
	case "darwin":
		names = append(names, "HOME", "USER", "LOGNAME", "TMPDIR", "TMP", "TEMP", "__CF_USER_TEXT_ENCODING")
	default:
		names = append(names, "HOME", "USER", "LOGNAME", "TMPDIR", "TMP", "TEMP", "DISPLAY", "WAYLAND_DISPLAY", "XDG_RUNTIME_DIR", "XDG_CONFIG_HOME", "XDG_CACHE_HOME", "XDG_DATA_HOME", "DBUS_SESSION_BUS_ADDRESS", "XAUTHORITY")
	}
	result := make([]string, 0, len(names)+1)
	for _, name := range names {
		if value, ok := os.LookupEnv(name); ok {
			result = append(result, name+"="+value)
		}
	}
	if runtime.GOOS != "windows" {
		result = append(result, "PATH=/usr/bin:/bin:/usr/sbin:/sbin")
	}
	return result
}

var environmentForHelper = helperEnvironment

func (l *Login) Focus() error {
	return l.write([]byte(`{"command":"focus"}`))
}

// Cancel starts bounded shutdown without blocking the native UI thread.
func (l *Login) Cancel() { l.stop(ErrCancelled) }

// Wait always returns the same terminal result. Only run calls cmd.Wait, so
// cancellation, timeout and concurrent callers cannot reap a process twice.
func (l *Login) Wait() (accountsession.Session, error) {
	<-l.done
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.result, l.err
}

func (l *Login) write(data []byte) error {
	l.writeMu.Lock()
	defer l.writeMu.Unlock()
	l.mu.Lock()
	finished := l.finished
	l.mu.Unlock()
	if finished {
		return ErrCancelled
	}
	if _, err := l.stdin.Write(append(data, '\n')); err != nil {
		return ErrFailed
	}
	return nil
}

func (l *Login) stop(reason error) {
	l.mu.Lock()
	if l.finished || l.stopReason != nil {
		l.mu.Unlock()
		return
	}
	l.stopReason = reason
	l.mu.Unlock()
	go func() {
		_ = l.write([]byte(`{"command":"cancel"}`))
		_ = l.stdin.Close()
	}()
	// A helper that does not read stdin cannot hold cancellation hostage.
	time.AfterFunc(exitGrace, l.kill)
}

func (l *Login) kill() {
	l.mu.Lock()
	finished := l.finished
	l.mu.Unlock()
	if finished {
		return
	}
	_ = l.stdin.Close()
	_ = l.stdout.Close()
	l.tree.kill()
	_ = l.cmd.Process.Kill()
}

func (l *Login) run() {
	data, readErr := io.ReadAll(io.LimitReader(l.stdout, maxWire+1))
	var session accountsession.Session
	var resultErr error
	if readErr != nil || len(data) > maxWire {
		resultErr = ErrFailed
		l.stop(ErrFailed)
	} else {
		session, resultErr = decodeResult(data, time.Now())
	}
	clear(data)
	// EOF alone is insufficient. A helper must release its window and exit;
	// closing stdout while remaining alive is also a protocol failure.
	timer := time.AfterFunc(exitGrace, func() {
		l.stop(ErrFailed)
		l.kill()
	})
	waitErr := l.cmd.Wait()
	timer.Stop()
	l.tree.close()
	_ = l.stdin.Close()
	_ = l.stdout.Close()
	l.mu.Lock()
	if l.stopReason != nil {
		resultErr = l.stopReason
	} else if l.ctx.Err() != nil {
		resultErr = contextError(l.ctx.Err())
	} else if waitErr != nil {
		resultErr = ErrFailed
	}
	if resultErr != nil {
		session = accountsession.Session{}
	}
	l.result, l.err, l.finished = session, resultErr, true
	l.mu.Unlock()
	close(l.done)
	l.cancel()
}

func contextError(err error) error {
	if errors.Is(err, context.DeadlineExceeded) {
		return ErrTimeout
	}
	return ErrCancelled
}

func decodeResult(data []byte, now time.Time) (accountsession.Session, error) {
	var response struct {
		Version int             `json:"version"`
		Status  string          `json:"status"`
		Session json.RawMessage `json:"session,omitempty"`
		Error   string          `json:"error,omitempty"`
	}
	defer func() { clear(response.Session) }()
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if d.Decode(&response) != nil || response.Version != protocolVersion {
		return accountsession.Session{}, ErrFailed
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return accountsession.Session{}, ErrFailed
	}
	switch response.Status {
	case "session":
		if response.Error != "" || len(response.Session) == 0 {
			return accountsession.Session{}, ErrFailed
		}
		session, err := accountsession.Decode(response.Session, now)
		if err != nil {
			return accountsession.Session{}, ErrFailed
		}
		return session, nil
	case "cancelled":
		if len(response.Session) != 0 || response.Error != "" {
			return accountsession.Session{}, ErrFailed
		}
		return accountsession.Session{}, ErrCancelled
	case "error":
		if len(response.Session) != 0 {
			return accountsession.Session{}, ErrFailed
		}
		switch response.Error {
		case "runtime_missing":
			return accountsession.Session{}, ErrRuntimeMissing
		case "runtime_outdated":
			return accountsession.Session{}, ErrRuntimeOutdated
		case "login_timeout":
			return accountsession.Session{}, ErrTimeout
		case "login_failed":
			return accountsession.Session{}, ErrFailed
		}
	}
	return accountsession.Session{}, ErrFailed
}

func helperPath(override string) (string, error) {
	if override == "" {
		override = os.Getenv("YTMGO_LOGIN_HELPER")
	}
	packaged := override == ""
	path := override
	manifestPath := ""
	if packaged {
		executable, err := os.Executable()
		if err != nil {
			return "", ErrRuntimeMissing
		}
		path, manifestPath = packagedPaths(executable)
	}
	path, err := filepath.Abs(path)
	if err != nil {
		return "", ErrRuntimeMissing
	}
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() || (runtime.GOOS != "windows" && info.Mode().Perm()&0111 == 0) {
		return "", ErrRuntimeMissing
	}
	if packaged {
		if err := validateManifest(manifestPath); err != nil {
			return "", err
		}
	}
	return path, nil
}

func packagedPaths(executable string) (helper, manifest string) {
	root := filepath.Dir(executable)
	if runtime.GOOS == "darwin" && filepath.Base(root) == "MacOS" {
		root = filepath.Join(filepath.Dir(root), "Resources")
	}
	root = filepath.Join(root, "login")
	return filepath.Join(root, filepath.FromSlash(packagedExecutable())), filepath.Join(root, "manifest.json")
}

func packagedExecutable() string {
	if runtime.GOOS == "darwin" {
		return "ytmgo Account Login.app/Contents/MacOS/ytmgo-account-login"
	}
	return executableName()
}

func executableName() string {
	if runtime.GOOS == "windows" {
		return "ytmgo-account-login.exe"
	}
	return "ytmgo-account-login"
}

func validateManifest(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return ErrRuntimeMissing
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, 4097))
	if err != nil || len(data) > 4096 {
		return ErrRuntimeOutdated
	}
	var manifest struct {
		Schema          int    `json:"schema"`
		Platform        string `json:"platform"`
		Architecture    string `json:"architecture"`
		Executable      string `json:"executable"`
		SHA256          string `json:"sha256"`
		Target          string `json:"target"`
		CargoLockSHA256 string `json:"cargo_lock_sha256"`
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if d.Decode(&manifest) != nil || manifest.Schema != 1 || manifest.Platform != runtime.GOOS || manifest.Architecture != runtime.GOARCH || manifest.Executable != packagedExecutable() {
		return ErrRuntimeOutdated
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return ErrRuntimeOutdated
	}
	return nil
}
