package core

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type fakeNative struct {
	states           map[string]Snapshot
	app              App
	sets             int
	restores         int
	resigns          int
	bundleNames      map[string]string
	failSet          bool
	failRestore      bool
	probeErr         error
	badSignature     bool
	changeExecutable bool
}

func (n *fakeNative) WriteProbe(string) error { return n.probeErr }

func (n *fakeNative) BundleIconFile(_ context.Context, _ string) (string, error) {
	return "AppIcon.icns", nil
}
func (n *fakeNative) SetBundleName(_ context.Context, p, name string) error {
	if n.bundleNames == nil {
		n.bundleNames = map[string]string{}
	}
	n.bundleNames[p] = name
	return nil
}
func (n *fakeNative) Resign(_ context.Context, _ string) error { n.resigns++; return nil }
func (n *fakeNative) BundleName(_ context.Context, p string) (string, error) {
	if name, ok := n.bundleNames[p]; ok {
		return name, nil
	}
	return n.app.Name, nil
}

func cloneSnapshot(s Snapshot) Snapshot {
	b, _ := json.Marshal(s)
	var x Snapshot
	_ = json.Unmarshal(b, &x)
	return x
}
func (n *fakeNative) Inspect(_ context.Context, ps []string) ([]App, error) {
	a := []App{}
	for _, p := range ps {
		if p == n.app.Path {
			a = append(a, n.app)
		}
	}
	return a, nil
}
func (n *fakeNative) Snapshot(p string) (Snapshot, error) { return cloneSnapshot(n.states[p]), nil }
func (n *fakeNative) Set(_ context.Context, p, icon string) error {
	n.sets++
	b, err := os.ReadFile(icon)
	if err != nil {
		return err
	}
	s := Snapshot{FinderInfo: make([]byte, 32), Exists: true, Data: []byte{}, Mode: 0600, Attributes: map[string][]byte{"com.apple.ResourceFork": b}}
	s.FinderInfo[8] = 4
	n.states[p] = s
	if n.changeExecutable {
		_ = os.WriteFile(filepath.Join(p, "Contents/MacOS/test"), []byte("updated executable"), 0700)
	}
	if n.failSet {
		return errors.New("simulated native set failure")
	}
	return nil
}
func (n *fakeNative) Restore(_ context.Context, p string, s Snapshot) error {
	n.restores++
	if n.failRestore {
		return errors.New("simulated restore failure")
	}
	n.states[p] = cloneSnapshot(s)
	return nil
}
func (n *fakeNative) ExportIcon(context.Context, string, bool) ([]byte, error) {
	return pngBytes(color.RGBA{255, 100, 20, 255}), nil
}
func (n *fakeNative) Normalize(context.Context, string) ([]byte, error) {
	return pngBytes(color.RGBA{255, 100, 20, 255}), nil
}
func (n *fakeNative) Signature(_ context.Context, p string) (string, error) {
	if n.badSignature && n.states[p].Custom() {
		return "", errors.New("invalid signature")
	}
	return "valid", nil
}
func (n *fakeNative) Choose(context.Context, string) (string, error) { return "", nil }
func pngBytes(c color.RGBA) []byte {
	im := image.NewRGBA(image.Rect(0, 0, 32, 32))
	for y := 0; y < 32; y++ {
		for x := 0; x < 32; x++ {
			im.SetRGBA(x, y, c)
		}
	}
	var b bytes.Buffer
	_ = png.Encode(&b, im)
	return b.Bytes()
}
func fixture(t *testing.T) (*Engine, *fakeNative, App, Asset) {
	t.Helper()
	// macOS TempDir lives under /var -> /private/var; SafeApp rightly refuses
	// symlinked bundle paths, so fixtures must use the resolved real path.
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(root, "PhotoCraft.app")
	if err := os.MkdirAll(filepath.Join(p, "Contents/MacOS"), 0700); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(filepath.Join(p, "Contents/Info.plist"), []byte("fixture-metadata"), 0600)
	_ = os.WriteFile(filepath.Join(p, "Contents/MacOS/test"), []byte("fixture-executable"), 0700)
	a := App{Name: "PhotoCraft", Path: p, BundleID: "ai.storyteller.photocraft", Version: "1.0", Executable: "test"}
	n := &fakeNative{states: map[string]Snapshot{}, app: a}
	e, err := Open(filepath.Join(root, "state"), root, n)
	if err != nil {
		t.Fatal(err)
	}
	e.Roots = []string{root}
	apps, _, err := e.Scan(context.Background())
	if err != nil || len(apps) != 1 {
		t.Fatalf("scan: %v %v", apps, err)
	}
	asset, err := e.ImportPNG("Original", "Fixture", pngBytes(color.RGBA{1, 2, 3, 255}))
	if err != nil {
		t.Fatal(err)
	}
	return e, n, apps[0], asset
}
func TestApplyRestoreDefault(t *testing.T) {
	e, n, a, asset := fixture(t)
	r, err := e.Apply(context.Background(), a.ID, asset.ID)
	if err != nil {
		t.Fatal(err)
	}
	if r.Status != "applied" || !n.states[a.Path].Custom() {
		t.Fatal("not applied")
	}
	_, err = e.Restore(context.Background(), r.ID)
	if err != nil {
		t.Fatal(err)
	}
	if n.states[a.Path].Custom() || n.states[a.Path].Exists {
		t.Fatal("not restored")
	}
}
func TestRestorePriorCustomIcon(t *testing.T) {
	e, n, a, asset := fixture(t)
	prior := Snapshot{FinderInfo: make([]byte, 32), Exists: true, Data: []byte("previous data"), Mode: 0644, Attributes: map[string][]byte{"com.apple.ResourceFork": []byte("prior artwork"), "user.note": []byte("keep")}}
	prior.FinderInfo[8] = 4
	n.states[a.Path] = prior
	r, err := e.Apply(context.Background(), a.ID, asset.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = e.Restore(context.Background(), r.ID); err != nil {
		t.Fatal(err)
	}
	if !n.states[a.Path].Equal(prior) {
		t.Fatal("lost original custom icon")
	}
}
func TestFailedSetRollsBack(t *testing.T) {
	e, n, a, asset := fixture(t)
	n.failSet = true
	r, err := e.Apply(context.Background(), a.ID, asset.ID)
	if err == nil || r.Status != "failed" || n.states[a.Path].Custom() {
		t.Fatal(r, err)
	}
}
func TestFailedRollbackRemainsRecoverable(t *testing.T) {
	e, n, a, asset := fixture(t)
	n.failSet = true
	n.failRestore = true
	r, err := e.Apply(context.Background(), a.ID, asset.ID)
	if err == nil || r.Status != "rollback-required" {
		t.Fatal(r, err)
	}
	n.failSet = false
	n.failRestore = false
	if _, err = e.Apply(context.Background(), a.ID, asset.ID); err == nil {
		t.Fatal("applied over interrupted journal")
	}
	if _, err = e.Restore(context.Background(), r.ID); err != nil {
		t.Fatal(err)
	}
}
func TestSignatureFailureRollsBack(t *testing.T) {
	e, n, a, asset := fixture(t)
	n.badSignature = true
	r, err := e.Apply(context.Background(), a.ID, asset.ID)
	if err == nil || r.Status != "failed" || n.states[a.Path].Custom() {
		t.Fatal(r, err)
	}
}
func TestApplicationUpdateBlocksOldRestore(t *testing.T) {
	e, _, a, asset := fixture(t)
	r, err := e.Apply(context.Background(), a.ID, asset.ID)
	if err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(filepath.Join(a.Path, "Contents/MacOS/test"), []byte("new build"), 0700)
	if _, err = e.Restore(context.Background(), r.ID); err == nil {
		t.Fatal("overwrote updated app")
	}
}
func TestExternalIconChangeBlocksRestore(t *testing.T) {
	e, n, a, asset := fixture(t)
	r, err := e.Apply(context.Background(), a.ID, asset.ID)
	if err != nil {
		t.Fatal(err)
	}
	s := n.states[a.Path]
	s.Attributes["com.apple.ResourceFork"] = []byte("someone else's icon")
	n.states[a.Path] = s
	if _, err = e.Restore(context.Background(), r.ID); err == nil {
		t.Fatal("overwrote external change")
	}
}
func TestConcurrentUpdateDoesNotRollbackOntoNewApp(t *testing.T) {
	e, n, a, asset := fixture(t)
	n.changeExecutable = true
	r, err := e.Apply(context.Background(), a.ID, asset.ID)
	if err == nil || r.Status != "rollback-required" {
		t.Fatal(r, err)
	}
	if !n.states[a.Path].Custom() {
		t.Fatal("incorrect rollback into new app")
	}
}
func TestNewestBackupRestoredFirst(t *testing.T) {
	e, _, a, asset := fixture(t)
	r1, err := e.Apply(context.Background(), a.ID, asset.ID)
	if err != nil {
		t.Fatal(err)
	}
	asset2, err := e.ImportPNG("Second", "Fixture", pngBytes(color.RGBA{4, 5, 6, 255}))
	if err != nil {
		t.Fatal(err)
	}
	r2, err := e.Apply(context.Background(), a.ID, asset2.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = e.Restore(context.Background(), r1.ID); err == nil {
		t.Fatal("old backup overwritten newer change")
	}
	if _, err = e.Restore(context.Background(), r2.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = e.Restore(context.Background(), r1.ID); err != nil {
		t.Fatal(err)
	}
}
func TestBackupCommitFailurePreventsMutation(t *testing.T) {
	e, n, a, asset := fixture(t)
	_ = os.Remove(filepath.Join(e.Dir, "state.json"))
	_ = os.Mkdir(filepath.Join(e.Dir, "state.json"), 0700)
	if _, err := e.Apply(context.Background(), a.ID, asset.ID); err == nil {
		t.Fatal("expected state failure")
	}
	if n.sets != 0 {
		t.Fatal("modified without durable backup")
	}
}
func TestUnknownAppCannotBeMutated(t *testing.T) {
	e, n, _, asset := fixture(t)
	if _, err := e.Apply(context.Background(), "../../etc", asset.ID); err == nil {
		t.Fatal("accepted unknown app")
	}
	if n.sets != 0 {
		t.Fatal("called native operation")
	}
}
func TestSystemAppCannotBeMutated(t *testing.T) {
	e, n, a, asset := fixture(t)
	a.Protected = true
	e.Apps[a.ID] = a
	if _, err := e.Apply(context.Background(), a.ID, asset.ID); err == nil {
		t.Fatal("accepted system app")
	}
	if n.sets != 0 {
		t.Fatal("called native operation")
	}
}
func TestCorruptedAssetBlocked(t *testing.T) {
	e, n, a, asset := fixture(t)
	_ = os.WriteFile(filepath.Join(e.Dir, "assets", asset.ID+".png"), []byte("corrupt"), 0600)
	if _, err := e.Apply(context.Background(), a.ID, asset.ID); err == nil {
		t.Fatal("accepted corruption")
	}
	if n.sets != 0 {
		t.Fatal("called native")
	}
}
func TestSymlinkIconBlocked(t *testing.T) {
	e, n, a, asset := fixture(t)
	victim := filepath.Join(t.TempDir(), "victim")
	_ = os.WriteFile(victim, []byte("keep"), 0600)
	_ = os.Symlink(victim, filepath.Join(a.Path, "Icon\r"))
	if _, err := e.Apply(context.Background(), a.ID, asset.ID); err == nil {
		t.Fatal("accepted symlink")
	}
	if n.sets != 0 {
		t.Fatal("called native")
	}
	b, _ := os.ReadFile(victim)
	if string(b) != "keep" {
		t.Fatal("victim changed")
	}
}
func TestExecutablePathEscapeBlocked(t *testing.T) {
	e, n, a, asset := fixture(t)
	a.Executable = "../../outside"
	e.Apps[a.ID] = a
	if _, err := e.Apply(context.Background(), a.ID, asset.ID); err == nil {
		t.Fatal("accepted escape")
	}
	if n.sets != 0 {
		t.Fatal("called native")
	}
}
func TestExecutableSymlinkEscapeBlocked(t *testing.T) {
	e, _, a, asset := fixture(t)
	p := filepath.Join(a.Path, "Contents/MacOS/test")
	_ = os.Remove(p)
	_ = os.Symlink("/bin/true", p)
	if _, err := e.Apply(context.Background(), a.ID, asset.ID); err == nil {
		t.Fatal("accepted external executable")
	}
}
func TestPersistentRestoreAcrossRestart(t *testing.T) {
	e, n, a, asset := fixture(t)
	r, err := e.Apply(context.Background(), a.ID, asset.ID)
	if err != nil {
		t.Fatal(err)
	}
	second, err := Open(e.Dir, e.Home, n)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = second.Restore(context.Background(), r.ID); err != nil {
		t.Fatal(err)
	}
}
func TestCorruptStateIsNotOverwritten(t *testing.T) {
	e, n, _, _ := fixture(t)
	p := filepath.Join(e.Dir, "state.json")
	_ = os.WriteFile(p, []byte("{corrupt"), 0600)
	if _, err := Open(e.Dir, e.Home, n); err == nil {
		t.Fatal("ignored corrupt journal")
	}
	b, _ := os.ReadFile(p)
	if string(b) != "{corrupt" {
		t.Fatal("overwrote corrupt journal")
	}
}
func TestDiscoverySkipsNestedHelperApps(t *testing.T) {
	root := t.TempDir()
	for _, p := range []string{"Vendor/PhotoCraft.app/Contents/Helpers/Evil.app", "Other/WordCraft.app"} {
		_ = os.MkdirAll(filepath.Join(root, p), 0700)
	}
	ps, _ := Discover([]string{root})
	if len(ps) != 2 {
		t.Fatal(ps)
	}
}
func TestNewAppDiscoveredOnRescan(t *testing.T) {
	e, n, a, _ := fixture(t)
	if a.CraftID != "photocraft" {
		t.Fatal(a)
	}
	_ = os.RemoveAll(a.Path) // simulate removal of the original installation
	newPath := filepath.Join(e.Home, "NewCraft.app")
	_ = os.MkdirAll(newPath, 0700)
	n.app.Path = newPath
	n.app.Name = "NewCraft"
	n.app.BundleID = "ai.storyteller.newcraft"
	apps, _, err := e.Scan(context.Background())
	if err != nil || len(apps) != 1 || apps[0].CraftID != "newcraft" || apps[0].Equivalent != "" {
		t.Fatal(apps, err)
	}
}
func TestExactMatchingAndUnknownCounterparts(t *testing.T) {
	c := DefaultCatalog()
	for _, tc := range []struct {
		a  App
		id string
	}{{App{Name: "PhotoCraft"}, "photocraft"}, {App{Name: "NotPhotoCraftMalware"}, ""}, {App{Name: "Future", BundleID: "ai.storyteller.futurecraft"}, "futurecraft"}, {App{Name: "Launcher", BundleID: "ai.storyteller.artcraft-launcher"}, ""}} {
		id, _ := c.Match(tc.a)
		if id != tc.id {
			t.Fatal(tc, id)
		}
	}
}
func TestCatalogRejectsUnsafeIDs(t *testing.T) {
	c, err := ParseFeed([]byte(`{"release_info":{"apps":{"newcraft":{},"../../victim":{},"<script>":{}}}}`), DefaultCatalog())
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := c.Apps["newcraft"]; !ok {
		t.Fatal("missing new app")
	}
	if _, ok := c.Apps["../../victim"]; ok {
		t.Fatal("unsafe id")
	}
	if c.Apps["newcraft"] != "" {
		t.Fatal("invented counterpart")
	}
}
func TestInvalidFeedRetainsCatalogue(t *testing.T) {
	c := DefaultCatalog()
	for _, b := range []string{"invalid", `{"release_info":{"apps":{}}}`, `{"release_info":{"apps":[]}}`} {
		got, err := ParseFeed([]byte(b), c)
		if err == nil || len(got.Apps) != len(c.Apps) {
			t.Fatal(got, err)
		}
	}
}
func TestPortableProfileRoundTripDoesNotApply(t *testing.T) {
	e, n, a, asset := fixture(t)
	_, err := e.Apply(context.Background(), a.ID, asset.ID)
	if err != nil {
		t.Fatal(err)
	}
	b, err := e.ExportProfile()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), a.Path) {
		t.Fatal("profile leaked device path")
	}
	other, err := Open(filepath.Join(t.TempDir(), "data"), t.TempDir(), n)
	if err != nil {
		t.Fatal(err)
	}
	sets := n.sets
	if err = other.ImportProfile(b); err != nil {
		t.Fatal(err)
	}
	if n.sets != sets {
		t.Fatal("profile auto-applied")
	}
	if other.State.Preferences[a.BundleID].AssetID != asset.ID {
		t.Fatal("missing preference")
	}
}
func TestProfileChecksumCorruptionRejected(t *testing.T) {
	e, _, _, _ := fixture(t)
	b, _ := e.ExportProfile()
	var p Profile
	_ = json.Unmarshal(b, &p)
	p.Assets[0].PNG = []byte("bad")
	bad, _ := json.Marshal(p)
	if err := e.ImportProfile(bad); err == nil {
		t.Fatal("bad profile accepted")
	}
}
func TestProfileUnexpectedPathFieldRejected(t *testing.T) {
	e, _, _, _ := fixture(t)
	if err := e.ImportProfile([]byte(`{"schema":1,"application":"IconCraft","assets":[],"preferences":{},"path":"/tmp/pwn"}`)); err == nil {
		t.Fatal("unknown field accepted")
	}
}
func TestAtomicWriteDoesNotFollowDestinationSymlink(t *testing.T) {
	root := t.TempDir()
	victim := filepath.Join(root, "victim")
	_ = os.WriteFile(victim, []byte("keep"), 0600)
	target := filepath.Join(root, "target")
	_ = os.Symlink(victim, target)
	if err := AtomicWrite(target, []byte("new"), 0600); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(victim)
	if string(b) != "keep" {
		t.Fatal("followed symlink")
	}
}
func TestSnapshotComparisonIgnoresUnrelatedRootFlags(t *testing.T) {
	s := Snapshot{FinderInfo: make([]byte, 32)}
	x := cloneSnapshot(s)
	x.FinderInfo[0] = 99
	if !s.Equal(x) {
		t.Fatal("unrelated flags changed comparison")
	}
	x.FinderInfo[8] = 4
	if s.Equal(x) {
		t.Fatal("custom icon flag ignored")
	}
}
func TestRootScopeRejectsWholeDiskAndHome(t *testing.T) {
	e, _, _, _ := fixture(t)
	if e.AddRoot("/") == nil || e.AddRoot(e.Home) == nil {
		t.Fatal("accepted broad root")
	}
}
func TestDecodeStrictRejectsTrailingData(t *testing.T) {
	var s State
	if DecodeStrict([]byte(`{"schema":1} {}`), &s) == nil {
		t.Fatal("trailing JSON accepted")
	}
}
func TestTruncatedPNGRejected(t *testing.T) {
	e, _, _, _ := fixture(t)
	b := pngBytes(color.RGBA{1, 1, 1, 255})
	if _, err := e.ImportPNG("bad", "test", b[:len(b)/2]); err == nil {
		t.Fatal("truncated PNG accepted")
	}
}
func TestAssetDeduplicatedByContent(t *testing.T) {
	e, _, _, a := fixture(t)
	b, _ := ReadLimited(filepath.Join(e.Dir, "assets", a.ID+".png"), MaxIconBytes)
	other, err := e.ImportPNG("duplicate", "test", b)
	if err != nil || other.ID != a.ID || len(e.State.Assets) != 1 {
		t.Fatal(other, err)
	}
}
func TestRestoringNewestRetainsEarlierPreference(t *testing.T) {
	e, _, a, asset := fixture(t)
	if _, err := e.Apply(context.Background(), a.ID, asset.ID); err != nil {
		t.Fatal(err)
	}
	second, err := e.ImportPNG("second", "test", pngBytes(color.RGBA{50, 80, 20, 255}))
	if err != nil {
		t.Fatal(err)
	}
	r, err := e.Apply(context.Background(), a.ID, second.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = e.Restore(context.Background(), r.ID); err != nil {
		t.Fatal(err)
	}
	if e.State.Preferences[a.BundleID].AssetID != asset.ID {
		t.Fatal("lost prior active preference")
	}
}

// The optional detailed inspector must preserve useful results and surface
// malformed bundle warnings without changing the legacy Native interface.
type warningNative struct {
	*fakeNative
	warning string
	failure error
}

func (n *warningNative) InspectWithWarnings(ctx context.Context, paths []string) ([]App, []string, error) {
	apps, _ := n.fakeNative.Inspect(ctx, paths)
	return apps, []string{n.warning}, n.failure
}

func TestScanKeepsValidApplicationsAndMetadataWarnings(t *testing.T) {
	e, n, a, _ := fixture(t)
	e.Native = &warningNative{fakeNative: n, warning: "Broken.app: unreadable Info.plist"}
	_ = os.WriteFile(filepath.Join(a.Path, "Contents/Info.plist"), []byte("changed metadata"), 0600)
	apps, warnings, err := e.Scan(context.Background())
	if err != nil || len(apps) != 1 || apps[0].ID != a.ID {
		t.Fatalf("valid application was lost: apps=%v err=%v", apps, err)
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "Broken.app") {
		t.Fatalf("warning was hidden: %v", warnings)
	}
}

func TestScanBridgeFailureDoesNotErasePriorInventory(t *testing.T) {
	e, n, a, _ := fixture(t)
	e.Native = &warningNative{fakeNative: n, warning: "metadata read", failure: errors.New("bridge unavailable")}
	_ = os.WriteFile(filepath.Join(a.Path, "Contents/Info.plist"), []byte("changed metadata"), 0600)
	if _, _, err := e.Scan(context.Background()); err == nil {
		t.Fatal("bridge failure reported as success")
	}
	if got, err := e.GetApp(a.ID); err != nil || got.Path != a.Path {
		t.Fatal("last successful inventory was erased on failure")
	}
}
