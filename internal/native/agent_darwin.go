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

func parseKeepAgentDisabled(output string) bool {
	for _, line := range strings.Split(output, "\n") {
		if !strings.Contains(line, keepAgentLabel) {
			continue
		}
		return strings.Contains(line, "=> true") || strings.Contains(line, "= true")
	}
	return false
}

// KeepAgentStatus observes the keeper without changing it. launchd's own
// records distinguish an installed-but-not-loaded or user-disabled helper
// from one that is ready, which a plist-existence check alone cannot do.
func (m *Mac) KeepAgentStatus() KeepAgentStatus {
	st := KeepAgentStatus{Installed: m.KeepAgentEnabled()}
	domain := fmt.Sprintf("gui/%d", os.Getuid())
	if out, err := exec.Command("/bin/launchctl", "print", domain+"/"+keepAgentLabel).CombinedOutput(); err == nil && strings.Contains(string(out), keepAgentLabel) {
		st.Loaded = true
	}
	if out, err := exec.Command("/bin/launchctl", "print-disabled", domain).CombinedOutput(); err == nil {
		st.Disabled = parseKeepAgentDisabled(string(out))
	} else if st.Installed {
		st.Error = "Could not read the macOS background-item state."
	}
	return st
}

// Launch-at-login agent: opens the IconCraft GUI when the user logs in
// (optionally straight into the menu bar). Kept separate from the
// keep-applied agent — persistence of skins never depended on the GUI,
// and this one never runs unless the user turns it on.
const loginAgentLabel = "io.iconcraft.login"

func (m *Mac) loginAgentPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "Library/LaunchAgents", loginAgentLabel+".plist"), nil
}

func (m *Mac) LoginAgentEnabled() bool {
	p, err := m.loginAgentPath()
	if err != nil {
		return false
	}
	_, err = os.Stat(p)
	return err == nil
}

func (m *Mac) SetLoginAgent(enabled, menuOnly bool) error {
	p, err := m.loginAgentPath()
	if err != nil {
		return err
	}
	domain := fmt.Sprintf("gui/%d", os.Getuid())
	if !enabled {
		_ = exec.Command("/bin/launchctl", "bootout", domain+"/"+loginAgentLabel).Run()
		if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	args := "<string>" + xmlEscape(exe) + "</string><string>--at-login</string>"
	if menuOnly {
		args += "<string>--menu-only</string>"
	}
	plist := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
<key>Label</key><string>%s</string>
<key>ProgramArguments</key><array>%s</array>
<key>RunAtLoad</key><true/>
<key>ProcessType</key><string>Interactive</string>
</dict></plist>
`, loginAgentLabel, args)
	if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
		return err
	}
	// No bootstrap: the agent fires at next login. Loading it now would
	// just bounce off the running instance's lock.
	return os.WriteFile(p, []byte(plist), 0600)
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
