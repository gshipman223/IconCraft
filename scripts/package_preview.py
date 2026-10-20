#!/usr/bin/env python3
from pathlib import Path
import hashlib,json,os,shutil,stat,zipfile
ROOT=Path(__file__).resolve().parents[1]
OUT=ROOT/'build'
DEST=Path(os.environ.get('ICONCRAFT_OUTPUT',str(ROOT.parent)))
DEST.mkdir(exist_ok=True,parents=True)
STAGE=OUT/'Mac Preview'
if STAGE.exists():shutil.rmtree(STAGE)
STAGE.mkdir()
shutil.copytree(OUT/'IconCraft.app',STAGE/'IconCraft.app')
shutil.copy2(OUT/'build-manifest.json',STAGE/'build-manifest.json')
(STAGE/'READ-ME-FIRST.txt').write_text('''ICONCRAFT — MAC NATIVE-VALIDATION PREVIEW
0.2.0-mac-preview.1 — Apple Silicon + Intel universal .app

This is a newly compiled Mac build, not a production-certified release.
It has not yet been executed on macOS and is not Developer ID signed or notarized.
The accompanying validation report distinguishes tests run from tests still needed.

Open IconCraft.app on your Mac. It should run native diagnostics automatically
using disposable test apps. Changes to installed apps remain disabled until the
checks pass. Use Diagnostics & settings to export the results.

No Homebrew, Rust, Go, Node, Python, Xcode, VM, or commercial icon pack is needed
to run the app. macOS's supplied osascript, AppKit/WebKit and codesign are used.
The window-host behaviour and permissions still require a native Mac test.

The included Run Mac Diagnostics.command can run the same native fixture checks
without the GUI. Quit an existing IconCraft instance before running it.

This preview may be blocked by Gatekeeper. Do not disable Gatekeeper or strip
security attributes globally. A proper public release still needs native testing,
Developer ID signing, notarization and a downloaded-app launch test.

Replacement icons must be copied from installed original apps or imported as
original PNG/ICNS artwork. No generated replacement logos are included.

Your apps are not renamed, their executable resources are not patched, and no
third-party application is re-signed. Finder custom icons do not guarantee that
every running-app or system surface adopts the replacement.

Local state and recovery journals:
  ~/Library/Application Support/IconCraft/
Keep that folder to preserve backups. Restore desired changes before uninstalling.
''')
cmd=STAGE/'Run Mac Diagnostics.command'
cmd.write_text('''#!/bin/bash
set -uo pipefail
HERE="$(cd "$(dirname "$0")" && pwd)"
DIR="$HOME/Library/Application Support/IconCraft/reports"
mkdir -p "$DIR"
chmod 700 "$DIR"
REPORT="$DIR/IconCraft-mac-diagnostics-$(date +%Y%m%d-%H%M%S).json"
echo "Running disposable-fixture checks. Your installed applications will not be changed."
"$HERE/IconCraft.app/Contents/MacOS/IconCraft" --self-test --report "$REPORT"
RESULT=$?
echo
if [[ -f "$REPORT" ]]; then
  echo "Report saved to: $REPORT"
  /usr/bin/open -R "$REPORT"
fi
if [[ $RESULT -eq 0 ]]; then
  echo "Native fixture checks passed. Interactive Finder/Dock and signing checks remain."
else
  echo "Checks did not pass. Do not treat this preview as production-ready."
fi
echo "Press Return to close."
read -r _
exit "$RESULT"
''')
cmd.chmod(0o755)
rows=[json.loads(x) for x in (ROOT/'docs/test-results.jsonl').read_text().splitlines()]
passes=[x for x in rows if x.get('Test') and x.get('Action')=='pass']
ui=json.loads((ROOT/'docs/ui-test-results.json').read_text())
summary={'buildVersion':'0.2.0-mac-preview.1','buildEnvironment':'Linux x86_64','unitAndHTTPTestsPassed':len(passes),'raceDetector':'passed','goVet':'passed','offlineDOMChecksPassed':ui['passed'],'offlineDOMEnvironment':ui['environment'],'macNativeRuntimeTests':'NOT RUN — no Mac execution connection available','macNativeTestGate':'implemented; run on first Mac launch','finderDockSpotlightLaunchpadVisualTests':'NOT RUN','developerIDSigning':'NOT PERFORMED','notarization':'NOT PERFORMED','githubPublication':'NOT PERFORMED','productionReady':False,'artifacts':'Universal app includes byte-verified x86_64 and arm64 Mach-O slices.'}
(ROOT/'docs/BUILD-VALIDATION.json').write_text(json.dumps(summary,indent=2)+'\n')
shutil.copy2(ROOT/'docs/BUILD-VALIDATION.json',STAGE/'BUILD-VALIDATION.json')
def zip_tree(root,out,prefix='',exclude=set()):
    with zipfile.ZipFile(out,'w',zipfile.ZIP_DEFLATED,compresslevel=6) as z:
        for p in sorted(root.rglob('*')):
            rel=p.relative_to(root)
            if any(x in exclude for x in rel.parts) or not p.is_file():continue
            if p.suffix.lower() in {'.ttf','.otf','.woff','.woff2'}:raise RuntimeError('Font files must not be distributed')
            z.write(p,str(Path(prefix)/rel))
    with zipfile.ZipFile(out) as z:
        assert z.testzip() is None
zip_tree(STAGE,DEST/'IconCraft-mac-universal-preview.zip')
zip_tree(ROOT,DEST/'IconCraft-mac-source.zip','IconCraft',{'build','.git','__pycache__'})
shutil.copy2(ROOT/'docs/BUILD-VALIDATION.json',DEST/'IconCraft-mac-validation.json')
lines=[]
for filename in ['IconCraft-mac-universal-preview.zip','IconCraft-mac-source.zip','IconCraft-mac-validation.json']:
    p=DEST/filename;digest=hashlib.sha256(p.read_bytes()).hexdigest();lines.append(digest+'  '+filename);print(filename,p.stat().st_size,'bytes',digest)
(DEST/'IconCraft-mac-SHA256SUMS.txt').write_text('\n'.join(lines)+'\n')
print(json.dumps(summary,indent=2))
