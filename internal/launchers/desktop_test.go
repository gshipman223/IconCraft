package launchers

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDesktopRoundTripPreservesExecAndActions(t *testing.T) {
	raw := []byte(strings.Join([]string{
		"[Desktop Entry]",
		"Type=Application",
		"Name=Photo Craft",
		"Name[fr]=Atelier",
		"Exec=/usr/bin/photocraft --flag %u",
		"Icon=photocraft",
		"Comment=A line with\\s spaces",
		"",
		"[Desktop Action New]",
		"Name=New window",
		"Exec=/usr/bin/photocraft --new",
		"",
	}, "\n"))
	e, err := ParseDesktop(raw)
	if err != nil {
		t.Fatal(err)
	}
	if e.Get("Exec") != "/usr/bin/photocraft --flag %u" {
		t.Fatal(e.Get("Exec"))
	}
	if got := ExecBinary(e.Get("Exec")); got != "/usr/bin/photocraft" {
		t.Fatal(got)
	}
	e.Del("Name")
	e.Set("Name", "Photoshop")
	e.Set("Icon", "/home/me/.local/share/IconCraft/launchers/photocraft.png")
	e.Set("X-IconCraft-Managed", "true")
	out, err := ParseDesktop(e.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if out.Get("Name") != "Photoshop" || out.Get("Icon") == "photocraft" {
		t.Fatalf("%q %q", out.Get("Name"), out.Get("Icon"))
	}
	if strings.Contains(string(out.Bytes()), "Name[fr]") {
		t.Fatal("localised name survived a rename")
	}
	if out.Get("Exec") != "/usr/bin/photocraft --flag %u" {
		t.Fatal(out.Get("Exec"))
	}
	action := false
	for _, g := range out.Groups {
		if g.Name == "Desktop Action New" && g.Get("Exec") == "/usr/bin/photocraft --new" {
			action = true
		}
	}
	if !action {
		t.Fatal("action group dropped")
	}
}

func TestDesktopIDUsesSubdirectoryDashes(t *testing.T) {
	id, ok := DesktopID("/usr/share/applications", "/usr/share/applications/kde/org.kde.Dolphin.desktop")
	if !ok || id != "kde-org.kde.Dolphin.desktop" {
		t.Fatal(id, ok)
	}
	if _, ok := DesktopID("/usr/share/applications", "/usr/share/applications/../secret.desktop"); ok {
		t.Fatal("accepted a path that escapes the applications directory")
	}
}

func TestHiddenDesktopIsNotALauncher(t *testing.T) {
	e, err := ParseDesktop([]byte("[Desktop Entry]\nType=Application\nName=Helper\nExec=/bin/true\nNoDisplay=true\n"))
	if err != nil {
		t.Fatal(err)
	}
	if e.TypeOK() {
		t.Fatal("hidden helper listed")
	}
}

func TestResolveIconPrefersLargerThemePNG(t *testing.T) {
	root := t.TempDir()
	small := filepath.Join(root, "icons", "hicolor", "32x32", "apps")
	large := filepath.Join(root, "icons", "hicolor", "256x256", "apps")
	if err := os.MkdirAll(small, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(large, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(small, "demo.png"), []byte("small"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(large, "demo.png"), []byte("large"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := ResolveIcon("demo", []string{root})
	if err != nil || !strings.HasSuffix(got, filepath.Join("256x256", "apps", "demo.png")) {
		t.Fatal(got, err)
	}
	abs := filepath.Join(root, "icons", "hicolor", "32x32", "apps", "demo.png")
	got, err = ResolveIcon(abs, nil)
	if err != nil || got != abs {
		t.Fatal(got, err)
	}
}
