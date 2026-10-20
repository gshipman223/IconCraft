package core

import (
	"bytes"
	"context"
	"image/color"
	"os"
	"path/filepath"
	"testing"
)

func TestMakeICNSIsValidContainer(t *testing.T) {
	b, err := makeICNS(pngBytes(color.RGBA{200, 30, 40, 255}))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(b, []byte("icns")) || len(b) < 100 {
		t.Fatal("bad icns", len(b))
	}
}

func TestRebrandApplyAndRestoreRoundTrip(t *testing.T) {
	e, n, a, asset := fixture(t)
	iconPath := filepath.Join(a.Path, "Contents/Resources/AppIcon.icns")
	before, err := Fingerprint(a)
	if err != nil {
		t.Fatal(err)
	}
	rec, err := e.Rebrand(context.Background(), a.ID, asset.ID, "Adobe Photoshop")
	if err != nil {
		t.Fatal(err)
	}
	if rec.Status != "applied" || n.bundleNames[a.Path] != "Adobe Photoshop" || n.resigns != 1 {
		t.Fatalf("not applied: %+v names=%v resigns=%d", rec, n.bundleNames, n.resigns)
	}
	b, err := os.ReadFile(iconPath)
	if err != nil || !bytes.HasPrefix(b, []byte("icns")) {
		t.Fatal("bundle icon not replaced", err)
	}
	if got := e.RebrandedBundles()[a.BundleID]; got != "Adobe Photoshop" {
		t.Fatal("rebrand not reported", got)
	}
	restored, err := e.RestoreRebrand(context.Background(), a.ID)
	if err != nil || restored.Status != "restored" {
		t.Fatal(restored, err)
	}
	after, err := Fingerprint(a)
	if err != nil || after != before {
		t.Fatal("fingerprint not restored", err)
	}
	if _, err := os.Lstat(iconPath); !os.IsNotExist(err) {
		t.Fatal("created icon file should be removed on restore")
	}
	// Second restore is a clean error, not a forced write.
	if _, err := e.RestoreRebrand(context.Background(), a.ID); err == nil {
		t.Fatal("double restore should fail")
	}
}

func TestRestoreRebrandRefusesChangedApp(t *testing.T) {
	e, _, a, asset := fixture(t)
	if _, err := e.Rebrand(context.Background(), a.ID, asset.ID, "Other Name"); err != nil {
		t.Fatal(err)
	}
	// Simulate an app update changing sealed contents after the rebrand.
	if err := os.WriteFile(filepath.Join(a.Path, "Contents/Info.plist"), []byte("updated-metadata"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := e.RestoreRebrand(context.Background(), a.ID); err == nil {
		t.Fatal("changed app must not be force-restored")
	}
}
