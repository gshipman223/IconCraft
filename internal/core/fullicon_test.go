package core

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func useFullIcons(t *testing.T, e *Engine) {
	t.Helper()
	s := e.Settings()
	s.IconMode = IconModeFull
	if _, err := e.SaveSettings(s); err != nil {
		t.Fatal(err)
	}
}

func plantBundleIcon(t *testing.T, app string) []byte {
	t.Helper()
	dir := filepath.Join(app, "Contents/Resources")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	orig := []byte("original-icns-bytes")
	if err := os.WriteFile(filepath.Join(dir, "AppIcon.icns"), orig, 0644); err != nil {
		t.Fatal(err)
	}
	return orig
}

func readBundleIcon(t *testing.T, app string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(app, "Contents/Resources/AppIcon.icns"))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestFullRenameDoesNotChangeTheRunningIcon(t *testing.T) {
	e, n, a, asset := fixture(t)
	n.follow = true
	writeIdentityPlist(t, a.Path, a.BundleID, "PhotoCraft")
	orig := plantBundleIcon(t, a.Path)
	useFullNames(t, e)
	got, err := e.Rename(context.Background(), a.ID, "Photoshop")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = e.Apply(context.Background(), got.ID, asset.ID); err != nil {
		t.Fatal(err)
	}
	if string(readBundleIcon(t, got.Path)) != string(orig) {
		t.Fatal("renaming the folder changed the icon inside the app")
	}
	if n.resigns != 0 || len(n.bundleNames) != 0 {
		t.Fatalf("full rename edited signed files: resigns=%d names=%v", n.resigns, n.bundleNames)
	}
}

func TestFullIconReplacesBundleIconWithoutTouchingIdentity(t *testing.T) {
	e, n, a, asset := fixture(t)
	writeIdentityPlist(t, a.Path, a.BundleID, "PhotoCraft")
	before := plistBytes(t, a.Path)
	orig := plantBundleIcon(t, a.Path)
	useFullIcons(t, e)
	rec, err := e.ApplyBundleIcon(context.Background(), a.ID, asset.ID)
	if err != nil {
		t.Fatal(err)
	}
	if rec.Kind != RebrandKindIcon || rec.Status != "applied" {
		t.Fatalf("record: %+v", rec)
	}
	if string(plistBytes(t, a.Path)) != string(before) {
		t.Fatal("Info.plist changed")
	}
	if string(readBundleIcon(t, a.Path)) == string(orig) {
		t.Fatal("bundle icon was not replaced")
	}
	if n.resigns != 1 || len(n.bundleNames) != 0 {
		t.Fatalf("resigns=%d names=%v", n.resigns, n.bundleNames)
	}
	if !artCraftSees(filepath.Dir(a.Path), "PhotoCraft", []string{a.BundleID}) {
		t.Fatal("launcher no longer sees the app")
	}
	if countApps(t, filepath.Dir(a.Path)) != 1 {
		t.Fatal("full icon duplicated the app")
	}
}

func TestFullIconWorksWhileTheFolderIsRenamed(t *testing.T) {
	e, n, a, asset := fixture(t)
	n.follow = true
	writeIdentityPlist(t, a.Path, a.BundleID, "PhotoCraft")
	before := plistBytes(t, a.Path)
	orig := plantBundleIcon(t, a.Path)
	useFullNames(t, e)
	useFullIcons(t, e)
	got, err := e.Rename(context.Background(), a.ID, "Photoshop")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = e.ApplyBundleIcon(context.Background(), got.ID, asset.ID); err != nil {
		t.Fatal(err)
	}
	if string(plistBytes(t, got.Path)) != string(before) {
		t.Fatal("Info.plist changed")
	}
	if string(readBundleIcon(t, got.Path)) == string(orig) {
		t.Fatal("renamed app kept the publisher icon inside the bundle")
	}
	if cfBundleID(got.Path) != a.BundleID {
		t.Fatal("bundle id changed")
	}
	if n.resigns != 1 || len(n.bundleNames) != 0 {
		t.Fatalf("resigns=%d names=%v", n.resigns, n.bundleNames)
	}
}

