package core

import "testing"

func TestSettingsDefaultsAndGuards(t *testing.T) {
	e, _, _, _ := fixture(t)
	s := e.Settings()
	if !s.ShowDockIcon || !s.ShowMenuBarIcon || s.LaunchAtLogin {
		t.Fatalf("unexpected defaults: %+v", s)
	}
	// Both ways back in may never be off together.
	bad := s
	bad.ShowDockIcon, bad.ShowMenuBarIcon = false, false
	if _, err := e.SaveSettings(bad); err == nil {
		t.Fatal("hiding both Dock and menu bar must be refused")
	}
	// Starting in the menu bar requires the menu bar icon.
	bad = s
	bad.StartInMenuBar, bad.ShowMenuBarIcon = true, false
	if _, err := e.SaveSettings(bad); err == nil {
		t.Fatal("menu-bar start without menu bar icon must be refused")
	}
	good := s
	good.LaunchAtLogin, good.StartInMenuBar = true, true
	saved, err := e.SaveSettings(good)
	if err != nil || !saved.LaunchAtLogin || e.Settings().StartInMenuBar != true {
		t.Fatalf("save failed: %v %+v", err, saved)
	}
}

func TestSaveSettingsPreservesUpdateCache(t *testing.T) {
	e, _, _, _ := fixture(t)
	e.mu.Lock()
	e.State.Settings.LatestVersion = "9.9.9"
	e.State.Settings.LastUpdateCheck = "2026-10-21T00:00:00Z"
	e.mu.Unlock()
	saved, err := e.SaveSettings(e.Settings())
	if err != nil {
		t.Fatal(err)
	}
	if saved.LatestVersion != "9.9.9" || saved.LastUpdateCheck == "" {
		t.Fatalf("update cache wiped: %+v", saved)
	}
}

func TestNewerVersionIsStrictSemver(t *testing.T) {
	newer := []struct{ latest, current string }{
		{"0.11.0", "0.10.1"}, {"1.0.0", "0.99.9"}, {"0.14.0", "0.13.3"}, {"v0.14.1", "0.14.0"},
		{"0.14.0", "0.14.0-beta.2"}, {"0.14.0-beta.2", "0.14.0-beta.1"}, {"0.14.0-rc.1", "0.14.0-beta.9"},
		{"0.14.0-beta.10", "0.14.0-beta.9"}, {"0.15", "0.14.9"},
	}
	for _, c := range newer {
		if !NewerVersion(c.latest, c.current) {
			t.Errorf("%s should be newer than %s", c.latest, c.current)
		}
	}
	notNewer := []struct{ latest, current string }{
		{"0.13.3", "0.14.0"},         // the round-4 bug: an older release must never be offered
		{"0.14.0", "0.14.0"},         // same version
		{"0.14.0-beta.1", "0.14.0"},  // prerelease of the running release
		{"0.14.0+build.7", "0.14.0"}, // build metadata is ignored
		{"latest", "0.14.0"}, {"", "0.14.0"}, {"0.14.0", "dev"}, {"1.2.3.4", "0.14.0"}, {"0.14.x", "0.14.0"},
	}
	for _, c := range notNewer {
		if NewerVersion(c.latest, c.current) {
			t.Errorf("%s must not be offered over %s", c.latest, c.current)
		}
	}
}
