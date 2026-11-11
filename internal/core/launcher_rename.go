package core

import (
	"context"
	"errors"
	"fmt"
)

// launcherNamer renames an app by editing the user's shortcut or desktop
// override. The program it launches is not moved or rewritten. macOS does
// not implement this; folder renames stay on the existing path.
type launcherNamer interface {
	ApplyLauncherName(context.Context, App, string) (App, error)
	RestoreLauncherName(context.Context, App, RenameRecord, string) (App, error)
	LauncherNameIntact(RenameRecord) bool
}

// finishLauncherRename journals a launcher rename and applies it.
// The caller holds both engine locks.
func (e *Engine) finishLauncherRename(ctx context.Context, a App, name string, namer launcherNamer) (App, error) {
	if a.Name == name {
		return a, nil
	}
	fp, err := Fingerprint(a)
	if err != nil {
		return a, err
	}
	to := a.Path
	if p, ok := namer.(interface {
		LauncherRenameDest(App, string) string
	}); ok {
		if dest := p.LauncherRenameDest(a, name); dest != "" {
			to = dest
		}
	}
	nr := RenameRecord{App: a, ID: newID(), BundleID: a.BundleID, From: a.Path, To: to, OldName: a.Name, NewName: name, Fingerprint: fp, Status: "prepared"}
	idx := len(e.State.Renames)
	e.State.Renames = append(e.State.Renames, nr)
	if err = e.save(); err != nil {
		e.State.Renames = e.State.Renames[:idx]
		return a, err
	}
	if err = ctx.Err(); err != nil {
		return a, err
	}
	out, err := namer.ApplyLauncherName(ctx, a, name)
	if err != nil {
		e.State.Renames[idx].Status = "failed"
		_ = e.save()
		return a, err
	}
	if out.Path == "" {
		out.Path = to
	}
	out.Name = name
	out.Renamed = true
	out.ID = appID(out)
	out.IconStamp = e.seenStamp(out)
	after, checkErr := Fingerprint(out)
	if checkErr != nil || after != fp {
		if back, rerr := namer.RestoreLauncherName(context.Background(), out, e.State.Renames[idx], a.Name); rerr == nil && back.Path != "" {
			_ = back
		}
		e.State.Renames[idx].Status = "failed"
		_ = e.save()
		return a, errors.New("app changed during rename; stopped and attempted rollback")
	}
	e.State.Renames[idx].To = out.Path
	e.State.Renames[idx].Status = "applied"
	e.moveReferencesLocked(a, out)
	if err = e.save(); err != nil {
		return out, fmt.Errorf("app renamed, but saving history failed: %w", err)
	}
	_ = e.saveInventoryLocked()
	if notifier, ok := e.Native.(interface {
		Notify(context.Context, string) error
	}); ok {
		_ = notifier.Notify(ctx, out.Path)
	}
	return out, nil
}

// finishLauncherRestore unwinds launcher renames newest first.
// The caller holds both engine locks. A running app is not a reason to
// refuse: the program file is never the thing being renamed.
func (e *Engine) finishLauncherRestore(ctx context.Context, a App, namer launcherNamer) (App, error) {
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
		fp, err := Fingerprint(a)
		if err != nil || fp != nr.Fingerprint {
			return a, errors.New("application changed since rename; its old name has not been forced onto a replacement")
		}
		e.State.Renames[idx].Status = "restoring"
		if err = e.save(); err != nil {
			e.State.Renames[idx].Status = "applied"
			return a, err
		}
		out, err := namer.RestoreLauncherName(ctx, a, nr, nr.OldName)
		if err != nil {
			e.State.Renames[idx].Status = "applied"
			_ = e.save()
			return a, fmt.Errorf("could not restore name: %w", err)
		}
		if out.Path == "" {
			out.Path = nr.From
		}
		out.Name = nr.OldName
		out.ID = appID(out)
		out.BundleID = a.BundleID
		out.IconStamp = e.seenStamp(out)
		e.moveReferencesLocked(a, out)
		e.State.Renames[idx].Status = "restored"
		if err = e.save(); err != nil {
			return out, err
		}
		a = out
	}
}

// recoverLauncherRenames finishes a launcher rename that was written to
// disk but not yet marked applied. It is a no-op for macOS folder renames.
func (e *Engine) recoverLauncherRenames(r *RenameRecord, fromExists, toExists bool) bool {
	namer, ok := e.Native.(interface {
		LauncherNameIntact(RenameRecord) bool
	})
	if !ok || !toExists {
		return false
	}
	if !namer.LauncherNameIntact(*r) {
		return false
	}
	to := r.App
	to.Path = r.To
	to.Name = r.NewName
	to.ID = appID(to)
	to.Renamed = true
	fp, err := Fingerprint(to)
	if err != nil || fp != r.Fingerprint {
		return false
	}
	from := r.App
	from.Path = r.From
	from.ID = appID(from)
	if r.Status == "prepared" && (r.From == r.To || fromExists) {
		e.moveReferencesLocked(from, to)
		r.Status = "applied"
		return true
	}
	return false
}

// launcherNameNeedsRepair reports that the renamed launcher is still on
// disk but no longer shows the journalled name. macOS folder renames do
// not use this: a present destination folder is the renamed app.
func launcherNameNeedsRepair(n Native, nr RenameRecord) bool {
	namer, ok := n.(interface {
		LauncherNameIntact(RenameRecord) bool
	})
	if !ok {
		return false
	}
	return !namer.LauncherNameIntact(nr)
}
