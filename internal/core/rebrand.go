package core

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
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

// Rebrand used to write the chosen name and icon into the app
// (CFBundleName, CFBundleDisplayName, the bundle .icns, sometimes the
// executable) and ad-hoc re-sign it. That keeps the bundle id, so a
// launcher that only checks the id still matches, but it destroys the
// Developer ID signature Sparkle and the Mac App Store use to recognise
// the same app, and codesign then rejects the bundle as modified.
//
// It now does the same safe customisation as Rename plus Apply: a display
// label and a Finder custom icon. Info.plist, the bundle icon, the
// executable and the signature are not modified. The Dock icon and menu
// bar title of a running app stay the publisher's; macOS reads those from
// the signed bundle and there is no supported override that leaves the
// signature intact.
//
// Existing full skins are undone by RepairIdentities, which still uses
// RestoreRebrand to put the backed-up files back.
func (e *Engine) Rebrand(ctx context.Context, appID, assetID, name string) (RebrandRecord, error) {
	app, err := e.Rename(ctx, appID, name)
	if err != nil {
		return RebrandRecord{}, err
	}
	if _, err := e.Apply(ctx, app.ID, assetID); err != nil {
		return RebrandRecord{}, err
	}
	return RebrandRecord{ID: newID(), BundleID: app.BundleID, App: app, AssetID: assetID, Name: name, Status: "external", Created: now()}, nil
}

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
	if err := e.probeWritable(a); err != nil {
		return rec, err
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
	if len(rec.ExecPatches) > 0 {
		exePath, _, er := readExecutable(a)
		if er != nil {
			return rec, er
		}
		if er := restoreExecIconPatches(exePath, rec.ExecPatches, dir); er != nil {
			return rec, er
		}
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
