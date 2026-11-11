//go:build windows

package native

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gshipman223/IconCraft/internal/core"
	"github.com/gshipman223/IconCraft/internal/i18n"
	"github.com/gshipman223/IconCraft/internal/launchers"
)

// Windows skins an application by editing the user's shortcut, or by
// copying an all-users shortcut into the user's Start Menu and editing
// the copy. Program files and all-users shortcuts are not modified.
type Windows struct {
	Dir           string
	Home          string
	UserStart     string
	UserDesktop   string
	UserPinned    string
	CommonStart   string
	CommonDesktop string
	SkipStore     bool
	SkipServices  bool
	SkipRegistry  bool
	TaskDir       string
	RunFile       string

	mu         sync.Mutex
	storeNames map[string]string
}

const storeReadOnly = "Store apps are read-only. Windows does not allow another program to change their icon or name."

func newWindows(dir string) (*Windows, error) {
	if err := os.MkdirAll(filepath.Join(dir, "runtime"), 0700); err != nil {
		return nil, err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	appData := os.Getenv("APPDATA")
	if appData == "" || !filepath.IsAbs(appData) {
		appData = filepath.Join(home, "AppData", "Roaming")
	}
	programData := os.Getenv("ProgramData")
	if programData == "" || !filepath.IsAbs(programData) {
		programData = `C:\ProgramData`
	}
	public := os.Getenv("PUBLIC")
	if public == "" || !filepath.IsAbs(public) {
		public = `C:\Users\Public`
	}
	return &Windows{
		Dir:           dir,
		Home:          home,
		UserStart:     filepath.Join(appData, "Microsoft", "Windows", "Start Menu", "Programs"),
		UserDesktop:   filepath.Join(home, "Desktop"),
		UserPinned:    filepath.Join(appData, "Microsoft", "Internet Explorer", "Quick Launch", "User Pinned", "TaskBar"),
		CommonStart:   filepath.Join(programData, "Microsoft", "Windows", "Start Menu", "Programs"),
		CommonDesktop: filepath.Join(public, "Desktop"),
	}, nil
}

func (w *Windows) iconDir() string { return filepath.Join(w.Dir, "launchers") }

func (w *Windows) icoPath(target string) string {
	sum := sha256.Sum256([]byte(strings.ToLower(target)))
	return filepath.Join(w.iconDir(), hex.EncodeToString(sum[:8])+".ico")
}

func (w *Windows) ourIcon(icon string) bool {
	if icon == "" || !filepath.IsAbs(icon) {
		return false
	}
	dir := filepath.Clean(w.iconDir())
	clean := filepath.Clean(icon)
	return strings.EqualFold(clean, dir) || strings.HasPrefix(strings.ToLower(clean), strings.ToLower(dir)+`\`)
}

func (w *Windows) under(path, dir string) bool {
	if path == "" || dir == "" {
		return false
	}
	rel, err := filepath.Rel(dir, path)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator))
}

func (w *Windows) userOwned(path string) bool {
	if w.under(path, w.CommonStart) || w.under(path, w.CommonDesktop) {
		return false
	}
	return w.under(path, w.UserStart) || w.under(path, w.UserDesktop) || w.under(path, w.UserPinned)
}

func (w *Windows) rank(path string) int {
	switch {
	case w.under(path, w.UserStart):
		return 0
	case w.under(path, w.UserDesktop):
		return 1
	case w.under(path, w.UserPinned):
		return 2
	case w.under(path, w.CommonStart):
		return 3
	case w.under(path, w.CommonDesktop):
		return 4
	default:
		return 5
	}
}

func (w *Windows) Discover(roots []string) ([]string, []string) {
	type group struct {
		target string
		best   string
		rank   int
	}
	found := map[string]*group{}
	var warnings []string
	note := func(path string) {
		base := strings.ToLower(filepath.Base(path))
		if base == "iconcraft.lnk" || base == "io.iconcraft.iconcraft.lnk" {
			return
		}
		target, err := w.targetOf(path)
		if err != nil || target == "" {
			if err != nil {
				warnings = append(warnings, path+": "+err.Error())
			}
			return
		}
		key := strings.ToLower(target)
		g := found[key]
		if g == nil || w.rank(path) < g.rank {
			found[key] = &group{target: target, best: path, rank: w.rank(path)}
		}
	}
	walk := func(dir string) {
		if dir == "" {
			return
		}
		_ = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				warnings = append(warnings, p+": "+err.Error())
				return nil
			}
			if d.IsDir() {
				rel, _ := filepath.Rel(dir, p)
				if strings.Count(rel, string(os.PathSeparator)) > 6 {
					return fs.SkipDir
				}
				return nil
			}
			if strings.EqualFold(filepath.Ext(d.Name()), ".lnk") {
				note(p)
			}
			return nil
		})
	}
	for _, dir := range []string{w.UserStart, w.UserDesktop, w.UserPinned, w.CommonStart, w.CommonDesktop} {
		walk(dir)
	}
	known := map[string]bool{}
	for _, dir := range []string{w.UserStart, w.UserDesktop, w.UserPinned, w.CommonStart, w.CommonDesktop} {
		known[filepath.Clean(dir)] = true
	}
	for _, root := range roots {
		root = filepath.Clean(root)
		if root != "" && !known[root] {
			walk(root)
		}
	}
	out := make([]string, 0, len(found))
	for _, g := range found {
		out = append(out, g.best)
	}
	if !w.SkipStore {
		apps, err := listStoreApps(context.Background())
		if err != nil {
			warnings = append(warnings, "store apps: "+err.Error())
		} else {
			w.mu.Lock()
			w.storeNames = map[string]string{}
			for _, a := range apps {
				if a.Name == "" || a.AppID == "" {
					continue
				}
				id := "msix:" + a.AppID
				w.storeNames[id] = a.Name
				out = append(out, id)
			}
			w.mu.Unlock()
		}
	}
	sort.Strings(out)
	return out, warnings
}

func (w *Windows) targetOf(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	s, err := launchers.ReadShortcut(b)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(s.Target), nil
}

func (w *Windows) readShortcut(path string) (launchers.Shortcut, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return launchers.Shortcut{}, err
	}
	return launchers.ReadShortcut(b)
}

func (w *Windows) userShortcuts() []string {
	var out []string
	for _, dir := range []string{w.UserStart, w.UserDesktop, w.UserPinned} {
		_ = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
			if err != nil || d == nil || d.IsDir() {
				return nil
			}
			if strings.EqualFold(filepath.Ext(p), ".lnk") {
				out = append(out, p)
			}
			return nil
		})
	}
	return out
}

func (w *Windows) viewPath(path string) string {
	if strings.HasPrefix(strings.ToLower(path), "msix:") || w.userOwned(path) {
		return path
	}
	target, err := w.targetOf(path)
	if err != nil || target == "" {
		return path
	}
	best := ""
	bestRank := 99
	for _, p := range w.userShortcuts() {
		t, err := w.targetOf(p)
		if err != nil || !strings.EqualFold(t, target) {
			continue
		}
		if w.rank(p) < bestRank {
			best, bestRank = p, w.rank(p)
		}
	}
	if best != "" {
		return best
	}
	return path
}

func (w *Windows) editPath(path string) (string, error) {
	if w.userOwned(path) {
		return path, nil
	}
	if view := w.viewPath(path); w.userOwned(view) {
		return view, nil
	}
	if err := os.MkdirAll(w.UserStart, 0755); err != nil {
		return "", err
	}
	dest := filepath.Join(w.UserStart, filepath.Base(path))
	if _, err := os.Lstat(dest); os.IsNotExist(err) {
		b, err := os.ReadFile(path)
		if err != nil {
			return "", err
		}
		if err = writeNoSymlink(dest, b, 0644); err != nil {
			return "", err
		}
	} else if err != nil {
		return "", err
	}
	return dest, nil
}

func (w *Windows) Inspect(ctx context.Context, paths []string) ([]core.App, error) {
	apps, _, err := w.InspectWithWarnings(ctx, paths)
	return apps, err
}

func (w *Windows) InspectWithWarnings(ctx context.Context, paths []string) ([]core.App, []string, error) {
	var apps []core.App
	var warnings []string
	for _, p := range paths {
		if err := ctx.Err(); err != nil {
			return apps, warnings, err
		}
		a, err := w.inspectOne(p)
		if err != nil {
			warnings = append(warnings, p+": "+err.Error())
			continue
		}
		apps = append(apps, a)
	}
	return apps, warnings, nil
}

func (w *Windows) inspectOne(path string) (core.App, error) {
	if strings.HasPrefix(strings.ToLower(path), "msix:") {
		w.mu.Lock()
		name := w.storeNames[path]
		w.mu.Unlock()
		if name == "" {
			name = strings.TrimPrefix(path, "msix:")
		}
		return core.App{
			Name:       name,
			Path:       path,
			BundleID:   path,
			Executable: strings.TrimPrefix(path, "msix:"),
			Protected:  true,
			ReadOnly:   i18n.T(storeReadOnly),
		}, nil
	}
	target, err := w.targetOf(path)
	if err != nil {
		return core.App{}, err
	}
	if target == "" {
		return core.App{}, errors.New("shortcut has no target")
	}
	view := w.viewPath(path)
	name := strings.TrimSuffix(filepath.Base(view), filepath.Ext(view))
	sum := sha256.Sum256([]byte(strings.ToLower(target)))
	app := core.App{
		Name:       name,
		Path:       path,
		BundleID:   "lnk:" + hex.EncodeToString(sum[:])[:32],
		Executable: target,
	}
	if strings.Contains(strings.ToLower(target), `\windowsapps\`) {
		app.Protected = true
		app.ReadOnly = i18n.T(storeReadOnly)
	}
	app.IconStamp = w.IconStamp(app)
	return app, nil
}

func (w *Windows) refuseLocked(path string) error {
	if strings.HasPrefix(strings.ToLower(path), "msix:") {
		return errors.New(storeReadOnly)
	}
	target, err := w.targetOf(path)
	if err == nil && strings.Contains(strings.ToLower(target), `\windowsapps\`) {
		return errors.New(storeReadOnly)
	}
	return nil
}

func (w *Windows) Set(ctx context.Context, path, icon string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := w.refuseLocked(path); err != nil {
		return err
	}
	png, err := os.ReadFile(icon)
	if err != nil {
		return err
	}
	if err = core.ValidatePNG(png); err != nil {
		return err
	}
	ico, err := launchers.EncodeICO(png)
	if err != nil {
		return err
	}
	edit, err := w.editPath(path)
	if err != nil {
		return err
	}
	target, err := w.targetOf(edit)
	if err != nil {
		return err
	}
	dest := w.icoPath(target)
	if err = os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
		return err
	}
	if err = writeNoSymlink(dest, ico, 0644); err != nil {
		return err
	}
	return w.withUserShortcuts(target, edit, func(p string) error {
		return w.setIconFile(p, dest, 0)
	})
}

func (w *Windows) withUserShortcuts(target, must string, fn func(string) error) error {
	seen := map[string]bool{}
	paths := []string{}
	if must != "" && w.userOwned(must) {
		paths = append(paths, must)
		seen[strings.ToLower(must)] = true
	}
	for _, p := range w.userShortcuts() {
		if seen[strings.ToLower(p)] || !w.userOwned(p) {
			continue
		}
		t, err := w.targetOf(p)
		if err != nil || !strings.EqualFold(t, target) {
			continue
		}
		paths = append(paths, p)
	}
	if len(paths) == 0 {
		return errors.New("no user shortcut to update")
	}
	for _, p := range paths {
		if err := fn(p); err != nil {
			return err
		}
	}
	return nil
}

func (w *Windows) setIconFile(path, icon string, index int32) error {
	s, err := w.readShortcut(path)
	if err != nil {
		return err
	}
	if icon == "" {
		s.ClearIcon()
	} else {
		s.SetIcon(icon, index)
	}
	out, err := s.Bytes()
	if err != nil {
		return err
	}
	return writeNoSymlink(path, out, 0644)
}

func (w *Windows) Snapshot(path string) (core.Snapshot, error) {
	if strings.HasPrefix(strings.ToLower(path), "msix:") {
		return core.Snapshot{}, nil
	}
	view := w.viewPath(path)
	s, err := w.readShortcut(view)
	if err != nil {
		return core.Snapshot{}, err
	}
	attrs := map[string][]byte{"iconIndex": []byte(strconv.FormatInt(int64(s.IconIndex), 10))}
	if s.Icon != "" {
		attrs["iconLocation"] = []byte(s.Icon)
	}
	if !w.ourIcon(s.Icon) {
		return core.Snapshot{Attributes: attrs}, nil
	}
	data, err := os.ReadFile(s.Icon)
	if err != nil {
		return core.Snapshot{}, err
	}
	info := make([]byte, 32)
	info[8] = 4
	return core.Snapshot{Exists: true, Data: data, FinderInfo: info, Attributes: attrs}, nil
}

func (w *Windows) Restore(ctx context.Context, path string, snap core.Snapshot) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if strings.HasPrefix(strings.ToLower(path), "msix:") {
		return errors.New(storeReadOnly)
	}
	target, err := w.targetOf(w.viewPath(path))
	if err != nil {
		return err
	}
	if snap.Custom() && len(snap.Data) > 0 {
		dest := string(snap.Attributes["iconLocation"])
		if !w.ourIcon(dest) {
			dest = w.icoPath(target)
		}
		if err = os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
			return err
		}
		if err = writeNoSymlink(dest, snap.Data, 0644); err != nil {
			return err
		}
		idx := int32(0)
		return w.withUserShortcuts(target, w.viewPath(path), func(p string) error {
			return w.setIconFile(p, dest, idx)
		})
	}
	loc := string(snap.Attributes["iconLocation"])
	idx := int32(0)
	if n, err := strconv.ParseInt(string(snap.Attributes["iconIndex"]), 10, 32); err == nil {
		idx = int32(n)
	}
	return w.withUserShortcuts(target, w.viewPath(path), func(p string) error {
		if loc == "" || w.ourIcon(loc) {
			return w.setIconFile(p, "", 0)
		}
		return w.setIconFile(p, loc, idx)
	})
}

func (w *Windows) ApplyLauncherName(ctx context.Context, a core.App, name string) (core.App, error) {
	if err := ctx.Err(); err != nil {
		return a, err
	}
	if strings.ContainsAny(name, `*?"<>|`) {
		return a, errors.New(`app names on Windows cannot contain * ? " < > |`)
	}
	if err := w.refuseLocked(a.Path); err != nil {
		return a, err
	}
	src, err := w.editPath(a.Path)
	if err != nil {
		return a, err
	}
	dest := filepath.Join(filepath.Dir(src), name+".lnk")
	if !strings.EqualFold(src, dest) {
		if _, err := os.Lstat(dest); err == nil {
			return a, errors.New("an app with that name already exists; choose another name")
		}
		if err = os.Rename(src, dest); err != nil {
			return a, err
		}
	}
	out := a
	out.Path = dest
	out.Name = name
	return out, nil
}

func (w *Windows) RestoreLauncherName(ctx context.Context, a core.App, nr core.RenameRecord, oldName string) (core.App, error) {
	if err := ctx.Err(); err != nil {
		return a, err
	}
	if !w.userOwned(nr.From) && strings.EqualFold(oldName, strings.TrimSuffix(filepath.Base(nr.From), ".lnk")) {
		if w.userOwned(a.Path) {
			_ = os.Remove(a.Path)
		}
		out := a
		out.Path = nr.From
		out.Name = oldName
		return out, nil
	}
	dest := nr.From
	if !w.userOwned(dest) {
		dest = filepath.Join(filepath.Dir(a.Path), oldName+".lnk")
	}
	if w.userOwned(a.Path) && !strings.EqualFold(a.Path, dest) {
		if err := os.Rename(a.Path, dest); err != nil {
			return a, err
		}
	}
	out := a
	out.Path = dest
	out.Name = oldName
	return out, nil
}

func (w *Windows) LauncherRenameDest(a core.App, name string) string {
	src := w.viewPath(a.Path)
	dir := filepath.Dir(src)
	if !w.userOwned(src) {
		dir = w.UserStart
	}
	return filepath.Join(dir, name+".lnk")
}

func (w *Windows) LauncherNameIntact(nr core.RenameRecord) bool {
	if nr.To == "" {
		return false
	}
	base := strings.TrimSuffix(filepath.Base(nr.To), ".lnk")
	return strings.EqualFold(base, nr.NewName)
}

func (w *Windows) IconStamp(a core.App) string {
	if strings.HasPrefix(strings.ToLower(a.Path), "msix:") {
		return ""
	}
	view := w.viewPath(a.Path)
	s, err := w.readShortcut(view)
	if err != nil {
		return ""
	}
	parts := a.Name + "\x00" + s.Icon
	if s.Icon != "" {
		if fi, err := os.Stat(s.Icon); err == nil {
			parts += fmt.Sprintf("\x00%d:%d", fi.ModTime().UnixNano(), fi.Size())
		}
	}
	sum := sha256.Sum256([]byte(parts))
	return hex.EncodeToString(sum[:])[:20]
}

func (w *Windows) ExportIcon(ctx context.Context, path string, bundled bool) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if strings.HasPrefix(strings.ToLower(path), "msix:") {
		return nil, errors.New("store app has no exportable icon")
	}
	view := path
	if !bundled {
		view = w.viewPath(path)
	}
	s, err := w.readShortcut(view)
	if err != nil {
		return nil, err
	}
	if s.Icon == "" || (bundled && w.ourIcon(s.Icon)) {
		return nil, errors.New("shortcut has no separate original icon")
	}
	b, err := os.ReadFile(s.Icon)
	if err != nil {
		return nil, err
	}
	return launchers.ToPNG(b)
}

