package core

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
)

// SetupRequired reports whether this install still needs the current
// first-run Setup & Permissions review. Skipping the review does not set
// this: only the explicit Finish setup action does.
func (e *Engine) SetupRequired() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.State.SetupVersion < SetupVersion
}

// CompleteSetup records that the user has reviewed the current setup and
// permissions information. Individual permissions remain independently
// testable afterwards; completing setup never claims a grant by itself.
func (e *Engine) CompleteSetup() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	oldVersion, oldDoneAt := e.State.SetupVersion, e.State.SetupDoneAt
	e.State.SetupVersion = SetupVersion
	e.State.SetupDoneAt = now()
	if err := e.save(); err != nil {
		e.State.SetupVersion, e.State.SetupDoneAt = oldVersion, oldDoneAt
		return err
	}
	return nil
}

// CheckDataWritable proves IconCraft can still create its local settings,
// journals and icon backups. The probe file is removed immediately and no
// application bundle is touched.
func (e *Engine) CheckDataWritable() error {
	f, err := os.CreateTemp(e.Dir, ".iconcraft-permission-check-")
	if err != nil {
		return err
	}
	name := f.Name()
	if err := f.Close(); err != nil {
		_ = os.Remove(name)
		return err
	}
	return os.Remove(name)
}

type appRunner interface {
	Running(context.Context, string) (bool, error)
}

func permissionDenied(err error) bool {
	return errors.Is(err, os.ErrPermission) || strings.Contains(strings.ToLower(err.Error()), "operation not permitted")
}

func setupCandidateRank(a App) int {
	switch {
	case strings.HasPrefix(a.Path, "/Applications/"):
		return 0
	case strings.Contains(a.Path, "/Applications/"):
		return 1
	default:
		return 2
	}
}

// CheckAppAccess runs the same harmless bundle-writability probe used before
// every real icon change. When appID is empty it picks the first suitable,
// not-running installed app it can test. A success is reported for that app
// and access class — not as a fabricated global TCC database reading. A
// permission refusal is strong evidence that App Management is blocking
// IconCraft and is returned immediately.
func (e *Engine) CheckAppAccess(ctx context.Context, appID string) (PermissionCheck, error) {
	apps := e.InstalledApps()
	candidates := make([]App, 0, len(apps))
	for _, a := range apps {
		if a.Protected || SafeApp(a.Path) != nil {
			continue
		}
		if appID != "" {
			if a.ID == appID {
				candidates = []App{a}
				break
			}
			continue
		}
		candidates = append(candidates, a)
	}
	if appID != "" && len(candidates) == 0 {
		return PermissionCheck{}, errors.New("that app is not available for a permission check; refresh the app list and try again")
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		ri, rj := setupCandidateRank(candidates[i]), setupCandidateRank(candidates[j])
		if ri != rj {
			return ri < rj
		}
		return strings.ToLower(candidates[i].Name) < strings.ToLower(candidates[j].Name)
	})

	check := PermissionCheck{Status: "unknown", CheckedAt: now(), Detail: "No suitable installed app was available to test yet. Refresh the app list, then run the check again."}
	prober, ok := e.Native.(writeProber)
	if !ok {
		check.Detail = "This system cannot run the native bundle-writability check."
		e.recordAppAccess(check)
		return check, nil
	}
	runner, hasRunner := e.Native.(appRunner)
	tested := 0
	for _, a := range candidates {
		if err := ctx.Err(); err != nil {
			return PermissionCheck{}, err
		}
		if appID == "" && hasRunner {
			if running, err := runner.Running(ctx, a.Path); err == nil && running {
				continue
			}
		}
		tested++
		err := prober.WriteProbe(a.Path)
		check.AppID, check.AppName, check.Path = a.ID, a.Name, a.Path
		if err == nil {
			check.Status = "ready"
			check.Detail = fmt.Sprintf("Test passed on %s: macOS allowed IconCraft to create and remove a temporary file inside that app — the exact write access needed before changing its icon or name.", a.Name)
			e.recordAppAccess(check)
			return check, nil
		}
		if permissionDenied(err) {
			check.Status = "blocked"
			check.Detail = fmt.Sprintf("Test blocked on %s: macOS refused a harmless temporary write inside the app. Turn IconCraft on in System Settings → Privacy & Security → App Management, then check again.", a.Name)
			e.recordAppAccess(check)
			return check, nil
		}
		check.Detail = fmt.Sprintf("Could not complete the test on %s: %v", a.Name, err)
		if appID != "" || tested >= 12 {
			break
		}
	}
	e.recordAppAccess(check)
	return check, nil
}

func (e *Engine) recordAppAccess(check PermissionCheck) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.State.AppAccess = check
	_ = e.save()
}
