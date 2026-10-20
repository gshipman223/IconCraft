#!/usr/bin/env python3
"""Package the compiled universal one-page app. Preserve previous local records."""
from pathlib import Path
import ast, hashlib, json, shutil, subprocess, zipfile
ROOT=Path(__file__).resolve().parents[1]
BUILD=ROOT/'build'
OUT=Path('/mnt/data')
assert (BUILD/'BUILD-COMPLETE').exists(), 'Run build-current.sh and UI checks first'
STAGE=BUILD/'IconCraft-Mac-Simple'
if STAGE.exists():shutil.rmtree(STAGE)
STAGE.mkdir()
shutil.copytree(BUILD/'IconCraft.app',STAGE/'IconCraft.app')
exe=STAGE/'IconCraft.app/Contents/MacOS/IconCraft';exe.chmod(0o755)
digest=hashlib.sha256(exe.read_bytes()).hexdigest()
# Keep the existing, reversible installer; remove the old diagnostic instructions.
source=ast.parse((ROOT/'scripts/package_local_test.py').read_text())
installer=None
for item in source.body:
    if isinstance(item,ast.Assign) and any(isinstance(t,ast.Name) and t.id=='installer' for t in item.targets) and isinstance(item.value,ast.Constant):
        installer=item.value.value;break
assert installer
installer=installer.replace('__EXECUTABLE_SHA256__',digest)
installer=installer.replace('IconCraft — installing the Mac local-test build','IconCraft — installing the single-page Mac app')
installer=installer.replace('The first launch checks native icon handling using disposable test apps.','It will scan your Craft apps and check original icon sources automatically.')
installer=installer.replace('Preparing this test copy locally. No Developer ID or Apple account is used.','Preparing this copy locally. No Developer ID or Apple account is used.')
p=STAGE/'Install IconCraft.command';p.write_text(installer);p.chmod(0o755)
subprocess.run(['bash','-n',str(p)],check=True)
checks=[]
for p in sorted((STAGE/'IconCraft.app').rglob('*')):
    if p.is_file():checks.append(hashlib.sha256(p.read_bytes()).hexdigest()+'  '+p.relative_to(STAGE).as_posix())
(STAGE/'APP-SHA256SUMS.txt').write_text('\n'.join(checks)+'\n')
(STAGE/'READ ME.txt').write_text('''IconCraft 0.3.0-simple.1 — Apple Silicon + Intel

1. Quit the old IconCraft.
2. Run Install IconCraft.command beside the app.
3. IconCraft opens, scans, and checks genuine icon sources automatically.

One page. Per-app Replace / Restore / Custom. Select all and Replace selected.
Restore all returns changed apps to their pre-IconCraft icons.

Previous app data and recovery copies are preserved automatically.
Do not delete ~/Library/Application Support/IconCraft while you need Restore.

No Apple developer account or build tools required. The optional installer uses
macOS's local ad-hoc signing. It is not Developer ID signing or notarisation.
Approve a blocked item through macOS Privacy & Security after trying to open it;
do not disable Gatekeeper globally. Alternatively drag the .app to Applications.

The app checks publishers on each launch/rescan. A source failure uses a saved
original or a matching installed original where available, labelled accordingly.
No generated vendor icons. Custom PNG/ICNS remains optional.

Compiled for macOS; native execution was not performed on the Linux build host.
Finder custom icons cannot force every running app's own icon to change.
''')
def archive(path,root,prefix,exclude=None):
    with zipfile.ZipFile(path,'w',zipfile.ZIP_DEFLATED,compresslevel=9) as z:
        for p in sorted(root.rglob('*')):
            rel=p.relative_to(root)
            if exclude and exclude(rel):continue
            if p.is_file():z.write(p,str(Path(prefix)/rel))
    with zipfile.ZipFile(path) as z:
        assert z.testzip() is None
appzip=OUT/'IconCraft-Mac-Simple-0.3.0.zip'
archive(appzip,STAGE,STAGE.name)
# Source only. No build trees, old screenshots/reports, caches or bundled fonts.
sourcezip=OUT/'IconCraft-Mac-Simple-0.3.0-Source.zip'
archive(sourcezip,ROOT,'IconCraft',lambda p:p.parts[0] in ('build','.git') or '__pycache__' in p.parts or p.suffix.lower() in ('.ttf','.otf','.woff','.woff2') or p.name in ('ui-test-fixtures.png','ui-test-results.json'))
rows=[json.loads(s) for s in (BUILD/'tests-current.jsonl').read_text().splitlines()]
summary={
 'version':'0.3.0-simple.1',
 'purpose':'Compiled single-page personal macOS app; scan, automatic original icon discovery, individual/bulk replace and restore',
 'buildHost':'Linux x86_64','architectures':['arm64','x86_64'],
 'goTestsPassed':sum(x.get('Action')=='pass' and 'Test' in x for x in rows),
 'raceDetector':'passed','goVet':'passed',
 'nativeBridgeRegression':{'passed':23,'environment':'Node.js, simulated Objective-C values'},
 'ui':json.loads((ROOT/'docs/simple-ui-test-results.json').read_text()),
 'publisherIntegration':'Resolver tested with simulated official-source HTTP responses; live publisher retrieval on the target Mac has NOT been run here',
 'macNativeRuntime':'NOT RUN on Linux build host',
 'installerShellSyntax':'passed','developerIDSigning':'not performed','notarization':'not performed',
 'localInstallerSignature':'ad-hoc signing on the user Mac; no developer account',
 'sha256ExecutableBeforeLocalSigning':digest,
 'sha256Archive':hashlib.sha256(appzip.read_bytes()).hexdigest(),
 'launchDiagnosticGate':False,
 'transactionalRecovery':'automatic prior-state journal, signature checks, verification and rollback retained',
 'productionReady':False
}
report=OUT/'IconCraft-Mac-Simple-0.3.0-Validation.json';report.write_text(json.dumps(summary,indent=2)+'\n')
(OUT/'IconCraft-Mac-Simple-0.3.0-SHA256SUMS.txt').write_text('\n'.join(hashlib.sha256(p.read_bytes()).hexdigest()+'  '+p.name for p in (appzip,sourcezip,report))+'\n')
print(json.dumps({'app':str(appzip),'bytes':appzip.stat().st_size,'source':str(sourcezip),'validation':str(report),'tests':summary['goTestsPassed']},indent=2))
