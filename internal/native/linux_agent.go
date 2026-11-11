//go:build linux

package native

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// systemctl is replaceable so tests never talk to the user service manager.
var systemctl = func(args ...string) error {
	bin, err := exec.LookPath("systemctl")
	if err != nil {
		return err
	}
	return exec.Command(bin, args...).Run()
}

func (l *Linux) unitPath(name string) string {
	return filepath.Join(l.SystemdUser, name)
}

func (l *Linux) KeepAgentEnabled() bool {
	_, err := os.Stat(l.unitPath("io.iconcraft.keepapplied.timer"))
	return err == nil
}

func (l *Linux) KeepAgentStatus() KeepAgentStatus {
	installed := l.KeepAgentEnabled()
	st := KeepAgentStatus{Installed: installed, Loaded: installed}
	if l.SkipServices {
		return st
	}
	if !installed {
		return st
	}
	if err := systemctl("--user", "is-active", "--quiet", "io.iconcraft.keepapplied.timer"); err != nil {
		st.Loaded = false
		st.Error = "the keep-applied timer is installed but not active"
	}
	return st
}

func (l *Linux) SetKeepAgent(enabled bool) error {
	if l.SystemdUser == "" {
		return errors.New("systemd user directory is not set")
	}
	timer := l.unitPath("io.iconcraft.keepapplied.timer")
	service := l.unitPath("io.iconcraft.keepapplied.service")
	if !enabled {
		if !l.SkipServices {
			_ = systemctl("--user", "disable", "--now", "io.iconcraft.keepapplied.timer")
		}
		_ = os.Remove(timer)
		_ = os.Remove(service)
		return nil
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	if err = os.MkdirAll(l.SystemdUser, 0755); err != nil {
		return err
	}
	svc := "[Unit]\nDescription=IconCraft keep-applied repair\n\n[Service]\nType=oneshot\nExecStart=" + quoteDesktopExec(exe, "--reapply") + "\n"
	tmr := "[Unit]\nDescription=IconCraft keep-applied timer\n\n[Timer]\nOnBootSec=1min\nOnUnitActiveSec=3min\nPersistent=true\n\n[Install]\nWantedBy=timers.target\n"
	if err = writeNoSymlink(service, []byte(svc), 0644); err != nil {
		return err
	}
	if err = writeNoSymlink(timer, []byte(tmr), 0644); err != nil {
		return err
	}
	if l.SkipServices {
		return nil
	}
	if err = systemctl("--user", "daemon-reload"); err != nil {
		return err
	}
	return systemctl("--user", "enable", "--now", "io.iconcraft.keepapplied.timer")
}

func (l *Linux) loginItem() string {
	return filepath.Join(l.AutostartDir, "io.iconcraft.IconCraft.desktop")
}

func (l *Linux) LoginAgentEnabled() bool {
	_, err := os.Stat(l.loginItem())
	return err == nil
}

func (l *Linux) SetLoginAgent(enabled, menuOnly bool) error {
	path := l.loginItem()
	if !enabled {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	args := []string{exe, "--at-login"}
	if menuOnly {
		args = append(args, "--menu-only")
	}
	body := "[Desktop Entry]\nType=Application\nName=IconCraft\nExec=" + quoteDesktopExec(args...) + "\nX-GNOME-Autostart-enabled=true\n"
	if err = os.MkdirAll(l.AutostartDir, 0755); err != nil {
		return err
	}
	return writeNoSymlink(path, []byte(body), 0644)
}

func quoteDesktopExec(args ...string) string {
	parts := make([]string, 0, len(args))
	for _, a := range args {
		if strings.ContainsAny(a, " \t\"") {
			parts = append(parts, `"`+strings.ReplaceAll(a, `"`, `\"`)+`"`)
		} else {
			parts = append(parts, a)
		}
	}
	return strings.Join(parts, " ")
}
