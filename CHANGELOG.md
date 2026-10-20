# Changelog

All notable changes to IconCraft. Versions are ad-hoc signed Universal
(Apple Silicon + Intel) macOS builds; Developer ID signing/notarisation has
not been performed.

## [0.7.0] — 2026-10-21 · Full skin (identity while running)
- **Full skin** (pencil dialog): also writes the chosen name + icon *inside*
  a third-party/clone app and ad-hoc re-signs it, so the identity shows in
  the Dock, menu bar, app switcher and Launchpad while it runs — the one
  place a Finder skin can't reach. Bundle ID/executable/links untouched;
  byte-exact backup + Restore; refuses system/running/asset-catalog apps.
  Trade-offs disclosed in-app: previous signature replaced; self-updaters
  may overwrite it (keeper re-applies when the bundle reverts cleanly).
  Proven live on a disposable clone copy: rebrand → launch → restore,
  codesign verified at every step.

## [0.6.0] — 2026-10-21 · Skins that stick
- **Keep skins applied**: third-party apps/updaters often revert their Finder
  icon or folder name on launch/update. A tiny background LaunchAgent
  (`IconCraft --reapply`, at login + every 3 min + on /Applications changes)
  and IconCraft itself (while open, every 3 min) now quietly re-apply your
  chosen icons/names from the safety journal. Toggle in the footer.
  Proven live: simulated revert on a renamed+skinned app → repaired
  automatically (`iconFixed: 1`).
- Honest scope note, also in the UI tooltip: while an app is *running*,
  macOS shows that app's own Dock/menu identity from inside its bundle.
  IconCraft never edits inside apps (that would break signing, updates and
  links), so that part belongs to the app — everything Finder/Applications/
  Dock-presented is kept applied.

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
