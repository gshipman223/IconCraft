package core

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"
)

type RenameRecord struct {
	App         App    `json:"app"`
	ID          string `json:"id"`
	BundleID    string `json:"bundleId"`
	From        string `json:"from"`
	To          string `json:"to"`
	OldName     string `json:"oldName"`
	NewName     string `json:"newName"`
	Fingerprint string `json:"fingerprint"`
	Status      string `json:"status"`
}

func ValidateName(s string) (string, error) {
	s = strings.TrimSpace(s)
	if strings.HasSuffix(strings.ToLower(s), ".app") {
		s = s[:len(s)-4]
	}
	if s == "" || s == "." || s == ".." || strings.HasPrefix(s, ".") || len([]byte(s)) > 220 {
		return "", errors.New("choose a name between 1 and 220 bytes, without a leading dot")
	}
	for _, r := range s {
		if unicode.IsControl(r) || strings.ContainsRune("/\\:", r) {
			return "", errors.New("app names cannot contain slashes, colons or control characters")
		}
	}
	return s, nil
}

// Rename remembers the name IconCraft shows for this app. It does not rename
// the .app folder, edit Info.plist (CFBundleName, CFBundleDisplayName,
// CFBundleIdentifier), replace icon resources, or re-sign.
//
// Finder, Launchpad, the Dock and the menu bar keep the name macOS reads
// from the folder and the signed bundle. Those surfaces have no separate
// display-name store: a different Finder name means renaming the folder or
// editing the signed plist, and both make path-based launchers (ArtCraft
// looks up /Applications/<Official>.app and then CFBundleIdentifier) and
// signature-based updaters (Sparkle, the Mac App Store) treat the app as
// missing. The label lives in IconCraft's journal, so an update that
// replaces the bundle cannot wipe it and cannot hide the app.
//
// Windows and Linux (ports branch) rename a shortcut or desktop file via
// launcherNamer and return before this macOS path. Do not move the bundle
// here when that hook is added.
func (e *Engine) Rename(ctx context.Context, id, name string) (App, error) {
	name, err := ValidateName(name)
	if err != nil {
		return App{}, err
	}
	e.scanMu.Lock()
	defer e.scanMu.Unlock()
	e.mu.Lock()
	defer e.mu.Unlock()
	a, ok := e.Apps[id]
	if !ok {
		return App{}, errors.New("application not found; rescan")
	}
	if a.Protected {
		return a, errors.New("system apps cannot be renamed")
	}
	if err = SafeApp(a.Path); err != nil {
		return a, err
	}
	e.captureOriginalLocked(ctx, a) // save the original to the vault before first change
	if a.Name == name && labelForLocked(e, a) == name {
		return a, nil
	}
	fp, err := Fingerprint(a)
	if err != nil {
		return a, err
	}
	// From == To: the bundle stays where launchers already look.
	nr := RenameRecord{App: a, ID: newID(), BundleID: a.BundleID, From: a.Path, To: a.Path, OldName: a.Name, NewName: name, Fingerprint: fp, Status: "prepared"}
	idx := len(e.State.Renames)
	e.State.Renames = append(e.State.Renames, nr)
	if err = e.save(); err != nil {
		e.State.Renames = e.State.Renames[:idx]
		return a, err
	}
	if err = ctx.Err(); err != nil {
		return a, err
	}
	after, checkErr := Fingerprint(a)
	if checkErr != nil || after != fp {
		e.State.Renames[idx].Status = "failed"
		_ = e.save()
		return a, errors.New("app changed during rename; stopped and attempted rollback")
	}
	out := a
	out.Name = name
	out.Renamed = name != bundleFolderName(a.Path)
	out.IconStamp = PreviewStamp(a.Path)
	e.moveReferencesLocked(a, out)
	e.State.Renames[idx].Status = "applied"
	if err = e.save(); err != nil {
		return out, fmt.Errorf("name saved, but saving history failed: %w", err)
	}
	_ = e.saveInventoryLocked()
	return out, nil
}

func bundleFolderName(path string) string {
	return strings.TrimSuffix(filepath.Base(path), ".app")
}

