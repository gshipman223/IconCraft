//go:build darwin

package native

import (
	"context"
	"errors"
	"fmt"
	"github.com/gshipman223/IconCraft/internal/core"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"unsafe"
)

const noFollow = 1

func attr(path, name string) ([]byte, error) {
	p, e := syscall.BytePtrFromString(path)
	if e != nil {
		return nil, e
	}
	n, e := syscall.BytePtrFromString(name)
	if e != nil {
		return nil, e
	}
	size, _, er := syscall.Syscall6(syscall.SYS_GETXATTR, uintptr(unsafe.Pointer(p)), uintptr(unsafe.Pointer(n)), 0, 0, 0, noFollow)
	runtime.KeepAlive(p)
	runtime.KeepAlive(n)
	if er == syscall.ENOATTR {
		return nil, nil
	}
	if er != 0 {
		return nil, er
	}
	if size > core.MaxIconBytes {
		return nil, errors.New("extended attribute too large")
	}
	if size == 0 {
		return []byte{}, nil
	}
	b := make([]byte, size)
	got, _, er := syscall.Syscall6(syscall.SYS_GETXATTR, uintptr(unsafe.Pointer(p)), uintptr(unsafe.Pointer(n)), uintptr(unsafe.Pointer(&b[0])), uintptr(len(b)), 0, noFollow)
	runtime.KeepAlive(p)
	runtime.KeepAlive(n)
	runtime.KeepAlive(b)
	if er != 0 {
		return nil, er
	}
	return b[:got], nil
}
func setAttr(path, name string, b []byte) error {
	p, e := syscall.BytePtrFromString(path)
	if e != nil {
		return e
	}
	n, e := syscall.BytePtrFromString(name)
	if e != nil {
		return e
	}
	var ptr uintptr
	if len(b) > 0 {
		ptr = uintptr(unsafe.Pointer(&b[0]))
	}
	_, _, er := syscall.Syscall6(syscall.SYS_SETXATTR, uintptr(unsafe.Pointer(p)), uintptr(unsafe.Pointer(n)), ptr, uintptr(len(b)), 0, noFollow)
	runtime.KeepAlive(p)
	runtime.KeepAlive(n)
	runtime.KeepAlive(b)
	if er != 0 {
		return er
	}
	return nil
}
func removeAttr(path, name string) error {
	p, e := syscall.BytePtrFromString(path)
	if e != nil {
		return e
	}
	n, e := syscall.BytePtrFromString(name)
	if e != nil {
		return e
	}
	_, _, er := syscall.Syscall(syscall.SYS_REMOVEXATTR, uintptr(unsafe.Pointer(p)), uintptr(unsafe.Pointer(n)), noFollow)
	runtime.KeepAlive(p)
	runtime.KeepAlive(n)
	if er != 0 && er != syscall.ENOATTR {
		return er
	}
	return nil
}
func attrs(path string) (map[string][]byte, error) {
	p, e := syscall.BytePtrFromString(path)
	if e != nil {
		return nil, e
	}
	size, _, er := syscall.Syscall6(syscall.SYS_LISTXATTR, uintptr(unsafe.Pointer(p)), 0, 0, noFollow, 0, 0)
	runtime.KeepAlive(p)
	if er != 0 {
		return nil, er
	}
	if size == 0 {
		return nil, nil
	}
	if size > 65536 {
		return nil, errors.New("too many extended attributes")
	}
	b := make([]byte, size)
	got, _, er := syscall.Syscall6(syscall.SYS_LISTXATTR, uintptr(unsafe.Pointer(p)), uintptr(unsafe.Pointer(&b[0])), uintptr(len(b)), noFollow, 0, 0)
	runtime.KeepAlive(p)
	runtime.KeepAlive(b)
	if er != 0 {
		return nil, er
	}
	out := map[string][]byte{}
	total := 0
	for _, name := range strings.Split(string(b[:got]), "\x00") {
		if name == "" {
			continue
		}
		v, err := attr(path, name)
		if err != nil {
			return nil, err
		}
		total += len(v)
		if total > core.MaxIconBytes {
			return nil, errors.New("icon metadata too large")
		}
		out[name] = v
	}
	return out, nil
}
func (m *Mac) Snapshot(path string) (core.Snapshot, error) {
	s := core.Snapshot{}
	if err := core.SafeApp(path); err != nil {
		return s, err
	}
	var err error
	s.FinderInfo, err = attr(path, "com.apple.FinderInfo")
	if err != nil {
		return s, err
	}
	p := filepath.Join(path, "Icon\r")
	fi, err := os.Lstat(p)
	if os.IsNotExist(err) {
		return s, nil
	}
	if err != nil {
		return s, err
	}
	if !fi.Mode().IsRegular() {
		return s, errors.New("unsafe icon file")
	}
	s.Exists = true
	s.Mode = uint32(fi.Mode().Perm())
	s.Data, err = core.ReadLimited(p, core.MaxIconBytes)
	if err != nil {
		return s, err
	}
	s.Attributes, err = attrs(p)
	return s, err
}
func (m *Mac) Restore(ctx context.Context, path string, s core.Snapshot) error {
	if err := core.SafeApp(path); err != nil {
		return err
	}
	p := filepath.Join(path, "Icon\r")
	if s.Exists {
		f, err := os.CreateTemp(path, ".iconcraft-restore-")
		if err != nil {
			if !errors.Is(err, os.ErrPermission) {
				return err
			}
			// App Management protection can refuse temp files inside the
			// bundle while still allowing the icon file itself to be
			// written. Fall back to a direct write rather than failing a
			// rollback that would otherwise leave recovery blocked.
			if werr := os.WriteFile(p, s.Data, 0644); werr != nil {
				return werr
			}
			for k, v := range s.Attributes {
				if aerr := setAttr(p, k, v); aerr != nil {
					return fmt.Errorf("restore attribute %s: %w", k, aerr)
				}
			}
			if cerr := os.Chmod(p, os.FileMode(s.Mode)); cerr != nil {
				return cerr
			}
			f = nil
		}
		if f == nil {
			// Direct write completed; skip the temp-file path below.
			goto finderInfo
		}
		tmp := f.Name()
		defer os.Remove(tmp)
		if _, err = f.Write(s.Data); err != nil {
			f.Close()
			return err
		}
		if err = f.Sync(); err != nil {
			f.Close()
			return err
		}
		if err = f.Close(); err != nil {
			return err
		}
		for k, v := range s.Attributes {
			if err = setAttr(tmp, k, v); err != nil {
				return fmt.Errorf("restore attribute %s: %w", k, err)
			}
		}
		if err = os.Chmod(tmp, os.FileMode(s.Mode)); err != nil {
			return err
		}
		if err = os.Rename(tmp, p); err != nil {
			return err
		}
	} else {
		if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
finderInfo:
	current, err := attr(path, "com.apple.FinderInfo")
	if err != nil {
		return err
	}
	if len(current) == 0 {
		current = make([]byte, 32)
	}
	if len(current) != 32 {
		return errors.New("unexpected FinderInfo size")
	}
	current[8] &= ^byte(4)
	if s.Custom() {
		current[8] |= 4
	}
	allZero := true
	for _, v := range current {
		if v != 0 {
			allZero = false
			break
		}
	}
	if len(s.FinderInfo) == 0 && allZero {
		err = removeAttr(path, "com.apple.FinderInfo")
	} else {
		err = setAttr(path, "com.apple.FinderInfo", current)
	}
	if err != nil {
		return err
	}
	// Finder notification is best-effort; exact on-disk verification is performed by the engine.
	_ = m.Notify(ctx, path)
	return nil
}
