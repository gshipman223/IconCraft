package native

import (
	"context"
	_ "embed"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/gshipman223/IconCraft/internal/core"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

//go:embed bridge.js
var bridge []byte

//go:embed window.js
var window []byte

type Mac struct {
	Dir   string
	slots chan struct{}
}

func New(dir string) (*Mac, error) {
	if err := os.MkdirAll(filepath.Join(dir, "runtime"), 0700); err != nil {
		return nil, err
	}
	m := &Mac{Dir: filepath.Join(dir, "runtime"), slots: make(chan struct{}, 3)}
	for name, b := range map[string][]byte{"bridge.js": bridge, "window.js": window} {
		if err := core.AtomicWrite(filepath.Join(m.Dir, name), b, 0600); err != nil {
			return nil, err
		}
	}
	return m, nil
}
func (m *Mac) call(ctx context.Context, request any, result any) error {
	if runtime.GOOS != "darwin" {
		return errors.New("native macOS operations require macOS; this process is running on " + runtime.GOOS)
	}
	select {
	case m.slots <- struct{}{}:
		defer func() { <-m.slots }()
	case <-ctx.Done():
		return ctx.Err()
	}
	b, err := json.Marshal(request)
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(m.Dir, "request-*.json")
	if err != nil {
		return err
	}
	p := f.Name()
	defer os.Remove(p)
	if _, err = f.Write(b); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	timeout := 60 * time.Second
	if req, ok := request.(map[string]string); ok && req["action"] == "choose" {
		timeout = 5 * time.Minute
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "/usr/bin/osascript", "-l", "JavaScript", filepath.Join(m.Dir, "bridge.js"), p)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("native operation failed: %v: %s", err, stderr.String())
	}
	if len(out) > 32<<20 {
		return errors.New("native response exceeded size limit")
	}
	var response struct {
		OK     bool            `json:"ok"`
		Result json.RawMessage `json:"result"`
		Error  string          `json:"error"`
	}
	if err = json.Unmarshal(out, &response); err != nil {
		return fmt.Errorf("invalid native response: %w", err)
	}
	if !response.OK {
		return errors.New(response.Error)
	}
	if result != nil {
		return json.Unmarshal(response.Result, result)
	}
	return nil
}

// Inspect keeps the Native interface stable for callers that do not need warnings.
func (m *Mac) Inspect(ctx context.Context, paths []string) ([]core.App, error) {
	apps, _, err := m.InspectWithWarnings(ctx, paths)
	return apps, err
}

// InspectWithWarnings isolates malformed app bundles and retains diagnostic context.
func (m *Mac) InspectWithWarnings(ctx context.Context, paths []string) ([]core.App, []string, error) {
	out := []core.App{}
	warnings := []string{}
	for i := 0; i < len(paths); i += 150 {
		end := i + 150
		if end > len(paths) {
			end = len(paths)
		}
		var batch struct {
			Apps     []core.App `json:"apps"`
			Warnings []string   `json:"warnings"`
		}
		if err := m.call(ctx, map[string]any{"action": "inspect", "paths": paths[i:end]}, &batch); err != nil {
			return nil, warnings, fmt.Errorf("macOS application scan: %w", err)
		}
		out = append(out, batch.Apps...)
		warnings = append(warnings, batch.Warnings...)
	}
	for _, warning := range warnings {
		log.Printf("scan: %s", warning)
	}
	return out, warnings, nil
}
func (m *Mac) Set(ctx context.Context, path, icon string) error {
	return m.call(ctx, map[string]string{"action": "set", "path": path, "icon": icon}, nil)
}
func (m *Mac) ExportIcon(ctx context.Context, path string, bundled bool) ([]byte, error) {
	var text string
	err := m.call(ctx, map[string]any{"action": "export", "path": path, "bundled": bundled}, &text)
	if err != nil {
		return nil, err
	}
	return base64.StdEncoding.DecodeString(text)
}
func (m *Mac) Normalize(ctx context.Context, path string) ([]byte, error) {
	var text string
	err := m.call(ctx, map[string]string{"action": "normalize", "path": path}, &text)
	if err != nil {
		return nil, err
	}
	return base64.StdEncoding.DecodeString(text)
}
func (m *Mac) Choose(ctx context.Context, kind string) (string, error) {
	var p string
	err := m.call(ctx, map[string]string{"action": "choose", "kind": kind}, &p)
	return p, err
}

// OpenURL opens one of IconCraft's two fixed external links in the default
// browser. Arbitrary URLs from the page are refused here and in bridge.js.
func (m *Mac) OpenURL(ctx context.Context, url string) error {
	if url != core.SupportURL && url != core.RepositoryURL {
		return errors.New("external link is not on IconCraft's allowlist")
	}
	return m.call(ctx, map[string]string{"action": "openurl", "url": url}, nil)
}

// BundleIconFile returns the bundle's CFBundleIconFile base name ("" when the
// app uses an asset catalog instead — those can't take a replacement .icns).
func (m *Mac) BundleIconFile(ctx context.Context, path string) (string, error) {
	if runtime.GOOS != "darwin" {
		return "", errors.New("bundle tools require macOS")
	}
	cctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	out, err := exec.CommandContext(cctx, "/usr/bin/plutil", "-extract", "CFBundleIconFile", "raw", "-o", "-", "--", filepath.Join(path, "Contents/Info.plist")).Output()
	if err != nil {
		return "", nil // missing key is a normal outcome, not a failure
	}
	return strings.TrimSpace(string(out)), nil
}

