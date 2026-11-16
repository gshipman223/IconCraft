package core

import (
	"context"
)

// ReconcileResult reports what the skin-persistence pass found and repaired.
type ReconcileResult struct {
	Checked   int      `json:"checked"`
	IconFixed int      `json:"iconFixed"`
	NameFixed int      `json:"nameFixed"`
	Skipped   []string `json:"skipped,omitempty"`
}

// Reconcile keeps a customisation without hiding the app from its updater.
//
//   - While Pause for updates is on, folders stay at the publisher's name
//     and custom icons are not put back. The keeper takes that same path.
//   - In the default Icon only mode a display name is a label. It is not
//     re-applied by renaming the .app folder.
//   - In Full rename mode a missing custom folder is renamed again, matched
//     by bundle id, after an update puts the publisher's folder back. A
//     second copy beside it is left in place.
//   - Icons are re-applied with the Finder custom-icon mechanism when the
//     custom icon is gone, or when the bundle's Info.plist was replaced
//     (an update) and the icon no longer matches the one IconCraft set.
//     A different custom icon on an unchanged bundle is left alone.
//   - Bundles an older build renamed or re-signed are repaired first
//     (RepairIdentities), except a full rename the user still wants.
//     Internal full skins are not written again.
//
// Callers must populate the inventory with Scan first. A running app still
// shows its own Dock icon and menu-bar name from inside the signed bundle.
// macOS does not let another process change those without editing the bundle.
func (e *Engine) Reconcile(ctx context.Context) (ReconcileResult, error) {
	out := ReconcileResult{}
	if e.UpdatesPaused() {
		// Finish putting apps back where launchers look. Do not rename
		// them again and do not put custom icons back while paused.
		held := e.ContinuePause(ctx, "")
		out.NameFixed += held.Moved
		out.Skipped = append(out.Skipped, held.Notes...)
		return out, nil
	}
	held := e.ContinuePause(ctx, "")
	out.Skipped = append(out.Skipped, held.Notes...)
	resumed := e.ResumeSuspended(ctx, "")
	out.NameFixed += resumed.Moved
	out.IconFixed += resumed.Icons
	out.Skipped = append(out.Skipped, resumed.Notes...)
	repair := e.RepairIdentities(ctx)
	out.Checked += repair.Checked
	out.NameFixed += repair.MovedBack
	out.IconFixed += repair.IconsApplied
	out.Skipped = append(out.Skipped, repair.Notes...)
	synced := e.SyncNameModes(ctx, "")
	out.NameFixed += synced.Moved
	out.Skipped = append(out.Skipped, synced.Notes...)

	state := e.SnapshotState()

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
		if e.bundlePaused(app.BundleID) {
			continue
		}
		// One-time icon-scale migration: skins applied before the macOS
		// icon-grid fitting existed may be full-bleed publisher artwork,
		// which renders larger than native icons. Refit only when the
		// current icon is still exactly the one this record applied.
		needsFit := false
		if r.IconFit < IconFitVersion {
			if needs, err := e.assetNeedsFit(r.AssetID); err == nil {
				needsFit = needs
				if !needs {
					_ = e.markIconFit(r.ID, false)
				}
			}
		}
		snap, err := e.Native.Snapshot(app.Path)
		if err != nil {
			out.Skipped = append(out.Skipped, app.Name+": "+err.Error())
			continue
		}
		// Info.plist replaced means an updater rewrote the bundle. The Finder
		// icon lives outside the seal and is usually wiped by that replace.
		// Re-apply unless the icon on disk is still exactly the one we set.
		bundleReplaced := app.InfoStamp != "" && r.App.InfoStamp != "" && app.InfoStamp != r.App.InfoStamp
		iconIsOurs := r.After != nil && snap.Equal(*r.After)
		if snap.Custom() && snap.Exists && (!bundleReplaced || iconIsOurs) {
			if needsFit {
				if err := e.refitFinderRecord(ctx, app, r.ID); err == nil {
					out.IconFixed++
				} else {
					out.Skipped = append(out.Skipped, app.Name+": icon resize deferred: "+err.Error())
				}
			}
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

	// Internal full skins are not re-applied. RepairIdentities retires any
	// that are still marked applied. Re-writing CFBundleName or the bundle
	// icon and re-signing would hide the app from Sparkle and the App Store
	// again after the vendor had just replaced it.
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