func (w *Windows) ExportThumbnails(ctx context.Context, paths []string) (map[string][]byte, error) {
	out := map[string][]byte{}
	var first error
	for _, p := range paths {
		b, err := w.ExportIcon(ctx, p, false)
		if err != nil {
			if first == nil {
				first = err
			}
			continue
		}
		out[p] = b
	}
	if len(out) == 0 && first != nil {
		return nil, first
	}
	return out, nil
}

func (w *Windows) Normalize(ctx context.Context, path string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > core.MaxIconBytes {
		return nil, errors.New("icon file is empty or exceeds 16 MiB")
	}
	return launchers.ToPNG(b)
}

func (w *Windows) Signature(ctx context.Context, path string) (string, error) {
	if strings.HasPrefix(strings.ToLower(path), "msix:") {
		return "unsigned", nil
	}
	if _, err := os.Stat(path); err != nil {
		return "", err
	}
	return "unsigned", nil
}

func (w *Windows) WriteProbe(string) error {
	if w.UserStart == "" {
		return errors.New("user Start Menu directory is not set")
	}
	if err := os.MkdirAll(w.UserStart, 0755); err != nil {
		return err
	}
	f, err := os.CreateTemp(w.UserStart, ".iconcraft-write-check-")
	if err != nil {
		return err
	}
	name := f.Name()
	_ = f.Close()
	return os.Remove(name)
}

