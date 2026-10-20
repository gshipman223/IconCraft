# 0.5.0 — new look, working icons, real-Mac fixes

IconCraft 0.5.0 · Universal (Apple Silicon + Intel), macOS 12.3+
Built, tested and run **on a real Mac** (darwin/arm64) — including the native
self-test (27/27 checks), live scanning of 111 installed apps, and screenshots
of the running app. Ad-hoc signed; Developer ID signing/notarization not performed.

## New branding & design
- **New IconCraft logo** designed for this release (`assets/logo.svg`, source of
  `assets/icon.icns` via `scripts/make_logo.py`): violet→indigo→cyan gradient
  squircle, app-tile grid with one lifted cyan tile being crafted. It is used
  in the app header, Finder and the Dock — replacing the generic placeholder.
- **Redesigned interface**: dark-first modern techy glass UI, gradient accents
  matching the logo, System / Light / Dark in the header.
- **Version number** shown in the header pill and in the support footer.

## Support / contributions
- Contribution bar at the bottom of the main window, like most open-source
  apps: *"IconCraft is free & open source… Contribute via PayPal"* plus a
  GitHub button. PayPal opens Gavin Shipman's verified PayPal business link
  **https://www.paypal.com/ncp/payment/E8YDD8BNAKVJA** in the real browser
  (allowlisted native bridge; the page cannot inject arbitrary URLs).
  Same link in `.github/FUNDING.yml`.

## Fixed: "no icons of the apps show at all"
Two stacked bugs, both fixed:
1. The list only ever showed catalogue-matched (ArtCraft-counterpart) apps —
   ~14 of 111 apps on the test Mac. Now **every installed app** is listed;
   unmapped apps show their own icon and "Pick any icon →".
2. Icon `<img>`s were `loading="lazy"` while `display:none` until load;
   WebKit never fetches hidden lazy images, so the revealing load event could
   never fire. Icons now load eagerly, and server thumbnails are normalized
   to 8-bit PNGs (≤192px, ~29KB instead of 16-bit ~100KB).

## Fixed on a real Mac (invisible from Linux CI)
- **Replace always rolled back on signed apps**: the post-change check used
  `codesign --verify --deep --strict`; Finder custom-icon data (created by
  every icon change) is "detritus not allowed" under `--strict`. Verification
  now uses `--deep` without `--strict` (still verifies sealed code/resources),
  both before and after a change.
- **Unsigned-by-construction universal binary**: the hand-assembled fat binary
  invalidated Go linker signatures and codesign refused the app entirely.
  `scripts/build_macos.py` now signs each slice and combines them with `lipo`,
  then ad-hoc signs and verifies the bundle.
- Test fixtures under macOS `/var`→`/private/var` temp dirs were rejected by
  the (correct) anti-symlink bundle guard; fixtures now resolve real paths.
- `build-current.sh` used `python`; now `python3`.

## Verified (this Mac, 2026-10-21)
`go test ./...` (119 tests) + `-race` + `go vet` clean · Node bridge/UI tests
36/36 · `--self-test` 27/27 · live scan 111 apps, icon PNGs 30/30 sampled OK
and visibly rendered · signed `/Applications/IconCraft.app` launched and used.
