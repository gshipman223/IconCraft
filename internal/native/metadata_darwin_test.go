//go:build darwin

package native

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// No icon mutation or user applications are involved in this regression test.
// It needs the real macOS osascript/Foundation bridge, not Node emulation.
func TestMacMetadataXMLBinaryAndInvalid(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	app := filepath.Join(root, "Metadata Test.app")
	if err := os.MkdirAll(filepath.Join(app, "Contents"), 0700); err != nil {
		t.Fatal(err)
	}
	plist := `<?xml version="1.0" encoding="UTF-8"?><plist version="1.0"><dict><key>CFBundleIdentifier</key><string>io.iconcraft.disposable-test</string><key>CFBundleName</key><string>IconCraft Test</string><key>CFBundleExecutable</key><string>IconCraftTest</string><key>CFBundleShortVersionString</key><string>1.0</string></dict></plist>`
	if err := os.WriteFile(filepath.Join(app, "Contents", "Info.plist"), []byte(plist), 0600); err != nil {
		t.Fatal(err)
	}
	m, err := New(filepath.Join(root, "state"))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	if err := m.checkMetadataVariants(ctx, root, app); err != nil {
		t.Fatal(err)
	}
}
