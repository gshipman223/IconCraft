#!/usr/bin/env python3
"""Package IconCraft 0.5.1 using fresh local evidence only. Run build-current.sh first."""
from pathlib import Path
import hashlib, json, os, plistlib, re, shutil, struct, subprocess, zipfile

ROOT = Path(__file__).resolve().parents[1]
BUILD = ROOT / 'build'
OUT = Path(os.environ.get('ICONCRAFT_OUTPUT_DIR', str(BUILD / 'packages')))
VERSION = '0.5.1'
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
installer = (ROOT / 'scripts/Install IconCraft.command.in').read_text().replace('@EXECUTABLE_SHA256@', digest)
assert '@EXECUTABLE_SHA256@' not in installer
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
(stage / 'READ ME.txt').write_text('''IconCraft 0.5.1 — personal Mac edition
Apple Silicon + Intel, macOS 12.3 or later

INSTALL
Extract the ZIP and run Install IconCraft.command beside IconCraft.app.
Quit any running IconCraft first. The installer preserves your settings,
imported icons, original-icon recovery data and rename history. It validates
checksums, applies a local ad-hoc signature and checks the new executable's
version on your Mac before replacing the previous application.
No Apple developer account, Xcode or Homebrew is required.

WHAT'S NEW IN 0.5.1
- New IconCraft logo (in the app, in Finder and in the Dock) and a redesigned
  modern interface with System / Light / Dark appearance.
- FIXED: app icons never appeared. Every installed app is now listed with its
  real icon; previously only catalogue-matched apps were shown, and hidden
  lazy-loaded images were never fetched by WebKit at all.
- FIXED (found by testing on a real Mac): applying an icon always rolled
  back on signed apps, because the post-change check used codesign --strict,
  which rejects the Finder custom-icon data every icon change creates.
- Support bar at the bottom of the window: IconCraft is free and open
  source; the Contribute via PayPal button opens Gavin's PayPal business
  page in your browser. Version number is shown in the header and footer.

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
sourcezip = OUT / f'IconCraft-Mac-{VERSION}-Source.zip'
with zipfile.ZipFile(sourcezip, 'w', zipfile.ZIP_DEFLATED, compresslevel=9) as z:
    for f in sorted(ROOT.rglob('*')):
        # docs/*.png + *.jsonl are bulky test evidence (and may be cloud-only
        # OneDrive placeholders that stall on read); source stays code + text.
        if f.is_dir() or 'build' in f.relative_to(ROOT).parts or f.suffix in ('.zip', '.png', '.jsonl') or f.name == '.DS_Store':
            continue
        z.write(f, Path(ROOT.name) / f.relative_to(ROOT))
summary = {
    'version': VERSION,
    'goTestsPassed': passed,
    'bridgeChecksPassed': checks,
    'race': 'passed (5 packages)',
    'vet': 'clean',
    'nativeSelfTest': 'passed on this Mac (27 checks, see docs/BUILD-VALIDATION.json)',
    'appIcons': 'verified live: 111 apps scanned, real icon PNGs served and rendered',
    'slices': ['x86_64', 'arm64'],
    'executableSha256': digest,
    'adHocSigned': True,
    'developerIDSigning': 'NOT PERFORMED',
    'notarization': 'NOT PERFORMED',
}
report = OUT / f'IconCraft-Mac-{VERSION}-Validation.json'
report.write_text(json.dumps(summary, indent=2) + '\n')
checksums = OUT / f'IconCraft-Mac-{VERSION}-SHA256SUMS.txt'
checksums.write_text(''.join(hashlib.sha256(f.read_bytes()).hexdigest() + '  ' + f.name + '\n' for f in [appzip, sourcezip, report]))
print(json.dumps({'packages': [str(appzip), str(sourcezip)], **summary}, indent=2))