// BundleName returns the app's current internal display name
// (CFBundleDisplayName, falling back to CFBundleName), "" when unset.
// Used to save the original internal name in the vault before a full skin.
func (m *Mac) BundleName(ctx context.Context, path string) (string, error) {
	if runtime.GOOS != "darwin" {
		return "", errors.New("bundle tools require macOS")
	}
	plist := filepath.Join(path, "Contents/Info.plist")
	for _, key := range []string{"CFBundleDisplayName", "CFBundleName"} {
		cctx, cancel := context.WithTimeout(ctx, 15*time.Second)
		out, err := exec.CommandContext(cctx, "/usr/bin/plutil", "-extract", key, "raw", "-o", "-", "--", plist).Output()
		cancel()
		if err == nil {
			if s := strings.TrimSpace(string(out)); s != "" {
				return s, nil
			}
		}
	}
	return "", nil
}

// SetBundleName sets CFBundleName/CFBundleDisplayName inside the bundle.
// Only used by the opt-in internal rebrand, with a journalled backup.
func (m *Mac) SetBundleName(ctx context.Context, path, name string) error {
	if runtime.GOOS != "darwin" {
		return errors.New("bundle tools require macOS")
	}
	plist := filepath.Join(path, "Contents/Info.plist")
	for _, key := range []string{"CFBundleName", "CFBundleDisplayName"} {
		if err := plistBuddy(ctx, plist, "Set", key, name); err != nil {
			if err := plistBuddy(ctx, plist, "Add", key, name); err != nil {
				return fmt.Errorf("could not set %s: %w", key, err)
			}
		}
	}
	return nil
}

func plistBuddy(ctx context.Context, plist, verb, key, value string) error {
	cctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	command := "Set :" + key + " " + value
	if verb == "Add" {
		command = "Add :" + key + " string " + value
	}
	cmd := exec.CommandContext(cctx, "/usr/libexec/PlistBuddy", "-c", command, plist)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// Resign ad-hoc re-signs a bundle whose contents IconCraft changed under a
// journalled internal rebrand. This intentionally replaces any previous
// signature; that trade-off is disclosed in the UI before the user opts in.
//
// codesign refuses to sign a bundle carrying Finder custom-icon detritus
// (root Icon\r file + com.apple.FinderInfo), which a prior Finder skin —
// ours or anyone's — leaves behind. That data is redundant once the bundle
// icon itself is replaced, so it is removed first; if a Finder skin is
// later restored, the keeper re-applies it from its own journal.
func (m *Mac) Resign(ctx context.Context, path string) error {
	if runtime.GOOS != "darwin" {
		return errors.New("bundle tools require macOS")
	}
	_ = os.Remove(filepath.Join(path, "Icon\r"))
	for _, attr := range []string{"com.apple.FinderInfo", "com.apple.ResourceFork"} {
		xctx, xcancel := context.WithTimeout(ctx, 10*time.Second)
		_ = exec.CommandContext(xctx, "/usr/bin/xattr", "-d", attr, path).Run()
		xcancel()
	}
	cctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(cctx, "/usr/bin/codesign", "--force", "--deep", "--sign", "-", "--timestamp=none", path)
	cmd.Env = append(os.Environ(), "LC_ALL=C")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("re-sign failed: %v: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func (m *Mac) Notify(ctx context.Context, path string) error {
	return m.call(ctx, map[string]string{"action": "notify", "path": path}, nil)
}
func (m *Mac) Signature(ctx context.Context, path string) (string, error) {
	if runtime.GOOS != "darwin" {
		return "", errors.New("signature verification requires macOS")
	}
	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	// No --strict: a Finder custom icon (Icon\r + com.apple.FinderInfo xattr —
	// exactly what this app sets) is "detritus not allowed" under --strict, so
	// --strict would fail every app that already has, or has just received, a
	// custom icon. --deep still verifies the sealed executable and resources.
	cmd := exec.CommandContext(ctx, "/usr/bin/codesign", "--verify", "--deep", path)
	cmd.Env = append(os.Environ(), "LC_ALL=C")
	out, err := cmd.CombinedOutput()
	if err == nil {
		return "valid", nil
	}
	if strings.Contains(string(out), "code object is not signed at all") {
		return "unsigned", nil
	}
	return "", fmt.Errorf("codesign: %s", strings.TrimSpace(string(out)))
}
func (m *Mac) Window(ctx context.Context, url, icon string) error {
	if runtime.GOOS != "darwin" {
		return errors.New("native window requires macOS")
	}
	cmd := exec.CommandContext(ctx, "/usr/bin/osascript", "-l", "JavaScript", filepath.Join(m.Dir, "window.js"), url, icon)
	cmd.Stdout = os.Stdout
	return runWindowCommand(cmd, log.Writer())
}

// ExportThumbnails obtains many small, current icons with one AppKit startup.
func (m *Mac) ExportThumbnails(ctx context.Context, paths []string) (map[string][]byte, error) {
	var raw map[string]string
	if err := m.call(ctx, map[string]any{"action": "thumbnails", "paths": paths}, &raw); err != nil {
		return nil, err
	}
	out := map[string][]byte{}
	for p, v := range raw {
		b, err := base64.StdEncoding.DecodeString(v)
		if err == nil {
			out[p] = b
		}
	}
	return out, nil
}
func (m *Mac) Running(ctx context.Context, path string) (bool, error) {
	var b bool
	err := m.call(ctx, map[string]string{"action": "running", "path": path}, &b)
	return b, err
}
