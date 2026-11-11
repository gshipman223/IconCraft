package native

import (
	"context"

	"github.com/gshipman223/IconCraft/internal/core"
)

// Host is the process-wide native layer main talks to. The macOS *Mac
// value already has every method. Linux and Windows provide their own.
type Host interface {
	core.Native
	KeepAgentEnabled() bool
	KeepAgentStatus() KeepAgentStatus
	SetKeepAgent(bool) error
	LoginAgentEnabled() bool
	SetLoginAgent(bool, bool) error
	Window(ctx context.Context, url, icon string, menuOnly bool) error
	NotifyUser(ctx context.Context, title, body string) error
	SelfSignature(ctx context.Context) SignatureInfo
	SelfTest(ctx context.Context) Report
}
