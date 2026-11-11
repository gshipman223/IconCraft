//go:build linux

package native

import (
	"context"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"sync"

	"github.com/godbus/dbus/v5"
	"github.com/godbus/dbus/v5/prop"
)

func (l *Linux) NotifyUser(ctx context.Context, title, body string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if bin, err := exec.LookPath("notify-send"); err == nil {
		cmd := exec.CommandContext(ctx, bin, "--app-name=IconCraft", title, body)
		if err = cmd.Run(); err == nil {
			return nil
		}
	}
	if bin, err := exec.LookPath("gdbus"); err == nil {
		cmd := exec.CommandContext(ctx, bin, "call", "--session",
			"--dest=org.freedesktop.Notifications",
			"--object-path=/org/freedesktop/Notifications",
			"--method=org.freedesktop.Notifications.Notify",
			"IconCraft", "0", "iconcraft", title, body, "[]", "{}", "5000")
		return cmd.Run()
	}
	return exec.ErrNotFound
}

func (l *Linux) Window(ctx context.Context, url, icon string, menuOnly bool) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	openReq := make(chan struct{}, 1)
	trayErr := l.startTray(ctx, cancel, icon, func() {
		select {
		case openReq <- struct{}{}:
		default:
		}
	})
	if !menuOnly || trayErr != nil {
		return l.runBrowser(ctx, url)
	}
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-openReq:
			_ = l.runBrowser(ctx, url)
		}
	}
}

func (l *Linux) runBrowser(ctx context.Context, url string) error {
	bin, appMode := findBrowser()
	if bin == "" {
		return os.ErrNotExist
	}
	var args []string
	if appMode {
		profile := filepath.Join(l.Dir, "browser-profile")
		if err := os.MkdirAll(profile, 0700); err != nil {
			return err
		}
		args = []string{
			"--app=" + url,
			"--class=iconcraft",
			"--user-data-dir=" + profile,
			"--no-first-run",
			"--no-default-browser-check",
			"--disable-extensions",
			"--disable-sync",
		}
	} else if filepath.Base(bin) == "xdg-open" {
		cmd := exec.CommandContext(ctx, bin, url)
		if err := cmd.Start(); err != nil {
			return err
		}
		<-ctx.Done()
		return nil
	} else {
		args = []string{"--new-window", url}
	}
	cmd := exec.CommandContext(ctx, bin, args...)
	err := cmd.Run()
	if ctx.Err() != nil {
		return nil
	}
	return err
}

func findBrowser() (string, bool) {
	for _, name := range []string{"google-chrome", "google-chrome-stable", "chromium", "chromium-browser", "microsoft-edge", "microsoft-edge-stable"} {
		if p, err := exec.LookPath(name); err == nil {
			return p, true
		}
	}
	if p, err := exec.LookPath("firefox"); err == nil {
		return p, false
	}
	if p, err := exec.LookPath("xdg-open"); err == nil {
		return p, false
	}
	return "", false
}

type trayIcon struct {
	mu     sync.Mutex
	open   func()
	cancel context.CancelFunc
}

func (t *trayIcon) Activate(int32, int32) *dbus.Error {
	t.mu.Lock()
	fn := t.open
	t.mu.Unlock()
	if fn != nil {
		fn()
	}
	return nil
}

func (t *trayIcon) SecondaryActivate(int32, int32) *dbus.Error { return t.Quit() }

func (t *trayIcon) ContextMenu(int32, int32) *dbus.Error { return nil }

func (t *trayIcon) Scroll(int32, string) *dbus.Error { return nil }

func (t *trayIcon) Quit() *dbus.Error {
	t.mu.Lock()
	cancel := t.cancel
	t.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	return nil
}

func (l *Linux) startTray(ctx context.Context, cancel context.CancelFunc, icon string, open func()) error {
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return err
	}
	go func() {
		<-ctx.Done()
		_ = conn.Close()
	}()
	item := &trayIcon{open: open, cancel: cancel}
	if err = conn.Export(item, "/StatusNotifierItem", "org.kde.StatusNotifierItem"); err != nil {
		_ = conn.Close()
		return err
	}
	props := map[string]map[string]*prop.Prop{
		"org.kde.StatusNotifierItem": {
			"Category":   {Value: "ApplicationStatus"},
			"Id":         {Value: "iconcraft"},
			"Title":      {Value: "IconCraft"},
			"Status":     {Value: "Active"},
			"IconName":   {Value: "applications-system"},
			"ItemIsMenu": {Value: false},
		},
	}
	if pix, ok := iconPixmap(icon); ok {
		props["org.kde.StatusNotifierItem"]["IconPixmap"] = &prop.Prop{Value: pix}
	}
	if _, err = prop.Export(conn, "/StatusNotifierItem", props); err != nil {
		_ = conn.Close()
		return err
	}
	watcher := conn.Object("org.kde.StatusNotifierWatcher", "/StatusNotifierWatcher")
	call := watcher.Call("org.kde.StatusNotifierWatcher.RegisterStatusNotifierItem", 0, conn.Names()[0])
	if call.Err != nil {
		_ = conn.Close()
		return call.Err
	}
	return nil
}

type iconPix struct {
	Width  int32
	Height int32
	Bytes  []byte
}

func iconPixmap(path string) ([]iconPix, bool) {
	f, err := os.Open(path)
	if err != nil {
		return nil, false
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		return nil, false
	}
	b := img.Bounds()
	if b.Dx() < 1 || b.Dy() < 1 || b.Dx() > 256 || b.Dy() > 256 {
		return nil, false
	}
	raw := make([]byte, 0, b.Dx()*b.Dy()*4)
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			r, g, b, a := img.At(x, y).RGBA()
			raw = append(raw, byte(a>>8), byte(r>>8), byte(g>>8), byte(b>>8))
		}
	}
	return []iconPix{{Width: int32(b.Dx()), Height: int32(b.Dy()), Bytes: raw}}, true
}
