package core

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"html"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// The Originals Vault: before IconCraft changes an app for the first time,
// it saves the app's original identity here — original folder/display name,
// original internal name, and the original icon as a checksummed asset —
// keyed by the app's stable bundle ID. Paths and app IDs change when a
// folder is renamed, moved, or the app is replaced; the bundle ID does not.
// Restore (RestoreFromVault) therefore finds the saved original under the
// app's *new* identifier and rebuilds from what was saved.

type bundleNamer interface {
	BundleName(context.Context, string) (string, error)
}

var plistStringRe = map[string]*regexp.Regexp{
	"CFBundleDisplayName": regexp.MustCompile(`(?s)<key>CFBundleDisplayName</key>\s*<string>([^<]*)</string>`),
	"CFBundleName":        regexp.MustCompile(`(?s)<key>CFBundleName</key>\s*<string>([^<]*)</string>`),
}

// plistNameFromBytes extracts the internal name from XML plist bytes
// (binary plists yield ""; the native BundleName reader covers those).
func plistNameFromBytes(b []byte) string {
	for _, key := range []string{"CFBundleDisplayName", "CFBundleName"} {
		if m := plistStringRe[key].FindSubmatch(b); m != nil {
			if s := strings.TrimSpace(html.UnescapeString(string(m[1]))); s != "" {
				return s
			}
		}
	}
	return ""
}

// captureOriginalLocked records the app's original identity in the vault the
// first time IconCraft is about to change it. Caller holds e.mu and calls
// this BEFORE mutating anything. If history already exists (the app was
// skinned before the vault existed), the entry is backfilled from that
// history instead — never from the app's current, already-skinned state.
func (e *Engine) captureOriginalLocked(ctx context.Context, a App) {
	if a.BundleID == "" {
		return
	}
	if _, ok := e.State.Vault[a.BundleID]; ok {
		return
	}
	entry := SavedOriginal{BundleID: a.BundleID, AppName: a.Name, FolderName: strings.TrimSuffix(filepath.Base(a.Path), ".app"), Captured: now()}
	pristine := true
	for _, nr := range e.State.Renames {
		if nr.BundleID == a.BundleID && (nr.Status == "applied" || nr.Status == "prepared") {
			entry.AppName = nr.OldName
			entry.FolderName = strings.TrimSuffix(filepath.Base(nr.From), ".app")
			pristine = false
			break
		}
	}
	for _, r := range e.State.Records {
		if r.App.BundleID == a.BundleID && isLive(r.Status) {
			pristine = false
			break
		}
	}
	rebranded := false
	for _, r := range e.State.Rebrands {
		if r.BundleID == a.BundleID {
			if r.OriginalName != "" && entry.InternalName == "" {
				entry.InternalName = r.OriginalName
			}
			if r.Status == "applied" {
				rebranded = true
				pristine = false
			}
		}
	}
	if !rebranded {
		if namer, ok := e.Native.(bundleNamer); ok {
			if name, err := namer.BundleName(ctx, a.Path); err == nil && name != "" {
				entry.InternalName = name
			}
		}
	}
	e.State.Vault[a.BundleID] = entry
	if pristine {
		e.savePristineIconLocked(ctx, a)
	}
	_ = e.save()
}

// savePristineIconLocked exports the untouched app's icon into the vault as
// a checksummed asset. Caller holds e.mu; the entry must already exist in
// State.Vault. Only valid while the app is untouched — afterwards the
// exported icon could already be a skin, and the journals (Record.Before /
// rebrand backups) are the source of truth instead.
func (e *Engine) savePristineIconLocked(ctx context.Context, a App) {
	entry, ok := e.State.Vault[a.BundleID]
	if !ok || entry.IconAssetID != "" {
		return
	}
	if b, err := e.Native.ExportIcon(ctx, a.Path, true); err == nil && ValidatePNG(b) == nil {
		if asset, err := e.importPNGBytesLocked("Original icon — "+entry.AppName, "Saved to the Originals Vault before IconCraft changed "+a.BundleID, b); err == nil {
			entry.IconAssetID = asset.ID
			e.State.Vault[a.BundleID] = entry
		}
	}
}

