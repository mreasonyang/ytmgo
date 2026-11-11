//go:build darwin || linux

package main

import (
	"os"
	"syscall"

	"golang.org/x/sys/unix"
)

func lockArtworkFile(f *os.File) error { return unix.Flock(int(f.Fd()), unix.LOCK_EX) }

func artworkAllocationUnit(path string) (int64, error) {
	var st unix.Statfs_t
	err := unix.Statfs(path, &st)
	return int64(st.Bsize), err
}

func artworkFreeSpace(path string) (uint64, error) {
	var st unix.Statfs_t
	err := unix.Statfs(path, &st)
	return uint64(st.Bavail) * uint64(st.Bsize), err
}

func artworkFileCharge(info os.FileInfo) int64 {
	if st, ok := info.Sys().(*syscall.Stat_t); ok {
		return max(artworkWriteCharge(info.Size()), st.Blocks*512)
	}
	return artworkWriteCharge(info.Size())
}
