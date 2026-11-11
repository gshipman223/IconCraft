package main

import (
	"os"
	"path/filepath"
	"runtime"
)

// appDataDir is IconCraft's private folder. The macOS path is unchanged.
func appDataDir(home string) string {
	switch runtime.GOOS {
	case "darwin":
		return filepath.Join(home, "Library", "Application Support", "IconCraft")
	case "windows":
		if v := os.Getenv("APPDATA"); v != "" && filepath.IsAbs(v) {
			return filepath.Join(v, "IconCraft")
		}
		return filepath.Join(home, "AppData", "Roaming", "IconCraft")
	default:
		if v := os.Getenv("XDG_DATA_HOME"); v != "" && filepath.IsAbs(v) {
			return filepath.Join(v, "IconCraft")
		}
		return filepath.Join(home, ".local", "share", "IconCraft")
	}
}
