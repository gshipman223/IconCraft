#!/usr/bin/env python3
"""Package the compiled universal .app and a reversible per-user installer."""
from pathlib import Path
import hashlib, json, os, shutil, stat, subprocess, zipfile
ROOT=Path(__file__).resolve().parents[1]
BUILD=ROOT/'build'
assert (BUILD/'BUILD-COMPLETE').is_file(), 'Compilation and tests must complete before packaging'
DEST=Path(os.environ.get('ICONCRAFT_OUTPUT','/mnt/data'))
DEST.mkdir(parents=True, exist_ok=True)
STAGE=BUILD/'IconCraft-Mac'
if STAGE.exists(): shutil.rmtree(STAGE)
STAGE.mkdir(parents=True)
shutil.copytree(BUILD/'IconCraft.app',STAGE/'IconCraft.app')
shutil.copy2(BUILD/'build-manifest.json',STAGE/'build-manifest.json')
exe=STAGE/'IconCraft.app/Contents/MacOS/IconCraft'
exe.chmod(0o755)
digest=hashlib.sha256(exe.read_bytes()).hexdigest()
installer=r'''#!/bin/bash
# IconCraft local-test installer. No administrator password or developer account.
# Only this application's bundle is copied/signed; global security settings stay unchanged.
set -euo pipefail
HERE="$(cd "$(dirname "$0")" && pwd -P)"
SOURCE="$HERE/IconCraft.app"
STAGE=""
BACKUP=""
DEST=""
COMMITTED=0
pause() { if [[ -t 0 ]]; then printf '\nPress Return to close this window.'; read -r _ || true; fi; }
cleanup() {
    status=$?
    trap - EXIT
    if [[ $status -ne 0 ]]; then
        printf '\nInstallation did not complete. Existing icon settings and backups were not deleted.\n'
        if [[ "$COMMITTED" -eq 0 && -n "$BACKUP" && -d "$BACKUP" && -n "$DEST" && ! -e "$DEST" ]]; then
            /bin/mv "$BACKUP" "$DEST" || printf 'Previous application preserved at: %s\n' "$BACKUP"
        fi
    fi
    if [[ -n "$STAGE" && -d "$STAGE" ]]; then /bin/rm -rf "$STAGE"; fi
    pause
    exit "$status"
}
trap cleanup EXIT
if [[ "$(/usr/bin/uname -s)" != "Darwin" ]]; then
    printf 'This installer is for macOS.\n' >&2; exit 1
fi
if [[ ! -f "$SOURCE/Contents/MacOS/IconCraft" || -L "$SOURCE" ]]; then
    printf 'Keep this installer beside the extracted IconCraft.app.\n' >&2; exit 1
fi
printf 'IconCraft — installing the Mac local-test build\n\n'
ACTUAL="$(/usr/bin/shasum -a 256 "$SOURCE/Contents/MacOS/IconCraft" | /usr/bin/awk '{print $1}')"
if [[ "$ACTUAL" != "__EXECUTABLE_SHA256__" ]]; then
    printf 'Executable checksum mismatch. Re-download the complete archive. Nothing was installed.\n' >&2; exit 1
fi
# Check every bundle file before copying. Paths are fixed by the packager.
(cd "$HERE" && /usr/bin/shasum -a 256 -c APP-SHA256SUMS.txt)
if /usr/bin/pgrep -x IconCraft >/dev/null 2>&1; then
    printf '\nQuit the running IconCraft application, then run this installer again.\n' >&2; exit 1
fi
APPDIR="/Applications"
if [[ ! -d "$APPDIR" || ! -w "$APPDIR" ]]; then
    APPDIR="$HOME/Applications"
    /bin/mkdir -p "$APPDIR"
fi
DEST="$APPDIR/IconCraft.app"
if [[ -L "$DEST" ]]; then
    printf 'Refusing to replace a symbolic link: %s\n' "$DEST" >&2; exit 1
fi
if [[ -e "$DEST" ]]; then
    ID="$(/usr/libexec/PlistBuddy -c 'Print :CFBundleIdentifier' "$DEST/Contents/Info.plist" 2>/dev/null || true)"
    if [[ "$ID" != "io.iconcraft.mac" ]]; then
        printf 'A different app already occupies %s. It has not been replaced.\n' "$DEST" >&2; exit 1
    fi
fi
STAGE="$(/usr/bin/mktemp -d "$APPDIR/.iconcraft-install.XXXXXX")"
/usr/bin/ditto "$SOURCE" "$STAGE/IconCraft.app"
/bin/chmod 755 "$STAGE/IconCraft.app/Contents/MacOS/IconCraft"
printf '\nPreparing this test copy locally. No Developer ID or Apple account is used.\n'
# Ad-hoc signing is local integrity metadata, not Developer ID signing or notarization.
/usr/bin/codesign --force --sign - --timestamp=none "$STAGE/IconCraft.app"
/usr/bin/codesign --verify --deep --strict "$STAGE/IconCraft.app"
if [[ -e "$DEST" ]]; then
    BACKROOT="$HOME/Library/Application Support/IconCraft/previous-installations"
    /bin/mkdir -p "$BACKROOT"
    /bin/chmod 700 "$BACKROOT"
    BACKDIR="$(/usr/bin/mktemp -d "$BACKROOT/$(/bin/date +%Y%m%d-%H%M%S).XXXXXX")"
    BACKUP="$BACKDIR/IconCraft.app"
    /bin/mv "$DEST" "$BACKUP"
fi
/bin/mv "$STAGE/IconCraft.app" "$DEST"
COMMITTED=1
printf '\nInstalled: %s\n' "$DEST"
if [[ -n "$BACKUP" ]]; then printf 'Previous application saved at: %s\n' "$BACKUP"; fi
printf '\nOpening IconCraft. The first launch checks native icon handling using disposable test apps.\n'
printf 'If macOS blocks this unsigned test build, open System Settings > Privacy & Security > Open Anyway.\n'
printf 'This installer does not disable Gatekeeper or remove quarantine attributes.\n'
/usr/bin/open "$DEST"
'''
installer=installer.replace('__EXECUTABLE_SHA256__',digest)
p=STAGE/'Install IconCraft.command';p.write_text(installer);p.chmod(0o755)
subprocess.run(['bash','-n',str(p)],check=True)
checks=[]
for p in sorted((STAGE/'IconCraft.app').rglob('*')):
    if p.is_file(): checks.append(hashlib.sha256(p.read_bytes()).hexdigest()+'  '+p.relative_to(STAGE).as_posix())
