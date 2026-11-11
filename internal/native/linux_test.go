//go:build linux

package native

import (
	"bytes"
	"context"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gshipman223/IconCraft/internal/core"
)

func TestLinuxOverrideDoesNotTouchSystemFile(t *testing.T) {
	root := t.TempDir()
	l := fixtureLinux(root)
	sysApps := filepath.Join(l.DataDirs[0], "applications")
	iconDir := filepath.Join(l.DataDirs[0], "icons", "hicolor", "48x48", "apps")
	if err := os.MkdirAll(sysApps, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(iconDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(iconDir, "photocraft.png"), pngBytes(t, 8, 8), 0644); err != nil {
		t.Fatal(err)
	}
	body := "[Desktop Entry]\nType=Application\nName=PhotoCraft\nExec=/usr/bin/photocraft %u\nIcon=photocraft\nMimeType=image/jpeg;\n"
	sysPath := filepath.Join(sysApps, "photocraft.desktop")
	if err := os.WriteFile(sysPath, []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(sysPath)
	e, err := core.Open(l.Dir, root, l)
	if err != nil {
		t.Fatal(err)
	}
	apps, _, err := e.Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(apps) != 1 || apps[0].Name != "PhotoCraft" || apps[0].Path != sysPath {
		t.Fatalf("scan = %+v", apps)
	}
	asset, err := e.ImportPNG("new", "test", pngBytes(t, 12, 12))
	if err != nil {
		t.Fatal(err)
	}
	rec, err := e.Apply(context.Background(), apps[0].ID, asset.ID)
	if err != nil {
		t.Fatal(err)
	}
	if rec.Status != "applied" {
		t.Fatal(rec.Status, rec.Error)
	}
	after, _ := os.ReadFile(sysPath)
	if !bytes.Equal(before, after) {
		t.Fatal("system desktop file changed")
	}
	user, err := os.ReadFile(filepath.Join(l.UserApps, "photocraft.desktop"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(user), "X-IconCraft-Managed=true") || !strings.Contains(string(user), l.iconPath("photocraft.desktop")) {
		t.Fatalf("override = %s", user)
	}
	renamed, err := e.Rename(context.Background(), apps[0].ID, "Darkroom")
	if err != nil {
		t.Fatal(err)
	}
	if renamed.Path != sysPath || renamed.Name != "Darkroom" {
		t.Fatalf("rename path/name = %s %s", renamed.Path, renamed.Name)
	}
	named, _ := os.ReadFile(filepath.Join(l.UserApps, "photocraft.desktop"))
	if !strings.Contains(string(named), "Name=Darkroom") {
		t.Fatalf("name override = %s", named)
	}
	if again, _ := os.ReadFile(sysPath); !bytes.Equal(before, again) {
		t.Fatal("rename changed the system file")
	}
	if _, err := e.Restore(context.Background(), rec.ID); err != nil {
		t.Fatal(err)
	}
	snap, err := l.Snapshot(sysPath)
	if err != nil {
		t.Fatal(err)
	}
	if snap.Custom() {
		t.Fatal("icon restore left a custom icon")
	}
	restored, err := e.RestoreName(context.Background(), renamed.ID)
	if err != nil {
		t.Fatal(err)
	}
	if restored.Name != "PhotoCraft" {
		t.Fatal(restored.Name)
	}
}

func TestLinuxMimeAndAgentsStayInTempDirs(t *testing.T) {
	root := t.TempDir()
	l := fixtureLinux(root)
	apps := filepath.Join(l.DataDirs[0], "applications")
	if err := os.MkdirAll(apps, 0755); err != nil {
		t.Fatal(err)
	}
	desk := "[Desktop Entry]\nType=Application\nName=Browser\nExec=browser\nMimeType=x-scheme-handler/https;x-scheme-handler/http;\n"
	if err := os.WriteFile(filepath.Join(apps, "browser.desktop"), []byte(desk), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := l.SetDefaultApp(context.Background(), "browser", "browser.desktop"); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(l.MimeApps)
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	if !strings.Contains(text, "x-scheme-handler/https=browser.desktop") || !strings.Contains(text, "x-scheme-handler/http=browser.desktop") {
		t.Fatalf("mimeapps = %s", text)
	}
	called := false
	prev := systemctl
	systemctl = func(args ...string) error {
		called = true
		return nil
	}
	defer func() { systemctl = prev }()
	if err := l.SetKeepAgent(true); err != nil {
		t.Fatal(err)
	}
	if called {
		t.Fatal("systemctl was invoked")
	}
	unit, err := os.ReadFile(filepath.Join(l.SystemdUser, "io.iconcraft.keepapplied.service"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(unit), "--reapply") {
		t.Fatalf("unit = %s", unit)
	}
	if err := l.SetLoginAgent(true, true); err != nil {
		t.Fatal(err)
	}
	item, err := os.ReadFile(l.loginItem())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(item), "--at-login") || !strings.Contains(string(item), "--menu-only") {
		t.Fatalf("autostart = %s", item)
	}
	if err := l.SetLoginAgent(false, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(l.loginItem()); !os.IsNotExist(err) {
		t.Fatal("login item remains", err)
	}
}

func TestLinuxSelfTestUsesDisposableFixtures(t *testing.T) {
	r := (&Linux{}).SelfTest(context.Background())
	if !r.Passed {
		t.Fatalf("%+v", r.Checks)
	}
}

func pngBytes(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	img.Set(0, 0, image.White)
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}
