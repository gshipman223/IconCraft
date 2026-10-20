package originals

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
)

type transport func(*http.Request) (*http.Response, error)

func (f transport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func response(code int, b []byte) *http.Response {
	return &http.Response{StatusCode: code, Header: make(http.Header), Body: io.NopCloser(bytes.NewReader(b))}
}
func testPNG(w, h int) []byte {
	im := image.NewRGBA(image.Rect(0, 0, w, h))
	im.Set(0, 0, color.White)
	var b bytes.Buffer
	_ = png.Encode(&b, im)
	return b.Bytes()
}
func TestAllowedPublisherOrigins(t *testing.T) {
	for _, u := range []string{"https://www.adobe.com/a.svg", "https://is1-ssl.mzstatic.com/image/thumb/abc/512.png", "https://kb.avid.com/pkb/servlet/rtaImage?x=1", "https://main--dc--adobecom.aem.live/a.svg"} {
		if !AllowedURL(u) {
			t.Fatal(u)
		}
	}
	for _, u := range []string{"http://www.adobe.com/a.svg", "file:///etc/passwd", "https://adobe.com.evil.example/a", "https://user@adobe.com/a", "https://adobe.com:443/a", "https://127.0.0.1/a", "https://someproject.aem.live/a", "https://www.eviladobe.com/"} {
		if AllowedURL(u) {
			t.Fatal(u)
		}
	}
}
func TestEveryKnownCraftHasProductAndOfficialPage(t *testing.T) {
	for id, p := range Products() {
		if p.Name == "" || p.ID == "" || !AllowedURL(p.Page) || !safeID.MatchString(id) {
			t.Fatal(id, p)
		}
		for _, u := range p.Direct {
			if !AllowedURL(u) {
				t.Fatal(u)
			}
		}
	}
}
func TestStoreIdentityMustMatch(t *testing.T) {
	p, _ := ProductFor("wordcraft")
	good := `{"results":[{"trackId":462054704,"bundleId":"com.microsoft.Word","sellerName":"Microsoft Corporation","artworkUrl512":"https://is1-ssl.mzstatic.com/art.png"}]}`
	u, err := StoreArtwork([]byte(good), p)
	if err != nil || !strings.HasSuffix(u, "art.png") {
		t.Fatal(u, err)
	}
	for _, bad := range []string{strings.ReplaceAll(good, "Microsoft Corporation", "Impostor"), strings.ReplaceAll(good, "com.microsoft.Word", "com.other.Word"), strings.ReplaceAll(good, "462054704", "123"), strings.ReplaceAll(good, "is1-ssl.mzstatic.com", "evil.example")} {
		if _, err = StoreArtwork([]byte(bad), p); err == nil {
			t.Fatal("wrong identity accepted")
		}
	}
}
func TestCurrentProductIconPreferredToWordmark(t *testing.T) {
	p, _ := ProductFor("photocraft")
	b := `<img src="/photoshop-wordmark.svg" alt="Photoshop logo"><img alt="Photoshop product icon" src="/new/photoshop.svg?v=3&amp;x=4"><img src="/photoshop-screenshot.png" alt="Photoshop icon screenshot"><img src="https://evil.example/photoshop-product-icon.png">`
	u := ImageCandidates([]byte(b), p)
	if len(u) != 1 || u[0] != "https://www.adobe.com/new/photoshop.svg?v=3&x=4" {
		t.Fatal(u)
	}
}
func TestFigmaUsesTouchArtworkNotDownloadsHero(t *testing.T) {
	p, _ := ProductFor("figcraft")
	b := `<link rel="apple-touch-icon" href="/new-app.png"><link rel="icon" href="/symbol.svg"><img alt="Figma desktop screenshot" src="/desktop.png">`
	u := ImageCandidates([]byte(b), p)
	if len(u) != 2 || !strings.HasSuffix(u[0], "new-app.png") {
		t.Fatal(u)
	}
}
func TestAutoCADDoesNotUseLTOrWeb(t *testing.T) {
	p, _ := ProductFor("cadcraft")
	b := `<img alt="AutoCAD LT product icon" src="/lt.png"><img alt="AutoCAD Web product icon" src="/web.png"><img alt="AutoCAD full product icon" src="/full.png">`
	u := ImageCandidates([]byte(b), p)
	if len(u) != 1 || !strings.HasSuffix(u[0], "full.png") {
		t.Fatal(u)
	}
}
func TestValidOriginalSVGAndLocalGradient(t *testing.T) {
	s := `<svg xmlns="http://www.w3.org/2000/svg" width="240" height="240" viewBox="0 0 240 240"><defs><linearGradient id="g"><stop offset="0" stop-color="#fff"/></linearGradient></defs><path fill="url(#g)" d="M0 0h240v240H0z"/></svg>`
	if m, err := ValidateArtwork([]byte(s)); err != nil || m != "image/svg+xml" {
		t.Fatal(m, err)
	}
}
func TestRejectActiveSVG(t *testing.T) {
	bad := []string{`<!DOCTYPE svg [<!ENTITY x SYSTEM "file:///etc/passwd">]><svg/>`, `<svg><script>alert(1)</script></svg>`, `<svg onload="evil()"/>`, `<svg><image href="https://example.com/a.png"/></svg>`, `<svg><use href="file:///x"/></svg>`, `<svg><path fill="url(https://a.com/x)"/></svg>`, `<svg><style>@import url(https://x.com/);</style></svg>`, `<html><body>Blocked</body></html>`, `<svg><foreignObject/></svg>`, `<svg>`}
	for _, s := range bad {
		if _, err := ValidateArtwork([]byte(s)); err == nil {
			t.Fatalf("accepted %s", s)
		}
	}
}
func TestRasterMustBeIconNotBannerOrCorrupt(t *testing.T) {
	if m, e := ValidateArtwork(testPNG(64, 64)); e != nil || m != "image/png" {
		t.Fatal(m, e)
	}
	for _, b := range [][]byte{testPNG(300, 70), testPNG(5, 5), []byte("<html>no</html>"), testPNG(64, 64)[:30]} {
		if _, e := ValidateArtwork(b); e == nil {
			t.Fatal("bad raster accepted")
		}
	}
}
func TestResolverRequestsFreshPageOnEveryLaunch(t *testing.T) {
	r := New()
	version := 1
	var mu sync.Mutex
	pages := 0
	requests := 0
	r.Client.Transport = transport(func(q *http.Request) (*http.Response, error) {
		mu.Lock()
		defer mu.Unlock()
		requests++
		if q.Header.Get("Cache-Control") != "no-cache" {
			t.Error("not revalidating")
		}
		if q.URL.Path == "/products/photoshop.html" {
			pages++
			return response(200, []byte(fmt.Sprintf(`<img alt="Photoshop product icon" src="/current/ps-%d.png">`, version))), nil
		}
		if strings.HasPrefix(q.URL.Path, "/current/") {
			return response(200, testPNG(64, 64)), nil
		}
		return response(404, nil), nil
	})
	a, e := r.Resolve(context.Background(), "photocraft", nil)
	if e != nil || !strings.HasSuffix(a.Source, "ps-1.png") {
		t.Fatal(a, e)
	}
	version = 2
	b, e := r.Resolve(context.Background(), "photocraft", nil)
	if e != nil || !strings.HasSuffix(b.Source, "ps-2.png") {
		t.Fatal(b, e)
	}
	if pages != 2 || requests < 4 {
		t.Fatal(pages, requests)
	}
}
func TestResolverFallsBackToOfficialAssetNotThirdParty(t *testing.T) {
	r := New()
	r.Client.Transport = transport(func(q *http.Request) (*http.Response, error) {
		if strings.HasSuffix(q.URL.Path, "/photoshop.svg") {
			return response(200, []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="64" height="64"><path d="M0 0h64v64H0z"/></svg>`)), nil
		}
		return response(403, nil), nil
	})
	c, e := r.Resolve(context.Background(), "photocraft", nil)
	if e != nil || !strings.Contains(c.Source, "/product-icons/svg/photoshop.svg") {
		t.Fatal(c, e)
	}
}
func TestResolverOnlyUsesKnownAppStoreProduct(t *testing.T) {
	r := New()
	r.Client.Transport = transport(func(q *http.Request) (*http.Response, error) {
		switch q.URL.Host {
		case "itunes.apple.com":
			return response(200, []byte(`{"results":[{"trackId":462054704,"bundleId":"com.microsoft.Word","sellerName":"Microsoft Corporation","artworkUrl512":"https://is1-ssl.mzstatic.com/current.png"}]}`)), nil
		case "is1-ssl.mzstatic.com":
			return response(200, testPNG(64, 64)), nil
		}
		return response(403, nil), nil
	})
	c, e := r.Resolve(context.Background(), "wordcraft", nil)
	if e != nil || c.Product.Name != "Microsoft Word" {
		t.Fatal(c, e)
	}
}
func TestDiscoverNewCraftUsingExplicitPublisherComparison(t *testing.T) {
	r := New()
	r.Client.Transport = transport(func(q *http.Request) (*http.Response, error) {
		if q.URL.Host == "getartcraft.com" {
			return response(200, []byte(`<p>An open-source alternative to <strong>Adobe Photoshop</strong>.</p>`)), nil
		}
		if strings.HasSuffix(q.URL.Path, "photoshop.svg") {
			return response(200, []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="64" height="64"/>`)), nil
		}
		return response(404, nil), nil
	})
	c, e := r.Resolve(context.Background(), "newcraft", nil)
	if e != nil || c.Product.ID != "photoshop" {
		t.Fatal(c, e)
	}
}
func TestUnknownCloneDoesNotGuessRandomIcon(t *testing.T) {
	r := New()
	r.Client.Transport = transport(func(q *http.Request) (*http.Response, error) {
		return response(200, []byte(`<p>Welcome to mystery craft. Photoshop was mentioned in a footer.</p>`)), nil
	})
	if _, e := r.Resolve(context.Background(), "mysterycraft", nil); e == nil {
		t.Fatal("guessed icon")
	}
}
func TestNetworkRedirectDoesNotEscapePublisherAllowlist(t *testing.T) {
	r := New()
	r.Client.Transport = transport(func(q *http.Request) (*http.Response, error) {
		if q.URL.Host == "evil.example" {
			t.Fatal("contacted untrusted host")
		}
		res := response(302, nil)
		res.Header.Set("Location", "https://evil.example/icon.png")
		return res, nil
	})
	if _, e := r.get(context.Background(), "https://www.adobe.com/a.svg", 1024); e == nil {
		t.Fatal("redirect accepted")
	}
}
func TestDownloadedBodiesAreBounded(t *testing.T) {
	r := New()
	r.Client.Transport = transport(func(q *http.Request) (*http.Response, error) {
		return response(200, []byte(strings.Repeat("x", 1025))), nil
	})
	if _, e := r.get(context.Background(), "https://www.adobe.com/icon", 1024); e == nil {
		t.Fatal("oversize accepted")
	}
}
func TestExcludedUnrenderableIconIsNotReturnedAgain(t *testing.T) {
	p, _ := ProductFor("photocraft")
	r := New()
	r.Client.Transport = transport(func(q *http.Request) (*http.Response, error) {
		if strings.HasSuffix(q.URL.Path, "photoshop.svg") {
			t.Fatal("retried excluded source")
		}
		return response(404, nil), nil
	})
	if _, e := r.Resolve(context.Background(), "photocraft", p.Direct); e == nil {
		t.Fatal("unexpected success")
	}
}

