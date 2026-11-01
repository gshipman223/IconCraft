#!/usr/bin/env python3
"""Build Intel + Apple Silicon and produce a universal macOS app. Signing is separate.

The Go slices use CGO_ENABLED=0, which cross-compiles on Linux. Go 1.23.2's
internal linker writes LC_BUILD_VERSION 11.0.0 for darwin/arm64 and
darwin/amd64 (cmd/link/internal/ld/macho.go, version = 11<<16). This script
raises a lower minos and sdk to 13.0.0 in each thin slice before lipo or the
hand-assembled fat header, and sets LSMinimumSystemVersion to 13.0.

scripts/defaults-helper.m is Objective-C (Foundation, AppKit,
UniformTypeIdentifiers, CoreServices). It is compiled only when clang and
lipo are on PATH. A Linux host has neither a macOS SDK nor those tools, so
the helper is not built here. scripts/release-macos.sh is the Mac release.
"""
from pathlib import Path
import hashlib, json, os, plistlib, shutil, struct, subprocess
ROOT=Path(__file__).resolve().parents[1]
OUT=ROOT/'build'
VERSION='0.14.1'
BUNDLE_VERSION='25'
MIN_MACOS='13.0'
MINOS=(13<<16)|(0<<8)|0
LC_BUILD_VERSION=0x32
MH_MAGIC_64=0xFEEDFACF
FAT_MAGIC=0xCAFEBABE

def run(args,**kw):subprocess.run(args,cwd=ROOT,check=True,**kw)

def decode_os_version(v):
    return ((v>>16)&0xffff,(v>>8)&0xff,v&0xff)

def slice_minos(data: bytes):
    """LC_BUILD_VERSION minos for a thin or fat Mach-O, one entry per slice."""
    if len(data)<8:
        raise SystemExit('Mach-O is too small')
    be=struct.unpack_from('>I', data, 0)[0]
    blobs=[]
    if be==FAT_MAGIC:
        n=struct.unpack_from('>I', data, 4)[0]
        if n<1:
            raise SystemExit('fat Mach-O has no slices')
        for i in range(n):
            _,_,offset,size,_=struct.unpack_from('>IIIII', data, 8+i*20)
            blobs.append(data[offset:offset+size])
    elif struct.unpack_from('<I', data, 0)[0]==MH_MAGIC_64:
        blobs.append(data)
    else:
        raise SystemExit(f'unrecognised Mach-O magic {be:#x}')
    out=[]
    for blob in blobs:
        if len(blob)<32 or struct.unpack_from('<I', blob, 0)[0]!=MH_MAGIC_64:
            raise SystemExit('slice is not a 64-bit Mach-O')
        _,_,_,_,ncmds,sizeofcmds,_,_=struct.unpack_from('<8I', blob, 0)
        off=32
        end=32+sizeofcmds
        got=None
        for _ in range(ncmds):
            if off+8>len(blob) or off+8>end:
                raise SystemExit('truncated Mach-O load commands')
            cmd,cmdsize=struct.unpack_from('<II', blob, off)
            if cmdsize<8 or off+cmdsize>len(blob):
                raise SystemExit('bad Mach-O command size')
            if cmd==LC_BUILD_VERSION and cmdsize>=24:
                got=struct.unpack_from('<I', blob, off+12)[0]
            off+=cmdsize
        if got is None:
            raise SystemExit('slice has no LC_BUILD_VERSION')
        out.append(got)
    return out

def raise_minos(data: bytes) -> bytes:
    if len(data)<32 or struct.unpack_from('<I', data, 0)[0]!=MH_MAGIC_64:
        raise SystemExit('Go output is not a 64-bit Mach-O')
    _,_,_,_,ncmds,sizeofcmds,_,_=struct.unpack_from('<8I', data, 0)
    off=32
    end=32+sizeofcmds
    buf=bytearray(data)
    found=0
    for _ in range(ncmds):
        if off+8>end or off+8>len(buf):
            raise SystemExit('truncated Mach-O load commands')
        cmd,cmdsize=struct.unpack_from('<II', buf, off)
        if cmdsize<8 or off+cmdsize>len(buf):
            raise SystemExit('bad Mach-O command size')
        if cmd==LC_BUILD_VERSION:
            if cmdsize<24:
                raise SystemExit('LC_BUILD_VERSION is too small')
            platform,minos,sdk,_=struct.unpack_from('<IIII', buf, off+8)
            if platform!=1:
                raise SystemExit(f'LC_BUILD_VERSION platform {platform} is not macOS')
            if minos<MINOS:
                struct.pack_into('<I', buf, off+12, MINOS)
            if sdk<MINOS:
                struct.pack_into('<I', buf, off+16, MINOS)
            found+=1
        off+=cmdsize
    if found!=1 or off!=end:
        raise SystemExit(f'expected one LC_BUILD_VERSION, found {found}; commands ended at {off}, header says {end}')
    mins=slice_minos(bytes(buf))
    if len(mins)!=1 or mins[0]<MINOS:
        raise SystemExit(f'minos {decode_os_version(mins[0])} is below 13.0.0')
    return bytes(buf)

