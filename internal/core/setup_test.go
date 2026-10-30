package core

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"syscall"
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

	n.probeErr = pathErrno(syscall.EPERM)
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

func TestBundleOwnedByRootReadsUID(t *testing.T) {
	dir := t.TempDir()
	if bundleOwnedByRoot(dir) {
		t.Fatal("a directory this process created must not be treated as root-owned")
	}
	if bundleOwnedByRoot(filepath.Join(dir, "missing")) {
		t.Fatal("an unreadable path must not be treated as root-owned")
	}
}

type pathProbe struct {
	*fakeNative
	errFor func(string) error
	seen   []string
}

func (p *pathProbe) WriteProbe(path string) error {
	p.seen = append(p.seen, path)
	if p.errFor != nil {
		return p.errFor(path)
	}
	return nil
}

func twoApps(t *testing.T) (*Engine, *pathProbe, App, App) {
	t.Helper()
	e, n, _, _ := fixture(t)
	root := e.Roots[0]
	makeBundle := func(name, id string) App {
		t.Helper()
		p := filepath.Join(root, name+".app")
		if err := os.MkdirAll(p, 0700); err != nil {
			t.Fatal(err)
		}
		return App{ID: id, Name: name, Path: p, BundleID: "test." + id, Executable: "test"}
	}
	alpha, zulu := makeBundle("Alpha", "alpha"), makeBundle("Zulu", "zulu")
	e.mu.Lock()
	e.Apps = map[string]App{alpha.ID: alpha, zulu.ID: zulu}
	e.mu.Unlock()
	probe := &pathProbe{fakeNative: n}
	e.Native = probe
	return e, probe, alpha, zulu
}

func TestSetupSkipsRootOwnedApps(t *testing.T) {
	e, probe, alpha, zulu := twoApps(t)
	prev := bundleOwnerOverride
	bundleOwnerOverride = func(path string) (uint32, bool) {
		if path == alpha.Path {
			return 0, true
		}
		return 501, true
	}
	t.Cleanup(func() { bundleOwnerOverride = prev })
	probe.errFor = func(path string) error {
		if path == alpha.Path {
			return pathErrno(syscall.EACCES)
		}
		return nil
	}
	check, err := e.CheckAppAccess(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if check.Status != "ready" || check.AppID != zulu.ID {
		t.Fatalf("automatic check should pass on the user-owned app, got %#v", check)
	}
	if len(probe.seen) != 1 || probe.seen[0] != zulu.Path {
		t.Fatalf("root-owned app was probed: %v", probe.seen)
	}
}

func TestSetupDoesNotStopOnPermissionDenied(t *testing.T) {
	e, probe, alpha, zulu := twoApps(t)
	probe.errFor = func(path string) error {
		if path == alpha.Path {
			return pathErrno(syscall.EACCES)
		}
		return nil
	}
	check, err := e.CheckAppAccess(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if check.Status != "ready" || check.AppID != zulu.ID {
		t.Fatalf("EACCES on the first app must not fail the check, got %#v", check)
	}
	if strings.Contains(check.Detail, "App Management") {
		t.Fatalf("success detail must not talk about App Management: %s", check.Detail)
	}
}

func TestSetupStopsOnOperationNotPermitted(t *testing.T) {
	e, probe, alpha, zulu := twoApps(t)
	probe.errFor = func(path string) error {
		if path == alpha.Path {
			return pathErrno(syscall.EPERM)
		}
		return nil
	}
	check, err := e.CheckAppAccess(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if check.Status != "blocked" || check.AppID != alpha.ID || !strings.Contains(check.Detail, "operation not permitted") {
		t.Fatalf("EPERM must block immediately, got %#v", check)
	}
	if len(probe.seen) != 1 || probe.seen[0] != alpha.Path {
		t.Fatalf("probe should have stopped after EPERM, saw %v (zulu %s)", probe.seen, zulu.Path)
	}
}

func TestSetupReportsRootOwnedAppWhenAsked(t *testing.T) {
	e, probe, alpha, _ := twoApps(t)
	prev := bundleOwnerOverride
	bundleOwnerOverride = func(path string) (uint32, bool) {
		if path == alpha.Path {
			return 0, true
		}
		return 501, true
	}
	t.Cleanup(func() { bundleOwnerOverride = prev })
	probe.errFor = func(string) error { return pathErrno(syscall.EACCES) }
	check, err := e.CheckAppAccess(context.Background(), alpha.ID)
	if err != nil {
		t.Fatal(err)
	}
	if check.Status != "unavailable" || !strings.Contains(check.Detail, "owned by root") || strings.Contains(check.Detail, "App Management") {
		t.Fatalf("explicit root-owned app should be reported as not writable, got %#v", check)
	}
}

func TestSetupAllRootOwnedIsUnavailable(t *testing.T) {
	e, probe, _, _ := twoApps(t)
	prev := bundleOwnerOverride
	bundleOwnerOverride = func(string) (uint32, bool) { return 0, true }
	t.Cleanup(func() { bundleOwnerOverride = prev })
	check, err := e.CheckAppAccess(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if check.Status != "unavailable" || !strings.Contains(check.Detail, "owned by root") {
		t.Fatalf("expected the root-owned skip explanation, got %#v", check)
	}
	if len(probe.seen) != 0 {
		t.Fatalf("no root-owned app should be probed automatically, saw %v", probe.seen)
	}
}

func TestSetupRootSkipDoesNotHideARunningUserApp(t *testing.T) {
	e, probe, alpha, zulu := twoApps(t)
	prev := bundleOwnerOverride
	bundleOwnerOverride = func(path string) (uint32, bool) {
		if path == alpha.Path {
			return 0, true
		}
		return 501, true
	}
	t.Cleanup(func() { bundleOwnerOverride = prev })
	e.Native = &runningProbe{pathProbe: probe, running: map[string]bool{zulu.Path: true}}
	check, err := e.CheckAppAccess(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if check.Status != "unknown" || strings.Contains(check.Detail, "owned by root") {
		t.Fatalf("a running user-owned app must stay eligible for a later check, got %#v", check)
	}
	if len(probe.seen) != 0 {
		t.Fatalf("neither the root-owned nor the running app should be probed, saw %v", probe.seen)
	}
}

type runningProbe struct {
	*pathProbe
	running map[string]bool
}

func (p *runningProbe) Running(_ context.Context, path string) (bool, error) {
	return p.running[path], nil
}

func TestSetupReadOnlyIsNotAppManagement(t *testing.T) {
	e, n, a, _ := fixture(t)
	n.probeErr = pathErrno(syscall.EROFS)
	check, err := e.CheckAppAccess(context.Background(), a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if check.Status != "unavailable" || !strings.Contains(check.Detail, "read-only") || strings.Contains(check.Detail, "App Management") {
		t.Fatalf("EROFS must not be reported as App Management, got %#v", check)
	}
}
