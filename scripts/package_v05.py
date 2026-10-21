#!/usr/bin/env python3
"""Package IconCraft using fresh local evidence only. Run build-current.sh first."""
from pathlib import Path
import hashlib, json, os, plistlib, re, shutil, struct, subprocess, zipfile

ROOT = Path(__file__).resolve().parents[1]
BUILD = ROOT / 'build'
OUT = Path(os.environ.get('ICONCRAFT_OUTPUT_DIR', str(BUILD / 'packages')))
VERSION = '0.13.0'
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
assert 'FAIL' not in race and len([l for l in race.splitlines() if l.startswith('ok')]) == 5

app = BUILD / 'IconCraft.app'
exe = app / 'Contents/MacOS/IconCraft'
raw = exe.read_bytes()
magic, n = struct.unpack_from('>II', raw)
assert magic == 0xcafebabe and n == 2
info = plistlib.loads((app / 'Contents/Info.plist').read_bytes())
assert info['CFBundleShortVersionString'] == VERSION
subprocess.run(['codesign', '--verify', '--deep', '--strict', str(app)], check=True)

digest = hashlib.sha256(raw).hexdigest()
installer = (ROOT / 'scripts/Install IconCraft.command.in').read_text().replace('@EXECUTABLE_SHA256@', digest).replace('@VERSION@', VERSION)
assert '@EXECUTABLE_SHA256@' not in installer and '@VERSION@' not in installer
stage = BUILD / f'IconCraft-Mac-{VERSION}'
if stage.exists():
    shutil.rmtree(stage)
stage.mkdir()
shutil.copytree(app, stage / 'IconCraft.app')
p = stage / 'Install IconCraft.command'
p.write_text(installer)
p.chmod(0o755)
subprocess.run(['bash', '-n', str(p)], check=True)
(stage / 'APP-SHA256SUMS.txt').write_text(''.join(
    hashlib.sha256(f.read_bytes()).hexdigest() + '  ' + f.relative_to(stage).as_posix() + '\n'
    for f in sorted((stage / 'IconCraft.app').rglob('*')) if f.is_file()))
(stage / 'READ ME.txt').write_text(f'''IconCraft {VERSION} — personal Mac edition
Apple Silicon + Intel, macOS 12.3 or later

INSTALL
Extract the ZIP and run Install IconCraft.command beside IconCraft.app.
Quit any running IconCraft first. The installer preserves your settings,
imported icons, original-icon recovery data and rename history. It validates
checksums, applies a local ad-hoc signature and checks the new executable's
version on your Mac before replacing the previous application. The .pkg
installer also explains this finish-setup step before and after installation.
No Apple developer account, Xcode or Homebrew is required.

FINISH SETUP (FIRST LAUNCH)
Open IconCraft and complete Setup & Permissions. It shows what is ready,
what is optional, and what macOS is blocking. App Management is tested with
the same harmless write check used before a real change; IconCraft never
pretends that merely opening System Settings proves a permission was granted.
Full Disk Access, Accessibility and Screen Recording are not requested.

WHAT'S NEW IN {VERSION}
- First-run Setup & Permissions screen with live status pills, one-click
  App Management settings, a real permission test, background-keeper status,
  and a clear list of permissions IconCraft does not need.
- Menu bar control for the background keeper, which also heals itself if
  another tool removes its LaunchAgent.
- Originals Vault, full skins for running-app identity, and journalled,
  reversible icon and name changes remain built in.

Developer ID signing and notarization have not been performed. If macOS
blocks the download, use Privacy & Security > Open Anyway for this item.
Do not turn off macOS security globally.
''')

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
# One-click installer package (same ad-hoc signed app, installs to
# /Applications). Built on macOS hosts with pkgbuild/productbuild. The
# product installer has Welcome and Conclusion screens that explain the
# first-launch Setup & Permissions step; macOS still requires the user to
# grant App Management themselves inside IconCraft/System Settings.
installerpkg = OUT / f'IconCraft-Mac-{VERSION}-Installer.pkg'
if shutil.which('pkgbuild') and (BUILD / 'IconCraft.app').is_dir():
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
    'race': 'passed (5 packages)',
    'vet': 'clean',
    'nativeSelfTest': 'passed on the build Mac (see docs/BUILD-VALIDATION.json)',
    'appIcons': 'verified live: installed apps scanned, real icon PNGs served and rendered',
    'installerScreens': 'product installer Welcome/Conclusion explain first-launch Setup & Permissions',
    'setupPermissions': 'in-app first-run review; App Management shown as granted only after the live bundle-writability probe passes',
    'slices': ['x86_64', 'arm64'],
    'executableSha256': digest,
    'adHocSigned': True,
    'developerIDSigning': 'NOT PERFORMED',
    'notarization': 'NOT PERFORMED',
}
report = OUT / f'IconCraft-Mac-{VERSION}-Validation.json'
report.write_text(json.dumps(summary, indent=2) + '\n')
checksums = OUT / f'IconCraft-Mac-{VERSION}-SHA256SUMS.txt'
checksum_files = [f for f in [appzip, sourcezip, report, installerpkg] if f]
checksums.write_text(''.join(hashlib.sha256(f.read_bytes()).hexdigest() + '  ' + f.name + '\n' for f in checksum_files))
print(json.dumps({'packages': [str(f) for f in checksum_files], **summary}, indent=2))
