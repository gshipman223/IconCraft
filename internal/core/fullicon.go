package core

import (
	"context"
	"errors"
	"fmt"
	"image"
	"os"
	"path/filepath"
	"strings"
)

const (
	// IconModeFinder leaves the signed bundle alone. The custom icon is a
	// Finder tile, which the Dock and Cmd-Tab show only while the app is
	// not running.
	IconModeFinder = "finder"
	// IconModeFull replaces the bundle's .icns (and a matching embedded
	// runtime icon, when one is found) and ad-hoc re-signs. That is the
	// only supported way to change another app's running Dock icon.
	IconModeFull = "full"
	// RebrandKindIcon marks a Full icon record. Legacy full skins have an
	// empty Kind and also rewrote the bundle's display name.
	RebrandKindIcon = "icon"
)

func canonicalIconMode(mode string) (string, error) {
	switch mode {
	case "", IconModeFinder:
		return IconModeFinder, nil
	case IconModeFull:
		return IconModeFull, nil
	default:
		return "", errors.New("icon mode must be Finder icon (safe) or Full icon")
	}
}

// iconModeLocked is finder or full for this bundle. A per-app choice wins.
// Caller holds e.mu.
func (e *Engine) iconModeLocked(bundle string) string {
	if m, ok := e.State.IconModes[bundle]; ok && (m == IconModeFinder || m == IconModeFull) {
		return m
	}
	if e.State.SettingsSet && e.State.Settings.IconMode == IconModeFull {
		return IconModeFull
	}
	return IconModeFinder
}

// IconMode reports the effective icon mode for a bundle.
func (e *Engine) IconMode(bundle string) string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.iconModeLocked(bundle)
}