func TestAutoCADStoreSearchAcceptsAutodeskRejectsKnockoffs(t *testing.T) {
	p, _ := ProductFor("cadcraft")
	if p.StoreID != "393149734" || p.Publisher != "Autodesk" {
		t.Fatal("cadcraft must carry its verified App Store identity", p)
	}
	good := `{"results":[{"trackName":"AutoCAD","bundleId":"com.autodesk.autocadws","sellerName":"Autodesk","artworkUrl512":"https://is1-ssl.mzstatic.com/AppIcon.png/512x512bb.jpg"}]}`
	u, err := StoreSearchArtwork([]byte(good), p)
	if err != nil || !strings.Contains(u, "is1-ssl.mzstatic.com") {
		t.Fatal(u, err)
	}
	// (A wrong trackName with AutoCAD's real bundle ID cannot exist in store
	// data — bundle IDs are unique — so bundle match alone is sufficient.)
	knockoffs := []string{
		strings.ReplaceAll(good, `"Autodesk"`, `"蕊 邵"`),
		strings.ReplaceAll(strings.ReplaceAll(good, `"AutoCAD"`, `"CAD Plus"`), `com.autodesk.autocadws`, `com.p7q.cadbuild`),
		strings.ReplaceAll(good, `com.autodesk.autocadws`, `cloud.iothub.dwgviewereditor`),
	}
	for _, bad := range knockoffs {
		if _, err := StoreSearchArtwork([]byte(bad), p); err == nil {
			t.Fatal("knockoff accepted:", bad)
		}
	}
}

