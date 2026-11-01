#!/bin/bash
# Production release pipeline for the maintainer's Mac.
# Linux can cross-compile the Go slices (CGO_ENABLED=0). It cannot compile
# scripts/defaults-helper.m. That file is Objective-C against Foundation,
# AppKit, UniformTypeIdentifiers and CoreServices. On this Linux host,
# /usr/bin/clang fails with: unsupported option '-arch' for target
# 'x86_64-pc-linux-gnu'. lipo and codesign are not installed. Do not
# codesign or notarise from Linux.
#
# Credentials come from the environment; no secrets are stored in this project.
#   SIGNING_IDENTITY   e.g. "Developer ID Application: Gavin Shipman (TEAMID)"
#   ASC_KEY            path to the App Store Connect API key (.p8)
#   ASC_KEY_ID         the key's ID (10 chars)
#   ASC_ISSUER         the account's issuer ID (UUID)
#   INSTALLER_IDENTITY optional "Developer ID Installer: ..." for the .pkg
#
# scripts/build_macos.py, run by build-current.sh below, executes:
#   GOOS=darwin GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o build/iconcraft-amd64 ./cmd/iconcraft
#   GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o build/iconcraft-arm64 ./cmd/iconcraft
#   # each slice's LC_BUILD_VERSION minos is raised to 13.0.0 when lower
#   codesign --force --sign - --timestamp=none build/iconcraft-amd64
#   codesign --force --sign - --timestamp=none build/iconcraft-arm64
#   lipo -create build/iconcraft-amd64 build/iconcraft-arm64 -output build/IconCraft.app/Contents/MacOS/IconCraft
#   clang -fobjc-arc -mmacosx-version-min=13.0 -arch arm64 -framework Foundation -framework AppKit -framework UniformTypeIdentifiers -framework CoreServices scripts/defaults-helper.m -o build/iconcraft-defaults-arm64
#   clang -fobjc-arc -mmacosx-version-min=13.0 -arch x86_64 -framework Foundation -framework AppKit -framework UniformTypeIdentifiers -framework CoreServices scripts/defaults-helper.m -o build/iconcraft-defaults-x86_64
#   lipo -create build/iconcraft-defaults-arm64 build/iconcraft-defaults-x86_64 -output build/IconCraft.app/Contents/MacOS/iconcraft-defaults
# Info.plist LSMinimumSystemVersion is 13.0.
#
# This script then runs the Developer ID commands below: codesign (hardened
# runtime, timestamp), notarytool, stapler, spctl, package_v05.py (ZIP, DMG
# via hdiutil, SHA256SUMS), then codesign/notarise/staple the DMG and refresh
# the checksums.
set -euo pipefail
cd "$(dirname "$0")/.."
[[ "$(uname -s)" == Darwin ]] || { echo 'Release requires a Mac. Linux cannot compile scripts/defaults-helper.m or sign and notarise.' >&2; exit 1; }
: "${SIGNING_IDENTITY:?Set SIGNING_IDENTITY to your Developer ID Application identity}"
: "${ASC_KEY:?Set ASC_KEY to the App Store Connect API key .p8 path}"
: "${ASC_KEY_ID:?Set ASC_KEY_ID to the App Store Connect API key ID}"
: "${ASC_ISSUER:?Set ASC_ISSUER to the App Store Connect issuer ID}"

APP='build/IconCraft.app'
stage() { printf '\n== %s ==\n' "$1"; }

stage "build + full test pipeline"
bash build-current.sh

