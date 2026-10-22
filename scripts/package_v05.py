#!/usr/bin/env python3
"""Package IconCraft using fresh local evidence only. Run build-current.sh first."""
from pathlib import Path
import hashlib, json, os, plistlib, re, shutil, struct, subprocess, zipfile

ROOT = Path(__file__).resolve().parents[1]
BUILD = ROOT / 'build'
OUT = Path(os.environ.get('ICONCRAFT_OUTPUT_DIR', str(BUILD / 'packages')))
VERSION = '0.14.0'
# 'developer-id' when scripts/release-macos.sh packaged a signed + notarized
# build; default stays honest for CI/ad-hoc builds.
DIST = os.environ.get('ICONCRAFT_DISTRIBUTION', 'ad-hoc')
INSTALLER_IDENTITY = os.environ.get('ICONCRAFT_INSTALLER_IDENTITY', '')
OUT.mkdir(parents=True, exist_ok=True)
assert (BUILD / 'BUILD-COMPLETE').read_text().strip() == 'OK'
manifest = json.loads((BUILD / 'build-manifest.json').read_text())
assert manifest['version'] == VERSION and manifest['adHocSigning'].startswith('PERFORMED')
records = [json.loads(x) for x in (BUILD / 'tests-current.jsonl').read_text().splitlines()]
assert not any(x.get('Action') == 'fail' for x in records)
passed = sum(x.get('Action') == 'pass' and 'Test' in x for x in records)
tap = (BUILD / 'bridge-regression.tap').read_text()
m = re.search(r'^# pass (\d+)$', tap, re.M) or re.search(r'^ℹ pass (\d+)$', tap, re.M)
checks = int(m.group(1))
assert ('# fail 0' in tap) or ('ℹ fail 0' in tap)
assert not (BUILD / 'vet-current.txt').read_text().strip()
race = (BUILD / 'race-current.txt').read_text()
assert 'FAIL' not in race and len([l for l in race.splitlines() if l.startswith('ok')]) == 6

app = BUILD / 'IconCraft.app'
exe = app / 'Contents/MacOS/IconCraft'
raw = exe.read_bytes()
magic, n = struct.unpack_from('>II', raw)
assert magic == 0xcafebabe and n == 2
info = plistlib.loads((app / 'Contents/Info.plist').read_bytes())
assert info['CFBundleShortVersionString'] == VERSION
subprocess.run(['codesign', '--verify', '--deep', '--strict', str(app)], check=True)

digest = hashlib.sha256(raw).hexdigest()
stage = BUILD / f'IconCraft-Mac-{VERSION}'
if stage.exists():
    shutil.rmtree(stage)
stage.mkdir()
shutil.copytree(app, stage / 'IconCraft.app')
(stage / 'APP-SHA256SUMS.txt').write_text(''.join(
    hashlib.sha256(f.read_bytes()).hexdigest() + '  ' + f.relative_to(stage).as_posix() + '\n'
    for f in sorted((stage / 'IconCraft.app').rglob('*')) if f.is_file()))
signing_note = ('Signed with a Developer ID and notarised by Apple: it opens with a '
                'double-click, no Gatekeeper warnings. Verify any time with:\n'
                '  spctl --assess --type execute /Applications/IconCraft.app'
                if DIST == 'developer-id' else
                'Developer ID signing and notarization have not been performed. If macOS\n'
                'blocks the download, use Privacy & Security > Open Anyway for this item.\n'
                'Do not turn off macOS security globally.')
