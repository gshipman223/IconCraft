package native

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// checkMetadataVariants exercises the actual JXA bridge on a Mac using valid
// XML/binary plists alongside malformed and missing plists. It only creates
// disposable directories; it never edits the supplied fixture or installed apps.
func (m *Mac) checkMetadataVariants(ctx context.Context, parent, fixture string) error {
	dir, err := os.MkdirTemp(parent, "metadata-variants-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	data, err := os.ReadFile(filepath.Join(fixture, "Contents", "Info.plist"))
	if err != nil {
		return err
	}
	binary := filepath.Join(dir, "Binary Metadata.app")
	broken := filepath.Join(dir, "Malformed Metadata.app")
	missing := filepath.Join(dir, "Missing Metadata.app")
	for _, p := range []string{binary, broken, missing} {
		if err := os.MkdirAll(filepath.Join(p, "Contents"), 0700); err != nil {
			return err
		}
	}
	binaryPlist := filepath.Join(binary, "Contents", "Info.plist")
	if err = os.WriteFile(binaryPlist, data, 0600); err != nil {
		return err
	}
	if err = os.WriteFile(filepath.Join(broken, "Contents", "Info.plist"), []byte("not a property list"), 0600); err != nil {
		return err
	}
	convert, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	out, err := exec.CommandContext(convert, "/usr/bin/plutil", "-convert", "binary1", "--", binaryPlist).CombinedOutput()
	if err != nil {
		return fmt.Errorf("create binary metadata fixture: %v: %s", err, out)
	}
	apps, warnings, err := m.InspectWithWarnings(ctx, []string{missing, fixture, broken, binary})
	if err != nil {
		return err
	}
	if len(apps) != 2 || len(warnings) != 2 {
		return fmt.Errorf("expected 2 valid apps and 2 skipped-bundle warnings; got %d apps and %d warnings: %v", len(apps), len(warnings), warnings)
	}
	for i, path := range []string{fixture, binary} {
		if apps[i].Path != path || apps[i].BundleID != "io.iconcraft.disposable-test" || apps[i].Executable != "IconCraftTest" {
			return fmt.Errorf("incorrect metadata decoded for %s", path)
		}
	}
	return nil
}
