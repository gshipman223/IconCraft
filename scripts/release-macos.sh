#!/bin/bash
# Production release pipeline for the maintainer's Mac:
#   build + test -> Developer ID sign (hardened runtime) -> notarize ->
#   staple -> package.
# Credentials come from the environment; no secrets are stored in this project.
#   SIGNING_IDENTITY   e.g. "Developer ID Application: Your Name (TEAMID)"
#   ASC_KEY            path to the App Store Connect API key (.p8)
#   ASC_KEY_ID         the key's ID (10 chars)
#   ASC_ISSUER         the account's issuer ID (UUID)
#   INSTALLER_IDENTITY optional "Developer ID Installer: ..." for the .pkg
set -euo pipefail
cd "$(dirname "$0")/.."
[[ "$(uname -s)" == Darwin ]] || { echo 'Release requires a Mac.' >&2; exit 1; }
: "${SIGNING_IDENTITY:?Set SIGNING_IDENTITY to your Developer ID Application identity}"
: "${ASC_KEY:?Set ASC_KEY to the App Store Connect API key .p8 path}"
: "${ASC_KEY_ID:?Set ASC_KEY_ID to the App Store Connect API key ID}"
: "${ASC_ISSUER:?Set ASC_ISSUER to the App Store Connect issuer ID}"

APP='build/IconCraft.app'
stage() { printf '\n== %s ==\n' "$1"; }

stage "build + full test pipeline"
bash build-current.sh

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
  spctl --assess --type open --verbose=4 "$DMG"
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
