package native

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// Names IconCraft will remove, and only as a direct child of a cache
// directory this process already decided is the current user's.
const (
	iconCacheStore = "com.apple.iconservices.store"
	iconCacheAgent = "com.apple.iconservicesagent"
	iconCacheTemp  = "com.apple.iconservices"
)

// clearUserIconCaches removes the current user's IconServices stores.
// homeCache is ~/Library/Caches. darwinCache is the directory from
// getconf DARWIN_USER_CACHE_DIR (/var/folders/…/C). Either may be empty.
// A symlink is left in place. A missing directory is not an error.
func clearUserIconCaches(homeCache, darwinCache string) {
	for _, name := range []string{iconCacheStore, iconCacheAgent} {
		if p, ok := safeUserIconCache(homeCache, name); ok {
			_ = os.RemoveAll(p)
		}
	}
	if p, ok := safeUserIconCache(darwinCache, iconCacheTemp); ok {
		_ = os.RemoveAll(p)
	}
}

// safeUserIconCache reports the directory parent/name when that path is
// a real directory (not a symlink) whose resolved location is exactly
// that child. name must be one of the three IconServices cache names.
func safeUserIconCache(parent, name string) (string, bool) {
	switch name {
	case iconCacheStore, iconCacheAgent, iconCacheTemp:
	default:
		return "", false
	}
	if !allowedIconCacheParent(parent) || strings.Contains(name, "..") {
		return "", false
	}
	parent = filepath.Clean(parent)
	parentInfo, err := os.Lstat(parent)
	if err != nil || !parentInfo.IsDir() || parentInfo.Mode()&os.ModeSymlink != 0 {
		return "", false
	}
	candidate := filepath.Join(parent, name)
	info, err := os.Lstat(candidate)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return "", false
	}
	resolvedParent, err := filepath.EvalSymlinks(parent)
	if err != nil {
		return "", false
	}
	resolved, err := filepath.EvalSymlinks(candidate)
	if err != nil {
		return "", false
	}
	rel, err := filepath.Rel(resolvedParent, resolved)
	if err != nil || rel != name {
		return "", false
	}
	return candidate, true
}

func allowedIconCacheParent(parent string) bool {
	if parent == "" || strings.Contains(parent, "..") {
		return false
	}
	parent = filepath.Clean(parent)
	if !filepath.IsAbs(parent) {
		return false
	}
	switch parent {
	case "/", "/var", "/private", "/private/var", "/var/folders", "/private/var/folders",
		"/Library", "/Library/Caches", "/System", "/System/Library", "/System/Library/Caches",
		"/tmp", "/private/tmp":
		return false
	}
	return true
}

// homeIconCacheDir is ~/Library/Caches, or "" when this process cannot
// prove the cache directory is that folder inside the current home.
func homeIconCacheDir() string {
	cache, err := os.UserCacheDir()
	if err != nil {
		return ""
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return validHomeIconCacheDir(home, cache)
}

func validHomeIconCacheDir(home, cache string) string {
	home = filepath.Clean(home)
	cache = filepath.Clean(cache)
	if !filepath.IsAbs(home) || !filepath.IsAbs(cache) {
		return ""
	}
	rel, err := filepath.Rel(home, cache)
	if err != nil {
		return ""
	}
	if rel != filepath.Join("Library", "Caches") {
		return ""
	}
	if !allowedIconCacheParent(cache) {
		return ""
	}
	return cache
}

// darwinUserCacheDir is the per-user cache from
// confstr(_CS_DARWIN_USER_CACHE_DIR). IconServices keeps
// com.apple.iconservices there. Anything that is not /var/folders/…/C
// is ignored.
func darwinUserCacheDir(ctx context.Context) string {
	if runtime.GOOS != "darwin" {
		return ""
	}
	cctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(cctx, "/usr/bin/getconf", "DARWIN_USER_CACHE_DIR").Output()
	if err != nil {
		return ""
	}
	return validDarwinUserCacheDir(string(out))
}

func validDarwinUserCacheDir(raw string) string {
	dir := strings.TrimSpace(raw)
	if strings.ContainsAny(dir, "\r\n") || strings.Contains(dir, "..") {
		return ""
	}
	dir = filepath.Clean(dir)
	if !filepath.IsAbs(dir) {
		return ""
	}
	marker := "/var/folders/"
	i := strings.Index(dir, marker)
	if i < 0 {
		return ""
	}
	rest := dir[i+len(marker):]
	parts := strings.Split(rest, "/")
	if len(parts) != 3 || parts[0] == "" || parts[1] == "" || parts[2] != "C" {
		return ""
	}
	if !allowedIconCacheParent(dir) {
		return ""
	}
	return dir
}
