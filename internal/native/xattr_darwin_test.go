//go:build darwin

package native

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestNativeExtendedAttributeRoundTrip(t *testing.T) {
	p := filepath.Join(t.TempDir(), "icon")
	if err := os.WriteFile(p, nil, 0600); err != nil {
		t.Fatal(err)
	}
	data := bytes.Repeat([]byte{1, 2, 3, 4}, 4096)
	if err := setAttr(p, "com.apple.ResourceFork", data); err != nil {
		t.Fatal(err)
	}
	got, err := attr(p, "com.apple.ResourceFork")
	if err != nil || !bytes.Equal(data, got) {
		t.Fatal("resource fork round trip", err)
	}
	all, err := attrs(p)
	if err != nil || !bytes.Equal(all["com.apple.ResourceFork"], data) {
		t.Fatal("attribute enumeration", err)
	}
	if err = removeAttr(p, "com.apple.ResourceFork"); err != nil {
		t.Fatal(err)
	}
}
func TestNativeSnapshotRestoreRetainsOtherFinderFlags(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(root, "Fixture.app")
	_ = os.Mkdir(p, 0700)
	m, err := New(filepath.Join(root, "state"))
	if err != nil {
		t.Fatal(err)
	}
	before, err := m.Snapshot(p)
	if err != nil {
		t.Fatal(err)
	}
	info := make([]byte, 32)
	info[8] = 4
	info[9] = 16
	if err = setAttr(p, "com.apple.FinderInfo", info); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(filepath.Join(p, "Icon\r"), nil, 0600)
	if err = m.Restore(context.Background(), p, before); err != nil {
		t.Fatal(err)
	}
	got, err := attr(p, "com.apple.FinderInfo")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 32 || got[8]&4 != 0 || got[9] != 16 {
		t.Fatal("unrelated Finder flag was lost")
	}
}
