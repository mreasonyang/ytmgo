package player

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// Runtime discovery uses the platform's executable names and permission rules.
func TestRuntimePlatformExecutableNames(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("YTMGO_RUNTIME_DIR", dir)
	suffix := ""
	if runtime.GOOS == "windows" {
		suffix = ".exe"
	}
	for _, name := range []string{"ytmgo-playback-host" + suffix, "ffmpeg" + suffix} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("test fixture"), 0700); err != nil {
			t.Fatal(err)
		}
	}
	paths, err := findRuntime()
	if err != nil || filepath.Base(paths.helper) != "ytmgo-playback-host"+suffix || filepath.Base(paths.ffmpeg) != "ffmpeg"+suffix {
		t.Fatalf("platform runtime not located: %v", err)
	}
	if err := os.Remove(paths.helper); err != nil {
		t.Fatal(err)
	}
	if _, err := findRuntime(); err == nil {
		t.Fatal("missing runtime executable accepted")
	}
}

// Helpers receive only the required OS environment, never parent Node hooks.
func TestChildEnvironmentDoesNotInheritHooks(t *testing.T) {
	t.Setenv("NODE_OPTIONS", "test-only-hook")
	t.Setenv("YTMGO_TEST_PARENT_ONLY", "test-only-value")
	cmd := childCommand(context.Background(), "unused-fixture-executable")
	allowed := map[string]bool{"PATH": true, "LANG": true, "LC_ALL": true}
	if runtime.GOOS == "windows" {
		for _, name := range []string{"SystemRoot", "WINDIR", "TEMP", "TMP"} {
			allowed[name] = true
		}
	}
	for _, entry := range cmd.Env {
		name, _, _ := strings.Cut(entry, "=")
		if !allowed[name] {
			t.Fatalf("unexpected inherited variable: %s", name)
		}
	}
	if cmd.Cancel == nil || cmd.SysProcAttr == nil {
		t.Fatal("child cancellation was not configured")
	}
}
