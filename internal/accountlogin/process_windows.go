//go:build windows

package accountlogin

import (
	"os"
	"os/exec"
	"sync"
	"syscall"
	"unsafe"
)

var (
	jobKernel         = syscall.NewLazyDLL("kernel32.dll")
	createJob         = jobKernel.NewProc("CreateJobObjectW")
	setJobInformation = jobKernel.NewProc("SetInformationJobObject")
	assignJobProcess  = jobKernel.NewProc("AssignProcessToJobObject")
	terminateJob      = jobKernel.NewProc("TerminateJobObject")
)

const (
	jobExtendedLimitInformation = 9
	jobKillOnClose              = 0x2000
	processSetQuota             = 0x0100
)

type jobBasicLimits struct {
	PerProcessUserTimeLimit int64
	PerJobUserTimeLimit     int64
	LimitFlags              uint32
	MinimumWorkingSetSize   uintptr
	MaximumWorkingSetSize   uintptr
	ActiveProcessLimit      uint32
	Affinity                uintptr
	PriorityClass           uint32
	SchedulingClass         uint32
}

type jobExtendedLimits struct {
	Basic                 jobBasicLimits
	IOCounters            [6]uint64
	ProcessMemoryLimit    uintptr
	JobMemoryLimit        uintptr
	PeakProcessMemoryUsed uintptr
	PeakJobMemoryUsed     uintptr
}

// An anonymous, non-inheritable job owns this helper and its descendants. Closing
// the parent or the last job handle terminates them, including forced shutdown.
type processTree struct {
	mu  sync.Mutex
	job syscall.Handle
}

func newProcessTree(_ *exec.Cmd) (*processTree, error) {
	job, _, _ := createJob.Call(0, 0)
	if job == 0 {
		return nil, ErrFailed
	}
	p := &processTree{job: syscall.Handle(job)}
	limits := jobExtendedLimits{}
	limits.Basic.LimitFlags = jobKillOnClose
	ok, _, _ := setJobInformation.Call(job, jobExtendedLimitInformation, uintptr(unsafe.Pointer(&limits)), unsafe.Sizeof(limits))
	if ok == 0 {
		p.close()
		return nil, ErrFailed
	}
	return p, nil
}

func (p *processTree) attach(process *os.Process) error {
	// Assignment precedes the first protocol request. The helper blocks on that
	// request before initializing WebView2, so browser children inherit this job.
	handle, err := syscall.OpenProcess(processSetQuota|syscall.PROCESS_TERMINATE, false, uint32(process.Pid))
	if err != nil {
		return ErrFailed
	}
	defer syscall.CloseHandle(handle)
	ok, _, _ := assignJobProcess.Call(uintptr(p.job), uintptr(handle))
	if ok == 0 {
		return ErrFailed
	}
	return nil
}

func (p *processTree) kill() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.job != 0 {
		_, _, _ = terminateJob.Call(uintptr(p.job), 1)
	}
}

func (p *processTree) close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.job != 0 {
		_ = syscall.CloseHandle(p.job)
		p.job = 0
	}
}
