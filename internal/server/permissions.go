package server

import (
	"context"
	"fmt"

	"github.com/gshipman223/IconCraft/internal/core"
	"github.com/gshipman223/IconCraft/internal/native"
)

type keeperStatusProvider interface {
	KeepAgentStatus() native.KeepAgentStatus
}

func permissionCheck(id, title, state, label, detail string) map[string]any {
	return map[string]any{"id": id, "title": title, "state": state, "label": label, "detail": detail}
}

// permissionsPayload builds the honest, observable permission picture.
// It never infers a global App Management grant from merely opening System
// Settings: only the recorded, user-run bundle-writability probe can move
// that row to "Granted — tested". Explicit checks may create/remove one
// temporary file in IconCraft's own data folder and inside one test app;
// status reads alone are non-mutating.
func (s *Server) permissionsPayload(ctx context.Context, runChecks bool, appID string) (map[string]any, error) {
	var dataErr error
	var access core.PermissionCheck
	if runChecks {
		dataErr = s.Engine.CheckDataWritable()
		var err error
		access, err = s.Engine.CheckAppAccess(ctx, appID)
		if err != nil {
			return nil, err
		}
	} else {
		access = s.Engine.SnapshotState().AppAccess
	}

	state := s.Engine.SnapshotState()
	if !runChecks {
		access = state.AppAccess
	}
	apps := s.Engine.InstalledApps()

	dataState, dataLabel := "ready", "Ready"
	dataDetail := "Settings, original-icon backups, restore journals and imported artwork stay in IconCraft's private folder on this Mac. No account or cloud storage is used."
	if runChecks && dataErr != nil {
		dataState, dataLabel = "blocked", "Blocked"
		dataDetail = fmt.Sprintf("IconCraft could not create a temporary file in its own data folder: %v", dataErr)
	}

	discoveryState, discoveryLabel := "ready", "Ready"
	discoveryDetail := fmt.Sprintf("IconCraft can see %d installed apps. It reads app names, bundle IDs and icons; reading an app does not change it.", len(apps))
	if len(apps) == 0 {
		discoveryState, discoveryLabel = "unknown", "Checking"
		discoveryDetail = "No apps have been loaded yet. Wait for the first scan to finish, then run the checks again."
	}

	accessState := access.Status
	if accessState == "" {
		accessState = "unknown"
	}
	accessLabel := map[string]string{"ready": "Granted — tested", "blocked": "Blocked", "unknown": "Not checked yet"}[accessState]
	if accessLabel == "" {
		accessLabel, accessState = "Not checked yet", "unknown"
	}
	accessDetail := access.Detail
	if accessDetail == "" {
		accessDetail = "macOS does not give ad-hoc apps a reliable global App Management readout, so IconCraft tests the real thing: a harmless create-and-remove file inside one installed app. Run the check before relying on changes to protected apps."
	}

	keeperDesired, keeperSet := s.Engine.KeeperPreference()
	agent := native.KeepAgentStatus{}
	if provider, ok := s.Engine.Native.(keeperStatusProvider); ok {
		agent = provider.KeepAgentStatus()
	}
	keeperState, keeperLabel := "off", "Off — optional"
	keeperDetail := "Optional. When on, a tiny background helper repairs icons or names that another app or updater reverts while IconCraft is closed. Your finished skins remain on disk either way."
	switch {
	case agent.Disabled:
		keeperState, keeperLabel = "blocked", "Turned off in macOS"
		keeperDetail = "The keep-applied helper is disabled in macOS background-item settings. Turn it on again here, or allow IconCraft's background item in System Settings → General → Login Items."
	case keeperDesired && agent.Installed && agent.Loaded:
		keeperState, keeperLabel = "ready", "Granted — on"
		keeperDetail = "On and loaded by macOS. It can keep repairing reverted icons and names after IconCraft closes."
	case keeperDesired && agent.Installed:
		keeperState, keeperLabel = "blocked", "Installed, not loaded"
		keeperDetail = "The helper's file is present but macOS has not loaded it. Turn Keep skins applied off and on again; IconCraft also restores a missing helper automatically at launch."
	case keeperDesired:
		keeperState, keeperLabel = "blocked", "Missing"
		keeperDetail = "You asked for the background helper, but macOS does not currently have it installed. IconCraft restores it at launch, or you can turn it on again now."
	case agent.Installed:
		keeperState, keeperLabel = "off", "Off — optional"
		keeperDetail = "The helper file exists but your saved choice is off. Turn it on if you want automatic repairs while IconCraft is closed."
	}
	if agent.Error != "" && keeperState == "ready" {
		keeperDetail += " (" + agent.Error + ")"
	}

	checks := []map[string]any{
		permissionCheck("launch", "Opening IconCraft", "ready", "Ready — app opened", "This copy of IconCraft launched successfully. If a future downloaded copy is blocked, macOS may ask you to use Privacy & Security → Open Anyway once; never disable Gatekeeper globally."),
		permissionCheck("storage", "IconCraft data & backups", dataState, dataLabel, dataDetail),
		permissionCheck("discovery", "Finding installed apps", discoveryState, discoveryLabel, discoveryDetail),
		permissionCheck("appManagement", "App Management — changing other apps", accessState, accessLabel, accessDetail+" Because this copy is locally ad-hoc signed, macOS may treat an updated IconCraft copy as new: if changes stop working after an update, open App Management and check IconCraft again."),
		permissionCheck("background", "Keep skins applied in the background", keeperState, keeperLabel, keeperDetail),
		permissionCheck("notRequired", "Permissions IconCraft does not request", "not-required", "Not requested", "No Full Disk Access, Accessibility, Screen Recording, Automation, microphone or camera permission is needed for IconCraft's normal work. It does not ask for broad access to your files or control of other apps."),
	}

	setupRequired := state.SetupVersion < core.SetupVersion
	return map[string]any{
		"setup": map[string]any{
			"required":       setupRequired,
			"completed":      !setupRequired,
			"version":        state.SetupVersion,
			"currentVersion": core.SetupVersion,
			"completedAt":    state.SetupDoneAt,
		},
		"appsFound": len(apps),
		"checks":    checks,
		"appAccess": access,
		"keeper": map[string]any{
			"desired":       keeperDesired,
			"preferenceSet": keeperSet,
			"installed":     agent.Installed,
			"loaded":        agent.Loaded,
			"disabled":      agent.Disabled,
		},
	}, nil
}
