//go:build !darwin

package native

import "errors"

func (m *Mac) KeepAgentEnabled() bool { return false }

func (m *Mac) SetKeepAgent(bool) error {
	return errors.New("the keep-applied agent requires macOS")
}
