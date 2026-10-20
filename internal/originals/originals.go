// Package originals discovers genuine artwork from product owners, not icon redraw sites.
// Every Resolve makes fresh network requests; persisted raster assets are offline fallbacks.
package originals

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"html"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

const maxImage = 8 << 20
const maxPage = 8 << 20

type Product struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	Page       string   `json:"page"`
	Aliases    []string `json:"-"`
	Direct     []string `json:"-"`
	StoreID    string   `json:"-"`
	BundleIDs  []string `json:"-"`
	Publisher  string   `json:"-"`
	Favicon    bool     `json:"-"`
	ExtraPages []string `json:"-"`
}
type Candidate struct {
	Product Product `json:"product"`
	Source  string  `json:"source"`
	MIME    string  `json:"mime"`
	Data    []byte  `json:"-"`
}

func (c Candidate) DataURL() string {
	return "data:" + c.MIME + ";base64," + base64.StdEncoding.EncodeToString(c.Data)
}
func adobe(id, name, page string, files ...string) Product {
	p := Product{ID: id, Name: "Adobe " + name, Page: "https://www.adobe.com/" + page, Publisher: "Adobe Inc.", Aliases: []string{strings.ToLower(name), strings.ReplaceAll(strings.ToLower(name), " ", "-")}}
	for _, file := range files {
		p.Direct = append(p.Direct, "https://www.adobe.com/cc-shared/assets/img/product-icons/svg/"+file)
	}
	if len(p.Direct) > 0 {
		p.Direct = append(p.Direct, "https://main--cc--adobecom.aem.live/cc-shared/assets/img/product-icons/svg/"+files[0])
	}
	p.ExtraPages = []string{"https://www.adobe.com/au/" + page, "https://helpx.adobe.com/" + id + ".html"}
	return p
}
func Products() map[string]Product {
	p := map[string]Product{
		"photocraft":  adobe("photoshop", "Photoshop", "products/photoshop.html", "photoshop-40.svg", "photoshop.svg"),
		"vectorcraft": adobe("illustrator", "Illustrator", "products/illustrator.html", "illustrator-40.svg", "illustrator.svg"),
		"filmcraft":   adobe("premiere", "Premiere", "products/premiere.html", "premiere-40.svg", "premiere.svg", "premiere-pro.svg"),
		"lightcraft":  adobe("lightroom", "Lightroom", "products/photoshop-lightroom.html", "lightroom-40.svg", "lightroom.svg"),
		"pdfcraft":    adobe("acrobat", "Acrobat", "acrobat.html", "acrobat-reader.svg", "acrobat-pro.svg", "acrobat.svg"),
		"effectcraft": adobe("after-effects", "After Effects", "products/aftereffects.html", "after-effects-40.svg", "after-effects.svg"),
		"designcraft": adobe("indesign", "InDesign", "products/indesign.html", "indesign-40.svg", "indesign.svg"),
		"figcraft":    {ID: "figma", Name: "Figma", Page: "https://www.figma.com/", Aliases: []string{"figma"}, Favicon: true, Publisher: "Figma, Inc.", BundleIDs: []string{"com.figma.Desktop"}},
		"wordcraft":   {ID: "word", Name: "Microsoft Word", Page: "https://apps.apple.com/us/app/microsoft-word/id462054704", Aliases: []string{"microsoft word"}, StoreID: "462054704", BundleIDs: []string{"com.microsoft.Word"}, Publisher: "Microsoft Corporation"},
		"gridcraft":   {ID: "excel", Name: "Microsoft Excel", Page: "https://apps.apple.com/us/app/microsoft-excel/id462058435", Aliases: []string{"microsoft excel"}, StoreID: "462058435", BundleIDs: []string{"com.microsoft.Excel"}, Publisher: "Microsoft Corporation"},
		"deckcraft":   {ID: "powerpoint", Name: "Microsoft PowerPoint", Page: "https://apps.apple.com/us/app/microsoft-powerpoint/id462062816", Aliases: []string{"microsoft powerpoint"}, StoreID: "462062816", BundleIDs: []string{"com.microsoft.Powerpoint"}, Publisher: "Microsoft Corporation"},
		"soundcraft":  {ID: "pro-tools", Name: "Pro Tools", Page: "https://www.avid.com/pro-tools", Aliases: []string{"pro tools", "pro-tools", "protools"}, Publisher: "Avid", BundleIDs: []string{"com.avid.ProTools", "com.digidesign.ProTools"}},
		// AutoCAD: autodesk.com bot-blocks programmatic page fetches (HTTP 403),
		// so the verified App Store listing (Autodesk's own AutoCAD app,
		// track 393149734) is a first-class source, not a last resort.
		"cadcraft": {ID: "autocad", Name: "AutoCAD", Page: "https://www.autodesk.com/products/autocad/overview", Aliases: []string{"autocad"}, Publisher: "Autodesk", StoreID: "393149734", BundleIDs: []string{"com.autodesk.AutoCAD", "com.autodesk.autocadws"}},
	}
	p["photocraft"] = withBundles(p["photocraft"], "com.adobe.Photoshop")
	p["vectorcraft"] = withBundles(p["vectorcraft"], "com.adobe.illustrator")
	p["filmcraft"] = withBundles(p["filmcraft"], "com.adobe.PremierePro")
	p["lightcraft"] = withBundles(p["lightcraft"], "com.adobe.LightroomCC", "com.adobe.LightroomClassicCC7", "com.adobe.Lightroom")
	p["pdfcraft"] = withBundles(p["pdfcraft"], "com.adobe.Acrobat.Pro", "com.adobe.Reader", "com.adobe.Acrobat.Reader")
	p["effectcraft"] = withBundles(p["effectcraft"], "com.adobe.AfterEffects")
	p["designcraft"] = withBundles(p["designcraft"], "com.adobe.InDesign")
	pdf := p["pdfcraft"]
	pdf.Direct = append([]string{"https://www.adobe.com/dc-shared/assets/images/product-icons/svg/acrobat-pro-40.svg", "https://main--dc--adobecom.aem.live/dc-shared/assets/images/product-icons/svg/acrobat-pro-40.svg"}, pdf.Direct...)
	p["pdfcraft"] = pdf
	cad := p["cadcraft"]
	cad.ExtraPages = []string{"https://www.autodesk.com/", "https://www.autodesk.com/au/products/autocad/overview"}
	p["cadcraft"] = cad
	sound := p["soundcraft"]
	sound.ExtraPages = []string{"https://kb.avid.com/pkb/articles/Knowledge/How-to-Acquire-Pro-Tools"}
	p["soundcraft"] = sound
	for craft, slug := range map[string]string{"wordcraft": "word", "gridcraft": "excel", "deckcraft": "powerpoint"} {
		v := p[craft]
		v.ExtraPages = []string{"https://www.microsoft.com/en-us/microsoft-365/" + slug, "https://apps.apple.com/au/app/microsoft-" + slug + "/id" + v.StoreID}
		p[craft] = v
	}
	fig := p["figcraft"]
	fig.ExtraPages = []string{"https://www.figma.com/downloads/", "https://www.figma.com/design/"}
	p["figcraft"] = fig
	p["printcraft"] = p["pdfcraft"]
	return p
}
func withBundles(p Product, ids ...string) Product { p.BundleIDs = ids; return p }

