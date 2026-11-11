//go:build linux

package native

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	"image/png"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/gshipman223/IconCraft/internal/core"
	"github.com/gshipman223/IconCraft/internal/i18n"
	"github.com/gshipman223/IconCraft/internal/launchers"
)

// Linux skins applications by writing a per-user desktop override.
// The system file and the program it starts are never modified.
type Linux struct {
	Dir          string
	Home         string
	UserApps     string
	DataDirs     []string
	MimeApps     string
	AutostartDir string
	SystemdUser  string
	IconRoots    []string
	SkipServices bool
}

var storytellerID = regexp.MustCompile(`ai\.storyteller\.[A-Za-z0-9._-]+`)

func newLinux(dir string) (*Linux, error) {
	if err := os.MkdirAll(filepath.Join(dir, "runtime"), 0700); err != nil {
		return nil, err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	dataHome := xdgPath("XDG_DATA_HOME", filepath.Join(home, ".local", "share"))
	configHome := xdgPath("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	roots := xdgDataDirs(home)
	return &Linux{
		Dir:          dir,
		Home:         home,
		UserApps:     filepath.Join(dataHome, "applications"),
		DataDirs:     roots,
		MimeApps:     filepath.Join(configHome, "mimeapps.list"),
		AutostartDir: filepath.Join(configHome, "autostart"),
		SystemdUser:  filepath.Join(configHome, "systemd", "user"),
		IconRoots:    append([]string{dataHome}, roots...),
	}, nil
}

func xdgPath(env, fallback string) string {
	if v := os.Getenv(env); v != "" && filepath.IsAbs(v) {
		return v
	}
	return fallback
}

func xdgDataDirs(home string) []string {
	v := os.Getenv("XDG_DATA_DIRS")
	if v == "" {
		v = "/usr/local/share:/usr/share"
	}
	var out []string
	seen := map[string]bool{}
	add := func(p string) {
		if p == "" || !filepath.IsAbs(p) || seen[p] {
			return
		}
		seen[p] = true
		out = append(out, p)
	}
	for _, p := range strings.Split(v, ":") {
		add(p)
	}
	add("/var/lib/flatpak/exports/share")
	add(filepath.Join(home, ".local/share/flatpak/exports/share"))
	add("/var/lib/snapd/desktop")
	return out
}

func (l *Linux) appDirs() []string {
	var out []string
	if l.UserApps != "" {
		out = append(out, l.UserApps)
	}
	for _, root := range l.DataDirs {
		out = append(out, filepath.Join(root, "applications"))
	}
	return out
}

func (l *Linux) iconDir() string { return filepath.Join(l.Dir, "launchers") }

func (l *Linux) iconPath(id string) string {
	base := strings.TrimSuffix(id, ".desktop")
	return filepath.Join(l.iconDir(), base+".png")
}

func (l *Linux) overridePath(id string) string { return filepath.Join(l.UserApps, id) }

func (l *Linux) ourIcon(icon string) bool {
	if icon == "" || !filepath.IsAbs(icon) {
		return false
	}
	dir := filepath.Clean(l.iconDir())
	clean := filepath.Clean(icon)
	return clean == dir || strings.HasPrefix(clean, dir+string(os.PathSeparator))
}

func (l *Linux) launcherID(path string) (string, bool) {
	for _, dir := range l.appDirs() {
		if id, ok := launchers.DesktopID(dir, path); ok {
			return id, true
		}
	}
	base := filepath.Base(path)
	if strings.HasSuffix(base, ".desktop") && base != ".desktop" && !strings.Contains(base, "..") {
		return base, true
	}
	return "", false
}

func (l *Linux) Discover(roots []string) ([]string, []string) {
	type pair struct{ system, user string }
	found := map[string]*pair{}
	var warnings []string
	note := func(path string, user bool) {
		if len(found) > 3000 {
			return
		}
		id, ok := l.launcherID(path)
		if !ok || id == "io.iconcraft.IconCraft.desktop" {
			return
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			warnings = append(warnings, path+": "+err.Error())
			return
		}
		e, err := launchers.ParseDesktop(raw)
		if err != nil || !e.TypeOK() {
			return
		}
		slot := found[id]
		if slot == nil {
			slot = &pair{}
			found[id] = slot
		}
		if user {
			if slot.user == "" {
				slot.user = path
			}
			return
		}
		if slot.system == "" {
			slot.system = path
		}
	}
	walk := func(dir string, user bool) {
		if dir == "" {
			return
		}
		err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				warnings = append(warnings, p+": "+err.Error())
				return nil
			}
			if d.IsDir() {
				if p != dir && strings.HasPrefix(d.Name(), ".") {
					return fs.SkipDir
				}
				rel, _ := filepath.Rel(dir, p)
				if strings.Count(rel, string(os.PathSeparator)) > 5 {
					return fs.SkipDir
				}
				return nil
			}
			if strings.HasSuffix(strings.ToLower(d.Name()), ".desktop") {
				note(p, user)
			}
			return nil
		})
		if err != nil {
			warnings = append(warnings, err.Error())
		}
	}
	walk(l.UserApps, true)
	for _, root := range l.DataDirs {
		walk(filepath.Join(root, "applications"), false)
	}
	known := map[string]bool{}
	for _, dir := range l.appDirs() {
		known[filepath.Clean(dir)] = true
	}
	for _, root := range roots {
		root = filepath.Clean(root)
		if root == "" || known[root] {
			continue
		}
		walk(root, false)
	}
	out := make([]string, 0, len(found))
	for _, slot := range found {
		if slot.system != "" {
			out = append(out, slot.system)
		} else if slot.user != "" {
			out = append(out, slot.user)
		}
	}
	sort.Strings(out)
	return out, warnings
}

