#!/bin/bash
# Package the Linux amd64 binary as a tarball, a .deb, and an AppDir.
# appimagetool is optional: when it is missing the AppDir is left for a
# later AppImage build. Nothing here is signed.
set -euo pipefail
cd "$(dirname "$0")/.."
ver="$(sed -n 's/^const Version = "\([^"]*\)".*/\1/p' internal/core/types.go)"
bin="build/iconcraft-linux-amd64"
if [ ! -f "$bin" ]; then
  CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -o "$bin" ./cmd/iconcraft
fi
stage="dist/linux/stage"
rm -rf "$stage" dist/linux/IconCraft-linux-amd64-"$ver".tar.gz
mkdir -p "$stage/IconCraft-linux-amd64-$ver"
install -m 0755 "$bin" "$stage/IconCraft-linux-amd64-$ver/iconcraft"
install -m 0644 packaging/linux/io.iconcraft.IconCraft.desktop "$stage/IconCraft-linux-amd64-$ver/io.iconcraft.IconCraft.desktop"
install -m 0644 assets/logo-1024.png "$stage/IconCraft-linux-amd64-$ver/io.iconcraft.IconCraft.png"
install -m 0644 PLATFORMS.md "$stage/IconCraft-linux-amd64-$ver/PLATFORMS.md"
mkdir -p dist/linux
tar -C "$stage" -czf "dist/linux/IconCraft-linux-amd64-$ver.tar.gz" "IconCraft-linux-amd64-$ver"

deb="dist/linux/deb"
rm -rf "$deb"
mkdir -p "$deb/DEBIAN" "$deb/usr/bin" "$deb/usr/share/applications" "$deb/usr/share/icons/hicolor/1024x1024/apps"
install -m 0755 "$bin" "$deb/usr/bin/iconcraft"
install -m 0644 packaging/linux/io.iconcraft.IconCraft.desktop "$deb/usr/share/applications/io.iconcraft.IconCraft.desktop"
install -m 0644 assets/logo-1024.png "$deb/usr/share/icons/hicolor/1024x1024/apps/io.iconcraft.IconCraft.png"
cat > "$deb/DEBIAN/control" <<EOF
Package: iconcraft
Version: ${ver}
Section: utils
Priority: optional
Architecture: amd64
Maintainer: IconCraft <https://github.com/gshipman223/IconCraft>
Depends: libc6
Recommends: xdg-utils, zenity | kdialog
Description: Restore and craft application icons
 IconCraft changes launcher names and icons from the user's account.
 It does not edit program files and does not need an administrator password.
EOF
if command -v dpkg-deb >/dev/null 2>&1; then
  dpkg-deb --root-owner-group --build "$deb" "dist/linux/iconcraft_${ver}_amd64.deb"
else
  echo "dpkg-deb is not installed; the deb tree is at ${deb}"
fi

appdir="dist/linux/IconCraft.AppDir"
rm -rf "$appdir"
mkdir -p "$appdir/usr/bin" "$appdir/usr/share/applications" "$appdir/usr/share/icons/hicolor/1024x1024/apps"
install -m 0755 "$bin" "$appdir/usr/bin/iconcraft"
install -m 0644 packaging/linux/io.iconcraft.IconCraft.desktop "$appdir/io.iconcraft.IconCraft.desktop"
install -m 0644 packaging/linux/io.iconcraft.IconCraft.desktop "$appdir/usr/share/applications/io.iconcraft.IconCraft.desktop"
install -m 0644 assets/logo-1024.png "$appdir/io.iconcraft.IconCraft.png"
install -m 0644 assets/logo-1024.png "$appdir/.DirIcon"
install -m 0644 assets/logo-1024.png "$appdir/usr/share/icons/hicolor/1024x1024/apps/io.iconcraft.IconCraft.png"
cat > "$appdir/AppRun" <<'EOF'
#!/bin/sh
here="$(dirname "$(readlink -f "$0")")"
exec "$here/usr/bin/iconcraft" "$@"
EOF
chmod 0755 "$appdir/AppRun"
if command -v appimagetool >/dev/null 2>&1; then
  ARCH=x86_64 appimagetool "$appdir" "dist/linux/IconCraft-linux-amd64-$ver.AppImage" || echo "appimagetool did not produce an AppImage; the AppDir is ready"
else
  echo "appimagetool is not installed; AppDir scaffolding is at ${appdir}"
fi
echo "linux packages:"
ls -l dist/linux || true
