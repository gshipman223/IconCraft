package server

import (
	"context"

	"github.com/gshipman223/IconCraft/internal/core"
	"github.com/gshipman223/IconCraft/internal/i18n"
	"github.com/gshipman223/IconCraft/internal/native"
)

type keeperStatusProvider interface {
	KeepAgentStatus() native.KeepAgentStatus
}

type selfSignatureProvider interface {
	SelfSignature(context.Context) native.SignatureInfo
}

// signatureCopy words the App Management and launch rows for how this
// copy is actually signed. A Developer ID identity (notarised or not)
// keeps TCC grants across updates; an ad-hoc build is a new app to macOS
// every time; anything else gets neutral advice rather than a guess.
func signatureCopy(sig native.SignatureInfo) (appManagement, launch string) {
	switch {
	case sig.DeveloperID() && sig.Notarized:
		return i18n.T("This copy is signed with a Developer ID and notarised by Apple, so macOS recognises updates as the same app and keeps this permission across them."),
			i18n.T("This copy of IconCraft launched successfully. It is signed with a Developer ID and notarised by Apple, so downloads open with a double-click and no Gatekeeper warning. Never disable Gatekeeper globally.")
	case sig.DeveloperID():
		return i18n.T("This copy is signed with a Developer ID, so macOS recognises updates as the same app and keeps this permission across them."),
			i18n.T("This copy of IconCraft launched successfully. If a future downloaded copy is blocked, macOS may ask you to use Privacy & Security → Open Anyway once; never disable Gatekeeper globally.")
	case sig.Kind == "adhoc":
		return i18n.T("Because this copy is locally ad-hoc signed, macOS may treat an updated IconCraft copy as new: if changes stop working after an update, open App Management and check IconCraft again."),
			i18n.T("This copy of IconCraft launched successfully. If a future downloaded copy is blocked, macOS may ask you to use Privacy & Security → Open Anyway once; never disable Gatekeeper globally.")
	default:
		return i18n.T("If changes stop working after an IconCraft update, open App Management and check IconCraft again."),
			i18n.T("This copy of IconCraft launched successfully. If a future downloaded copy is blocked, macOS may ask you to use Privacy & Security → Open Anyway once; never disable Gatekeeper globally.")
	}
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
//
// All prose here is user-facing and passes through i18n.T; the probe
// detail recorded by the engine stays English in the journal and is
// localised on the way out with i18n.Text.
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

	dataState, dataLabel := "ready", i18n.T("Ready")
	dataDetail := i18n.T("Settings, original-icon backups, restore journals and imported artwork stay in IconCraft's private folder on this Mac. No account or cloud storage is used.")
	if runChecks && dataErr != nil {
		dataState, dataLabel = "blocked", i18n.T("Blocked")
		dataDetail = i18n.T("IconCraft could not create a temporary file in its own data folder: %v", dataErr)
	}

	discoveryState, discoveryLabel := "ready", i18n.T("Ready")
	discoveryDetail := i18n.TN(len(apps), "IconCraft can see 1 installed app. It reads app names, bundle IDs and icons; reading an app does not change it.", "IconCraft can see %d installed apps. It reads app names, bundle IDs and icons; reading an app does not change it.", len(apps))
	if len(apps) == 0 {
		discoveryState, discoveryLabel = "unknown", i18n.T("Checking")
		discoveryDetail = i18n.T("No apps have been loaded yet. Wait for the first scan to finish, then run the checks again.")
	}

	accessState := access.Status
	if accessState == "" {
		accessState = "unknown"
	}
	accessLabel := map[string]string{"ready": i18n.T("Granted — tested"), "blocked": i18n.T("Blocked"), "unknown": i18n.T("Not checked yet")}[accessState]
	if accessLabel == "" {
		accessLabel, accessState = i18n.T("Not checked yet"), "unknown"
	}
	accessDetail := i18n.Text(access.Detail)
	if accessDetail == "" {
		accessDetail = i18n.T("macOS does not give apps a reliable global App Management readout, so IconCraft tests the real thing: a harmless create-and-remove file inside one installed app. Run the check before relying on changes to protected apps.")
	}

	sig := native.SignatureInfo{Kind: "unknown"}
	if provider, ok := s.Engine.Native.(selfSignatureProvider); ok {
		sig = provider.SelfSignature(ctx)
	}
	accessSigning, launchDetail := signatureCopy(sig)

	keeperDesired, keeperSet := s.Engine.KeeperPreference()
	agent := native.KeepAgentStatus{}
	if provider, ok := s.Engine.Native.(keeperStatusProvider); ok {
		agent = provider.KeepAgentStatus()
	}
	keeperState, keeperLabel := "off", i18n.T("Off — optional")
	keeperDetail := i18n.T("Optional. When on, a tiny background helper repairs icons or names that another app or updater reverts while IconCraft is closed. Your finished skins remain on disk either way.")
	switch {
	case agent.Disabled:
		keeperState, keeperLabel = "blocked", i18n.T("Turned off in macOS")
		keeperDetail = i18n.T("The keep-applied helper is disabled in macOS background-item settings. Turn it on again here, or allow IconCraft's background item in System Settings → General → Login Items.")
	case keeperDesired && agent.Installed && agent.Loaded:
		keeperState, keeperLabel = "ready", i18n.T("Granted — on")
		keeperDetail = i18n.T("On and loaded by macOS. It can keep repairing reverted icons and names after IconCraft closes.")
	case keeperDesired && agent.Installed:
		keeperState, keeperLabel = "blocked", i18n.T("Installed, not loaded")
		keeperDetail = i18n.T("The helper's file is present but macOS has not loaded it. Turn Keep skins applied off and on again; IconCraft also restores a missing helper automatically at launch.")
	case keeperDesired:
		keeperState, keeperLabel = "blocked", i18n.T("Missing")
		keeperDetail = i18n.T("You asked for the background helper, but macOS does not currently have it installed. IconCraft restores it at launch, or you can turn it on again now.")
	case agent.Installed:
		keeperState, keeperLabel = "off", i18n.T("Off — optional")
		keeperDetail = i18n.T("The helper file exists but your saved choice is off. Turn it on if you want automatic repairs while IconCraft is closed.")
	}
	if agent.Error != "" && keeperState == "ready" {
		keeperDetail += " (" + i18n.Text(agent.Error) + ")"
	}

	checks := []map[string]any{
		permissionCheck("launch", i18n.T("Opening IconCraft"), "ready", i18n.T("Ready — app opened"), launchDetail),
		permissionCheck("storage", i18n.T("IconCraft data & backups"), dataState, dataLabel, dataDetail),
		permissionCheck("discovery", i18n.T("Finding installed apps"), discoveryState, discoveryLabel, discoveryDetail),
		permissionCheck("appManagement", i18n.T("App Management — changing other apps"), accessState, accessLabel, accessDetail+" "+accessSigning),
		permissionCheck("background", i18n.T("Keep skins applied in the background"), keeperState, keeperLabel, keeperDetail),
		permissionCheck("notRequired", i18n.T("Permissions IconCraft does not request"), "not-required", i18n.T("Not requested"), i18n.T("No Full Disk Access, Accessibility, Screen Recording, Automation, microphone or camera permission is needed for IconCraft's normal work. It does not ask for broad access to your files or control of other apps.")),
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
		"signature": sig,
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
