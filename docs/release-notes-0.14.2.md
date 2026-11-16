# IconCraft 0.14.2

Universal (Apple Silicon + Intel), macOS 13 or newer. Signed with a
Developer ID and notarised by Apple — double-click and it opens.

This release keeps vendor launchers and updaters able to find apps you
customise, and adds optional, clearly warned ways to go further. Tested
on a Mac with the 0.14.2-test4 build (same source as the merged pull
request, then this version bump). Not tested on Intel hardware.

## Supported Macs

macOS 13 or newer, Apple Silicon and Intel. `LSMinimumSystemVersion` is
13.0. IconCraft itself is signed with Developer ID and notarised.

## What 0.14.2 changes

- **Launcher-safe icons are the default (Icon only).** The app's bundle
  path, name and ID are left alone. A custom name is a label inside
  IconCraft. The custom icon is a Finder custom icon, put back after an
  update. Finder, Launchpad, the Dock and the menu bar keep the
  publisher's name.
- **Full rename is optional.** It renames the `.app` folder so the
  custom name shows in Finder, the Dock and Launchpad. It still does not
  edit `Info.plist`. Turning it on asks for confirmation, because a
  launcher that looks up the publisher's folder may not recognise the
  app and may install a second copy. Each app can override the global
  choice.
- **Pause for updates** puts original names and icons back so launchers
  and updaters can find their apps, and turns the custom names and icons
  back on when you switch it off.
- **Full icon is optional**, including **Also change document icons**.
  That replaces the icon file inside the bundle (and, if you ask, the
  document icons that app uses) so the running Dock tile and some
  document views can change. It ad-hoc re-signs the app and **removes
  the publisher's Developer ID signature**, which cannot be put back.
  Sparkle, the App Store and Gatekeeper may stop treating the app as the
  publisher's. Turning it on shows that warning and asks for
  confirmation.
- **Repair.** Apps that an older IconCraft version renamed or re-signed
  are moved back to their original names when that path is free, and the
  custom name is kept as a label. A second copy is never deleted.

## What this release does not do

- It does not reconstruct a publisher signature after Full icon.
- Mail and other apps that are already open can keep a previous document
  icon until they are quit and opened again.
- A document icon that lives only in an asset catalog cannot be
  replaced.
- Windows and Linux ports are not in this release.

## Install

1. Disk image (recommended): open `IconCraft-Mac-0.14.2.dmg` and drag
   IconCraft into Applications.
2. ZIP: unzip `IconCraft-Mac-0.14.2.zip` and drag `IconCraft.app` into
   Applications.
3. Open IconCraft and complete the first-run Setup & Permissions review.

Verify any time with:

```sh
spctl --assess --type execute /Applications/IconCraft.app
```

It should report `accepted, source=Notarized Developer ID`. Checksums
are in `IconCraft-Mac-0.14.2-SHA256SUMS.txt` on the GitHub release.