// BackfillVaultIcons fills in missing vault original icons from the change
// journals: apps first skinned before the vault existed have no pristine
// export, but the Finder-icon journal keeps a PNG of the pre-change state
// and the full-skin journal keeps the original icon file. Backfilled
// entries are persisted so the UI can always show the real original icon.
func (e *Engine) BackfillVaultIcons(ctx context.Context) {
	e.mu.Lock()
	defer e.mu.Unlock()
	changed := false
	for bundle, entry := range e.State.Vault {
		if entry.IconAssetID != "" {
			continue
		}
		if png, ok := e.originalIconFromJournalsLocked(bundle); ok && ValidatePNG(png) == nil {
			if asset, err := e.importPNGBytesLocked("Original icon — "+entry.AppName, "Recovered from IconCraft's change journal for "+bundle, png); err == nil {
				entry.IconAssetID = asset.ID
				e.State.Vault[bundle] = entry
				changed = true
			}
			continue
		}
		// No journal source: if the app itself is untouched again (every
		// change undone), its current icon IS the original.
		live := false
		for _, r := range e.State.Records {
			if r.App.BundleID == bundle && isLive(r.Status) {
				live = true
				break
			}
		}
		if !live {
			for _, r := range e.State.Rebrands {
				if r.BundleID == bundle && r.Status == "applied" {
					live = true
					break
				}
			}
		}
		if live {
			continue
		}
		for _, nr := range e.State.Renames {
			if nr.BundleID == bundle && (nr.Status == "applied" || nr.Status == "prepared") {
				live = true
				break
			}
		}
		if live {
			continue
		}
		// Inline app lookup: appByBundle takes e.mu, which we already hold.
		for _, a := range e.Apps {
			if a.BundleID != bundle {
				continue
			}
			e.savePristineIconLocked(ctx, a)
			if e.State.Vault[bundle].IconAssetID != "" {
				changed = true
			}
			break
		}
	}
	if changed {
		_ = e.save()
	}
}

// originalIconFromJournalsLocked recovers the pre-change icon for a bundle
// from the change journals: the earliest Finder-icon record's Before
// snapshot, or the earliest full-skin backup's saved icon file. Caller
// holds e.mu. Returns PNG bytes.
func (e *Engine) originalIconFromJournalsLocked(bundle string) ([]byte, bool) {
	var earliestRec *Record
	for i := range e.State.Records {
		r := &e.State.Records[i]
		if r.App.BundleID != bundle || len(r.Before.Data) == 0 {
			continue
		}
		if earliestRec == nil || r.Created < earliestRec.Created {
			earliestRec = r
		}
	}
	if earliestRec != nil {
		if pngB, ok := asPNG(earliestRec.Before.Data); ok {
			return pngB, true
		}
	}
	var firstRebrand *RebrandRecord
	for i := range e.State.Rebrands {
		r := &e.State.Rebrands[i]
		if r.BundleID != bundle || !r.IconExisted {
			continue
		}
		if firstRebrand == nil || r.Created < firstRebrand.Created {
			firstRebrand = r
		}
	}
	if firstRebrand != nil {
		if b, err := ReadLimited(filepath.Join(e.rebrandDir(firstRebrand.ID), "icon.icns"), MaxIconBytes); err == nil {
			if firstRebrand.BeforeIconHash == "" || hash(b) == firstRebrand.BeforeIconHash {
				if pngB, ok := asPNG(b); ok {
					return pngB, true
				}
			}
		}
	}
	return nil, false
}

// asPNG returns b when it is already a valid PNG, else extracts the largest
// embedded PNG from an ICNS container.
func asPNG(b []byte) ([]byte, bool) {
	if ValidatePNG(b) == nil {
		return b, true
	}
	return icnsToPNG(b)
}

// icnsToPNG extracts the largest embedded PNG payload from an ICNS
// container (modern icon elements are PNG data). ok=false when none parse.
func icnsToPNG(b []byte) ([]byte, bool) {
	if len(b) < 8 || string(b[:4]) != "icns" {
		return nil, false
	}
	var best []byte
	for off := 8; off+8 <= len(b); {
		size := int(binary.BigEndian.Uint32(b[off+4 : off+8]))
		if size < 8 || off+size > len(b) {
			break
		}
		if payload := b[off+8 : off+size]; ValidatePNG(payload) == nil && len(payload) > len(best) {
			best = payload
		}
		off += size
	}
	return best, best != nil
}

