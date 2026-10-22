package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
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
	if _, ok := parseVersion(latest); !ok {
		return UpdateCheck{}, fmt.Errorf("GitHub named release %q, which is not a version IconCraft can compare", body.TagName)
	}
	out := UpdateCheck{
		Current:         Version,
		Latest:          latest,
		UpdateAvailable: NewerVersion(latest, Version),
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

// NewerVersion reports whether latest is a strictly newer release than
// current. Both must be well-formed semantic versions (an optional "v"
// prefix, MAJOR.MINOR[.PATCH], optional -prerelease and +build); a
// prerelease sorts before its release (0.14.0-beta.1 < 0.14.0), and a tag
// IconCraft cannot parse is never offered as an update.
func NewerVersion(latest, current string) bool {
	l, okL := parseVersion(latest)
	c, okC := parseVersion(current)
	return okL && okC && compareParsed(l, c) > 0
}

type parsedVersion struct {
	nums [3]int
	pre  []string
}

var versionRE = regexp.MustCompile(`^v?(\d+)\.(\d+)(?:\.(\d+))?(?:-([0-9A-Za-z.-]+))?(?:\+[0-9A-Za-z.-]+)?$`)

func parseVersion(s string) (parsedVersion, bool) {
	m := versionRE.FindStringSubmatch(strings.TrimSpace(s))
	if m == nil {
		return parsedVersion{}, false
	}
	var v parsedVersion
	for i, part := range m[1:4] {
		if part == "" {
			continue
		}
		n, err := strconv.Atoi(part)
		if err != nil {
			return parsedVersion{}, false
		}
		v.nums[i] = n
	}
	if m[4] != "" {
		v.pre = strings.Split(m[4], ".")
	}
	return v, true
}

func compareParsed(a, b parsedVersion) int {
	for i := 0; i < 3; i++ {
		if a.nums[i] != b.nums[i] {
			if a.nums[i] > b.nums[i] {
				return 1
			}
			return -1
		}
	}
	switch {
	case len(a.pre) == 0 && len(b.pre) == 0:
		return 0
	case len(a.pre) == 0:
		return 1 // release beats its prerelease
	case len(b.pre) == 0:
		return -1
	}
	for i := 0; i < len(a.pre) || i < len(b.pre); i++ {
		if i >= len(a.pre) {
			return -1
		}
		if i >= len(b.pre) {
			return 1
		}
		pa, pb := a.pre[i], b.pre[i]
		na, errA := strconv.Atoi(pa)
		nb, errB := strconv.Atoi(pb)
		switch {
		case errA == nil && errB == nil:
			if na != nb {
				if na > nb {
					return 1
				}
				return -1
			}
		case errA == nil:
			return -1 // numeric identifiers sort before alphanumeric ones
		case errB == nil:
			return 1
		default:
			if pa != pb {
				if pa > pb {
					return 1
				}
				return -1
			}
		}
	}
	return 0
}