func (l *Linux) Inspect(ctx context.Context, paths []string) ([]core.App, error) {
	apps, _, err := l.InspectWithWarnings(ctx, paths)
	return apps, err
}

func (l *Linux) InspectWithWarnings(ctx context.Context, paths []string) ([]core.App, []string, error) {
	var apps []core.App
	var warnings []string
	for _, p := range paths {
		if err := ctx.Err(); err != nil {
			return apps, warnings, err
		}
		a, err := l.inspectOne(p)
		if err != nil {
			warnings = append(warnings, p+": "+err.Error())
			continue
		}
		apps = append(apps, a)
	}
	return apps, warnings, nil
}

func (l *Linux) inspectOne(path string) (core.App, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return core.App{}, err
	}
	e, err := launchers.ParseDesktop(raw)
	if err != nil {
		return core.App{}, err
	}
	if !e.TypeOK() {
		return core.App{}, errors.New("not an application launcher")
	}
	id, ok := l.launcherID(path)
	if !ok {
		return core.App{}, errors.New("desktop file has no id")
	}
	name := displayName(e)
	icon := e.Get("Icon")
	execLine := e.Get("Exec")
	if ob, err := os.ReadFile(l.overridePath(id)); err == nil {
		if oe, err := launchers.ParseDesktop(ob); err == nil {
			if n := displayName(oe); n != "" {
				name = n
			}
			if oe.Get("Icon") != "" {
				icon = oe.Get("Icon")
			}
		}
	}
	bundle := id
	if m := storytellerID.FindString(string(raw) + "\n" + execLine); m != "" {
		bundle = m
	}
	app := core.App{
		Name:       name,
		Path:       path,
		BundleID:   bundle,
		Version:    e.Get("Version"),
		Executable: execLine,
	}
	if app.Executable == "" {
		app.Executable = id
	}
	app.IconStamp = l.IconStamp(app)
	_ = icon
	return app, nil
}

func displayName(e launchers.Entry) string {
	lang := os.Getenv("LANG")
	if i := strings.IndexAny(lang, ".@"); i >= 0 {
		lang = lang[:i]
	}
	short := lang
	if i := strings.Index(lang, "_"); i >= 0 {
		short = lang[:i]
	}
	g := e.Desktop()
	if g == nil {
		return ""
	}
	var exact, base, plain string
	for _, f := range g.Keys {
		if f.Key != "Name" {
			continue
		}
		switch {
		case f.Locale == "" && plain == "":
			plain = f.Value
		case lang != "" && strings.EqualFold(f.Locale, lang):
			exact = f.Value
		case short != "" && strings.EqualFold(f.Locale, short):
			base = f.Value
		}
	}
	if exact != "" {
		return exact
	}
	if base != "" {
		return base
	}
	return plain
}

