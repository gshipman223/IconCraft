package core

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func useFullNames(t *testing.T, e *Engine) {
	t.Helper()
	s := e.Settings()
	s.NameMode = NameModeFull
	if _, err := e.SaveSettings(s); err != nil {
		t.Fatal(err)
	}
}

func useSafeNames(t *testing.T, e *Engine) {
	t.Helper()
	s := e.Settings()
	s.NameMode = NameModeSafe
	s.UpdatesPaused = false
	if _, err := e.SaveSettings(s); err != nil {
		t.Fatal(err)
	}
}

func pauseUpdates(t *testing.T, e *Engine, paused bool) {
	t.Helper()
	s := e.Settings()
	s.UpdatesPaused = paused
	if _, err := e.SaveSettings(s); err != nil {
		t.Fatal(err)
	}
}

func plistBytes(t *testing.T, app string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(app, "Contents/Info.plist"))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func countApps(t *testing.T, dir string) int {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, e := range entries {
		if e.IsDir() && filepath.Ext(e.Name()) == ".app" {
			n++
		}
	}
	return n
}

func TestDefaultNameModeDoesNotMoveTheBundle(t *testing.T) {
	e, n, a, _ := fixture(t)
	writeIdentityPlist(t, a.Path, a.BundleID, "PhotoCraft")
	before := plistBytes(t, a.Path)
	got, err := e.Rename(context.Background(), a.ID, "Photoshop")
	if err != nil {
		t.Fatal(err)
	}
	if got.Path != a.Path || got.Name != "Photoshop" {
		t.Fatalf("label changed the bundle: %+v", got)
	}
	if !artCraftSees(filepath.Dir(a.Path), "PhotoCraft", []string{a.BundleID}) {
		t.Fatal("launcher no longer sees the installed app")
	}
	if n.resigns != 0 || len(n.bundleNames) != 0 {
		t.Fatalf("signed files were edited: resigns=%d names=%v", n.resigns, n.bundleNames)
	}
	if string(plistBytes(t, a.Path)) != string(before) {
		t.Fatal("Info.plist changed")
	}
}

func TestFullRenameMovesFolderWithoutTouchingTheSignature(t *testing.T) {
	e, n, a, _ := fixture(t)
	writeIdentityPlist(t, a.Path, a.BundleID, "PhotoCraft")
	before := plistBytes(t, a.Path)
	useFullNames(t, e)
	got, err := e.Rename(context.Background(), a.ID, "Photoshop")
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(got.Path) != "Photoshop.app" {
		t.Fatalf("folder not renamed: %s", got.Path)
	}
	if artCraftSees(filepath.Dir(a.Path), "PhotoCraft", []string{a.BundleID}) {
		t.Fatal("launcher still sees a renamed app at the official path")
	}
	if n.resigns != 0 || len(n.bundleNames) != 0 {
		t.Fatalf("signed files were edited: resigns=%d names=%v", n.resigns, n.bundleNames)
	}
	if string(plistBytes(t, got.Path)) != string(before) {
		t.Fatal("Info.plist changed")
	}
	if countApps(t, filepath.Dir(a.Path)) != 1 {
		t.Fatal("rename duplicated the app")
	}
	back, err := e.Rename(context.Background(), got.ID, "PhotoCraft")
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(back.Path) != "PhotoCraft.app" {
		t.Fatalf("renaming back to the launcher name left the folder at %s", back.Path)
	}
	if !artCraftSees(filepath.Dir(a.Path), "PhotoCraft", []string{a.BundleID}) {
		t.Fatal("launcher does not see the restored folder")
	}
}

func TestSwitchingBackToSafeRestoresTheLauncherPath(t *testing.T) {
	e, n, a, _ := fixture(t)
	n.follow = true
	writeIdentityPlist(t, a.Path, a.BundleID, "PhotoCraft")
	useFullNames(t, e)
	if _, err := e.Rename(context.Background(), a.ID, "Photoshop"); err != nil {
		t.Fatal(err)
	}
	useSafeNames(t, e)
	if notes := e.ApplyNamePolicy(context.Background()); len(notes.Notes) > 0 {
		t.Fatal(notes.Notes)
	}
	if !artCraftSees(filepath.Dir(a.Path), "PhotoCraft", []string{a.BundleID}) {
		t.Fatal("safe mode did not restore the launcher path")
	}
	if countApps(t, filepath.Dir(a.Path)) != 1 {
		t.Fatal("switching mode duplicated or deleted a copy")
	}
	apps, _, err := e.Scan(context.Background())
	if err != nil || len(apps) != 1 {
		t.Fatal(apps, err)
	}
	if apps[0].Name != "Photoshop" || apps[0].Path != a.Path {
		t.Fatalf("custom name should remain a label at the original path: %+v", apps[0])
	}
}