// SavedOriginals returns a copy of the vault (bundle ID → saved original).
func (e *Engine) SavedOriginals() map[string]SavedOriginal {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := map[string]SavedOriginal{}
	for k, v := range e.State.Vault {
		out[k] = v
	}
	return out
}

type VaultRestoreReport struct {
	Restored int      `json:"restored"`
	Steps    []string `json:"steps,omitempty"`
}

// RestoreFromVault is the user-initiated, confirmed restore. It first tries
// the strict journalled restores; when an app has changed since skinning
// (updated, replaced, moved — a new identifier), the strict paths refuse
// rather than forcing old files on a new version, so this falls back to the
// vault: the saved original icon and names are applied onto the app as it
// exists now, located by its stable bundle ID. Nothing here runs
// automatically — the background keeper keeps using the strict paths only.
func (e *Engine) RestoreFromVault(ctx context.Context, appID string) (VaultRestoreReport, error) {
	rep := VaultRestoreReport{}
	app, err := e.GetApp(appID)
	if err != nil {
		return rep, err
	}
	bundle := app.BundleID
	var failures []string

	// 1. Full (internal) skin.
	hasRebrand := false
	for _, r := range e.SnapshotState().Rebrands {
		if r.BundleID == bundle && r.Status == "applied" {
			hasRebrand = true
		}
	}
	if hasRebrand {
		if _, err := e.RestoreRebrand(ctx, appID); err == nil {
			rep.Restored++
			rep.Steps = append(rep.Steps, "internal name and icon restored from the saved backup")
		} else if strings.Contains(err.Error(), "changed since") {
			if err := e.restoreRebrandOntoCurrent(ctx, app); err != nil {
				failures = append(failures, "full skin: "+err.Error())
			} else {
				rep.Restored++
				rep.Steps = append(rep.Steps, "app had changed, so the saved original name and icon were applied onto the current version")
			}
		} else {
			failures = append(failures, "full skin: "+err.Error())
		}
	}

	// 2. Finder icon layers.
	if n, err := e.RestoreOriginal(ctx, appID); err == nil {
		if n > 0 {
			rep.Restored += n
			rep.Steps = append(rep.Steps, "Finder icon restored")
		}
	} else {
		rep.Restored += n
		if cur, ok := e.appByPath(app.Path); ok && e.hasLiveRecord(bundle) {
			if ferr := e.restoreFinderFromVault(ctx, cur); ferr != nil {
				failures = append(failures, "icon: "+ferr.Error())
			} else {
				rep.Restored++
				rep.Steps = append(rep.Steps, "app had changed, so the saved original Finder icon was applied onto the current version")
			}
		} else if n == 0 {
			failures = append(failures, "icon: "+err.Error())
		}
	}

	// 3. Folder name. Always the app the user selected (by its current
	// path) — never another app that merely shares the bundle ID, since
	// cloned apps legitimately share one.
	if cur, ok := e.appByPath(app.Path); ok {
		saved := e.SavedOriginals()[bundle]
		currentFolder := strings.TrimSuffix(filepath.Base(cur.Path), ".app")
		renamed := false
		for _, nr := range e.SnapshotState().Renames {
			if nr.BundleID == bundle && nr.Status == "applied" && nr.To == cur.Path {
				renamed = true
			}
		}
		if renamed || (saved.FolderName != "" && saved.FolderName != currentFolder) {
			before := cur.Name
			restoredApp, rerr := e.RestoreName(ctx, cur.ID)
			if rerr == nil && restoredApp.Path != "" {
				cur = restoredApp
			}
			switch {
			case rerr == nil && cur.Name != before:
				rep.Restored++
				rep.Steps = append(rep.Steps, "folder name restored to "+cur.Name)
			case saved.FolderName != "" && strings.TrimSuffix(filepath.Base(cur.Path), ".app") != saved.FolderName:
				// Strict restore refused (app changed) or had nothing to
				// undo (folder moved externally): rebuild from the vault.
				if _, ferr := e.renameFolderToSaved(ctx, cur, saved.FolderName); ferr != nil {
					if rerr != nil {
						failures = append(failures, "name: "+rerr.Error())
					} else {
						failures = append(failures, "name: "+ferr.Error())
					}
				} else {
					rep.Restored++
					rep.Steps = append(rep.Steps, "app had changed, so it was renamed back to the saved original name "+saved.FolderName)
				}
			case rerr != nil:
				failures = append(failures, "name: "+rerr.Error())
			}
		}
	}

	if len(failures) > 0 {
		return rep, errors.New(strings.Join(failures, "; "))
	}
	return rep, nil
}

