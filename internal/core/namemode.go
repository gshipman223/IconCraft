package core

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

const (
	NameModeSafe = "safe"
	NameModeFull = "full"
)

// NamePolicyResult is what a pause, resume, or name-mode change did.
type NamePolicyResult struct {
	Moved int      `json:"moved"`
	Icons int      `json:"icons"`
	Notes []string `json:"notes,omitempty"`
}

func canonicalNameMode(mode string) (string, error) {
	switch mode {
	case "", NameModeSafe:
		return NameModeSafe, nil
	case NameModeFull:
		return NameModeFull, nil
	default:
		return "", errors.New("name mode must be Icon only (safe) or Full rename")
	}
}

// nameModeLocked is safe or full for this bundle. A per-app choice wins.
// Caller holds e.mu.
func (e *Engine) nameModeLocked(bundle string) string {
	if m, ok := e.State.NameModes[bundle]; ok && (m == NameModeSafe || m == NameModeFull) {
		return m
	}
	if e.State.SettingsSet && e.State.Settings.NameMode == NameModeFull {
		return NameModeFull
	}
	return NameModeSafe
}

// UpdatesPaused reports the global pause. The keeper must not rename or
// re-icon while this is set.
func (e *Engine) UpdatesPaused() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.updatesPausedLocked()
}

func (e *Engine) updatesPausedLocked() bool {
	return e.State.SettingsSet && e.State.Settings.UpdatesPaused
}

func (e *Engine) bundlePausedLocked(bundle string) bool {
	if e.updatesPausedLocked() {
		return true
	}
	return e.State.PausedBundles[bundle]
}

func (e *Engine) bundlePaused(bundle string) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.bundlePausedLocked(bundle)
}