func TestFullIconRefusesAssetCatalog(t *testing.T) {
	e, n, a, asset := fixture(t)
	n.iconFile = "catalog"
	useFullIcons(t, e)
	_, err := e.ApplyBundleIcon(context.Background(), a.ID, asset.ID)
	if err == nil || !strings.Contains(err.Error(), "asset catalog") {
		t.Fatal(err)
	}
	if n.resigns != 0 {
		t.Fatalf("asset catalog was re-signed: %d", n.resigns)
	}
}

func TestPauseRestoresBundleIconAndUnpauseReapplies(t *testing.T) {
	e, n, a, asset := fixture(t)
	writeIdentityPlist(t, a.Path, a.BundleID, "PhotoCraft")
	before := plistBytes(t, a.Path)
	orig := plantBundleIcon(t, a.Path)
	useFullIcons(t, e)
	if _, err := e.ApplyBundleIcon(context.Background(), a.ID, asset.ID); err != nil {
		t.Fatal(err)
	}
	pauseUpdates(t, e, true)
	if notes := e.ApplyNamePolicy(context.Background()); len(notes.Notes) > 0 {
		t.Fatal(notes.Notes)
	}
	if string(readBundleIcon(t, a.Path)) != string(orig) {
		t.Fatal("pause left the custom icon inside the app")
	}
	if string(plistBytes(t, a.Path)) != string(before) {
		t.Fatal("pause changed Info.plist")
	}
	held := false
	for _, r := range e.SnapshotState().Rebrands {
		if r.Kind == RebrandKindIcon && r.Held && r.Status == "applied" {
			held = true
		}
	}
	if !held {
		t.Fatal("full icon was not held")
	}
	resigns := n.resigns
	if _, err := e.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if string(readBundleIcon(t, a.Path)) != string(orig) || n.resigns != resigns {
		t.Fatal("reconcile while paused put the custom icon back")
	}
	pauseUpdates(t, e, false)
	if notes := e.ApplyNamePolicy(context.Background()); len(notes.Notes) > 0 {
		t.Fatal(notes.Notes)
	}
	if string(readBundleIcon(t, a.Path)) == string(orig) {
		t.Fatal("unpause did not put the custom icon back")
	}
	if string(plistBytes(t, a.Path)) != string(before) {
		t.Fatal("unpause changed Info.plist")
	}
	if !artCraftSees(filepath.Dir(a.Path), "PhotoCraft", []string{a.BundleID}) {
		t.Fatal("launcher no longer sees the app")
	}
	if countApps(t, filepath.Dir(a.Path)) != 1 {
		t.Fatal("pause duplicated the app")
	}
}

func TestUnpauseDoesNotWriteOldPlistOntoUpdatedBundle(t *testing.T) {
	e, _, a, asset := fixture(t)
	writeIdentityPlist(t, a.Path, a.BundleID, "PhotoCraft")
	plantBundleIcon(t, a.Path)
	useFullIcons(t, e)
	if _, err := e.ApplyBundleIcon(context.Background(), a.ID, asset.ID); err != nil {
		t.Fatal(err)
	}
	pauseUpdates(t, e, true)
	if notes := e.ApplyNamePolicy(context.Background()); len(notes.Notes) > 0 {
		t.Fatal(notes.Notes)
	}
	writeIdentityPlist(t, a.Path, a.BundleID, "PhotoCraft Updated")
	updated := plistBytes(t, a.Path)
	pauseUpdates(t, e, false)
	if notes := e.ApplyNamePolicy(context.Background()); len(notes.Notes) > 0 {
		t.Fatal(notes.Notes)
	}
	if string(plistBytes(t, a.Path)) != string(updated) {
		t.Fatal("unpause wrote the old Info.plist onto the updated app")
	}
	if cfBundleID(a.Path) != a.BundleID {
		t.Fatal("bundle id changed")
	}
	if !artCraftSees(filepath.Dir(a.Path), "PhotoCraft", []string{a.BundleID}) {
		t.Fatal("launcher no longer sees the updated app")
	}
}