func (e *Engine) hasLiveRecord(bundle string) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, r := range e.State.Records {
		if r.App.BundleID == bundle && isLive(r.Status) {
			return true
		}
	}
	return false
}

// restoreFinderFromVault applies the earliest live record's saved "before"
// Finder-icon metadata onto the app at its current path, then closes those
// records. Used only from a confirmed user restore when the strict path
// refused because the app changed.
func (e *Engine) restoreFinderFromVault(ctx context.Context, app App) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	idx := -1
	for i, r := range e.State.Records {
		if r.App.BundleID == app.BundleID && isLive(r.Status) {
			idx = i
			break // earliest live record holds the true original
		}
	}
	if idx < 0 {
		return errors.New("no saved icon layers for this app")
	}
	if err := SafeApp(app.Path); err != nil {
		return err
	}
	before := e.State.Records[idx].Before
	if err := e.probeWritable(app); err != nil {
		return err
	}
	if err := e.Native.Restore(ctx, app.Path, before); err != nil {
		return err
	}
	check, err := e.Native.Snapshot(app.Path)
	if err != nil {
		return err
	}
	if !check.Equal(before) {
		return errors.New("restored icon metadata did not match the saved original")
	}
	for i := range e.State.Records {
		if e.State.Records[i].App.BundleID == app.BundleID && isLive(e.State.Records[i].Status) {
			e.State.Records[i].Status = "restored"
			e.State.Records[i].Error = ""
		}
	}
	delete(e.State.Preferences, app.BundleID)
	if err := e.save(); err != nil {
		return err
	}
	if a, ok := e.Apps[app.ID]; ok {
		a.IconStamp = PreviewStamp(a.Path)
		e.Apps[a.ID] = a
		_ = e.saveInventoryLocked()
	}
	return nil
}

// restoreRebrandOntoCurrent applies the vault's saved original internal name
// and the backed-up original .icns onto the app as it exists now (it updated
// or was replaced since the full skin), then re-signs. The backed-up
// Info.plist is deliberately NOT forced onto a changed app — only the name
// and icon, which are version-independent.
func (e *Engine) restoreRebrandOntoCurrent(ctx context.Context, target App) error {
	t, err := e.tools()
	if err != nil {
		return err
	}
	e.scanMu.Lock()
	defer e.scanMu.Unlock()
	e.mu.Lock()
	defer e.mu.Unlock()
	// Operate only on the exact app the user selected. The bundle ID finds
	// the journal entry, but cloned apps can share a bundle ID — resolving
	// the target by bundle alone could rebuild the wrong app.
	app, ok := e.Apps[target.ID]
	if !ok || app.Path != target.Path {
		return errors.New("application not found; rescan")
	}
	bundle := app.BundleID
	if err := SafeApp(app.Path); err != nil {
		return err
	}
	idx := -1
	for i := len(e.State.Rebrands) - 1; i >= 0; i-- {
		if e.State.Rebrands[i].BundleID == bundle && e.State.Rebrands[i].Status == "applied" {
			idx = i
			break
		}
	}
	if idx < 0 {
		return errors.New("this app has no full skin to restore")
	}
	rec := e.State.Rebrands[idx]
	if runner, ok := e.Native.(interface {
		Running(context.Context, string) (bool, error)
	}); ok {
		running, er := runner.Running(ctx, app.Path)
		if er != nil {
			return er
		}
		if running {
			return errors.New("quit this application before restoring its original identity")
		}
	}
	dir := e.rebrandDir(rec.ID)
	originalName := rec.OriginalName
	if originalName == "" {
		if b, er := ReadLimited(filepath.Join(dir, "Info.plist"), 4<<20); er == nil {
			originalName = plistNameFromBytes(b)
		}
	}
	if originalName == "" {
		if v, ok := e.State.Vault[bundle]; ok {
			originalName = v.InternalName
		}
	}
	if originalName == "" {
		originalName = rec.App.Name
	}
	iconPath := filepath.Join(app.Path, "Contents/Resources", rec.IconFile)
	if rec.IconExisted {
		iconBytes, err := ReadLimited(filepath.Join(dir, "icon.icns"), MaxIconBytes)
		if err != nil {
			return fmt.Errorf("full-skin icon backup is missing: %w", err)
		}
		if rec.BeforeIconHash != "" && hash(iconBytes) != rec.BeforeIconHash {
			return errors.New("saved original icon failed its checksum; not applied")
		}
		if err := AtomicWrite(iconPath, iconBytes, 0644); err != nil {
			return err
		}
	} else {
		_ = os.Remove(iconPath)
	}
	if err := t.SetBundleName(ctx, app.Path, originalName); err != nil {
		return err
	}
	if err := t.Resign(ctx, app.Path); err != nil {
		return err
	}
	if status, er := e.Native.Signature(ctx, app.Path); er != nil || status != "valid" {
		e.State.Rebrands[idx].Status = "rollback-required"
		_ = e.save()
		return errors.New("restored app did not pass signature verification")
	}
	if rec.IconExisted {
		b, er := ReadLimited(iconPath, MaxIconBytes)
		if er != nil || (rec.BeforeIconHash != "" && hash(b) != rec.BeforeIconHash) {
			e.State.Rebrands[idx].Status = "rollback-required"
			_ = e.save()
			return errors.New("restored icon did not match the saved original")
		}
	}
	e.State.Rebrands[idx].Status = "restored"
	if v, ok := e.State.Vault[bundle]; ok && v.InternalName == "" {
		v.InternalName = originalName
		e.State.Vault[bundle] = v
	}
	if err := e.save(); err != nil {
		return err
	}
	if notifier, ok := e.Native.(interface {
		Notify(context.Context, string) error
	}); ok {
		_ = notifier.Notify(ctx, app.Path)
	}
	return nil
}

