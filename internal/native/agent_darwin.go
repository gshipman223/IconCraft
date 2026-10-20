//go:build darwin

package native

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Keep-applied agent: a tiny LaunchAgent that runs `IconCraft --reapply`
// at login, every 3 minutes, and whenever /Applications changes. It repairs
// skins that third-party apps/updaters reverted (cleared Finder icon or
// restored folder name) without the main window ever opening. It never
// touches the inside of any application bundle.
const keepAgentLabel = "io.iconcraft.keepapplied"

func (m *Mac) keepAgentPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "Library/LaunchAgents", keepAgentLabel+".plist"), nil
}

func (m *Mac) KeepAgentEnabled() bool {
	p, err := m.keepAgentPath()
	if err != nil {
		return false
	}
	_, err = os.Stat(p)
	return err == nil
}

func xmlEscape(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;", "'", "&apos;")
	return r.Replace(s)
}

func (m *Mac) SetKeepAgent(enabled bool) error {
	p, err := m.keepAgentPath()
	if err != nil {
		return err
	}
	domain := fmt.Sprintf("gui/%d", os.Getuid())
	if !enabled {
		_ = exec.Command("/bin/launchctl", "bootout", domain+"/"+keepAgentLabel).Run()
		if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	logPath := filepath.Join(filepath.Dir(m.Dir), "keepalive.log")
	plist := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
<key>Label</key><string>%s</string>
<key>ProgramArguments</key><array><string>%s</string><string>--reapply</string></array>
<key>RunAtLoad</key><true/>
<key>StartInterval</key><integer>180</integer>
<key>WatchPaths</key><array><string>/Applications</string><string>%s</string></array>
<key>ProcessType</key><string>Background</string>
<key>StandardErrorPath</key><string>%s</string>
</dict></plist>
`, keepAgentLabel, xmlEscape(exe), xmlEscape(filepath.Join(home, "Applications")), xmlEscape(logPath))
	if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
		return err
	}
	if err := os.WriteFile(p, []byte(plist), 0600); err != nil {
		return err
	}
	_ = exec.Command("/bin/launchctl", "bootout", domain+"/"+keepAgentLabel).Run()
	if out, err := exec.Command("/bin/launchctl", "bootstrap", domain, p).CombinedOutput(); err != nil {
		return fmt.Errorf("could not start the keep-applied agent: %v: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}
