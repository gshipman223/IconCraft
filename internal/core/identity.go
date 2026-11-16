package core

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

// RepairResult is what RepairIdentities did for bundles an older IconCraft
// had renamed or re-signed.
type RepairResult struct {
	Checked      int
	MovedBack    int
	IconsApplied int
	Notes        []string
}

// RepairIdentities puts apps back where a path-based launcher looks, and
// stops treating an internal full skin as something to re-apply.
//
// Older IconCraft versions renamed PhotoCraft.app to a custom folder and,
// for a full skin, edited CFBundleName / CFBundleDisplayName, replaced the
// bundle icon and ad-hoc re-signed. ArtCraft Launcher's macOS detector
// (storytold/craft-launcher, crates/native/src/detect.rs) only accepts a
// real directory at /Applications/<OfficialName>.app or
// ~/Applications/<OfficialName>.app whose CFBundleIdentifier is the app's
// id. A renamed folder is "not installed", so the launcher copies a second
// app beside it. A symlink at the official path is rejected by its
// installer. Sparkle and the Mac App Store also stop recognising an app
// whose Developer ID signature was replaced.
//
// Repair moves a renamed bundle back when the original path is free, and
// never deletes a second copy that is already there. It restores a full
// skin's backed-up files when they are still the bytes IconCraft wrote,
// then re-signs only so the app can still launch — the publisher's
// Developer ID signature cannot be rebuilt. A bundle an updater has
// already replaced is not written to. The custom name is kept as a label.
// The custom icon is re-applied with the Finder custom-icon mechanism,
// which does not modify sealed bundle contents.
func (e *Engine) RepairIdentities(ctx context.Context) RepairResult {
	var out RepairResult
	e.mu.Lock()
	renames := append([]RenameRecord(nil), e.State.Renames...)
	rebrands := append([]RebrandRecord(nil), e.State.Rebrands...)
	e.mu.Unlock()

	for _, nr := range renames {
		if nr.Status != "applied" || nr.From == "" || nr.From == nr.To || nr.BundleID == "" || nr.Suspended {
			continue
		}
		// Full rename is opt-in. Repair must not undo it, and must not
		// move an app that Pause for updates is holding at its original path.
		if e.currentMode(nr.BundleID) == NameModeFull || e.bundlePaused(nr.BundleID) {
			continue
		}
		out.Checked++
		note, moved := e.repairLegacyRename(ctx, nr)
		if note != "" {
			out.Notes = append(out.Notes, note)
		}
		if moved {
			out.MovedBack++
		}
	}
	for _, rec := range rebrands {
		if rec.Status != "applied" && rec.Status != "icon-pending" {
			continue
		}
		out.Checked++
		note, icon := e.retireRebrand(ctx, rec)
		if note != "" {
			out.Notes = append(out.Notes, note)
		}
		if icon {
			out.IconsApplied++
		}
	}
	return out
}

func (e *Engine) repairLegacyRename(ctx context.Context, nr RenameRecord) (note string, moved bool) {
	if err := ctx.Err(); err != nil {
		return nr.NewName + ": " + err.Error(), false
	}
	if runner, ok := e.Native.(interface {
		Running(context.Context, string) (bool, error)
	}); ok {
		running, err := runner.Running(ctx, nr.To)
		if err != nil {
			return nr.NewName + ": " + err.Error(), false
		}
		if running {
			return nr.NewName + ": quit it before IconCraft can put it back at " + filepath.Base(nr.From), false
		}
	}
	fromInfo, fromErr := os.Lstat(nr.From)
	toInfo, toErr := os.Lstat(nr.To)
	switch {
	case toErr == nil && os.IsNotExist(fromErr):
		if toInfo.Mode()&os.ModeSymlink != 0 || !toInfo.IsDir() {
			return nr.NewName + ": the renamed item is not an app folder; left it alone", false
		}
		if err := moveExclusive(nr.To, nr.From); err != nil {
			return nr.NewName + ": " + err.Error(), false
		}
		e.finishMoveBack(nr)
		return "", true
	case toErr == nil && fromErr == nil:
		if fromInfo.Mode()&os.ModeSymlink != 0 {
			e.markLeftAlone(nr)
			return fmt.Sprintf("%s: %s is a shortcut, which launchers do not accept as the installed app; left both copies alone", nr.NewName, filepath.Base(nr.From)), false
		}
		e.markDuplicate(nr)
		return fmt.Sprintf("%s: %s is still beside %s; IconCraft did not delete it. Launchers use %s.", nr.NewName, filepath.Base(nr.To), filepath.Base(nr.From), filepath.Base(nr.From)), false
	case os.IsNotExist(toErr) && fromErr == nil:
		e.collapseLabel(nr)
		return "", false
	default:
		return nr.NewName + ": app not found at either name; leaving it alone", false
	}
}

func (e *Engine) finishMoveBack(nr RenameRecord) {
	e.mu.Lock()
	defer e.mu.Unlock()
	for i := range e.State.Renames {
		r := &e.State.Renames[i]
		if r.ID != nr.ID {
			continue
		}
		r.To = r.From
		r.Status = "applied"
		r.Suspended = false
		e.relocateLocked(nr.To, nr.From, nr.BundleID, nr.NewName)
		break
	}
	_ = e.save()
	_ = e.saveInventoryLocked()
}

func (e *Engine) markLeftAlone(nr RenameRecord) {
	e.mu.Lock()
	defer e.mu.Unlock()
	for i := range e.State.Renames {
		if e.State.Renames[i].ID == nr.ID && e.State.Renames[i].Status == "applied" {
			e.State.Renames[i].Status = "left-alone"
		}
	}
	_ = e.save()
}

