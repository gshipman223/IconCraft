#!/bin/bash
# Run on the maintainer's Mac only. No account secrets are stored in this project.
set -euo pipefail
cd "$(dirname "$0")/.."
[[ "$(uname -s)" == Darwin ]] || { echo 'Signing requires a Mac.' >&2; exit 1; }
: "${SIGNING_IDENTITY:?Set an existing Developer ID Application identity from your Keychain}"
: "${NOTARY_PROFILE:?Set the name of an existing notarytool Keychain profile}"
: "${MAC_ACCEPTANCE_APPROVED:?Complete docs/MAC-ACCEPTANCE.md, then explicitly set MAC_ACCEPTANCE_APPROVED=yes}"
[[ "$MAC_ACCEPTANCE_APPROVED" == yes ]] || { echo 'Interactive Mac acceptance has not been approved.' >&2; exit 1; }
APP='build/IconCraft.app'
[[ -d "$APP" ]] || { echo 'Build the application first.' >&2; exit 1; }
"$APP/Contents/MacOS/IconCraft" --self-test --report build/native-release-checks.json
codesign --force --options runtime --timestamp --sign "$SIGNING_IDENTITY" "$APP/Contents/MacOS/IconCraft"
codesign --force --options runtime --timestamp --sign "$SIGNING_IDENTITY" "$APP"
codesign --verify --deep --strict --verbose=2 "$APP"
ditto -c -k --keepParent "$APP" build/IconCraft-notarization.zip
xcrun notarytool submit build/IconCraft-notarization.zip --keychain-profile "$NOTARY_PROFILE" --wait
xcrun stapler staple "$APP"
xcrun stapler validate "$APP"
spctl --assess --type execute --verbose=4 "$APP"
ditto -c -k --keepParent "$APP" build/IconCraft-mac-universal.zip
shasum -a 256 build/IconCraft-mac-universal.zip > build/IconCraft-mac-universal.zip.sha256
echo 'Signed and notarized archive created. Perform the final quarantined-download launch test before publishing.'
