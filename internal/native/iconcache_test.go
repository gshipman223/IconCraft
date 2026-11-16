package native

import (
	"os"
	"path/filepath"
	"testing"
)

func TestClearUserIconCachesRemovesOnlyTheIconServicesStore(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "home", "Library", "Caches")
	darwin := filepath.Join(root, "var", "folders", "ab", "cdef", "C")
	for _, dir := range []string{
		filepath.Join(home, "com.apple.iconservices.store"),
		filepath.Join(home, "com.apple.iconservicesagent"),
		filepath.Join(home, "com.apple.iconservices"),
		filepath.Join(home, "keep-me"),
		filepath.Join(darwin, "com.apple.iconservices"),
		filepath.Join(darwin, "com.apple.iconservices.store"),
	} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	outside := filepath.Join(root, "outside")
	if err := os.WriteFile(outside, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(home, "com.apple.iconservices.store.link")); err != nil {
		t.Fatal(err)
	}
	// A symlink wearing the cache's name must not be followed.
	if err := os.RemoveAll(filepath.Join(home, "com.apple.iconservicesagent")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(home, "com.apple.iconservicesagent")); err != nil {
		t.Fatal(err)
	}

	clearUserIconCaches(home, darwin)

	if _, err := os.Stat(filepath.Join(home, "com.apple.iconservices.store")); !os.IsNotExist(err) {
		t.Fatal("user IconServices store was not removed")
	}
	if _, err := os.Stat(filepath.Join(darwin, "com.apple.iconservices")); !os.IsNotExist(err) {
		t.Fatal("per-user IconServices cache was not removed")
	}
	for _, keep := range []string{
		filepath.Join(home, "com.apple.iconservices"),
		filepath.Join(home, "keep-me"),
		filepath.Join(darwin, "com.apple.iconservices.store"),
		outside,
	} {
		if _, err := os.Stat(keep); err != nil {
			t.Fatalf("removed or lost %s: %v", keep, err)
		}
	}
	if _, err := os.Lstat(filepath.Join(home, "com.apple.iconservicesagent")); err != nil {
		t.Fatal("symlink named like the agent cache was removed")
	}
}

func TestSafeUserIconCacheRejectsBroadParents(t *testing.T) {
	for _, parent := range []string{"", "/", "/var", "/Library/Caches", "/tmp", "relative/caches"} {
		if _, ok := safeUserIconCache(parent, "com.apple.iconservices.store"); ok {
			t.Fatalf("accepted parent %q", parent)
		}
	}
	if _, ok := safeUserIconCache(t.TempDir(), "com.apple.finder"); ok {
		t.Fatal("accepted a cache name that is not IconServices")
	}
	if validDarwinUserCacheDir("/var/folders\n/etc/passwd") != "" {
		t.Fatal("accepted a cache dir with a newline")
	}
	if validDarwinUserCacheDir("/Library/Caches") != "" {
		t.Fatal("accepted a system cache dir")
	}
	if validDarwinUserCacheDir("/var/folders/ab/cdef/C") == "" {
		t.Fatal("rejected the documented per-user cache dir")
	}
	home := filepath.Join(t.TempDir(), "me")
	if validHomeIconCacheDir(home, filepath.Join(home, "Library", "Caches")) == "" {
		t.Fatal("rejected ~/Library/Caches")
	}
	if validHomeIconCacheDir(home, filepath.Join(home, "Library")) != "" {
		t.Fatal("accepted a parent of the cache dir")
	}
}
