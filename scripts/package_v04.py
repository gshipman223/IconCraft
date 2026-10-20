#!/usr/bin/env python3
"""Package the compiled Mac app and the evidence actually collected by this build."""
from pathlib import Path
import ast,hashlib,json,os,re,shutil,subprocess,zipfile
ROOT=Path(__file__).resolve().parents[1];BUILD=ROOT/'build'
OUT=Path(os.environ.get('ICONCRAFT_OUTPUT','/mnt/data'));OUT.mkdir(parents=True,exist_ok=True)
assert (BUILD/'BUILD-COMPLETE').is_file(), 'Build/check pipeline has not finished'
ui=json.loads((ROOT/'docs/v04-ui-results.json').read_text())
assert ui['passed']>=25
manifest=json.loads((BUILD/'build-manifest.json').read_text());assert manifest['version']=='0.4.0'
STAGE=BUILD/'IconCraft-Mac-0.4.0'
if STAGE.exists():shutil.rmtree(STAGE)
STAGE.mkdir();shutil.copytree(BUILD/'IconCraft.app',STAGE/'IconCraft.app')
exe=STAGE/'IconCraft.app/Contents/MacOS/IconCraft';exe.chmod(0o755)
digest=hashlib.sha256(exe.read_bytes()).hexdigest();assert digest==manifest['sha256Executable']
# Reuse the existing reversible installer without executing the legacy packager.
old=ast.parse((ROOT/'scripts/package_local_test.py').read_text());installer=None
for item in old.body:
 if isinstance(item,ast.Assign) and any(isinstance(t,ast.Name) and t.id=='installer' for t in item.targets) and isinstance(item.value,ast.Constant):installer=item.value.value;break
assert installer
installer=installer.replace('__EXECUTABLE_SHA256__',digest).replace('Mac local-test build','Mac app 0.4.0').replace('Preparing this test copy locally.','Preparing this copy locally.').replace('The first launch checks native icon handling using disposable test apps.','Saved apps appear first; new apps and original icons refresh in the background.').replace('this unsigned test build','this locally signed app')
p=STAGE/'Install IconCraft.command';p.write_text(installer);p.chmod(0o755);subprocess.run(['bash','-n',str(p)],check=True)
(STAGE/'APP-SHA256SUMS.txt').write_text(''.join(hashlib.sha256(p.read_bytes()).hexdigest()+'  '+p.relative_to(STAGE).as_posix()+'\n' for p in sorted((STAGE/'IconCraft.app').rglob('*')) if p.is_file()))
(STAGE/'READ ME.txt').write_text('''IconCraft 0.4.0 | Personal Mac edition | Apple Silicon + Intel | macOS 12.3+

Quit the old IconCraft, extract the ZIP, then run Install IconCraft.command.
Your data and previous application are preserved. No Apple developer account,
Xcode, Homebrew, VM or compiler is needed. Local ad-hoc signing is performed by
the installer; this is not Developer ID signing or notarization.

ONE PAGE
Saved apps and icons appear first; new apps and original artwork refresh in the
background. Each launch checks several official sources with short time limits.
A failed source does not erase a saved original. Unknown artwork is never drawn.

Replace / Restore per app. Select all / Replace selected. Restore all.
Grid button: choose another installed app's icon in a compact preview panel,
or optionally choose a custom PNG/ICNS file. Donor apps are not launched.
Pencil button: use the original counterpart's name or type a custom name.
System / Sun / Moon buttons: automatic, light or dark appearance, remembered.

Rename changes the .app filename in Finder, not signed bundle metadata or the
app's own internal menu titles. Quit the target app first. Its updater may
reinstall the old filename; Restore name reverses IconCraft's recorded renames.
Restore all also unwinds recorded icon/name changes. No global cache is purged.

Keep ~/Library/Application Support/IconCraft while Restore is needed.
The installer does not delete this folder, bypass Gatekeeper or remove quarantine.
Approve a blocked download individually in macOS Privacy & Security after
trying to open it. Do not disable macOS security globally.

Actual executable compiled for both Mac architectures. Automated core, contract
and simulated UI tests were run on Linux. Native macOS execution and live icon
retrieval from your Mac have not been tested in this build environment.
The previously mentioned replacement error was not supplied; this version shows
operation failures directly in the affected row rather than a diagnostics page.

MIT license for code. Vendor artwork remains the property of its owners.
''')
rows=[json.loads(x) for x in (BUILD/'tests-current.jsonl').read_text().splitlines()]
assert not any(x.get('Action')=='fail' for x in rows)
passes=sum(x.get('Action')=='pass' and 'Test' in x for x in rows)
tap=(BUILD/'bridge-regression.tap').read_text();m=re.search(r'^# pass (\d+)$',tap,re.M);assert m
assert '# fail 0' in tap
race=(BUILD/'race-current.txt').read_text();assert 'FAIL' not in race and race.count('\nok ')+race.startswith('ok ')>=4
assert not (BUILD/'vet-current.txt').read_text().strip()
summary={
 'version':'0.4.0','purpose':'Compact personal Mac app with cached startup, parallel original-icon lookup, appearance controls, in-window icon picker, and reversible app filename rename',
 'buildHost':manifest['toolchain'],'architectures':['arm64','x86_64'],
 'goTestsPassed':passes,'raceDetector':'passed','goVet':'passed',
 'nativeBridgeRegression':{'passed':int(m.group(1)),'environment':'Node.js, simulated Objective-C values, plus embedded window-script syntax check; not native macOS'},
 'ui':ui,'darwinTestCompilation':['arm64','amd64'],'nativeMacRuntime':'NOT RUN on Linux build host',
 'livePublisherDownloads':'NOT RUN successfully on build host; deterministic parallel source/failure/conditional-cache tests executed',
 'installerShellSyntax':'passed','developerIDSigning':'not performed','notarization':'not performed','localInstallerSigning':'ad-hoc on target Mac; no developer account',
 'renameSemantics':'Moves the outer .app filename only; internal menu titles are unchanged, and updater path assumptions may be affected',
 'previousUnspecifiedReplacementError':'No error text or screenshot supplied; exact cause not reproduced',
 'benchmark':{'environment':'Linux, 100 isolated mock application bundles, warm metadata cache; not a Mac launch-time measurement','output':(BUILD/'benchmark-current.txt').read_text().strip()},
 'sha256ExecutableBeforeLocalSigning':digest,'productionReady':False
}
# Replace stale reports, so the current source contains only current evidence.
for name in ['BUILD-VALIDATION.json','automated-test-output.txt','bridge-regression.tap','simple-ui-fixtures.png','simple-ui-test-results.json','test-results.jsonl','LOCAL-TEST-INSTALL.txt']:
 (ROOT/'docs'/name).unlink(missing_ok=True)
