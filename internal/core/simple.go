package core

import (
	"context"
	"errors"
	"sort"
)

// RememberOriginal only caches a preview. It never changes any application.
func (e *Engine) RememberOriginal(craftID, product, source, assetID string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if !validCraftID.MatchString(craftID) {
		return errors.New("unknown app identity")
	}
	if _, err := e.assetPath(assetID); err != nil {
		return err
	}
	prior, had := e.State.Originals[craftID]
	if had && prior.AssetID == assetID && prior.Source == source && prior.Product == product {
		return nil
	}
	e.State.Originals[craftID] = Original{AssetID: assetID, Product: product, Source: source, Checked: now()}
	if err := e.save(); err != nil {
		if had {
			e.State.Originals[craftID] = prior
		} else {
			delete(e.State.Originals, craftID)
		}
		return err
	}
	return nil
}
func (e *Engine) RememberChoice(appID, assetID string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	app, ok := e.Apps[appID]
	if !ok {
		return errors.New("rescan applications first")
	}
	if assetID != "" {
		if _, err := e.assetPath(assetID); err != nil {
			return err
		}
	}
	old, had := e.State.Choices[app.BundleID]
	if assetID == "" {
		delete(e.State.Choices, app.BundleID)
	} else {
		e.State.Choices[app.BundleID] = assetID
	}
	if err := e.save(); err != nil {
		if had {
			e.State.Choices[app.BundleID] = old
		} else {
			delete(e.State.Choices, app.BundleID)
		}
		return err
	}
	return nil
}
func (e *Engine) InstalledApps() []App {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := []App{}
	for _, a := range e.Apps {
		out = append(out, a)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}

// RestoreOriginal unwinds ALL IconCraft changes for this app, not just one layer.
// Each layer retains the original fingerprint/conflict and exact-metadata checks.
// Records are matched by stable bundle ID as well as current app ID, so an app
// that moved or was renamed since skinning still finds its layers.
func (e *Engine) RestoreOriginal(ctx context.Context, appID string) (int, error) {
	app, err := e.GetApp(appID)
	if err != nil {
		return 0, err
	}
	count := 0
	for {
		state := e.SnapshotState()
		latest := ""
		for i := len(state.Records) - 1; i >= 0; i-- {
			r := state.Records[i]
			if (r.App.ID == appID || (app.BundleID != "" && r.App.BundleID == app.BundleID)) && isLive(r.Status) {
				latest = r.ID
				break
			}
		}
		if latest == "" {
			return count, nil
		}
		if err := ctx.Err(); err != nil {
			return count, err
		}
		if _, err := e.Restore(ctx, latest); err != nil {
			return count, err
		}
		count++
	}
}