func (l *Linux) readEntry(path string) (launchers.Entry, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return launchers.Entry{}, err
	}
	return launchers.ParseDesktop(b)
}

// entryForEdit returns the user override, creating its contents from the
// upstream file when the override does not exist yet. Exec is refreshed
// from the upstream file so a package update is what the launcher runs.
func (l *Linux) entryForEdit(path string) (launchers.Entry, string, error) {
	id, ok := l.launcherID(path)
	if !ok {
		return launchers.Entry{}, "", errors.New("not a desktop entry")
	}
	upstream := path
	if path == l.overridePath(id) {
		if cur, err := l.readEntry(path); err == nil && cur.Get("X-IconCraft-Source") != "" {
			upstream = cur.Get("X-IconCraft-Source")
		}
	}
	var e launchers.Entry
	if ob, err := os.ReadFile(l.overridePath(id)); err == nil {
		parsed, perr := launchers.ParseDesktop(ob)
		if perr != nil {
			return launchers.Entry{}, "", perr
		}
		e = parsed
	} else if !os.IsNotExist(err) {
		return launchers.Entry{}, "", err
	} else {
		src := path
		if path == l.overridePath(id) {
			src = upstream
		}
		parsed, perr := l.readEntry(src)
		if perr != nil {
			return launchers.Entry{}, "", perr
		}
		e = parsed
	}
	if upstream != "" && upstream != l.overridePath(id) {
		if ue, err := l.readEntry(upstream); err == nil && ue.Get("Exec") != "" {
			e.Set("Exec", ue.Get("Exec"))
		}
		e.Set("X-IconCraft-Source", upstream)
	}
	e.Set("X-IconCraft-Managed", "true")
	return e, id, nil
}

func (l *Linux) writeOverride(id string, e launchers.Entry) error {
	if err := os.MkdirAll(l.UserApps, 0755); err != nil {
		return err
	}
	return writeNoSymlink(l.overridePath(id), e.Bytes(), 0644)
}

func (l *Linux) Set(ctx context.Context, path, icon string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	png, err := os.ReadFile(icon)
	if err != nil {
		return err
	}
	if err = core.ValidatePNG(png); err != nil {
		return err
	}
	e, id, err := l.entryForEdit(path)
	if err != nil {
		return err
	}
	dest := l.iconPath(id)
	if err = os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
		return err
	}
	if err = writeNoSymlink(dest, png, 0644); err != nil {
		return err
	}
	sum := sha256.Sum256(png)
	e.Set("Icon", dest)
	e.Set("X-IconCraft-IconSHA256", hex.EncodeToString(sum[:]))
	if err = l.writeOverride(id, e); err != nil {
		return err
	}
	l.refreshDesktop()
	return nil
}

func (l *Linux) Snapshot(path string) (core.Snapshot, error) {
	id, ok := l.launcherID(path)
	if !ok {
		return core.Snapshot{}, errors.New("not a desktop entry")
	}
	b, err := os.ReadFile(l.overridePath(id))
	if err != nil {
		if os.IsNotExist(err) {
			return core.Snapshot{}, nil
		}
		return core.Snapshot{}, err
	}
	e, err := launchers.ParseDesktop(b)
	if err != nil {
		return core.Snapshot{}, err
	}
	icon := e.Get("Icon")
	if !l.ourIcon(icon) {
		return core.Snapshot{}, nil
	}
	png, err := os.ReadFile(icon)
	if err != nil {
		return core.Snapshot{}, nil
	}
	info := make([]byte, 32)
	info[8] = 4
	return core.Snapshot{Exists: true, Data: png, FinderInfo: info}, nil
}

