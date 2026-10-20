package native

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/gshipman223/IconCraft/internal/core"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"
)

type Check struct {
	Name   string `json:"name"`
	Passed bool   `json:"passed"`
	Detail string `json:"detail,omitempty"`
}
type Report struct {
	Version string  `json:"version"`
	OS      string  `json:"os"`
	Arch    string  `json:"arch"`
	Time    string  `json:"time"`
	Passed  bool    `json:"passed"`
	Checks  []Check `json:"checks"`
	Scope   string  `json:"scope"`
}

func (m *Mac) SelfTest(ctx context.Context) Report {
	r := Report{Version: core.Version, OS: runtime.GOOS, Arch: runtime.GOARCH, Time: time.Now().UTC().Format(time.RFC3339), Checks: []Check{}, Scope: "Disposable fixture only; installed user apps are not modified. Finder/Dock appearance and Gatekeeper still require interactive acceptance testing."}
	add := func(name string, err error) bool {
		c := Check{Name: name, Passed: err == nil}
		if err != nil {
			c.Detail = err.Error()
		}
		r.Checks = append(r.Checks, c)
		return err == nil
	}
	if runtime.GOOS != "darwin" {
		add("Native operating system", fmt.Errorf("running on %s, not macOS", runtime.GOOS))
		return r
	}
	add("Native operating system", nil)
	temp, err := os.MkdirTemp("", "iconcraft-native-checks-")
	if !add("Create isolated fixture", err) {
		return r
	}
	defer os.RemoveAll(temp)
	temp, err = filepath.EvalSymlinks(temp)
	if !add("Resolve fixture path", err) {
		return r
	}
	app := filepath.Join(temp, "IconCraft Test.app")
	err = os.MkdirAll(filepath.Join(app, "Contents/MacOS"), 0700)
	if !add("Create test app bundle", err) {
		return r
	}
	plist := `<?xml version="1.0" encoding="UTF-8"?><!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd"><plist version="1.0"><dict><key>CFBundleIdentifier</key><string>io.iconcraft.disposable-test</string><key>CFBundleName</key><string>IconCraft Test</string><key>CFBundleExecutable</key><string>IconCraftTest</string><key>CFBundlePackageType</key><string>APPL</string><key>CFBundleShortVersionString</key><string>1.0</string></dict></plist>`
	err = os.WriteFile(filepath.Join(app, "Contents/Info.plist"), []byte(plist), 0600)
	if !add("Write test metadata", err) {
		return r
	}
	exe, err := os.ReadFile("/usr/bin/true")
	if !add("Read inert system test executable", err) {
		return r
	}
	err = os.WriteFile(filepath.Join(app, "Contents/MacOS/IconCraftTest"), exe, 0700)
	if !add("Install fixture executable", err) {
		return r
	}
	out, err := exec.CommandContext(ctx, "/usr/bin/codesign", "--force", "--sign", "-", "--timestamp=none", app).CombinedOutput()
	if err != nil {
		err = fmt.Errorf("%v: %s", err, out)
	}
	if !add("Ad-hoc sign disposable fixture", err) {
		return r
	}
	apps, warnings, err := m.InspectWithWarnings(ctx, []string{app})
	if err == nil && (len(apps) != 1 || apps[0].BundleID != "io.iconcraft.disposable-test") {
		err = fmt.Errorf("unexpected bundle inspection result; metadata warnings: %v", warnings)
	}
	if !add("Read XML bundle metadata through the macOS bridge", err) {
		return r
	}
	if !add("Read XML/binary metadata and isolate unreadable bundles", m.checkMetadataVariants(ctx, temp, app)) {
		return r
	}
	before, err := m.Snapshot(app)
	if !add("Read native FinderInfo and resource fork", err) {
		return r
	}
	donor := "/System/Applications/TextEdit.app"
	if _, err = os.Stat(donor); err != nil {
		donor = "/System/Library/CoreServices/Finder.app"
	}
	png, err := m.ExportIcon(ctx, donor, true)
	if !add("Extract an installed application's actual icon", err) {
		return r
	}
	e, err := core.Open(filepath.Join(temp, "state"), temp, m)
	if !add("Initialize isolated backup journal", err) {
		return r
	}
	e.Roots = []string{temp}
	found, _, err := e.Scan(ctx)
	if err == nil && len(found) != 1 {
		err = fmt.Errorf("expected one fixture, got %d", len(found))
	}
	if !add("Discover fixture through the app scanner", err) {
		return r
	}
	a, err := e.ImportPNG("Native test icon", "Native diagnostic donor", png)
	if !add("Import and checksum icon", err) {
		return r
	}
	record, err := e.Apply(ctx, found[0].ID, a.ID)
	if !add("Apply with durable backup and signature verification", err) {
		return r
	}
	_, err = e.Restore(ctx, record.ID)
	if !add("Restore the initial icon", err) {
		return r
	}
	after, err := m.Snapshot(app)
	if err == nil && !before.Equal(after) {
		err = fmt.Errorf("initial metadata was not restored exactly")
	}
	if !add("Exact original-icon round trip", err) {
		return r
	}
	// Start with a third-party custom icon, then change and restore it.
	path, err := e.AssetPath(a.ID)
	if err == nil {
		err = m.Set(ctx, app, path)
	}
	if !add("Seed a pre-existing custom icon", err) {
		return r
	}
	existing, err := m.Snapshot(app)
	if !add("Capture pre-existing custom icon bytes", err) {
		return r
	}
	next, err := e.Apply(ctx, found[0].ID, a.ID)
	if !add("Replace a pre-existing custom icon", err) {
		return r
	}
	_, err = e.Restore(ctx, next.ID)
	if !add("Restore a pre-existing custom icon", err) {
		return r
	}
	again, err := m.Snapshot(app)
	if err == nil && !existing.Equal(again) {
		err = fmt.Errorf("pre-existing custom icon bytes changed")
	}
	if !add("Exact pre-existing-icon round trip", err) {
		return r
	}
	status, err := m.Signature(ctx, app)
	if err == nil && status != "valid" {
		err = fmt.Errorf("fixture no longer has a valid signature")
	}
	if !add("Signature remains valid after restoration", err) {
		return r
	}
	profile, err := e.ExportProfile()
	if !add("Export portable preferences and icon assets", err) {
		return r
	}
	err = e.ImportProfile(profile)
	if !add("Import profile without changing apps", err) {
		return r
	}
	r.Passed = true
	return r
}
func WriteReport(path string, r Report) error {
	b, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	return core.AtomicWrite(path, b, 0600)
}
