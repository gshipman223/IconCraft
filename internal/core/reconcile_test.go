package core

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// An app launch/update that clears the Finder custom icon must be repaired
// by Reconcile, while an untouched skin must be left alone (no record spam).
func TestReconcileRepairsClearedIconOnly(t *testing.T) {
	e, n, a, asset := fixture(t)
	if _, err := e.Apply(context.Background(), a.ID, asset.ID); err != nil {
		t.Fatal(err)
	}
	recordsBefore := len(e.SnapshotState().Records)

	// Skin intact: reconcile is a no-op.
	res, err := e.Reconcile(context.Background())
	if err != nil || res.IconFixed != 0 || len(e.SnapshotState().Records) != recordsBefore {
		t.Fatalf("intact skin touched: %+v %v", res, err)
	}

	// Simulated launch/update wiping the custom icon.
	n.states[a.Path] = Snapshot{Exists: false}
	res, err = e.Reconcile(context.Background())
	if err != nil || res.IconFixed != 1 {
		t.Fatalf("cleared skin not repaired: %+v %v", res, err)
	}
	if !n.states[a.Path].Custom() {
		t.Fatal("icon not re-applied")
	}

	// A *different* custom icon (user changed it elsewhere) is respected.
	n.states[a.Path] = Snapshot{FinderInfo: append(make([]byte, 0, 32), func() []byte { b := make([]byte, 32); b[8] = 4; return b }()...), Exists: true}
	res, err = e.Reconcile(context.Background())
	if err != nil || res.IconFixed != 0 {
		t.Fatalf("foreign custom icon stomped: %+v %v", res, err)
	}
}

// A rename reverted externally (folder back at its original name) is
// re-applied; while the app is missing entirely nothing is forced.
func TestReconcileRepairsRevertedRename(t *testing.T) {
	e, _, a, _ := fixture(t)
	renamed, err := e.Rename(context.Background(), a.ID, "PhotoCraft Custom")
	if err != nil {
		t.Fatal(err)
	}
	// External revert: move the folder back to its original name.
	if err := os.Rename(renamed.Path, a.Path); err != nil {
		t.Fatal(err)
	}
	if _, _, err := e.Scan(context.Background()); err != nil {
		t.Fatal(err)
	}
	res, err := e.Reconcile(context.Background())
	if err != nil || res.NameFixed != 1 {
		t.Fatalf("reverted rename not repaired: %+v %v", res, err)
	}
	if _, err := os.Lstat(filepath.Join(filepath.Dir(a.Path), "PhotoCraft Custom.app")); err != nil {
		t.Fatal("custom name not restored:", err)
	}
	// Second pass: name already in place, nothing to do.
	res, err = e.Reconcile(context.Background())
	if err != nil || res.NameFixed != 0 {
		t.Fatalf("stable name touched: %+v %v", res, err)
	}
}