(ROOT/'docs/BUILD-VALIDATION.json').write_text(json.dumps(summary,indent=2)+'\n')
shutil.copy2(BUILD/'tests-current.jsonl',ROOT/'docs/test-results.jsonl');shutil.copy2(BUILD/'bridge-regression.tap',ROOT/'docs/bridge-regression.tap')
(ROOT/'docs/automated-test-output.txt').write_text(race+'\ngo vet: passed\n')
(STAGE/'BUILD-VALIDATION.json').write_text(json.dumps(summary,indent=2)+'\n')
def archive(path,base,prefix,source=False):
 with zipfile.ZipFile(path,'w',zipfile.ZIP_DEFLATED,compresslevel=9) as z:
  for p in sorted(base.rglob('*')):
   rel=p.relative_to(base)
   if source and (rel.parts[0] in ('build','.git') or '__pycache__' in rel.parts or p.suffix.lower() in ('.ttf','.otf','.woff','.woff2')):continue
   if p.is_file():z.write(p,str(Path(prefix)/rel))
 with zipfile.ZipFile(path) as z:assert z.testzip() is None
appzip=OUT/'IconCraft-Mac-0.4.0.zip';archive(appzip,STAGE,STAGE.name)
sourcezip=OUT/'IconCraft-Mac-0.4.0-Source.zip';archive(sourcezip,ROOT,'IconCraft',True)
summary['sha256Archive']=hashlib.sha256(appzip.read_bytes()).hexdigest()
report=OUT/'IconCraft-Mac-0.4.0-Validation.json';report.write_text(json.dumps(summary,indent=2)+'\n')
checks=OUT/'IconCraft-Mac-0.4.0-SHA256SUMS.txt';checks.write_text(''.join(hashlib.sha256(p.read_bytes()).hexdigest()+'  '+p.name+'\n' for p in [appzip,sourcezip,report]))
print(json.dumps({'app':str(appzip),'bytes':appzip.stat().st_size,'source':str(sourcezip),'report':str(report),'goTests':passes,'bridgeChecks':int(m.group(1)),'uiChecks':ui['passed']},indent=2))
