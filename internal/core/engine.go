package core

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image/png"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

type Engine struct {
	mu      sync.Mutex
	scanMu  sync.Mutex
	Dir     string
	Home    string
	Native  Native
	State   State
	Catalog Catalog
	Apps    map[string]App
	Roots   []string
}

func Open(dir, home string, n Native) (*Engine, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	fi, err := os.Lstat(dir)
	if err != nil || !fi.IsDir() || fi.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("unsafe data directory")
	}
	if err = os.Chmod(dir, 0700); err != nil {
		return nil, err
	}
	e := &Engine{Dir: dir, Home: home, Native: n, Apps: map[string]App{}, Roots: []string{"/Applications", filepath.Join(home, "Applications"), "/System/Applications"}, Catalog: DefaultCatalog(), State: State{Schema: 1, Assets: map[string]Asset{}, Records: []Record{}, Preferences: map[string]Preference{}, ExtraRoots: []string{}}}
	b, err := ReadLimited(filepath.Join(dir, "state.json"), 128<<20)
	if err == nil {
		if err = DecodeStrict(b, &e.State); err != nil {
			return nil, fmt.Errorf("state unreadable; preserved without overwrite: %w", err)
		}
		if e.State.Schema != 1 {
			return nil, errors.New("unsupported state version")
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	if e.State.Originals == nil {
		e.State.Originals = map[string]Original{}
	}
	if e.State.Choices == nil {
		e.State.Choices = map[string]string{}
	}
	if e.State.Assets == nil {
		e.State.Assets = map[string]Asset{}
	}
	if e.State.Preferences == nil {
		e.State.Preferences = map[string]Preference{}
	}
	if e.State.Vault == nil {
		e.State.Vault = map[string]SavedOriginal{}
	}
	if b, err = ReadLimited(filepath.Join(dir, "catalogue.json"), 3<<20); err == nil {
		var c Catalog
		if json.Unmarshal(b, &c) == nil && len(c.Apps) > 0 {
			e.Catalog = c
		}
	}
	e.loadInventory()
	e.recoverRenames()
	return e, nil
}
func (e *Engine) save() error {
	b, err := json.MarshalIndent(e.State, "", "  ")
	if err != nil {
		return err
	}
	return AtomicWrite(filepath.Join(e.Dir, "state.json"), b, 0600)
}
func (e *Engine) SnapshotState() State {
	e.mu.Lock()
	defer e.mu.Unlock()
	b, _ := json.Marshal(e.State)
	var s State
	_ = json.Unmarshal(b, &s)
	return s
}
func (e *Engine) Catalogue() Catalog {
	e.mu.Lock()
	defer e.mu.Unlock()
	c := e.Catalog
	c.Apps = map[string]string{}
	for k, v := range e.Catalog.Apps {
		c.Apps[k] = v
	}
	return c
}
func (e *Engine) Refresh(ctx context.Context) error {
	c, err := FetchCatalog(ctx, e.Catalogue())
	if err != nil {
		return err
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	b, _ := json.MarshalIndent(c, "", "  ")
	if err = AtomicWrite(filepath.Join(e.Dir, "catalogue.json"), b, 0600); err != nil {
		return err
	}
	e.Catalog = c
	return nil
}
func Discover(roots []string) ([]string, []string) {
	seen := map[string]bool{}
	warnings := []string{}
	for _, root := range roots {
		root = filepath.Clean(root)
		if !filepath.IsAbs(root) {
			continue
		}
		if _, err := os.Stat(root); os.IsNotExist(err) {
			continue
		}
		err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				warnings = append(warnings, p+": "+err.Error())
				return fs.SkipDir
			}
			if len(seen) >= 3000 {
				return fs.SkipAll
			}
			rel, _ := filepath.Rel(root, p)
			if strings.Count(rel, string(os.PathSeparator)) > 5 {
				return fs.SkipDir
			}
			if d.Type()&os.ModeSymlink != 0 {
				return nil
			}
			if d.IsDir() && strings.EqualFold(filepath.Ext(p), ".app") {
				if real, er := filepath.EvalSymlinks(p); er == nil {
					seen[real] = true
				}
				return fs.SkipDir
			}
			return nil
		})
		if err != nil {
			warnings = append(warnings, err.Error())
		}
	}
	out := make([]string, 0, len(seen))
	for p := range seen {
		out = append(out, p)
	}
	sort.Strings(out)
	return out, warnings
}
func (e *Engine) Scan(ctx context.Context) ([]App, []string, error) {
	e.scanMu.Lock()
	defer e.scanMu.Unlock()
	e.mu.Lock()
	roots := append(append([]string{}, e.Roots...), e.State.ExtraRoots...)
	e.mu.Unlock()
	paths, warnings := Discover(roots)
	// ArtCraft's own install manifest catches portable/custom installs that are not in /Applications.
	manifest := filepath.Join(e.Home, "Library/Application Support/ArtCraft Launcher/installed.json")
	if b, err := ReadLimited(manifest, 4<<20); err == nil {
		var m struct {
			Apps map[string]struct {
				Path string `json:"path"`
			} `json:"apps"`
		}
		if json.Unmarshal(b, &m) == nil {
			for _, r := range m.Apps {
				if SafeApp(r.Path) == nil {
					paths = append(paths, r.Path)
				}
			}
		}
	}
	seen := map[string]bool{}
	unique := []string{}
	for _, p := range paths {
		if !seen[p] {
			seen[p] = true
			unique = append(unique, p)
		}
	}
	var apps []App
	var inspect []string
	e.mu.Lock()
	cached := make(map[string]App, len(e.Apps))
	for _, a := range e.Apps {
		cached[a.Path] = a
	}
	e.mu.Unlock()
	for _, path := range unique {
		old, ok := cached[path]
		if ok && old.InfoStamp != "" && old.InfoStamp == MetadataStamp(path) {
			apps = append(apps, old)
		} else {
			inspect = append(inspect, path)
		}
	}
	var fresh []App
	var err error
	if len(inspect) > 0 {
		if inspector, ok := e.Native.(interface {
			InspectWithWarnings(context.Context, []string) ([]App, []string, error)
		}); ok {
			var skipped []string
			fresh, skipped, err = inspector.InspectWithWarnings(ctx, inspect)
			warnings = append(warnings, skipped...)
		} else {
			fresh, err = e.Native.Inspect(ctx, inspect)
		}
		if err != nil {
			return nil, warnings, err
		}
	}
	apps = append(apps, fresh...)
	e.mu.Lock()
	defer e.mu.Unlock()
	e.Apps = map[string]App{}
	result := []App{}
	for _, a := range apps {
		if !seen[a.Path] || a.BundleID == "io.iconcraft.mac" {
			continue
		}
		a.ID = appID(a)
		a.ActiveRecord = ""
		a.AssetID = ""
		a.InfoStamp = MetadataStamp(a.Path)
		a.IconStamp = PreviewStamp(a.Path)
		a.Renamed = false
		for _, nr := range e.State.Renames {
			if nr.To == a.Path && nr.Status == "applied" {
				a.Renamed = true
				a.Name = nr.NewName
			}
		}
		a.Protected = Protected(a.Path)
		a.CraftID, a.Equivalent = e.Catalog.Match(a)
		if p, ok := e.State.Preferences[a.BundleID]; ok {
			a.AssetID = p.AssetID
		}
		for i := len(e.State.Records) - 1; i >= 0; i-- {
			r := e.State.Records[i]
			if r.App.ID == a.ID && isLive(r.Status) {
				a.ActiveRecord = r.ID
				break
			}
		}
		if a.ActiveRecord == "" {
			// The app moved or was replaced since the skin was applied, so its
			// path-based ID changed. The bundle ID is stable: re-link by it.
			for i := len(e.State.Records) - 1; i >= 0; i-- {
				r := e.State.Records[i]
				if r.App.BundleID == a.BundleID && a.BundleID != "" && isLive(r.Status) {
					a.ActiveRecord = r.ID
					break
				}
			}
		}
		e.Apps[a.ID] = a
		result = append(result, a)
	}
	sort.Slice(result, func(i, j int) bool {
		if (result[i].CraftID != "") != (result[j].CraftID != "") {
			return result[i].CraftID != ""
		}
		return strings.ToLower(result[i].Name) < strings.ToLower(result[j].Name)
	})
	if err := e.saveInventoryLocked(); err != nil {
		warnings = append(warnings, "Inventory cache: "+err.Error())
	}
	return result, warnings, nil
}
func isLive(s string) bool { return s == "applied" || s == "prepared" || s == "rollback-required" }
func (e *Engine) GetApp(id string) (App, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	a, ok := e.Apps[id]
	if !ok {
		return a, errors.New("application is not in the current scan")
	}
	return a, nil
}
func (e *Engine) AddRoot(root string) error {
	if !filepath.IsAbs(root) {
		return errors.New("choose an absolute folder")
	}
	root, err := filepath.EvalSymlinks(root)
	if err != nil {
		return err
	}
	fi, err := os.Stat(root)
	if err != nil || !fi.IsDir() {
		return errors.New("not a folder")
	}
	if root == "/" || root == e.Home || root == "/Volumes" {
		return errors.New("select an applications folder, not the whole disk or home folder")
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, r := range e.State.ExtraRoots {
		if r == root {
			return nil
		}
	}
	e.State.ExtraRoots = append(e.State.ExtraRoots, root)
	return e.save()
}
func (e *Engine) AssetPath(id string) (string, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.assetPath(id)
}
func (e *Engine) assetPath(id string) (string, error) {
	if _, ok := e.State.Assets[id]; !ok || len(id) != 64 {
		return "", errors.New("unknown icon asset")
	}
	p := filepath.Join(e.Dir, "assets", id+".png")
	b, err := ReadLimited(p, MaxIconBytes)
	if err != nil {
		return "", err
	}
	if hash(b) != id {
		return "", errors.New("stored icon checksum mismatch")
	}
	return p, nil
}
func (e *Engine) ImportPNG(name, source string, b []byte) (Asset, error) {
	if err := ValidatePNG(b); err != nil {
		return Asset{}, err
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.importPNGBytesLocked(name, source, b)
}

// importPNGBytesLocked stores b as a checksummed asset; caller holds e.mu.
func (e *Engine) importPNGBytesLocked(name, source string, b []byte) (Asset, error) {
	id := hash(b)
	a := Asset{ID: id, Name: name, Source: source, Created: now()}
	if existing, ok := e.State.Assets[id]; ok {
		return existing, nil
	}
	if err := AtomicWrite(filepath.Join(e.Dir, "assets", id+".png"), b, 0600); err != nil {
		return a, err
	}
	e.State.Assets[id] = a
	if err := e.save(); err != nil {
		delete(e.State.Assets, id)
		return a, err
	}
	return a, nil
}
func (e *Engine) ImportFile(ctx context.Context, p string) (Asset, error) {
	ext := strings.ToLower(filepath.Ext(p))
	if ext != ".png" && ext != ".icns" {
		return Asset{}, errors.New("select an original PNG or ICNS file")
	}
	if _, err := ReadLimited(p, MaxIconBytes); err != nil {
		return Asset{}, err
	}
	b, err := e.Native.Normalize(ctx, p)
	if err != nil {
		return Asset{}, err
	}
	return e.ImportPNG(filepath.Base(p), "User-supplied original icon; authenticity not independently verified", b)
}
func (e *Engine) CopyDonor(ctx context.Context, id string) (Asset, error) {
	a, err := e.GetApp(id)
	if err != nil {
		return Asset{}, err
	}
	b, err := e.Native.ExportIcon(ctx, a.Path, true)
	if err != nil {
		return Asset{}, err
	}
	return e.ImportPNG(a.Name+" icon", "Extracted from installed application: "+a.BundleID, b)
}
func (e *Engine) Apply(ctx context.Context, id, assetID string) (Record, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	a, ok := e.Apps[id]
	if !ok {
		return Record{}, errors.New("application not found; rescan")
	}
	if a.Protected {
		return Record{}, errors.New("system applications are read-only donors")
	}
	if err := SafeApp(a.Path); err != nil {
		return Record{}, err
	}
	p, err := e.assetPath(assetID)
	if err != nil {
		return Record{}, err
	}
	e.captureOriginalLocked(ctx, a) // save the original to the vault before first change
	fp, err := Fingerprint(a)
	if err != nil {
		return Record{}, err
	}
	for _, r := range e.State.Records {
		if r.App.ID == id && (r.Status == "prepared" || r.Status == "rollback-required") {
			return Record{}, errors.New("recover the interrupted change before applying another icon")
		}
	}
	signed, err := e.Native.Signature(ctx, a.Path)
	if err != nil {
		return Record{}, fmt.Errorf("original signature check failed: %w", err)
	}
	before, err := e.Native.Snapshot(a.Path)
	if err != nil {
		return Record{}, fmt.Errorf("could not back up icon metadata for %s: %w", a.Name, err)
	}
	r := Record{ID: newID(), App: a, Fingerprint: fp, AssetID: assetID, Created: now(), Status: "prepared", Before: before}
	e.State.Records = append(e.State.Records, r)
	idx := len(e.State.Records) - 1
	if err = e.save(); err != nil {
		e.State.Records = e.State.Records[:idx]
		return Record{}, fmt.Errorf("backup could not be committed; no change made: %w", err)
	}
	priorPreference, hadPreference := e.State.Preferences[a.BundleID]
	fail := func(cause error) (Record, error) {
		if hadPreference {
			e.State.Preferences[a.BundleID] = priorPreference
		} else {
			delete(e.State.Preferences, a.BundleID)
		}
		current, fe := Fingerprint(a)
		if fe != nil || current != fp {
			e.State.Records[idx].Status = "rollback-required"
			e.State.Records[idx].Error = "Application changed during operation; automatic rollback refused. " + cause.Error()
			_ = e.save()
			return e.State.Records[idx], errors.New(e.State.Records[idx].Error)
		}
		re := e.Native.Restore(context.Background(), a.Path, before)
		if re == nil {
			check, ce := e.Native.Snapshot(a.Path)
			if ce != nil {
				re = ce
			} else if !check.Equal(before) {
				re = errors.New("rollback verification did not match the original backup")
			}
		}
		status := "failed"
		message := cause.Error()
		if re != nil {
			status = "rollback-required"
			message += "; rollback failed: " + re.Error()
		}
		e.State.Records[idx].Status = status
		e.State.Records[idx].Error = message
		se := e.save()
		if se != nil {
			message += "; journal save failed: " + se.Error()
		}
		return e.State.Records[idx], errors.New(message)
	}
	if err = e.Native.Set(ctx, a.Path, p); err != nil {
		return fail(fmt.Errorf("could not set the icon for %s: %w", a.Name, err))
	}
	check, err := Fingerprint(a)
	if err != nil || check != fp {
		return fail(errors.New("application contents changed during icon application"))
	}
	if signed == "valid" {
		if status, er := e.Native.Signature(ctx, a.Path); er != nil || status != "valid" {
			return fail(errors.New("application code signature did not pass after the icon change"))
		}
	}
	after, err := e.Native.Snapshot(a.Path)
	if err != nil {
		return fail(err)
	}
	if !after.Custom() {
		return fail(errors.New("macOS did not record a custom icon"))
	}
	e.State.Records[idx].After = &after
	e.State.Records[idx].Status = "applied"
	e.State.Preferences[a.BundleID] = Preference{BundleID: a.BundleID, CraftID: a.CraftID, AssetID: assetID}
	if err = e.save(); err != nil {
		return fail(fmt.Errorf("could not save successful change: %w", err))
	}
	a.IconStamp = PreviewStamp(a.Path)
	e.Apps[a.ID] = a
	_ = e.saveInventoryLocked()
	return e.State.Records[idx], nil
}
func (e *Engine) Restore(ctx context.Context, recordID string) (Record, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	idx := -1
	for i, r := range e.State.Records {
		if r.ID == recordID {
			idx = i
			break
		}
	}
	if idx < 0 {
		return Record{}, errors.New("backup not found")
	}
	r := e.State.Records[idx]
	if !isLive(r.Status) {
		return r, errors.New("this backup is not active")
	}
	for i := idx + 1; i < len(e.State.Records); i++ {
		later := e.State.Records[i]
		if later.App.ID == r.App.ID && isLive(later.Status) {
			return r, errors.New("restore the newest change for this application first")
		}
	}
	fp, err := Fingerprint(r.App)
	if err != nil || fp != r.Fingerprint {
		return r, errors.New("application was updated, replaced or removed; old backup will not be written onto the new version")
	}
	current, err := e.Native.Snapshot(r.App.Path)
	if err != nil {
		return r, err
	}
	if r.After != nil && !current.Equal(*r.After) {
		return r, errors.New("the icon was changed outside IconCraft; restore stopped to preserve that change")
	}
	if err = e.Native.Restore(ctx, r.App.Path, r.Before); err != nil {
		return r, err
	}
	after, err := e.Native.Snapshot(r.App.Path)
	if err != nil {
		return r, err
	}
	if !after.Equal(r.Before) {
		return r, errors.New("restored icon metadata did not match the backup")
	}
	e.State.Records[idx].Status = "restored"
	e.State.Records[idx].Error = ""
	delete(e.State.Preferences, r.App.BundleID)
	for i := idx - 1; i >= 0; i-- {
		prev := e.State.Records[i]
		if prev.App.ID == r.App.ID && prev.Status == "applied" && prev.Fingerprint == r.Fingerprint {
			e.State.Preferences[r.App.BundleID] = Preference{BundleID: r.App.BundleID, CraftID: r.App.CraftID, AssetID: prev.AssetID}
			break
		}
	}
	if err = e.save(); err != nil {
		return r, err
	}
	if a, ok := e.Apps[r.App.ID]; ok {
		a.IconStamp = PreviewStamp(a.Path)
		e.Apps[a.ID] = a
		_ = e.saveInventoryLocked()
	}
	return e.State.Records[idx], nil
}

// ValidatePNG rejects truncated files and oversized decompression surfaces before persistence.
func ValidatePNG(b []byte) error {
	if len(b) < 24 || len(b) > MaxIconBytes {
		return errors.New("icon file is empty or exceeds 16 MiB")
	}
	cfg, err := png.DecodeConfig(bytes.NewReader(b))
	if err != nil {
		return fmt.Errorf("invalid PNG: %w", err)
	}
	if cfg.Width < 1 || cfg.Height < 1 || cfg.Width > 4096 || cfg.Height > 4096 || cfg.Width*cfg.Height > 16_777_216 {
		return errors.New("icon image dimensions exceed the 4096-pixel safety limit")
	}
	if _, err = png.Decode(bytes.NewReader(b)); err != nil {
		return fmt.Errorf("PNG pixel data is corrupt: %w", err)
	}
	return nil
}
