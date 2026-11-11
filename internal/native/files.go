package native

import (
	"errors"
	"os"

	"github.com/gshipman223/IconCraft/internal/core"
)

// writeNoSymlink writes a launcher file and refuses to follow a symlink.
// Flatpak and theme trees publish desktop files as symlinks; those are
// read, never written.
func writeNoSymlink(path string, b []byte, mode os.FileMode) error {
	if fi, err := os.Lstat(path); err == nil && fi.Mode()&os.ModeSymlink != 0 {
		return errors.New("refusing to write a launcher through a symlink")
	}
	return core.AtomicWrite(path, b, mode)
}
