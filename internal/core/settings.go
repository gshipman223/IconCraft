package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Settings holds the app's basic behaviour preferences. They are
// journalled like everything else; side effects that live outside the
// journal (the launch-at-login LaunchAgent, Dock/menu-bar visibility)
// are reconciled from these values by the native layer.
type Settings struct {
	LaunchAtLogin    bool `json:"launchAtLogin"`
	StartInMenuBar   bool `json:"startInMenuBar"`
	ShowDockIcon     bool `json:"showDockIcon"`
	ShowMenuBarIcon  bool `json:"showMenuBarIcon"`
	NotifyRepaired   bool `json:"notifyRepaired"`
	NotifyAttention  bool `json:"notifyAttention"`
	AutoCheckUpdates bool `json:"autoCheckUpdates"`

	// Update-check cache, managed by RecordUpdateCheck (not user input).
	LastUpdateCheck string `json:"lastUpdateCheck,omitempty"`
	LatestVersion   string `json:"latestVersion,omitempty"`
}

// DefaultSettings are the out-of-box behaviours: visible in the Dock and
// the menu bar, told about skins that need attention, and quietly aware
// of new releases. Nothing launches at login until the user asks.
func DefaultSettings() Settings {
	return Settings{
		ShowDockIcon:     true,
		ShowMenuBarIcon:  true,
		NotifyAttention:  true,
		AutoCheckUpdates: true,
	}
}

// Settings returns the effective preferences (defaults until saved).
func (e *Engine) Settings() Settings {
	e.mu.Lock()
	defer e.mu.Unlock()
	if !e.State.SettingsSet {
		return DefaultSettings()
	}
	return e.State.Settings
}

// SaveSettings validates and journals new preferences. The one hard
// rule: IconCraft must always leave the user a way back in — the Dock
// icon and the menu bar icon may not both be hidden, and starting in
// the menu bar requires the menu bar icon to exist.
func (e *Engine) SaveSettings(s Settings) (Settings, error) {
	if !s.ShowDockIcon && !s.ShowMenuBarIcon {
		return Settings{}, errors.New("IconCraft needs at least one way back in: turn on the Dock icon or the menu bar icon (or both)")
	}
	if s.StartInMenuBar && !s.ShowMenuBarIcon {
		return Settings{}, errors.New("starting in the menu bar needs the menu bar icon turned on — otherwise there would be nothing to click after login")
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	// The update-check cache is machine-managed; never let a settings
	// save from an older screen wipe it.
	s.LastUpdateCheck = e.State.Settings.LastUpdateCheck
	s.LatestVersion = e.State.Settings.LatestVersion
	e.State.Settings = s
	e.State.SettingsSet = true
	if err := e.save(); err != nil {
		return Settings{}, err
	}
	return s, nil
}

// UpdateCheck is the outcome of asking GitHub about the latest release.
type UpdateCheck struct {
	Current         string `json:"current"`
	Latest          string `json:"latest"`
	UpdateAvailable bool   `json:"updateAvailable"`
	ReleaseURL      string `json:"releaseUrl"`
	CheckedAt       string `json:"checkedAt"`
}

// CheckForUpdate queries the GitHub releases API for the newest
// published version. It never downloads or installs anything — the app
// is ad-hoc signed, so updating stays a deliberate user action.
func (e *Engine) CheckForUpdate(ctx context.Context) (UpdateCheck, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.github.com/repos/gshipman223/IconCraft/releases/latest", nil)
	if err != nil {
		return UpdateCheck{}, err
	}
	req.Header.Set("User-Agent", "IconCraft/"+Version)
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return UpdateCheck{}, fmt.Errorf("could not reach GitHub to check for updates: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return UpdateCheck{}, fmt.Errorf("GitHub answered %s while checking for updates", resp.Status)
	}
	var body struct {
		TagName string `json:"tag_name"`
		HTMLURL string `json:"html_url"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return UpdateCheck{}, err
	}
	latest := strings.TrimPrefix(strings.TrimSpace(body.TagName), "v")
	if latest == "" {
		return UpdateCheck{}, errors.New("GitHub did not name a latest release")
	}
	out := UpdateCheck{
		Current:         Version,
		Latest:          latest,
		UpdateAvailable: compareVersions(latest, Version) > 0,
		ReleaseURL:      body.HTMLURL,
		CheckedAt:       now(),
	}
	e.mu.Lock()
	e.State.Settings.LatestVersion = latest
	e.State.Settings.LastUpdateCheck = out.CheckedAt
	_ = e.save()
	e.mu.Unlock()
	return out, nil
}

// compareVersions compares dotted numeric versions (1.10.0 > 1.9.9).
func compareVersions(a, b string) int {
	pa, pb := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < len(pa) || i < len(pb); i++ {
		var na, nb int
		if i < len(pa) {
			na, _ = strconv.Atoi(pa[i])
		}
		if i < len(pb) {
			nb, _ = strconv.Atoi(pb[i])
		}
		if na != nb {
			if na > nb {
				return 1
			}
			return -1
		}
	}
	return 0
}
