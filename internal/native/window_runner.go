package native

import (
	"errors"
	"fmt"
	"io"
	"os/exec"
	"regexp"
	"strings"
	"syscall"
)

// ErrWindowKilled marks the native window process dying from SIGKILL or
// SIGTERM: the user force-quit it, the app is being replaced, or the
// process tree is shutting down. That is an intentional close, not a
// launch failure, and must never surface an error dialog.
var ErrWindowKilled = errors.New("native window closed")

// Keep diagnostic output bounded even if WebKit emits many messages. The same
// text is also streamed into the private log. Session URLs are redacted from
// the returned failure before it is displayed or written to a shareable report.
type windowTail struct{ b []byte }

const maxWindowError = 24 << 10

func (w *windowTail) Write(p []byte) (int, error) {
	n := len(p)
	if n >= maxWindowError {
		w.b = append(w.b[:0], p[n-maxWindowError:]...)
		return n, nil
	}
	if over := len(w.b) + n - maxWindowError; over > 0 {
		copy(w.b, w.b[over:])
		w.b = w.b[:len(w.b)-over]
	}
	w.b = append(w.b, p...)
	return n, nil
}

var sessionURL = regexp.MustCompile(`http://127\.0\.0\.1:[0-9]+/[a-f0-9]{64}/?`)

func redactWindowError(s string) string {
	return sessionURL.ReplaceAllString(s, "[private local interface]")
}
func runWindowCommand(cmd *exec.Cmd, output io.Writer) error {
	tail := &windowTail{}
	// Retain the native error for the visible startup failure report. Previously
	// callers only received "exit status 1", hiding the actual bridge error.
	cmd.Stderr = io.MultiWriter(output, tail)
	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			if ws, ok := exitErr.ProcessState.Sys().(syscall.WaitStatus); ok && ws.Signaled() &&
				(ws.Signal() == syscall.SIGKILL || ws.Signal() == syscall.SIGTERM) {
				return fmt.Errorf("%w (%v)", ErrWindowKilled, err)
			}
		}
		detail := strings.TrimSpace(redactWindowError(string(tail.b)))
		if detail != "" {
			return fmt.Errorf("native window failed: %w\n%s", err, detail)
		}
		return fmt.Errorf("native window failed: %w", err)
	}
	return nil
}