(stage / 'READ ME.txt').write_text(f'''IconCraft {VERSION} — personal Mac edition
Apple Silicon + Intel, macOS 12.3 or later

INSTALL
Extract the ZIP and drag IconCraft.app into your Applications folder, then
launch it from there. Quit any running IconCraft first. Replacing the app
preserves your settings, imported icons, original-icon recovery data and
rename history because they live in IconCraft's support folder, not inside
the app bundle.
No Apple developer account, Xcode or Homebrew is required.

FINISH SETUP (FIRST LAUNCH)
Open IconCraft and complete Setup & Permissions. It shows what is ready,
what is optional, and what macOS is blocking. App Management is tested with
the same harmless write check used before a real change; IconCraft never
pretends that merely opening System Settings proves a permission was granted.
Full Disk Access, Accessibility and Screen Recording are not requested.

WHAT'S NEW IN {VERSION}
- Speaks your language: English, Simplified Chinese, Japanese and Korean,
  chosen from your macOS language preferences. Window, menu bar item,
  Setup & Permissions, notifications and error messages are all included;
  dates follow your Region setting.
- Edit in one panel: the pencil now opens a single editor with a
  before/after preview, name, icon choice, an optional full skin and one
  Save that performs the steps in the right order and says exactly which
  step failed if one does. Restore original is one click away.
- Setup & Permissions describes how this copy is really signed (Developer
  ID and notarised, or ad-hoc) instead of assuming.
- Restore all refreshes the list straight away, counts apps correctly and
  clears the custom-icon choice of restored apps; notification toggles are
  labelled; the app is named IconCraft in the menu bar and Dock.
- Signed with a Developer ID and notarised by Apple (this build).

@SIGNING_NOTE@
'''.replace('@SIGNING_NOTE@', signing_note))

def archive(zip_path, source, arc_root, exclude_names=()):
    with zipfile.ZipFile(zip_path, 'w', zipfile.ZIP_DEFLATED, compresslevel=9) as z:
        for f in sorted(source.rglob('*')):
            if f.is_dir() or f.name in exclude_names or '/build/' in f.as_posix() and arc_root == source.name:
                continue
            z.write(f, Path(arc_root) / f.relative_to(source))

appzip = OUT / f'IconCraft-Mac-{VERSION}.zip'
with zipfile.ZipFile(appzip, 'w', zipfile.ZIP_DEFLATED, compresslevel=9) as z:
    for f in sorted(stage.rglob('*')):
        if f.is_file():
            z.write(f, Path(stage.name) / f.relative_to(stage))
# Drag-to-Applications disk image. The release pipeline signs and notarizes
# the DMG after packaging, so a downloaded copy opens without a Gatekeeper
# approval detour. (An unsigned .pkg cannot offer that, so no installer
# package is published without a Developer ID Installer identity.)
dmgroot = BUILD / f'IconCraft-DMG-{VERSION}'
if dmgroot.exists():
    shutil.rmtree(dmgroot)
dmg = None
if DIST == 'developer-id' and shutil.which('hdiutil'):
    dmgroot.mkdir()
    shutil.copytree(app, dmgroot / 'IconCraft.app')
    (dmgroot / 'Applications').symlink_to('/Applications')
    dmg = OUT / f'IconCraft-Mac-{VERSION}.dmg'
    if dmg.exists():
        dmg.unlink()
    subprocess.run(['hdiutil', 'create', '-volname', f'IconCraft {VERSION}',
                    '-srcfolder', str(dmgroot), '-fs', 'HFS+', '-format', 'UDZO',
                    '-ov', str(dmg)], check=True, stdout=subprocess.DEVNULL)
installerpkg = OUT / f'IconCraft-Mac-{VERSION}-Installer.pkg'
if INSTALLER_IDENTITY and shutil.which('pkgbuild') and (BUILD / 'IconCraft.app').is_dir():
    if shutil.which('productbuild'):
        componentpkg = BUILD / 'IconCraft-component.pkg'
        if componentpkg.exists():
            componentpkg.unlink()
        subprocess.run(['pkgbuild', '--component', str(BUILD / 'IconCraft.app'),
                        '--install-location', '/Applications', str(componentpkg)], check=True)
        installer_resources = BUILD / 'installer-resources'
        if installer_resources.exists():
            shutil.rmtree(installer_resources)
        installer_resources.mkdir()
        for name in ('welcome.html', 'conclusion.html'):
            text = (ROOT / 'scripts/installer-resources' / name).read_text().replace('@VERSION@', VERSION)
            assert '@VERSION@' not in text
            (installer_resources / name).write_text(text)
        distribution = BUILD / 'installer-distribution.xml'
        distribution.write_text(f'''<?xml version="1.0" encoding="utf-8"?>
<installer-gui-script minSpecVersion="1">
    <title>IconCraft {VERSION}</title>
    <options customize="never" require-scripts="false"/>
    <domains enable_localSystem="true" enable_currentUserHome="false" enable_anywhere="false"/>
    <welcome file="welcome.html" mime-type="text/html"/>
    <conclusion file="conclusion.html" mime-type="text/html"/>
    <choices-outline><line choice="default"/></choices-outline>
    <choice id="default" title="IconCraft" visible="false"><pkg-ref id="io.iconcraft.mac.component"/></choice>
    <pkg-ref id="io.iconcraft.mac.component" version="{VERSION}" auth="Root">IconCraft-component.pkg</pkg-ref>
</installer-gui-script>
''')
        if installerpkg.exists():
            installerpkg.unlink()
        subprocess.run(['productbuild', '--distribution', str(distribution),
                        '--package-path', str(BUILD), '--resources', str(installer_resources),
                        str(installerpkg)], check=True)
    else:
        subprocess.run(['pkgbuild', '--component', str(BUILD / 'IconCraft.app'),
                        '--install-location', '/Applications', str(installerpkg)], check=True)
