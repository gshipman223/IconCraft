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
		app, ok := e.appByBundle(r.App.BundleID, r.App.Path)
		if !ok {
			out.Skipped = append(out.Skipped, r.App.Name+": not in inventory; rescan needed")
			continue
		}
		snap, err := e.Native.Snapshot(app.Path)
		if err != nil {
			out.Skipped = append(out.Skipped, app.Name+": "+err.Error())
			continue
		}
		if snap.Custom() && snap.Exists {
			continue // skin still on (or user set another one — respect it)
		}
		// A custom-icon bit with no Icon file is stale residue of a
		// refused/removed change (Finder shows the generic icon):
		// fall through and re-apply so the skin actually returns.
		if _, err := e.Apply(ctx, app.ID, r.AssetID); err != nil {
			out.Skipped = append(out.Skipped, app.Name+": "+err.Error())
			continue
		}
		out.IconFixed++
	}

	// Full (internal) skins: an updater that reinstalls the bundle puts the
	// original plist/icon back — recognisable because the fingerprint returns
	// to the pre-skin value. Re-apply the same recorded name+icon. If the
	// fingerprint is something else entirely, the app genuinely changed:
	// don't force it, just report.
	latestRebrand := map[string]RebrandRecord{}
	for _, r := range state.Rebrands {
		if r.Status == "applied" {
			latestRebrand[r.BundleID] = r
		}
	}
	for _, r := range latestRebrand {
		out.Checked++
		app, ok := e.appByBundle(r.BundleID, r.App.Path)
		if !ok {
			continue
		}
		fp, err := Fingerprint(app)
		if err != nil {
			out.Skipped = append(out.Skipped, app.Name+" (full skin): "+err.Error())
			continue
		}
		switch fp {
		case r.AfterPrint:
			// Full skin intact.
		case r.BeforePrint:
			if _, err := e.Rebrand(ctx, app.ID, r.AssetID, r.Name); err != nil {
				out.Skipped = append(out.Skipped, app.Name+" (full skin): "+err.Error())
			} else {
				out.IconFixed++
			}
		default:
			out.Skipped = append(out.Skipped, app.Name+" (full skin): app changed since skinning (update?); open IconCraft to review")
		}
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

// appByBundle finds an app by bundle ID, preferring the journalled path:
// cloned apps can share a bundle ID, and picking an arbitrary duplicate
// could repair (or rename) the wrong app.
func (e *Engine) appByBundle(bundleID string, preferPath ...string) (App, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	var fallback *App
	for _, a := range e.Apps {
		if a.BundleID != bundleID {
			continue
		}
		if len(preferPath) > 0 && preferPath[0] != "" && a.Path == preferPath[0] {
			return a, true
		}
		if fallback == nil || a.Path < fallback.Path {
			v := a
			fallback = &v
		}
	}
	if fallback != nil {
		return *fallback, true
	}
	return App{}, false
}
