package native

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"
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
