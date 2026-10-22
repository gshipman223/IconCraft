package native

import (
	"errors"
	"testing"
)

const developerIDOutput = `Executable=/Applications/IconCraft.app/Contents/MacOS/IconCraft
Identifier=io.iconcraft.mac
Format=app bundle with Mach-O universal (x86_64 arm64)
CodeDirectory v=20500 size=1234 flags=0x10000(runtime) hashes=30+7 location=embedded
Signature size=9000
Authority=Developer ID Application: Gavin Shipman (ABCDE12345)
Authority=Developer ID Certification Authority
Authority=Apple Root CA
Timestamp=21 Oct 2026 at 10:00:00 pm
Info.plist entries=14
TeamIdentifier=ABCDE12345
Runtime Version=14.0.0
Sealed Resources version=2 rules=13 files=4
Internal requirements count=1 size=180
`

const adHocOutput = `Executable=/Users/me/build/IconCraft.app/Contents/MacOS/IconCraft
Identifier=io.iconcraft.mac
Format=app bundle with Mach-O universal (x86_64 arm64)
CodeDirectory v=20400 size=1234 flags=0x2(adhoc) hashes=30+7 location=embedded
Signature=adhoc
Info.plist entries=14
TeamIdentifier=not set
Sealed Resources version=2 rules=13 files=4
Internal requirements count=1 size=180
`

func TestParseCodesignClassifiesSignatures(t *testing.T) {
	dev := ParseCodesign(developerIDOutput, nil)
	if !dev.DeveloperID() || dev.Team != "ABCDE12345" || dev.Authority != "Developer ID Application: Gavin Shipman (ABCDE12345)" {
		t.Fatalf("developer id: %+v", dev)
	}
	adhoc := ParseCodesign(adHocOutput, nil)
	if adhoc.Kind != "adhoc" || adhoc.Team != "" || adhoc.DeveloperID() {
		t.Fatalf("adhoc: %+v", adhoc)
	}
	unsigned := ParseCodesign("/x/Foo.app: code object is not signed at all\n", errors.New("exit status 1"))
	if unsigned.Kind != "unsigned" {
		t.Fatalf("unsigned: %+v", unsigned)
	}
	devCert := ParseCodesign("Authority=Apple Development: Someone (XYZ)\nAuthority=Apple Worldwide Developer Relations Certification Authority\nTeamIdentifier=XYZ\n", nil)
	if devCert.Kind != "development" {
		t.Fatalf("development: %+v", devCert)
	}
	if got := ParseCodesign("", errors.New("exec: not found")); got.Kind != "unknown" {
		t.Fatalf("unknown: %+v", got)
	}
}

func TestParseSpctlNotarized(t *testing.T) {
	notarized := "/Applications/IconCraft.app: accepted\nsource=Notarized Developer ID\norigin=Developer ID Application: Gavin Shipman (ABCDE12345)\n"
	if !ParseSpctlNotarized(notarized) {
		t.Fatal("notarised verdict missed")
	}
	plain := "/Applications/IconCraft.app: accepted\nsource=Developer ID\norigin=Developer ID Application: Gavin Shipman (ABCDE12345)\n"
	if ParseSpctlNotarized(plain) {
		t.Fatal("un-notarised Developer ID reported as notarised")
	}
	if ParseSpctlNotarized("/x/IconCraft.app: rejected\nsource=no usable signature\n") {
		t.Fatal("rejected reported as notarised")
	}
}

func TestBundlePath(t *testing.T) {
	if got := bundlePath("/Applications/IconCraft.app/Contents/MacOS/IconCraft"); got != "/Applications/IconCraft.app" {
		t.Fatal(got)
	}
	for _, p := range []string{"/usr/local/bin/iconcraft", "/tmp/build/iconcraft-arm64", "/Applications/IconCraft.app/Contents/Resources/x"} {
		if got := bundlePath(p); got != "" {
			t.Fatalf("%s -> %q", p, got)
		}
	}
}
