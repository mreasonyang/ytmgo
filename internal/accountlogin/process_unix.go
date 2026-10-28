//go:build !windows

package accountlogin

import (
	"os"
	"os/exec"
	"sync"
	"syscall"
)

// Every helper starts a new process group. Signals never target a discovered
// process or the application's own group; only this invocation's group is owned.
type processTree struct {
	mu     sync.Mutex
	pid    int
	closed bool
}

func newProcessTree(cmd *exec.Cmd) (*processTree, error) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	return &processTree{}, nil
}

func (p *processTree) attach(process *os.Process) error {
	p.pid = process.Pid
	return nil
}

func (p *processTree) kill() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.closed && p.pid > 0 {
		_ = syscall.Kill(-p.pid, syscall.SIGKILL)
	}
}

func (p *processTree) close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.closed && p.pid > 0 {
		// Also terminate remaining group members after a normal helper exit.
		_ = syscall.Kill(-p.pid, syscall.SIGKILL)
	}
	p.closed = true
}
