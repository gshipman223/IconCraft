package server

import (
	"os"
	"path/filepath"
	"strings"
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

	s.Engine.Native = permissionNative{probeErr: os.ErrPermission}
	w = request(s, "POST", "/"+s.Token+"/api/permissions/check", "", `{"appId":"fixture"}`)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"state":"blocked"`) {
		t.Fatal(w.Code, w.Body.String())
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