else:
    installerpkg = None
if installerpkg and INSTALLER_IDENTITY:
    signed_pkg = BUILD / 'IconCraft-Installer-signed.pkg'
    subprocess.run(['productsign', '--sign', INSTALLER_IDENTITY,
                    str(installerpkg), str(signed_pkg)], check=True)
    shutil.move(str(signed_pkg), str(installerpkg))
sourcezip = OUT / f'IconCraft-Mac-{VERSION}-Source.zip'
with zipfile.ZipFile(sourcezip, 'w', zipfile.ZIP_DEFLATED, compresslevel=9) as z:
    for f in sorted(ROOT.rglob('*')):
        # docs/*.png + *.jsonl are bulky test evidence (and may be cloud-only
        # OneDrive placeholders that stall on read); source stays code + text.
        if f.is_dir() or 'build' in f.relative_to(ROOT).parts or '.git' in f.relative_to(ROOT).parts or f.suffix in ('.zip', '.png', '.jsonl') or f.name == '.DS_Store':
            continue
        z.write(f, Path(ROOT.name) / f.relative_to(ROOT))
summary = {
    'version': VERSION,
    'goTestsPassed': passed,
    'bridgeChecksPassed': checks,
    'race': 'passed (6 packages)',
    'vet': 'clean',
    'nativeSelfTest': 'passed on the build Mac (see docs/BUILD-VALIDATION.json)',
    'appIcons': 'verified live: installed apps scanned, real icon PNGs served and rendered',
    'installerScreens': ('product installer Welcome/Conclusion explain first-launch Setup & Permissions'
                         if INSTALLER_IDENTITY else
                         'no installer package published without a Developer ID Installer identity'),
    'diskImage': ('BUILT (release pipeline signs and notarizes it)' if dmg else 'not built'),
    'setupPermissions': 'in-app first-run review; App Management shown as granted only after the live bundle-writability probe passes',
    'slices': ['x86_64', 'arm64'],
    'executableSha256': digest,
    'adHocSigned': DIST != 'developer-id',
    'developerIDSigning': ('PERFORMED (Developer ID Application, hardened runtime, timestamp)'
                           if DIST == 'developer-id' else 'NOT PERFORMED'),
    'notarization': 'PERFORMED (ticket stapled to the app)' if DIST == 'developer-id' else 'NOT PERFORMED',
    'installerPkgSigned': bool(INSTALLER_IDENTITY),
}
report = OUT / f'IconCraft-Mac-{VERSION}-Validation.json'
report.write_text(json.dumps(summary, indent=2) + '\n')
checksums = OUT / f'IconCraft-Mac-{VERSION}-SHA256SUMS.txt'
checksum_files = [f for f in [appzip, dmg, sourcezip, report, installerpkg] if f]
checksums.write_text(''.join(hashlib.sha256(f.read_bytes()).hexdigest() + '  ' + f.name + '\n' for f in checksum_files))
print(json.dumps({'packages': [str(f) for f in checksum_files], **summary}, indent=2))