func (l *Linux) Restore(ctx context.Context, path string, snap core.Snapshot) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	id, ok := l.launcherID(path)
	if !ok {
		return errors.New("not a desktop entry")
	}
	if !snap.Custom() || len(snap.Data) == 0 {
		return l.clearIcon(path, id)
	}
	dest := l.iconPath(id)
	if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
		return err
	}
	if err := writeNoSymlink(dest, snap.Data, 0644); err != nil {
		return err
	}
	e, _, err := l.entryForEdit(path)
	if err != nil {
		return err
	}
	sum := sha256.Sum256(snap.Data)
	e.Set("Icon", dest)
	e.Set("X-IconCraft-IconSHA256", hex.EncodeToString(sum[:]))
	if err = l.writeOverride(id, e); err != nil {
		return err
	}
	l.refreshDesktop()
	return nil
}

func (l *Linux) clearIcon(path, id string) error {
	ov := l.overridePath(id)
	e, err := l.readEntry(ov)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	upstreamName, upstreamIcon := l.upstreamIdentity(path, e)
	if l.ourIcon(e.Get("Icon")) {
		_ = os.Remove(e.Get("Icon"))
	}
	if upstreamIcon != "" {
		e.Set("Icon", upstreamIcon)
	}
	e.Del("X-IconCraft-IconSHA256")
	if e.Get("Name") == "" || e.Get("Name") == upstreamName {
		return os.Remove(ov)
	}
	return l.writeOverride(id, e)
}

func (l *Linux) upstreamIdentity(path string, e launchers.Entry) (name, icon string) {
	src := e.Get("X-IconCraft-Source")
	if src == "" && path != "" {
		id, _ := l.launcherID(path)
		if path != l.overridePath(id) {
			src = path
		}
	}
	if src == "" {
		return "", ""
	}
	ue, err := l.readEntry(src)
	if err != nil {
		return "", ""
	}
	return ue.Get("Name"), ue.Get("Icon")
}

func (l *Linux) ApplyLauncherName(ctx context.Context, a core.App, name string) (core.App, error) {
	if err := ctx.Err(); err != nil {
		return a, err
	}
	e, id, err := l.entryForEdit(a.Path)
	if err != nil {
		return a, err
	}
	e.Del("Name")
	e.Set("Name", name)
	if err = l.writeOverride(id, e); err != nil {
		return a, err
	}
	l.refreshDesktop()
	out := a
	out.Name = name
	return out, nil
}

func (l *Linux) RestoreLauncherName(ctx context.Context, a core.App, _ core.RenameRecord, oldName string) (core.App, error) {
	if err := ctx.Err(); err != nil {
		return a, err
	}
	e, id, err := l.entryForEdit(a.Path)
	if err != nil {
		if os.IsNotExist(err) {
			out := a
			out.Name = oldName
			return out, nil
		}
		return a, err
	}
	upstreamName, _ := l.upstreamIdentity(a.Path, e)
	e.Del("Name")
	e.Set("Name", oldName)
	if (oldName == upstreamName || oldName == "") && !l.ourIcon(e.Get("Icon")) {
		if err = os.Remove(l.overridePath(id)); err != nil && !os.IsNotExist(err) {
			return a, err
		}
	} else if err = l.writeOverride(id, e); err != nil {
		return a, err
	}
	l.refreshDesktop()
	out := a
	out.Name = oldName
	out.Path = a.Path
	return out, nil
}

func (l *Linux) LauncherRenameDest(a core.App, _ string) string { return a.Path }

func (l *Linux) LauncherNameIntact(nr core.RenameRecord) bool {
	path := nr.To
	if path == "" {
		path = nr.From
	}
	a, err := l.inspectOne(path)
	if err != nil {
		return false
	}
	return a.Name == nr.NewName
}

func (l *Linux) IconStamp(a core.App) string {
	id, ok := l.launcherID(a.Path)
	if !ok {
		return ""
	}
	parts := a.Name
	icon := ""
	if e, err := l.readEntry(l.overridePath(id)); err == nil {
		icon = e.Get("Icon")
		if n := displayName(e); n != "" {
			parts = n
		}
	} else if e, err := l.readEntry(a.Path); err == nil {
		icon = e.Get("Icon")
	}
	parts += "\x00" + icon
	if icon != "" {
		if resolved, err := launchers.ResolveIcon(icon, l.iconRoots()); err == nil {
			if fi, err := os.Stat(resolved); err == nil {
				parts += fmt.Sprintf("\x00%d:%d", fi.ModTime().UnixNano(), fi.Size())
			}
		}
	}
	sum := sha256.Sum256([]byte(parts))
	return hex.EncodeToString(sum[:])[:20]
}