def build():
    OUT.mkdir(exist_ok=True)
    run(['go','test','./...'])
    pieces=[]
    for arch,cpu,subtype in [('amd64',0x01000007,3),('arm64',0x0100000c,0)]:
        target=OUT/('iconcraft-'+arch)
        env=dict(os.environ,GOOS='darwin',GOARCH=arch,CGO_ENABLED='0')
        run(['go','build','-trimpath','-ldflags=-s -w','-o',str(target),'./cmd/iconcraft'],env=env)
        patched=raise_minos(target.read_bytes())
        target.write_bytes(patched)
        b=target.read_bytes()
        assert b[:4]==b'\xcf\xfa\xed\xfe'
        assert struct.unpack_from('<I',b,4)[0]==cpu
        assert slice_minos(b)[0]>=MINOS
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
        # The minos patch is applied before this signature.
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
    mins=slice_minos(fat)
    if len(mins)!=2 or any(v<MINOS for v in mins):
        raise SystemExit(f'universal executable minos {[decode_os_version(v) for v in mins]} is not 13.0.0 or newer')
    # Bundled helper that changes default apps through NSWorkspace. It must
    # live inside the bundle: LaunchServices refuses those writes from
    # unbundled processes (the JXA bridge gets -54 / silent no-ops).
    # Objective-C against the Apple SDK cannot be cross-compiled on Linux.
    helper_built=False
    if shutil.which('clang') and shutil.which('lipo'):
        parts=[]
        for arch in ['arm64','x86_64']:
            part=OUT/('iconcraft-defaults-'+arch)
            run(['clang','-fobjc-arc','-mmacosx-version-min=13.0','-arch',arch,'-framework','Foundation','-framework','AppKit','-framework','UniformTypeIdentifiers','-framework','CoreServices',str(ROOT/'scripts/defaults-helper.m'),'-o',str(part)])
            parts.append(str(part))
        helper=contents/'MacOS/iconcraft-defaults'
        run(['lipo','-create',*parts,'-output',str(helper)])
        helper.chmod(0o755)
        helper_mins=slice_minos(helper.read_bytes())
        if not helper_mins or any(v<MINOS for v in helper_mins):
            raise SystemExit(f'iconcraft-defaults minos {[decode_os_version(v) for v in helper_mins]} is below 13.0.0')
        if signed:
            run(['codesign','--force','--sign','-','--timestamp=none',str(helper)])
        helper_built=True
    metadata={'CFBundleName':'IconCraft','CFBundleDisplayName':'IconCraft','CFBundleIdentifier':'io.iconcraft.mac','CFBundleExecutable':'IconCraft','CFBundleIconFile':'icon.icns','CFBundlePackageType':'APPL','CFBundleShortVersionString':VERSION,'CFBundleVersion':BUNDLE_VERSION,'LSMinimumSystemVersion':MIN_MACOS,'CFBundleSupportedPlatforms':['MacOSX'],'NSHighResolutionCapable':True,'LSUIElement':True,'NSAppTransportSecurity':{'NSAllowsLocalNetworking':True},'NSHumanReadableCopyright':'Copyright © 2026 Gavin Shipman and IconCraft contributors. MIT license.'}
    (contents/'Info.plist').write_bytes(plistlib.dumps(metadata));(contents/'PkgInfo').write_bytes(b'APPL????')
    shutil.copy2(ROOT/'assets/icon.icns',contents/'Resources/icon.icns')
    shutil.copy2(ROOT/'assets/logo.svg',contents/'Resources/logo.svg')
    shutil.copy2(ROOT/'LICENSE',contents/'Resources/LICENSE')
    if signed:
        run(['codesign','--force','--sign','-','--timestamp=none',str(app)])
        run(['codesign','--verify','--deep','--strict',str(app)])
        fat=exe.read_bytes()
        mins=slice_minos(fat)
        if len(mins)!=2 or any(v<MINOS for v in mins):
            raise SystemExit('codesign changed the executable below macOS 13.0.0')
    record={'version':VERSION,'bundleVersion':BUNDLE_VERSION,'minimumSystemVersion':MIN_MACOS,'lcBuildVersionMinos':'13.0.0','architectures':['x86_64','arm64'],'format':'Universal Mach-O app bundle','toolchain':subprocess.check_output(['go','version'],text=True).strip(),'nativeExecution':'see docs/BUILD-VALIDATION.json','defaultsHelper':'BUILT (universal, -mmacosx-version-min=13.0, ad-hoc signed with the bundle)' if helper_built else 'NOT BUILT (clang and lipo are both required; scripts/defaults-helper.m is Objective-C for the macOS SDK and was not compiled on this host)','adHocSigning':'PERFORMED on macOS build host' if signed else 'NOT PERFORMED (non-macOS host)','developerIDSigning':'NOT PERFORMED','notarization':'NOT PERFORMED','sha256Executable':hashlib.sha256(fat).hexdigest()}
    (OUT/'build-manifest.json').write_text(json.dumps(record,indent=2)+'\n')
    print(json.dumps(record,indent=2))
if __name__=='__main__':build()
