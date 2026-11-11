package core

import (
	"path/filepath"
	"strings"
)

// selfApp is IconCraft itself. Listing it would let a session skin the
// tool that is doing the skinning. The macOS bundle id stays first.
func selfApp(a App) bool {
	switch a.BundleID {
	case "io.iconcraft.mac", "io.iconcraft.IconCraft", "io.iconcraft.IconCraft.desktop":
		return true
	}
	base := strings.ToLower(filepath.Base(a.Path))
	switch base {
	case "iconcraft.lnk", "io.iconcraft.iconcraft.lnk", "io.iconcraft.iconcraft.desktop":
		return true
	}
	return false
}

// seenStamp is the value used to notice that an app's visible icon
// changed. Platforms that skin a launcher file can report their own
// stamp; everyone else, including macOS, keeps the Finder preview stamp.
func (e *Engine) seenStamp(a App) string {
	if s, ok := e.Native.(interface{ IconStamp(App) string }); ok {
		if v := s.IconStamp(a); v != "" {
			return v
		}
	}
	return PreviewStamp(a.Path)
}
