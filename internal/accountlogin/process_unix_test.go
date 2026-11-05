//go:build !windows

package accountlogin

import (
	"context"
	"errors"
	"fmt"
	"os"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestHelperSignalCrashRemainsFailure(t *testing.T) {
	for _, signal := range []syscall.Signal{syscall.SIGKILL, syscall.SIGABRT} {
		t.Run(signal.String(), func(t *testing.T) {
			l := startFixture(t, "stubborn", context.Background(), 5*time.Second)
			if err := l.cmd.Process.Signal(signal); err != nil {
				t.Fatal("could not signal synthetic helper")
			}
			session, err := l.Wait()
			if !errors.Is(err, ErrFailed) || len(session.Cookies) != 0 {
				t.Fatal("a signal crash was hidden as cancellation")
			}
		})
	}
}

// Only the known synthetic descendant PID is examined. No argv or environment
// is read. Linux container init may leave an already-dead orphan as a zombie.
func syntheticProcessRunning(pid int) bool {
	if syscall.Kill(pid, 0) != nil {
		return false
	}
	if runtime.GOOS == "linux" {
		stat, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
		if os.IsNotExist(err) {
			return false
		}
		if err == nil {
			tail := string(stat)
			if index := strings.LastIndexByte(tail, ')'); index >= 0 && len(tail) > index+2 {
				return tail[index+2] != 'Z'
			}
		}
	}
	return true
}
