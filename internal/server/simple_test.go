package server

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"github.com/gshipman223/IconCraft/internal/core"
	"github.com/gshipman223/IconCraft/internal/originals"
	"image"
	"image/png"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type originalTransport func(*http.Request) (*http.Response, error)

func (f originalTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func originalPNG() []byte {
	var b bytes.Buffer
	_ = png.Encode(&b, image.NewRGBA(image.Rect(0, 0, 32, 32)))
	return b.Bytes()
}
func seedOriginalApp(s *Server) core.App {
	a := core.App{ID: "fixture", Name: "PhotoCraft", BundleID: "ai.storyteller.photocraft", CraftID: "photocraft", Path: "/Applications/PhotoCraft.app"}
	s.Engine.Apps[a.ID] = a
	return a
}
func TestSimpleMutationRoutesRequirePOST(t *testing.T) {
	s := testServer(t)
	for _, p := range []string{"original", "original/commit", "custom", "choice", "restore-app", "restore-all"} {
		w := request(s, "GET", "/"+s.Token+"/api/"+p, "", "")
		if w.Code != 405 {
			t.Fatal(p, w.Code)
		}
	}
}
func TestSimpleRestoresStillRequireExplicitRequest(t *testing.T) {
	s := testServer(t)
	for _, p := range []string{"restore-app", "restore-all"} {
		w := request(s, "POST", "/"+s.Token+"/api/"+p, "", `{"confirmed":false}`)
		if w.Code != 400 || !strings.Contains(w.Body.String(), "confirmation") {
			t.Fatal(w.Code, w.Body.String())
		}
	}
}
func TestOriginalPreviewDownloadDoesNotChangeApp(t *testing.T) {
	s := testServer(t)
	seedOriginalApp(s)
	s.Resolver.Client.Transport = originalTransport(func(r *http.Request) (*http.Response, error) {
		body := originalPNG()
		if strings.HasSuffix(r.URL.Path, ".html") {
			body = []byte(`<img src="/photoshop-product-icon.png" alt="Photoshop product icon">`)
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(bytes.NewReader(body))}, nil
	})
	w := request(s, "POST", "/"+s.Token+"/api/original", "", `{"appId":"fixture"}`)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"status":"fresh"`) {
		t.Fatal(w.Code, w.Body.String())
	}
	st := s.Engine.SnapshotState()
	if len(st.Records) != 0 || len(st.Originals) != 1 {
		t.Fatal("not a preview", st)
	}
}
func TestOriginalCachedOfflineFallbackIsExplicit(t *testing.T) {
	s := testServer(t)
	a := seedOriginalApp(s)
	asset, e := s.Engine.ImportPNG("Photoshop", "fixture", originalPNG())
	if e != nil {
		t.Fatal(e)
	}
	if e = s.Engine.RememberOriginal(a.CraftID, "Adobe Photoshop", "https://www.adobe.com/ps.png", asset.ID); e != nil {
		t.Fatal(e)
	}
	s.Resolver.Client.Transport = originalTransport(func(r *http.Request) (*http.Response, error) { return nil, context.DeadlineExceeded })
	w := request(s, "POST", "/"+s.Token+"/api/original", "", `{"appId":"fixture"}`)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"status":"cached"`) {
		t.Fatal(w.Code, w.Body.String())
	}
}
func TestOriginalRenderingTicketIsSingleUse(t *testing.T) {
	s := testServer(t)
	a := seedOriginalApp(s)
	p, _ := originals.ProductFor(a.CraftID)
	ticket := s.ticket(a, originals.Candidate{Product: p, Source: p.Direct[0], MIME: "image/svg+xml"})
	body, _ := json.Marshal(map[string]string{"ticket": ticket, "png": "data:image/png;base64," + base64.StdEncoding.EncodeToString(originalPNG())})
	for attempt := 0; attempt < 2; attempt++ {
		w := request(s, "POST", "/"+s.Token+"/api/original/commit", "", string(body))
		want := 200
		if attempt > 0 {
			want = 400
		}
		if w.Code != want {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	if len(s.Engine.SnapshotState().Records) != 0 {
		t.Fatal("render applied to app")
	}
}
func TestExpiredAndForeignTicketsCannotImport(t *testing.T) {
	s := testServer(t)
	s.pendingIcons["expired"] = pendingIcon{Expires: time.Now().Add(-time.Minute)}
	for _, ticket := range []string{"expired", "invented"} {
		body, _ := json.Marshal(map[string]string{"ticket": ticket, "png": "data:image/png;base64," + base64.StdEncoding.EncodeToString(originalPNG())})
		w := request(s, "POST", "/"+s.Token+"/api/original/commit", "", string(body))
		if w.Code != 400 {
			t.Fatal(w.Code)
		}
	}
	if len(s.Engine.SnapshotState().Assets) != 0 {
		t.Fatal("invalid ticket imported")
	}
}
func TestCustomPickerCancellationLeavesOriginalChoice(t *testing.T) {
	s := testServer(t)
	a := seedOriginalApp(s)
	asset, e := s.Engine.ImportPNG("custom", "fixture", originalPNG())
	if e != nil {
		t.Fatal(e)
	}
	if e = s.Engine.RememberChoice(a.ID, asset.ID); e != nil {
		t.Fatal(e)
	}
	w := request(s, "POST", "/"+s.Token+"/api/custom", "", `{"appId":"fixture"}`)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "cancelled") {
		t.Fatal(w.Code, w.Body.String())
	}
	if s.Engine.SnapshotState().Choices[a.BundleID] != asset.ID {
		t.Fatal("cancel cleared choice")
	}
}
func TestAutomaticSourceRejectsArbitraryURLField(t *testing.T) {
	s := testServer(t)
	seedOriginalApp(s)
	w := request(s, "POST", "/"+s.Token+"/api/original", "", `{"appId":"fixture","url":"http://localhost"}`)
	if w.Code != 400 || !strings.Contains(w.Body.String(), "unknown field") {
		t.Fatal(w.Code, w.Body.String())
	}
}
func TestSimpleRestoreAllWithNothingIsHarmless(t *testing.T) {
	s := testServer(t)
	w := request(s, "POST", "/"+s.Token+"/api/restore-all", "", `{"confirmed":true}`)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"restored":0`) {
		t.Fatal(w.Code, w.Body.String())
	}
}

func TestSupportEndpointOnlyOpensAllowlistedTargets(t *testing.T) {
	s := testServer(t)
	w := request(s, "POST", "/"+s.Token+"/api/support", "", `{"target":"paypal"}`)
	if w.Code != 200 || !strings.Contains(w.Body.String(), core.SupportURL) {
		t.Fatal(w.Code, w.Body.String())
	}
	w = request(s, "POST", "/"+s.Token+"/api/support", "", `{"target":"github"}`)
	if w.Code != 200 || !strings.Contains(w.Body.String(), core.RepositoryURL) {
		t.Fatal(w.Code, w.Body.String())
	}
	for _, body := range []string{`{"target":"evil"}`, `{"target":"paypal","url":"https://example.com"}`} {
		w = request(s, "POST", "/"+s.Token+"/api/support", "", body)
		if w.Code != 400 {
			t.Fatal(body, w.Code, w.Body.String())
		}
	}
	w = request(s, "GET", "/"+s.Token+"/api/support", "", "")
	if w.Code != 405 {
		t.Fatal("support must be POST", w.Code)
	}
}

func TestLogoIsServed(t *testing.T) {
	s := testServer(t)
	w := request(s, "GET", "/"+s.Token+"/logo.svg", "", "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), "<svg") || !strings.Contains(w.Header().Get("Content-Type"), "image/svg+xml") {
		t.Fatal(w.Code, w.Header().Get("Content-Type"))
	}
}