func (e *Engine) markDuplicate(nr RenameRecord) {
	e.mu.Lock()
	defer e.mu.Unlock()
	for i := range e.State.Renames {
		if e.State.Renames[i].ID == nr.ID && e.State.Renames[i].Status == "applied" {
			e.State.Renames[i].Status = "duplicate-left"
		}
	}
	old := nr.OldName
	if old == "" {
		old = bundleFolderName(nr.From)
	}
	e.ensureLabelLocked(nr.BundleID, nr.From, old, nr.NewName)
	_ = e.save()
	_ = e.saveInventoryLocked()
}

func (e *Engine) collapseLabel(nr RenameRecord) {
	e.mu.Lock()
	defer e.mu.Unlock()
	for i := range e.State.Renames {
		r := &e.State.Renames[i]
		if r.ID != nr.ID {
			continue
		}
		r.To = r.From
		r.Status = "applied"
		r.Suspended = false
		if cur, ok := e.Apps[appID(App{Path: r.From, BundleID: r.BundleID})]; ok {
			cur.Name = r.NewName
			cur.Renamed = r.NewName != bundleFolderName(r.From)
			e.Apps[cur.ID] = cur
		}
		break
	}
	_ = e.save()
	_ = e.saveInventoryLocked()
}

// relocateLocked retargets the in-memory app from oldPath to newPath.
// Caller holds e.mu. No-op when the app is not in the current inventory.
func (e *Engine) relocateLocked(oldPath, newPath, bundleID, label string) {
	var old App
	found := false
	for _, a := range e.Apps {
		if a.Path == oldPath && (bundleID == "" || a.BundleID == bundleID) {
			old = a
			found = true
			break
		}
	}
	if !found {
		return
	}
	next := old
	next.Path = newPath
	next.Name = label
	next.ID = appID(next)
	next.Renamed = label != "" && label != bundleFolderName(newPath)
	next.IconStamp = PreviewStamp(newPath)
	e.moveReferencesLocked(old, next)
}

// ensureLabelLocked records a display label at path without moving the
// bundle. Caller holds e.mu.
func (e *Engine) ensureLabelLocked(bundleID, path, oldName, newName string) {
	if bundleID == "" || path == "" || newName == "" || newName == bundleFolderName(path) {
		return
	}
	for _, nr := range e.State.Renames {
		if nr.Status == "applied" && nr.BundleID == bundleID && nr.To == path && nr.From == nr.To && nr.NewName == newName {
			return
		}
	}
	stub := App{BundleID: bundleID, Path: path, Name: oldName}
	stub.ID = appID(stub)
	e.State.Renames = append(e.State.Renames, RenameRecord{
		App: stub, ID: newID(), BundleID: bundleID, From: path, To: path,
		OldName: oldName, NewName: newName, Status: "applied",
	})
	if cur, ok := e.Apps[stub.ID]; ok {
		cur.Name = newName
		cur.Renamed = true
		e.Apps[cur.ID] = cur
	}
}

func (e *Engine) retireRebrand(ctx context.Context, rec RebrandRecord) (note string, icon bool) {
	if err := ctx.Err(); err != nil {
		return rec.Name + ": " + err.Error(), false
	}
	app, ok := e.appByBundle(rec.BundleID, rec.App.Path)
	if !ok {
		return rec.Name + ": full skin not retired; rescan needed", false
	}
	if runner, ok := e.Native.(interface {
		Running(context.Context, string) (bool, error)
	}); ok {
		running, err := runner.Running(ctx, app.Path)
		if err != nil {
			return rec.Name + ": " + err.Error(), false
		}
		if running {
			return rec.Name + ": quit it before IconCraft can remove the old full skin", false
		}
	}
	if rec.Status == "applied" {
		fp, err := Fingerprint(app)
		if err != nil {
			return rec.Name + ": " + err.Error(), false
		}
		if rec.AfterPrint != "" && fp == rec.AfterPrint {
			// Files are still the ones IconCraft wrote. Put the backed-up
			// plist and icon back. Resign is ad-hoc: the publisher signature
			// is already gone and cannot be reconstructed.
			if _, err := e.RestoreRebrand(ctx, app.ID); err != nil {
				return rec.Name + ": " + err.Error(), false
			}
		} else if err := e.setRebrandStatus(rec.ID, "migrated"); err != nil {
			return rec.Name + ": " + err.Error(), false
		}
	}
	if again, ok := e.appByBundle(rec.BundleID, app.Path); ok {
		app = again
	}
	e.mu.Lock()
	oldName := rec.OriginalName
	if oldName == "" {
		oldName = bundleFolderName(app.Path)
	}
	e.ensureLabelLocked(app.BundleID, app.Path, oldName, rec.Name)
	if err := e.save(); err != nil {
		e.mu.Unlock()
		return rec.Name + ": " + err.Error(), false
	}
	e.mu.Unlock()
	if rec.AssetID == "" {
		return "", false
	}
	if _, err := e.Apply(ctx, app.ID, rec.AssetID); err != nil {
		_ = e.setRebrandStatus(rec.ID, "icon-pending")
		return rec.Name + ": Finder icon deferred: " + err.Error(), false
	}
	if rec.Status == "icon-pending" {
		_ = e.setRebrandStatus(rec.ID, "migrated")
	}
	return "", true
}

func (e *Engine) setRebrandStatus(id, status string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	for i := range e.State.Rebrands {
		if e.State.Rebrands[i].ID == id {
			e.State.Rebrands[i].Status = status
			return e.save()
		}
	}
	return nil
}