// SetAppNameMode stores a per-app override. mode "" follows the global
// setting. The caller applies the policy afterwards.
func (e *Engine) SetAppNameMode(appID, mode string) (App, error) {
	if mode != "" {
		var err error
		mode, err = canonicalNameMode(mode)
		if err != nil {
			return App{}, err
		}
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	a, ok := e.Apps[appID]
	if !ok {
		return App{}, errors.New("application not found; rescan")
	}
	if e.State.NameModes == nil {
		e.State.NameModes = map[string]string{}
	}
	if mode == "" {
		delete(e.State.NameModes, a.BundleID)
	} else {
		e.State.NameModes[a.BundleID] = mode
	}
	a.NameChoice = mode
	a.NameMode = e.nameModeLocked(a.BundleID)
	e.Apps[a.ID] = a
	if err := e.save(); err != nil {
		return a, err
	}
	return a, nil
}

// SetBundlePaused pauses or resumes one app without changing the global
// switch. Pausing saves the flag before any folder is moved.
func (e *Engine) SetBundlePaused(ctx context.Context, appID string, paused bool) (NamePolicyResult, error) {
	e.mu.Lock()
	a, ok := e.Apps[appID]
	if !ok {
		e.mu.Unlock()
		return NamePolicyResult{}, errors.New("application not found; rescan")
	}
	bundle := a.BundleID
	if e.State.PausedBundles == nil {
		e.State.PausedBundles = map[string]bool{}
	}
	if paused {
		e.State.PausedBundles[bundle] = true
	} else {
		delete(e.State.PausedBundles, bundle)
	}
	if err := e.save(); err != nil {
		e.mu.Unlock()
		return NamePolicyResult{}, err
	}
	e.mu.Unlock()
	if _, _, err := e.Scan(ctx); err != nil {
		return NamePolicyResult{}, err
	}
	if paused {
		return e.ContinuePause(ctx, bundle), nil
	}
	if e.UpdatesPaused() {
		return NamePolicyResult{}, nil
	}
	res := e.ResumeSuspended(ctx, bundle)
	icons := e.SyncIconModes(ctx, bundle)
	res.Icons += icons.Icons
	res.Notes = append(res.Notes, icons.Notes...)
	return res, nil
}

// ApplyNamePolicy makes folders and icons match the saved name mode and
// pause flags. Settings are already journalled, including UpdatesPaused,
// so a quit halfway through still resumes the same direction.
func (e *Engine) ApplyNamePolicy(ctx context.Context) NamePolicyResult {
	if _, _, err := e.Scan(ctx); err != nil {
		return NamePolicyResult{Notes: []string{err.Error()}}
	}
	if e.UpdatesPaused() {
		return e.ContinuePause(ctx, "")
	}
	res := e.ResumeSuspended(ctx, "")
	sync := e.SyncNameModes(ctx, "")
	icons := e.SyncIconModes(ctx, "")
	res.Moved += sync.Moved
	res.Icons += sync.Icons + icons.Icons
	res.Notes = append(res.Notes, sync.Notes...)
	res.Notes = append(res.Notes, icons.Notes...)
	return res
}

// SyncNameModes moves folders to match the effective mode. A paused bundle
// is left where Pause for updates put it.
func (e *Engine) SyncNameModes(ctx context.Context, onlyBundle string) NamePolicyResult {
	var out NamePolicyResult
	e.mu.Lock()
	renames := append([]RenameRecord(nil), e.State.Renames...)
	e.mu.Unlock()
	seen := map[string]bool{}
	for i := len(renames) - 1; i >= 0; i-- {
		nr := renames[i]
		if nr.Status != "applied" && nr.Status != "pause-blocked" {
			continue
		}
		if nr.BundleID == "" || seen[nr.BundleID] {
			continue
		}
		if onlyBundle != "" && nr.BundleID != onlyBundle {
			continue
		}
		seen[nr.BundleID] = true
		if e.bundlePaused(nr.BundleID) {
			continue
		}
		mode := e.currentMode(nr.BundleID)
		switch {
		case mode != NameModeFull && nr.From != nr.To:
			note, moved := e.repairLegacyRename(ctx, nr)
			if note != "" {
				out.Notes = append(out.Notes, note)
			}
			if moved {
				out.Moved++
			}
			if !moved && e.atOriginal(nr) {
				e.settleLabel(nr)
			}
		case mode == NameModeFull && nr.From != nr.To:
			_, toErr := os.Lstat(nr.To)
			_, fromErr := os.Lstat(nr.From)
			if toErr == nil && fromErr == nil {
				if nr.Status != "pause-blocked" {
					_ = e.setRenameStatus(nr.ID, "pause-blocked")
					out.Notes = append(out.Notes, fmt.Sprintf("%s: %s is still beside %s; IconCraft did not delete it.", nr.NewName, filepath.Base(nr.To), filepath.Base(nr.From)))
				}
				continue
			}
			if toErr == nil {
				e.clearSuspended(nr.ID)
				continue
			}
			moved, note := e.resumeRename(ctx, nr.ID)
			if note != "" {
				out.Notes = append(out.Notes, note)
			}
			if moved {
				out.Moved++
			}
		case mode == NameModeFull && nr.From == nr.To && nr.NewName != bundleFolderName(nr.To):
			app, ok := e.appByBundle(nr.BundleID, nr.To)
			if !ok {
				out.Notes = append(out.Notes, nr.NewName+": not in inventory; rescan needed")
				continue
			}
			if _, err := e.Rename(ctx, app.ID, nr.NewName); err != nil {
				out.Notes = append(out.Notes, nr.NewName+": "+err.Error())
				continue
			}
			out.Moved++
		}
	}
	return out
}

func (e *Engine) currentMode(bundle string) string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.nameModeLocked(bundle)
}

func (e *Engine) atOriginal(nr RenameRecord) bool {
	_, fromErr := os.Lstat(nr.From)
	_, toErr := os.Lstat(nr.To)
	return fromErr == nil && os.IsNotExist(toErr)
}

