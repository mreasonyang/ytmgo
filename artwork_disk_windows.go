package main

import (
	"os"
	"unsafe"

	"golang.org/x/sys/windows"
)

func lockArtworkFile(f *os.File) error {
	return windows.LockFileEx(windows.Handle(f.Fd()), windows.LOCKFILE_EXCLUSIVE_LOCK, 0, 1, 0, &windows.Overlapped{})
}

var getArtworkClusterSize = windows.NewLazySystemDLL("kernel32.dll").NewProc("GetDiskFreeSpaceW")

func artworkAllocationUnit(path string) (int64, error) {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return 0, err
	}
	var root [windows.MAX_PATH + 1]uint16
	if err := windows.GetVolumePathName(p, &root[0], uint32(len(root))); err != nil {
		return 0, err
	}
	var sectors, bytesPerSector uint32
	ok, _, err := getArtworkClusterSize.Call(uintptr(unsafe.Pointer(&root[0])), uintptr(unsafe.Pointer(&sectors)), uintptr(unsafe.Pointer(&bytesPerSector)), 0, 0)
	if ok == 0 {
		return 0, err
	}
	return int64(sectors) * int64(bytesPerSector), nil
}

func artworkFreeSpace(path string) (uint64, error) {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return 0, err
	}
	var free uint64
	err = windows.GetDiskFreeSpaceEx(p, &free, nil, nil)
	return free, err
}

func artworkFileCharge(info os.FileInfo) int64 { return artworkWriteCharge(info.Size()) }
