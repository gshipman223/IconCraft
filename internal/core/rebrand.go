package core

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Optional native bundle tools, implemented by the macOS native layer.
// Kept as asserted interfaces so test doubles and other platforms degrade
// to a clear "unavailable" instead of failing to compile or half-applying.
type bundleTools interface {
	BundleIconFile(context.Context, string) (string, error)
	SetBundleName(context.Context, string, string) error
	Resign(context.Context, string) error
}

func (e *Engine) tools() (bundleTools, error) {
	t, ok := e.Native.(bundleTools)
	if !ok {
		return nil, errors.New("internal rebrand needs the macOS bundle tools")
	}
	return t, nil
}

func (e *Engine) rebrandDir(id string) string { return filepath.Join(e.Dir, "rebrands", id) }

// Rebrand applies the chosen name + icon *inside* a third-party app so the
// identity also shows while it runs (Dock, menu bar, switcher, Launchpad):
// CFBundleName/CFBundleDisplayName are set and the bundle's .icns replaced,
// then the bundle is ad-hoc re-signed. Bundle ID, executable, URL schemes
// and document types are untouched, so links and associations keep working.
//
// Safety: refuses protected/running apps and asset-catalog apps (no .icns
// to swap); journals a byte-exact backup first; verifies fingerprint and
// signature after; attempts rollback on any failure. Re-signing replaces
// the previous signature — the UI discloses this before opt-in, and app
// updaters may overwrite the rebrand (the keeper re-applies it; see
// Reconcile), at which point files match the backup fingerprint again.
func (e *Engine) Rebrand(ctx context.Context, appID, assetID, name string) (RebrandRecord, error) {
	name, err := ValidateName(name)
	if err != nil {
		return RebrandRecord{}, err
	}
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
		return RebrandRecord{}, errors.New("system apps cannot be rebranded")
	}
	if err := SafeApp(a.Path); err != nil {
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
			return RebrandRecord{}, errors.New("quit this application before applying the full skin")
		}
	}
	iconRel, err := t.BundleIconFile(ctx, a.Path)
	if err != nil {
		return RebrandRecord{}, err
	}
	if iconRel == "" || filepath.Base(iconRel) != iconRel || strings.Contains(iconRel, "..") {
		return RebrandRecord{}, errors.New("this app draws its icon from an asset catalog, so its running icon can't be swapped without rebuilding it — Finder skin still applies")
	}
	if !strings.HasSuffix(strings.ToLower(iconRel), ".icns") {
		iconRel += ".icns"
	}
	assetPath, err := e.assetPath(assetID)
	if err != nil {
		return RebrandRecord{}, errors.New("choose an icon first, then apply the full skin")
	}
	pngBytes, err := ReadLimited(assetPath, MaxIconBytes)
	if err != nil {
		return RebrandRecord{}, err
	}
	icns, err := makeICNS(pngBytes)
	if err != nil {
		return RebrandRecord{}, err
	}
	if _, err := e.Native.Signature(ctx, a.Path); err != nil {
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
	before, err := Fingerprint(a)
	if err != nil {
		return RebrandRecord{}, err
	}
	rec := RebrandRecord{ID: newID(), BundleID: a.BundleID, App: a, AssetID: assetID, Name: name, IconFile: iconRel, IconExisted: iconExisted, BeforePrint: before, Status: "prepared", Created: now()}
	if iconExisted {
		rec.BeforeIconHash = hash(iconBytes)
	}
	dir := e.rebrandDir(rec.ID)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return RebrandRecord{}, err
	}
	if err := AtomicWrite(filepath.Join(dir, "Info.plist"), plistBytes, 0600); err != nil {
		return RebrandRecord{}, err
	}
	if iconExisted {
		if err := AtomicWrite(filepath.Join(dir, "icon.icns"), iconBytes, 0600); err != nil {
			return RebrandRecord{}, err
		}
	}
	e.State.Rebrands = append(e.State.Rebrands, rec)
	idx := len(e.State.Rebrands) - 1
	if err := e.save(); err != nil {
		e.State.Rebrands = e.State.Rebrands[:idx]
		return RebrandRecord{}, fmt.Errorf("backup could not be committed; no change made: %w", err)
	}
	rollback := func(cause error) (RebrandRecord, error) {
		_ = AtomicWrite(plistPath, plistBytes, 0644)
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
	if err := AtomicWrite(iconPath, icns, 0644); err != nil {
		return rollback(fmt.Errorf("could not replace the bundle icon: %w", err))
	}
	if err := t.SetBundleName(ctx, a.Path, name); err != nil {
		return rollback(err)
	}
	if err := t.Resign(ctx, a.Path); err != nil {
		return rollback(err)
	}
	if status, er := e.Native.Signature(ctx, a.Path); er != nil || status != "valid" {
		return rollback(errors.New("re-signed bundle did not verify"))
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
	if err := e.save(); err != nil {
		return e.State.Rebrands[idx], fmt.Errorf("full skin applied, but saving history failed: %w", err)
	}
	if notifier, ok := e.Native.(interface {
		Notify(context.Context, string) error
	}); ok {
		_ = notifier.Notify(ctx, a.Path)
	}
	return e.State.Rebrands[idx], nil
}

// RestoreRebrand puts back the byte-exact backed-up Info.plist and icon and
// re-signs. It refuses when the bundle changed since the rebrand (e.g. the
// app updated itself) rather than forcing old files onto a new version.
func (e *Engine) RestoreRebrand(ctx context.Context, appID string) (RebrandRecord, error) {
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
	idx := -1
	for i := len(e.State.Rebrands) - 1; i >= 0; i-- {
		if e.State.Rebrands[i].BundleID == a.BundleID && e.State.Rebrands[i].Status == "applied" {
			idx = i
			break
		}
	}
	if idx < 0 {
		return RebrandRecord{}, errors.New("this app has no full skin to restore")
	}
	rec := e.State.Rebrands[idx]
	if runner, ok := e.Native.(interface {
		Running(context.Context, string) (bool, error)
	}); ok {
		running, er := runner.Running(ctx, a.Path)
		if er != nil {
			return rec, er
		}
		if running {
			return rec, errors.New("quit this application before restoring its original identity")
		}
	}
	current, err := Fingerprint(a)
	if err != nil {
		return rec, err
	}
	if rec.AfterPrint != "" && current != rec.AfterPrint {
		return rec, errors.New("the app changed since the full skin was applied (it may have updated); its original files have not been forced onto a different version")
	}
	dir := e.rebrandDir(rec.ID)
	plistBytes, err := ReadLimited(filepath.Join(dir, "Info.plist"), 4<<20)
	if err != nil {
		return rec, fmt.Errorf("full-skin backup is missing: %w", err)
	}
	plistPath := filepath.Join(a.Path, "Contents/Info.plist")
	iconPath := filepath.Join(a.Path, "Contents/Resources", rec.IconFile)
	if err := AtomicWrite(plistPath, plistBytes, 0644); err != nil {
		return rec, err
	}
	if rec.IconExisted {
		iconBytes, err := ReadLimited(filepath.Join(dir, "icon.icns"), MaxIconBytes)
		if err != nil {
			return rec, fmt.Errorf("full-skin icon backup is missing: %w", err)
		}
		if err := AtomicWrite(iconPath, iconBytes, 0644); err != nil {
			return rec, err
		}
	} else {
		_ = os.Remove(iconPath)
	}
	if err := t.Resign(ctx, a.Path); err != nil {
		return rec, err
	}
	// Verify the restored *files* and the new signature — not the full
	// fingerprint: re-signing rewrites the executable's signature bytes, so
	// the pre-skin fingerprint (which hashes them) can never match again,
	// even on a byte-perfect restore.
	restoredPlist, perr := ReadLimited(plistPath, 4<<20)
	restoredIconOK := true
	if rec.IconExisted {
		if b, er := ReadLimited(iconPath, MaxIconBytes); er != nil || hash(b) != rec.BeforeIconHash {
			restoredIconOK = false
		}
	}
	sigOK := false
	if status, er := e.Native.Signature(ctx, a.Path); er == nil && status == "valid" {
		sigOK = true
	}
	if perr != nil || hash(restoredPlist) != hash(plistBytes) || !restoredIconOK || !sigOK {
		e.State.Rebrands[idx].Status = "rollback-required"
		_ = e.save()
		return e.State.Rebrands[idx], errors.New("restored files or signature did not verify; manual check recommended")
	}
	e.State.Rebrands[idx].Status = "restored"
	if err := e.save(); err != nil {
		return e.State.Rebrands[idx], err
	}
	if notifier, ok := e.Native.(interface {
		Notify(context.Context, string) error
	}); ok {
		_ = notifier.Notify(ctx, a.Path)
	}
	return e.State.Rebrands[idx], nil
}

// RebrandedBundles lists bundle IDs with an applied full skin (for the UI).
func (e *Engine) RebrandedBundles() map[string]string {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := map[string]string{}
	for _, r := range e.State.Rebrands {
		if r.Status == "applied" {
			out[r.BundleID] = r.Name
		}
	}
	return out
}
