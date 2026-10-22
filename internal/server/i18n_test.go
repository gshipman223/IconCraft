package server

import (
	"strings"
	"testing"

	"github.com/gshipman223/IconCraft/internal/i18n"
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
