package core

import (
	"context"
	"image/color"
	"os"
	"strings"
	"testing"
)

// refusingNative models macOS App Management refusing the icon write
// atomically: Set fails and nothing on disk changes.
type refusingNative struct {
	*fakeNative
	refuse   bool
	restores int
}

func (r *refusingNative) Set(ctx context.Context, p, icon string) error {
	if r.refuse {
		return errRefused
	}
	return r.fakeNative.Set(ctx, p, icon)
}
func (r *refusingNative) Restore(ctx context.Context, p string, s Snapshot) error {
	r.restores++
	return r.fakeNative.Restore(ctx, p, s)
}

var errRefused = &refusedError{}

type refusedError struct{}

func (*refusedError) Error() string { return "macOS refused the change" }

// Regression: a refused icon write must not wedge the app in
// "rollback-required". When nothing was written, Apply closes the record
// as a clean failure without attempting a rollback write, and the app
// stays usable.
func TestAtomicRefusalSkipsRollbackWrite(t *testing.T) {
	e, n, a, asset := fixture(t)
	refusal := &refusingNative{fakeNative: n, refuse: true}
	e.Native = refusal
	if _, err := e.Apply(context.Background(), a.ID, asset.ID); err == nil {
		t.Fatal("expected apply failure")
	}
	if refusal.restores != 0 {
		t.Fatalf("rollback write attempted though nothing changed (%d restores)", refusal.restores)
	}
	st := e.SnapshotState()
	last := st.Records[len(st.Records)-1]
	if last.Status != "failed" {
		t.Fatalf("record should close as failed, got %q (%s)", last.Status, last.Error)
	}
	// Not blocked: once macOS allows it, applying works again.
	refusal.refuse = false
	if _, err := e.Apply(context.Background(), a.ID, asset.ID); err != nil {
		t.Fatal("apply after refusal should succeed:", err)
	}
}

func TestProbeFailureBlocksBeforeJournal(t *testing.T) {
	e, n, a, asset := fixture(t)
	n.probeErr = os.ErrPermission
	before := len(e.SnapshotState().Records)
	if _, err := e.Apply(context.Background(), a.ID, asset.ID); err == nil {
		t.Fatal("probe failure must block apply")
	} else if !strings.Contains(err.Error(), "App Management") {
		t.Fatalf("error should guide the user to App Management, got: %s", err.Error())
	}
	if len(e.SnapshotState().Records) != before {
		t.Fatal("no record should be journalled when the probe fails")
	}
	// Rebrand is probed too.
	if _, err := e.Rebrand(context.Background(), a.ID, asset.ID, "New Name"); err == nil {
		t.Fatal("probe failure must block rebrand")
	}
}

func TestScanAutoHealsLayeredStuckRecord(t *testing.T) {
	e, n, a, asset := fixture(t)
	rec1, err := e.Apply(context.Background(), a.ID, asset.ID)
	if err != nil {
		t.Fatal(err)
	}
	other, err := e.ImportPNG("Other", "Fixture", pngBytes(color.RGBA{7, 7, 7, 255}))
	if err != nil {
		t.Fatal(err)
	}
	rec2, err := e.Apply(context.Background(), a.ID, other.ID)
	if err != nil {
		t.Fatal(err)
	}
	// The icon later reverted externally to the true original (the state
	// before layer 1), while layer 2 sits stuck at rollback-required: its
	// own "before" was the layer-1 skin, so only the layered rule heals it.
	if err := n.Restore(context.Background(), a.Path, rec1.Before); err != nil {
		t.Fatal(err)
	}
	e.mu.Lock()
	for i := range e.State.Records {
		if e.State.Records[i].ID == rec2.ID {
			e.State.Records[i].Status = "rollback-required"
		}
	}
	_ = e.save()
	e.mu.Unlock()
	if _, _, err := e.Scan(context.Background()); err != nil {
		t.Fatal(err)
	}
	byID := map[string]string{}
	for _, r := range e.SnapshotState().Records {
		byID[r.ID] = r.Status
	}
	if byID[rec2.ID] != "failed" {
		t.Fatalf("layered stuck record not healed: %q", byID[rec2.ID])
	}
	if byID[rec1.ID] != "applied" {
		t.Fatalf("earlier applied layer must stay live for the keeper: %q", byID[rec1.ID])
	}
}

func TestScanAutoHealsStaleFinderInfoBit(t *testing.T) {
	e, n, a, asset := fixture(t)
	rec, err := e.Apply(context.Background(), a.ID, asset.ID)
	if err != nil {
		t.Fatal(err)
	}
	// Icon file removed externally; only the stale custom-icon bit in
	// FinderInfo remains. The app's visible icon is its original again.
	stale := Snapshot{Exists: false, FinderInfo: make([]byte, 32)}
	stale.FinderInfo[8] = 4
	n.states[a.Path] = stale
	e.mu.Lock()
	for i := range e.State.Records {
		if e.State.Records[i].ID == rec.ID {
			e.State.Records[i].Status = "rollback-required"
		}
	}
	_ = e.save()
	e.mu.Unlock()
	if _, _, err := e.Scan(context.Background()); err != nil {
		t.Fatal(err)
	}
	for _, r := range e.SnapshotState().Records {
		if r.ID == rec.ID && r.Status != "failed" {
			t.Fatalf("stale-bit record not auto-healed, status=%q", r.Status)
		}
	}
}

func TestScanAutoHealsRefusedRecords(t *testing.T) {
	e, n, a, asset := fixture(t)
	rec, err := e.Apply(context.Background(), a.ID, asset.ID)
	if err != nil {
		t.Fatal(err)
	}
	// Put the icon state back to the original (as if the change never
	// landed) while the record sits stuck at rollback-required.
	if err := n.Restore(context.Background(), a.Path, rec.Before); err != nil {
		t.Fatal(err)
	}
	e.mu.Lock()
	for i := range e.State.Records {
		if e.State.Records[i].ID == rec.ID {
			e.State.Records[i].Status = "rollback-required"
		}
	}
	_ = e.save()
	e.mu.Unlock()
	if _, _, err := e.Scan(context.Background()); err != nil {
		t.Fatal(err)
	}
	for _, r := range e.SnapshotState().Records {
		if r.ID == rec.ID && r.Status != "failed" {
			t.Fatalf("stuck record not auto-healed, status=%q", r.Status)
		}
	}
}