func (l *Linux) iconRoots() []string {
	if len(l.IconRoots) > 0 {
		return l.IconRoots
	}
	return l.DataDirs
}

func (l *Linux) ExportIcon(ctx context.Context, path string, bundled bool) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	e, err := l.readEntry(path)
	if err != nil {
		return nil, err
	}
	if !bundled {
		if id, ok := l.launcherID(path); ok {
			if oe, err := l.readEntry(l.overridePath(id)); err == nil && oe.Get("Icon") != "" {
				e = oe
			}
		}
	}
	resolved, err := launchers.ResolveIcon(e.Get("Icon"), l.iconRoots())
	if err != nil {
		return nil, err
	}
	b, err := os.ReadFile(resolved)
	if err != nil {
		return nil, err
	}
	return launchers.ToPNG(b)
}

func (l *Linux) ExportThumbnails(ctx context.Context, paths []string) (map[string][]byte, error) {
	out := map[string][]byte{}
	var first error
	for _, p := range paths {
		b, err := l.ExportIcon(ctx, p, false)
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

func (l *Linux) Normalize(ctx context.Context, path string) ([]byte, error) {
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

func (l *Linux) Signature(ctx context.Context, path string) (string, error) {
	if _, err := os.Stat(path); err != nil {
		return "", err
	}
	return "unsigned", nil
}

func (l *Linux) WriteProbe(string) error {
	if l.UserApps == "" {
		return errors.New("user applications directory is not set")
	}
	if err := os.MkdirAll(l.UserApps, 0755); err != nil {
		return err
	}
	f, err := os.CreateTemp(l.UserApps, ".iconcraft-write-check-")
	if err != nil {
		return err
	}
	name := f.Name()
	_ = f.Close()
	return os.Remove(name)
}

func (l *Linux) Running(ctx context.Context, path string) (bool, error) {
	e, err := l.readEntry(path)
	if err != nil {
		return false, err
	}
	return processMatches(e.Get("Exec")), nil
}

func processMatches(execLine string) bool {
	bin := launchers.ExecBinary(execLine)
	if bin == "" {
		return false
	}
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return false
	}
	self := os.Getpid()
	want := bin
	for _, ent := range entries {
		pid, err := strconvAtoi(ent.Name())
		if err != nil || pid == self || pid == 1 {
			continue
		}
		base := filepath.Join("/proc", ent.Name())
		if dest, err := os.Readlink(filepath.Join(base, "exe")); err == nil {
			if dest == want || filepath.Base(dest) == filepath.Base(want) {
				return true
			}
		}
		cmd, err := os.ReadFile(filepath.Join(base, "cmdline"))
		if err != nil || len(cmd) == 0 {
			continue
		}
		first := string(cmd)
		if i := strings.IndexByte(first, 0); i >= 0 {
			first = first[:i]
		}
		if first == want || filepath.Base(first) == filepath.Base(want) {
			return true
		}
		if strings.Contains(execLine, "flatpak") && strings.Contains(string(cmd), want) && want != "flatpak" {
			return true
		}
	}
	return false
}

func strconvAtoi(s string) (int, error) {
	n := 0
	if s == "" {
		return 0, errors.New("empty")
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, errors.New("not a pid")
		}
		n = n*10 + int(c-'0')
	}
	return n, nil
}

func (l *Linux) Choose(ctx context.Context, kind string) (string, error) {
	title := i18n.T("Choose an original icon")
	switch kind {
	case "folder":
		title = i18n.T("Choose a folder")
	case "profile":
		title = i18n.T("Choose an IconCraft profile")
	}
	if bin, err := exec.LookPath("zenity"); err == nil {
		args := []string{"--file-selection", "--title=" + title}
		if kind == "folder" {
			args = append(args, "--directory")
		} else if kind != "profile" {
			args = append(args, "--file-filter=Images | *.png *.jpg *.jpeg *.gif *.ico *.svg *.icns")
		}
		return chooserOutput(ctx, bin, args...)
	}
	if bin, err := exec.LookPath("kdialog"); err == nil {
		args := []string{"--getopenfilename", ".", title}
		if kind == "folder" {
			args = []string{"--getexistingdirectory", ".", title}
		}
		return chooserOutput(ctx, bin, args...)
	}
	return "", errors.New("could not open the file chooser: zenity or kdialog is not installed")
}

func chooserOutput(ctx context.Context, bin string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, bin, args...)
	out, err := cmd.Output()
	if err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) && exit.ExitCode() == 1 {
			return "", nil
		}
		return "", fmt.Errorf("could not open the file chooser: %v", err)
	}
	return strings.TrimSpace(string(out)), nil
}