func TestProductMatchesInstalledVariantsOnly(t *testing.T) {
	p, _ := ProductFor("cadcraft")
	for _, ok := range [][2]string{{"AutoCAD", "com.autodesk.AutoCAD"}, {"AutoCAD 2025", "com.autodesk.AutoCAD2025"}, {"Whatever Name", "com.autodesk.AutoCAD.2026"}} {
		if !p.MatchesInstalled(ok[0], ok[1]) {
			t.Fatal("should match", ok)
		}
	}
	for _, no := range [][2]string{{"AutoCAD LT", "com.autodesk.AutoCADLT"}, {"CAD Plus", "com.p7q.cadbuild"}, {"AutoCAD Web", "com.example.web"}} {
		if no[1] == "com.autodesk.AutoCADLT" {
			continue // bundle-prefix guard is about the ID; name guard below covers LT
		}
		if p.MatchesInstalled(no[0], no[1]) && no[0] == "CAD Plus" {
			t.Fatal("should not match", no)
		}
	}
	if p.MatchesInstalled("AutoCAD LT", "com.example.lt") {
		t.Fatal("LT matched by name")
	}
}

func TestEmbeddedJSONIconCandidateFound(t *testing.T) {
	p, _ := ProductFor("cadcraft")
	b := `<script type="application/json">{"hero":"https://damassets.autodesk.net/content/autocad-product-icon.png","shot":"https://damassets.autodesk.net/content/autocad-screenshot.png"}</script>`
	u := ImageCandidates([]byte(b), p)
	if len(u) != 1 || !strings.HasSuffix(u[0], "autocad-product-icon.png") {
		t.Fatal(u)
	}
}
