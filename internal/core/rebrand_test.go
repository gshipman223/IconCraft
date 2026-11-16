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
	plistPath := filepath.Join(a.Path, "Contents/Info.plist")
	exePath := filepath.Join(a.Path, "Contents/MacOS", a.Executable)
	beforePlist, err := os.ReadFile(plistPath)
	if err != nil {
		t.Fatal(err)
	}
	beforeExe, err := os.ReadFile(exePath)
	if err != nil {
		t.Fatal(err)
	}
	before, err := Fingerprint(a)
	if err != nil {
		t.Fatal(err)
	}
	rec, err := e.Rebrand(context.Background(), a.ID, asset.ID, "Adobe Photoshop")
	if err != nil {
		t.Fatal(err)
	}
	if rec.Status != "external" || n.resigns != 0 || len(n.bundleNames) != 0 {
		t.Fatalf("rebrand wrote inside the bundle: %+v names=%v resigns=%d", rec, n.bundleNames, n.resigns)
	}
	if !n.states[a.Path].Custom() {
		t.Fatal("Finder icon was not applied")
	}
	gotPlist, _ := os.ReadFile(plistPath)
	gotExe, _ := os.ReadFile(exePath)
	if !bytes.Equal(gotPlist, beforePlist) || !bytes.Equal(gotExe, beforeExe) {
		t.Fatal("Info.plist or executable changed")
	}
	after, err := Fingerprint(a)
	if err != nil || after != before {
		t.Fatal("fingerprint changed", err)
	}
	if got := e.RebrandedBundles()[a.BundleID]; got != "" {
		t.Fatal("external customisation was recorded as an internal skin", got)
	}
	if rec.App.Path != a.Path || rec.App.Name != "Adobe Photoshop" {
		t.Fatalf("label did not stick on the original path: %+v", rec.App)
	}
	// There is no internal skin to undo.
	if _, err := e.RestoreRebrand(context.Background(), a.ID); err == nil {
		t.Fatal("restore of an untouched bundle should fail")
	}
}

func TestRestoreRebrandRefusesChangedApp(t *testing.T) {
	e, n, a, _ := fixture(t)
	original, err := os.ReadFile(filepath.Join(a.Path, "Contents/Info.plist"))
	if err != nil {
		t.Fatal(err)
	}
	fp, err := Fingerprint(a)
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(e.Dir, "rebrands", "old-skin")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "Info.plist"), original, 0600); err != nil {
		t.Fatal(err)
	}
	e.mu.Lock()
	e.State.Rebrands = append(e.State.Rebrands, RebrandRecord{
		ID: "old-skin", BundleID: a.BundleID, App: a, Status: "applied", AfterPrint: fp,
		IconFile: "AppIcon.icns", Name: "Other Name", OriginalName: "PhotoCraft",
	})
	e.mu.Unlock()
	updated := []byte("updated-metadata")
	if err := os.WriteFile(filepath.Join(a.Path, "Contents/Info.plist"), updated, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := e.RestoreRebrand(context.Background(), a.ID); err == nil {
		t.Fatal("changed app must not be force-restored")
	}
	got, err := os.ReadFile(filepath.Join(a.Path, "Contents/Info.plist"))
	if err != nil || !bytes.Equal(got, updated) {
		t.Fatal("restore wrote into the updated bundle", err)
	}
	if n.resigns != 0 {
		t.Fatal("restore re-signed an updated bundle")
	}
}
