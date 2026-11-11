package launchers

import (
	"os"
	"path/filepath"
	"strings"
)

// ResolveIcon finds the file a desktop Icon= value points at. Absolute paths
// and theme names are both accepted. The search follows the hicolor fallback
// the desktop uses when no theme is selected, then pixmaps.
func ResolveIcon(icon string, roots []string) (string, error) {
	icon = strings.TrimSpace(icon)
	if icon == "" {
		return "", os.ErrNotExist
	}
	if strings.ContainsAny(icon, `/\`) {
		if filepath.IsAbs(icon) {
			if _, err := os.Stat(icon); err == nil {
				return icon, nil
			}
		}
		return "", os.ErrNotExist
	}
	name := icon
	ext := strings.ToLower(filepath.Ext(name))
	sizes := []string{"512x512", "256x256", "128x128", "64x64", "48x48", "32x32", "24x24", "22x22", "16x16", "scalable"}
	contexts := []string{"apps", "places", "mimetypes", "devices"}
	exts := []string{".png", ".svg", ".xpm"}
	if ext == ".png" || ext == ".svg" || ext == ".xpm" {
		exts = []string{ext}
		name = strings.TrimSuffix(name, ext)
	}
	var dirs []string
	for _, root := range roots {
		dirs = append(dirs,
			filepath.Join(root, "icons", "hicolor"),
			filepath.Join(root, "icons"),
			filepath.Join(root, "pixmaps"),
		)
	}
	for _, dir := range dirs {
		if strings.HasSuffix(dir, "pixmaps") {
			for _, e := range exts {
				p := filepath.Join(dir, name+e)
				if regularFile(p) {
					return p, nil
				}
			}
			continue
		}
		for _, size := range sizes {
			for _, ctx := range contexts {
				for _, e := range exts {
					p := filepath.Join(dir, size, ctx, name+e)
					if regularFile(p) {
						return p, nil
					}
				}
			}
		}
	}
	return "", os.ErrNotExist
}

func regularFile(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && fi.Mode().IsRegular() && fi.Size() > 0 && fi.Size() < 16<<20
}
