package core

import (
	"errors"
	"os"
	"syscall"
)

// windowsInvalidName reports a path Windows rejects as a file name.
// Finder's custom-icon file is named with a trailing carriage return,
// which Windows will not stat. That is the same as the file not existing.
func windowsInvalidName(err error) bool {
	var pe *os.PathError
	if !errors.As(err, &pe) {
		return false
	}
	errno, ok := pe.Err.(syscall.Errno)
	return ok && errno == 123 // ERROR_INVALID_NAME
}
