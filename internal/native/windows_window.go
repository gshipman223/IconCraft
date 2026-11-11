//go:build windows

package native

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/gshipman223/IconCraft/internal/core"
	"github.com/gshipman223/IconCraft/internal/i18n"
)

// startDetached is replaceable so tests never open a browser or Settings.
var startDetached = func(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	return cmd.Start()
}

func (w *Windows) OpenURL(ctx context.Context, url string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if url != core.SupportURL && url != core.RepositoryURL && url != core.AppManagementURL {
		return os.ErrInvalid
	}
	return startDetached("cmd", "/c", "start", "", url)
}

func (w *Windows) OpenFolder(ctx context.Context, path string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if path == "" {
		return os.ErrInvalid
	}
	return startDetached("explorer.exe", path)
}

func (w *Windows) Choose(ctx context.Context, kind string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	title := i18n.T("Choose an original icon")
	filter := `Images|*.png;*.jpg;*.jpeg;*.gif;*.ico;*.svg;*.icns|All files|*.*`
	if kind == "folder" {
		title = i18n.T("Choose a folder")
	}
	if kind == "profile" {
		title = i18n.T("Choose an IconCraft profile")
		filter = `IconCraft profile|*.json|All files|*.*`
	}
	script := `$ErrorActionPreference='Stop'
Add-Type -AssemblyName System.Windows.Forms
if ($env:ICONCRAFT_KIND -eq 'folder') {
  $d = New-Object System.Windows.Forms.FolderBrowserDialog
  $d.Description = $env:ICONCRAFT_TITLE
  if ($d.ShowDialog() -eq 'OK') { $d.SelectedPath }
  exit 0
}
$d = New-Object System.Windows.Forms.OpenFileDialog
$d.Title = $env:ICONCRAFT_TITLE
$d.Filter = $env:ICONCRAFT_FILTER
if ($d.ShowDialog() -eq 'OK') { $d.FileName }
`
	cmd := exec.CommandContext(ctx, "powershell", "-NoProfile", "-STA", "-Command", script)
	cmd.Env = append(os.Environ(), "ICONCRAFT_KIND="+kind, "ICONCRAFT_TITLE="+title, "ICONCRAFT_FILTER="+filter)
	out, err := cmd.Output()
	if err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func (w *Windows) NotifyUser(ctx context.Context, title, body string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	script := `Add-Type -AssemblyName System.Windows.Forms
Add-Type -AssemblyName System.Drawing
$n = New-Object System.Windows.Forms.NotifyIcon
$n.Icon = [System.Drawing.SystemIcons]::Application
$n.Visible = $true
$n.ShowBalloonTip(4000, $env:ICONCRAFT_TITLE, $env:ICONCRAFT_BODY, 'Info')
Start-Sleep -Seconds 5
$n.Dispose()
`
	cmd := exec.CommandContext(ctx, "powershell", "-NoProfile", "-STA", "-Command", script)
	cmd.Env = append(os.Environ(), "ICONCRAFT_TITLE="+title, "ICONCRAFT_BODY="+body)
	return cmd.Run()
}

func (w *Windows) Window(ctx context.Context, url, icon string, menuOnly bool) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	openReq := make(chan struct{}, 1)
	go func() {
		_ = w.trayLoop(ctx, cancel, icon, func() {
			select {
			case openReq <- struct{}{}:
			default:
			}
		})
	}()
	if !menuOnly {
		return w.runBrowser(ctx, url)
	}
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-openReq:
			go func() { _ = w.runBrowser(ctx, url) }()
		}
	}
}

func (w *Windows) runBrowser(ctx context.Context, url string) error {
	if bin := findWindowsBrowser(); bin != "" {
		profile := filepath.Join(w.Dir, "browser-profile")
		if err := os.MkdirAll(profile, 0700); err != nil {
			return err
		}
		cmd := exec.CommandContext(ctx, bin,
			"--app="+url,
			"--user-data-dir="+profile,
			"--no-first-run",
			"--disable-extensions",
			"--class=iconcraft",
		)
		err := cmd.Run()
		if ctx.Err() != nil {
			return nil
		}
		return err
	}
	if err := startDetached("cmd", "/c", "start", "", url); err != nil {
		return err
	}
	<-ctx.Done()
	return nil
}

func findWindowsBrowser() string {
	local := os.Getenv("LOCALAPPDATA")
	candidates := []string{
		filepath.Join(os.Getenv("PROGRAMFILES"), `Microsoft\Edge\Application\msedge.exe`),
		filepath.Join(os.Getenv("PROGRAMFILES(X86)"), `Microsoft\Edge\Application\msedge.exe`),
		filepath.Join(local, `Google\Chrome\Application\chrome.exe`),
		filepath.Join(os.Getenv("PROGRAMFILES"), `Google\Chrome\Application\chrome.exe`),
		filepath.Join(os.Getenv("PROGRAMFILES(X86)"), `Google\Chrome\Application\chrome.exe`),
	}
	for _, p := range candidates {
		if p == "" {
			continue
		}
		if fi, err := os.Stat(p); err == nil && fi.Mode().IsRegular() {
			return p
		}
	}
	return ""
}