func (w *Windows) Running(context.Context, string) (bool, error) { return false, nil }

func (w *Windows) Notify(context.Context, string) error { return nil }

func (w *Windows) RefreshIconCache(context.Context, []string) error { return nil }

func (w *Windows) UsesMacIconGrid() bool  { return false }
func (w *Windows) SupportsFullSkin() bool { return false }

func (w *Windows) Platform() PlatformInfo {
	return PlatformInfo{
		OS:                "windows",
		FullSkin:          false,
		RenameSuffix:      "",
		DefaultsMode:      "settings",
		TaglineKey:        "header.taglineWindows",
		NameHintKey:       "edit.nameHintWindows",
		LauncherNoteKey:   "edit.launcherNoteWindows",
		DefaultsIntroKey:  "defaults.introWindows",
		DefaultsNoteKey:   "defaults.noteWindows",
		HideDockSetting:   true,
		MenuBarLabelKey:   "settings.menuBarTray",
		MenuBarTextKey:    "settings.menuBarTrayText",
		MenuStartLabelKey: "settings.menuStartTray",
		MenuStartTextKey:  "settings.menuStartTrayText",
		NotificationsKey:  "settings.notificationsPortable",
		MaintenanceKey:    "settings.maintenanceWindows",
		FolderTitleKey:    "folder.titlePortable",
		ThemeTitleKey:     "theme.systemTitlePortable",
		SearchShortcut:    "Ctrl K",
		SetupNoteKey:      "setup.noteWindows",
		SetupSubtitleKey:  "setup.subtitlePortable",
	}
}

func (w *Windows) SelfSignature(context.Context) SignatureInfo {
	return SignatureInfo{Kind: "unsigned", Source: "Windows builds are not code-signed"}
}

func (w *Windows) SelfTest(ctx context.Context) Report {
	r := Report{
		Version: core.Version,
		OS:      runtime.GOOS,
		Arch:    runtime.GOARCH,
		Time:    time.Now().UTC().Format(time.RFC3339),
		Scope:   "Disposable shortcut fixtures only. The Start Menu, registry and scheduled tasks are not modified.",
	}
	add := func(name string, err error) {
		c := Check{Name: name, Passed: err == nil}
		if err != nil {
			c.Detail = err.Error()
		}
		r.Checks = append(r.Checks, c)
	}
	temp, err := os.MkdirTemp("", "iconcraft-windows-selftest-")
	add("Create isolated fixture", err)
	if err == nil {
		defer os.RemoveAll(temp)
		fix := fixtureWindows(temp)
		add("Shortcut round trip", fix.roundTrip(ctx))
		add("Start Menu directory is writable", fix.WriteProbe(""))
	}
	r.Passed = true
	for _, c := range r.Checks {
		if !c.Passed {
			r.Passed = false
		}
	}
	return r
}