func TestFullRenameLeavesASecondCopyAlone(t *testing.T) {
	e, n, a, _ := fixture(t)
	n.follow = true
	writeIdentityPlist(t, a.Path, a.BundleID, "PhotoCraft")
	useFullNames(t, e)
	got, err := e.Rename(context.Background(), a.ID, "Photoshop")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.MkdirAll(filepath.Join(a.Path, "Contents"), 0700); err != nil {
		t.Fatal(err)
	}
	writeIdentityPlist(t, a.Path, a.BundleID, "PhotoCraft")
	if _, _, err = e.Scan(context.Background()); err != nil {
		t.Fatal(err)
	}
	res, err := e.Reconcile(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !artCraftSees(filepath.Dir(a.Path), "PhotoCraft", []string{a.BundleID}) {
		t.Fatal("official copy disappeared")
	}
	if _, err = os.Lstat(got.Path); err != nil {
		t.Fatal("custom copy was deleted", err)
	}
	if countApps(t, filepath.Dir(a.Path)) != 2 {
		t.Fatal("expected both copies to remain")
	}
	for _, note := range res.Skipped {
		if note == "" {
			t.Fatal(res.Skipped)
		}
	}
}

func TestFullRenameReappliesAfterTheOfficialFolderReturns(t *testing.T) {
	e, n, a, _ := fixture(t)
	n.follow = true
	writeIdentityPlist(t, a.Path, a.BundleID, "PhotoCraft")
	useFullNames(t, e)
	got, err := e.Rename(context.Background(), a.ID, "Photoshop")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.Rename(got.Path, a.Path); err != nil {
		t.Fatal(err)
	}
	updated := []byte("updated-by-vendor")
	if err = os.WriteFile(filepath.Join(a.Path, "Contents/Info.plist"), updated, 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, err = e.Scan(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err = e.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Lstat(a.Path); !os.IsNotExist(err) {
		t.Fatal("official folder was left behind or a second copy was made", err)
	}
	if string(plistBytes(t, got.Path)) != string(updated) {
		t.Fatal("re-applying the rename did not keep the updated bundle")
	}
	if countApps(t, filepath.Dir(a.Path)) != 1 {
		t.Fatal("reapply duplicated the app")
	}
}

func TestPauseRestoresNamesAndUnpauseReappliesByBundleID(t *testing.T) {
	e, n, a, asset := fixture(t)
	n.follow = true
	writeIdentityPlist(t, a.Path, a.BundleID, "PhotoCraft")
	apps, _, err := e.Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	a = apps[0]
	if _, err = e.Apply(context.Background(), a.ID, asset.ID); err != nil {
		t.Fatal(err)
	}
	useFullNames(t, e)
	got, err := e.Rename(context.Background(), a.ID, "Photoshop")
	if err != nil {
		t.Fatal(err)
	}
	pauseUpdates(t, e, true)
	policy := e.ApplyNamePolicy(context.Background())
	if len(policy.Notes) > 0 {
		t.Fatal(policy.Notes)
	}
	if !artCraftSees(filepath.Dir(a.Path), "PhotoCraft", []string{a.BundleID}) {
		t.Fatal("pause did not restore the launcher path")
	}
	if _, err = os.Lstat(got.Path); !os.IsNotExist(err) {
		t.Fatal("custom folder still present during pause", err)
	}
	if n.states[a.Path].Custom() {
		t.Fatal("pause left the custom Finder icon in place")
	}
	if _, err = e.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Lstat(got.Path); !os.IsNotExist(err) {
		t.Fatal("reconcile renamed the app while paused")
	}
	updated := []byte("replaced-while-paused")
	if err = os.WriteFile(filepath.Join(a.Path, "Contents/Info.plist"), updated, 0600); err != nil {
		t.Fatal(err)
	}
	pauseUpdates(t, e, false)
	policy = e.ApplyNamePolicy(context.Background())
	if len(policy.Notes) > 0 {
		t.Fatal(policy.Notes)
	}
	if _, err = os.Lstat(a.Path); !os.IsNotExist(err) {
		t.Fatal("unpause left the official folder behind", err)
	}
	if string(plistBytes(t, got.Path)) != string(updated) {
		t.Fatal("unpause did not keep the bundle the updater wrote")
	}
	if !n.states[got.Path].Custom() {
		t.Fatal("custom Finder icon was not put back")
	}
	if countApps(t, filepath.Dir(a.Path)) != 1 {
		t.Fatal("unpause duplicated the app")
	}
	if e.UpdatesPaused() {
		t.Fatal("pause flag did not clear")
	}
}

func TestPausePersistsAndSurvivesAQuitMidRestore(t *testing.T) {
	e, n, a, _ := fixture(t)
	n.follow = true
	writeIdentityPlist(t, a.Path, a.BundleID, "PhotoCraft")
	useFullNames(t, e)
	got, err := e.Rename(context.Background(), a.ID, "Photoshop")
	if err != nil {
		t.Fatal(err)
	}
	e.mu.Lock()
	e.State.Renames[len(e.State.Renames)-1].Status = "suspending"
	e.State.Settings.UpdatesPaused = true
	if err = e.save(); err != nil {
		e.mu.Unlock()
		t.Fatal(err)
	}
	e.mu.Unlock()
	if err = os.Rename(got.Path, a.Path); err != nil {
		t.Fatal(err)
	}
	e2, err := Open(e.Dir, e.Home, n)
	if err != nil {
		t.Fatal(err)
	}
	e2.Roots = e.Roots
	if !e2.UpdatesPaused() {
		t.Fatal("pause did not persist")
	}
	if _, err = os.Lstat(a.Path); err != nil {
		t.Fatal("original folder missing after recovery", err)
	}
	if _, err = os.Lstat(got.Path); !os.IsNotExist(err) {
		t.Fatal("recovery moved the app back to the custom name", err)
	}
	suspended := false
	for _, r := range e2.SnapshotState().Renames {
		if r.BundleID == a.BundleID && r.Suspended && r.Status == "applied" {
			suspended = true
		}
	}
	if !suspended {
		t.Fatal("interrupted pause was not left suspended")
	}
	if _, _, err = e2.Scan(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err = e2.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Lstat(got.Path); !os.IsNotExist(err) {
		t.Fatal("keeper renamed the app while the pause was still saved")
	}

	// The other half of a quit: pause is already off, the move back to the
	// custom name was journalled, and the folder is still at the original path.
	e2.mu.Lock()
	for i := range e2.State.Renames {
		if e2.State.Renames[i].BundleID == a.BundleID && e2.State.Renames[i].Suspended {
			e2.State.Renames[i].Status = "resuming"
		}
	}
	e2.State.Settings.UpdatesPaused = false
	if err = e2.save(); err != nil {
		e2.mu.Unlock()
		t.Fatal(err)
	}
	e2.mu.Unlock()
	e3, err := Open(e.Dir, e.Home, n)
	if err != nil {
		t.Fatal(err)
	}
	if e3.UpdatesPaused() {
		t.Fatal("unpause did not persist")
	}
	if _, err = os.Lstat(got.Path); err != nil {
		t.Fatal("interrupted resume did not finish", err)
	}
	if _, err = os.Lstat(a.Path); !os.IsNotExist(err) {
		t.Fatal("interrupted resume left a second copy", err)
	}
}

func TestPerAppSafeOverrideDoesNotMove(t *testing.T) {
	e, _, a, _ := fixture(t)
	writeIdentityPlist(t, a.Path, a.BundleID, "PhotoCraft")
	useFullNames(t, e)
	if _, err := e.SetAppNameMode(a.ID, NameModeSafe); err != nil {
		t.Fatal(err)
	}
	got, err := e.Rename(context.Background(), a.ID, "Photoshop")
	if err != nil {
		t.Fatal(err)
	}
	if got.Path != a.Path {
		t.Fatalf("per-app safe mode moved the folder: %s", got.Path)
	}
	if !artCraftSees(filepath.Dir(a.Path), "PhotoCraft", []string{a.BundleID}) {
		t.Fatal("launcher lost the app")
	}
}

func TestPerAppPauseRestoresOnlyThatBundle(t *testing.T) {
	e, n, a, _ := fixture(t)
	n.follow = true
	writeIdentityPlist(t, a.Path, a.BundleID, "PhotoCraft")
	useFullNames(t, e)
	got, err := e.Rename(context.Background(), a.ID, "Photoshop")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = e.SetBundlePaused(context.Background(), got.ID, true); err != nil {
		t.Fatal(err)
	}
	if e.UpdatesPaused() {
		t.Fatal("per-app pause turned on the global switch")
	}
	if !artCraftSees(filepath.Dir(a.Path), "PhotoCraft", []string{a.BundleID}) {
		t.Fatal("per-app pause did not restore the launcher path")
	}
	if _, err = e.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Lstat(got.Path); !os.IsNotExist(err) {
		t.Fatal("reconcile undid a per-app pause")
	}
	apps, _, err := e.Scan(context.Background())
	if err != nil || len(apps) != 1 {
		t.Fatal(apps, err)
	}
	if _, err = e.SetBundlePaused(context.Background(), apps[0].ID, false); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Lstat(got.Path); err != nil {
		t.Fatal("ending the per-app pause did not put the custom name back", err)
	}
	if countApps(t, filepath.Dir(a.Path)) != 1 {
		t.Fatal("per-app pause duplicated the app")
	}
}
