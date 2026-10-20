# Changelog

All notable changes to IconCraft. Versions are ad-hoc signed Universal
(Apple Silicon + Intel) macOS builds; Developer ID signing/notarisation has
not been performed.

## [0.12.0] — 2026-10-21 · Basic settings: startup, visibility, notifications, updates

- **Settings screen** (header → Settings) with the basics expected of a
  menu-bar utility, all journalled and applied live where macOS allows:
  - **Launch at login** via its own LaunchAgent (separate from the
    keep-applied agent; skins never depended on the GUI).
  - **Start in the menu bar only** — at login the window stays hidden
    until opened from the menu bar item.
  - **Show in the Dock** on/off (menu-bar-only mode) and **menu bar
    icon** on/off, applied live through the window host. One guard:
    both can never be off at once, and menu-bar start requires the
    menu bar icon — IconCraft always leaves a way back in.
  - **Notifications**: optional banners when the keeper re-applies a
    reverted skin, and when a skin needs attention (throttled to at
    most one every 6 hours), plus a Send test button.
  - **Updates**: manual “Check now” against GitHub releases and an
    optional weekly background check. IconCraft never downloads or
    installs updates itself.
  - **Maintenance**: refresh the Finder/Dock icon cache for skinned
    apps, open the data folder, re-run Setup & Permissions.

## [0.11.0] — 2026-10-21 · Running (Dock) icons that stay skinned

- **Fixed icons flipping back while an app is open**: some apps (the
  Rust/eframe clones included) ignore their bundle icon while running
  and set the Dock/window icon at startup from artwork embedded inside
  the executable, so a bundle-only skin always flipped on launch.
- Full skins now also swap that embedded artwork: the executable is
  scanned for PNGs that pixel-match the app's original bundle icon,
  and only those byte regions are replaced — same total length,
  zero-padded, so no other offset in the binary moves — before the
  usual ad-hoc re-sign. The original bytes are backed up with the
  full-skin backup and restored byte-exactly on unskin.
- Existing full skins are migrated by the keep-applied pass (only while
  the app is not running and unchanged). If an app sets its own running
  icon but no replaceable embedded copy is found, IconCraft now says so
  in the app list and rename dialog instead of silently flipping.
- **Rounded corners**: full-bleed artwork fitted onto the macOS icon
  grid is now also clipped to the macOS squircle (superellipse), since
  macOS never rounds square publisher/App Store artwork itself. All
  existing fitted skins — Finder, full and embedded runtime icons —
  are re-rendered by the keeper.
- Detection is conservative: non-icon images inside the binary are
  never touched, and a replacement that cannot fit its byte slot
  without moving data is never forced.

## [0.10.1] — 2026-10-21 · Icons at native macOS scale

- **Fixed oversized icons**: IconCraft's own app icon and the replacement
  icons it applies were using nearly the whole 1024px icon canvas, so they
  rendered about 20% larger than native macOS app icons in the Dock,
  Finder and Launchpad (measured: 99–100% canvas occupancy vs ~80% for
  genuine icons such as Cursor).
- IconCraft's logo is regenerated on the standard macOS icon grid — an
  824px body centred in the 1024px canvas.
- Finder skins and full (internal) skins now fit *full-bleed* artwork
  (publisher/App Store icons that run edge-to-edge) onto that same grid
  when applied. Artwork that already carries proper macOS padding — a
  genuine app icon or a glyph on transparency — is passed through
  untouched, never shrunk twice.
- Existing skins are migrated in place by the keep-applied pass: the
  journal record is updated, not stacked, and only when the current icon
  is still exactly the one IconCraft applied. Full skins are re-fitted
  and re-signed only while the app is not running.
- Icon renditions are now resampled with bilinear interpolation instead
  of nearest-neighbour.

## [0.10.0] — 2026-10-21 · Setup & Permissions

- **Installer finish step**: the one-click `.pkg` now opens with a Welcome
  screen explaining the permissions step and ends with a Conclusion screen
  telling the user to open IconCraft and complete **Finish setup**. The ZIP
  installer and packaged READ ME give the same guidance.
- **First-run Setup & Permissions screen**: after install/update, IconCraft
  now presents a dedicated review with live status pills for opening the
  app, local data/backups, app discovery, App Management, the optional
  background keeper, and the permissions it deliberately does not request
  (Full Disk Access, Accessibility, Screen Recording and Automation).
