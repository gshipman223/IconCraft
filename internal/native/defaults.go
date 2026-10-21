package native

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Default-app categories: the things macOS lets a user point at another
// app, gathered into one place because System Settings and Finder spread
// them across a dozen Get Info panels. Scheme categories read/write URL
// handlers (browser, email); the rest read/write UTI role handlers.
type defaultCategory struct {
	ID      string
	Label   string
	Detail  string
	Kind    string // "scheme" | "uti"
	Scheme  string // primary scheme for scheme kinds (read + first write)
	Schemes string // comma-joined schemes written together (browser: http+https)
	UTI     string
}

var defaultCategories = []defaultCategory{
	{ID: "browser", Label: "Web browser", Detail: "Links you click in mail, messages and other apps", Kind: "scheme", Scheme: "https", Schemes: "https,http"},
	{ID: "email", Label: "Email", Detail: "Email links that start a new message", Kind: "scheme", Scheme: "mailto"},
	{ID: "pdf", Label: "PDF documents", Detail: "Double-clicking a PDF in Finder", Kind: "uti", UTI: "com.adobe.pdf"},
	{ID: "images", Label: "Images", Detail: "Double-clicking a picture in Finder", Kind: "uti", UTI: "public.image"},
	{ID: "video", Label: "Movies & video", Detail: "Double-clicking a video file in Finder", Kind: "uti", UTI: "public.movie"},
	{ID: "audio", Label: "Audio", Detail: "Double-clicking a music or audio file in Finder", Kind: "uti", UTI: "public.audio"},
	{ID: "text", Label: "Text files", Detail: "Plain text, notes and read-me files", Kind: "uti", UTI: "public.plain-text"},
	{ID: "code", Label: "Source code", Detail: "Code and configuration files", Kind: "uti", UTI: "public.source-code"},
	{ID: "html", Label: "HTML files", Detail: "Saved web pages opened from Finder", Kind: "uti", UTI: "public.html"},
	{ID: "archives", Label: "ZIP archives", Detail: "Double-clicking a .zip in Finder", Kind: "uti", UTI: "public.zip-archive"},
	{ID: "calendar", Label: "Calendar files", Detail: ".ics invites saved from mail and the web", Kind: "uti", UTI: "com.apple.ical.ics"},
}

// DefaultCandidate is one app macOS says can take over a category.
type DefaultCandidate struct {
	BundleID string `json:"bundleId"`
	Name     string `json:"name"`
	Path     string `json:"path"`
}

// DefaultCategoryState is one category with its live default and choices.
type DefaultCategoryState struct {
	ID         string             `json:"id"`
	Label      string             `json:"label"`
	Detail     string             `json:"detail"`
	Current    string             `json:"current"`
	Candidates []DefaultCandidate `json:"candidates"`
}

type defaultCategorySpec struct {
	ID      string   `json:"id"`
	Kind    string   `json:"kind"`
	Scheme  string   `json:"scheme,omitempty"`
	Schemes []string `json:"schemes,omitempty"`
	UTI     string   `json:"uti,omitempty"`
}

func (c defaultCategory) spec() defaultCategorySpec {
	s := defaultCategorySpec{ID: c.ID, Kind: c.Kind, Scheme: c.Scheme, UTI: c.UTI}
	if c.Schemes != "" {
		s.Schemes = strings.Split(c.Schemes, ",")
	}
	return s
}

type defaultReading struct {
	ID         string            `json:"id"`
	Current    string            `json:"current"`
	Candidates []string          `json:"candidates"`
	Paths      map[string]string `json:"paths"`
}

