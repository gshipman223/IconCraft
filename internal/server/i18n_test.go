package server

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/gshipman223/IconCraft/internal/core"
	"github.com/gshipman223/IconCraft/internal/i18n"
	"github.com/gshipman223/IconCraft/internal/native"
)

func TestPageServesCatalogueForDetectedLanguage(t *testing.T) {
	s := testServer(t)
	defer i18n.Set(i18n.English)

	// English default: page keeps lang="en", strings.js carries English.
	w := request(s, "GET", "/"+s.Token+"/", "", "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), `<html lang="en"`) {
		t.Fatal(w.Code, w.Body.String()[:120])
	}
	w = request(s, "GET", "/"+s.Token+"/strings.js", "", "")
	if w.Code != 200 || !strings.HasPrefix(w.Header().Get("Content-Type"), "application/javascript") {
		t.Fatal(w.Code, w.Header())
	}
	if !strings.Contains(w.Body.String(), `"locale":"en"`) || !strings.Contains(w.Body.String(), `"header.settings":"Settings"`) {
		t.Fatal(w.Body.String()[:200])
	}

	// Japanese system language: the same page, now tagged and translated.
	i18n.Set(i18n.Japanese)
	w = request(s, "GET", "/"+s.Token+"/index.html", "", "")
	if !strings.Contains(w.Body.String(), `<html lang="ja"`) {
		t.Fatal("lang attribute not localised")
	}
	w = request(s, "GET", "/"+s.Token+"/strings.js", "", "")
	body := w.Body.String()
	if !strings.Contains(body, `"locale":"ja"`) || !strings.Contains(body, `"header.settings":"設定"`) {
		t.Fatal(body[:200])
	}
	// Static allowlist is unchanged apart from strings.js.
	if w := request(s, "GET", "/"+s.Token+"/locales/ja.json", "", ""); w.Code != 404 {
		t.Fatal("raw catalogue must not be served", w.Code)
	}
}

func TestStringsScriptCarriesRegionFormatLocale(t *testing.T) {
	s := testServer(t)
	defer i18n.SetFormatLocale("")
	i18n.SetFormatLocale("en_AU")
	w := request(s, "GET", "/"+s.Token+"/strings.js", "", "")
	if !strings.Contains(w.Body.String(), `"locale":"en"`) || !strings.Contains(w.Body.String(), `"format":"en-AU"`) {
		t.Fatal(w.Body.String()[:160])
	}
}

type signedNative struct {
	noNative
	sig native.SignatureInfo
}

func (n signedNative) SelfSignature(context.Context) native.SignatureInfo { return n.sig }

func permissionDetail(t *testing.T, s *Server, id string) string {
	t.Helper()
	w := request(s, "GET", "/"+s.Token+"/api/permissions", "", "")
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	var payload struct {
		Checks []struct {
			ID     string `json:"id"`
			Detail string `json:"detail"`
		} `json:"checks"`
		Signature native.SignatureInfo `json:"signature"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	for _, c := range payload.Checks {
		if c.ID == id {
			return c.Detail
		}
	}
	t.Fatalf("no %s check", id)
	return ""
}

func TestPermissionsCopyMatchesActualSignature(t *testing.T) {
	defer i18n.Set(i18n.English)
	make := func(sig native.SignatureInfo) *Server {
		e, err := core.Open(t.TempDir(), t.TempDir(), signedNative{sig: sig})
		if err != nil {
			t.Fatal(err)
		}
		s := New(e)
		s.Host = "127.0.0.1:12345"
		return s
	}
	notarised := make(native.SignatureInfo{Kind: "developer-id", Notarized: true, Team: "ABCDE12345"})
	d := permissionDetail(t, notarised, "appManagement")
	if strings.Contains(d, "ad-hoc") || !strings.Contains(d, "notarised by Apple") {
		t.Fatal("notarised build described as ad-hoc:", d)
	}
	if l := permissionDetail(t, notarised, "launch"); !strings.Contains(l, "no Gatekeeper warning") {
		t.Fatal("launch row:", l)
	}
	devOnly := make(native.SignatureInfo{Kind: "developer-id"})
	if d := permissionDetail(t, devOnly, "appManagement"); strings.Contains(d, "notarised") || !strings.Contains(d, "Developer ID") {
		t.Fatal("developer id (not notarised):", d)
	}
	adhoc := make(native.SignatureInfo{Kind: "adhoc"})
	if d := permissionDetail(t, adhoc, "appManagement"); !strings.Contains(d, "ad-hoc signed") {
		t.Fatal("adhoc:", d)
	}
	// No provider at all (tests, headless Linux): neutral advice, no claim.
	plain := testServer(t)
	if d := permissionDetail(t, plain, "appManagement"); strings.Contains(d, "ad-hoc") || strings.Contains(d, "Developer ID") {
		t.Fatal("unknown signature must not claim either:", d)
	}
	// And the same in another language.
	i18n.Set(i18n.Japanese)
	if d := permissionDetail(t, notarised, "appManagement"); !strings.Contains(d, "公証") || strings.Contains(d, "アドホック") {
		t.Fatal("ja notarised:", d)
	}
}

func TestErrorPayloadsAreTranslatedAtTheBoundary(t *testing.T) {
	s := testServer(t)
	defer i18n.Set(i18n.English)
	post := func() string {
		w := request(s, "POST", "/"+s.Token+"/api/settings", "", `{"settings":{"showDockIcon":false,"showMenuBarIcon":false}}`)
		if w.Code != 400 {
			t.Fatal(w.Code, w.Body.String())
		}
		return w.Body.String()
	}
	if got := post(); !strings.Contains(got, "way back in") {
		t.Fatal("english default:", got)
	}
	i18n.Set(i18n.Korean)
	if got := post(); strings.Contains(got, "way back in") || !strings.Contains(got, "Dock") {
		t.Fatal("korean:", got)
	}
	// Default-app labels are localised in the payload the page receives.
	i18n.Set(i18n.ChineseSimplified)
	if got := i18n.T("Web browser"); got != "网页浏览器" {
		t.Fatal(got)
	}
}
