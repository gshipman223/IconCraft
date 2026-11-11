//go:build windows

package native

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func (w *Windows) taskFile() string {
	dir := w.TaskDir
	if dir == "" {
		dir = filepath.Join(w.Dir, "runtime")
	}
	return filepath.Join(dir, "keepapplied.txt")
}

func (w *Windows) KeepAgentEnabled() bool {
	if w.SkipServices || w.TaskDir != "" {
		_, err := os.Stat(w.taskFile())
		return err == nil
	}
	cmd := exec.Command("schtasks", "/Query", "/TN", "IconCraft Keep Applied")
	return cmd.Run() == nil
}

func (w *Windows) KeepAgentStatus() KeepAgentStatus {
	on := w.KeepAgentEnabled()
	return KeepAgentStatus{Installed: on, Loaded: on}
}

func (w *Windows) SetKeepAgent(enabled bool) error {
	if w.SkipServices || w.TaskDir != "" {
		path := w.taskFile()
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
		if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			return err
		}
		return os.WriteFile(path, []byte(exe+" --reapply\n"), 0600)
	}
	if !enabled {
		cmd := exec.Command("schtasks", "/Delete", "/F", "/TN", "IconCraft Keep Applied")
		if err := cmd.Run(); err != nil {
			return err
		}
		return nil
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	tr := `"` + exe + `" --reapply`
	cmd := exec.Command("schtasks", "/Create", "/F", "/SC", "MINUTE", "/MO", "3", "/TN", "IconCraft Keep Applied", "/TR", tr)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return errors.New(strings.TrimSpace(string(out)) + ": " + err.Error())
	}
	return nil
}

func (w *Windows) LoginAgentEnabled() bool {
	if w.RunFile != "" || w.SkipRegistry {
		_, err := os.Stat(w.RunFile)
		return w.RunFile != "" && err == nil
	}
	_, err := queryRunValue()
	return err == nil
}

func (w *Windows) SetLoginAgent(enabled, menuOnly bool) error {
	if w.RunFile != "" || w.SkipRegistry {
		if w.RunFile == "" {
			return errors.New("login item path is not set")
		}
		if !enabled {
			if err := os.Remove(w.RunFile); err != nil && !os.IsNotExist(err) {
				return err
			}
			return nil
		}
		exe, err := os.Executable()
		if err != nil {
			return err
		}
		line := quoteWindows(exe) + " --at-login"
		if menuOnly {
			line += " --menu-only"
		}
		return os.WriteFile(w.RunFile, []byte(line+"\n"), 0600)
	}
	if !enabled {
		return deleteRunValue()
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	line := quoteWindows(exe) + " --at-login"
	if menuOnly {
		line += " --menu-only"
	}
	return setRunValue(line)
}

func quoteWindows(s string) string {
	if strings.ContainsAny(s, " \t\"") {
		return `"` + strings.ReplaceAll(s, `"`, `\"`) + `"`
	}
	return s
}
