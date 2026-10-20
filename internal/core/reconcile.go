package core

import (
	"context"
	"os"
)

// ReconcileResult reports what the skin-persistence pass found and repaired.
type ReconcileResult struct {
	Checked   int      `json:"checked"`
	IconFixed int      `json:"iconFixed"`
	NameFixed int      `json:"nameFixed"`
	Skipped   []string `json:"skipped,omitempty"`
}

// Reconcile re-applies skins that something else reverted — the classic case
// being an app (or its updater) clearing its Finder custom icon, or restoring
// its original folder name, on launch/update. It only repairs, never invents:
//
//   - Icons: re-applied through the full Apply safety path, and only when the
//     app currently has NO custom icon at all. A different custom icon the
//     user set elsewhere is respected, not stomped.
//   - Names: re-renamed through Rename (which refuses running apps, name
//     collisions and changed bundles — those are skipped and retried on a
//     later pass, e.g. after the app quits).
//
// Callers must populate the inventory with Scan first. This is what makes a
// skin "stick" in Finder/Applications/Dock-without touching the inside of
// any app bundle. (A *running* app presents its own icon/name from inside
// its bundle; macOS gives the app that right, and we deliberately do not
// override it — that would require editing or injecting into the app.)
func (e *Engine) Reconcile(ctx context.Context) (ReconcileResult, error) {
	out := ReconcileResult{}
	state := e.SnapshotState()

	// Names first: paths determine app IDs used below.
	appliedRenames := map[string]RenameRecord{}
	for _, nr := range state.Renames {
		if nr.Status == "applied" {
			appliedRenames[nr.BundleID] = nr // newest wins (slice is chronological)
		}
	}
	for _, nr := range appliedRenames {
		out.Checked++
		if _, err := os.Lstat(nr.To); err == nil {
			continue // custom name still in place
		}
		if _, err := os.Lstat(nr.From); err != nil {
			out.Skipped = append(out.Skipped, nr.NewName+": app not found at either name; leaving it alone")
			continue
		}
		app, ok := e.appByPath(nr.From)
		if !ok {
			out.Skipped = append(out.Skipped, nr.NewName+": not in inventory; rescan needed")
			continue
		}
		if _, err := e.Rename(ctx, app.ID, nr.NewName); err != nil {
			// Running app, collision, or changed bundle: skip now, the
			// watcher retries on its next pass.
			out.Skipped = append(out.Skipped, nr.NewName+": "+err.Error())
			continue
		}
		out.NameFixed++
	}

	// Latest live icon record per app.
	latest := map[string]Record{}
	for _, r := range state.Records {
		if isLive(r.Status) {
			latest[r.App.BundleID] = r
		}
	}
	for _, r := range latest {
		out.Checked++
		app, ok := e.appByBundle(r.App.BundleID)
		if !ok {
			out.Skipped = append(out.Skipped, r.App.Name+": not in inventory; rescan needed")
			continue
		}
		snap, err := e.Native.Snapshot(app.Path)
		if err != nil {
			out.Skipped = append(out.Skipped, app.Name+": "+err.Error())
			continue
		}
		if snap.Custom() {
			continue // skin still on (or user set another one — respect it)
		}
		if _, err := e.Apply(ctx, app.ID, r.AssetID); err != nil {
			out.Skipped = append(out.Skipped, app.Name+": "+err.Error())
			continue
		}
		out.IconFixed++
	}
	return out, nil
}

func (e *Engine) appByPath(path string) (App, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, a := range e.Apps {
		if a.Path == path {
			return a, true
		}
	}
	return App{}, false
}

func (e *Engine) appByBundle(bundleID string) (App, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, a := range e.Apps {
		if a.BundleID == bundleID {
			return a, true
		}
	}
	return App{}, false
}
