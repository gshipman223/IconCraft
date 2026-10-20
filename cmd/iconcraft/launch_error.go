package main

import (
	"context"
	"fmt"
	"github.com/gshipman223/IconCraft/internal/core"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"
)

var privateSession = regexp.MustCompile(`http://127\.0\.0\.1:[0-9]+/[a-f0-9]{64}/?`)

func launchErrorText(err error) string {
	msg := privateSession.ReplaceAllString(err.Error(), "[private local interface]")
	if len(msg) > 32<<10 {
		msg = msg[len(msg)-(32<<10):]
	}
	return fmt.Sprintf("IconCraft %s — launch error\n%s\nPlatform: %s/%s\n\n%s\n\nSettings, original icons and recovery history were not reset.\n", core.Version, time.Now().Format(time.RFC3339), runtime.GOOS, runtime.GOARCH, msg)
}

func writeLaunchError(dir string, err error) (string, error) {
	if e := os.MkdirAll(dir, 0700); e != nil {
		return "", e
	}
	path := filepath.Join(dir, "startup-error.txt")
	if e := core.AtomicWrite(path, []byte(launchErrorText(err)), 0600); e != nil {
		return "", e
	}
	return path, nil
}

func commandLineMode(args []string) bool {
	for _, arg := range args {
		for _, name := range []string{"self-test", "headless", "reapply", "version", "help", "h"} {
			s := strings.TrimLeft(arg, "-")
			if strings.HasPrefix(arg, "-") && (s == name || strings.HasPrefix(s, name+"=")) {
				return true
			}
		}
	}
	return false
}

func reportLaunchFailure(err error) {
	fmt.Fprintln(os.Stderr, "IconCraft:", err)
	if commandLineMode(os.Args[1:]) {
		return
	}
	home, _ := os.UserHomeDir()
	dir := filepath.Join(home, "Library/Application Support/IconCraft")
	if runtime.GOOS != "darwin" {
		dir = filepath.Join(home, ".local/share/IconCraft")
	}
	path, saveErr := writeLaunchError(dir, err)
	if saveErr != nil {
		if f, e := os.CreateTemp("", "IconCraft-launch-error-*.txt"); e == nil {
			path = f.Name()
			_, _ = f.WriteString(launchErrorText(err))
			_ = f.Close()
		}
	}
	if path != "" {
		fmt.Fprintln(os.Stderr, "Launch error saved:", path)
	}
	if runtime.GOOS != "darwin" {
		return
	}
	message := privateSession.ReplaceAllString(err.Error(), "[private local interface]")
	if len([]rune(message)) > 1000 {
		message = string([]rune(message)[:1000]) + "\n…"
	}
	message += "\n\nYour icons and backups were not reset."
	if path != "" {
		message += "\n\nUse Show error to open the full launch error."
	}
	// Fixed AppleScript, with all variable content passed as arguments. No
	// interpolation, administrator privileges, security changes, or extra UI page.
	script := `on run argv
        set msg to item 1 of argv
        activate
        set choice to display dialog msg with title "IconCraft couldn't open" buttons {"Close", "Show error"} default button "Show error" with icon caution
        return button returned of choice
    end run`
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	out, e := exec.CommandContext(ctx, "/usr/bin/osascript", "-e", script, message).Output()
	if e == nil && strings.TrimSpace(string(out)) == "Show error" && path != "" {
		openCtx, stop := context.WithTimeout(context.Background(), 10*time.Second)
		defer stop()
		_ = exec.CommandContext(openCtx, "/usr/bin/open", "-t", path).Run()
	}
}