// renameFolderToSaved renames the selected app's folder back to the saved
// original folder name. It operates on the exact app passed in — never on
// another app that merely shares the bundle ID. Used only from a confirmed
// user restore when the strict rename restore refused because the app
// changed, or the folder was moved externally.
func (e *Engine) renameFolderToSaved(ctx context.Context, target App, folder string) (App, error) {
	e.scanMu.Lock()
	defer e.scanMu.Unlock()
	e.mu.Lock()
	defer e.mu.Unlock()
	name, err := ValidateName(folder)
	if err != nil {
		return target, err
	}
	current, ok := e.Apps[target.ID]
	if !ok || current.Path != target.Path {
		return target, errors.New("application not found; rescan")
	}
	app := &current
	bundle := app.BundleID
	dest := filepath.Join(filepath.Dir(app.Path), name+".app")
	if dest == app.Path {
		return target, nil
	}
	if _, err := os.Lstat(dest); err == nil {
		return target, errors.New("an app with the saved original name already exists; rename or remove it first")
	} else if !os.IsNotExist(err) {
		return target, err
	}
	if runner, ok := e.Native.(interface {
		Running(context.Context, string) (bool, error)
	}); ok {
		running, er := runner.Running(ctx, app.Path)
		if er != nil {
			return target, er
		}
		if running {
			return target, errors.New("quit this application before restoring its name")
		}
	}
	old := *app
	if err := moveExclusive(app.Path, dest); err != nil {
		return target, fmt.Errorf("could not rename back to the saved name: %w", err)
	}
	updated := old
	updated.Path = dest
	updated.Name = name
	updated.ID = appID(updated)
	updated.Renamed = false
	updated.IconStamp = PreviewStamp(dest)
	e.moveReferencesLocked(old, updated)
	for i := range e.State.Renames {
		if e.State.Renames[i].BundleID == bundle && e.State.Renames[i].Status == "applied" && e.State.Renames[i].To == old.Path {
			e.State.Renames[i].Status = "restored"
		}
	}
	if err := e.save(); err != nil {
		return updated, err
	}
	_ = e.saveInventoryLocked()
	if notifier, ok := e.Native.(interface {
		Notify(context.Context, string) error
	}); ok {
		_ = notifier.Notify(ctx, dest)
	}
	return updated, nil
}
