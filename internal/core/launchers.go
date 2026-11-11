package core

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/gshipman223/IconCraft/internal/launchers"
)

const (
	kindBundle = iota
	kindDesktop
	kindShortcut
)

// launcherKind reports how a path is skinned. macOS bundles stay the default
// for every other path, including .desktop and .lnk files when those are
// encountered on Darwin, so the Mac checks never change meaning.
func launcherKind(path string) int {
	ext := strings.ToLower(filepath.Ext(path))
	switch {
	case runtime.GOOS == "linux" && ext == ".desktop":
		return kindDesktop
	case runtime.GOOS == "windows" && ext == ".lnk":
		return kindShortcut
	default:
		return kindBundle
	}
}

func launcherFile(path string) bool {
	return launcherKind(path) != kindBundle
}

func safeDesktop(path string) error {
	if !filepath.IsAbs(path) || !strings.HasSuffix(strings.ToLower(path), ".desktop") {
		return errors.New("not an absolute desktop entry")
	}
	fi, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if fi.IsDir() {
		return errors.New("desktop entry is a directory")
	}
	if fi.Mode()&os.ModeSymlink != 0 {
		target, err := os.Stat(path)
		if err != nil {
			return err
		}
		if !target.Mode().IsRegular() || target.Size() == 0 || target.Size() > 1<<20 {
			return errors.New("desktop entry symlink does not point at a regular file")
		}
		return nil
	}
	if !fi.Mode().IsRegular() || fi.Size() == 0 || fi.Size() > 1<<20 {
		return errors.New("desktop entry is not a regular file")
	}
	return nil
}

func safeShortcut(path string) error {
	if !filepath.IsAbs(path) || !strings.EqualFold(filepath.Ext(path), ".lnk") {
		return errors.New("not an absolute shortcut")
	}
	fi, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if fi.IsDir() {
		return errors.New("shortcut is a directory")
	}
	info := fi
	if fi.Mode()&os.ModeSymlink != 0 {
		info, err = os.Stat(path)
		if err != nil {
			return err
		}
	}
	if !info.Mode().IsRegular() || info.Size() == 0 || info.Size() > 1<<20 {
		return errors.New("shortcut is not a regular file")
	}
	return nil
}

// fingerprintDesktop hashes the desktop id and the upstream Exec line.
// Name and Icon are deliberately not part of it: rewriting the user
// override must not look like the application was replaced.
func fingerprintDesktop(a App) (string, error) {
	if err := safeDesktop(a.Path); err != nil {
		return "", err
	}
	b, err := os.ReadFile(a.Path)
	if err != nil {
		return "", err
	}
	e, err := launchers.ParseDesktop(b)
	if err != nil {
		return "", err
	}
	execLine := e.Get("Exec")
	if src := e.Get("X-IconCraft-Source"); src != "" && safeDesktop(src) == nil && src != a.Path {
		if sb, err := os.ReadFile(src); err == nil {
			if se, err := launchers.ParseDesktop(sb); err == nil && se.Get("Exec") != "" {
				execLine = se.Get("Exec")
			}
		}
	}
	if execLine == "" && !e.Bool("DBusActivatable") {
		return "", errors.New("launcher has no command")
	}
	sum := sha256.Sum256([]byte(a.BundleID + "\x00" + execLine))
	return hex.EncodeToString(sum[:]), nil
}

// fingerprintShortcut hashes the shortcut target, not its file name, so a
// rename of the .lnk does not look like a different application.
func fingerprintShortcut(a App) (string, error) {
	if err := safeShortcut(a.Path); err != nil {
		return "", err
	}
	b, err := os.ReadFile(a.Path)
	if err != nil {
		return "", err
	}
	s, err := launchers.ReadShortcut(b)
	if err != nil {
		return "", err
	}
	target := strings.TrimSpace(s.Target)
	if target == "" {
		return "", errors.New("shortcut has no target")
	}
	sum := sha256.Sum256([]byte(strings.ToLower(target)))
	return hex.EncodeToString(sum[:]), nil
}
