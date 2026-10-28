//go:build windows

package player

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"testing"
	"time"
)

// Cancelling a runtime command must stop its own child process as well.
func TestWindowsChildCancellationStopsDescendant(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	cmd := childCommand(ctx, executable, "-test.run=TestWindowsPlayerChildFixture", "--", "parent")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		cancel()
		if cmd.ProcessState == nil {
			_ = cmd.Wait()
		}
	}()
	var pid uint32
	if _, err := fmt.Fscan(stdout, &pid); err != nil {
		t.Fatal("fixture did not create its child")
	}
	const synchronize = 0x00100000
	child, err := syscall.OpenProcess(synchronize|syscall.PROCESS_TERMINATE, false, pid)
	if err != nil {
		t.Fatal("could not observe fixture child")
	}
	defer syscall.CloseHandle(child)
	defer syscall.TerminateProcess(child, 1)
	cancel()
	_ = cmd.Wait()
	status, err := syscall.WaitForSingleObject(child, 3000)
	if err != nil || status != syscall.WAIT_OBJECT_0 {
		t.Fatal("cancelled runtime left its descendant running")
	}
}

// This fixture creates only processes owned by the cancellation test.
func TestWindowsPlayerChildFixture(t *testing.T) {
	if len(os.Args) < 2 || os.Args[len(os.Args)-2] != "--" {
		return
	}
	switch os.Args[len(os.Args)-1] {
	case "parent":
		executable, err := os.Executable()
		if err != nil {
			os.Exit(2)
		}
		child := exec.Command(executable, "-test.run=TestWindowsPlayerChildFixture", "--", "child")
		child.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
		if err := child.Start(); err != nil {
			os.Exit(2)
		}
		fmt.Fprintln(os.Stdout, child.Process.Pid)
		time.Sleep(30 * time.Second)
		_ = child.Process.Kill()
		_ = child.Wait()
		os.Exit(0)
	case "child":
		time.Sleep(30 * time.Second)
		os.Exit(0)
	}
}