// ContinuePause puts full-renamed apps back at the folder their launcher
// looks up, and puts the original Finder icon back when that can be done
// without writing onto a changed bundle. It never deletes a second copy.
func (e *Engine) ContinuePause(ctx context.Context, onlyBundle string) NamePolicyResult {
	var out NamePolicyResult
	e.mu.Lock()
	renames := append([]RenameRecord(nil), e.State.Renames...)
	records := append([]Record(nil), e.State.Records...)
	global := e.updatesPausedLocked()
	paused := map[string]bool{}
	for k, v := range e.State.PausedBundles {
		if v {
			paused[k] = true
		}
	}
	e.mu.Unlock()
	want := func(bundle string) bool {
		if onlyBundle != "" {
			return bundle == onlyBundle
		}
		return global || paused[bundle]
	}
	seen := map[string]bool{}
	for i := len(renames) - 1; i >= 0; i-- {
		nr := renames[i]
		if nr.BundleID == "" || seen[nr.BundleID] || !want(nr.BundleID) {
			continue
		}
		if nr.Status != "applied" && nr.Status != "suspending" && nr.Status != "pause-blocked" {
			continue
		}
		seen[nr.BundleID] = true
		if e.currentMode(nr.BundleID) != NameModeFull && nr.From != nr.To && !nr.Suspended {
			note, moved := e.repairLegacyRename(ctx, nr)
			if note != "" {
				out.Notes = append(out.Notes, note)
			}
			if moved {
				out.Moved++
			}
			continue
		}
		note, moved := e.suspendRename(ctx, nr.ID)
		if note != "" {
			out.Notes = append(out.Notes, note)
		}
		if moved {
			out.Moved++
		}
	}
	// Put the bundle icon file back before the Finder tile. Re-signing
	// removes the Finder custom icon, and the hold below writes the
	// original tile afterwards.
	iconsHeld := e.holdBundleIcons(ctx, onlyBundle)
	out.Notes = append(out.Notes, iconsHeld.Notes...)
	out.Icons += iconsHeld.Icons
	held := map[string]bool{}
	for i := len(records) - 1; i >= 0; i-- {
		rec := records[i]
		if !isLive(rec.Status) || rec.App.BundleID == "" || held[rec.App.BundleID] || !want(rec.App.BundleID) {
			continue
		}
		held[rec.App.BundleID] = true
		if note := e.holdFinderIcon(ctx, rec.ID); note != "" {
			out.Notes = append(out.Notes, note)
		} else {
			out.Icons++
		}
	}
	// Finder and the Dock reload after the original files and the
	// original Finder tile are both back. Already-held apps are not
	// refreshed again, so the keeper does not reload Finder on every pass.
	if iconsHeld.Icons > 0 {
		e.refreshHeldBundles(ctx, onlyBundle)
	}
	return out
}

// ResumeSuspended puts custom folder names and Finder icons back on the
// bundles that were held, matched by bundle id. A path that is already
// occupied is left alone.
func (e *Engine) ResumeSuspended(ctx context.Context, onlyBundle string) NamePolicyResult {
	var out NamePolicyResult
	e.mu.Lock()
	renames := append([]RenameRecord(nil), e.State.Renames...)
	records := append([]Record(nil), e.State.Records...)
	e.mu.Unlock()
	seen := map[string]bool{}
	for i := len(renames) - 1; i >= 0; i-- {
		nr := renames[i]
		if nr.BundleID == "" || seen[nr.BundleID] {
			continue
		}
		if onlyBundle != "" && nr.BundleID != onlyBundle {
			continue
		}
		if e.bundlePaused(nr.BundleID) {
			continue
		}
		if nr.Status != "applied" && nr.Status != "resuming" && nr.Status != "pause-blocked" && nr.Status != "suspending" {
			continue
		}
		if !nr.Suspended && nr.Status != "resuming" && nr.Status != "pause-blocked" && nr.Status != "suspending" {
			continue
		}
		seen[nr.BundleID] = true
		moved, note := e.resumeRename(ctx, nr.ID)
		if note != "" {
			out.Notes = append(out.Notes, note)
		}
		if moved {
			out.Moved++
		}
	}
	released := e.releaseBundleIcons(ctx, onlyBundle)
	out.Icons += released.Icons
	out.Notes = append(out.Notes, released.Notes...)
	held := map[string]bool{}
	for i := len(records) - 1; i >= 0; i-- {
		rec := records[i]
		if !rec.Held || !isLive(rec.Status) || held[rec.App.BundleID] {
			continue
		}
		if onlyBundle != "" && rec.App.BundleID != onlyBundle {
			continue
		}
		if e.bundlePaused(rec.App.BundleID) {
			continue
		}
		held[rec.App.BundleID] = true
		app, ok := e.appByBundle(rec.App.BundleID, rec.App.Path)
		if !ok {
			out.Notes = append(out.Notes, rec.App.Name+": icon not reapplied; rescan needed")
			continue
		}
		if _, err := e.Apply(ctx, app.ID, rec.AssetID); err != nil {
			out.Notes = append(out.Notes, app.Name+": "+err.Error())
			continue
		}
		e.clearHeld(rec.App.BundleID)
		out.Icons++
	}
	return out
}

