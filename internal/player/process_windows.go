//go:build windows

package player

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
	"time"
)

func configureChild(cmd *exec.Cmd) {
	const createNoWindow = 0x08000000
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: createNoWindow}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return os.ErrProcessDone
		}
		// The PID belongs to this command. /T also stops its proof/decoder
		// descendants instead of leaving them alive after the parent exits.
		root := os.Getenv("SystemRoot")
		if root == "" {
			return os.ErrNotExist
		}
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		kill := exec.CommandContext(ctx, filepath.Join(root, "System32", "taskkill.exe"),
			"/PID", strconv.Itoa(cmd.Process.Pid), "/T", "/F")
		kill.Env = childEnvironment()
		kill.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: createNoWindow}
		return kill.Run()
	}
	cmd.WaitDelay = 2 * time.Second
}
