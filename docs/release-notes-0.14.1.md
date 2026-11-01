# IconCraft 0.14.1

Developer: **Gavin Shipman**.

That name is the copyright holder in the app's `Info.plist`
(`NSHumanReadableCopyright`), the author in `website/index.html`, and the
name on the README. `docs/BUILD-VALIDATION.json` records the 0.13.1
signing identity as `Developer ID Application: Gavin Shipman (65GNPK6TDT)`.
This document does not claim that certificate was used to sign 0.14.1.
Nothing in 0.14.1 was signed or notarised here.

Version `0.14.1` is `internal/core/types.go` `Version`. The built
`Info.plist` uses that string as `CFBundleShortVersionString` and
`CFBundleVersion` `25` (the previous source value was `24`).

These notes describe only behaviour read from this tree and from issues
[#10](https://github.com/gshipman223/IconCraft/issues/10) and
[#11](https://github.com/gshipman223/IconCraft/issues/11). They do not
describe a run on macOS.

## Supported Macs

- macOS **13 or newer** (`LSMinimumSystemVersion` `13.0`).
- Universal: Intel (`x86_64` / `darwin/amd64`) and Apple Silicon (`arm64`).
- 0.14.0's README and generated `Info.plist` said `12.3`. Those current
  statements now say 13. Older notes in `docs/CHANGES-0.4.1.md` and
  `docs/CHANGES-0.5.0.md` still record that those older releases stated
  12.3, and they say 0.14.1 requires macOS 13 or newer.

## What was broken

### Issue #10

[Issue #10](https://github.com/gshipman223/IconCraft/issues/10) reports
IconCraft **0.14.0** on **macOS 27.2 beta RC**, Apple Silicon (M2).
Replacing an icon reported that macOS was blocking the change through App
Management, with “0 icons replaced” and “Nothing was changed”, for every
app tried, including after App Management was turned on for IconCraft.
The report names Bitwarden as the screenshot example and says other apps
failed the same way. It does not include an errno.

In 0.14.0 the pre-change probe treated `os.ErrPermission` as App
Management. In Go 1.23.2, `syscall.Errno.Is` reports both `EPERM` and
`EACCES` as `os.ErrPermission` (`src/syscall/syscall_unix.go`). `EROFS`
does not. The 0.14.0 message ended with “Nothing was changed”, which is
the text returned when that probe fails, before a journal record is
written and before `NSWorkspace.setIcon`. So the failure the report
describes happened at the temporary-file probe (`os.CreateTemp` inside
the app bundle), not after a partial icon write. The issue does not say
whether the errno was EPERM or EACCES.

### Issue #11

[Issue #11](https://github.com/gshipman223/IconCraft/issues/11) says the
Setup row “App Management — changing other apps” does not pass after the
permission is removed and added back. It does not state a macOS version.
That row is `CheckAppAccess`: the same `CreateTemp` probe, on one
installed app. In 0.14.0 any `os.ErrPermission` was status `blocked` and
told the user to turn App Management on, and the first such app stopped
the automatic check.

The probe is not a read of the TCC database. `permissions.go` still says
macOS does not give the app a reliable global App Management readout.
Opening System Settings does not mark the row granted. Only a successful
create-and-remove of a temporary file inside one app does.

## What 0.14.1 changes

### Probe errors

`internal/core/writefail.go` classifies the probe error before any
message is chosen:

| Result | Errno | What the user is told | Setup status | Open App Management button |
| --- | --- | --- | --- | --- |
| Operation not permitted | `EPERM` | Reported as App Management. If IconCraft is off, switch it on and try again. If it is already on, that switch is not the remaining cause. Nothing was changed. | `blocked` | Shown |
| Permission denied | `EACCES` | This account cannot add a file inside the app. If the bundle owner uid is 0, the message says the app is owned by root. | `unavailable`, label “Could not test”, title “Write access — changing other apps” | Not shown |
| Read-only file system | `EROFS` | The app is on a read-only disk. | same as EACCES | Not shown |
| Any other probe error | — | “could not write inside …”, with the original error | `unknown` on Setup, and the automatic check tries the next app | Not shown for that text |

A bare `os.ErrPermission` (the Go sentinel, text “permission denied”, no
errno) is classified as permission denied, not App Management. A text
error whose message contains “operation not permitted” and no errno is
classified as App Management. “read-only file system” is the read-only
case.

The notice button in the window is shown only when the error text
contains “App Management” (or the translated `term.appManagement`). The
EACCES and EROFS sentences do not. Setup's own button is omitted when
the row state is `unavailable`.

The signing sentence (ad-hoc copy may be a new app to macOS; Developer ID
copy keeps the permission across updates) is appended only when the row
is not `unavailable`. Appending it to an ownership or read-only result
was telling the user to open App Management for a failure that is not
that result.

English, Simplified Chinese, Japanese and Korean catalogues carry the
new sentences. The engine still stores the English sentence; the server
translates it on the way out.

### Setup check

Automatic check (`appID` empty), in `internal/core/setup.go`:

- Skips protected paths and paths `SafeApp` rejects.
- Skips a bundle whose `lstat` owner uid is 0, and tries a later app.
- Skips an app the native layer reports as running.
- On EPERM, stops and records `blocked` for that app.
- On EACCES or EROFS, records the failure and tries the next app, up to
  12 probes.
- If every candidate was root-owned and none was skipped for being
  running, the row is `unavailable` and says root-owned apps were
  skipped. It does not say App Management is off.
- An explicit app id is probed even when the owner is root.

A success is recorded for that one app. It is not a global TCC grant.

### Icon set

`NSWorkspace.setIconForFileOptions` in `internal/native/bridge.js`
returns a boolean. It does not return an errno. 0.14.0's false result
always said to check write access and App Management. A real change
calls `Set` only after `probeWritable` has succeeded (`Apply` in
`engine.go`, and the icon-fit paths in `iconfit.go`). 0.14.1's false
result is: “macOS refused to set the icon. No application contents were
replaced.” It does not name App Management, so the notice button is not
shown.

`diagnostics.go` also calls `Set` directly when seeding a fixture icon
during `--self-test`. That path uses the same sentence.

Restore's direct icon-file fallback (`internal/native/xattr_darwin.go`)
still runs only when the temporary file fails with the EPERM
classification (`AppManagementRefusal`). EACCES and EROFS return that
error and do not try the direct write.

### What this release does not do

If the probe still returns EPERM while App Management is already on, the
write is still refused and Setup still does not pass. The message says
so. No change here makes `CreateTemp` succeed in that case. It has not
been run on macOS 27, which is the system in issue #10 (27.2 beta RC).
Issue #11 does not state an OS version.

`CreateTemp` inside the bundle and `NSWorkspace.setIcon` are different
operations. This tree does not contain a measurement of whether they
agree on macOS 27. `docs/MAC-ACCEPTANCE.md` lists that comparison as
not run.

No journal format changed. A failed probe still journals nothing.

## Install

0.14.1 packages are not attached to a GitHub release by this change.
After they are published, the install the README describes is:

1. Quit any IconCraft that is already running.
2. Disk image: open `IconCraft-Mac-0.14.1.dmg` and drag IconCraft into
   Applications. ZIP: unzip `IconCraft-Mac-0.14.1.zip` and drag
   `IconCraft.app` into Applications.
3. Open IconCraft from Applications and choose **Run checks**.
4. If the App Management row is blocked, the text says “operation not
   permitted”. Use **Open App Management**, switch this copy on if it is
   off, return, and choose **Check again**. If it is already on, the
   same text says that switch is not the remaining cause. Turning it on
   again is not something this version claims will clear an EPERM result.
5. If the row says **Could not test**, read it. That is file permissions
   or a read-only disk. The Open App Management button is not on that
   row.
6. **Granted — tested** means the temporary-file probe succeeded on the
   app named in the row.
7. Choose whether the optional background helper stays on, then
   **Finish setup**.

Settings, backups and rename history stay in IconCraft's support folder,
not in the app bundle. Replacing the app does not delete them. That is
the install note in `scripts/package_v05.py`.

Full Disk Access, Accessibility and Screen Recording are not requested.
That sentence is in the permissions payload.

## Checksums

Not computed. The signed disk image, ZIP and checksum file do not exist
until the Mac release script below has run. Placeholders:

```
IconCraft-Mac-0.14.1.dmg:                 (pending — not built on a Mac)
IconCraft-Mac-0.14.1.zip:                 (pending — not built on a Mac)
IconCraft-Mac-0.14.1-Source.zip:          (pending — not built on a Mac)
IconCraft-Mac-0.14.1-Validation.json:     (pending — not built on a Mac)
IconCraft-Mac-0.14.1-Installer.pkg:       (pending — only if a Developer ID Installer identity is set)
IconCraft-Mac-0.14.1-SHA256SUMS.txt:      (pending — written by scripts/package_v05.py and refreshed after the DMG is signed)
```

When `scripts/release-macos.sh` finishes, the checksum file is
`build/packages/IconCraft-Mac-0.14.1-SHA256SUMS.txt`. Replace this
section with those lines. Do not invent them.

## What still has to happen on a Mac

This environment is Linux. It cannot sign with a Developer ID or
notarise. `lipo` and `codesign` are not on `PATH`. `/usr/bin/clang` is
the host Linux compiler. Compiling the helper with the Mac flags failed
before any framework was linked:

```
clang -fobjc-arc -mmacosx-version-min=13.0 -arch arm64 \
  -framework Foundation -framework AppKit \
  -framework UniformTypeIdentifiers -framework CoreServices \
  scripts/defaults-helper.m -o /tmp/iconcraft-defaults-arm64
```

```
clang: warning: argument unused during compilation: '-mmacosx-version-min=13.0' [-Wunused-command-line-argument]
clang: error: unsupported option '-arch' for target 'x86_64-pc-linux-gnu'
```

Exit status 1. The Go slices do compile here with `CGO_ENABLED=0`.
On this machine the Go 1.23.2 linker wrote `LC_BUILD_VERSION` 11.0.0 in
both `darwin/amd64` and `darwin/arm64` slices; `raise_minos` in
`scripts/build_macos.py` rewrote each to 13.0.0. The assembled
`build/IconCraft.app/Contents/MacOS/IconCraft` is a fat Mach-O
(`0xcafebabe`) and its `Info.plist` has `LSMinimumSystemVersion` 13.0,
`CFBundleShortVersionString` 0.14.1 and `CFBundleVersion` 25. The
defaults helper was not in that bundle. Ad-hoc signing, Developer ID
signing and notarisation were not performed.

On the Mac, from a checkout of this commit, with the Developer ID
Application identity in the login keychain and an App Store Connect API
key (`.p8`) kept outside the repo:

```sh
export SIGNING_IDENTITY="Developer ID Application: Gavin Shipman (TEAMID)"
export ASC_KEY="$HOME/.appstoreconnect/private_keys/AuthKey_XXXXXXXXXX.p8"
export ASC_KEY_ID="XXXXXXXXXX"
export ASC_ISSUER="xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx"
# optional; without it no installer .pkg is published
export INSTALLER_IDENTITY="Developer ID Installer: Gavin Shipman (TEAMID)"

bash scripts/release-macos.sh
```

`scripts/release-macos.sh` refuses to run unless `uname` is Darwin. It
then:

1. `bash build-current.sh`, which runs `scripts/build_macos.py`. That
   builds both Go slices with `CGO_ENABLED=0`, raises each slice's
   `LC_BUILD_VERSION` minos to 13.0.0 when the linker wrote a lower
   value, ad-hoc signs each slice, and runs:

   ```sh
   lipo -create build/iconcraft-amd64 build/iconcraft-arm64 \
     -output build/IconCraft.app/Contents/MacOS/IconCraft
   clang -fobjc-arc -mmacosx-version-min=13.0 -arch arm64 \
     -framework Foundation -framework AppKit \
     -framework UniformTypeIdentifiers -framework CoreServices \
     scripts/defaults-helper.m -o build/iconcraft-defaults-arm64
   clang -fobjc-arc -mmacosx-version-min=13.0 -arch x86_64 \
     -framework Foundation -framework AppKit \
     -framework UniformTypeIdentifiers -framework CoreServices \
     scripts/defaults-helper.m -o build/iconcraft-defaults-x86_64
   lipo -create build/iconcraft-defaults-arm64 build/iconcraft-defaults-x86_64 \
     -output build/IconCraft.app/Contents/MacOS/iconcraft-defaults
   ```

   `Info.plist` `LSMinimumSystemVersion` is `13.0`.

2. Checks `lipo -info` on both binaries reports `x86_64` and `arm64`,
   and that `LSMinimumSystemVersion` is `13.0`.

3. Runs `IconCraft --self-test`.

4. Developer ID, hardened runtime, timestamp:

   ```sh
   codesign --force --options runtime --timestamp --sign "$SIGNING_IDENTITY" \
     build/IconCraft.app/Contents/MacOS/iconcraft-defaults
   codesign --force --options runtime --timestamp --sign "$SIGNING_IDENTITY" \
     build/IconCraft.app/Contents/MacOS/IconCraft
   codesign --force --options runtime --timestamp --sign "$SIGNING_IDENTITY" \
     build/IconCraft.app
   codesign --verify --deep --strict --verbose=2 build/IconCraft.app
   ```

5. Notarise and staple the app:

   ```sh
   ditto -c -k --keepParent build/IconCraft.app build/IconCraft-notarization.zip
   xcrun notarytool submit build/IconCraft-notarization.zip \
     --key "$ASC_KEY" --key-id "$ASC_KEY_ID" --issuer "$ASC_ISSUER" --wait
   xcrun stapler staple build/IconCraft.app
   xcrun stapler validate build/IconCraft.app
   spctl --assess --type execute --verbose=4 build/IconCraft.app
   ```

6. `ICONCRAFT_DISTRIBUTION=developer-id python3 scripts/package_v05.py`
   writes the ZIP, the DMG (`hdiutil create`), the validation JSON, and
   `IconCraft-Mac-0.14.1-SHA256SUMS.txt`. A `.pkg` is built only when
   `INSTALLER_IDENTITY` is set.

7. Signs, notarises and staples the DMG, then rewrites the checksum file
   with `shasum -a 256` over the ZIP, DMG, source ZIP and validation JSON.

Tagging, `gh release upload`, replacing CI assets, and deploying
`website/` are not part of that script and were not done for 0.14.1.
`docs/RELEASING.md` has the upload command for after the packages exist.
`docs/MAC-ACCEPTANCE.md` is still **NOT RUN** for the App Management
cases above.
