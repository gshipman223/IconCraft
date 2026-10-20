package core

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestSetupReviewAndAppAccessCheckPersist(t *testing.T) {
	e, n, a, _ := fixture(t)
	if !e.SetupRequired() {
		t.Fatal("a state written before setup versioning must require the permissions review")
	}
	if err := e.CheckDataWritable(); err != nil {
		t.Fatal(err)
	}
	check, err := e.CheckAppAccess(context.Background(), a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if check.Status != "ready" || check.AppID != a.ID || check.CheckedAt == "" {
		t.Fatalf("expected a recorded successful test, got %#v", check)
	}

	n.probeErr = os.ErrPermission
	check, err = e.CheckAppAccess(context.Background(), a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if check.Status != "blocked" {
		t.Fatalf("permission refusal must be reported as blocked, got %#v", check)
	}

	if err := e.CompleteSetup(); err != nil {
		t.Fatal(err)
	}
	if e.SetupRequired() {
		t.Fatal("completed setup still reports as required")
	}
	reopened, err := Open(e.Dir, filepath.Dir(e.Dir), n)
	if err != nil {
		t.Fatal(err)
	}
	if reopened.SetupRequired() {
		t.Fatal("setup completion was not persisted")
	}
	st := reopened.SnapshotState()
	if st.SetupVersion != SetupVersion || st.SetupDoneAt == "" || st.AppAccess.Status != "blocked" {
		t.Fatalf("setup state was not persisted exactly: %#v", st)
	}
}

func TestSetupCannotCheckUnknownApp(t *testing.T) {
	e, _, _, _ := fixture(t)
	if _, err := e.CheckAppAccess(context.Background(), "missing"); err == nil {
		t.Fatal("an unknown app must not produce a permission result")
	}
}