// DefaultApps reads every category's current default and candidate apps
// from LaunchServices, resolving names from the app bundles themselves.
func (m *Mac) DefaultApps(ctx context.Context) ([]DefaultCategoryState, error) {
	specs := make([]defaultCategorySpec, 0, len(defaultCategories))
	for _, c := range defaultCategories {
		specs = append(specs, c.spec())
	}
	var readings []defaultReading
	err := m.call(ctx, map[string]any{"action": "defaults", "categories": specs}, &readings)
	if err != nil {
		return nil, err
	}
	// Resolve display names for every distinct candidate bundle once.
	pathSet := map[string]bool{}
	for _, r := range readings {
		for _, p := range r.Paths {
			if p != "" {
				pathSet[p] = true
			}
		}
	}
	names := map[string]string{}
	if len(pathSet) > 0 {
		paths := make([]string, 0, len(pathSet))
		for p := range pathSet {
			paths = append(paths, p)
		}
		if apps, ierr := m.Inspect(ctx, paths); ierr == nil {
			for _, a := range apps {
				names[a.Path] = a.Name
			}
		}
	}
	byID := map[string]defaultReading{}
	for _, r := range readings {
		byID[r.ID] = r
	}
	out := make([]DefaultCategoryState, 0, len(defaultCategories))
	for _, c := range defaultCategories {
		r := byID[c.ID]
		state := DefaultCategoryState{ID: c.ID, Label: c.Label, Detail: c.Detail, Current: r.Current, Candidates: []DefaultCandidate{}}
		for _, bid := range r.Candidates {
			p := r.Paths[bid]
			name := names[p]
			if name == "" && p != "" {
				name = strings.TrimSuffix(filepath.Base(p), ".app")
			}
			if name == "" {
				name = bid
			}
			state.Candidates = append(state.Candidates, DefaultCandidate{BundleID: bid, Name: name, Path: p})
		}
		out = append(out, state)
	}
	return out, nil
}

func (c defaultCategory) schemes() []string {
	if c.Schemes != "" {
		return strings.Split(c.Schemes, ",")
	}
	return []string{c.Scheme}
}

// SetDefaultApp points one category at a new app and reports what
// LaunchServices actually kept — never just what was asked for.
//
// Writes run through the bundled iconcraft-defaults helper: macOS
// refuses these writes from unbundled processes (the JXA bridge gets
// -54 for schemes and silent no-ops for UTIs, both verified live), and
// the supported NSWorkspace setters carry the app identity and may put
// up a macOS confirmation for the user to approve.
func (m *Mac) SetDefaultApp(ctx context.Context, categoryID, bundleID string) (DefaultCategoryState, error) {
	var cat *defaultCategory
	for i := range defaultCategories {
		if defaultCategories[i].ID == categoryID {
			cat = &defaultCategories[i]
			break
		}
	}
	if cat == nil {
		return DefaultCategoryState{}, fmt.Errorf("unknown default-app category %q", categoryID)
	}
	if strings.TrimSpace(bundleID) == "" {
		return DefaultCategoryState{}, errors.New("choose an app first")
	}
	exe, err := os.Executable()
	if err != nil {
		return DefaultCategoryState{}, err
	}
	helper := filepath.Join(filepath.Dir(exe), "iconcraft-defaults")
	if _, err := os.Stat(helper); err != nil {
		return DefaultCategoryState{}, errors.New("this build is missing the default-app helper, so defaults cannot be changed")
	}
	type key struct{ kind, value string }
	keys := []key{}
	if cat.Kind == "scheme" {
		for _, s := range cat.schemes() {
			keys = append(keys, key{"scheme", s})
		}
	} else {
		keys = append(keys, key{"uti", cat.UTI})
	}
	for _, k := range keys {
		if err := runDefaultsHelper(ctx, helper, k.kind, k.value, bundleID); err != nil {
			return DefaultCategoryState{}, err
		}
	}
	// Return the refreshed category so the UI shows ground truth.
	all, err := m.DefaultApps(ctx)
	if err != nil {
		return DefaultCategoryState{}, err
	}
	for _, s := range all {
		if s.ID == categoryID {
			if s.Current != bundleID {
				current := s.Current
				if current == "" {
					current = "nothing"
				}
				return DefaultCategoryState{}, fmt.Errorf("macOS kept %s as the default for %s — if it showed a confirmation, accept it there and try again", current, cat.Label)
			}
			return s, nil
		}
	}
	return DefaultCategoryState{}, fmt.Errorf("category %q disappeared after the change", categoryID)
}

// runDefaultsHelper performs one LaunchServices write via the bundled
// helper and translates its JSON verdict into a Go error.
func runDefaultsHelper(ctx context.Context, helper, kind, key, bundleID string) error {
	cctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(cctx, helper, kind, key, bundleID)
	out, err := cmd.Output()
	var res struct {
		Current   string `json:"current"`
		Error     string `json:"error"`
		Cancelled bool   `json:"cancelled"`
	}
	if jerr := json.Unmarshal(bytes.TrimSpace(out), &res); jerr == nil && res.Error != "" {
		if res.Cancelled {
			return errors.New("the macOS confirmation was not approved, so nothing changed")
		}
		return fmt.Errorf("macOS could not make the change: %s", res.Error)
	}
	if err != nil {
		return fmt.Errorf("the default-app helper failed: %v", err)
	}
	if res.Current != bundleID {
		return fmt.Errorf("macOS kept %s as the default instead", res.Current)
	}
	return nil
}
