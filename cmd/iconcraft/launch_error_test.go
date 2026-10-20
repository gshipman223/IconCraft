package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gshipman223/IconCraft/internal/core"
)

func TestLaunchErrorPreservesExistingUserState(t *testing.T) {
	dir := t.TempDir()
	state := filepath.Join(dir, "state.json")
	original := []byte(`{"schema":1,"records":[{"original":"keep"}]}`)
	if err := os.WriteFile(state, original, 0600); err != nil {
		t.Fatal(err)
	}
	p, err := writeLaunchError(dir, errors.New("fixture-startup-failure"))
	if err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(p)
	if !strings.Contains(string(b), "fixture-startup-failure") || !strings.Contains(string(b), "IconCraft "+core.Version) {
		t.Fatal(string(b))
	}
	kept, _ := os.ReadFile(state)
	if string(kept) != string(original) {
		t.Fatal("modified user recovery state")
	}
	fi, _ := os.Stat(p)
	if fi.Mode().Perm() != 0600 {
		t.Fatal("report permissions", fi.Mode())
	}
}
func TestLaunchErrorStorageFailureDoesNotPanic(t *testing.T) {
	file := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(file, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := writeLaunchError(file, errors.New("oops")); err == nil {
		t.Fatal("expected directory error")
	}
}
func TestLaunchErrorRedactsSessionToken(t *testing.T) {
	token := strings.Repeat("b", 64)
	got := launchErrorText(errors.New("http://127.0.0.1:50001/" + token + "/"))
	if strings.Contains(got, token) {
		t.Fatal("secret in report")
	}
}
func TestCommandLineModesDoNotShowGUIFailureDialog(t *testing.T) {
	for _, arg := range []string{"--version", "-version", "--headless", "--self-test", "--help", "--headless=true"} {
		if !commandLineMode([]string{arg}) {
			t.Errorf("not CLI: %s", arg)
		}
	}
	if commandLineMode(nil) || commandLineMode([]string{"file-version"}) {
		t.Fatal("ordinary launch is CLI")
	}
}
