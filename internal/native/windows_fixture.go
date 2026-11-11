//go:build windows

package native

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"

	"github.com/gshipman223/IconCraft/internal/core"
	"github.com/gshipman223/IconCraft/internal/launchers"
)

func fixtureWindows(root string) *Windows {
	return &Windows{
		Dir:           filepath.Join(root, "data"),
		Home:          root,
		UserStart:     filepath.Join(root, "start"),
		UserDesktop:   filepath.Join(root, "desktop"),
		UserPinned:    filepath.Join(root, "pinned"),
		CommonStart:   filepath.Join(root, "common-start"),
		CommonDesktop: filepath.Join(root, "common-desktop"),
		SkipStore:     true,
		SkipServices:  true,
		SkipRegistry:  true,
		TaskDir:       filepath.Join(root, "tasks"),
		RunFile:       filepath.Join(root, "run.txt"),
	}
}

func (w *Windows) roundTrip(ctx context.Context) error {
	if err := os.MkdirAll(w.UserStart, 0755); err != nil {
		return err
	}
	if err := os.MkdirAll(w.CommonStart, 0755); err != nil {
		return err
	}
	target := `C:\Program Files\PhotoCraft\PhotoCraft.exe`
	sc := launchers.NewShortcut(target)
	raw, err := sc.Bytes()
	if err != nil {
		return err
	}
	userPath := filepath.Join(w.UserStart, "PhotoCraft.lnk")
	if err = os.WriteFile(userPath, raw, 0644); err != nil {
		return err
	}
	commonPath := filepath.Join(w.CommonStart, "Shared.lnk")
	if err = os.WriteFile(commonPath, raw, 0644); err != nil {
		return err
	}
	commonBefore := append([]byte{}, raw...)
	found, _ := w.Discover(nil)
	sawUser := false
	for _, p := range found {
		if strings.EqualFold(p, userPath) {
			sawUser = true
		}
	}
	if !sawUser {
		return errors.New("user shortcut was not discovered")
	}
	png := windowsPNG()
	icon := filepath.Join(w.Dir, "new.png")
	if err = os.MkdirAll(w.Dir, 0700); err != nil {
		return err
	}
	if err = os.WriteFile(icon, png, 0644); err != nil {
		return err
	}
	if err = w.Set(ctx, userPath, icon); err != nil {
		return err
	}
	got, err := os.ReadFile(userPath)
	if err != nil {
		return err
	}
	parsed, err := launchers.ReadShortcut(got)
	if err != nil {
		return err
	}
	if !strings.EqualFold(parsed.Target, target) {
		return errors.New("setting the icon changed the shortcut target")
	}
	if !w.ourIcon(parsed.Icon) {
		return errors.New("shortcut icon was not pointed at IconCraft's icon file")
	}
	snap, err := w.Snapshot(userPath)
	if err != nil {
		return err
	}
	if !snap.Custom() {
		return errors.New("snapshot did not record the custom icon")
	}
	if err = w.Restore(ctx, userPath, core.Snapshot{Attributes: map[string][]byte{"iconIndex": []byte("0")}}); err != nil {
		return err
	}
	cleared, err := w.Snapshot(userPath)
	if err != nil {
		return err
	}
	if cleared.Custom() {
		return errors.New("restore left the custom icon in place")
	}
	again, err := w.readShortcut(userPath)
	if err != nil {
		return err
	}
	if !strings.EqualFold(again.Target, target) {
		return errors.New("restore changed the shortcut target")
	}
	commonAfter, err := os.ReadFile(commonPath)
	if err != nil {
		return err
	}
	if !bytes.Equal(commonBefore, commonAfter) {
		return errors.New("all-users shortcut was modified")
	}
	renamed, err := w.ApplyLauncherName(ctx, core.App{Path: userPath, Name: "PhotoCraft", Executable: target}, "Darkroom")
	if err != nil {
		return err
	}
	if !strings.EqualFold(filepath.Base(renamed.Path), "Darkroom.lnk") {
		return errors.New("rename did not change the shortcut file name")
	}
	moved, err := w.readShortcut(renamed.Path)
	if err != nil {
		return err
	}
	if !strings.EqualFold(moved.Target, target) {
		return errors.New("rename changed the shortcut target")
	}
	return nil
}

func windowsPNG() []byte {
	img := image.NewNRGBA(image.Rect(0, 0, 8, 8))
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}
