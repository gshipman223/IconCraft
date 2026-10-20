package server

import (
	"strings"
	"testing"
)

func TestSettingsEndpointGuardsAndSaves(t *testing.T) {
	s := testServer(t)
	// Session carries effective settings for the window host to apply.
	w := request(s, "GET", "/"+s.Token+"/api/session", "", "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"showDockIcon":true`) || !strings.Contains(w.Body.String(), `"showMenuBarIcon":true`) {
		t.Fatal(w.Code, w.Body.String())
	}
	// Hiding both ways back in is refused with a clear message.
	w = request(s, "POST", "/"+s.Token+"/api/settings", "", `{"settings":{"showDockIcon":false,"showMenuBarIcon":false}}`)
	if w.Code == 200 || !strings.Contains(w.Body.String(), "way back in") {
		t.Fatal(w.Code, w.Body.String())
	}
	// A valid save round-trips.
	w = request(s, "POST", "/"+s.Token+"/api/settings", "", `{"settings":{"launchAtLogin":true,"startInMenuBar":true,"showDockIcon":false,"showMenuBarIcon":true,"notifyAttention":true,"autoCheckUpdates":true}}`)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"launchAtLogin":true`) || !strings.Contains(w.Body.String(), `"showDockIcon":false`) {
		t.Fatal(w.Code, w.Body.String())
	}
	if got := s.Engine.Settings(); !got.LaunchAtLogin || got.ShowDockIcon {
		t.Fatalf("settings not journalled: %+v", got)
	}
}
