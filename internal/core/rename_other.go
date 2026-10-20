//go:build !darwin

package core

import (
	"os"
)

func moveExclusive(old, new string) error {
	if _, err := os.Lstat(new); err == nil {
		return os.ErrExist
	} else if !os.IsNotExist(err) {
		return err
	}
	return os.Rename(old, new)
}
