package native

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestNonMacChecksDoNotReportSuccess(t *testing.T) {
	if runtime.GOOS == "darwin" {
		t.Skip("native checks run separately on macOS")
	}
	m, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	r := m.SelfTest(context.Background())
	if r.Passed || len(r.Checks) != 1 || r.Checks[0].Passed {
		t.Fatal("reported macOS testing on a non-Mac", r)
	}
}
func TestBridgeRejectsNonMacExecution(t *testing.T) {
	if runtime.GOOS == "darwin" {
		t.Skip("native bridge requires GUI test session")
	}
	m, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err = m.Normalize(context.Background(), "/tmp/icon.png"); err == nil {
		t.Fatal("native operation unexpectedly succeeded")
	}
}

func TestBridgeUsesExplicitNilAndDictionaryConversion(t *testing.T) {
	text := string(bridge)
	if strings.Contains(text, ".objectForKey(") {
		t.Fatal("the failing raw dictionary method lookup was reintroduced")
	}
	if !strings.Contains(text, "ObjC.deepUnwrap(nativeValue)") || !strings.Contains(text, "value.isNil()") {
		t.Fatal("native dictionary conversion or nil guard is missing")
	}
}

func TestNativeInitializationReplacesStaleBridgeWithoutClearingData(t *testing.T) {
	root := t.TempDir()
	m, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	state := filepath.Join(root, "state.json")
	if err = os.WriteFile(state, []byte("preserve-existing-backups"), 0600); err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(m.Dir, "bridge.js")
	if err = os.WriteFile(script, []byte("old bridge with broken lookup"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = New(root); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(script)
	if err != nil || !bytes.Equal(got, bridge) {
		t.Fatal("new embedded bridge was not installed", err)
	}
	kept, err := os.ReadFile(state)
	if err != nil || string(kept) != "preserve-existing-backups" {
		t.Fatal("application state was changed", err)
	}
}