func TestSwitchingFullIconOffRestoresTheIconFile(t *testing.T) {
	e, n, a, asset := fixture(t)
	writeIdentityPlist(t, a.Path, a.BundleID, "PhotoCraft")
	before := plistBytes(t, a.Path)
	orig := plantBundleIcon(t, a.Path)
	useFullIcons(t, e)
	if _, err := e.ApplyBundleIcon(context.Background(), a.ID, asset.ID); err != nil {
		t.Fatal(err)
	}
	s := e.Settings()
	s.IconMode = IconModeFinder
	if _, err := e.SaveSettings(s); err != nil {
		t.Fatal(err)
	}
	if notes := e.SyncIconModes(context.Background(), ""); len(notes.Notes) > 0 {
		t.Fatal(notes.Notes)
	}
	if string(readBundleIcon(t, a.Path)) != string(orig) {
		t.Fatal("turning Full icon off left the replacement icon in place")
	}
	if string(plistBytes(t, a.Path)) != string(before) {
		t.Fatal("Info.plist changed")
	}
	if n.resigns < 2 {
		t.Fatalf("restore did not re-sign: %d", n.resigns)
	}
}

func TestPerAppFinderOverrideDoesNotResign(t *testing.T) {
	e, n, a, asset := fixture(t)
	useFullIcons(t, e)
	if _, err := e.SetAppIconMode(a.ID, IconModeFinder); err != nil {
		t.Fatal(err)
	}
	if _, err := e.ApplyBundleIcon(context.Background(), a.ID, asset.ID); err == nil {
		t.Fatal("per-app Finder override still replaced the bundle icon")
	}
	if n.resigns != 0 {
		t.Fatalf("override re-signed: %d", n.resigns)
	}
}

func TestRepairLeavesAnActiveFullIconAlone(t *testing.T) {
	e, n, a, asset := fixture(t)
	plantBundleIcon(t, a.Path)
	useFullIcons(t, e)
	if _, err := e.ApplyBundleIcon(context.Background(), a.ID, asset.ID); err != nil {
		t.Fatal(err)
	}
	custom := readBundleIcon(t, a.Path)
	resigns := n.resigns
	_ = e.RepairIdentities(context.Background())
	if string(readBundleIcon(t, a.Path)) != string(custom) || n.resigns != resigns {
		t.Fatal("repair undid a Full icon the user still wants")
	}
}

func TestFullIconAndFullRenamePauseTogether(t *testing.T) {
	e, n, a, asset := fixture(t)
	n.follow = true
	writeIdentityPlist(t, a.Path, a.BundleID, "PhotoCraft")
	before := plistBytes(t, a.Path)
	orig := plantBundleIcon(t, a.Path)
	useFullNames(t, e)
	useFullIcons(t, e)
	got, err := e.Rename(context.Background(), a.ID, "Photoshop")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = e.ApplyBundleIcon(context.Background(), got.ID, asset.ID); err != nil {
		t.Fatal(err)
	}
	pauseUpdates(t, e, true)
	if notes := e.ApplyNamePolicy(context.Background()); len(notes.Notes) > 0 {
		t.Fatal(notes.Notes)
	}
	if !artCraftSees(filepath.Dir(a.Path), "PhotoCraft", []string{a.BundleID}) {
		t.Fatal("pause did not put the folder back where the launcher looks")
	}
	if string(readBundleIcon(t, a.Path)) != string(orig) {
		t.Fatal("pause did not put the original icon file back")
	}
	if countApps(t, filepath.Dir(a.Path)) != 1 {
		t.Fatal("pause duplicated the app")
	}
	pauseUpdates(t, e, false)
	if notes := e.ApplyNamePolicy(context.Background()); len(notes.Notes) > 0 {
		t.Fatal(notes.Notes)
	}
	apps, _, err := e.Scan(context.Background())
	if err != nil || len(apps) != 1 {
		t.Fatal(apps, err)
	}
	if filepath.Base(apps[0].Path) != "Photoshop.app" {
		t.Fatalf("unpause did not restore the custom folder: %s", apps[0].Path)
	}
	if string(readBundleIcon(t, apps[0].Path)) == string(orig) {
		t.Fatal("unpause did not put the custom icon back")
	}
	if string(plistBytes(t, apps[0].Path)) != string(before) {
		t.Fatal("unpause changed Info.plist")
	}
	if countApps(t, filepath.Dir(a.Path)) != 1 {
		t.Fatal("unpause duplicated the app")
	}
}
