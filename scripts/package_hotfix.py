#!/usr/bin/env python3
"""Package the launch-focused 0.4.1 build using fresh local evidence only."""
from pathlib import Path
import hashlib,json,os,plistlib,re,shutil,struct,subprocess,zipfile
ROOT=Path(__file__).resolve().parents[1];BUILD=ROOT/'build';OUT=Path(os.environ.get('ICONCRAFT_OUTPUT_DIR',str(BUILD/'packages')));VERSION='0.4.1'
OUT.mkdir(parents=True,exist_ok=True)
assert (BUILD/'BUILD-COMPLETE').read_text().strip()=='OK'
manifest=json.loads((BUILD/'build-manifest.json').read_text());assert manifest['version']==VERSION
records=[json.loads(x) for x in (BUILD/'tests-current.jsonl').read_text().splitlines()]
assert not any(x.get('Action')=='fail' for x in records)
passed=sum(x.get('Action')=='pass' and 'Test' in x for x in records)
tap=(BUILD/'bridge-regression.tap').read_text();checks=int(re.search(r'^# pass (\d+)$',tap,re.M).group(1));assert '# fail 0' in tap
assert not (BUILD/'vet-current.txt').read_text().strip()
race=(BUILD/'race-current.txt').read_text();assert 'FAIL' not in race and len([l for l in race.splitlines() if l.startswith('ok')])==5
ui=json.loads((ROOT/'docs/v04-ui-results.json').read_text());assert ui['passed']==28
smoke=json.loads((BUILD/'host-smoke.json').read_text())
app=BUILD/'IconCraft.app';exe=app/'Contents/MacOS/IconCraft';raw=exe.read_bytes()
magic,n=struct.unpack_from('>II',raw);assert magic==0xcafebabe and n==2
slices=[]
for i in range(n):
 cpu,sub,offset,size,alignment=struct.unpack_from('>IIIII',raw,8+i*20)
 part=raw[offset:offset+size]
 assert part[:4]==b'\xcf\xfa\xed\xfe' and struct.unpack_from('<I',part,4)[0]==cpu
 assert offset%(1<<alignment)==0
 arch={0x01000007:'amd64',0x0100000c:'arm64'}[cpu]
 assert part==(BUILD/f'iconcraft-{arch}').read_bytes()
 slices.append({'architecture':arch,'bytes':size,'sha256':hashlib.sha256(part).hexdigest()})
