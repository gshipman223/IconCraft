package server

import (
	"context"

	"github.com/gshipman223/IconCraft/internal/core"
	"github.com/gshipman223/IconCraft/internal/i18n"
	"github.com/gshipman223/IconCraft/internal/native"
)

// portablePermissions is the Setup screen for Linux and Windows.
// There is no App Management permission on these systems: a normal
// account can write its own shortcuts, and that is the whole grant.
func (s *Server) portablePermissions(ctx context.Context, runChecks bool, info native.PlatformInfo) (map[string]any, error) {
	_ = ctx
	var dataErr error
	if runChecks {
		dataErr = s.Engine.CheckDataWritable()
	}
	state := s.Engine.SnapshotState()
	apps := s.Engine.InstalledApps()

	dataState, dataLabel := "ready", i18n.T("Ready")
	dataDetail := i18n.T("IconCraft's settings, original-icon backups and restore journals stay in a private folder in your account. No cloud storage is used.")
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

	launcherState, launcherLabel := "ready", i18n.T("Ready")
	launcherDetail := i18n.T("IconCraft can write the shortcuts and launcher files it uses to rename and re-icon apps. That does not need an administrator password.")
	if runChecks {
		if prober, ok := s.Engine.Native.(interface{ WriteProbe(string) error }); ok {
			if err := prober.WriteProbe(""); err != nil {
				launcherState, launcherLabel = "blocked", i18n.T("Blocked")
				launcherDetail = i18n.T("IconCraft could not write a launcher file in your account: %v", err)
			}
		}
	}

	keeperDesired, keeperSet := s.Engine.KeeperPreference()
	agent := native.KeepAgentStatus{}
	if provider, ok := s.Engine.Native.(keeperStatusProvider); ok {
		agent = provider.KeepAgentStatus()
	}
	keeperState, keeperLabel := "off", i18n.T("Off — optional")
	keeperDetail := i18n.T("Optional. When on, a per-user timer repairs icons or names that an update reverts while IconCraft is closed. Your finished skins remain either way.")
	if keeperDesired && agent.Installed {
		keeperState, keeperLabel = "ready", i18n.T("On")
		keeperDetail = i18n.T("On. A per-user timer repairs icons and names that an update reverts after IconCraft closes.")
	}

	launchDetail := i18n.T("This copy of IconCraft is running. It is not code-signed. On Windows, SmartScreen may ask the first time you open a downloaded copy. No administrator password is required for normal use.")
	if info.OS == "linux" {
		launchDetail = i18n.T("This copy of IconCraft is running. It is not code-signed. No administrator password is required for normal use.")
	}

	checks := []map[string]any{
		permissionCheck("launch", i18n.T("Opening IconCraft"), "ready", i18n.T("Ready — app opened"), launchDetail),
		permissionCheck("storage", i18n.T("IconCraft data & backups"), dataState, dataLabel, dataDetail),
		permissionCheck("discovery", i18n.T("Finding installed apps"), discoveryState, discoveryLabel, discoveryDetail),
		permissionCheck("launchers", i18n.T("Shortcuts and launchers"), launcherState, launcherLabel, launcherDetail),
		permissionCheck("background", i18n.T("Keep skins applied in the background"), keeperState, keeperLabel, keeperDetail),
		permissionCheck("notRequired", i18n.T("Permissions IconCraft does not request"), "not-required", i18n.T("Not requested"), i18n.T("No administrator rights and no permission to change other programs' files are required. IconCraft only writes shortcuts and settings in your own account.")),
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
		"signature": native.SignatureInfo{Kind: "unsigned"},
		"appAccess": core.PermissionCheck{},
		"keeper": map[string]any{
			"desired":       keeperDesired,
			"preferenceSet": keeperSet,
			"installed":     agent.Installed,
			"loaded":        agent.Loaded,
			"disabled":      agent.Disabled,
		},
	}, nil
}
