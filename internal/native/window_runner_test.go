package native

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

func TestWindowHelperProcess(t *testing.T) {
	mode := os.Getenv("ICONCRAFT_TEST_WINDOW_HELPER")
	if mode == "" {
		t.Skip("subprocess fixture only")
	}
	if mode == "fail" {
		fmt.Fprintln(os.Stderr, "execution error: IconCraft could not open (creating the WebKit view): fixture-native-error")
		os.Exit(1)
	}
	if mode == "hang" {
		// Stay alive until the parent test kills us (a self-kill could race
		// our own exit under load). Exit 3 if that never happens so the
		// parent fails instead of hanging.
		fmt.Fprintln(os.Stderr, windowReadyMarker)
		time.Sleep(30 * time.Second)
		os.Exit(3)
	}
	fmt.Fprintln(os.Stderr, "IconCraft: native window created")
	os.Exit(0)
}
func windowHelper(mode string) *exec.Cmd {
	cmd := exec.Command(os.Args[0], "-test.run=^TestWindowHelperProcess$")
	cmd.Env = append(os.Environ(), "ICONCRAFT_TEST_WINDOW_HELPER="+mode)
	return cmd
}
func TestWindowFailureRetainsNativeErrorNotJustExitStatus(t *testing.T) {
	var log bytes.Buffer
	err := runWindowCommand(windowHelper("fail"), &log)
	if err == nil || !strings.Contains(err.Error(), "fixture-native-error") || !strings.Contains(err.Error(), "creating the WebKit view") {
		t.Fatalf("lost native error: %v", err)
	}
	if !strings.Contains(log.String(), "fixture-native-error") {
		t.Fatal("not logged")
	}
}

const windowReadyMarker = "IconCraft: native window created"

// signalOnMarker is the window log sink for the kill test: the moment the
// helper reports its window, the parent delivers the signal. cmd.Process is
// assigned by Start before the stderr copier that calls Write is launched,
// so reading it here is ordered and race-free.
type signalOnMarker struct {
	cmd  *exec.Cmd
	sig  syscall.Signal
	once sync.Once
	seen bytes.Buffer
	err  error
}

func (k *signalOnMarker) Write(p []byte) (int, error) {
	k.seen.Write(p)
	if bytes.Contains(k.seen.Bytes(), []byte(windowReadyMarker)) {
		k.once.Do(func() { k.err = k.cmd.Process.Signal(k.sig) })
	}
	return len(p), nil
}

func TestWindowKilledBySignalIsCleanCloseNotFailure(t *testing.T) {
	for _, sig := range []syscall.Signal{syscall.SIGKILL, syscall.SIGTERM} {
		cmd := windowHelper("hang")
		killer := &signalOnMarker{cmd: cmd, sig: sig}
		err := runWindowCommand(cmd, killer)
		if killer.err != nil {
			t.Fatalf("%v: could not signal helper: %v", sig, killer.err)
		}
		if !errors.Is(err, ErrWindowKilled) {
			t.Fatalf("%v: killed window should be ErrWindowKilled, got: %v", sig, err)
		}
		if strings.Contains(err.Error(), "native window failed") {
			t.Fatalf("%v: killed window misreported as failure: %v", sig, err)
		}
	}
}

func TestWindowRunnerSuccessfulExit(t *testing.T) {
	if err := runWindowCommand(windowHelper("ok"), io.Discard); err != nil {
		t.Fatal(err)
	}
}
func TestWindowRunnerMissingExecutableIsReported(t *testing.T) {
	cmd := exec.Command(t.TempDir() + "/does-not-exist")
	if err := runWindowCommand(cmd, io.Discard); err == nil {
		t.Fatal("missing window executable silently succeeded")
	}
}
func TestWindowErrorTailIsBoundedAndKeepsMostRecentBytes(t *testing.T) {
	w := &windowTail{}
	big := bytes.Repeat([]byte{'x'}, maxWindowError*3)
	n, err := w.Write(big)
	if n != len(big) || err != nil || len(w.b) != maxWindowError {
		t.Fatal("unbounded")
	}
	_, _ = w.Write([]byte("FINAL-ERROR"))
	if len(w.b) != maxWindowError || !bytes.HasSuffix(w.b, []byte("FINAL-ERROR")) {
		t.Fatal("tail truncated incorrectly")
	}
}
func TestPrivateSessionURLRedactedInFailure(t *testing.T) {
	token := strings.Repeat("a", 64)
	got := redactWindowError("failed at http://127.0.0.1:49152/" + token + "/ then stopped")
	if strings.Contains(got, token) || !strings.Contains(got, "[private local interface]") {
		t.Fatal(got)
	}
}
