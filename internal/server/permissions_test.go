package server

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/gshipman223/IconCraft/internal/core"
)

type permissionNative struct {
	noNative
	probeErr error
}

func (n permissionNative) WriteProbe(string) error { return n.probeErr }

func TestPermissionsStatusDoesNotInventAppManagementGrant(t *testing.T) {
	s := testServer(t)
	w := request(s, "GET", "/"+s.Token+"/api/permissions", "", "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"required":true`) || !strings.Contains(w.Body.String(), `"state":"unknown"`) || !strings.Contains(w.Body.String(), `"label":"Not checked yet"`) || !strings.Contains(w.Body.String(), `"state":"not-required"`) {
		t.Fatal(w.Code, w.Body.String())
	}
	w = request(s, "GET", "/"+s.Token+"/api/permissions/check", "", "")
	if w.Code != 405 {
		t.Fatal("permission checks must be explicit POSTs", w.Code)
	}
	w = request(s, "GET", "/"+s.Token+"/api/setup/complete", "", "")
	if w.Code != 405 {
		t.Fatal("setup completion must be a POST", w.Code)
	}
}

func TestPermissionCheckAndSetupCompletion(t *testing.T) {
	s := testServer(t)
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	appPath := filepath.Join(root, "PhotoCraft.app")
	if err := os.MkdirAll(appPath, 0700); err != nil {
		t.Fatal(err)
	}
	s.Engine.Native = permissionNative{}
	s.Engine.Apps["fixture"] = core.App{ID: "fixture", Name: "PhotoCraft", Path: appPath, BundleID: "ai.storyteller.photocraft", Executable: "photocraft"}

	w := request(s, "POST", "/"+s.Token+"/api/permissions/check", "", `{"appId":"fixture"}`)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"label":"Granted — tested"`) || !strings.Contains(w.Body.String(), `"status":"ready"`) {
		t.Fatal(w.Code, w.Body.String())
	}
	if got := s.Engine.SnapshotState().AppAccess.Status; got != "ready" {
		t.Fatalf("successful permission probe was not recorded: %q", got)
	}

	s.Engine.Native = permissionNative{probeErr: &os.PathError{Op: "open", Path: appPath, Err: syscall.EPERM}}
	w = request(s, "POST", "/"+s.Token+"/api/permissions/check", "", `{"appId":"fixture"}`)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	state, title, detail := appManagementCheck(t, w.Body.String())
	if state != "blocked" || title != "App Management — changing other apps" || !strings.Contains(detail, "operation not permitted") || !strings.Contains(detail, "already on") || !strings.Contains(detail, "App Management") {
		t.Fatal(state, title, detail)
	}

	w = request(s, "POST", "/"+s.Token+"/api/setup/complete", "", `{"confirmed":false}`)
	if w.Code != 400 || !strings.Contains(w.Body.String(), "confirmation") {
		t.Fatal(w.Code, w.Body.String())
	}
	w = request(s, "POST", "/"+s.Token+"/api/setup/complete", "", `{"confirmed":true}`)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"setupRequired":false`) {
		t.Fatal(w.Code, w.Body.String())
	}
	w = request(s, "GET", "/"+s.Token+"/api/session", "", "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"setupRequired":false`) {
		t.Fatal(w.Code, w.Body.String())
	}
}

func TestPermissionCheckDoesNotCallEACCESAppManagement(t *testing.T) {
	s := testServer(t)
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	appPath := filepath.Join(root, "PhotoCraft.app")
	if err := os.MkdirAll(appPath, 0700); err != nil {
		t.Fatal(err)
	}
	s.Engine.Apps["fixture"] = core.App{ID: "fixture", Name: "PhotoCraft", Path: appPath, BundleID: "ai.storyteller.photocraft", Executable: "photocraft"}
	s.Engine.Native = permissionNative{probeErr: &os.PathError{Op: "open", Path: appPath, Err: syscall.EACCES}}
	w := request(s, "POST", "/"+s.Token+"/api/permissions/check", "", `{"appId":"fixture"}`)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	state, title, detail := appManagementCheck(t, w.Body.String())
	if state != "unavailable" || title != "Write access — changing other apps" || !strings.Contains(detail, "permission denied") {
		t.Fatal(state, title, detail)
	}
	if strings.Contains(detail, "App Management") || strings.Contains(detail, "operation not permitted") || strings.Contains(detail, "switch") {
		t.Fatal("EACCES was described as App Management:", detail)
	}

	s.Engine.Native = permissionNative{probeErr: &os.PathError{Op: "open", Path: appPath, Err: syscall.EROFS}}
	w = request(s, "POST", "/"+s.Token+"/api/permissions/check", "", `{"appId":"fixture"}`)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	state, title, detail = appManagementCheck(t, w.Body.String())
	if state != "unavailable" || title != "Write access — changing other apps" || !strings.Contains(detail, "read-only") || strings.Contains(detail, "App Management") {
		t.Fatal(state, title, detail)
	}
}

func appManagementCheck(t *testing.T, body string) (state, title, detail string) {
	t.Helper()
	var payload struct {
		Checks []struct {
			ID     string `json:"id"`
			State  string `json:"state"`
			Title  string `json:"title"`
			Detail string `json:"detail"`
		} `json:"checks"`
	}
	if err := json.Unmarshal([]byte(body), &payload); err != nil {
		t.Fatal(err)
	}
	for _, c := range payload.Checks {
		if c.ID == "appManagement" {
			return c.State, c.Title, c.Detail
		}
	}
	t.Fatal("no appManagement check", body)
	return "", "", ""
}