stage "universal binary and macOS 13 minimum"
[[ -x "$APP/Contents/MacOS/iconcraft-defaults" ]] || { echo 'iconcraft-defaults was not built. scripts/defaults-helper.m needs clang, lipo and the macOS SDK.' >&2; exit 1; }
lipo -info "$APP/Contents/MacOS/IconCraft"
lipo -info "$APP/Contents/MacOS/iconcraft-defaults"
main_info="$(lipo -info "$APP/Contents/MacOS/IconCraft")"
helper_info="$(lipo -info "$APP/Contents/MacOS/iconcraft-defaults")"
[[ "$main_info" == *x86_64* && "$main_info" == *arm64* ]] || { echo "IconCraft is not universal: $main_info" >&2; exit 1; }
[[ "$helper_info" == *x86_64* && "$helper_info" == *arm64* ]] || { echo "iconcraft-defaults is not universal: $helper_info" >&2; exit 1; }
MIN="$(/usr/libexec/PlistBuddy -c 'Print :LSMinimumSystemVersion' "$APP/Contents/Info.plist")"
[[ "$MIN" == "13.0" ]] || { echo "LSMinimumSystemVersion is '$MIN', want 13.0" >&2; exit 1; }
VER="$(/usr/libexec/PlistBuddy -c 'Print :CFBundleShortVersionString' "$APP/Contents/Info.plist")"
SRC_VER="$(sed -n 's/^const Version = "\(.*\)"$/\1/p' internal/core/types.go)"
[[ -n "$SRC_VER" && "$VER" == "$SRC_VER" ]] || { echo "CFBundleShortVersionString is '$VER', source Version is '$SRC_VER'" >&2; exit 1; }

stage "self-test (ad-hoc build)"
"$APP/Contents/MacOS/IconCraft" --self-test > /dev/null

stage "Developer ID sign (inside-out, hardened runtime, timestamp)"
codesign --force --options runtime --timestamp --sign "$SIGNING_IDENTITY" "$APP/Contents/MacOS/iconcraft-defaults"
codesign --force --options runtime --timestamp --sign "$SIGNING_IDENTITY" "$APP/Contents/MacOS/IconCraft"
codesign --force --options runtime --timestamp --sign "$SIGNING_IDENTITY" "$APP"
codesign --verify --deep --strict --verbose=2 "$APP"

stage "notarize"
ditto -c -k --keepParent "$APP" build/IconCraft-notarization.zip
xcrun notarytool submit build/IconCraft-notarization.zip \
  --key "$ASC_KEY" --key-id "$ASC_KEY_ID" --issuer "$ASC_ISSUER" --wait
xcrun stapler staple "$APP"
xcrun stapler validate "$APP"
spctl --assess --type execute --verbose=4 "$APP"

stage "package (stapled app; installer signed when INSTALLER_IDENTITY is set)"
ICONCRAFT_DISTRIBUTION=developer-id \
ICONCRAFT_INSTALLER_IDENTITY="${INSTALLER_IDENTITY:-}" \
  python3 scripts/package_v05.py

stage "sign + notarize disk image"
VERSION="$(/usr/libexec/PlistBuddy -c 'Print :CFBundleShortVersionString' "$APP/Contents/Info.plist")"
DMG="build/packages/IconCraft-Mac-$VERSION.dmg"
if [[ -f "$DMG" ]]; then
  codesign --force --timestamp --sign "$SIGNING_IDENTITY" "$DMG"
  codesign --verify --verbose=2 "$DMG"
  xcrun notarytool submit "$DMG" \
    --key "$ASC_KEY" --key-id "$ASC_KEY_ID" --issuer "$ASC_ISSUER" --wait
  xcrun stapler staple "$DMG"
  xcrun stapler validate "$DMG"
  spctl --assess --type open --context context:primary-signature --verbose=4 "$DMG"
  # Signing/notarizing changed the DMG, so refresh the published checksums.
  (
    cd build/packages
    shasum -a 256 \
      "IconCraft-Mac-$VERSION.zip" \
      "IconCraft-Mac-$VERSION.dmg" \
      "IconCraft-Mac-$VERSION-Source.zip" \
      "IconCraft-Mac-$VERSION-Validation.json" \
      > "IconCraft-Mac-$VERSION-SHA256SUMS.txt"
  )
fi

echo
echo 'Signed, notarized and packaged into build/packages/.'
echo 'Publish: tag the release, then replace the CI-built assets with these:'
echo "  gh release upload <tag> build/packages/IconCraft-Mac-$VERSION.zip build/packages/IconCraft-Mac-$VERSION.dmg build/packages/IconCraft-Mac-$VERSION-Source.zip build/packages/IconCraft-Mac-$VERSION-Validation.json build/packages/IconCraft-Mac-$VERSION-SHA256SUMS.txt --clobber"
