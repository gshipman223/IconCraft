package core

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// artCraftSees mirrors storytold/craft-launcher crates/native/src/detect.rs
// on macOS: a real directory named <official>.app whose CFBundleIdentifier
// is one of the accepted ids. It does not look at the folder's custom
// icon, CFBundleName, or the code signature. A symlink is not accepted.
func artCraftSees(dir, official string, ids []string) bool {
	p := filepath.Join(dir, official+".app")
	fi, err := os.Lstat(p)
	if err != nil || !fi.IsDir() || fi.Mode()&os.ModeSymlink != 0 {
		return false
	}
	got := cfBundleID(p)
	for _, id := range ids {
		if got == id {
			return true
		}
	}
	return false
}

func cfBundleID(app string) string {
	b, err := os.ReadFile(filepath.Join(app, "Contents/Info.plist"))
	if err != nil {
		return ""
	}
	const key = "<key>CFBundleIdentifier</key>"
	i := strings.Index(string(b), key)
	if i < 0 {
		return ""
	}
	rest := string(b)[i+len(key):]
	s := strings.Index(rest, "<string>")
	e := strings.Index(rest, "</string>")
	if s < 0 || e < 0 || e < s {
		return ""
	}
	return rest[s+len("<string>") : e]
}

func writeIdentityPlist(t *testing.T, app, id, name string) {
	t.Helper()
	body := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<plist version="1.0"><dict>
<key>CFBundleIdentifier</key><string>%s</string>
<key>CFBundleName</key><string>%s</string>
<key>CFBundleDisplayName</key><string>%s</string>
</dict></plist>
`, id, name, name)
	if err := os.WriteFile(filepath.Join(app, "Contents/Info.plist"), []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestArtCraftStillSeesCustomisedApp(t *testing.T) {
	e, n, a, asset := fixture(t)
	writeIdentityPlist(t, a.Path, a.BundleID, "PhotoCraft")
	codesign := filepath.Join(a.Path, "Contents/_CodeSignature")
	if err := os.MkdirAll(codesign, 0700); err != nil {
		t.Fatal(err)
	}
	resources := []byte("sealed-resources")
	if err := os.WriteFile(filepath.Join(codesign, "CodeResources"), resources, 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := e.Scan(context.Background()); err != nil {
		t.Fatal(err)
	}
	a, err := e.GetApp(a.ID)
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Dir(a.Path)
	ids := []string{a.BundleID}
	if !artCraftSees(dir, "PhotoCraft", ids) {
		t.Fatal("fixture is not visible to a path-and-bundle-id detector")
	}
	before, err := os.ReadFile(filepath.Join(a.Path, "Contents/Info.plist"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.Rename(context.Background(), a.ID, "Adobe Photoshop"); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Apply(context.Background(), a.ID, asset.ID); err != nil {
		t.Fatal(err)
	}
	if !artCraftSees(dir, "PhotoCraft", ids) {
		t.Fatal("custom name and Finder icon hid the app from the launcher")
	}
	if _, err := os.Lstat(filepath.Join(dir, "Adobe Photoshop.app")); !os.IsNotExist(err) {
		t.Fatal("a second app folder was created")
	}
	got, err := os.ReadFile(filepath.Join(a.Path, "Contents/Info.plist"))
	if err != nil || string(got) != string(before) || !strings.Contains(string(got), "<string>PhotoCraft</string>") {
		t.Fatal("Info.plist changed", err)
	}
	sealed, err := os.ReadFile(filepath.Join(codesign, "CodeResources"))
	if err != nil || string(sealed) != string(resources) {
		t.Fatal("code signature resources changed", err)
	}
	if n.resigns != 0 || len(n.bundleNames) != 0 {
		t.Fatal("bundle was re-signed or renamed internally")
	}
	// The same helper rejects the situation IconCraft used to create.
	if err := os.Rename(a.Path, filepath.Join(dir, "Adobe Photoshop.app")); err != nil {
		t.Fatal(err)
	}
	if artCraftSees(dir, "PhotoCraft", ids) {
		t.Fatal("detector should miss an app whose folder was renamed")
	}
	link := filepath.Join(dir, "PhotoCraft.app")
	if err := os.Symlink(filepath.Join(dir, "Adobe Photoshop.app"), link); err != nil {
		t.Fatal(err)
	}
	if artCraftSees(dir, "PhotoCraft", ids) {
		t.Fatal("detector should reject a symlink at the official path")
	}
}

func TestRepairMovesLegacyRenameBack(t *testing.T) {
	e, _, a, _ := fixture(t)
	custom := filepath.Join(filepath.Dir(a.Path), "Photoshop.app")
	if err := os.Rename(a.Path, custom); err != nil {
		t.Fatal(err)
	}
	fp, err := Fingerprint(App{Path: custom, Executable: a.Executable})
	if err != nil {
		t.Fatal(err)
	}
	e.mu.Lock()
	e.State.Renames = append(e.State.Renames, RenameRecord{
		App: a, ID: "legacy", BundleID: a.BundleID, From: a.Path, To: custom,
		OldName: "PhotoCraft", NewName: "Photoshop", Fingerprint: fp, Status: "applied",
	})
	if err := e.save(); err != nil {
		e.mu.Unlock()
		t.Fatal(err)
	}
	e.mu.Unlock()
	res := e.RepairIdentities(context.Background())
	if res.MovedBack != 1 {
		t.Fatalf("not moved back: %+v", res)
	}
	fi, err := os.Lstat(a.Path)
	if err != nil || !fi.IsDir() || fi.Mode()&os.ModeSymlink != 0 {
		t.Fatal("official path is not a real app folder", err)
	}
	if _, err := os.Lstat(custom); !os.IsNotExist(err) {
		t.Fatal("custom folder still exists", err)
	}
	nr := e.SnapshotState().Renames[0]
	if nr.To != a.Path || nr.From != a.Path || nr.Status != "applied" || nr.NewName != "Photoshop" {
		t.Fatalf("label not kept: %+v", nr)
	}
	apps, _, err := e.Scan(context.Background())
	if err != nil || len(apps) != 1 || apps[0].Path != a.Path || apps[0].Name != "Photoshop" || !apps[0].Renamed {
		t.Fatalf("scan did not keep the label on the official path: %+v %v", apps, err)
	}
}

func TestRepairLeavesBothCopies(t *testing.T) {
	e, _, a, _ := fixture(t)
	custom := filepath.Join(filepath.Dir(a.Path), "Photoshop.app")
	if err := os.MkdirAll(filepath.Join(custom, "Contents/MacOS"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(custom, "keep"), []byte("custom"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(a.Path, "keep"), []byte("official"), 0600); err != nil {
		t.Fatal(err)
	}
	e.mu.Lock()
	e.State.Renames = append(e.State.Renames, RenameRecord{
		App: a, ID: "legacy", BundleID: a.BundleID, From: a.Path, To: custom,
		OldName: "PhotoCraft", NewName: "Photoshop", Status: "applied",
	})
	if err := e.save(); err != nil {
		e.mu.Unlock()
		t.Fatal(err)
	}
	e.mu.Unlock()
	res := e.RepairIdentities(context.Background())
	if res.MovedBack != 0 {
		t.Fatalf("a second copy was moved: %+v", res)
	}
	joined := strings.Join(res.Notes, " ")
	if !strings.Contains(joined, "Photoshop.app") || !strings.Contains(joined, "PhotoCraft.app") {
		t.Fatalf("note should name both copies: %+v", res)
	}
	for _, p := range []struct{ path, want string }{{filepath.Join(custom, "keep"), "custom"}, {filepath.Join(a.Path, "keep"), "official"}} {
		b, err := os.ReadFile(p.path)
		if err != nil || string(b) != p.want {
			t.Fatal("a copy was deleted or rewritten", p.path, err, string(b))
		}
	}
	var legacy, label int
	for _, nr := range e.SnapshotState().Renames {
		if nr.ID == "legacy" && nr.Status == "duplicate-left" {
			legacy++
		}
		if nr.Status == "applied" && nr.From == a.Path && nr.To == a.Path && nr.NewName == "Photoshop" {
			label++
		}
	}
	if legacy != 1 || label != 1 {
		t.Fatalf("journal: %+v", e.SnapshotState().Renames)
	}
}

func TestRepairRestoresAppliedFullSkinThenStops(t *testing.T) {
	e, n, a, asset := fixture(t)
	plistPath := filepath.Join(a.Path, "Contents/Info.plist")
	original, err := os.ReadFile(plistPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(plistPath, []byte("skinned-metadata"), 0600); err != nil {
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
		ID: "old-skin", BundleID: a.BundleID, App: a, AssetID: asset.ID, Status: "applied",
		AfterPrint: fp, IconFile: "AppIcon.icns", Name: "Photoshop", OriginalName: "PhotoCraft",
	})
	if err := e.save(); err != nil {
		e.mu.Unlock()
		t.Fatal(err)
	}
	e.mu.Unlock()
	res := e.RepairIdentities(context.Background())
	if res.IconsApplied != 1 || n.resigns != 1 {
		t.Fatalf("skin not restored once: %+v resigns=%d", res, n.resigns)
	}
	got, err := os.ReadFile(plistPath)
	if err != nil || string(got) != string(original) {
		t.Fatal("backed-up plist was not restored", err)
	}
	if !n.states[a.Path].Custom() {
		t.Fatal("Finder icon was not applied after the internal skin was undone")
	}
	if err := os.WriteFile(plistPath, []byte("vendor-update"), 0600); err != nil {
		t.Fatal(err)
	}
	n.states[a.Path] = Snapshot{Exists: false}
	if _, _, err := e.Scan(context.Background()); err != nil {
		t.Fatal(err)
	}
	rec, err := e.Reconcile(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if n.resigns != 1 || len(n.bundleNames) != 0 {
		t.Fatalf("update was re-signed or renamed: resigns=%d names=%v result=%+v", n.resigns, n.bundleNames, rec)
	}
	updated, err := os.ReadFile(plistPath)
	if err != nil || string(updated) != "vendor-update" {
		t.Fatal("reconcile wrote into the updated bundle", err)
	}
}

func TestRepairDoesNotWriteUpdatedBundle(t *testing.T) {
	e, n, a, asset := fixture(t)
	plistPath := filepath.Join(a.Path, "Contents/Info.plist")
	current := []byte("already-updated")
	if err := os.WriteFile(plistPath, current, 0600); err != nil {
		t.Fatal(err)
	}
	e.mu.Lock()
	e.State.Rebrands = append(e.State.Rebrands, RebrandRecord{
		ID: "old-skin", BundleID: a.BundleID, App: a, AssetID: asset.ID, Status: "applied",
		AfterPrint: "bytes-iconcraft-wrote", IconFile: "AppIcon.icns", Name: "Photoshop", OriginalName: "PhotoCraft",
	})
	if err := e.save(); err != nil {
		e.mu.Unlock()
		t.Fatal(err)
	}
	e.mu.Unlock()
	res := e.RepairIdentities(context.Background())
	if n.resigns != 0 || len(n.bundleNames) != 0 {
		t.Fatalf("updated bundle was edited: %+v resigns=%d", res, n.resigns)
	}
	got, err := os.ReadFile(plistPath)
	if err != nil || string(got) != string(current) {
		t.Fatal("plist changed", err)
	}
	if !n.states[a.Path].Custom() || res.IconsApplied != 1 {
		t.Fatalf("Finder icon not applied: %+v custom=%v", res, n.states[a.Path].Custom())
	}
	var migrated bool
	for _, r := range e.SnapshotState().Rebrands {
		if r.ID == "old-skin" && r.Status == "migrated" {
			migrated = true
		}
	}
	if !migrated {
		t.Fatalf("status: %+v", e.SnapshotState().Rebrands)
	}
}

func TestReconcileReappliesFinderIconAfterBundleReplace(t *testing.T) {
	e, n, a, asset := fixture(t)
	if _, err := e.Apply(context.Background(), a.ID, asset.ID); err != nil {
		t.Fatal(err)
	}
	plistPath := filepath.Join(a.Path, "Contents/Info.plist")
	if err := os.WriteFile(plistPath, []byte("vendor-update"), 0600); err != nil {
		t.Fatal(err)
	}
	n.states[a.Path] = Snapshot{Exists: false}
	if _, _, err := e.Scan(context.Background()); err != nil {
		t.Fatal(err)
	}
	res, err := e.Reconcile(context.Background())
	if err != nil || res.IconFixed < 1 {
		t.Fatalf("icon not reapplied: %+v %v", res, err)
	}
	if !n.states[a.Path].Custom() {
		t.Fatal("Finder icon missing after reapply")
	}
	if n.resigns != 0 || len(n.bundleNames) != 0 {
		t.Fatal("reapply edited the bundle")
	}
	got, err := os.ReadFile(plistPath)
	if err != nil || string(got) != "vendor-update" {
		t.Fatal("Info.plist was rewritten", err)
	}
}