- **Honest App Management status**: macOS does not expose a reliable global
  grant readout to an ad-hoc app, so IconCraft does not fake one. **Run
  checks** uses the same harmless create-and-remove bundle-writability
  probe used before a real change. Success is shown as **Granted — tested**
  for the tested app; refusal is shown as **Blocked**, with a one-click
  button to System Settings → Privacy & Security → App Management and a
  **Check again** action. The copy also warns that a future locally signed
  update may be treated as a new app by macOS.
- **Background status that distinguishes causes**: the keeper row now
  separates on/loaded, off, installed-but-not-loaded, missing and
  disabled-in-Login-Items states, with an in-place on/off button.
- Setup is shown once per setup version, can be skipped with **Review
  later**, and can be reopened from **Setup & permissions** in the header
  or **Setup & Permissions…** in the menu bar icon.
- Release packaging now also attaches the `.pkg` installer in CI, and the
  ZIP installer reports the correct release version instead of an old
  hard-coded one.

## [0.9.0] — 2026-10-21 · Menu bar control + keeper that heals itself
- **Menu bar icon**: while IconCraft is open, its icon sits in the menu
  bar with Open IconCraft, **Keep Skins Applied: On/Off** (live state),
  a plain-English keeper status line, App Management Settings…,
  Support, and Quit. (Your skins never needed the app running — they
  are written to disk; only the repair keeper is background, via a
  LaunchAgent that works with the app closed.)
- **Keeper desired-state healing**: your on/off intent is journalled.
  If anything removes the keep-applied agent (cleaner tools managing
  launch agents do this), IconCraft quietly restores it at launch and
  during its periodic checks — a deleted agent was observed live on
  the maintainer's Mac and is now restored automatically (logged).
- Fixed the menu bar item (and window-close handling) silently dying
  after a while: JXA block-scoped wrappers were being collected by the
  JS engine; they are now function-scoped. Closing the window quits
  the app cleanly again.

## [0.8.2] — 2026-10-21 · No more "couldn't open" dialogs on close
- Killing/force-closing the native window (or the app being replaced
  during an update) is now treated as a normal close: the process
  exits silently instead of reporting "native window failed: signal:
  killed" with the "IconCraft couldn't open" dialog.
- Relaunching while IconCraft is already running exits quietly with
  code 0 instead of the same scary dialog (the app is agent-style and
  may be open on another Space). Genuine launch failures still report.
Regression-tested (SIGKILLed window helper) and proven live: window
process killed → main exits silently, no dialog; second launch →
exit 0, one instance.

## [0.8.1] — 2026-10-21 · App Management permission handling
Fixes the "operation not permitted / rollback failed" dead-end seen on
protected app bundles (macOS App Management, com.apple.macl):
- **Write probe first**: Apply/full-skin/restore now test writability
  inside the bundle *before* journalling anything. If macOS will refuse,
  you get one clear message and nothing is half-changed.
- **No more wedged apps**: when a change is refused atomically, the
  rollback write is skipped (there is nothing to roll back) instead of
  failing and parking the app in "rollback-required" forever.
- **Auto-heal**: scans close out stuck records when the app already
  matches its saved original — including the stale FinderInfo
  custom-icon bit with no icon file, which also no longer fools the
  keeper into thinking a skin is still on. Live result: the two stuck
  apps healed on launch and the keeper re-skinned them (iconsFixed=2).
- **One-click fix**: permission errors now show an "Open App Management
  settings" button that jumps to System Settings → Privacy & Security →
  App Management (fixed allowlisted target, same pattern as support).
- Restore's icon write falls back to a direct write when temp files
  inside the bundle are refused.
Verified in the real GUI via CUA driver: the exact Replace click that
failed in the bug report now applies cleanly (new applied record, icon
in place). Full suite green.

## [0.8.0] — 2026-10-21 · Originals Vault
- **Originals Vault**: before IconCraft changes an app for the first time it
  now saves the original identity — folder/display name, internal
  CFBundleName, and the original icon as a checksummed asset — keyed by the
  app's stable bundle ID (never by path, which changes on rename/move).
- **Restore rebuilds from the vault**: if an app updated, was replaced, or
  moved since skinning (a new identifier), the strict journalled restore
  still refuses to force old files onto a new version — but a confirmed
  user Restore now falls back to the vault and applies the saved original
  name + icon onto the app as it exists today. The old Info.plist is never
  forced onto a changed app; only the version-independent name and icon.
- Vault shown per-app ("Original saved" in the list; details in the rename
  dialog). Safety fix: restores operate on the exact app selected, never
  on another app that merely shares the bundle ID (clones do); the keeper's
  bundle lookup now prefers the journalled path too. Proven live: skinned
  clone → simulated update + folder move → vault restore rebuilt the
  original name/icon/folder, signature valid, duplicate-bundle real app
  verified untouched.

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