// MatchesInstalled reports whether an installed app is this product, for the
// extract-the-genuine-icon-from-the-real-app fallback. Accepts an exact
// bundle ID, a year/variant-suffixed bundle ID (com.autodesk.AutoCAD2025,
// com.avid.ProTools12), or an exact product-name/alias match. Deliberately
// rejects lookalikes (AutoCAD LT, knockoff store apps) via the alias guards
// in ImageCandidates-style exact matching: a suffixed bundle must extend a
// known bundle ID, and a name must equal the product name or an alias.
func (p Product) bundleMatches(bundleID string) bool {
	for _, id := range p.BundleIDs {
		if strings.EqualFold(bundleID, id) {
			return true
		}
		if len(bundleID) > len(id) && strings.EqualFold(bundleID[:len(id)], id) {
			rest := bundleID[len(id):]
			if rest[0] == '.' || (rest[0] >= '0' && rest[0] <= '9') {
				return true
			}
		}
	}
	return false
}

func (p Product) MatchesInstalled(name, bundleID string) bool {
	if p.bundleMatches(bundleID) {
		return true
	}
	n := normalize(name)
	if n == normalize(p.Name) {
		return true
	}
	for _, a := range p.Aliases {
		if n == normalize(a) {
			return true
		}
	}
	return false
}
func ProductFor(craftID string) (Product, bool) {
	p, ok := Products()[strings.ToLower(craftID)]
	return p, ok
}

