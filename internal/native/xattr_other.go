//go:build !darwin

package native

import (
	"context"
	"errors"
	"github.com/gshipman223/IconCraft/internal/core"
)

func (m *Mac) Snapshot(string) (core.Snapshot, error) {
	return core.Snapshot{}, errors.New("macOS extended attributes are unavailable on this operating system")
}
func (m *Mac) Restore(context.Context, string, core.Snapshot) error {
	return errors.New("native restore requires macOS")
}