func (l *Linux) OpenURL(ctx context.Context, url string) error {
	if url != core.SupportURL && url != core.RepositoryURL && url != core.AppManagementURL {
		return errors.New("external link is not on IconCraft's allowlist")
	}
	return l.start("xdg-open", url)
}

func (l *Linux) OpenFolder(ctx context.Context, path string) error {
	if path == "" {
		return errors.New("folder path is empty")
	}
	return l.start("xdg-open", path)
}

func (l *Linux) OpenDefaultSettings(ctx context.Context) error {
	return l.start("xdg-open", l.MimeApps)
}

func (l *Linux) start(name string, args ...string) error {
	bin, err := exec.LookPath(name)
	if err != nil {
		return err
	}
	cmd := exec.Command(bin, args...)
	return cmd.Start()
}

func (l *Linux) Notify(ctx context.Context, path string) error {
	return l.RefreshIconCache(ctx, []string{path})
}

func (l *Linux) RefreshIconCache(context.Context, []string) error {
	l.refreshDesktop()
	return nil
}

func (l *Linux) refreshDesktop() {
	if l.UserApps == "" {
		return
	}
	if bin, err := exec.LookPath("update-desktop-database"); err == nil {
		_ = exec.Command(bin, l.UserApps).Run()
	}
}

func (l *Linux) UsesMacIconGrid() bool  { return false }
func (l *Linux) SupportsFullSkin() bool { return false }

func (l *Linux) Platform() PlatformInfo {
	return PlatformInfo{
		OS:                "linux",
		FullSkin:          false,
		RenameSuffix:      "",
		DefaultsMode:      "set",
		TaglineKey:        "header.taglineLinux",
		NameHintKey:       "edit.nameHintLinux",
		LauncherNoteKey:   "edit.launcherNoteLinux",
		DefaultsIntroKey:  "defaults.introLinux",
		DefaultsNoteKey:   "defaults.noteLinux",
		HideDockSetting:   true,
		MenuBarLabelKey:   "settings.menuBarTray",
		MenuBarTextKey:    "settings.menuBarTrayText",
		MenuStartLabelKey: "settings.menuStartTray",
		MenuStartTextKey:  "settings.menuStartTrayText",
		NotificationsKey:  "settings.notificationsPortable",
		MaintenanceKey:    "settings.maintenancePortable",
		FolderTitleKey:    "folder.titlePortable",
		ThemeTitleKey:     "theme.systemTitlePortable",
		SearchShortcut:    "Ctrl K",
		SetupNoteKey:      "setup.noteLinux",
		SetupSubtitleKey:  "setup.subtitlePortable",
	}
}

func (l *Linux) SelfSignature(context.Context) SignatureInfo {
	return SignatureInfo{Kind: "unsigned", Source: "Linux builds are not code-signed"}
}

func (l *Linux) SelfTest(ctx context.Context) Report {
	r := Report{
		Version: core.Version,
		OS:      runtime.GOOS,
		Arch:    runtime.GOARCH,
		Time:    time.Now().UTC().Format(time.RFC3339),
		Scope:   "Disposable launcher fixtures only. Installed applications, mimeapps and systemd are not modified.",
	}
	add := func(name string, err error) {
		c := Check{Name: name, Passed: err == nil}
		if err != nil {
			c.Detail = err.Error()
		}
		r.Checks = append(r.Checks, c)
	}
	temp, err := os.MkdirTemp("", "iconcraft-linux-selftest-")
	add("Create isolated fixture", err)
	if err == nil {
		defer os.RemoveAll(temp)
		fix := fixtureLinux(temp)
		add("Desktop override round trip", fix.roundTrip(ctx))
		add("Launcher directory is writable", fix.WriteProbe(""))
	}
	r.Passed = true
	for _, c := range r.Checks {
		if !c.Passed {
			r.Passed = false
			break
		}
	}
	return r
}