// Only known publisher/CDN origins. No user URL, file URL, credentials, private host,
// arbitrary redirect, search-result executable or downloaded code is accepted.
func AllowedURL(raw string) bool {
	u, e := url.Parse(raw)
	if e != nil || u.Scheme != "https" || u.User != nil || u.Port() != "" {
		return false
	}
	host := strings.ToLower(u.Hostname())
	for _, domain := range []string{"adobe.com", "adobecc.com", "adobecqms.net", "adobe.io", "apple.com", "mzstatic.com", "figma.com", "avid.com", "autodesk.com", "autodesk.net", "microsoft.com", "office.net", "office.com"} {
		if host == domain || strings.HasSuffix(host, "."+domain) {
			return true
		}
	}
	// Adobe-owned deployment aliases and CDNs referenced by official publisher pages.
	for _, hostOK := range []string{"main--dc--adobecom.aem.live", "main--dc--adobecom.aem.page", "main--da-dc--adobecom.aem.page", "main--edu--adobecom.aem.page", "avid.secure.force.com", "avidtech.my.salesforce-sites.com", "main--cc--adobecom.aem.live", "main--cc--adobecom.aem.page", "main--da-cc--adobecom.aem.live", "main--da-cc--adobecom.aem.page", "cdn.sanity.io", "static.figma.com", "edge.sitecorecloud.io", "damassets.autodesk.net", "damassets.autodesk.com", "api.storyteller.ai", "getartcraft.com", "www.getartcraft.com"} {
		if host == hostOK {
			return true
		}
	}
	return false
}

type httpEntry struct {
	Body           []byte
	ETag, Modified string
}
type Resolver struct {
	Client *http.Client
	mu     sync.Mutex
	cache  map[string]httpEntry
	limit  chan struct{}
}

