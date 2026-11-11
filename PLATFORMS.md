# IconCraft on macOS, Linux, and Windows

IconCraft is one program: a Go backend and the same web UI. The macOS app is unchanged. Linux and Windows change shortcuts and launcher files in your account. They do not edit program files, and normal use does not ask for an administrator password. These builds are not code-signed.

## What works

| | macOS | Linux | Windows |
| --- | --- | --- | --- |
| App list | Installed `.app` bundles | `.desktop` launchers, including Flatpak and snap exports that already have a desktop file | Start menu, desktop, and pinned taskbar shortcuts, plus a read-only Store list |
| Rename | Renames the `.app` | Writes `Name=` in your launcher override | Renames your shortcut file |
| New icon | Finder custom icon | `Icon=` in your launcher override | Icon location on your shortcut |
| Running app | Dock and menu bar follow the app's own icon while it is running | The dock usually follows the same desktop entry. Apps that draw their own window icon do not | The window and Alt-Tab keep the program's icon |
| Original-icon search | Installed apps, the vault, and the publisher catalogue | Same pipeline. Flathub is not queried | Same pipeline. The Microsoft Store catalogue is not queried |
| Originals Vault, backup, Restore all | Yes | Yes | Yes |
| Default apps | LaunchServices, changed in IconCraft | Your `mimeapps.list`, changed in IconCraft | Opens Windows Settings. IconCraft does not switch them |
| Settings and four languages | English, Simplified Chinese, Japanese, Korean | Same | Same |
| Keep skins applied | LaunchAgent | systemd user timer, every 3 minutes | Scheduled task in your account, every 3 minutes |
| Open at login | LaunchAgent | XDG autostart | Your Run key |
| Notifications | macOS notification | `notify-send`, then a desktop-bus fallback | A tray balloon |
| Tray | Menu-bar icon | StatusNotifierItem. GNOME needs the AppIndicator extension | Notification-area icon |
| Window | Native window | Chrome, Edge, or Chromium in app mode, then Firefox, then the default browser | Edge or Chrome in app mode, then the default browser |

## What each system does not do

**macOS.** The signed app behaves as it does today: Finder icons, `.app` rename, full skin, LaunchServices defaults, the menu bar, and LaunchAgents.

**Linux.** IconCraft writes a user override under `~/.local/share/applications` and leaves the system file alone. It does not edit binaries. AppImages appear only when they already installed a desktop file. SVG icons need `rsvg-convert`. The tray is best-effort; if the desktop bus has no StatusNotifier watcher, the window still opens. There is no Flatpak or Snap build of IconCraft itself. Snap apps are listed when snapd has exported a desktop file.

**Windows.** IconCraft edits shortcuts you can write: your Start menu, your desktop, and shortcuts pinned to your taskbar. An all-users shortcut is not edited in place. IconCraft makes a personal copy in your Start menu, and the original common shortcut can still appear beside it. Store and MSIX apps are read-only. Default apps stay in Settings. The running window and Alt-Tab keep the executable's icon. Authenticode signatures are never touched. The tray is best-effort; the window still opens if the notification area rejects the icon. A downloaded unsigned copy can show a SmartScreen prompt.

## Try a build

From a checkout, with Go installed:

```bash
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o iconcraft ./cmd/iconcraft
./iconcraft --self-test
./iconcraft
```

`scripts/build_ports.sh` builds `linux/amd64`, `linux/arm64`, `windows/amd64`, and `windows/arm64` with `CGO_ENABLED=0`. `scripts/package_linux.sh` writes a tarball, a `.deb` when `dpkg-deb` is present, and an AppDir. It writes an AppImage only when `appimagetool` is installed. `scripts/package_windows.ps1` writes a zip. `packaging/windows/iconcraft.iss` is an Inno Setup script and is not compiled here.

`--self-test` uses a throwaway directory. It does not change installed apps, the Start menu, mimeapps, systemd, or the registry. `--headless` prints the local page address and does not open a window.

The version number is still the macOS release. These ports do not cut a GitHub release.