func fixtureLinux(root string) *Linux {
	sys := filepath.Join(root, "share")
	return &Linux{
		Dir:          filepath.Join(root, "data"),
		Home:         root,
		UserApps:     filepath.Join(root, "user-apps"),
		DataDirs:     []string{sys},
		MimeApps:     filepath.Join(root, "mimeapps.list"),
		AutostartDir: filepath.Join(root, "autostart"),
		SystemdUser:  filepath.Join(root, "systemd"),
		IconRoots:    []string{sys},
		SkipServices: true,
	}
}

func (l *Linux) roundTrip(ctx context.Context) error {
	apps := filepath.Join(l.DataDirs[0], "applications")
	iconDir := filepath.Join(l.DataDirs[0], "icons", "hicolor", "256x256", "apps")
	if err := os.MkdirAll(apps, 0755); err != nil {
		return err
	}
	if err := os.MkdirAll(iconDir, 0755); err != nil {
		return err
	}
	png := mustTinyPNG()
	if err := os.WriteFile(filepath.Join(iconDir, "photocraft.png"), png, 0644); err != nil {
		return err
	}
	desktop := "[Desktop Entry]\nType=Application\nName=PhotoCraft\nExec=/usr/bin/photocraft %u\nIcon=photocraft\n"
	sysPath := filepath.Join(apps, "photocraft.desktop")
	if err := os.WriteFile(sysPath, []byte(desktop), 0644); err != nil {
		return err
	}
	before, err := os.ReadFile(sysPath)
	if err != nil {
		return err
	}
	found, _ := l.Discover(nil)
	if len(found) != 1 || found[0] != sysPath {
		return fmt.Errorf("discover = %v", found)
	}
	iconFile := filepath.Join(l.Dir, "new.png")
	if err := os.MkdirAll(l.Dir, 0700); err != nil {
		return err
	}
	if err := os.WriteFile(iconFile, mustTinyPNG(), 0644); err != nil {
		return err
	}
	if err := l.Set(ctx, sysPath, iconFile); err != nil {
		return err
	}
	after, err := os.ReadFile(sysPath)
	if err != nil {
		return err
	}
	if string(after) != string(before) {
		return errors.New("system desktop file was modified")
	}
	snap, err := l.Snapshot(sysPath)
	if err != nil {
		return err
	}
	if !snap.Custom() {
		return errors.New("override was not recorded as a custom icon")
	}
	user, err := os.ReadFile(l.overridePath("photocraft.desktop"))
	if err != nil {
		return err
	}
	if !strings.Contains(string(user), "Icon="+l.iconPath("photocraft.desktop")) {
		return errors.New("user override is missing the icon path")
	}
	if _, err := l.ApplyLauncherName(ctx, core.App{Path: sysPath, Name: "PhotoCraft", BundleID: "photocraft.desktop", Executable: "/usr/bin/photocraft %u"}, "Darkroom"); err != nil {
		return err
	}
	named, _ := os.ReadFile(l.overridePath("photocraft.desktop"))
	if !strings.Contains(string(named), "Name=Darkroom") {
		return errors.New("rename did not set Name=")
	}
	if again, _ := os.ReadFile(sysPath); string(again) != string(before) {
		return errors.New("rename modified the system desktop file")
	}
	if _, err := l.RestoreLauncherName(ctx, core.App{Path: sysPath, Name: "Darkroom"}, core.RenameRecord{}, "PhotoCraft"); err != nil {
		return err
	}
	empty := core.Snapshot{}
	if err := l.Restore(ctx, sysPath, empty); err != nil {
		return err
	}
	cleared, err := l.Snapshot(sysPath)
	if err != nil {
		return err
	}
	if cleared.Custom() || cleared.Exists {
		return errors.New("restore left a custom icon in place")
	}
	return nil
}

func mustTinyPNG() []byte {
	img := image.NewNRGBA(image.Rect(0, 0, 8, 8))
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		panic(err)
	}
	return buf.Bytes()
}
