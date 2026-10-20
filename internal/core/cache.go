package core

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
)

// MetadataStamp invalidates the persisted inventory when Info.plist is replaced.
// It is a performance hint, never a substitute for SafeApp/Fingerprint on writes.
func MetadataStamp(path string) string {
	f, err := os.Stat(filepath.Join(path, "Contents/Info.plist"))
	if err != nil {
		return ""
	}
	return fmt.Sprintf("%d:%d", f.ModTime().UnixNano(), f.Size()) + changeTime(f)
}
func PreviewStamp(path string) string {
	v := MetadataStamp(path)
	for _, p := range []string{path, filepath.Join(path, "Icon\r")} {
		if f, err := os.Stat(p); err == nil {
			v += fmt.Sprintf(":%d:%d", f.ModTime().UnixNano(), f.Size()) + changeTime(f)
		}
	}
	return hash([]byte(v))[:20]
}
func (e *Engine) loadInventory() {
	b, err := ReadLimited(filepath.Join(e.Dir, "inventory.json"), 8<<20)
	if err != nil {
		return
	}
	var apps []App
	if json.Unmarshal(b, &apps) != nil || len(apps) > 3000 {
		return
	}
	for _, a := range apps {
		if a.ID != appID(a) || a.Executable == "" || a.BundleID == "" {
			continue
		}
		if fi, err := os.Lstat(a.Path); err != nil || !fi.IsDir() || fi.Mode()&os.ModeSymlink != 0 {
			continue
		}
		e.Apps[a.ID] = a
	}
}
func (e *Engine) saveInventoryLocked() error {
	apps := make([]App, 0, len(e.Apps))
	for _, a := range e.Apps {
		apps = append(apps, a)
	}
	sort.Slice(apps, func(i, j int) bool { return apps[i].Path < apps[j].Path })
	b, err := json.Marshal(apps)
	if err != nil {
		return err
	}
	return AtomicWrite(filepath.Join(e.Dir, "inventory.json"), b, 0600)
}
func (e *Engine) SetTheme(theme string) error {
	if theme != "system" && theme != "light" && theme != "dark" {
		return fmt.Errorf("invalid appearance")
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	old := e.State.Theme
	e.State.Theme = theme
	if err := e.save(); err != nil {
		e.State.Theme = old
		return err
	}
	return nil
}

// Session is intentionally small. It never serializes icon recovery payloads to
// the WebView, unlike SnapshotState, which is retained for local recovery tools.
func (e *Engine) Session() map[string]any {
	e.mu.Lock()
	defer e.mu.Unlock()
	apps := make([]App, 0, len(e.Apps))
	for _, a := range e.Apps {
		apps = append(apps, a)
	}
	assets := map[string]Asset{}
	for k, v := range e.State.Assets {
		assets[k] = v
	}
	originals := map[string]Original{}
	for k, v := range e.State.Originals {
		originals[k] = v
	}
	choices := map[string]string{}
	for k, v := range e.State.Choices {
		choices[k] = v
	}
	records := []map[string]any{}
	for _, r := range e.State.Records {
		if isLive(r.Status) {
			records = append(records, map[string]any{"appId": r.App.ID, "id": r.ID, "status": r.Status, "error": r.Error})
		}
	}
	renames := []RenameRecord{}
	for _, r := range e.State.Renames {
		if r.Status == "applied" || r.Status == "prepared" {
			renames = append(renames, r)
		}
	}
	theme := e.State.Theme
	if theme == "" {
		theme = "system"
	}
	return map[string]any{"version": Version, "supportUrl": SupportURL, "repositoryUrl": RepositoryURL, "apps": apps, "assets": assets, "originals": originals, "choices": choices, "records": records, "renames": renames, "theme": theme}
}

// Extended-attribute writes may change ctime without changing mtime on macOS.
func changeTime(info os.FileInfo) string {
	v := reflect.ValueOf(info.Sys())
	if v.Kind() == reflect.Pointer {
		v = v.Elem()
	}
	if !v.IsValid() || v.Kind() != reflect.Struct {
		return ""
	}
	for _, name := range []string{"Ctimespec", "Ctim"} {
		f := v.FieldByName(name)
		if f.IsValid() && f.CanInterface() {
			return fmt.Sprintf(":%v", f.Interface())
		}
	}
	return ""
}