// SetAppIconMode stores a per-app override. mode "" follows the global
// setting. The caller applies the policy afterwards.
func (e *Engine) SetAppIconMode(appID, mode string) (App, error) {
	if mode != "" {
		var err error
		mode, err = canonicalIconMode(mode)
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
	if e.State.IconModes == nil {
		e.State.IconModes = map[string]string{}
	}
	if mode == "" {
		delete(e.State.IconModes, a.BundleID)
	} else {
		e.State.IconModes[a.BundleID] = mode
	}
	a.IconChoice = mode
	a.IconMode = e.iconModeLocked(a.BundleID)
	e.Apps[a.ID] = a
	if err := e.save(); err != nil {
		return a, err
	}
	return a, nil
}

// ApplyBundleIcon replaces the icon inside the app so the Dock and
// Cmd-Tab show it while the app is running, then puts the same artwork
// on the Finder tile for when it is closed.
//
// NSApplication.dockTile and NSDockTile.setContentView only change the
// calling process. NSWorkspace didLaunchApplicationNotification hands
// over a read-only icon. The Dock icon cache and lsregister refresh the
// closed tile, which a Finder custom icon already covers. macOS 26's
// icon format does not add a cross-process override. Private Dock calls
// are not a supported way to push an icon into another hardened-runtime
// process. The running tile therefore has to come from the bundle.
//
// Renaming the .app folder does not change this tile, in Icon only or
// in Full rename. Full icon is independent of the name mode.
//
// Info.plist, the bundle id and the .app path are not modified. The
// publisher signature is replaced with an ad-hoc signature and cannot
// be reconstructed. Asset-catalog apps are refused. An app that sets
// its own icon from artwork this cannot match will still show that icon;
// the record's RuntimeIcon says which.
func (e *Engine) ApplyBundleIcon(ctx context.Context, appID, assetID string) (RebrandRecord, error) {
	rec, err := e.writeBundleIcon(ctx, appID, assetID)
	if err != nil {
		return rec, err
	}
	_, applyErr := e.Apply(ctx, appID, assetID)
	// Touch, re-register and reload Finder after the tile is set, so the
	// app icon and any document icons show without a reboot.
	e.refreshBundleIcons(ctx, rec.App.Path)
	if applyErr != nil {
		return rec, fmt.Errorf("running icon applied, but the Finder icon was not: %w", applyErr)
	}
	return rec, nil
}

func (e *Engine) writeBundleIcon(ctx context.Context, appID, assetID string) (RebrandRecord, error) {
	t, err := e.tools()
	if err != nil {
		return RebrandRecord{}, err
	}
	e.scanMu.Lock()
	defer e.scanMu.Unlock()
	e.mu.Lock()
	defer e.mu.Unlock()
	a, ok := e.Apps[appID]
	if !ok {
		return RebrandRecord{}, errors.New("application not found; rescan")
	}
	if a.Protected {
		return RebrandRecord{}, errors.New("system apps cannot take a full icon")
	}
	if err = SafeApp(a.Path); err != nil {
		return RebrandRecord{}, err
	}
	if e.bundlePausedLocked(a.BundleID) {
		return RebrandRecord{}, errors.New("pause for updates is on; the icon inside the app stays the publisher's until pause ends")
	}
	if e.iconModeLocked(a.BundleID) != IconModeFull {
		return RebrandRecord{}, errors.New("full icon is off")
	}
	if err = e.probeWritable(a); err != nil {
		return RebrandRecord{}, err
	}
	if runner, ok := e.Native.(interface {
		Running(context.Context, string) (bool, error)
	}); ok {
		running, er := runner.Running(ctx, a.Path)
		if er != nil {
			return RebrandRecord{}, er
		}
		if running {
			return RebrandRecord{}, errors.New("quit this application before replacing its icon; the Dock updates the next time it opens")
		}
	}
	iconRel, err := t.BundleIconFile(ctx, a.Path)
	if err != nil {
		return RebrandRecord{}, err
	}
	iconRel = strings.TrimSpace(iconRel)
	if iconRel == "" || filepath.Base(iconRel) != iconRel || strings.Contains(iconRel, "..") {
		return RebrandRecord{}, errors.New("this app draws its icon from an asset catalog, so its running Dock icon can't be replaced; the Finder icon still applies")
	}
	if !strings.HasSuffix(strings.ToLower(iconRel), ".icns") {
		iconRel += ".icns"
	}
	assetPath, err := e.assetPath(assetID)
	if err != nil {
		return RebrandRecord{}, errors.New("choose an icon first")
	}
	pngBytes, err := ReadLimited(assetPath, MaxIconBytes)
	if err != nil {
		return RebrandRecord{}, err
	}
	icns, err := makeICNS(pngBytes)
	if err != nil {
		return RebrandRecord{}, err
	}
	if _, err = e.Native.Signature(ctx, a.Path); err != nil {
		return RebrandRecord{}, fmt.Errorf("original signature check failed: %w", err)
	}
	plistPath := filepath.Join(a.Path, "Contents/Info.plist")
	iconPath := filepath.Join(a.Path, "Contents/Resources", iconRel)
	plistBytes, err := ReadLimited(plistPath, 4<<20)
	if err != nil {
		return RebrandRecord{}, err
	}
	iconBytes, iconErr := ReadLimited(iconPath, MaxIconBytes)
	iconExisted := iconErr == nil
	var docFiles []DocIconFile
	var docOriginals [][]byte
	docNote := ""
	if e.documentIconsLocked(a.BundleID) {
		docFiles, docOriginals, docNote = e.planDocumentIcons(ctx, a, plistBytes, iconRel)
	}
	before, err := Fingerprint(a)
	if err != nil {
		return RebrandRecord{}, err
	}
	e.captureOriginalLocked(ctx, a)
	rec := RebrandRecord{
		ID: newID(), BundleID: a.BundleID, App: a, AssetID: assetID, Name: a.Name,
		IconFile: iconRel, IconExisted: iconExisted, BeforePrint: before,
		Status: "prepared", IconFit: IconFitVersion, Kind: RebrandKindIcon, Created: now(),
		DocIcons: docFiles, DocNote: docNote,
	}
	if iconExisted {
		rec.BeforeIconHash = hash(iconBytes)
	}
	dir := e.rebrandDir(rec.ID)
	if err = os.MkdirAll(dir, 0700); err != nil {
		return RebrandRecord{}, err
	}
	if err = AtomicWrite(filepath.Join(dir, "Info.plist"), plistBytes, 0600); err != nil {
		return RebrandRecord{}, err
	}
	if iconExisted {
		if err = AtomicWrite(filepath.Join(dir, "icon.icns"), iconBytes, 0600); err != nil {
			return RebrandRecord{}, err
		}
	}
	for i, b := range docOriginals {
		if err = AtomicWrite(filepath.Join(dir, fmt.Sprintf("doc-%d.icns", i)), b, 0600); err != nil {
			return RebrandRecord{}, err
		}
	}
	e.State.Rebrands = append(e.State.Rebrands, rec)
	idx := len(e.State.Rebrands) - 1
	if err = e.save(); err != nil {
		e.State.Rebrands = e.State.Rebrands[:idx]
		return RebrandRecord{}, fmt.Errorf("backup could not be committed; no change made: %w", err)
	}
	exePath, exeBytes, exeErr := readExecutable(a)
	var patches []ExecIconPatch
	var originals, replacements [][]byte
	runtimeIcon := "none"
	if exeErr == nil {
		var origImg image.Image
		if iconExisted {
			origImg, _ = icnsImage(iconBytes)
		}
		assetImg, _ := decodeAssetImage(assetPath)
		var setsOwn bool
		patches, originals, replacements, setsOwn = planExecIconPatches(exeBytes, origImg, assetImg)
		switch {
		case len(patches) > 0:
			runtimeIcon = "patched"
		case setsOwn:
			runtimeIcon = "own"
		}
	}
	exePatched := false
	docsWritten := 0
	rollback := func(cause error) (RebrandRecord, error) {
		if exePatched {
			_ = restoreExecIconPatches(exePath, patches, dir)
		}
		for i := 0; i < docsWritten && i < len(docOriginals); i++ {
			p := filepath.Join(a.Path, "Contents/Resources", docFiles[i].File)
			_ = AtomicWrite(p, docOriginals[i], 0644)
		}
		if iconExisted {
			_ = AtomicWrite(iconPath, iconBytes, 0644)
		} else {
			_ = os.Remove(iconPath)
		}
		_ = t.Resign(ctx, a.Path)
		e.State.Rebrands[idx].Status = "failed"
		_ = e.save()
		return e.State.Rebrands[idx], cause
	}
	if err = os.MkdirAll(filepath.Dir(iconPath), 0755); err != nil {
		return rollback(err)
	}
	if err = AtomicWrite(iconPath, icns, 0644); err != nil {
		return rollback(fmt.Errorf("could not replace the bundle icon: %w", err))
	}
	if len(docFiles) > 0 {
		var docICNS []byte
		docICNS, err = makeDocumentICNS(pngBytes)
		if err != nil {
			return rollback(err)
		}
		for i, d := range docFiles {
			p := filepath.Join(a.Path, "Contents/Resources", d.File)
			if err = AtomicWrite(p, docICNS, 0644); err != nil {
				return rollback(fmt.Errorf("could not replace document icon %s: %w", d.File, err))
			}
			docsWritten = i + 1
			e.State.Rebrands[idx].DocIcons[i].AfterHash = hash(docICNS)
		}
	}
	if len(patches) > 0 {
		for i, orig := range originals {
			if err = AtomicWrite(filepath.Join(dir, fmt.Sprintf("exec-patch-%d.bin", i)), orig, 0600); err != nil {
				return rollback(fmt.Errorf("runtime-icon backup could not be written: %w", err))
			}
		}
		if err = applyExecIconPatches(exePath, exeBytes, patches, replacements); err != nil {
			return rollback(fmt.Errorf("could not swap the running icon inside the app: %w", err))
		}
		exePatched = true
		e.State.Rebrands[idx].ExecPatches = patches
	}
	e.State.Rebrands[idx].RuntimeIcon = runtimeIcon
	e.State.Rebrands[idx].ExecIconCheck = ExecIconCheckVersion
	if err = t.Resign(ctx, a.Path); err != nil {
		return rollback(err)
	}
	if status, er := e.Native.Signature(ctx, a.Path); er != nil || status != "valid" {
		return rollback(errors.New("re-signed bundle did not verify"))
	}
	if got, er := ReadLimited(plistPath, 4<<20); er != nil || hash(got) != hash(plistBytes) {
		return rollback(errors.New("Info.plist changed while replacing the icon; stopped"))
	}
	after, err := Fingerprint(a)
	if err != nil {
		return rollback(err)
	}
	if current, er := ReadLimited(iconPath, MaxIconBytes); er == nil {
		e.State.Rebrands[idx].AfterIconHash = hash(current)
	}
	e.State.Rebrands[idx].AfterPrint = after
	e.State.Rebrands[idx].Status = "applied"
	e.State.Rebrands[idx].Held = false
	if err = e.save(); err != nil {
		return e.State.Rebrands[idx], fmt.Errorf("full icon applied, but saving history failed: %w", err)
	}
	if notifier, ok := e.Native.(interface {
		Notify(context.Context, string) error
	}); ok {
		_ = notifier.Notify(ctx, a.Path)
	}
	return e.State.Rebrands[idx], nil
}

// holdBundleIcons puts original icon files back for paused bundles whose
// Full icon is still the bytes IconCraft wrote. A changed bundle is not
// overwritten. The publisher signature is not reconstructed.
func (e *Engine) holdBundleIcons(ctx context.Context, onlyBundle string) NamePolicyResult {
	var out NamePolicyResult
	e.mu.Lock()
	rebrands := append([]RebrandRecord(nil), e.State.Rebrands...)
	e.mu.Unlock()
	seen := map[string]bool{}
	for i := len(rebrands) - 1; i >= 0; i-- {
		rec := rebrands[i]
		if rec.Kind != RebrandKindIcon || rec.BundleID == "" || rec.Status != "applied" || seen[rec.BundleID] {
			continue
		}
		seen[rec.BundleID] = true
		if onlyBundle != "" && rec.BundleID != onlyBundle {
			continue
		}
		if !e.bundlePaused(rec.BundleID) || rec.Held {
			continue
		}
		note, ok := e.holdOneBundleIcon(ctx, rec)
		if note != "" {
			out.Notes = append(out.Notes, note)
		}
		if ok {
			out.Icons++
		}
	}
	return out
}

func (e *Engine) holdOneBundleIcon(ctx context.Context, rec RebrandRecord) (string, bool) {
	app, ok := e.appByBundle(rec.BundleID, rec.App.Path)
	if !ok {
		app = rec.App
	}
	if note, stop := e.quitNote(ctx, RenameRecord{NewName: app.Name, From: app.Path, To: app.Path}); stop {
		return note, false
	}
	fp, err := Fingerprint(app)
	if err != nil {
		return app.Name + ": " + err.Error(), false
	}
	iconPath := filepath.Join(app.Path, "Contents/Resources", rec.IconFile)
	iconNow, _ := ReadLimited(iconPath, MaxIconBytes)
	switch {
	case rec.AfterPrint != "" && fp == rec.AfterPrint:
		if err = e.restoreHeldIconFiles(ctx, app, rec); err != nil {
			return app.Name + ": " + err.Error(), false
		}
		return "", e.markIconHeld(rec.ID, true)
	case rec.BeforeIconHash != "" && hash(iconNow) == rec.BeforeIconHash:
		if err = restoreDocIconFiles(app, rec, e.rebrandDir(rec.ID), true); err != nil {
			return app.Name + ": " + err.Error(), false
		}
		if t, er := e.tools(); er == nil {
			_ = t.Resign(ctx, app.Path)
		}
		return "", e.markIconHeld(rec.ID, true)
	default:
		e.mu.Lock()
		for i := range e.State.Rebrands {
			if e.State.Rebrands[i].ID == rec.ID {
				e.State.Rebrands[i].Held = true
				e.State.Rebrands[i].HeldPrint = ""
				break
			}
		}
		_ = e.save()
		e.mu.Unlock()
		return app.Name + ": the app changed, so its old icon was not written back", true
	}
}

func (e *Engine) restoreHeldIconFiles(ctx context.Context, app App, rec RebrandRecord) error {
	t, err := e.tools()
	if err != nil {
		return err
	}
	dir := e.rebrandDir(rec.ID)
	if len(rec.ExecPatches) > 0 {
		exePath, _, er := readExecutable(app)
		if er != nil {
			return er
		}
		if er = restoreExecIconPatches(exePath, rec.ExecPatches, dir); er != nil {
			return er
		}
	}
	iconPath := filepath.Join(app.Path, "Contents/Resources", rec.IconFile)
	if rec.IconExisted {
		iconBytes, er := ReadLimited(filepath.Join(dir, "icon.icns"), MaxIconBytes)
		if er != nil {
			return fmt.Errorf("full-icon backup is missing: %w", er)
		}
		if er = AtomicWrite(iconPath, iconBytes, 0644); er != nil {
			return er
		}
	} else if rec.IconFile != "" {
		_ = os.Remove(iconPath)
	}
	if err = restoreDocIconFiles(app, rec, dir, false); err != nil {
		return err
	}
	if err = t.Resign(ctx, app.Path); err != nil {
		return err
	}
	if status, er := e.Native.Signature(ctx, app.Path); er != nil || status != "valid" {
		return errors.New("re-signed bundle did not verify after restoring its icon")
	}
	return nil
}

func (e *Engine) markIconHeld(id string, withPrint bool) bool {
	appPath, bundle := "", ""
	e.mu.Lock()
	for i := range e.State.Rebrands {
		r := &e.State.Rebrands[i]
		if r.ID != id {
			continue
		}
		r.Held = true
		r.Status = "applied"
		appPath, bundle = r.App.Path, r.BundleID
		break
	}
	e.mu.Unlock()
	if withPrint {
		app, ok := e.appByBundle(bundle, appPath)
		if !ok {
			app = App{Path: appPath, BundleID: bundle}
		}
		if fp, err := Fingerprint(app); err == nil {
			e.mu.Lock()
			for i := range e.State.Rebrands {
				if e.State.Rebrands[i].ID == id {
					e.State.Rebrands[i].HeldPrint = fp
					break
				}
			}
			e.mu.Unlock()
		}
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	_ = e.save()
	return true
}

// releaseBundleIcons puts Full icon artwork back onto bundles that pause
// was holding, matched by bundle id. A bundle that changed while paused
// gets a fresh icon on its current files. The previous copy is not deleted.
func (e *Engine) releaseBundleIcons(ctx context.Context, onlyBundle string) NamePolicyResult {
	var out NamePolicyResult
	e.mu.Lock()
	rebrands := append([]RebrandRecord(nil), e.State.Rebrands...)
	e.mu.Unlock()
	seen := map[string]bool{}
	for i := len(rebrands) - 1; i >= 0; i-- {
		rec := rebrands[i]
		if rec.Kind != RebrandKindIcon || rec.BundleID == "" || !rec.Held || seen[rec.BundleID] {
			continue
		}
		if rec.Status != "applied" {
			continue
		}
		seen[rec.BundleID] = true
		if onlyBundle != "" && rec.BundleID != onlyBundle {
			continue
		}
		if e.bundlePaused(rec.BundleID) {
			continue
		}
		note, n := e.releaseOneBundleIcon(ctx, rec)
		if note != "" {
			out.Notes = append(out.Notes, note)
		}
		out.Icons += n
	}
	return out
}

func (e *Engine) releaseOneBundleIcon(ctx context.Context, rec RebrandRecord) (string, int) {
	app, ok := e.appByBundle(rec.BundleID, rec.App.Path)
	if !ok {
		return rec.Name + ": running icon not reapplied; rescan needed", 0
	}
	fp, err := Fingerprint(app)
	if err != nil {
		return app.Name + ": " + err.Error(), 0
	}
	if e.IconMode(app.BundleID) != IconModeFull {
		if rec.HeldPrint != "" && fp == rec.HeldPrint {
			_ = e.setRebrandStatus(rec.ID, "restored")
			return "", 1
		}
		if rec.AfterPrint != "" && fp == rec.AfterPrint && iconStillOurs(app, rec) {
			if _, er := e.RestoreRebrand(ctx, app.ID); er != nil {
				return app.Name + ": " + er.Error(), 0
			}
			return "", 1
		}
		_ = e.setRebrandStatus(rec.ID, "migrated")
		return "", 0
	}
	if _, er := e.ApplyBundleIcon(ctx, app.ID, rec.AssetID); er != nil {
		return app.Name + ": " + er.Error(), 0
	}
	_ = e.setRebrandStatus(rec.ID, "replaced")
	e.clearHeld(app.BundleID)
	return "", 1
}

// SyncIconModes applies or removes Full icon to match the saved mode.
// Paused bundles are left to holdBundleIcons.
func (e *Engine) SyncIconModes(ctx context.Context, onlyBundle string) NamePolicyResult {
	var out NamePolicyResult
	e.mu.Lock()
	rebrands := append([]RebrandRecord(nil), e.State.Rebrands...)
	prefs := map[string]string{}
	for k, v := range e.State.Preferences {
		prefs[k] = v.AssetID
	}
	e.mu.Unlock()
	released := e.releaseBundleIcons(ctx, onlyBundle)
	out.Icons += released.Icons
	out.Notes = append(out.Notes, released.Notes...)

	seen := map[string]bool{}
	for i := len(rebrands) - 1; i >= 0; i-- {
		rec := rebrands[i]
		if rec.Kind != RebrandKindIcon || rec.BundleID == "" || seen[rec.BundleID] {
			continue
		}
		if rec.Status != "applied" {
			continue
		}
		seen[rec.BundleID] = true
		if onlyBundle != "" && rec.BundleID != onlyBundle {
			continue
		}
		if e.bundlePaused(rec.BundleID) || rec.Held {
			continue
		}
		app, ok := e.appByBundle(rec.BundleID, rec.App.Path)
		if !ok {
			out.Notes = append(out.Notes, rec.Name+": running icon not updated; rescan needed")
			continue
		}
		fp, err := Fingerprint(app)
		if err != nil {
			out.Notes = append(out.Notes, app.Name+": "+err.Error())
			continue
		}
		if e.IconMode(app.BundleID) != IconModeFull {
			if rec.AfterPrint != "" && fp == rec.AfterPrint && iconStillOurs(app, rec) {
				if _, er := e.RestoreRebrand(ctx, app.ID); er != nil {
					out.Notes = append(out.Notes, app.Name+": "+er.Error())
				} else {
					out.Icons++
				}
			} else if er := e.setRebrandStatus(rec.ID, "migrated"); er != nil {
				out.Notes = append(out.Notes, app.Name+": "+er.Error())
			}
			continue
		}
		// The fingerprint covers Info.plist and the executable, not the
		// .icns. An updater can put the publisher icon back without
		// changing that fingerprint; the icon hash is what says the
		// running tile is still ours.
		if rec.AfterPrint != "" && fp == rec.AfterPrint && iconStillOurs(app, rec) {
			wantDocs := e.DocumentIcons(app.BundleID)
			switch {
			case wantDocs && !docIconsApplied(app, rec):
				changed, er := e.applyDocumentIcons(ctx, app, rec)
				if er != nil {
					out.Notes = append(out.Notes, app.Name+": "+er.Error())
				} else if changed {
					out.Icons++
				}
			case !wantDocs && docIconsStillCustom(app, rec):
				if er := e.restoreDocumentIconsOnly(ctx, app, rec); er != nil {
					out.Notes = append(out.Notes, app.Name+": "+er.Error())
				} else {
					out.Icons++
				}
			}
			continue
		}
		asset := rec.AssetID
		if asset == "" {
			asset = prefs[rec.BundleID]
		}
		if asset == "" {
			continue
		}
		if _, er := e.ApplyBundleIcon(ctx, app.ID, asset); er != nil {
			out.Notes = append(out.Notes, app.Name+": "+er.Error())
			continue
		}
		_ = e.setRebrandStatus(rec.ID, "replaced")
		out.Icons++
	}
	if e.UpdatesPaused() {
		return out
	}
	e.mu.Lock()
	apps := make([]App, 0, len(e.Apps))
	for _, a := range e.Apps {
		apps = append(apps, a)
	}
	e.mu.Unlock()
	for _, a := range apps {
		if onlyBundle != "" && a.BundleID != onlyBundle {
			continue
		}
		if seen[a.BundleID] || e.bundlePaused(a.BundleID) || e.IconMode(a.BundleID) != IconModeFull {
			continue
		}
		asset := prefs[a.BundleID]
		if asset == "" {
			continue
		}
		if _, er := e.ApplyBundleIcon(ctx, a.ID, asset); er != nil {
			out.Notes = append(out.Notes, a.Name+": "+er.Error())
			continue
		}
		out.Icons++
	}
	return out
}

func bundleIconHash(app App, rel string) string {
	if rel == "" || filepath.Base(rel) != rel {
		return ""
	}
	b, err := ReadLimited(filepath.Join(app.Path, "Contents/Resources", rel), MaxIconBytes)
	if err != nil {
		return ""
	}
	return hash(b)
}

func iconStillOurs(app App, rec RebrandRecord) bool {
	if rec.AfterIconHash == "" {
		return false
	}
	return bundleIconHash(app, rec.IconFile) == rec.AfterIconHash
}

// recoverBundleIcons rolls back a Full icon that was journalled as
// prepared and never finished. The backup is the publisher's icon file.
func (e *Engine) recoverBundleIcons(ctx context.Context) {
	e.mu.Lock()
	recs := append([]RebrandRecord(nil), e.State.Rebrands...)
	e.mu.Unlock()
	for _, rec := range recs {
		if rec.Kind != RebrandKindIcon || rec.Status != "prepared" {
			continue
		}
		app, ok := e.appByBundle(rec.BundleID, rec.App.Path)
		if !ok {
			app = rec.App
		}
		_ = e.restoreHeldIconFiles(ctx, app, rec)
		_ = e.setRebrandStatus(rec.ID, "failed")
	}
}
