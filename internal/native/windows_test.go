//go:build windows

package native

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gshipman223/IconCraft/internal/core"
	"github.com/gshipman223/IconCraft/internal/launchers"
)

func TestWindowsShortcutRoundTripLeavesTargetAlone(t *testing.T) {
	w := fixtureWindows(t.TempDir())
	if err := w.roundTrip(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestWindowsAllUsersShortcutIsCopiedNotEdited(t *testing.T) {
	root := t.TempDir()
	w := fixtureWindows(root)
	if err := os.MkdirAll(w.CommonStart, 0755); err != nil {
		t.Fatal(err)
	}
	target := `C:\Program Files\Shared\Shared.exe`
	raw, err := launchers.NewShortcut(target).Bytes()
	if err != nil {
		t.Fatal(err)
	}
	common := filepath.Join(w.CommonStart, "Shared.lnk")
	if err = os.WriteFile(common, raw, 0644); err != nil {
		t.Fatal(err)
	}
	icon := filepath.Join(w.Dir, "icon.png")
	if err = os.MkdirAll(w.Dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(icon, windowsPNG(), 0644); err != nil {
		t.Fatal(err)
	}
	if err = w.Set(context.Background(), common, icon); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(common)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(raw, after) {
		t.Fatal("ProgramData shortcut bytes changed")
	}
	copyPath := filepath.Join(w.UserStart, "Shared.lnk")
	copied, err := os.ReadFile(copyPath)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := launchers.ReadShortcut(copied)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.EqualFold(parsed.Target, target) || !w.ourIcon(parsed.Icon) {
		t.Fatalf("copy target %q icon %q", parsed.Target, parsed.Icon)
	}
}

func TestWindowsStoreListIsNotQueriedWhenSkipped(t *testing.T) {
	w := fixtureWindows(t.TempDir())
	called := false
	prev := listStoreApps
	listStoreApps = func(context.Context) ([]storeApp, error) {
		called = true
		return nil, nil
	}
	defer func() { listStoreApps = prev }()
	if _, warns := w.Discover(nil); len(warns) != 0 {
		t.Fatal(warns)
	}
	if called {
		t.Fatal("store listing ran")
	}
}

func TestWindowsEngineApplyAndRename(t *testing.T) {
	root := t.TempDir()
	w := fixtureWindows(root)
	if err := os.MkdirAll(w.UserStart, 0755); err != nil {
		t.Fatal(err)
	}
	target := `D:\Apps\PhotoCraft.exe`
	raw, err := launchers.NewShortcut(target).Bytes()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(w.UserStart, "PhotoCraft.lnk")
	if err = os.WriteFile(path, raw, 0644); err != nil {
		t.Fatal(err)
	}
	before := append([]byte{}, raw...)
	e, err := core.Open(w.Dir, root, w)
	if err != nil {
		t.Fatal(err)
	}
	apps, _, err := e.Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var app core.App
	for _, a := range apps {
		if strings.EqualFold(a.Path, path) {
			app = a
		}
	}
	if app.ID == "" {
		t.Fatalf("missing app in %+v", apps)
	}
	asset, err := e.ImportPNG("n", "t", windowsPNG())
	if err != nil {
		t.Fatal(err)
	}
	rec, err := e.Apply(context.Background(), app.ID, asset.ID)
	if err != nil {
		t.Fatal(err)
	}
	if rec.Status != "applied" {
		t.Fatal(rec.Status, rec.Error)
	}
	got, err := launchers.ReadShortcut(mustRead(t, path))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.EqualFold(got.Target, target) {
		t.Fatal(got.Target)
	}
	renamed, err := e.Rename(context.Background(), app.ID, "Darkroom")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("old shortcut name still present", err)
	}
	if !strings.EqualFold(filepath.Base(renamed.Path), "Darkroom.lnk") {
		t.Fatal(renamed.Path)
	}
	moved, err := launchers.ReadShortcut(mustRead(t, renamed.Path))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.EqualFold(moved.Target, target) {
		t.Fatal(moved.Target)
	}
	_ = before
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
