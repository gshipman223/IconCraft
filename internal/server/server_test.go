package server

import (
	"bytes"
	"context"
	"github.com/gshipman223/IconCraft/internal/core"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type noNative struct{}

func (noNative) Inspect(context.Context, []string) ([]core.App, error)    { return []core.App{}, nil }
func (noNative) Snapshot(string) (core.Snapshot, error)                   { return core.Snapshot{}, nil }
func (noNative) Set(context.Context, string, string) error                { return nil }
func (noNative) Restore(context.Context, string, core.Snapshot) error     { return nil }
func (noNative) ExportIcon(context.Context, string, bool) ([]byte, error) { return nil, nil }
func (noNative) Normalize(context.Context, string) ([]byte, error)        { return nil, nil }
func (noNative) Signature(context.Context, string) (string, error)        { return "valid", nil }
func (noNative) Choose(context.Context, string) (string, error)           { return "", nil }
func testServer(t *testing.T) *Server {
	e, err := core.Open(t.TempDir(), t.TempDir(), noNative{})
	if err != nil {
		t.Fatal(err)
	}
	s := New(e)
	s.Host = "127.0.0.1:12345"
	return s
}
func request(s *Server, method, path, origin, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, "http://"+s.Host+path, bytes.NewBufferString(body))
	r.Host = s.Host
	r.Header.Set("Content-Type", "application/json")
	if origin != "" {
		r.Header.Set("Origin", origin)
	}
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	return w
}
func TestSessionTokenRequired(t *testing.T) {
	s := testServer(t)
	for _, p := range []string{"/", "/api/status", "/wrong/api/status"} {
		w := request(s, "GET", p, "", "")
		if w.Code != 404 {
			t.Fatal(w.Code)
		}
	}
}
func TestRejectsForeignOrigin(t *testing.T) {
	s := testServer(t)
	w := request(s, "GET", "/"+s.Token+"/api/status", "https://attacker.example", "")
	if w.Code != 403 {
		t.Fatal(w.Code)
	}
}
func TestRejectsRebindingHost(t *testing.T) {
	s := testServer(t)
	r := httptest.NewRequest("GET", "http://attacker.example/"+s.Token+"/api/status", nil)
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatal(w.Code)
	}
}
func TestStatusAndSecurityHeaders(t *testing.T) {
	s := testServer(t)
	w := request(s, "GET", "/"+s.Token+"/api/status", "", "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"testing":false`) {
		t.Fatal(w.Code, w.Body.String())
	}
	if w.Header().Get("X-Frame-Options") != "DENY" || !strings.Contains(w.Header().Get("Content-Security-Policy"), "frame-ancestors 'none'") {
		t.Fatal(w.Header())
	}
}
func TestMutationsRequirePOST(t *testing.T) {
	s := testServer(t)
	for _, p := range []string{"apply", "restore", "import", "root", "profile-import"} {
		w := request(s, "GET", "/"+s.Token+"/api/"+p, "", "")
		if w.Code != http.StatusMethodNotAllowed {
			t.Fatal(p, w.Code)
		}
	}
}
func TestUnknownApplicationRejectedWithoutDeveloperGate(t *testing.T) {
	s := testServer(t)
	w := request(s, "POST", "/"+s.Token+"/api/apply", "", `{"appId":"x","assetId":"y","confirmed":true}`)
	if w.Code != 400 || !strings.Contains(w.Body.String(), "application not found") {
		t.Fatal(w.Code, w.Body.String())
	}
}
func TestStaticFileAllowlist(t *testing.T) {
	s := testServer(t)
	for _, p := range []string{"../../etc/passwd", "state.json", "bridge.js", "../ui/index.html"} {
		w := request(s, "GET", "/"+s.Token+"/"+p, "", "")
		if w.Code != 404 {
			t.Fatal(p, w.Code)
		}
	}
	for _, p := range []string{"", "app.js", "style.css"} {
		w := request(s, "GET", "/"+s.Token+"/"+p, "", "")
		if w.Code != 200 {
			t.Fatal(p, w.Code)
		}
	}
}
func TestRestoreRequiresConfirmation(t *testing.T) {
	s := testServer(t)
	w := request(s, "POST", "/"+s.Token+"/api/restore", "", `{"recordId":"x","confirmed":false}`)
	if w.Code != 400 || !strings.Contains(w.Body.String(), "confirmation") {
		t.Fatal(w.Code, w.Body.String())
	}
}
func TestUnknownMutationFieldsRejected(t *testing.T) {
	s := testServer(t)
	w := request(s, "POST", "/"+s.Token+"/api/donor", "", `{"appId":"x","path":"/etc/passwd"}`)
	if w.Code != 400 || !strings.Contains(w.Body.String(), "unknown field") {
		t.Fatal(w.Code, w.Body.String())
	}
}
func TestUnknownIconAssetRejected(t *testing.T) {
	s := testServer(t)
	w := request(s, "GET", "/"+s.Token+"/api/icon?asset=../../etc/passwd", "", "")
	if w.Code != 400 {
		t.Fatal(w.Code)
	}
}
func TestNativeCancellationDoesNotWrite(t *testing.T) {
	s := testServer(t)
	for _, p := range []string{"import", "root"} {
		w := request(s, "POST", "/"+s.Token+"/api/"+p, "", `{}`)
		if w.Code != 200 || !strings.Contains(w.Body.String(), "cancelled") {
			t.Fatal(p, w.Code, w.Body.String())
		}
	}
}
func TestBodySizeLimited(t *testing.T) {
	s := testServer(t)
	w := request(s, "POST", "/"+s.Token+"/api/donor", "", strings.Repeat("x", 2<<20))
	if w.Code != 400 {
		t.Fatal(w.Code)
	}
}
func TestPreviewCacheBoundedAndCleared(t *testing.T) {
	s := testServer(t)
	for i := 0; i < 80; i++ {
		s.cacheIcon(strings.Repeat("x", i+1), make([]byte, 1<<20))
	}
	if s.iconBytes > 32<<20 || len(s.iconCache) > 64 {
		t.Fatal("cache unbounded")
	}
	s.clearIcons()
	if s.iconBytes != 0 || len(s.iconCache) != 0 {
		t.Fatal("cache not cleared")
	}
}
func TestProfilePickerCancellationDoesNotChangeState(t *testing.T) {
	s := testServer(t)
	w := request(s, "POST", "/"+s.Token+"/api/profile-pick", "", `{}`)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "cancelled") {
		t.Fatal(w.Code, w.Body.String())
	}
}
