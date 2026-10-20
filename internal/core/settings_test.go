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

func TestCompareVersions(t *testing.T) {
	if compareVersions("0.11.0", "0.10.1") <= 0 || compareVersions("1.0.0", "0.99.9") <= 0 {
		t.Fatal("newer versions must compare greater")
	}
	if compareVersions("0.11.0", "0.11.0") != 0 || compareVersions("0.9.0", "0.11.0") >= 0 {
		t.Fatal("equal/older versions mis-compared")
	}
}