func New() *Resolver {
	return &Resolver{cache: map[string]httpEntry{}, limit: make(chan struct{}, 18), Client: &http.Client{Timeout: 4 * time.Second, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) > 3 || !AllowedURL(req.URL.String()) {
			return errors.New("icon source redirected outside publisher origins")
		}
		return nil
	}}}
}
func (r *Resolver) get(ctx context.Context, raw string, limit int64) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 3500*time.Millisecond)
	defer cancel()
	if r.limit != nil {
		select {
		case r.limit <- struct{}{}:
			defer func() { <-r.limit }()
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	if !AllowedURL(raw) {
		return nil, errors.New("unapproved icon source")
	}
	req, err := http.NewRequestWithContext(ctx, "GET", raw, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 IconCraft/0.4")
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("Accept", "image/svg+xml,image/png,image/jpeg,application/json,text/html;q=0.9,*/*;q=0.5")
	r.mu.Lock()
	cached, hasCache := r.cache[raw]
	r.mu.Unlock()
	if hasCache {
		if cached.ETag != "" {
			req.Header.Set("If-None-Match", cached.ETag)
		}
		if cached.Modified != "" {
			req.Header.Set("If-Modified-Since", cached.Modified)
		}
	}
	res, err := r.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode == 304 && hasCache {
		return cached.Body, nil
	}
	if res.StatusCode != 200 {
		return nil, fmt.Errorf("source returned HTTP %d", res.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(res.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > limit {
		return nil, errors.New("source exceeded size limit")
	}
	if len(b) <= 2<<20 && (res.Header.Get("ETag") != "" || res.Header.Get("Last-Modified") != "") {
		r.mu.Lock()
		if r.cache == nil {
			r.cache = map[string]httpEntry{}
		}
		if len(r.cache) > 128 {
			r.cache = map[string]httpEntry{}
		}
		r.cache[raw] = httpEntry{b, res.Header.Get("ETag"), res.Header.Get("Last-Modified")}
		r.mu.Unlock()
	}
	return b, nil
}
func (r *Resolver) Resolve(ctx context.Context, craftID string, exclude []string) (Candidate, error) {
	p, ok := ProductFor(craftID)
	if !ok {
		var err error
		p, err = r.discoverProduct(ctx, craftID)
		if err != nil {
			return Candidate{}, err
		}
	}
	return r.resolveProduct(ctx, p, exclude)
}

// Resolve independent publisher sources concurrently. Every layer runs even
// if another is slow or blocked (e.g. autodesk.com/avid.com return HTTP 403
// to programmatic fetches): publisher product pages (+ embedded assets),
// fixed publisher CDN paths, verified App Store listing by ID, verified App
// Store search by publisher+product, and finally — in the caller — a saved
// original, then the icon of the genuinely installed product. "Not found" is
// only reported after all of them, with a per-layer summary of what failed.
func (r *Resolver) resolveProduct(ctx context.Context, p Product, exclude []string) (Candidate, error) {
	ctx, cancel := context.WithTimeout(ctx, 12*time.Second)
	var wg sync.WaitGroup
	defer func() { cancel(); wg.Wait() }()
	type answer struct {
		candidate Candidate
		priority  int
		layer     string
		err       error
	}
	type job struct {
		layer    string
		priority int
		run      func() (Candidate, error)
	}
	jobs := []job{}
	excluded := map[string]bool{}
	for _, u := range exclude {
		excluded[u] = true
	}
	download := func(urls []string) (Candidate, error) {
		last := errors.New("no suitable icon link on this source")
		attempts := 0
		for _, u := range urls {
			if excluded[u] || !AllowedURL(u) {
				continue
			}
			attempts++
			if attempts > 2 {
				break
			}
			b, err := r.get(ctx, u, maxImage)
			if err != nil {
				last = err
				continue
			}
			mime, err := ValidateArtwork(b)
			if err != nil {
				last = err
				continue
			}
			return Candidate{Product: p, Source: u, MIME: mime, Data: b}, nil
		}
		return Candidate{}, last
	}
	pages := append([]string{p.Page}, p.ExtraPages...)
	for i, page := range pages {
		if i >= 3 {
			break
		}
		page := page
		jobs = append(jobs, job{"publisher page " + pageHost(page), 100, func() (Candidate, error) {
			b, err := r.get(ctx, page, maxPage)
			if err != nil {
				return Candidate{}, err
			}
			v := p
			v.Page = page
			return download(ImageCandidates(b, v))
		}})
	}
	if p.StoreID != "" {
		for _, country := range []string{"us", "au"} {
			country := country
			jobs = append(jobs, job{"App Store listing (" + country + ")", 110, func() (Candidate, error) {
				b, err := r.get(ctx, "https://itunes.apple.com/lookup?id="+p.StoreID+"&country="+country, 1<<20)
				if err != nil {
					return Candidate{}, err
				}
				u, err := StoreArtwork(b, p)
				if err != nil {
					return Candidate{}, err
				}
				return download([]string{u})
			}})
		}
	}
	if p.Publisher != "" {
		jobs = append(jobs, job{"App Store search", 105, func() (Candidate, error) {
			var lastErr error
			for _, entity := range []string{"macSoftware", "software"} {
				b, err := r.get(ctx, "https://itunes.apple.com/search?term="+url.QueryEscape(p.Name)+"&entity="+entity+"&country=us&limit=10", 1<<20)
				if err != nil {
					lastErr = err
					continue
				}
				u, err := StoreSearchArtwork(b, p)
				if err != nil {
					lastErr = err
					continue
				}
				return download([]string{u})
			}
			return Candidate{}, lastErr
		}})
	}
	for i, u := range p.Direct {
		if i >= 6 {
			break
		}
		u := u
		if excluded[u] {
			continue
		}
		jobs = append(jobs, job{"publisher CDN", 80, func() (Candidate, error) { return download([]string{u}) }})
	}
	answers := make(chan answer, len(jobs))
	for _, j := range jobs {
		wg.Add(1)
		go func(j job) {
			defer wg.Done()
			c, err := j.run()
			answers <- answer{c, j.priority, j.layer, err}
		}(j)
	}
	var best *answer
	var grace <-chan time.Time
	var timer *time.Timer
	defer func() {
		if timer != nil {
			timer.Stop()
		}
	}()
	failures := map[string]string{}
	recordFailure := func(v answer) {
		if _, ok := failures[v.layer]; !ok {
			failures[v.layer] = v.err.Error()
		}
	}
	summary := func() string {
		layers := make([]string, 0, len(failures))
		for k := range failures {
			layers = append(layers, k)
		}
		sort.Strings(layers)
		parts := make([]string, 0, len(layers))
		for _, k := range layers {
			parts = append(parts, k+": "+failures[k])
		}
		s := strings.Join(parts, "; ")
		if len(s) > 300 {
			s = s[:297] + "…"
		}
		return s
	}
	for left := len(jobs); left > 0; left-- {
		select {
		case v := <-answers:
			if v.err != nil {
				recordFailure(v)
				continue
			}
			if best == nil || v.priority > best.priority {
				copy := v
				best = &copy
			}
			if v.priority >= 100 {
				return v.candidate, nil
			}
			if timer == nil {
				timer = time.NewTimer(300 * time.Millisecond)
				grace = timer.C
			}
		case <-grace:
			if best != nil {
				return best.candidate, nil
			}
		case <-ctx.Done():
			if best != nil {
				return best.candidate, nil
			}
			return Candidate{}, fmt.Errorf("%s: timed out after checking %s — saved or installed originals are still available", p.Name, summary())
		}
	}
	if best != nil {
		return best.candidate, nil
	}
	return Candidate{}, fmt.Errorf("%s: no usable icon after %d checks (%s)", p.Name, len(jobs), summary())
}

func pageHost(raw string) string {
	if u, err := url.Parse(raw); err == nil && u.Host != "" {
		return u.Host
	}
	return raw
}
func StoreArtwork(data []byte, p Product) (string, error) {
	var v struct {
		Results []struct {
			TrackID    int64  `json:"trackId"`
			BundleID   string `json:"bundleId"`
			SellerName string `json:"sellerName"`
			Artwork512 string `json:"artworkUrl512"`
			Artwork100 string `json:"artworkUrl100"`
		} `json:"results"`
	}
	if err := json.Unmarshal(data, &v); err != nil {
		return "", err
	}
	for _, x := range v.Results {
		if fmt.Sprint(x.TrackID) != p.StoreID || !strings.EqualFold(x.SellerName, p.Publisher) {
			continue
		}
		good := false
		for _, id := range p.BundleIDs {
			if strings.EqualFold(id, x.BundleID) {
				good = true
			}
		}
		if !good {
			continue
		}
		u := x.Artwork512
		if u == "" {
			u = x.Artwork100
		}
		if AllowedURL(u) {
			return u, nil
		}
	}
	return "", errors.New("App Store product identity did not match")
}

// StoreSearchArtwork picks the genuine icon from an iTunes *search* response.
// Unlike StoreArtwork (exact track ID), this must not be fooled by knockoffs:
// the seller must be the publisher and the app must be the product itself —
// exact track-name/alias match or a known bundle ID (incl. suffixed variants).
func StoreSearchArtwork(data []byte, p Product) (string, error) {
	var v struct {
		Results []struct {
			TrackName  string `json:"trackName"`
			BundleID   string `json:"bundleId"`
			SellerName string `json:"sellerName"`
			Artwork512 string `json:"artworkUrl512"`
			Artwork100 string `json:"artworkUrl100"`
		} `json:"results"`
	}
	if err := json.Unmarshal(data, &v); err != nil {
		return "", err
	}
	for _, x := range v.Results {
		if !sellerMatches(x.SellerName, p) {
			continue
		}
		nameOK := normalize(x.TrackName) == normalize(p.Name)
		for _, a := range p.Aliases {
			if normalize(x.TrackName) == normalize(a) {
				nameOK = true
			}
		}
		// All three must agree: publisher seller, exact product name, and a
		// known (or variant-suffixed) bundle ID.
		if !nameOK || !p.bundleMatches(x.BundleID) {
			continue
		}
		u := x.Artwork512
		if u == "" {
			u = x.Artwork100
		}
		if AllowedURL(u) {
			return u, nil
		}
	}
	return "", errors.New("App Store search found no verified " + p.Name + " listing")
}

func sellerMatches(seller string, p Product) bool {
	if p.Publisher == "" {
		return false
	}
	s, pub := normalize(seller), normalize(p.Publisher)
	return s != "" && (s == pub || strings.Contains(s, pub) || strings.Contains(pub, s))
}

var rawImageURLs = regexp.MustCompile(`(?i)https?://[^\s"'<>\\]+?\.(?:png|svg|jpe?g)(?:\?[^\s"'<>\\]*)?`)

var tags = regexp.MustCompile(`(?is)<(?:img|source|link|meta)\b[^>]*>`)
var attrs = regexp.MustCompile(`(?is)([a-z][a-z0-9:_-]*)\s*=\s*(?:"([^"]*)"|'([^']*)')`)

func attributes(tag string) map[string]string {
	out := map[string]string{}
	for _, m := range attrs.FindAllStringSubmatch(tag, -1) {
		v := m[2]
		if v == "" {
			v = m[3]
		}
		out[strings.ToLower(m[1])] = html.UnescapeString(v)
	}
	return out
}
func normalize(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}
func ImageCandidates(page []byte, p Product) []string {
	type item struct {
		u     string
		score int
	}
	items := []item{}
	base, _ := url.Parse(p.Page)
	for _, tag := range tags.FindAllString(string(page), -1) {
		if len(tag) > 16000 {
			continue
		}
		a := attributes(tag)
		value := a["src"]
		if value == "" {
			value = a["data-src"]
		}
		if value == "" {
			value = a["href"]
		}
		if value == "" && a["srcset"] != "" {
			parts := strings.Split(a["srcset"], ",")
			fields := strings.Fields(strings.TrimSpace(parts[len(parts)-1]))
			if len(fields) > 0 {
				value = fields[0]
			}
		}
		if value == "" && a["property"] == "og:image" && p.StoreID != "" {
			value = a["content"]
		}
		if value == "" {
			continue
		}
		u, err := url.Parse(value)
		if err != nil {
			continue
		}
		u = base.ResolveReference(u)
		if !AllowedURL(u.String()) {
			continue
		}
		desc := strings.ToLower(a["alt"] + " " + a["title"] + " " + u.Path + " " + a["class"])
		compact := normalize(desc)
		matched := false
		for _, v := range p.Aliases {
			if strings.Contains(compact, normalize(v)) {
				matched = true
			}
		}
		score := 0
		if p.ID == "figma" && strings.Contains(desc, "desktop app icon") {
			score = 130
		}
		if p.StoreID != "" && (matched || strings.Contains(strings.ToLower(a["class"]), "app-icon") || a["property"] == "og:image") {
			if strings.Contains(desc, "artwork") || strings.Contains(desc, "app-icon") || a["property"] == "og:image" {
				score = 90
			}
		}
		if p.Favicon && strings.Contains(strings.ToLower(a["rel"]), "apple-touch-icon") {
			score = 120
		}
		if p.Favicon && a["rel"] == "icon" && (strings.HasSuffix(u.Path, ".svg") || strings.HasSuffix(u.Path, ".png")) {
			score = 60
		}
		if matched && (strings.Contains(desc, "icon") || strings.Contains(desc, "logo")) {
			score += 70
		}
		if matched && strings.Contains(desc, "product-icon") {
			score += 40
		}
		if strings.Contains(desc, "screenshot") || strings.Contains(desc, "wordmark") || strings.Contains(desc, "lockup") || strings.Contains(desc, "logotype") || strings.Contains(desc, "banner") || strings.Contains(desc, "mockup") {
			score = 0
		}
		if p.ID == "autocad" && (strings.Contains(compact, "autocadlt") || strings.Contains(compact, "autocadweb") || strings.Contains(compact, "autocadmobile")) {
			score = 0
		}
		if score < 60 {
			continue
		}
		items = append(items, item{u.String(), score})
	}
	// JS-heavy publisher pages often carry the real icon URL only inside
	// embedded JSON/script data, not in any <img> tag. Scan raw URLs too and
	// apply the same alias/icon scoring and wordmark/screenshot rejections.
	for _, raw := range rawImageURLs.FindAllString(string(page), -1) {
		u, err := url.Parse(strings.TrimRight(raw, `",')\]}`))
		if err != nil || !AllowedURL(u.String()) {
			continue
		}
		desc := strings.ToLower(u.Path)
		compact := normalize(desc)
		matched := false
		for _, v := range p.Aliases {
			if strings.Contains(compact, normalize(v)) {
				matched = true
			}
		}
		if !matched {
			continue
		}
		score := 0
		if strings.Contains(desc, "icon") || strings.Contains(desc, "logo") {
			score += 70
		}
		if strings.Contains(desc, "product-icon") {
			score += 40
		}
		if strings.Contains(desc, "screenshot") || strings.Contains(desc, "wordmark") || strings.Contains(desc, "banner") || strings.Contains(desc, "mockup") {
			score = 0
		}
		if p.ID == "autocad" && (strings.Contains(compact, "autocadlt") || strings.Contains(compact, "autocadweb") || strings.Contains(compact, "autocadmobile")) {
			score = 0
		}
		if score >= 60 {
			items = append(items, item{u.String(), score})
		}
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].score > items[j].score })
	out := []string{}
	seen := map[string]bool{}
	for _, i := range items {
		if !seen[i.u] {
			seen[i.u] = true
			out = append(out, i.u)
		}
	}
	return out
}
func ValidateArtwork(b []byte) (string, error) {
	if len(b) == 0 || len(b) > maxImage {
		return "", errors.New("empty or oversized image")
	}
	trim := bytes.TrimSpace(b)
	if bytes.HasPrefix(trim, []byte("<")) {
		if err := ValidateSVG(trim); err != nil {
			return "", err
		}
		return "image/svg+xml", nil
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(b))
	if err != nil {
		return "", errors.New("publisher response was not an icon image")
	}
	if cfg.Width < 32 || cfg.Height < 32 || cfg.Width > 4096 || cfg.Height > 4096 {
		return "", errors.New("icon dimensions outside supported range")
	}
	ratio := float64(cfg.Width) / float64(cfg.Height)
	if ratio < 0.60 || ratio > 1.6 {
		return "", errors.New("publisher image is a wordmark or banner, not an app icon")
	}
	if _, _, err = image.Decode(bytes.NewReader(b)); err != nil {
		return "", err
	}
	switch format {
	case "png":
		return "image/png", nil
	case "jpeg":
		return "image/jpeg", nil
	case "gif":
		return "image/gif", nil
	}
	return "", errors.New("unsupported image format")
}
func ValidateSVG(b []byte) error {
	if len(b) > 1<<20 {
		return errors.New("SVG is too large")
	}
	d := xml.NewDecoder(bytes.NewReader(b))
	root := false
	depth := 0
	nodes := 0
	for {
		t, err := d.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		switch v := t.(type) {
		case xml.Directive:
			return errors.New("SVG directives are not accepted")
		case xml.ProcInst:
			if v.Target != "xml" {
				return errors.New("external SVG stylesheets are not accepted")
			}
		case xml.StartElement:
			nodes++
			depth++
			if nodes > 20000 || depth > 80 {
				return errors.New("SVG complexity limit")
			}
			name := strings.ToLower(v.Name.Local)
			if !root {
				if name != "svg" {
					return errors.New("not an SVG image")
				}
				root = true
			}
			switch name {
			case "script", "foreignobject", "iframe", "image", "audio", "video", "a", "animate", "animatetransform", "set":
				return errors.New("active or external SVG content is not accepted")
			}
			for _, a := range v.Attr {
				k := strings.ToLower(a.Name.Local)
				value := strings.ToLower(strings.TrimSpace(a.Value))
				if strings.HasPrefix(k, "on") || strings.Contains(value, "javascript:") || strings.Contains(value, "@import") {
					return errors.New("active SVG attribute")
				}
				if (k == "href" || k == "src") && !strings.HasPrefix(value, "#") {
					return errors.New("external SVG resource")
				}
				if strings.Contains(value, "url(") && !internalURLs(value) {
					return errors.New("external SVG paint")
				}
			}
		case xml.EndElement:
			depth--
		case xml.CharData:
			text := strings.ToLower(string(v))
			if strings.Contains(text, "@import") || strings.Contains(text, "@font-face") || (strings.Contains(text, "url(") && !internalURLs(text)) {
				return errors.New("external SVG stylesheet")
			}
		}
	}
	if !root || depth != 0 {
		return errors.New("incomplete SVG")
	}
	return nil
}

var urlsInCSS = regexp.MustCompile(`(?i)url\(\s*['"]?([^)'"\s]+)`)

func internalURLs(s string) bool {
	for _, m := range urlsInCSS.FindAllStringSubmatch(s, -1) {
		if !strings.HasPrefix(m[1], "#") {
			return false
		}
	}
	return !strings.Contains(s, `\`)
}

var safeID = regexp.MustCompile(`^[a-z][a-z0-9-]{1,79}$`)
var explicitComparison = regexp.MustCompile(`(?is)(?:alternative\s+to|replacement\s+for|inspired\s+by|equivalent\s+to|clone\s+of)\s*[:\-]?\s*(?:<[^>]+>\s*)?([a-z][a-z 0-9-]{2,60})`)

func (r *Resolver) discoverProduct(ctx context.Context, id string) (Product, error) {
	if !safeID.MatchString(id) {
		return Product{}, errors.New("unknown counterpart")
	}
	b, err := r.get(ctx, "https://getartcraft.com/apps/"+id, 2<<20)
	if err != nil {
		return Product{}, errors.New("no known counterpart; Custom icon is available")
	}
	for _, m := range explicitComparison.FindAllStringSubmatch(html.UnescapeString(string(b)), -1) {
		best := Product{}
		length := 0
		for _, p := range Products() {
			for _, name := range append(p.Aliases, p.Name) {
				n := normalize(name)
				if strings.HasPrefix(normalize(m[1]), n) && len(n) > length {
					best = p
					length = len(n)
				}
			}
		}
		if length > 0 {
			return best, nil
		}
	}
	return Product{}, errors.New("no verified counterpart found; choose a custom icon")
}