func (e *Engine) suspendRename(ctx context.Context, id string) (note string, moved bool) {
	nr, ok := e.renameByID(id)
	if !ok || nr.From == "" || nr.From == nr.To {
		return "", false
	}
	if nr.Suspended && nr.Status == "applied" {
		return "", false
	}
	if note, stop := e.quitNote(ctx, nr); stop {
		return note, false
	}
	_, fromErr := os.Lstat(nr.From)
	toInfo, toErr := os.Lstat(nr.To)
	switch {
	case toErr == nil && os.IsNotExist(fromErr):
		if toInfo.Mode()&os.ModeSymlink != 0 || !toInfo.IsDir() {
			return nr.NewName + ": the renamed item is not an app folder; left it alone", false
		}
		if err := e.setRenameStatus(id, "suspending"); err != nil {
			return nr.NewName + ": " + err.Error(), false
		}
		if err := moveExclusive(nr.To, nr.From); err != nil {
			_ = e.setRenameStatus(id, "applied")
			return nr.NewName + ": " + err.Error(), false
		}
		e.finishSuspend(nr)
		return "", true
	case toErr == nil && fromErr == nil:
		_ = e.setRenameStatus(id, "pause-blocked")
		return fmt.Sprintf("%s: %s is still beside %s; IconCraft did not delete it. Launchers use %s.", nr.NewName, filepath.Base(nr.To), filepath.Base(nr.From), filepath.Base(nr.From)), false
	case os.IsNotExist(toErr) && fromErr == nil:
		e.finishSuspend(nr)
		return "", false
	default:
		return nr.NewName + ": app not found at either name; leaving it alone", false
	}
}

func (e *Engine) resumeRename(ctx context.Context, id string) (moved bool, note string) {
	nr, ok := e.renameByID(id)
	if !ok {
		return false, ""
	}
	if nr.From == nr.To {
		e.clearSuspended(id)
		return false, ""
	}
	if e.currentMode(nr.BundleID) != NameModeFull {
		n, movedBack := e.repairLegacyRename(ctx, nr)
		if e.atOriginal(nr) {
			e.settleLabel(nr)
		}
		return movedBack, n
	}
	_, fromErr := os.Lstat(nr.From)
	_, toErr := os.Lstat(nr.To)
	switch {
	case toErr == nil && os.IsNotExist(fromErr):
		e.clearSuspended(id)
		return false, ""
	case toErr == nil && fromErr == nil:
		if nr.Status != "pause-blocked" {
			_ = e.setRenameStatus(id, "pause-blocked")
			return false, fmt.Sprintf("%s: %s is still beside %s; IconCraft did not delete it.", nr.NewName, filepath.Base(nr.To), filepath.Base(nr.From))
		}
		return false, ""
	case os.IsNotExist(toErr) && fromErr == nil:
		if note, stop := e.quitNote(ctx, nr); stop {
			return false, note
		}
		if err := e.setRenameStatus(id, "resuming"); err != nil {
			return false, nr.NewName + ": " + err.Error()
		}
		if err := moveExclusive(nr.From, nr.To); err != nil {
			_ = e.setRenameStatus(id, "applied")
			return false, nr.NewName + ": " + err.Error()
		}
		e.finishResume(nr)
		return true, ""
	default:
		return false, nr.NewName + ": app not found at either name; leaving it alone"
	}
}

func (e *Engine) quitNote(ctx context.Context, nr RenameRecord) (string, bool) {
	runner, ok := e.Native.(interface {
		Running(context.Context, string) (bool, error)
	})
	if !ok {
		return "", false
	}
	for _, p := range []string{nr.To, nr.From} {
		if _, err := os.Lstat(p); err != nil {
			continue
		}
		running, err := runner.Running(ctx, p)
		if err != nil {
			return nr.NewName + ": " + err.Error(), true
		}
		if running {
			return nr.NewName + ": quit it before IconCraft moves it", true
		}
	}
	return "", false
}

