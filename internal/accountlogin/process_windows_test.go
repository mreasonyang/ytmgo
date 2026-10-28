//go:build windows

package accountlogin

import "syscall"

// Wait on the exact synthetic child handle, without inspecting its argv/env.
func syntheticProcessRunning(pid int) bool {
	const synchronize = 0x00100000
	handle, err := syscall.OpenProcess(synchronize, false, uint32(pid))
	if err != nil {
		return false
	}
	defer syscall.CloseHandle(handle)
	state, err := syscall.WaitForSingleObject(handle, 0)
	return err != nil || state == syscall.WAIT_TIMEOUT
}