info=plistlib.loads((app/'Contents/Info.plist').read_bytes());assert info['CFBundleShortVersionString']==VERSION
# Reuse the previously provided, non-destructive installer and add a real target-
# Mac executable probe before replacing a previous installation.
digest=hashlib.sha256(raw).hexdigest()
installer=(ROOT/'scripts/Install IconCraft.command.in').read_text().replace('@EXECUTABLE_SHA256@',digest)
assert '@EXECUTABLE_SHA256@' not in installer
stage=BUILD/f'IconCraft-Mac-{VERSION}'
if stage.exists():shutil.rmtree(stage)
stage.mkdir();shutil.copytree(app,stage/'IconCraft.app')
p=stage/'Install IconCraft.command';p.write_text(installer);p.chmod(0o755)
subprocess.run(['bash','-n',str(p)],check=True)
(stage/'APP-SHA256SUMS.txt').write_text(''.join(hashlib.sha256(p.read_bytes()).hexdigest()+'  '+p.relative_to(stage).as_posix()+'\n' for p in sorted((stage/'IconCraft.app').rglob('*')) if p.is_file()))
notes='''IconCraft 0.4.1 — Mac launch repair build
Apple Silicon + Intel, macOS 12.3 or later

INSTALL
Extract the ZIP and run Install IconCraft.command beside IconCraft.app.
Quit any running IconCraft first. The installer preserves the old application,
your settings, imported icons, original-icon recovery data and rename history.
It validates checksums, applies a local ad-hoc signature and checks the new
executable's version on your Mac before replacing the previous application.
No Apple developer account, Xcode or Homebrew is required.

WHAT CHANGED
The native appearance callback added in 0.4.0 is removed from window startup.
The main page still has System, Light and Dark controls and the same compact
layout, original-icon discovery, custom picker, bulk actions and renaming.
The native titlebar follows the Mac's system appearance independently of the
page's manual override. A Dock-artwork failure no longer prevents opening.

Startup failures now retain the native error and offer a small Show error
button. This is an error message, not a new diagnostics page. The report is
saved to ~/Library/Application Support/IconCraft/startup-error.txt. Nothing
resets the existing settings or recovery files.

If the app cannot open, click Show error and send the text it opens.
If no dialog appears, the existing startup log can be read in Terminal with:
tail -n 60 "$HOME/Library/Application Support/IconCraft/iconcraft.log"

The exact cause of the reported 0.4.0 failure is not confirmed: no target-Mac
launch log was available. This repair removes a startup dependency introduced
in that release and improves failure visibility. It is not a claim of native
macOS acceptance testing. See BUILD-VALIDATION.json for checks actually run.

Developer ID signing and notarization have not been performed. The installer
signs locally and does not disable Gatekeeper or remove quarantine attributes.
If macOS blocks the download, use Privacy & Security > Open Anyway to approve
this item. Do not turn off macOS security globally.
'''
(stage/'READ ME.txt').write_text(notes)
summary={
 'version':VERSION,'purpose':'Launch-focused repair build, retaining the 0.4 single-page application',
 'reportedFailureCause':'UNCONFIRMED: target-Mac launch error was not supplied',
 'changes':['Removed startup dependency on newly introduced native appearance callback','Kept System/Light/Dark in the page without native message posting','Made cosmetic Dock-artwork failure non-fatal','Preserved actual native stderr and startup stage in failure report','Log initialisation precedes state loading and instance-lock checks','Added startup-error notification and Show error action','Installer checks executable version on the target Mac before replacing prior app'],
 'buildHost':manifest['toolchain'],'architectures':slices,
 'goTestsPassed':passed,'raceDetector':'passed on Linux','goVet':'passed',
 'bridgeAndWindowContractTests':{'passed':checks,'environment':'Node.js on Linux with simulated Objective-C objects; NOT a native macOS test'},
 'ui':ui,'compiledHostSmoke':smoke,
 'darwinTestCompilation':['arm64','amd64'],
 'installerShellSyntax':'passed; installer NOT executed on macOS here',
 'nativeMacRuntime':'NOT RUN: build environment is Linux',
 'liveIconSources':'not revalidated by this launch-only repair',
 'developerIDSigning':'not performed','notarization':'not performed','targetInstallerSignature':'local ad-hoc; not a Developer ID signature',
 'sha256ExecutableBeforeLocalSigning':digest,'productionReady':False
}
(ROOT/'docs/BUILD-VALIDATION.json').write_text(json.dumps(summary,indent=2)+'\n')
(ROOT/'docs/CHANGES-0.4.1.md').write_text('# 0.4.1 — launch repair\n\n'+notes)
shutil.copy2(BUILD/'tests-current.jsonl',ROOT/'docs/test-results.jsonl')
shutil.copy2(BUILD/'bridge-regression.tap',ROOT/'docs/bridge-regression.tap')
(ROOT/'docs/automated-test-output.txt').write_text(race+'\ngo vet: passed\n')
(stage/'BUILD-VALIDATION.json').write_text(json.dumps(summary,indent=2)+'\n')
readme=ROOT/'README.md';text=readme.read_text();
if '## Current launch repair' not in text:
 text=text.replace('\n','\n\n## Current launch repair\n\nVersion **0.4.1** removes the native appearance hook from startup and adds visible startup errors. The exact 0.4.0 failure on the user Mac is unconfirmed. Native macOS launch testing has not been performed in this Linux environment. See `docs/CHANGES-0.4.1.md` and `docs/BUILD-VALIDATION.json`.\n',1)
 readme.write_text(text)
def archive(destination,base,prefix,source=False):
 with zipfile.ZipFile(destination,'w',zipfile.ZIP_DEFLATED,compresslevel=9) as z:
  for p in sorted(base.rglob('*')):
   rel=p.relative_to(base)
   if source and (rel.parts[0] in ('build','.git') or '__pycache__' in rel.parts or p.suffix.lower() in ('.ttf','.otf','.woff','.woff2')):continue
   if p.is_file():z.write(p,str(Path(prefix)/rel))
 with zipfile.ZipFile(destination) as z:
  assert z.testzip() is None
  if not source:
   perm=z.getinfo(prefix+'/IconCraft.app/Contents/MacOS/IconCraft').external_attr>>16
   assert perm&0o111==0o111
appzip=OUT/f'IconCraft-Mac-{VERSION}.zip';archive(appzip,stage,stage.name)
sourcezip=OUT/f'IconCraft-Mac-{VERSION}-Source.zip';archive(sourcezip,ROOT,'IconCraft',True)
summary['sha256Archive']=hashlib.sha256(appzip.read_bytes()).hexdigest()
report=OUT/f'IconCraft-Mac-{VERSION}-Validation.json';report.write_text(json.dumps(summary,indent=2)+'\n')
(OUT/f'IconCraft-Mac-{VERSION}-SHA256SUMS.txt').write_text(''.join(hashlib.sha256(p.read_bytes()).hexdigest()+'  '+p.name+'\n' for p in [appzip,sourcezip,report]))
print(json.dumps({'app':str(appzip),'bytes':appzip.stat().st_size,'source':str(sourcezip),'goTests':passed,'bridgeTests':checks,'uiTests':ui['passed'],'nativeMac':'NOT RUN'},indent=2))
