# Releasing IconCraft

CI builds are ad-hoc signed (no certificates live in GitHub). Published
release assets are built, signed and notarised on the maintainer's Mac,
then uploaded over the CI-built assets.

## One-time setup

- A **Developer ID Application** certificate in the login Keychain
  (`security find-identity -v -p codesigning`).
- An **App Store Connect API key** (`.p8`) plus its key ID and the
  account issuer ID (App Store Connect → Users and Access → Integrations
  → App Store Connect API). Keep the `.p8` out of the repo
  (e.g. `~/.appstoreconnect/private_keys/`).
- Optional: a **Developer ID Installer** certificate (create it at
  developer.apple.com → Certificates, or in Xcode → Settings → Accounts
  → Manage Certificates). Without it the `.pkg` ships unsigned while the
  app inside is notarised; the ZIP is then the recommended download.

## Per release

1. Bump the version: `internal/core/types.go`, `scripts/build_macos.py`
   (VERSION + CFBundleVersion), `scripts/package_v05.py`,
   `scripts/smoke_host.py`, `internal/server/ui/index.html`,
   `internal/server/ui/app.js`, `README.md`, plus a `CHANGELOG.md` entry.
2. `git commit`, `git tag vX.Y.Z`, `git push origin main vX.Y.Z`.
   CI runs the full validation suite and creates the GitHub release with
   ad-hoc assets.
3. On the Mac, build the production packages:

   ```sh
   export SIGNING_IDENTITY="Developer ID Application: Your Name (TEAMID)"
   export ASC_KEY=~/.appstoreconnect/private_keys/AuthKey_XXXXXXXXXX.p8
   export ASC_KEY_ID=XXXXXXXXXX
   export ASC_ISSUER=xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx
   export INSTALLER_IDENTITY="Developer ID Installer: Your Name (TEAMID)"  # optional
   bash scripts/release-macos.sh
   ```

4. Replace the release assets with the signed + notarised ones:

   ```sh
   gh release upload vX.Y.Z build/packages/IconCraft-Mac-X.Y.Z.zip \
     build/packages/IconCraft-Mac-X.Y.Z-Installer.pkg \
     build/packages/IconCraft-Mac-X.Y.Z-Validation.json \
     build/packages/IconCraft-Mac-X.Y.Z-SHA256SUMS.txt \
     build/packages/IconCraft-Mac-X.Y.Z-Source.zip --clobber
   ```

5. Verify: `gh release view vX.Y.Z`, then download the ZIP on a Mac,
   unzip, and confirm `spctl --assess --type execute` reports
   `accepted / source=Notarized Developer ID`.

## Download counting

GitHub counts every release-asset download:

```sh
gh api repos/gshipman223/IconCraft/releases \
  --jq '.[] | .tag_name, (.assets[] | "  \(.name): \(.download_count)")'
```

The README and iconcraft.co show a live total via the shields.io
`github/downloads` badge.
