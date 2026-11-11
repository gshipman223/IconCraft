//go:build windows

package main

import (
	"os"
	"syscall"
	"unsafe"
)

var (
	kernel32         = syscall.NewLazyDLL("kernel32.dll")
	procLockFileEx   = kernel32.NewProc("LockFileEx")
	procUnlockFileEx = kernel32.NewProc("UnlockFileEx")
)

// lockOverlap must stay alive for the whole process. Windows locks are
// tied to this structure, and the lock file itself stays open in main.
var lockOverlap syscall.Overlapped

func lockExclusive(f *os.File) error {
	const exclusiveNow = 0x00000001 | 0x00000002 // FAIL_IMMEDIATELY | EXCLUSIVE
	r, _, err := procLockFileEx.Call(f.Fd(), exclusiveNow, 0, 1, 0, uintptr(unsafe.Pointer(&lockOverlap)))
	if r == 0 {
		return err
	}
	return nil
}

func unlockExclusive(f *os.File) error {
	r, _, err := procUnlockFileEx.Call(f.Fd(), 0, 1, 0, uintptr(unsafe.Pointer(&lockOverlap)))
	if r == 0 {
		return err
	}
	return nil
}
