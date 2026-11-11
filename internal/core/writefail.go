package core

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"syscall"
)

// writeFailure is the class of a bundle write-probe error.
//
// Go's os.ErrPermission is true for both EPERM and EACCES (syscall.Errno.Is).
// Those are different failures here:
//
//   - EPERM, "operation not permitted": the result this project associates
//     with macOS App Management. That association is the existing model; it
//     has not been re-measured on macOS 27.
//   - EACCES, "permission denied": Unix permissions refused the write, which
//     is typical when the bundle is owned by root or otherwise not writable
//     by this account.
//   - EROFS, "read-only file system": the volume itself is read-only.
//
// EROFS is not an os.ErrPermission error. Callers must use this classifier
// rather than errors.Is(err, os.ErrPermission).
type writeFailure int

const (
	writeFailureOther writeFailure = iota
	writeFailureAppManagement
	writeFailureNotPermitted
	writeFailureReadOnly
)

// bundleOwnerOverride, when non-nil, replaces the lstat owner lookup.
// Tests set it; production leaves it nil. Automatic Setup checks skip
// root-owned bundles so a Unix permission refusal is not reported as
// App Management.
var bundleOwnerOverride func(path string) (uid uint32, ok bool)

func bundleOwnedByRoot(path string) bool {
	uid, ok := bundleOwner(path)
	return ok && uid == 0
}

// AppManagementRefusal reports the EPERM ("operation not permitted") result
// associated with App Management. It is false for EACCES and EROFS.
func AppManagementRefusal(err error) bool {
	return classifyWriteFailure(err) == writeFailureAppManagement
}

func classifyWriteFailure(err error) writeFailure {
	if err == nil {
		return writeFailureOther
	}
	var errno syscall.Errno
	if errors.As(err, &errno) {
		switch errno {
		case syscall.EPERM:
			return writeFailureAppManagement
		case syscall.EACCES:
			return writeFailureNotPermitted
		case syscall.EROFS:
			return writeFailureReadOnly
		}
	}
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "operation not permitted"):
		return writeFailureAppManagement
	case strings.Contains(msg, "read-only file system"):
		return writeFailureReadOnly
	case errors.Is(err, os.ErrPermission) || strings.Contains(msg, "permission denied"):
		return writeFailureNotPermitted
	default:
		return writeFailureOther
	}
}

func writeProbeError(name, path string, err error) error {
	switch classifyWriteFailure(err) {
	case writeFailureAppManagement:
		return fmt.Errorf("macOS refused the change to %s with operation not permitted. IconCraft reports that result as App Management. If IconCraft is off in System Settings → Privacy & Security → App Management, switch it on and try again. If it is already on, that switch is not the remaining cause of this result. Nothing was changed", name)
	case writeFailureNotPermitted:
		if bundleOwnedByRoot(path) {
			return fmt.Errorf("IconCraft cannot change %s because the app is owned by root, so this account cannot add a file inside it (permission denied). Nothing was changed", name)
		}
		return fmt.Errorf("IconCraft cannot change %s because this account is not allowed to add a file inside it (permission denied). Nothing was changed", name)
	case writeFailureReadOnly:
		return fmt.Errorf("IconCraft cannot change %s because the app is on a read-only disk. Nothing was changed", name)
	default:
		return fmt.Errorf("could not write inside %s: %w", name, err)
	}
}

func setupWriteDetail(name, path string, kind writeFailure) (status, detail string) {
	switch kind {
	case writeFailureAppManagement:
		return "blocked", fmt.Sprintf("Test blocked on %s: macOS refused a harmless temporary write inside the app (operation not permitted). IconCraft reports that result as App Management. If IconCraft is off in System Settings → Privacy & Security → App Management, switch it on and check again. If it is already on, that switch is not the remaining cause of this result.", name)
	case writeFailureNotPermitted:
		if bundleOwnedByRoot(path) {
			return "unavailable", fmt.Sprintf("Could not use %s for this test: the app is owned by root, so this account cannot add a file inside it (permission denied).", name)
		}
		return "unavailable", fmt.Sprintf("Could not use %s for this test: this account is not allowed to add a file inside it (permission denied).", name)
	case writeFailureReadOnly:
		return "unavailable", fmt.Sprintf("Could not use %s for this test: the app is on a read-only disk.", name)
	default:
		return "", ""
	}
}
