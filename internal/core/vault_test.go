package core

import (
	"bytes"
	"context"
	"encoding/binary"
	"image"
	"image/color"
	"image/png"
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

// TestVaultBackfillIconFromRebrandJournal reproduces the reported bug: an
// app first full-skinned before the vault saved icons has a vault entry
// with a name but no icon, so the "Originally …" row showed a placeholder.
// The original icon must be recovered from the full-skin journal's backup.
func TestVaultBackfillIconFromRebrandJournal(t *testing.T) {
	e, _, a, asset := fixture(t)
	if _, err := e.Rebrand(context.Background(), a.ID, asset.ID, "Adobe Photoshop"); err != nil {
		t.Fatal(err)
	}
	// The fixture app has no bundle icon file until the rebrand creates
	// one, so build the journal by hand: a pristine .icns in Contents, then
	// a rebrand — its journal now holds the true original icon.
	p := filepath.Join(a.Path, "Contents/Resources/AppIcon.icns")
	origPNG := pngBytes(color.RGBA{7, 77, 177, 255})
	icns := []byte("icns")
	var sz [4]byte
	binary.BigEndian.PutUint32(sz[:], uint32(8+8+len(origPNG)))
	icns = append(icns, sz[:]...)
	icns = append(icns, []byte("ic09")...)
	binary.BigEndian.PutUint32(sz[:], uint32(8+len(origPNG)))
	icns = append(icns, sz[:]...)
	icns = append(icns, origPNG...)
	if err := os.WriteFile(p, icns, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Rebrand(context.Background(), a.ID, asset.ID, "Adobe Photoshop"); err != nil {
		t.Fatal(err)
	}
	// Simulate the pre-vault state: entry exists, icon never saved.
	e.mu.Lock()
	v := e.State.Vault[a.BundleID]
	v.IconAssetID = ""
	e.State.Vault[a.BundleID] = v
	e.mu.Unlock()
	e.BackfillVaultIcons(context.Background())
	got := e.SavedOriginals()[a.BundleID]
	if got.IconAssetID == "" {
		t.Fatal("backfill did not recover the original icon")
	}
	ap, err := e.AssetPath(got.IconAssetID)
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(ap)
	if err != nil || !bytes.Equal(b, origPNG) {
		t.Fatalf("recovered icon is not the original artwork: %v", err)
	}
}

// TestVaultBackfillNeverStealsSkin: an app still wearing a skin, with no
// journal icon, must NOT have its current (skin) icon saved as "original".
func TestVaultBackfillNeverStealsSkin(t *testing.T) {
	e, _, a, asset := fixture(t)
	if _, err := e.Apply(context.Background(), a.ID, asset.ID); err != nil {
		t.Fatal(err)
	}
	e.mu.Lock()
	v := e.State.Vault[a.BundleID]
	v.IconAssetID = ""
	e.State.Vault[a.BundleID] = v
	e.mu.Unlock()
	// The live Finder-icon record blocks the export fallback: the app's
	// current on-disk appearance is the skin, never the original.
	e.BackfillVaultIcons(context.Background())
	if got := e.SavedOriginals()[a.BundleID]; got.IconAssetID != "" {
		t.Fatalf("backfill captured the skin as the original: %+v", got)
	}
}

func TestICNSToPNGExtractsLargest(t *testing.T) {
	small := pngBytes(color.RGBA{1, 1, 1, 255})
	// A gradient so the "big" payload is genuinely larger than the solid one.
	im := image.NewRGBA(image.Rect(0, 0, 64, 64))
	for y := 0; y < 64; y++ {
		for x := 0; x < 64; x++ {
			im.SetRGBA(x, y, color.RGBA{uint8(x * 4), uint8(y * 4), 128, 255})
		}
	}
	var bigBuf bytes.Buffer
	if err := png.Encode(&bigBuf, im); err != nil {
		t.Fatal(err)
	}
	big := bigBuf.Bytes()
	mk := func(els ...[]byte) []byte {
		out := []byte("icns")
		total := 8
		for _, el := range els {
			total += 8 + len(el)
		}
		var sz [4]byte
		binary.BigEndian.PutUint32(sz[:], uint32(total))
		out = append(out, sz[:]...)
		for i, el := range els {
			out = append(out, []byte("ic0x")...)
			out[len(out)-1] = byte('0' + i)
			binary.BigEndian.PutUint32(sz[:], uint32(8+len(el)))
			out = append(out, sz[:]...)
			out = append(out, el...)
		}
		return out
	}
	got, ok := icnsToPNG(mk([]byte("not-a-png"), small, big))
	if !ok || !bytes.Equal(got, big) {
		t.Fatalf("icnsToPNG picked the wrong payload: ok=%v", ok)
	}
	if _, ok = icnsToPNG([]byte("icns")); ok {
		t.Fatal("empty icns must not parse")
	}
	if _, ok := asPNG(small); !ok {
		t.Fatal("plain PNG must pass through asPNG")
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
