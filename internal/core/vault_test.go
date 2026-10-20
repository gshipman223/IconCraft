package core

import (
	"context"
	"image/color"
	"os"
	"path/filepath"
	"testing"
)

func TestVaultCapturesOriginalBeforeFirstApply(t *testing.T) {
	e, _, a, asset := fixture(t)
	if _, err := e.Apply(context.Background(), a.ID, asset.ID); err != nil {
		t.Fatal(err)
	}
	v, ok := e.SavedOriginals()[a.BundleID]
	if !ok {
		t.Fatal("no vault entry saved")
	}
	if v.AppName != "PhotoCraft" || v.FolderName != "PhotoCraft" || v.InternalName != "PhotoCraft" {
		t.Fatalf("wrong saved original: %+v", v)
	}
	if v.IconAssetID == "" {
		t.Fatal("original icon was not saved to the vault")
	}
	if _, err := e.AssetPath(v.IconAssetID); err != nil {
		t.Fatal("saved original icon asset is not retrievable:", err)
	}
	// A second skin must never overwrite the saved original.
	other, err := e.ImportPNG("Other", "Fixture", pngBytes(color.RGBA{9, 9, 9, 255}))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.Apply(context.Background(), a.ID, other.ID); err != nil {
		t.Fatal(err)
	}
	if got := e.SavedOriginals()[a.BundleID]; got.IconAssetID != v.IconAssetID || got.AppName != v.AppName {
		t.Fatalf("vault overwritten by later skin: %+v", got)
	}
}

func TestVaultRestoreFinderIconAfterAppChanged(t *testing.T) {
	e, n, a, asset := fixture(t)
	rec, err := e.Apply(context.Background(), a.ID, asset.ID)
	if err != nil {
		t.Fatal(err)
	}
	// Simulate an app update: sealed contents change, strict restore refuses.
	if err := os.WriteFile(filepath.Join(a.Path, "Contents/MacOS/test"), []byte("updated executable"), 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Restore(context.Background(), rec.ID); err == nil {
		t.Fatal("strict restore should refuse a changed app")
	}
	rep, err := e.RestoreFromVault(context.Background(), a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Restored == 0 || n.states[a.Path].Custom() {
		t.Fatalf("vault restore did not rebuild the original icon: %+v custom=%v", rep, n.states[a.Path].Custom())
	}
	if e.hasLiveRecord(a.BundleID) {
		t.Fatal("icon records should be closed after vault restore")
	}
}

func TestVaultRestoreNeverTouchesDuplicateBundleApp(t *testing.T) {
	e, _, a, _ := fixture(t)
	// A second app sharing the same bundle ID (clones legitimately do).
	twinPath := filepath.Join(filepath.Dir(a.Path), "Twin.app")
	if err := os.MkdirAll(filepath.Join(twinPath, "Contents/MacOS"), 0700); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(filepath.Join(twinPath, "Contents/Info.plist"), []byte("fixture-metadata"), 0600)
	_ = os.WriteFile(filepath.Join(twinPath, "Contents/MacOS/test"), []byte("fixture-executable"), 0700)
	twin := App{Name: "Twin", Path: twinPath, BundleID: a.BundleID, Version: "1.0", Executable: "test"}
	twin.ID = appID(twin)
	e.mu.Lock()
	e.Apps[twin.ID] = twin
	e.mu.Unlock()

	renamed, err := e.Rename(context.Background(), a.ID, "Adobe Photoshop")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(renamed.Path, "Contents/MacOS/test"), []byte("updated executable"), 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := e.RestoreFromVault(context.Background(), renamed.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(a.Path); err != nil {
		t.Fatal("selected app not renamed back to its saved name:", err)
	}
	if _, err := os.Lstat(twinPath); err != nil {
		t.Fatal("the duplicate-bundle app must never be touched:", err)
	}
}

func TestVaultRestoreRebrandAfterAppChanged(t *testing.T) {
	e, n, a, asset := fixture(t)
	rec, err := e.Rebrand(context.Background(), a.ID, asset.ID, "Adobe Photoshop")
	if err != nil {
		t.Fatal(err)
	}
	if rec.OriginalName != "PhotoCraft" {
		t.Fatalf("original internal name not journalled: %+v", rec)
	}
	// Simulate an update replacing the plist (fingerprint no longer matches).
	if err := os.WriteFile(filepath.Join(a.Path, "Contents/Info.plist"), []byte("updated-metadata"), 0600); err != nil {
		t.Fatal(err)
	}
	rep, err := e.RestoreFromVault(context.Background(), a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Restored == 0 {
		t.Fatalf("nothing restored: %+v", rep)
	}
	if got := n.bundleNames[a.Path]; got != "PhotoCraft" {
		t.Fatalf("internal name not restored from vault, got %q", got)
	}
	if _, err := os.Lstat(filepath.Join(a.Path, "Contents/Resources/AppIcon.icns")); !os.IsNotExist(err) {
		t.Fatal("created icon file should be removed on vault restore")
	}
	state := e.SnapshotState()
	for _, r := range state.Rebrands {
		if r.Status == "applied" {
			t.Fatal("rebrand still marked applied")
		}
	}
}

func TestVaultRestoreFolderNameAfterAppChanged(t *testing.T) {
	e, _, a, _ := fixture(t)
	renamed, err := e.Rename(context.Background(), a.ID, "Adobe Photoshop")
	if err != nil {
		t.Fatal(err)
	}
	if v := e.SavedOriginals()[a.BundleID]; v.FolderName != "PhotoCraft" || v.AppName != "PhotoCraft" {
		t.Fatalf("original name not saved before rename: %+v", v)
	}
	// App updates in place: strict rename-restore refuses on fingerprint.
	if err := os.WriteFile(filepath.Join(renamed.Path, "Contents/MacOS/test"), []byte("updated executable"), 0700); err != nil {
		t.Fatal(err)
	}
	rep, err := e.RestoreFromVault(context.Background(), renamed.ID)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Restored == 0 {
		t.Fatalf("nothing restored: %+v", rep)
	}
	if _, err := os.Lstat(a.Path); err != nil {
		t.Fatal("folder not renamed back to the saved original:", err)
	}
	cur, ok := e.appByBundle(a.BundleID)
	if !ok || cur.Name != "PhotoCraft" || cur.Path != a.Path {
		t.Fatalf("app not re-linked under its original identifier: %+v ok=%v", cur, ok)
	}
}
