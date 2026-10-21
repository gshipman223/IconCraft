package server

import (
	"context"
	"strings"
	"testing"

	"github.com/gshipman223/IconCraft/internal/core"
	"github.com/gshipman223/IconCraft/internal/native"
)

// fakeDefaultsNative extends the no-op native stub with a scripted
// LaunchServices so the HTTP contract can be tested anywhere.
type fakeDefaultsNative struct{ noNative }

func (fakeDefaultsNative) DefaultApps(context.Context) ([]native.DefaultCategoryState, error) {
	return []native.DefaultCategoryState{{
		ID: "browser", Label: "Web browser", Detail: "Links", Current: "com.example.alpha",
		Candidates: []native.DefaultCandidate{
			{BundleID: "com.example.alpha", Name: "Alpha", Path: "/Applications/Alpha.app"},
			{BundleID: "com.example.beta", Name: "Beta", Path: "/Applications/Beta.app"},
		},
	}}, nil
}

func (fakeDefaultsNative) SetDefaultApp(_ context.Context, categoryID, bundleID string) (native.DefaultCategoryState, error) {
	if categoryID != "browser" {
		return native.DefaultCategoryState{}, errFakeCategory
	}
	return native.DefaultCategoryState{
		ID: "browser", Label: "Web browser", Detail: "Links", Current: bundleID,
		Candidates: []native.DefaultCandidate{
			{BundleID: "com.example.alpha", Name: "Alpha", Path: "/Applications/Alpha.app"},
			{BundleID: "com.example.beta", Name: "Beta", Path: "/Applications/Beta.app"},
		},
	}, nil
}

var errFakeCategory = &fakeError{"unknown default-app category"}

type fakeError struct{ msg string }

func (e *fakeError) Error() string { return e.msg }

func TestDefaultsEndpoints(t *testing.T) {
	e, err := core.Open(t.TempDir(), t.TempDir(), fakeDefaultsNative{})
	if err != nil {
		t.Fatal(err)
	}
	s := New(e)
	s.Host = "127.0.0.1:12345"

	w := request(s, "GET", "/"+s.Token+"/api/defaults", "", "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"current":{"appId":""`) && !strings.Contains(w.Body.String(), `"bundleId":"com.example.alpha"`) {
		t.Fatalf("defaults read: %d %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"label":"Web browser"`) || !strings.Contains(w.Body.String(), `"name":"Beta"`) {
		t.Fatalf("defaults payload missing candidates: %s", w.Body.String())
	}

	w = request(s, "POST", "/"+s.Token+"/api/defaults/set", "", `{"category":"browser","bundleId":"com.example.beta"}`)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"current":{"appId":"","bundleId":"com.example.beta"`) {
		t.Fatalf("defaults set: %d %s", w.Code, w.Body.String())
	}

	w = request(s, "POST", "/"+s.Token+"/api/defaults/set", "", `{"category":"nope","bundleId":"com.example.beta"}`)
	if w.Code == 200 || !strings.Contains(w.Body.String(), "unknown default-app category") {
		t.Fatalf("unknown category must fail honestly: %d %s", w.Code, w.Body.String())
	}
}

func TestDefaultsRequireNativeLayer(t *testing.T) {
	s := testServer(t) // noNative{} has no DefaultApps
	w := request(s, "GET", "/"+s.Token+"/api/defaults", "", "")
	if w.Code == 200 || !strings.Contains(w.Body.String(), "macOS native layer") {
		t.Fatalf("expected native-layer refusal: %d %s", w.Code, w.Body.String())
	}
}

func TestOrganizeEndpoint(t *testing.T) {
	s := testServer(t)
	w := request(s, "POST", "/"+s.Token+"/api/organize", "", `{"sort":"bogus","group":"status"}`)
	if w.Code == 200 {
		t.Fatalf("invalid sort must fail: %d %s", w.Code, w.Body.String())
	}
	w = request(s, "POST", "/"+s.Token+"/api/organize", "", `{"sort":"name-desc","group":"status"}`)
	if w.Code != 200 {
		t.Fatalf("valid organize save: %d %s", w.Code, w.Body.String())
	}
	if got := s.Engine.Organize(); got.Sort != "name-desc" || got.Group != "status" {
		t.Fatalf("organize not journalled: %+v", got)
	}
	w = request(s, "GET", "/"+s.Token+"/api/session", "", "")
	if !strings.Contains(w.Body.String(), `"organize":{"sort":"name-desc","group":"status"}`) {
		t.Fatalf("session must carry organize prefs: %s", w.Body.String())
	}
}
