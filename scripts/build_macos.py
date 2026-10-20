#!/usr/bin/env python3
"""Build Intel + Apple Silicon and produce a universal macOS app. Signing is separate."""
from pathlib import Path
import hashlib, json, os, plistlib, shutil, struct, subprocess, zipfile
ROOT=Path(__file__).resolve().parents[1]
OUT=ROOT/'build'
VERSION='0.8.0'

def run(args,**kw):subprocess.run(args,cwd=ROOT,check=True,**kw)
def build():
    OUT.mkdir(exist_ok=True)
    run(['go','test','./...'])
    pieces=[]
    for arch,cpu,subtype in [('amd64',0x01000007,3),('arm64',0x0100000c,0)]:
        target=OUT/('iconcraft-'+arch)
        env=dict(os.environ,GOOS='darwin',GOARCH=arch,CGO_ENABLED='0')
        run(['go','build','-trimpath','-ldflags=-s -w','-o',str(target),'./cmd/iconcraft'],env=env)
        b=target.read_bytes()
        assert b[:4]==b'\xcf\xfa\xed\xfe'
        assert struct.unpack_from('<I',b,4)[0]==cpu
        pieces.append((cpu,subtype,b))
    app=OUT/'IconCraft.app';contents=app/'Contents'
    if app.exists(): shutil.rmtree(app)
    (contents/'MacOS').mkdir(parents=True,exist_ok=True);(contents/'Resources').mkdir(exist_ok=True)
    exe=contents/'MacOS/IconCraft'
    signed=False
    if shutil.which('lipo') and shutil.which('codesign'):
        # Sign each thin slice first, then lipo: a hand-assembled fat binary
        # invalidates the Go linker signature location and codesign refuses
        # it ("main executable failed strict validation"). lipo preserves
        # valid per-slice signatures, so the bundle can actually be signed.
        for arch,_,_ in [('amd64',0,0),('arm64',0,0)]:
            run(['codesign','--force','--sign','-','--timestamp=none',str(OUT/('iconcraft-'+arch))])
        run(['lipo','-create',str(OUT/'iconcraft-amd64'),str(OUT/'iconcraft-arm64'),'-output',str(exe)])
        exe.chmod(0o755); signed=True
        fat=exe.read_bytes()
    else:
        # Non-macOS build host fallback: hand-assembled unsigned fat binary.
        align=14;offset=1<<align;entries=[]
        for cpu,sub,b in pieces:
            entries.append((cpu,sub,offset,len(b),align));offset=(offset+len(b)+(1<<align)-1)&~((1<<align)-1)
        fat=bytearray(offset);struct.pack_into('>II',fat,0,0xcafebabe,len(entries))
        for i,(entry,piece) in enumerate(zip(entries,pieces)):
            struct.pack_into('>IIIII',fat,8+i*20,*entry);fat[entry[2]:entry[2]+entry[3]]=piece[2]
        exe.write_bytes(fat);exe.chmod(0o755)
        for entry,piece in zip(entries,pieces):assert bytes(fat[entry[2]:entry[2]+entry[3]])==piece[2]
        fat=bytes(fat)
    metadata={'CFBundleName':'IconCraft','CFBundleDisplayName':'IconCraft','CFBundleIdentifier':'io.iconcraft.mac','CFBundleExecutable':'IconCraft','CFBundleIconFile':'icon.icns','CFBundlePackageType':'APPL','CFBundleShortVersionString':'0.8.0','CFBundleVersion':'12','LSMinimumSystemVersion':'12.3','CFBundleSupportedPlatforms':['MacOSX'],'NSHighResolutionCapable':True,'LSUIElement':True,'NSAppTransportSecurity':{'NSAllowsLocalNetworking':True},'NSHumanReadableCopyright':'Copyright © 2026 Gavin Shipman and IconCraft contributors. MIT license.'}
    (contents/'Info.plist').write_bytes(plistlib.dumps(metadata));(contents/'PkgInfo').write_bytes(b'APPL????')
    shutil.copy2(ROOT/'assets/icon.icns',contents/'Resources/icon.icns')
    shutil.copy2(ROOT/'assets/logo.svg',contents/'Resources/logo.svg')
    shutil.copy2(ROOT/'LICENSE',contents/'Resources/LICENSE')
    if signed:
        run(['codesign','--force','--sign','-','--timestamp=none',str(app)])
        run(['codesign','--verify','--deep','--strict',str(app)])
        fat=exe.read_bytes()
    record={'version':VERSION,'architectures':['x86_64','arm64'],'format':'Universal Mach-O app bundle','toolchain':subprocess.check_output(['go','version'],text=True).strip(),'nativeExecution':'see docs/BUILD-VALIDATION.json','adHocSigning':'PERFORMED on macOS build host' if signed else 'NOT PERFORMED (non-macOS host)','developerIDSigning':'NOT PERFORMED','notarization':'NOT PERFORMED','sha256Executable':hashlib.sha256(fat).hexdigest()}
    (OUT/'build-manifest.json').write_text(json.dumps(record,indent=2)+'\n')
    print(json.dumps(record,indent=2))
if __name__=='__main__':build()
