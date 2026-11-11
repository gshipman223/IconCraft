//go:build unix

package core

import (
	"os"
	"syscall"
)

func bundleOwner(path string) (uint32, bool) {
	if bundleOwnerOverride != nil {
		return bundleOwnerOverride(path)
	}
	fi, err := os.Lstat(path)
	if err != nil {
		return 0, false
	}
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok || st == nil {
		return 0, false
	}
	return st.Uid, true
}
