package core

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type countingInspector struct {
	*fakeNative
	calls int
}

func (n *countingInspector) Inspect(c context.Context, p []string) ([]App, error) {
	n.calls++
	return n.fakeNative.Inspect(c, p)
}
func TestWarmScanSkipsNativeMetadata(t *testing.T) {
	e, n, a, _ := fixture(t)
	cn := &countingInspector{fakeNative: n}
	e.Native = cn
	if _, _, err := e.Scan(context.Background()); err != nil {
		t.Fatal(err)
	}
	if cn.calls != 0 {
		t.Fatal("unchanged app was re-inspected")
	}
	_ = os.WriteFile(filepath.Join(a.Path, "Contents/Info.plist"), []byte("updated metadata"), 0600)
	_, _, err := e.Scan(context.Background())
	if err != nil || cn.calls != 1 {
		t.Fatal(cn.calls, err)
	}
}
func TestInventoryAvailableBeforeScanOnRelaunch(t *testing.T) {
	e, n, a, _ := fixture(t)
	e2, err := Open(e.Dir, e.Home, n)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := e2.GetApp(a.ID); err != nil || got.Path != a.Path {
		t.Fatal(got, err)
	}
}
func TestSessionDoesNotSendBackupPayloads(t *testing.T) {
	e, _, a, asset := fixture(t)
	_, _ = e.Apply(context.Background(), a.ID, asset.ID)
	b, _ := json.Marshal(e.Session())
	if strings.Contains(string(b), "finderInfo") || strings.Contains(string(b), "attributes") {
		t.Fatal("recovery data leaked to UI")
	}
}
func TestThemePersistsAcrossRestart(t *testing.T) {
	e, n, _, _ := fixture(t)
	if e.SetTheme("matrix") == nil {
		t.Fatal("bad theme")
	}
	_ = e.SetTheme("dark")
	e2, _ := Open(e.Dir, e.Home, n)
	if e2.State.Theme != "dark" {
		t.Fatal("theme lost")
	}
}
func TestRenameLeavesSignedContentsUnchangedAndRestores(t *testing.T) {
	e, _, a, _ := fixture(t)
	fp, _ := Fingerprint(a)
	b, err := e.Rename(context.Background(), a.ID, "Adobe Photoshop")
	if err != nil {
		t.Fatal(err)
	}
	if b.Name != "Adobe Photoshop" || !b.Renamed || b.Path == a.Path {
		t.Fatal(b)
	}
	fp2, _ := Fingerprint(b)
	if fp != fp2 {
		t.Fatal("bundle contents changed")
	}
	b2, err := e.RestoreName(context.Background(), b.ID)
	if err != nil || b2.Path != a.Path || b2.Renamed {
		t.Fatal(b2, err)
	}
}
func TestRenameCannotOverwriteAnyDestination(t *testing.T) {
	e, _, a, _ := fixture(t)
	dest := filepath.Join(e.Home, "Photoshop.app")
	_ = os.Mkdir(dest, 0700)
	_ = os.WriteFile(filepath.Join(dest, "keep"), []byte("keep"), 0600)
	if _, err := e.Rename(context.Background(), a.ID, "Photoshop"); err == nil {
		t.Fatal("collision accepted")
	}
	if _, err := os.Stat(filepath.Join(dest, "keep")); err != nil {
		t.Fatal("collision modified")
	}
}
func TestRenameRejectsUnsafeNames(t *testing.T) {
	for _, s := range []string{"", "..", ".app", "../Oops", "a:b", "a\\b", "a\x00b", strings.Repeat("a", 230)} {
		if _, err := ValidateName(s); err == nil {
			t.Fatal(s)
		}
	}
	if n, err := ValidateName(" Photo Editor.app "); err != nil || n != "Photo Editor" {
		t.Fatal(n, err)
	}
}
func TestRenameMovesIconBackupReferencesAndUnwindsNames(t *testing.T) {
	e, _, a, asset := fixture(t)
	_, err := e.Apply(context.Background(), a.ID, asset.ID)
	if err != nil {
		t.Fatal(err)
	}
	b, err := e.Rename(context.Background(), a.ID, "Photoshop")
	if err != nil {
		t.Fatal(err)
	}
	c, err := e.Rename(context.Background(), b.ID, "My Editor")
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range e.State.Records {
		if r.App.Path != c.Path || r.App.ID != c.ID {
			t.Fatal("stale recovery path")
		}
	}
	back, err := e.RestoreName(context.Background(), c.ID)
	if err != nil || back.Path != a.Path {
		t.Fatal(back, err)
	}
	if e.State.Records[0].App.Path != a.Path {
		t.Fatal("history not repaired")
	}
}
func TestRenameRestoreRefusesUpdatedApplication(t *testing.T) {
	e, _, a, _ := fixture(t)
	b, _ := e.Rename(context.Background(), a.ID, "Photoshop")
	_ = os.WriteFile(filepath.Join(b.Path, "Contents/MacOS/test"), []byte("update"), 0700)
	if _, err := e.RestoreName(context.Background(), b.ID); err == nil {
		t.Fatal("updated app renamed from stale record")
	}
}
func TestRenameRecoversPreparedMoveAfterRestart(t *testing.T) {
	e, n, a, _ := fixture(t)
	b, _ := e.Rename(context.Background(), a.ID, "Photoshop")
	e.State.Renames[0].Status = "prepared"
	_ = e.save()
	// Simulate old inventory on disk and an interrupted state commit.
	raw, _ := json.Marshal([]App{a})
	_ = AtomicWrite(filepath.Join(e.Dir, "inventory.json"), raw, 0600)
	again, err := Open(e.Dir, e.Home, n)
	if err != nil {
		t.Fatal(err)
	}
	if again.State.Renames[0].Status != "applied" {
		t.Fatal("pending rename not recovered")
	}
	if _, err = again.GetApp(b.ID); err != nil {
		t.Fatal(err)
	}
}
func BenchmarkWarmInventoryScan(b *testing.B) {
	root := b.TempDir()
	n := &fakeNative{states: map[string]Snapshot{}}
	e, _ := Open(filepath.Join(root, "state"), root, n)
	e.Roots = []string{root}
	for i := 0; i < 100; i++ {
		p := filepath.Join(root, fmt.Sprintf("App%d.app", i))
		_ = os.MkdirAll(filepath.Join(p, "Contents"), 0700)
		_ = os.WriteFile(filepath.Join(p, "Contents/Info.plist"), []byte("test"), 0600)
		a := App{Path: p, Name: "Test", BundleID: fmt.Sprint(i), Executable: "x", InfoStamp: MetadataStamp(p)}
		a.ID = appID(a)
		e.Apps[a.ID] = a
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, _ = e.Scan(context.Background())
	}
}
