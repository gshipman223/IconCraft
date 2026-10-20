# Changelog

All notable changes to IconCraft. Versions are ad-hoc signed Universal
(Apple Silicon + Intel) macOS builds; Developer ID signing/notarisation has
not been performed.

## [0.5.1] — 2026-10-21 · Exhaustive original-icon search
- Six-layer resolver: publisher pages (+ embedded JSON assets), publisher CDN,
  App Store by ID, **verified App Store search** (seller + name + bundle ID
  triple-check, knockoffs rejected), saved original, installed genuine app
  (incl. year-suffixed bundles like `AutoCAD2025`).
- AutoCAD now resolves via Autodesk's verified App Store listing after
  autodesk.com's HTTP 403 bot-block; Avid handled the same honest way.
- "Not found" now reports every layer checked and what it returned.
- Details: `docs/CHANGES-0.5.1.md`.

## [0.5.0] — 2026-10-21 · New look, working icons
- New IconCraft logo + full modern UI redesign (System/Light/Dark), version
  pill, and an in-app PayPal/GitHub support bar (FUNDING.yml too).
- Fixed icons never appearing: all installed apps are now listed (was only
  catalogue-matched ones), and hidden lazy-loaded `<img>`s that WebKit never
  fetched now load correctly; thumbnails normalised to light 8-bit PNGs.
- Fixed Replace always rolling back on signed apps (`codesign --strict`
  rejects Finder custom-icon data) and the un-signable hand-assembled fat
  binary (now per-slice sign + `lipo`). All verified live on a real Mac,
  native self-test 27/27. Details: `docs/CHANGES-0.5.0.md`.

## [0.4.1] — 2026-10-20 · Launch repair
- Removed the native appearance hook from startup; visible startup errors
  with a saved report. Details: `docs/CHANGES-0.4.1.md`.

## [0.4.0] — 2026-10-20
- One-screen personal Mac edition: cached inventory, background original-icon
  discovery, in-window icon picker, bulk replace/restore, app renaming,
  System/Light/Dark appearance. Details: `docs/CHANGES-0.4.md`.
