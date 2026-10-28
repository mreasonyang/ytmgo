//go:build !windows

package accountlogin

import (
	"fmt"
	"os"
	"runtime"
	"strings"
	"syscall"
)

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