func (e *Engine) holdFinderIcon(ctx context.Context, recordID string) string {
	e.mu.Lock()
	idx := -1
	var rec Record
	for i := range e.State.Records {
		if e.State.Records[i].ID == recordID {
			idx = i
			rec = e.State.Records[i]
			break
		}
	}
	e.mu.Unlock()
	if idx < 0 || !isLive(rec.Status) {
		return ""
	}
	app, ok := e.appByBundle(rec.App.BundleID, rec.App.Path)
	if !ok {
		app = rec.App
	}
	snap, err := e.Native.Snapshot(app.Path)
	if err != nil {
		return app.Name + ": " + err.Error()
	}
	if snap.Custom() {
		fp, ferr := Fingerprint(app)
		if ferr == nil && (rec.Fingerprint == "" || fp == rec.Fingerprint) {
			if err = e.probeWritable(app); err != nil {
				return app.Name + ": " + err.Error()
			}
			if err = e.Native.Restore(ctx, app.Path, rec.Before); err != nil {
				return app.Name + ": " + err.Error()
			}
		}
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	for i := range e.State.Records {
		if e.State.Records[i].ID == recordID && isLive(e.State.Records[i].Status) {
			e.State.Records[i].Held = true
			break
		}
	}
	_ = e.save()
	return ""
}

func (e *Engine) clearHeld(bundle string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	for i := range e.State.Records {
		if e.State.Records[i].App.BundleID == bundle {
			e.State.Records[i].Held = false
		}
	}
	_ = e.save()
}

func (e *Engine) renameByID(id string) (RenameRecord, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, nr := range e.State.Renames {
		if nr.ID == id {
			return nr, true
		}
	}
	return RenameRecord{}, false
}

func (e *Engine) setRenameStatus(id, status string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	for i := range e.State.Renames {
		if e.State.Renames[i].ID == id {
			e.State.Renames[i].Status = status
			return e.save()
		}
	}
	return nil
}

func (e *Engine) finishSuspend(nr RenameRecord) {
	e.mu.Lock()
	defer e.mu.Unlock()
	for i := range e.State.Renames {
		r := &e.State.Renames[i]
		if r.ID != nr.ID {
			continue
		}
		r.Status = "applied"
		r.Suspended = true
		e.relocateLocked(nr.To, nr.From, nr.BundleID, bundleFolderName(nr.From))
		break
	}
	_ = e.save()
	_ = e.saveInventoryLocked()
}

func (e *Engine) finishResume(nr RenameRecord) {
	e.mu.Lock()
	defer e.mu.Unlock()
	for i := range e.State.Renames {
		r := &e.State.Renames[i]
		if r.ID != nr.ID {
			continue
		}
		r.Status = "applied"
		r.Suspended = false
		e.relocateLocked(nr.From, nr.To, nr.BundleID, nr.NewName)
		break
	}
	_ = e.save()
	_ = e.saveInventoryLocked()
}

func (e *Engine) clearSuspended(id string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	for i := range e.State.Renames {
		if e.State.Renames[i].ID == id {
			e.State.Renames[i].Suspended = false
			if e.State.Renames[i].Status == "suspending" || e.State.Renames[i].Status == "resuming" || e.State.Renames[i].Status == "pause-blocked" {
				e.State.Renames[i].Status = "applied"
			}
			break
		}
	}
	_ = e.save()
}

func (e *Engine) settleLabel(nr RenameRecord) {
	e.mu.Lock()
	defer e.mu.Unlock()
	for i := range e.State.Renames {
		r := &e.State.Renames[i]
		if r.ID != nr.ID && !(r.BundleID == nr.BundleID && r.Status == "applied" && r.From == nr.From) {
			continue
		}
		if r.ID != nr.ID && r.From == r.To {
			continue
		}
		r.To = r.From
		r.Status = "applied"
		r.Suspended = false
		if cur, ok := e.Apps[appID(App{Path: r.From, BundleID: r.BundleID})]; ok {
			cur.Name = r.NewName
			cur.Renamed = r.NewName != bundleFolderName(r.From)
			cur.Paused = false
			e.Apps[cur.ID] = cur
		}
		if r.ID == nr.ID {
			break
		}
	}
	_ = e.save()
	_ = e.saveInventoryLocked()
}

// retargetSuspendedLocked updates the custom name a paused full rename
// will use when pause ends. The folder stays where the launcher looks.
// Caller holds e.mu. False means there is no suspended rename to update.
func (e *Engine) retargetSuspendedLocked(a App, name string) bool {
	dest := filepath.Join(filepath.Dir(a.Path), name+".app")
	for i := len(e.State.Renames) - 1; i >= 0; i-- {
		r := &e.State.Renames[i]
		if r.BundleID == a.BundleID && r.Suspended && (r.Status == "applied" || r.Status == "pause-blocked") {
			r.NewName = name
			r.To = dest
			_ = e.save()
			return true
		}
	}
	return false
}

// renameFolderLocked renames the outer .app folder. Info.plist and the
// signature are not modified. Caller holds e.scanMu and e.mu.
func (e *Engine) renameFolderLocked(ctx context.Context, a App, name string) (App, error) {
	dest := filepath.Join(filepath.Dir(a.Path), name+".app")
	if dest == a.Path {
		return a, nil
	}
	if _, err := os.Lstat(dest); err == nil {
		return a, errors.New("an app with that name already exists; choose another name")
	} else if !os.IsNotExist(err) {
		return a, err
	}
	if runner, ok := e.Native.(interface {
		Running(context.Context, string) (bool, error)
	}); ok {
		running, er := runner.Running(ctx, a.Path)
		if er != nil {
			return a, er
		}
		if running {
			return a, errors.New("quit this application before renaming it")
		}
	}
	fp, err := Fingerprint(a)
	if err != nil {
		return a, err
	}
	from := a.Path
	prior := -1
	for i := len(e.State.Renames) - 1; i >= 0; i-- {
		r := e.State.Renames[i]
		if r.BundleID != a.BundleID || (r.Status != "applied" && r.Status != "pause-blocked") {
			continue
		}
		if r.To == a.Path || r.From == a.Path {
			prior = i
			if r.From != "" && r.From != r.To {
				from = r.From
			}
			break
		}
	}
	var idx int
	if prior >= 0 {
		idx = prior
		e.State.Renames[idx].From = from
		e.State.Renames[idx].To = dest
		e.State.Renames[idx].NewName = name
		e.State.Renames[idx].Fingerprint = fp
		e.State.Renames[idx].Status = "prepared"
		e.State.Renames[idx].Suspended = false
	} else {
		nr := RenameRecord{App: a, ID: newID(), BundleID: a.BundleID, From: from, To: dest, OldName: a.Name, NewName: name, Fingerprint: fp, Status: "prepared"}
		idx = len(e.State.Renames)
		e.State.Renames = append(e.State.Renames, nr)
	}
	if err = e.save(); err != nil {
		return a, err
	}
	if err = ctx.Err(); err != nil {
		return a, err
	}
	if err = moveExclusive(a.Path, dest); err != nil {
		e.State.Renames[idx].Status = "failed"
		_ = e.save()
		return a, fmt.Errorf("could not rename app: %w", err)
	}
	out := a
	out.Path = dest
	out.Name = name
	out.ID = appID(out)
	out.Renamed = true
	out.IconStamp = PreviewStamp(dest)
	after, checkErr := Fingerprint(out)
	if checkErr != nil || after != fp {
		if back := moveExclusive(dest, a.Path); back == nil {
			e.State.Renames[idx].Status = "failed"
			_ = e.save()
		}
		return a, errors.New("app changed during rename; stopped and attempted rollback")
	}
	e.moveReferencesLocked(a, out)
	e.State.Renames[idx].Status = "applied"
	e.State.Renames[idx].Suspended = false
	if err = e.save(); err != nil {
		return out, fmt.Errorf("app renamed, but saving history failed: %w", err)
	}
	_ = e.saveInventoryLocked()
	if notifier, ok := e.Native.(interface {
		Notify(context.Context, string) error
	}); ok {
		_ = notifier.Notify(ctx, dest)
	}
	return out, nil
}

// recoverPauseMoves finishes a folder move that was journalled and then
// interrupted, without putting a paused app back under its custom name.
func (e *Engine) recoverPauseMoves() {
	dirty := false
	for i := range e.State.Renames {
		r := &e.State.Renames[i]
		if r.Status != "suspending" && r.Status != "resuming" {
			continue
		}
		_, fe := os.Lstat(r.From)
		_, te := os.Lstat(r.To)
		switch {
		case fe == nil && os.IsNotExist(te):
			// The folder is at the original path. Keep it there and let
			// resume retry if pause is no longer wanted.
			r.Status = "applied"
			r.Suspended = true
			dirty = true
		case os.IsNotExist(fe) && te == nil:
			r.Status = "applied"
			r.Suspended = false
			dirty = true
		}
	}
	if dirty {
		_ = e.save()
	}
}
