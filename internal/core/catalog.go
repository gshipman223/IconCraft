package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

type Catalog struct {
	Apps    map[string]string `json:"apps"`
	Fetched string            `json:"fetched,omitempty"`
	Message string            `json:"message"`
}

func DefaultCatalog() Catalog {
	return Catalog{Apps: map[string]string{
		"photocraft": "Adobe Photoshop", "vectorcraft": "Adobe Illustrator", "filmcraft": "Adobe Premiere Pro", "lightcraft": "Adobe Lightroom", "pdfcraft": "Adobe Acrobat", "printcraft": "Adobe Acrobat", "effectcraft": "Adobe After Effects", "designcraft": "Adobe InDesign", "figcraft": "Figma", "wordcraft": "Microsoft Word", "gridcraft": "Microsoft Excel", "deckcraft": "Microsoft PowerPoint", "soundcraft": "Pro Tools (suggested)", "cadcraft": "AutoCAD (suggested)"}, Message: "Built-in suggestions. Verify each counterpart before applying an icon."}
}

var validCraftID = regexp.MustCompile(`^[a-z][a-z0-9-]{1,79}$`)

func NormalizeName(s string) string {
	return strings.Map(func(r rune) rune {
		if r >= 'A' && r <= 'Z' {
			return r + 32
		}
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, s)
}
func (c Catalog) Match(a App) (string, string) {
	id := strings.TrimPrefix(a.BundleID, "ai.storyteller.")
	if id != a.BundleID && validCraftID.MatchString(id) && id != "artcraft-launcher" {
		return id, c.Apps[id]
	}
	n := NormalizeName(a.Name)
	for id, v := range c.Apps {
		if NormalizeName(id) == n {
			return id, v
		}
	}
	return "", ""
}
func ParseFeed(data []byte, old Catalog) (Catalog, error) {
	var root struct {
		ReleaseInfo struct {
			Apps map[string]json.RawMessage `json:"apps"`
		} `json:"release_info"`
	}
	if err := json.Unmarshal(data, &root); err != nil {
		return old, err
	}
	if len(root.ReleaseInfo.Apps) == 0 || len(root.ReleaseInfo.Apps) > 1000 {
		return old, errors.New("catalogue contained no apps or exceeded the entry limit")
	}
	c := Catalog{Apps: map[string]string{}, Fetched: now(), Message: "ArtCraft release feed refreshed. Unknown counterparts require your choice."}
	for k, v := range old.Apps {
		c.Apps[k] = v
	}
	for key := range root.ReleaseInfo.Apps {
		id := strings.ToLower(key)
		if validCraftID.MatchString(id) && id != "artcraft-launcher" && id != "craft-launcher" {
			if _, ok := c.Apps[id]; !ok {
				c.Apps[id] = ""
			}
		}
	}
	return c, nil
}
func FetchCatalog(ctx context.Context, old Catalog) (Catalog, error) {
	client := &http.Client{Timeout: 12 * time.Second, CheckRedirect: func(r *http.Request, via []*http.Request) error {
		if len(via) > 2 || r.URL.Scheme != "https" || r.URL.Host != "api.storyteller.ai" {
			return errors.New("unexpected catalogue redirect")
		}
		return nil
	}}
	req, err := http.NewRequestWithContext(ctx, "GET", "https://api.storyteller.ai/v1/craft_apps/release_info", nil)
	if err != nil {
		return old, err
	}
	req.Header.Set("User-Agent", "IconCraft/"+Version)
	resp, err := client.Do(req)
	if err != nil {
		return old, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return old, fmt.Errorf("catalogue returned HTTP %d", resp.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 3<<20+1))
	if err != nil {
		return old, err
	}
	if len(b) > 3<<20 {
		return old, errors.New("catalogue exceeded size limit")
	}
	return ParseFeed(b, old)
}
