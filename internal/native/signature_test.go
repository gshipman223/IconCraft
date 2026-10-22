package native

import (
	"errors"
	"os"
	"path/filepath"
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

func TestNotarizationVerdictUsesSpctlThenStapledTicket(t *testing.T) {
	confirmed := "/Applications/IconCraft.app: accepted\nsource=Notarized Developer ID\n"
	if ok, src := NotarizationVerdict(confirmed, false); !ok || src != "spctl: Notarized Developer ID" {
		t.Fatal(ok, src)
	}
	raw := "<key>assessment:authority:source</key>\n<string>Notarized Developer ID</string>\n"
	if ok, _ := NotarizationVerdict(raw, false); !ok {
		t.Fatal("--raw spelling not recognised")
	}
	// Gatekeeper could not reach the ticket service: a stapled ticket decides.
	offline := "/Applications/IconCraft.app: accepted\nsource=Developer ID\n"
	if ok, src := NotarizationVerdict(offline, true); !ok || src != "stapled ticket (spctl: Developer ID)" {
		t.Fatal(ok, src)
	}
	if ok, src := NotarizationVerdict(offline, false); ok || src != "spctl: Developer ID" {
		t.Fatal(ok, src)
	}
	// Timed out / empty output, but a ticket is stapled.
	if ok, src := NotarizationVerdict("", true); !ok || src != "stapled ticket" {
		t.Fatal(ok, src)
	}
	if ok, src := NotarizationVerdict("", false); ok || src != "spctl: no verdict" {
		t.Fatal(ok, src)
	}
	// An explicit rejection is never overridden by a stray ticket file.
	rejected := "/x/IconCraft.app: rejected\nsource=no usable signature\n"
	if ok, _ := NotarizationVerdict(rejected, true); ok {
		t.Fatal("rejected bundle reported as notarised")
	}
}

func TestHasStapledTicket(t *testing.T) {
	bundle := filepath.Join(t.TempDir(), "IconCraft.app")
	if err := os.MkdirAll(filepath.Join(bundle, "Contents"), 0700); err != nil {
		t.Fatal(err)
	}
	if hasStapledTicket(bundle) {
		t.Fatal("no ticket yet")
	}
	if err := os.WriteFile(filepath.Join(bundle, "Contents", "CodeResources"), []byte("ticket"), 0600); err != nil {
		t.Fatal(err)
	}
	if !hasStapledTicket(bundle) {
		t.Fatal("ticket not seen")
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