// labelForLocked is the newest applied label for this exact bundle path.
// Caller holds e.mu.
func labelForLocked(e *Engine, a App) string {
	label := ""
	for _, nr := range e.State.Renames {
		if nr.Status == "applied" && nr.BundleID == a.BundleID && nr.To == a.Path && nr.From == nr.To {
			label = nr.NewName
		}
	}
	return label
}
func (e *Engine) moveReferencesLocked(old, new App) {
	delete(e.Apps, old.ID)
	e.Apps[new.ID] = new
	for i := range e.State.Records {
		r := &e.State.Records[i]
		if r.App.ID == old.ID && r.App.BundleID == old.BundleID {
			r.App = new
		}
	}
	for i := range e.State.Rebrands {
		r := &e.State.Rebrands[i]
		if r.App.ID == old.ID && r.BundleID == old.BundleID {
			r.App = new
		}
	}
}
func (e *Engine) RestoreName(ctx context.Context, id string) (App, error) {
	// Reverse one rename at a time, newest first, updating icon recovery paths.
	e.scanMu.Lock()
	defer e.scanMu.Unlock()
	e.mu.Lock()
	defer e.mu.Unlock()
	a, ok := e.Apps[id]
	if !ok {
		return a, errors.New("application not found; rescan")
	}
	for {
		idx := -1
		for i := len(e.State.Renames) - 1; i >= 0; i-- {
			r := e.State.Renames[i]
			if r.To == a.Path && r.BundleID == a.BundleID && r.Status == "applied" {
				idx = i
				break
			}
		}
		if idx < 0 {
			a.Renamed = false
			e.Apps[a.ID] = a
			_ = e.saveInventoryLocked()
			return a, nil
		}
		nr := e.State.Renames[idx]
		if err := ctx.Err(); err != nil {
			return a, err
		}
		// A label never moved the bundle, so clearing it does not depend on
		// the app still matching an old fingerprint or being quit. An update
		// may replace the bundle; the label still has to come off.
		if nr.From == nr.To {
			e.State.Renames[idx].Status = "restoring"
			if err := e.save(); err != nil {
				e.State.Renames[idx].Status = "applied"
				return a, err
			}
			old := a
			a.Name = nr.OldName
			a.Renamed = false
			a.IconStamp = PreviewStamp(a.Path)
			e.moveReferencesLocked(old, a)
			e.State.Renames[idx].Status = "restored"
			if err := e.save(); err != nil {
				return a, err
			}
			continue
		}
		if runner, ok := e.Native.(interface {
			Running(context.Context, string) (bool, error)
		}); ok {
			running, err := runner.Running(ctx, a.Path)
			if err != nil {
				return a, err
			}
			if running {
				return a, errors.New("quit this application before restoring its name")
			}
		}
		fp, err := Fingerprint(a)
		if err != nil || fp != nr.Fingerprint {
			return a, errors.New("application changed since rename; its old name has not been forced onto a replacement")
		}
		e.State.Renames[idx].Status = "restoring"
		if err = e.save(); err != nil {
			e.State.Renames[idx].Status = "applied"
			return a, err
		}
		if err = moveExclusive(a.Path, nr.From); err != nil {
			e.State.Renames[idx].Status = "applied"
			_ = e.save()
			return a, fmt.Errorf("could not restore name (another app may already use it): %w", err)
		}
		old := a
		a.Path = nr.From
		a.Name = nr.OldName
		a.ID = appID(a)
		a.Renamed = false
		a.IconStamp = PreviewStamp(a.Path)
		e.moveReferencesLocked(old, a)
		e.State.Renames[idx].Status = "restored"
		if err = e.save(); err != nil {
			return a, err
		}
	}
}

// A process interruption after the rename but before journal commit is reconciled
// only when exactly one of the paths exists and the fingerprint still matches.
func (e *Engine) recoverRenames() {
	dirty := false
	for i := range e.State.Renames {
		r := &e.State.Renames[i]
		if r.Status != "prepared" && r.Status != "restoring" {
			continue
		}
		_, fe := os.Lstat(r.From)
		_, te := os.Lstat(r.To)
		if r.App.BundleID == "" {
			continue
		}
		from := r.App
		from.Path = r.From
		from.ID = appID(from)
		to := r.App
		to.Path = r.To
		to.Name = r.NewName
		to.ID = appID(to)
		to.Renamed = true
		// A label (From == To) never moved a folder. Marking it applied
		// only updates the name IconCraft shows.
		if r.From == r.To && te == nil {
			fp, err := Fingerprint(to)
			if err != nil || fp != r.Fingerprint {
				continue
			}
			if cur, ok := e.Apps[to.ID]; ok {
				cur.Name = r.NewName
				cur.Renamed = r.NewName != bundleFolderName(r.To)
				e.Apps[to.ID] = cur
			} else {
				e.Apps[to.ID] = to
			}
			if r.Status == "restoring" {
				if cur, ok := e.Apps[to.ID]; ok {
					cur.Name = r.OldName
					cur.Renamed = false
					e.Apps[to.ID] = cur
				}
				r.Status = "restored"
			} else {
				r.Status = "applied"
			}
			dirty = true
			continue
		}
		if os.IsNotExist(fe) && te == nil {
			fp, err := Fingerprint(to)
			if err != nil || fp != r.Fingerprint {
				continue
			}
			e.moveReferencesLocked(from, to)
			r.Status = "applied"
			dirty = true
		} else if fe == nil && os.IsNotExist(te) {
			fp, err := Fingerprint(from)
			if err != nil || fp != r.Fingerprint {
				continue
			}
			e.moveReferencesLocked(to, from)
			if r.Status == "restoring" {
				r.Status = "restored"
			} else {
				r.Status = "failed"
			}
			dirty = true
		}
	}
	if dirty {
		_ = e.save()
		_ = e.saveInventoryLocked()
	}
}
