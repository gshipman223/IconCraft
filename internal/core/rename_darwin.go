//go:build darwin

package core

import (
	"runtime"
	"syscall"
	"unsafe"
)

func moveExclusive(old, new string) error {
	a, e := syscall.BytePtrFromString(old)
	if e != nil {
		return e
	}
	b, e := syscall.BytePtrFromString(new)
	if e != nil {
		return e
	}
	// Darwin renameatx_np(2), RENAME_EXCL=4, AT_FDCWD=-2.
	// XNU bsd/kern/syscalls.master: syscall 488 takes five arguments.
	_, _, err := syscall.Syscall6(488, ^uintptr(1), uintptr(unsafe.Pointer(a)), ^uintptr(1), uintptr(unsafe.Pointer(b)), 4, 0)
	runtime.KeepAlive(a)
	runtime.KeepAlive(b)
	if err != 0 {
		return err
	}
	return nil
}