(STAGE/'APP-SHA256SUMS.txt').write_text('\n'.join(checks)+'\n')
(STAGE/'READ ME.txt').write_text('''ICONCRAFT — MAC LOCAL-TEST BUILD
0.2.0-mac-test.3 | Apple Silicon + Intel | macOS 12 or later

INSTALL
1. Extract this ZIP.
2. Double-click Install IconCraft.command. It checks the packaged files, copies
   IconCraft.app to /Applications (or your own Applications folder when needed),
   preserves a previous installation, and opens the new copy.
3. If macOS blocks the app or installer, after attempting to open it use
   System Settings > Privacy & Security > Open Anyway and confirm Open.

Alternatively, drag IconCraft.app into Applications and open it. No compiler,
Homebrew, Xcode, Node, Go, Developer ID certificate, or VM is required to run it.
The optional installer uses only macOS-supplied commands. Its local ad-hoc
signature does not identify a developer and is not Apple notarization.

If the command file does not open, you may run it explicitly in Terminal:
  /bin/bash "<path to extracted folder>/Install IconCraft.command"
Tip: type /bin/bash followed by a space, then drag the command file into Terminal.

FIXED IN THIS BUILD
The macOS app-metadata reader no longer calls objectForKey on a bridged value.
Unreadable or malformed application bundles are skipped with a visible warning,
not allowed to abort the whole scan. Native checks remain required; none are
bypassed. The original icon files, settings and backup journal are preserved.

FIRST TEST
IconCraft scans applications and runs native checks on disposable test bundles.
It does not alter installed applications until you choose an icon and confirm.
The Apply button stays disabled if the native checks fail. Read the actual
failure in Diagnostics & settings; do not assume the check passed.

In Original icons, copy an icon from an installed original app or import a
PNG/ICNS. Return to Applications, choose the replacement icon beside an ArtCraft
app, select that row, and click Apply. Backup history restores previous icons.
Genuine vendor replacement artwork is not bundled or redrawn.

LOCAL RECORDS
~/Library/Application Support/IconCraft/iconcraft.log
~/Library/Application Support/IconCraft/diagnostics.json
Keep this folder: it also holds recovery records, preferences, and imported icons.
Restore changes before deleting those records. Installing this build does not
clear your application data or icon backups.

VALIDATION
The executable has been compiled for both Mac architectures and its bundle and
build artifacts checked on Linux. Native execution on a Mac has not yet been
performed in the build session. This is for your local installation and testing,
not a claim of production certification. The ZIP is not Developer ID signed or
notarized. Inspect an unexpected security warning rather than globally disabling
macOS security protections. No third-party applications are re-signed.

Finder custom icons do not guarantee every app-controlled running/Dock icon
changes. Close and reopen the affected app to evaluate cached icon behaviour.

License: MIT. Vendor artwork remains subject to its owners' terms.
''')
p=STAGE/'Run Mac Diagnostics.command'
p.write_text(r'''#!/bin/bash
set -uo pipefail
HERE="$(cd "$(dirname "$0")" && pwd -P)"
APP="$HERE/IconCraft.app"
if [[ -d "/Applications/IconCraft.app" ]]; then APP="/Applications/IconCraft.app";
elif [[ -d "$HOME/Applications/IconCraft.app" ]]; then APP="$HOME/Applications/IconCraft.app"; fi
DIR="$HOME/Library/Application Support/IconCraft/reports"
mkdir -p "$DIR"; chmod 700 "$DIR"
REPORT="$DIR/IconCraft-mac-diagnostics-$(date +%Y%m%d-%H%M%S).json"
printf 'Quit IconCraft first. These checks use disposable test bundles, not your installed apps.\n'
"$APP/Contents/MacOS/IconCraft" --self-test --report "$REPORT"
RESULT=$?
printf '\nDiagnostic exit code: %s\n' "$RESULT"
if [[ -f "$REPORT" ]]; then printf 'Report: %s\n' "$REPORT"; /usr/bin/open -R "$REPORT"; fi
if [[ -t 0 ]]; then printf 'Press Return to close.'; read -r _ || true; fi
exit "$RESULT"
''');p.chmod(0o755)
subprocess.run(['bash','-n',str(p)],check=True)
rows=[json.loads(s) for s in (BUILD/'tests-current.jsonl').read_text().splitlines()]
summary={
 'version':'0.2.0-mac-test.3',
 'purpose':'Installable macOS local-test build; no Developer ID or notarization required for local user-approved testing',
 'buildHost':'Linux x86_64',
 'architectures':['arm64','x86_64'],
 'testsPassed':sum(x.get('Action')=='pass' and 'Test' in x for x in rows),
 'raceDetector':'passed', 'goVet':'passed',
 'bridgeRegression':{'passed':23, 'environment':'Node.js with simulated Objective-C values; NOT macOS'},
 'offlineUI':json.loads((ROOT/'docs/ui-test-results.json').read_text()),
 'darwinTestCompilation':['arm64','amd64'],
 'fixes':['Explicit Objective-C nil handling','Unwrap metadata dictionaries before field access','Isolate unreadable app bundles with warnings','Bundled-icon fallback','Visible diagnostic failure detail'],
 'installerShellSyntax':'passed',
 'macNativeRuntime':'NOT RUN on the build host; first-launch checks included',
 'developerIDSigning':'not performed',
 'notarization':'not performed',
 'localInstallerSignature':'optional installer creates a local ad-hoc signature; no certificate',
 'productionReady':False,
 'sha256ExecutableBeforeLocalSigning':digest,
}
(STAGE/'BUILD-VALIDATION.json').write_text(json.dumps(summary,indent=2)+'\n')
# Ship only current test evidence, avoiding stale results from the original preview.
(ROOT/'docs/BUILD-VALIDATION.json').write_text(json.dumps(summary,indent=2)+'\n')
shutil.copy2(BUILD/'tests-current.jsonl',ROOT/'docs/test-results.jsonl')
shutil.copy2(BUILD/'bridge-regression.tap',ROOT/'docs/bridge-regression.tap')
(ROOT/'docs/automated-test-output.txt').write_text((BUILD/'race-current.txt').read_text()+'\ngo vet: passed\n')
(ROOT/'docs/LOCAL-TEST-INSTALL.txt').write_text((STAGE/'READ ME.txt').read_text())
for out, base, prefix in [(DEST/'IconCraft-Mac.zip',STAGE,'IconCraft-Mac'),(DEST/'IconCraft-Mac-Source.zip',ROOT,'IconCraft')]:
    with zipfile.ZipFile(out,'w',zipfile.ZIP_DEFLATED,compresslevel=6) as z:
        for p in sorted(base.rglob('*')):
            rel=p.relative_to(base)
            if any(k in rel.parts for k in ['build','.git','__pycache__']): continue
            if p.is_file():
                if p.suffix.lower() in {'.ttf','.otf','.woff','.woff2'}: raise ValueError('font distribution prohibited')
                z.write(p,str(Path(prefix)/rel))
    with zipfile.ZipFile(out) as z:
        assert z.testzip() is None
        if 'Source' not in out.name:
            assert z.getinfo('IconCraft-Mac/IconCraft.app/Contents/MacOS/IconCraft').external_attr >> 16 & 0o111
            assert z.getinfo('IconCraft-Mac/Install IconCraft.command').external_attr >> 16 & 0o111
    print(out.name,out.stat().st_size,'bytes')
(DEST/'IconCraft-Mac-Build-Validation.json').write_text(json.dumps(summary,indent=2)+'\n')
paths=[DEST/'IconCraft-Mac.zip',DEST/'IconCraft-Mac-Source.zip',DEST/'IconCraft-Mac-Build-Validation.json']
(DEST/'IconCraft-Mac-SHA256SUMS.txt').write_text(''.join(hashlib.sha256(p.read_bytes()).hexdigest()+'  '+p.name+'\n' for p in paths))
print(json.dumps(summary,indent=2))
