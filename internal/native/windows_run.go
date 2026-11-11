//go:build windows

package native

import (
	"syscall"
	"unsafe"
)

var (
	advapi32         = syscall.NewLazyDLL("advapi32.dll")
	procRegCreateKey = advapi32.NewProc("RegCreateKeyExW")
	procRegQuery     = advapi32.NewProc("RegQueryValueExW")
	procRegSet       = advapi32.NewProc("RegSetValueExW")
	procRegDelete    = advapi32.NewProc("RegDeleteValueW")
	procRegClose     = advapi32.NewProc("RegCloseKey")
)

const (
	hkeyCurrentUser = 0x80000001
	keyAllAccess    = 0xF003F
	regSZ           = 1
	runSubKey       = `Software\Microsoft\Windows\CurrentVersion\Run`
	runValueName    = "IconCraft"
)

func openRunKey() (syscall.Handle, error) {
	sub, err := syscall.UTF16PtrFromString(runSubKey)
	if err != nil {
		return 0, err
	}
	var key syscall.Handle
	r, _, callErr := procRegCreateKey.Call(
		uintptr(hkeyCurrentUser),
		uintptr(unsafe.Pointer(sub)),
		0, 0, 0,
		uintptr(keyAllAccess),
		0,
		uintptr(unsafe.Pointer(&key)),
		0,
	)
	if r != 0 {
		if callErr != syscall.Errno(0) {
			return 0, callErr
		}
		return 0, syscall.Errno(r)
	}
	return key, nil
}

func closeKey(key syscall.Handle) { _, _, _ = procRegClose.Call(uintptr(key)) }

func queryRunValue() (string, error) {
	key, err := openRunKey()
	if err != nil {
		return "", err
	}
	defer closeKey(key)
	name, err := syscall.UTF16PtrFromString(runValueName)
	if err != nil {
		return "", err
	}
	var kind uint32
	var n uint32
	r, _, _ := procRegQuery.Call(uintptr(key), uintptr(unsafe.Pointer(name)), 0, uintptr(unsafe.Pointer(&kind)), 0, uintptr(unsafe.Pointer(&n)))
	if r != 0 || n < 2 {
		return "", syscall.Errno(r)
	}
	buf := make([]uint16, n/2+1)
	r, _, callErr := procRegQuery.Call(uintptr(key), uintptr(unsafe.Pointer(name)), 0, uintptr(unsafe.Pointer(&kind)), uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&n)))
	if r != 0 {
		if callErr != syscall.Errno(0) {
			return "", callErr
		}
		return "", syscall.Errno(r)
	}
	return syscall.UTF16ToString(buf), nil
}

func setRunValue(value string) error {
	key, err := openRunKey()
	if err != nil {
		return err
	}
	defer closeKey(key)
	name, err := syscall.UTF16PtrFromString(runValueName)
	if err != nil {
		return err
	}
	data, err := syscall.UTF16FromString(value)
	if err != nil {
		return err
	}
	r, _, callErr := procRegSet.Call(uintptr(key), uintptr(unsafe.Pointer(name)), 0, uintptr(regSZ), uintptr(unsafe.Pointer(&data[0])), uintptr(len(data)*2))
	if r != 0 {
		if callErr != syscall.Errno(0) {
			return callErr
		}
		return syscall.Errno(r)
	}
	return nil
}

func deleteRunValue() error {
	key, err := openRunKey()
	if err != nil {
		return err
	}
	defer closeKey(key)
	name, err := syscall.UTF16PtrFromString(runValueName)
	if err != nil {
		return err
	}
	r, _, callErr := procRegDelete.Call(uintptr(key), uintptr(unsafe.Pointer(name)))
	if r != 0 && r != 2 { // 2 = not found
		if callErr != syscall.Errno(0) {
			return callErr
		}
		return syscall.Errno(r)
	}
	return nil
}
